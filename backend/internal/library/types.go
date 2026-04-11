package library

import "time"

// ItemType categorizes content library entries
type ItemType string

const (
	ItemTypeISO          ItemType = "iso"
	ItemTypeVZTemplate   ItemType = "vztmpl"
	ItemTypeVMTemplate   ItemType = "vm-template"
	ItemTypeOVA          ItemType = "ova"
	ItemTypeSnippet      ItemType = "snippet"
)

// Item represents a content library entry with metadata
type Item struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Type        ItemType `json:"type"`
	Category    string   `json:"category,omitempty"` // e.g. "Linux", "Windows", "Appliance"
	Version     string   `json:"version,omitempty"`
	Tags        []string `json:"tags"`

	// Source location (where the actual file/template lives)
	Cluster string `json:"cluster"`          // Proxmox cluster name
	Node    string `json:"node,omitempty"`   // Node where stored (optional, for shared storage)
	Storage string `json:"storage"`          // Storage pool name
	Volume  string `json:"volume"`           // Volume ID (e.g. "local:iso/ubuntu-24.04.iso")

	// File info
	Size   int64  `json:"size"`              // Bytes
	Format string `json:"format,omitempty"`  // qcow2, raw, iso, tar.gz, etc.

	// VM template specifics (when type=vm-template)
	VMID    int    `json:"vmid,omitempty"`    // Source VM ID (for template cloning)
	OSType  string `json:"os_type,omitempty"` // l26, win10, etc.
	Cores   int    `json:"cores,omitempty"`
	Memory  int    `json:"memory,omitempty"`  // MB

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by,omitempty"`
}

// CreateItemRequest is the API request to add a library item
type CreateItemRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Type        ItemType `json:"type"`
	Category    string   `json:"category,omitempty"`
	Version     string   `json:"version,omitempty"`
	Tags        []string `json:"tags"`

	// Source
	Cluster string `json:"cluster"`
	Node    string `json:"node,omitempty"`
	Storage string `json:"storage"`
	Volume  string `json:"volume"`

	// File info
	Size   int64  `json:"size,omitempty"`
	Format string `json:"format,omitempty"`

	// VM template specifics
	VMID   int    `json:"vmid,omitempty"`
	OSType string `json:"os_type,omitempty"`
	Cores  int    `json:"cores,omitempty"`
	Memory int    `json:"memory,omitempty"`
}

// UpdateItemRequest is the API request to update a library item
type UpdateItemRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Category    *string  `json:"category,omitempty"`
	Version     *string  `json:"version,omitempty"`
	Tags        []string `json:"tags"`
}

// DeployRequest deploys a library item to a target cluster/storage
type DeployRequest struct {
	TargetCluster string `json:"target_cluster"`
	TargetNode    string `json:"target_node"`
	TargetStorage string `json:"target_storage,omitempty"` // defaults to same storage name
	NewName       string `json:"new_name,omitempty"`       // for VM templates: new VM name
	NewVMID       int    `json:"new_vmid,omitempty"`       // for VM templates: new VMID
	Full          bool   `json:"full"`                     // full clone vs linked
}

// ListFilter for querying library items
type ListFilter struct {
	Type     ItemType `json:"type,omitempty"`
	Category string   `json:"category,omitempty"`
	Cluster  string   `json:"cluster,omitempty"`
	Search   string   `json:"search,omitempty"`
	Tag      string   `json:"tag,omitempty"`
}
