package api

import (
	"encoding/json"
	"net/http"

	"github.com/moconnor/pcenter/internal/auth"
	"github.com/moconnor/pcenter/internal/rbac"
)

// requirePermission checks if the current user has a permission on an object.
// Returns false and writes a 403 error if denied.
func (h *Handler) requirePermission(w http.ResponseWriter, r *http.Request, permission, objectType, objectID string) bool {
	if h.rbac == nil {
		return true // RBAC not configured, allow all
	}

	_, user := auth.GetAuthContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}

	// Admin role in legacy auth system always passes
	if user.Role == auth.RoleAdmin {
		return true
	}

	if !h.rbac.HasPermission(user.ID, permission, objectType, objectID) {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return false
	}
	return true
}

// --- RBAC API Handlers ---

// GetRoles returns all roles (built-in + custom)
func (h *Handler) GetRoles(w http.ResponseWriter, r *http.Request) {
	if h.rbac == nil {
		writeError(w, http.StatusNotImplemented, "RBAC not enabled")
		return
	}

	roles, err := h.rbac.DB().ListRoles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if roles == nil {
		roles = []rbac.Role{}
	}
	writeJSON(w, roles)
}

// CreateRole creates a custom role
func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	if h.rbac == nil {
		writeError(w, http.StatusNotImplemented, "RBAC not enabled")
		return
	}

	var req rbac.CreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || len(req.Permissions) == 0 {
		writeError(w, http.StatusBadRequest, "name and permissions are required")
		return
	}

	role, err := h.rbac.DB().CreateRole(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, role)
}

// UpdateRole updates a custom role
func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	if h.rbac == nil {
		writeError(w, http.StatusNotImplemented, "RBAC not enabled")
		return
	}

	id := r.PathValue("id")
	var req rbac.UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.rbac.DB().UpdateRole(id, req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.rbac.InvalidateAll()

	writeJSON(w, map[string]string{"status": "ok"})
}

// DeleteRole deletes a custom role
func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	if h.rbac == nil {
		writeError(w, http.StatusNotImplemented, "RBAC not enabled")
		return
	}

	id := r.PathValue("id")
	if err := h.rbac.DB().DeleteRole(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.rbac.InvalidateAll()

	writeJSON(w, map[string]string{"status": "ok"})
}

// GetRoleAssignments returns role assignments, optionally filtered
func (h *Handler) GetRoleAssignments(w http.ResponseWriter, r *http.Request) {
	if h.rbac == nil {
		writeError(w, http.StatusNotImplemented, "RBAC not enabled")
		return
	}

	userID := r.URL.Query().Get("user_id")
	objectType := r.URL.Query().Get("object_type")
	objectID := r.URL.Query().Get("object_id")

	assignments, err := h.rbac.DB().ListAssignments(userID, objectType, objectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if assignments == nil {
		assignments = []rbac.RoleAssignment{}
	}
	writeJSON(w, assignments)
}

// CreateRoleAssignment assigns a role to a user on an object
func (h *Handler) CreateRoleAssignment(w http.ResponseWriter, r *http.Request) {
	if h.rbac == nil {
		writeError(w, http.StatusNotImplemented, "RBAC not enabled")
		return
	}

	var req rbac.AssignRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" || req.RoleID == "" || req.ObjectType == "" {
		writeError(w, http.StatusBadRequest, "user_id, role_id, and object_type are required")
		return
	}

	assignment, err := h.rbac.DB().CreateAssignment(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.rbac.InvalidateUser(req.UserID)

	writeJSON(w, assignment)
}

// DeleteRoleAssignment removes a role assignment
func (h *Handler) DeleteRoleAssignment(w http.ResponseWriter, r *http.Request) {
	if h.rbac == nil {
		writeError(w, http.StatusNotImplemented, "RBAC not enabled")
		return
	}

	id := r.PathValue("id")
	if err := h.rbac.DB().DeleteAssignment(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.rbac.InvalidateAll() // assignment deletion could affect any user

	writeJSON(w, map[string]string{"status": "ok"})
}

// GetMyPermissions returns the current user's effective permissions on an object
func (h *Handler) GetMyPermissions(w http.ResponseWriter, r *http.Request) {
	if h.rbac == nil {
		// No RBAC = everything allowed
		writeJSON(w, rbac.AllPermissions)
		return
	}

	_, user := auth.GetAuthContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Legacy admin gets all permissions
	if user.Role == auth.RoleAdmin {
		writeJSON(w, rbac.AllPermissions)
		return
	}

	objectType := r.URL.Query().Get("object_type")
	objectID := r.URL.Query().Get("object_id")
	if objectType == "" {
		objectType = rbac.ObjectRoot
	}

	perms := h.rbac.GetEffectivePermissions(user.ID, objectType, objectID)
	if perms == nil {
		perms = []string{}
	}
	writeJSON(w, perms)
}

// GetAllPermissions returns the list of all defined permissions (for UI dropdowns)
func (h *Handler) GetAllPermissions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, rbac.AllPermissions)
}

// GetVersion returns current version and update availability
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	if h.updater == nil {
		writeJSON(w, map[string]string{"current_version": "unknown"})
		return
	}
	writeJSON(w, h.updater.GetUpdateInfo())
}
