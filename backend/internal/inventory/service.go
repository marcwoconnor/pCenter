package inventory

import (
	"context"
	"fmt"
	"strings"

	"github.com/moconnor/pcenter/internal/config"
)

// Service provides inventory operations with business logic
type Service struct {
	db *DB
}

// NewService creates a new inventory service
func NewService(db *DB) *Service {
	return &Service{db: db}
}

// === Datacenter Operations ===

// ListDatacenters returns all datacenters
func (s *Service) ListDatacenters(ctx context.Context) ([]Datacenter, error) {
	return s.db.ListDatacenters(ctx)
}

// GetDatacenter retrieves a datacenter by ID
func (s *Service) GetDatacenter(ctx context.Context, id string) (*Datacenter, error) {
	return s.db.GetDatacenter(ctx, id)
}

// GetDatacenterByName retrieves a datacenter by name
func (s *Service) GetDatacenterByName(ctx context.Context, name string) (*Datacenter, error) {
	return s.db.GetDatacenterByName(ctx, name)
}

// CreateDatacenter creates a new datacenter with validation
func (s *Service) CreateDatacenter(ctx context.Context, req CreateDatacenterRequest) (*Datacenter, error) {
	// Validate name
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Check for duplicate name
	existing, err := s.db.GetDatacenterByName(ctx, req.Name)
	if err != nil {
		return nil, fmt.Errorf("check name: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("datacenter with name '%s' already exists", req.Name)
	}

	return s.db.CreateDatacenter(ctx, req)
}

// UpdateDatacenter updates a datacenter
func (s *Service) UpdateDatacenter(ctx context.Context, id string, req UpdateDatacenterRequest) error {
	// Validate name
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Check datacenter exists
	dc, err := s.db.GetDatacenter(ctx, id)
	if err != nil {
		return fmt.Errorf("get datacenter: %w", err)
	}
	if dc == nil {
		return fmt.Errorf("datacenter not found")
	}

	// Check for duplicate name (if changed)
	if req.Name != dc.Name {
		existing, err := s.db.GetDatacenterByName(ctx, req.Name)
		if err != nil {
			return fmt.Errorf("check name: %w", err)
		}
		if existing != nil {
			return fmt.Errorf("datacenter with name '%s' already exists", req.Name)
		}
	}

	return s.db.UpdateDatacenter(ctx, id, req)
}

// DeleteDatacenter deletes a datacenter (clusters become orphans)
func (s *Service) DeleteDatacenter(ctx context.Context, id string) error {
	dc, err := s.db.GetDatacenter(ctx, id)
	if err != nil {
		return fmt.Errorf("get datacenter: %w", err)
	}
	if dc == nil {
		return fmt.Errorf("datacenter not found")
	}

	return s.db.DeleteDatacenter(ctx, id)
}

// GetDatacenterTree returns datacenters with their clusters and hosts populated
func (s *Service) GetDatacenterTree(ctx context.Context) ([]Datacenter, []Cluster, error) {
	// Get all datacenters
	datacenters, err := s.db.ListDatacenters(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list datacenters: %w", err)
	}

	// Get all clusters
	clusters, err := s.db.ListClusters(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list clusters: %w", err)
	}

	// Get hosts for each cluster
	for i := range clusters {
		hosts, err := s.db.ListHostsByCluster(ctx, clusters[i].ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list hosts: %w", err)
		}
		clusters[i].Hosts = hosts
	}

	// Group clusters by datacenter
	clustersByDC := make(map[string][]Cluster)
	var orphanClusters []Cluster
	for _, c := range clusters {
		if c.DatacenterID != nil {
			clustersByDC[*c.DatacenterID] = append(clustersByDC[*c.DatacenterID], c)
		} else {
			orphanClusters = append(orphanClusters, c)
		}
	}

	// Populate clusters on datacenters
	for i := range datacenters {
		datacenters[i].Clusters = clustersByDC[datacenters[i].ID]
	}

	return datacenters, orphanClusters, nil
}

// === Cluster Operations ===

// ListClusters returns all clusters
func (s *Service) ListClusters(ctx context.Context) ([]Cluster, error) {
	clusters, err := s.db.ListClusters(ctx)
	if err != nil {
		return nil, err
	}

	// Get hosts for each cluster
	for i := range clusters {
		hosts, err := s.db.ListHostsByCluster(ctx, clusters[i].ID)
		if err != nil {
			return nil, fmt.Errorf("list hosts: %w", err)
		}
		clusters[i].Hosts = hosts
	}

	return clusters, nil
}

// GetCluster retrieves a cluster by ID
func (s *Service) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	cluster, err := s.db.GetCluster(ctx, id)
	if err != nil || cluster == nil {
		return cluster, err
	}

	// Get hosts
	hosts, err := s.db.ListHostsByCluster(ctx, cluster.ID)
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}
	cluster.Hosts = hosts

	return cluster, nil
}

// GetClusterByName retrieves a cluster by name
func (s *Service) GetClusterByName(ctx context.Context, name string) (*Cluster, error) {
	cluster, err := s.db.GetClusterByName(ctx, name)
	if err != nil || cluster == nil {
		return cluster, err
	}

	// Get hosts
	hosts, err := s.db.ListHostsByCluster(ctx, cluster.ID)
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}
	cluster.Hosts = hosts

	return cluster, nil
}

// CreateCluster creates a new cluster with validation
func (s *Service) CreateCluster(ctx context.Context, req CreateClusterRequest) (*Cluster, error) {
	// Validate name
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Check for duplicate name
	existing, err := s.db.GetClusterByName(ctx, req.Name)
	if err != nil {
		return nil, fmt.Errorf("check name: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("cluster with name '%s' already exists", req.Name)
	}

	// Validate datacenter exists if specified
	if req.DatacenterID != nil {
		dc, err := s.db.GetDatacenter(ctx, *req.DatacenterID)
		if err != nil {
			return nil, fmt.Errorf("check datacenter: %w", err)
		}
		if dc == nil {
			return nil, fmt.Errorf("datacenter not found")
		}
	}

	return s.db.CreateCluster(ctx, req)
}

// UpdateCluster updates a cluster
func (s *Service) UpdateCluster(ctx context.Context, id string, req UpdateClusterRequest) error {
	// Validate name
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Check cluster exists
	cluster, err := s.db.GetCluster(ctx, id)
	if err != nil {
		return fmt.Errorf("get cluster: %w", err)
	}
	if cluster == nil {
		return fmt.Errorf("cluster not found")
	}

	// Check for duplicate name (if changed)
	if req.Name != cluster.Name {
		existing, err := s.db.GetClusterByName(ctx, req.Name)
		if err != nil {
			return fmt.Errorf("check name: %w", err)
		}
		if existing != nil {
			return fmt.Errorf("cluster with name '%s' already exists", req.Name)
		}
	}

	// Validate datacenter exists if specified
	if req.DatacenterID != nil {
		dc, err := s.db.GetDatacenter(ctx, *req.DatacenterID)
		if err != nil {
			return fmt.Errorf("check datacenter: %w", err)
		}
		if dc == nil {
			return fmt.Errorf("datacenter not found")
		}
	}

	return s.db.UpdateCluster(ctx, id, req)
}

// UpdateClusterByName updates a cluster by name
func (s *Service) UpdateClusterByName(ctx context.Context, name string, req UpdateClusterRequest) error {
	cluster, err := s.db.GetClusterByName(ctx, name)
	if err != nil {
		return fmt.Errorf("get cluster: %w", err)
	}
	if cluster == nil {
		return fmt.Errorf("cluster not found")
	}

	return s.UpdateCluster(ctx, cluster.ID, req)
}

// DeleteCluster deletes a cluster
func (s *Service) DeleteCluster(ctx context.Context, id string) error {
	cluster, err := s.db.GetCluster(ctx, id)
	if err != nil {
		return fmt.Errorf("get cluster: %w", err)
	}
	if cluster == nil {
		return fmt.Errorf("cluster not found")
	}

	return s.db.DeleteCluster(ctx, id)
}

// DeleteClusterByName deletes a cluster by name
func (s *Service) DeleteClusterByName(ctx context.Context, name string) error {
	cluster, err := s.db.GetClusterByName(ctx, name)
	if err != nil {
		return fmt.Errorf("get cluster: %w", err)
	}
	if cluster == nil {
		return fmt.Errorf("cluster not found")
	}

	return s.db.DeleteCluster(ctx, cluster.ID)
}

// SetClusterEnabled enables or disables a cluster
func (s *Service) SetClusterEnabled(ctx context.Context, id string, enabled bool) error {
	cluster, err := s.db.GetCluster(ctx, id)
	if err != nil {
		return fmt.Errorf("get cluster: %w", err)
	}
	if cluster == nil {
		return fmt.Errorf("cluster not found")
	}

	return s.db.SetClusterEnabled(ctx, id, enabled)
}

// SetClusterStatus updates a cluster's status
func (s *Service) SetClusterStatus(ctx context.Context, id string, status ClusterStatus) error {
	return s.db.SetClusterStatus(ctx, id, status)
}

// MoveClusterToDatacenter moves a cluster to a datacenter (or makes it orphan if dcID is nil)
func (s *Service) MoveClusterToDatacenter(ctx context.Context, clusterName string, datacenterID *string) error {
	cluster, err := s.db.GetClusterByName(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("get cluster: %w", err)
	}
	if cluster == nil {
		return fmt.Errorf("cluster not found")
	}

	// Validate datacenter exists if specified
	if datacenterID != nil {
		dc, err := s.db.GetDatacenter(ctx, *datacenterID)
		if err != nil {
			return fmt.Errorf("check datacenter: %w", err)
		}
		if dc == nil {
			return fmt.Errorf("datacenter not found")
		}
	}

	req := UpdateClusterRequest{
		Name:         cluster.Name,
		DatacenterID: datacenterID,
		Enabled:      cluster.Enabled,
	}

	return s.db.UpdateCluster(ctx, cluster.ID, req)
}

// === Host Operations ===

// AddHost adds a host to a cluster
func (s *Service) AddHost(ctx context.Context, clusterID string, req AddHostRequest) (*InventoryHost, error) {
	// Validate cluster exists
	cluster, err := s.db.GetCluster(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	if cluster == nil {
		return nil, fmt.Errorf("cluster not found")
	}

	// Validate address
	req.Address = strings.TrimSpace(req.Address)
	if req.Address == "" {
		return nil, fmt.Errorf("address is required")
	}

	// Validate token ID
	req.TokenID = strings.TrimSpace(req.TokenID)
	if req.TokenID == "" {
		return nil, fmt.Errorf("token_id is required")
	}

	host, err := s.db.AddHost(ctx, clusterID, req)
	if err != nil {
		return nil, err
	}

	// Update cluster status if this is the first host
	if cluster.Status == ClusterStatusEmpty {
		s.db.SetClusterStatus(ctx, clusterID, ClusterStatusPending)
	}

	return host, nil
}

// AddHostByClusterName adds a host to a cluster by cluster name
func (s *Service) AddHostByClusterName(ctx context.Context, clusterName string, req AddHostRequest) (*InventoryHost, error) {
	cluster, err := s.db.GetClusterByName(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	if cluster == nil {
		return nil, fmt.Errorf("cluster not found")
	}

	return s.AddHost(ctx, cluster.ID, req)
}

// GetHost retrieves a host by ID
func (s *Service) GetHost(ctx context.Context, id string) (*InventoryHost, error) {
	return s.db.GetHost(ctx, id)
}

// ListHostsByCluster retrieves hosts for a cluster
func (s *Service) ListHostsByCluster(ctx context.Context, clusterID string) ([]InventoryHost, error) {
	return s.db.ListHostsByCluster(ctx, clusterID)
}

// UpdateHost updates a host's connection details
func (s *Service) UpdateHost(ctx context.Context, id string, req UpdateHostRequest) error {
	// Validate address
	req.Address = strings.TrimSpace(req.Address)
	if req.Address == "" {
		return fmt.Errorf("address is required")
	}

	// Validate token ID
	req.TokenID = strings.TrimSpace(req.TokenID)
	if req.TokenID == "" {
		return fmt.Errorf("token_id is required")
	}

	return s.db.UpdateHost(ctx, id, req)
}

// DeleteHost deletes a host
func (s *Service) DeleteHost(ctx context.Context, id string) error {
	host, err := s.db.GetHost(ctx, id)
	if err != nil {
		return fmt.Errorf("get host: %w", err)
	}
	if host == nil {
		return fmt.Errorf("host not found")
	}

	err = s.db.DeleteHost(ctx, id)
	if err != nil {
		return err
	}

	// Check if cluster should revert to empty status
	count, err := s.db.HostCountByCluster(ctx, host.ClusterID)
	if err == nil && count == 0 {
		s.db.SetClusterStatus(ctx, host.ClusterID, ClusterStatusEmpty)
	}

	return nil
}

// SetHostStatus updates a host's status
func (s *Service) SetHostStatus(ctx context.Context, id string, status HostStatus, errMsg, nodeName string) error {
	return s.db.SetHostStatus(ctx, id, status, errMsg, nodeName)
}

// === Integration Methods ===

// GetClusterConfigs returns cluster configurations for the poller
// Only returns clusters that are active with online hosts
func (s *Service) GetClusterConfigs(ctx context.Context, secrets map[string]string) ([]config.ClusterConfig, error) {
	clusters, err := s.db.ListClusters(ctx)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}

	var configs []config.ClusterConfig
	for _, c := range clusters {
		if !c.Enabled || c.Status != ClusterStatusActive {
			continue
		}

		// Get first online host for this cluster
		hosts, err := s.db.ListHostsByCluster(ctx, c.ID)
		if err != nil {
			continue
		}

		var onlineHost *InventoryHost
		for i := range hosts {
			if hosts[i].Status == HostStatusOnline {
				onlineHost = &hosts[i]
				break
			}
		}

		if onlineHost == nil {
			continue
		}

		// Use AgentName for secrets lookup and config name (what agents report as)
		agentName := c.AgentName
		if agentName == "" {
			agentName = c.Name // Fallback for legacy
		}

		secret := secrets[agentName]
		if secret == "" {
			continue
		}

		configs = append(configs, config.ClusterConfig{
			Name:          agentName,
			DiscoveryNode: onlineHost.Address,
			TokenID:       onlineHost.TokenID,
			TokenSecret:   secret,
			Insecure:      onlineHost.Insecure,
		})
	}

	return configs, nil
}

// ClusterCount returns total cluster count (for migration check)
func (s *Service) ClusterCount(ctx context.Context) (int, error) {
	return s.db.ClusterCount(ctx)
}
