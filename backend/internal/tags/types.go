package tags

import "time"

// Tag represents a user-defined tag with category and color
type Tag struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"`   // Environment, Owner, Purpose, etc.
	Name      string    `json:"name"`
	Color     string    `json:"color"`      // hex color e.g. "#3b82f6"
	CreatedAt time.Time `json:"created_at"`
}

// TagAssignment links a tag to a resource (VM, CT, node, storage)
type TagAssignment struct {
	TagID      string `json:"tag_id"`
	ObjectType string `json:"object_type"` // vm, ct, node, storage
	ObjectID   string `json:"object_id"`   // VMID or node name
	Cluster    string `json:"cluster"`
}

// CreateTagRequest is the body for POST /api/tags
type CreateTagRequest struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Color    string `json:"color"`
}

// UpdateTagRequest is the body for PUT /api/tags/{id}
type UpdateTagRequest struct {
	Category string `json:"category,omitempty"`
	Name     string `json:"name,omitempty"`
	Color    string `json:"color,omitempty"`
}

// AssignTagRequest is the body for POST /api/tags/assign
type AssignTagRequest struct {
	TagID      string `json:"tag_id"`
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	Cluster    string `json:"cluster"`
}

// UnassignTagRequest is the body for DELETE /api/tags/assign
type UnassignTagRequest struct {
	TagID      string `json:"tag_id"`
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	Cluster    string `json:"cluster"`
}

// BulkAssignRequest assigns one or more tags to one or more objects
type BulkAssignRequest struct {
	TagIDs  []string     `json:"tag_ids"`
	Objects []ObjectRef  `json:"objects"`
}

// ObjectRef identifies a resource
type ObjectRef struct {
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	Cluster    string `json:"cluster"`
}

// Default categories
var DefaultCategories = []string{
	"Environment",
	"Owner",
	"Purpose",
	"Tier",
	"Application",
}

// Default colors for quick assignment
var DefaultColors = []string{
	"#3b82f6", // blue
	"#10b981", // green
	"#f59e0b", // amber
	"#ef4444", // red
	"#8b5cf6", // purple
	"#ec4899", // pink
	"#06b6d4", // cyan
	"#f97316", // orange
}
