package inventory

import "time"

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

// Cluster represents a Proxmox cluster configuration (without secrets)
type Cluster struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DatacenterID  *string   `json:"datacenter_id,omitempty"` // nil = orphan cluster
	DiscoveryNode string    `json:"discovery_node"`          // host:port of any PVE node
	TokenID       string    `json:"token_id"`                // API token ID
	Insecure      bool      `json:"insecure"`                // skip TLS verification
	Enabled       bool      `json:"enabled"`                 // polling enabled
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Computed field for joined queries
	DatacenterName string `json:"datacenter_name,omitempty"`
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
	Name          string  `json:"name"`
	DatacenterID  *string `json:"datacenter_id,omitempty"`
	DiscoveryNode string  `json:"discovery_node"`
	TokenID       string  `json:"token_id"`
	Insecure      bool    `json:"insecure"`
}

// UpdateClusterRequest is the request body for updating a cluster
type UpdateClusterRequest struct {
	Name          string  `json:"name"`
	DatacenterID  *string `json:"datacenter_id,omitempty"` // explicit nil removes datacenter
	DiscoveryNode string  `json:"discovery_node"`
	TokenID       string  `json:"token_id"`
	Insecure      bool    `json:"insecure"`
	Enabled       bool    `json:"enabled"`
}

// TestConnectionRequest is the request body for testing cluster connectivity
type TestConnectionRequest struct {
	DiscoveryNode string `json:"discovery_node"`
	TokenID       string `json:"token_id"`
	TokenSecret   string `json:"token_secret"` // required for test, not stored
	Insecure      bool   `json:"insecure"`
}

// TestConnectionResult is the response from testing cluster connectivity
type TestConnectionResult struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	NodeCount int      `json:"node_count,omitempty"`
	Nodes     []string `json:"nodes,omitempty"`
}
