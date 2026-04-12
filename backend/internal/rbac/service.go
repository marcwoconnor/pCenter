package rbac

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// AncestryResolver resolves an object's ancestors in the hierarchy.
// Returns the chain from most specific to root, e.g.:
//
//	VM 102 → [{node, pve04}, {cluster, default}, {datacenter, abc123}, {root, ""}]
type AncestryResolver interface {
	GetAncestors(objectType, objectID string) []ObjectRef
}

// Service provides RBAC permission checking
type Service struct {
	db       *DB
	resolver AncestryResolver

	// Cache: userID → []cachedAssignment
	mu    sync.RWMutex
	cache map[string]*userCache
}

type userCache struct {
	assignments []RoleAssignment
	roles       map[string]*Role // roleID → Role
	loadedAt    time.Time
}

const cacheTTL = 30 * time.Second

// NewService creates an RBAC service
func NewService(db *DB, resolver AncestryResolver) *Service {
	return &Service{
		db:       db,
		resolver: resolver,
		cache:    make(map[string]*userCache),
	}
}

// DB returns the underlying database for direct CRUD operations
func (s *Service) DB() *DB {
	return s.db
}

// InvalidateUser clears the cache for a specific user
func (s *Service) InvalidateUser(userID string) {
	s.mu.Lock()
	delete(s.cache, userID)
	s.mu.Unlock()
}

// InvalidateAll clears the entire cache
func (s *Service) InvalidateAll() {
	s.mu.Lock()
	s.cache = make(map[string]*userCache)
	s.mu.Unlock()
}

// HasPermission checks if a user has a specific permission on an object.
// It walks up the object hierarchy checking for role assignments.
func (s *Service) HasPermission(userID, permission, objectType, objectID string) bool {
	uc := s.getUserCache(userID)
	if uc == nil || len(uc.assignments) == 0 {
		return false
	}

	// Build the ancestry chain: the object itself + all ancestors
	chain := []ObjectRef{{Type: objectType, ID: objectID}}
	if s.resolver != nil {
		chain = append(chain, s.resolver.GetAncestors(objectType, objectID)...)
	}
	// Always include root
	hasRoot := false
	for _, ref := range chain {
		if ref.Type == ObjectRoot {
			hasRoot = true
			break
		}
	}
	if !hasRoot {
		chain = append(chain, ObjectRef{Type: ObjectRoot, ID: ""})
	}

	// Check each level in the hierarchy
	for i, ref := range chain {
		for _, assignment := range uc.assignments {
			if assignment.ObjectType != ref.Type || assignment.ObjectID != ref.ID {
				continue
			}

			// If this assignment is NOT on the direct object, check propagate flag
			if i > 0 && !assignment.Propagate {
				continue
			}

			// Check if the assigned role grants the requested permission
			role, ok := uc.roles[assignment.RoleID]
			if !ok {
				continue
			}

			if roleHasPermission(role, permission) {
				return true
			}
		}
	}

	return false
}

// HasAnyPermission checks if a user has at least one of the given permissions
func (s *Service) HasAnyPermission(userID string, permissions []string, objectType, objectID string) bool {
	for _, p := range permissions {
		if s.HasPermission(userID, p, objectType, objectID) {
			return true
		}
	}
	return false
}

// GetEffectivePermissions returns all permissions a user has on an object
func (s *Service) GetEffectivePermissions(userID, objectType, objectID string) []string {
	permSet := make(map[string]bool)
	for _, p := range AllPermissions {
		if s.HasPermission(userID, p, objectType, objectID) {
			permSet[p] = true
		}
	}

	perms := make([]string, 0, len(permSet))
	for p := range permSet {
		perms = append(perms, p)
	}
	return perms
}

// IsAdmin checks if a user has the Admin role at root level
func (s *Service) IsAdmin(userID string) bool {
	return s.HasPermission(userID, PermAll, ObjectRoot, "")
}

// getUserCache loads (or returns cached) user assignments and roles
func (s *Service) getUserCache(userID string) *userCache {
	s.mu.RLock()
	uc, ok := s.cache[userID]
	s.mu.RUnlock()

	if ok && time.Since(uc.loadedAt) < cacheTTL {
		return uc
	}

	// Load from DB
	assignments, roles, err := s.db.GetUserAssignments(userID)
	if err != nil {
		slog.Warn("rbac: failed to load user assignments", "user", userID, "error", err)
		return nil
	}

	roleMap := make(map[string]*Role, len(roles))
	for i := range roles {
		roleMap[roles[i].ID] = &roles[i]
	}

	uc = &userCache{
		assignments: assignments,
		roles:       roleMap,
		loadedAt:    time.Now(),
	}

	s.mu.Lock()
	s.cache[userID] = uc
	s.mu.Unlock()

	return uc
}

// roleHasPermission checks if a role grants a specific permission
func roleHasPermission(role *Role, permission string) bool {
	for _, p := range role.Permissions {
		if matchPermission(p, permission) {
			return true
		}
	}
	return false
}

// matchPermission checks if a granted permission matches a requested permission.
//
//	"*" matches everything
//	"vm.*" matches "vm.power", "vm.console", etc.
//	"vm.power" matches "vm.power" exactly
func matchPermission(granted, requested string) bool {
	if granted == PermAll {
		return true
	}
	if granted == requested {
		return true
	}
	// Wildcard: "vm.*" matches "vm.power"
	if strings.HasSuffix(granted, ".*") {
		prefix := strings.TrimSuffix(granted, ".*")
		if strings.HasPrefix(requested, prefix+".") {
			return true
		}
	}
	return false
}
