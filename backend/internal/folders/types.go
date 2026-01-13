package folders

import "time"

// TreeView identifies which navigation tree a folder belongs to
type TreeView string

const (
	TreeViewHosts TreeView = "hosts"
	TreeViewVMs   TreeView = "vms"
)

// Folder represents a user-created organizational folder
type Folder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parent_id,omitempty"`
	TreeView  TreeView  `json:"tree_view"`
	Cluster   *string   `json:"cluster,omitempty"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Computed fields for tree response
	Children []Folder       `json:"children,omitempty"`
	Members  []FolderMember `json:"members,omitempty"`
}

// FolderMember represents a resource assigned to a folder
type FolderMember struct {
	FolderID     string    `json:"folder_id"`
	ResourceType string    `json:"resource_type"` // vm, ct, node, storage
	ResourceID   string    `json:"resource_id"`
	Cluster      string    `json:"cluster"`
	AddedAt      time.Time `json:"added_at"`
}

// CreateFolderRequest is the request body for creating a folder
type CreateFolderRequest struct {
	Name     string   `json:"name"`
	ParentID *string  `json:"parent_id,omitempty"`
	TreeView TreeView `json:"tree_view"`
	Cluster  *string  `json:"cluster,omitempty"`
}

// RenameFolderRequest is the request body for renaming a folder
type RenameFolderRequest struct {
	Name string `json:"name"`
}

// MoveFolderRequest is the request body for moving a folder
type MoveFolderRequest struct {
	ParentID *string `json:"parent_id"` // nil = move to root
}

// AddMemberRequest is the request body for adding a resource to a folder
type AddMemberRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Cluster      string `json:"cluster"`
}

// RemoveMemberRequest is the request body for removing a resource from a folder
type RemoveMemberRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Cluster      string `json:"cluster"`
}

// MoveResourceRequest is the request body for moving a resource to a folder
type MoveResourceRequest struct {
	ResourceType string  `json:"resource_type"`
	ResourceID   string  `json:"resource_id"`
	Cluster      string  `json:"cluster"`
	ToFolderID   *string `json:"to_folder_id"` // nil = move to root (unorganized)
}
