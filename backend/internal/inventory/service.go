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

// GetDatacenterTree returns datacenters with their clusters populated
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
	return s.db.ListClusters(ctx)
}

// GetCluster retrieves a cluster by ID
func (s *Service) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	return s.db.GetCluster(ctx, id)
}

// GetClusterByName retrieves a cluster by name
func (s *Service) GetClusterByName(ctx context.Context, name string) (*Cluster, error) {
	return s.db.GetClusterByName(ctx, name)
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

	// Validate discovery node
	req.DiscoveryNode = strings.TrimSpace(req.DiscoveryNode)
	if req.DiscoveryNode == "" {
		return nil, fmt.Errorf("discovery_node is required")
	}

	// Validate token ID
	req.TokenID = strings.TrimSpace(req.TokenID)
	if req.TokenID == "" {
		return nil, fmt.Errorf("token_id is required")
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

	// Validate discovery node
	req.DiscoveryNode = strings.TrimSpace(req.DiscoveryNode)
	if req.DiscoveryNode == "" {
		return fmt.Errorf("discovery_node is required")
	}

	// Validate token ID
	req.TokenID = strings.TrimSpace(req.TokenID)
	if req.TokenID == "" {
		return fmt.Errorf("token_id is required")
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

// === Integration Methods ===

// GetClusterConfig returns a cluster configuration for the poller
// Looks up the secret from the provided secrets map
func (s *Service) GetClusterConfig(ctx context.Context, name string, secrets map[string]string) (*config.ClusterConfig, error) {
	cluster, err := s.db.GetClusterByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	if cluster == nil {
		return nil, fmt.Errorf("cluster not found")
	}

	secret := secrets[name]
	if secret == "" {
		return nil, fmt.Errorf("no secret configured for cluster '%s'", name)
	}

	return &config.ClusterConfig{
		Name:          cluster.Name,
		DiscoveryNode: cluster.DiscoveryNode,
		TokenID:       cluster.TokenID,
		TokenSecret:   secret,
		Insecure:      cluster.Insecure,
	}, nil
}

// GetAllClusterConfigs returns all enabled cluster configurations for the poller
func (s *Service) GetAllClusterConfigs(ctx context.Context, secrets map[string]string) ([]config.ClusterConfig, error) {
	clusters, err := s.db.ListClusters(ctx)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}

	var configs []config.ClusterConfig
	for _, c := range clusters {
		if !c.Enabled {
			continue
		}

		secret := secrets[c.Name]
		if secret == "" {
			// Skip clusters without secrets configured
			continue
		}

		configs = append(configs, config.ClusterConfig{
			Name:          c.Name,
			DiscoveryNode: c.DiscoveryNode,
			TokenID:       c.TokenID,
			TokenSecret:   secret,
			Insecure:      c.Insecure,
		})
	}

	return configs, nil
}

// ClusterCount returns total cluster count (for migration check)
func (s *Service) ClusterCount(ctx context.Context) (int, error) {
	return s.db.ClusterCount(ctx)
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
		Name:          cluster.Name,
		DatacenterID:  datacenterID,
		DiscoveryNode: cluster.DiscoveryNode,
		TokenID:       cluster.TokenID,
		Insecure:      cluster.Insecure,
		Enabled:       cluster.Enabled,
	}

	return s.db.UpdateCluster(ctx, cluster.ID, req)
}
