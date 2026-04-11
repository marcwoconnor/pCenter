package folders

import (
	"context"
	"fmt"
)

// Service provides folder operations with business logic
type Service struct {
	db *DB
}

// NewService creates a new folder service
func NewService(db *DB) *Service {
	return &Service{db: db}
}

// GetFolderTree returns the complete folder tree for a view with members populated
func (s *Service) GetFolderTree(ctx context.Context, treeView TreeView) ([]Folder, error) {
	// Get all folders
	folders, err := s.db.GetFolderTree(ctx, treeView)
	if err != nil {
		return nil, fmt.Errorf("get folders: %w", err)
	}

	// Get all members grouped by folder
	membersByFolder, err := s.db.GetAllMembers(ctx, treeView)
	if err != nil {
		return nil, fmt.Errorf("get members: %w", err)
	}

	// Build folder map for tree construction
	folderMap := make(map[string]*Folder)
	for i := range folders {
		folders[i].Members = membersByFolder[folders[i].ID]
		folders[i].Children = []Folder{} // Initialize to empty slice
		folderMap[folders[i].ID] = &folders[i]
	}

	// Build tree structure
	var roots []Folder
	for i := range folders {
		f := &folders[i]
		if f.ParentID == nil {
			roots = append(roots, *f)
		} else if parent, ok := folderMap[*f.ParentID]; ok {
			parent.Children = append(parent.Children, *f)
		} else {
			// Orphaned folder (parent deleted?) - treat as root
			roots = append(roots, *f)
		}
	}

	// Re-populate children from map (since we modified map entries)
	return s.buildTreeFromMap(folderMap, roots), nil
}

// buildTreeFromMap recursively builds the tree with proper nesting
func (s *Service) buildTreeFromMap(folderMap map[string]*Folder, folders []Folder) []Folder {
	result := make([]Folder, 0, len(folders))
	for _, f := range folders {
		folder := *folderMap[f.ID]
		if len(folder.Children) > 0 {
			folder.Children = s.buildTreeFromMap(folderMap, folder.Children)
		}
		result = append(result, folder)
	}
	return result
}

// CreateFolder creates a new folder with validation
func (s *Service) CreateFolder(ctx context.Context, req CreateFolderRequest) (*Folder, error) {
	// Validate tree view
	if req.TreeView != TreeViewHosts && req.TreeView != TreeViewVMs {
		return nil, fmt.Errorf("invalid tree_view: must be 'hosts' or 'vms'")
	}

	// Validate name
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Validate parent exists if specified
	if req.ParentID != nil {
		parent, err := s.db.GetFolder(ctx, *req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("check parent: %w", err)
		}
		if parent == nil {
			return nil, fmt.Errorf("parent folder not found")
		}
		if parent.TreeView != req.TreeView {
			return nil, fmt.Errorf("parent folder is in different tree view")
		}
	}

	return s.db.CreateFolder(ctx, req)
}

// RenameFolder renames a folder
func (s *Service) RenameFolder(ctx context.Context, id, newName string) error {
	if newName == "" {
		return fmt.Errorf("name is required")
	}

	folder, err := s.db.GetFolder(ctx, id)
	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}
	if folder == nil {
		return fmt.Errorf("folder not found")
	}

	return s.db.RenameFolder(ctx, id, newName)
}

// MoveFolder moves a folder to a new parent
func (s *Service) MoveFolder(ctx context.Context, id string, newParentID *string) error {
	folder, err := s.db.GetFolder(ctx, id)
	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}
	if folder == nil {
		return fmt.Errorf("folder not found")
	}

	// Validate new parent
	if newParentID != nil {
		// Can't move to self
		if *newParentID == id {
			return fmt.Errorf("cannot move folder into itself")
		}

		parent, err := s.db.GetFolder(ctx, *newParentID)
		if err != nil {
			return fmt.Errorf("check parent: %w", err)
		}
		if parent == nil {
			return fmt.Errorf("parent folder not found")
		}
		if parent.TreeView != folder.TreeView {
			return fmt.Errorf("cannot move folder to different tree view")
		}

		// Check for circular reference (can't move to a descendant)
		if s.isDescendant(ctx, *newParentID, id) {
			return fmt.Errorf("cannot move folder into its own descendant")
		}
	}

	return s.db.MoveFolder(ctx, id, newParentID)
}

// isDescendant checks if potentialDescendant is a descendant of ancestorID
func (s *Service) isDescendant(ctx context.Context, potentialDescendant, ancestorID string) bool {
	folder, err := s.db.GetFolder(ctx, potentialDescendant)
	if err != nil || folder == nil {
		return false
	}

	if folder.ParentID == nil {
		return false
	}

	if *folder.ParentID == ancestorID {
		return true
	}

	return s.isDescendant(ctx, *folder.ParentID, ancestorID)
}

// DeleteFolder deletes a folder
func (s *Service) DeleteFolder(ctx context.Context, id string) error {
	folder, err := s.db.GetFolder(ctx, id)
	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}
	if folder == nil {
		return fmt.Errorf("folder not found")
	}

	return s.db.DeleteFolder(ctx, id)
}

// AddMember adds a resource to a folder
func (s *Service) AddMember(ctx context.Context, folderID string, req AddMemberRequest) error {
	folder, err := s.db.GetFolder(ctx, folderID)
	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}
	if folder == nil {
		return fmt.Errorf("folder not found")
	}

	// Validate resource type
	switch req.ResourceType {
	case "vm", "ct", "node", "storage":
		// OK
	default:
		return fmt.Errorf("invalid resource_type: must be vm, ct, node, or storage")
	}

	return s.db.AddMember(ctx, folderID, req)
}

// RemoveMember removes a resource from a folder
func (s *Service) RemoveMember(ctx context.Context, folderID string, req RemoveMemberRequest) error {
	return s.db.RemoveMember(ctx, folderID, req)
}

// MoveResource moves a resource to a folder
func (s *Service) MoveResource(ctx context.Context, req MoveResourceRequest, treeView TreeView) error {
	// Validate resource type
	switch req.ResourceType {
	case "vm", "ct", "node", "storage":
		// OK
	default:
		return fmt.Errorf("invalid resource_type: must be vm, ct, node, or storage")
	}

	// Validate target folder if specified
	if req.ToFolderID != nil {
		folder, err := s.db.GetFolder(ctx, *req.ToFolderID)
		if err != nil {
			return fmt.Errorf("get folder: %w", err)
		}
		if folder == nil {
			return fmt.Errorf("target folder not found")
		}
		if folder.TreeView != treeView {
			return fmt.Errorf("target folder is in different tree view")
		}
	}

	return s.db.MoveResource(ctx, treeView, req.ResourceType, req.ResourceID, req.Cluster, req.ToFolderID)
}

// GetResourceFolder gets which folder a resource is in
func (s *Service) GetResourceFolder(ctx context.Context, treeView TreeView, resourceType, resourceID, cluster string) (*string, error) {
	return s.db.GetResourceFolder(ctx, treeView, resourceType, resourceID, cluster)
}
