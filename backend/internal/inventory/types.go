package inventory

import "time"

// ClusterStatus represents the state of a cluster
type ClusterStatus string

const (
	ClusterStatusEmpty   ClusterStatus = "empty"   // Just created, no hosts
	ClusterStatusPending ClusterStatus = "pending" // Has hosts but not connected
	ClusterStatusActive  ClusterStatus = "active"  // Connected and working
	ClusterStatusError   ClusterStatus = "error"   // Connection failed
)

// HostStatus represents the state of a staged host
type HostStatus string

const (
	HostStatusStaged     HostStatus = "staged"     // Connection details entered
	HostStatusConnecting HostStatus = "connecting" // Attempting connection
	HostStatusOnline     HostStatus = "online"     // Connected successfully
	HostStatusOffline    HostStatus = "offline"    // Was connected, now unreachable
	HostStatusError      HostStatus = "error"      // Connection failed
)

// Datacenter represents a logical datacenter boundary
// (networking scope, storage visibility, permissions, fault domain)
type Datacenter struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Computed fields for tree response
	Clusters []Cluster `json:"clusters,omitempty"`
}

// Cluster represents a Proxmox cluster (container for hosts)
type Cluster struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`                        // Display name (user-editable)
	AgentName    string        `json:"agent_name,omitempty"`        // Name agents report (for matching runtime data)
	DatacenterID *string       `json:"datacenter_id,omitempty"`     // nil = orphan cluster
	Status       ClusterStatus `json:"status"`                      // empty, pending, active, error
	Enabled      bool          `json:"enabled"`                     // polling enabled
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`

	// Computed fields for responses
	DatacenterName string          `json:"datacenter_name,omitempty"`
	Hosts          []InventoryHost `json:"hosts,omitempty"`
}

// InventoryHost represents a Proxmox host staged/connected to a cluster
type InventoryHost struct {
	ID        string     `json:"id"`
	ClusterID string     `json:"cluster_id"`
	Address   string     `json:"address"`   // host:port
	TokenID   string     `json:"token_id"`  // API token ID
	Insecure  bool       `json:"insecure"`  // skip TLS verification
	Status    HostStatus `json:"status"`    // staged, online, error, etc.
	Error     string     `json:"error,omitempty"` // last error message
	NodeName  string     `json:"node_name,omitempty"` // discovered PVE node name
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// CreateDatacenterRequest is the request body for creating a datacenter
type CreateDatacenterRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// UpdateDatacenterRequest is the request body for updating a datacenter
type UpdateDatacenterRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateClusterRequest is the request body for creating a cluster
type CreateClusterRequest struct {
	Name         string  `json:"name"`
	DatacenterID *string `json:"datacenter_id,omitempty"`
}

// UpdateClusterRequest is the request body for updating a cluster
type UpdateClusterRequest struct {
	Name         string  `json:"name"`
	DatacenterID *string `json:"datacenter_id,omitempty"`
	Enabled      bool    `json:"enabled"`
}

// AddHostRequest is the request body for adding a host to a cluster
type AddHostRequest struct {
	Address     string `json:"address"`      // host:port
	TokenID     string `json:"token_id"`     // API token ID
	TokenSecret string `json:"token_secret"` // for initial connection test
	Insecure    bool   `json:"insecure"`     // skip TLS verification
}

// UpdateHostRequest is the request body for updating a host
type UpdateHostRequest struct {
	Address     string `json:"address"`
	TokenID     string `json:"token_id"`
	TokenSecret string `json:"token_secret,omitempty"` // only if changing
	Insecure    bool   `json:"insecure"`
}

// TestConnectionRequest is the request body for testing host connectivity
type TestConnectionRequest struct {
	Address     string `json:"address"`
	TokenID     string `json:"token_id"`
	TokenSecret string `json:"token_secret"`
	Insecure    bool   `json:"insecure"`
}

// TestConnectionResult is the response from testing connectivity
type TestConnectionResult struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	NodeName  string   `json:"node_name,omitempty"`
	NodeCount int      `json:"node_count,omitempty"`
	Nodes     []string `json:"nodes,omitempty"`
}
