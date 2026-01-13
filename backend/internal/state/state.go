package state

import (
	"sync"
	"time"

	"github.com/moconnor/pcenter/internal/pve"
)

// Store holds the aggregated state from all clusters
type Store struct {
	mu       sync.RWMutex
	clusters map[string]*ClusterStore // keyed by cluster name

	// Active migrations across all clusters
	migrations map[string]*pve.MigrationProgress // keyed by UPID

	// DRS recommendations per cluster
	drsRecommendations map[string][]pve.DRSRecommendation // keyed by cluster name

	// Maintenance mode state per node (cluster:node -> state)
	maintenanceStates map[string]*pve.MaintenanceState
}

// ClusterStore holds state for a single Proxmox cluster
type ClusterStore struct {
	mu sync.RWMutex

	name        string
	nodes       map[string]pve.Node        // keyed by node name
	nodeDetails map[string]*pve.NodeStatus // keyed by node name (version, kernel, etc.)
	vms         map[int]pve.VM             // keyed by VMID (unique within cluster)
	containers  map[int]pve.Container      // keyed by VMID
	storage     map[string][]pve.Storage   // keyed by node name
	ceph        map[string]*pve.CephStatus // keyed by node name
	haStatus    *pve.HAStatus

	// Network data
	networkInterfaces map[string][]pve.NetworkInterface // keyed by node name
	sdnZones          []pve.SDNZone                     // cluster-wide
	sdnVNets          []pve.SDNVNet                     // cluster-wide
	sdnSubnets        []pve.SDNSubnet                   // cluster-wide
	sdnControllers    []pve.SDNController               // cluster-wide

	lastUpdate map[string]time.Time // per-node update times
	errors     map[string]error     // per-node errors
}

// New creates a new multi-cluster state store
func New() *Store {
	return &Store{
		clusters:           make(map[string]*ClusterStore),
		migrations:         make(map[string]*pve.MigrationProgress),
		drsRecommendations: make(map[string][]pve.DRSRecommendation),
		maintenanceStates:  make(map[string]*pve.MaintenanceState),
	}
}

// GetMaintenanceState returns the maintenance state for a node
func (s *Store) GetMaintenanceState(cluster, node string) *pve.MaintenanceState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := cluster + ":" + node
	return s.maintenanceStates[key]
}

// SetMaintenanceState sets the maintenance state for a node
func (s *Store) SetMaintenanceState(cluster, node string, state *pve.MaintenanceState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := cluster + ":" + node
	if state == nil {
		delete(s.maintenanceStates, key)
	} else {
		s.maintenanceStates[key] = state
	}
}

// GetAllMaintenanceStates returns all maintenance states
func (s *Store) GetAllMaintenanceStates() map[string]*pve.MaintenanceState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*pve.MaintenanceState, len(s.maintenanceStates))
	for k, v := range s.maintenanceStates {
		result[k] = v
	}
	return result
}

// GetOrCreateCluster returns the cluster store, creating it if needed
func (s *Store) GetOrCreateCluster(name string) *ClusterStore {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cs, ok := s.clusters[name]; ok {
		return cs
	}

	cs := &ClusterStore{
		name:              name,
		nodes:             make(map[string]pve.Node),
		nodeDetails:       make(map[string]*pve.NodeStatus),
		vms:               make(map[int]pve.VM),
		containers:        make(map[int]pve.Container),
		storage:           make(map[string][]pve.Storage),
		ceph:              make(map[string]*pve.CephStatus),
		networkInterfaces: make(map[string][]pve.NetworkInterface),
		lastUpdate:        make(map[string]time.Time),
		errors:            make(map[string]error),
	}
	s.clusters[name] = cs
	return cs
}

// GetCluster returns a cluster store by name
func (s *Store) GetCluster(name string) (*ClusterStore, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cs, ok := s.clusters[name]
	return cs, ok
}

// GetClusterNames returns all cluster names
func (s *Store) GetClusterNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.clusters))
	for name := range s.clusters {
		names = append(names, name)
	}
	return names
}

// UpdateNode updates the state for a single node in a cluster
func (cs *ClusterStore) UpdateNode(nodeName string, node pve.Node, vms []pve.VM, cts []pve.Container, storage []pve.Storage, ceph *pve.CephStatus) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Set cluster name on node
	node.Cluster = cs.name
	cs.nodes[nodeName] = node
	cs.storage[nodeName] = storage
	cs.ceph[nodeName] = ceph
	cs.lastUpdate[nodeName] = time.Now()
	delete(cs.errors, nodeName)

	// Update VMs - remove old ones from this node first
	for vmid, vm := range cs.vms {
		if vm.Node == nodeName {
			delete(cs.vms, vmid)
		}
	}
	for _, vm := range vms {
		vm.Cluster = cs.name
		cs.vms[vm.VMID] = vm
	}

	// Update containers
	for vmid, ct := range cs.containers {
		if ct.Node == nodeName {
			delete(cs.containers, vmid)
		}
	}
	for _, ct := range cts {
		ct.Cluster = cs.name
		cs.containers[ct.VMID] = ct
	}

	// Set cluster on storage
	for i := range storage {
		storage[i].Cluster = cs.name
	}
}

// SetNodeError records an error for a node
func (cs *ClusterStore) SetNodeError(nodeName string, err error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.errors[nodeName] = err
	cs.lastUpdate[nodeName] = time.Now()
}

// SetHAStatus updates the HA status for the cluster
func (cs *ClusterStore) SetHAStatus(status *pve.HAStatus) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.haStatus = status
}

// SetNodeDetails updates detailed node info (version, kernel, etc.)
func (cs *ClusterStore) SetNodeDetails(nodeName string, details *pve.NodeStatus) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.nodeDetails[nodeName] = details
}

// GetNodeDetails returns detailed info for all nodes
func (cs *ClusterStore) GetNodeDetails() map[string]*pve.NodeStatus {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	result := make(map[string]*pve.NodeStatus)
	for name, details := range cs.nodeDetails {
		result[name] = details
	}
	return result
}

// GetNodes returns all nodes in the cluster
func (cs *ClusterStore) GetNodes() []pve.Node {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	nodes := make([]pve.Node, 0, len(cs.nodes))
	for _, n := range cs.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

// GetVMs returns all VMs in the cluster
func (cs *ClusterStore) GetVMs() []pve.VM {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	vms := make([]pve.VM, 0, len(cs.vms))
	for _, vm := range cs.vms {
		vms = append(vms, vm)
	}
	return vms
}

// GetVM returns a single VM by VMID
func (cs *ClusterStore) GetVM(vmid int) (pve.VM, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	vm, ok := cs.vms[vmid]
	return vm, ok
}

// GetContainers returns all containers in the cluster
func (cs *ClusterStore) GetContainers() []pve.Container {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	cts := make([]pve.Container, 0, len(cs.containers))
	for _, ct := range cs.containers {
		cts = append(cts, ct)
	}
	return cts
}

// GetContainer returns a single container by VMID
func (cs *ClusterStore) GetContainer(vmid int) (pve.Container, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	ct, ok := cs.containers[vmid]
	return ct, ok
}

// GetStorage returns all storage, optionally filtered by node
func (cs *ClusterStore) GetStorage(nodeName string) []pve.Storage {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if nodeName != "" {
		return cs.storage[nodeName]
	}

	var all []pve.Storage
	for _, storages := range cs.storage {
		all = append(all, storages...)
	}
	return all
}

// GetCeph returns Ceph status (from first node that has it)
func (cs *ClusterStore) GetCeph() *pve.CephStatus {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	for _, status := range cs.ceph {
		if status != nil {
			return status
		}
	}
	return nil
}

// GetHAStatus returns the cluster's HA status
func (cs *ClusterStore) GetHAStatus() *pve.HAStatus {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.haStatus
}

// --- Network data methods ---

// UpdateNetworkInterfaces updates network interfaces for a node
func (cs *ClusterStore) UpdateNetworkInterfaces(nodeName string, ifaces []pve.NetworkInterface) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Tag with cluster name
	for i := range ifaces {
		ifaces[i].Cluster = cs.name
		ifaces[i].Node = nodeName
	}
	cs.networkInterfaces[nodeName] = ifaces
}

// SetSDNData updates all SDN data for the cluster
func (cs *ClusterStore) SetSDNData(zones []pve.SDNZone, vnets []pve.SDNVNet, subnets []pve.SDNSubnet, controllers []pve.SDNController) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Tag with cluster name
	for i := range zones {
		zones[i].Cluster = cs.name
	}
	for i := range vnets {
		vnets[i].Cluster = cs.name
	}
	for i := range subnets {
		subnets[i].Cluster = cs.name
	}
	for i := range controllers {
		controllers[i].Cluster = cs.name
	}

	cs.sdnZones = zones
	cs.sdnVNets = vnets
	cs.sdnSubnets = subnets
	cs.sdnControllers = controllers
}

// GetNetworkInterfaces returns network interfaces, optionally filtered by node
func (cs *ClusterStore) GetNetworkInterfaces(nodeName string) []pve.NetworkInterface {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if nodeName != "" {
		return cs.networkInterfaces[nodeName]
	}

	var all []pve.NetworkInterface
	for _, ifaces := range cs.networkInterfaces {
		all = append(all, ifaces...)
	}
	return all
}

// GetSDNZones returns all SDN zones
func (cs *ClusterStore) GetSDNZones() []pve.SDNZone {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.sdnZones
}

// GetSDNVNets returns all SDN virtual networks
func (cs *ClusterStore) GetSDNVNets() []pve.SDNVNet {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.sdnVNets
}

// GetSDNSubnets returns all SDN subnets
func (cs *ClusterStore) GetSDNSubnets() []pve.SDNSubnet {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.sdnSubnets
}

// GetSDNControllers returns all SDN controllers
func (cs *ClusterStore) GetSDNControllers() []pve.SDNController {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.sdnControllers
}

// GetNodeStatus returns the last update time and error for a node
func (cs *ClusterStore) GetNodeStatus(nodeName string) (time.Time, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.lastUpdate[nodeName], cs.errors[nodeName]
}

// GetAllNodeStatuses returns update times and errors for all nodes
func (cs *ClusterStore) GetAllNodeStatuses() map[string]NodeStatus {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	statuses := make(map[string]NodeStatus)
	for name := range cs.lastUpdate {
		statuses[name] = NodeStatus{
			LastUpdate: cs.lastUpdate[name],
			Error:      cs.errors[name],
		}
	}
	return statuses
}

// NodeStatus holds status info for a node
type NodeStatus struct {
	LastUpdate time.Time
	Error      error
}

// Summary returns cluster summary stats
type Summary struct {
	TotalNodes      int     `json:"TotalNodes"`
	OnlineNodes     int     `json:"OnlineNodes"`
	TotalVMs        int     `json:"TotalVMs"`
	RunningVMs      int     `json:"RunningVMs"`
	TotalContainers int     `json:"TotalContainers"`
	RunningCTs      int     `json:"RunningCTs"`
	TotalCPU        int     `json:"TotalCPU"`
	UsedCPU         float64 `json:"UsedCPU"`
	TotalMemGB      float64 `json:"TotalMemGB"`
	UsedMemGB       float64 `json:"UsedMemGB"`
}

// GetSummary returns cluster summary stats
func (cs *ClusterStore) GetSummary() Summary {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	sum := Summary{
		TotalNodes: len(cs.nodes),
	}

	for _, n := range cs.nodes {
		if n.Status == "online" {
			sum.OnlineNodes++
		}
		sum.TotalCPU += n.MaxCPU
		sum.UsedCPU += float64(n.MaxCPU) * n.CPU
		sum.TotalMemGB += float64(n.MaxMem) / (1024 * 1024 * 1024)
		sum.UsedMemGB += float64(n.Mem) / (1024 * 1024 * 1024)
	}

	for _, vm := range cs.vms {
		sum.TotalVMs++
		if vm.Status == "running" {
			sum.RunningVMs++
		}
	}

	for _, ct := range cs.containers {
		sum.TotalContainers++
		if ct.Status == "running" {
			sum.RunningCTs++
		}
	}

	return sum
}

// ClusterSummary includes cluster name with summary
type ClusterSummary struct {
	Name    string  `json:"name"`
	Summary Summary `json:"summary"`
}

// GlobalSummary aggregates all clusters
type GlobalSummary struct {
	Clusters []ClusterSummary `json:"clusters"`
	Total    Summary          `json:"total"`
}

// GetGlobalSummary returns summary across all clusters
func (s *Store) GetGlobalSummary() GlobalSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	gs := GlobalSummary{
		Clusters: make([]ClusterSummary, 0, len(s.clusters)),
	}

	for name, cs := range s.clusters {
		sum := cs.GetSummary()
		gs.Clusters = append(gs.Clusters, ClusterSummary{
			Name:    name,
			Summary: sum,
		})

		// Aggregate totals
		gs.Total.TotalNodes += sum.TotalNodes
		gs.Total.OnlineNodes += sum.OnlineNodes
		gs.Total.TotalVMs += sum.TotalVMs
		gs.Total.RunningVMs += sum.RunningVMs
		gs.Total.TotalContainers += sum.TotalContainers
		gs.Total.RunningCTs += sum.RunningCTs
		gs.Total.TotalCPU += sum.TotalCPU
		gs.Total.UsedCPU += sum.UsedCPU
		gs.Total.TotalMemGB += sum.TotalMemGB
		gs.Total.UsedMemGB += sum.UsedMemGB
	}

	return gs
}

// Migration management

// AddMigration tracks a new migration
func (s *Store) AddMigration(m *pve.MigrationProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.migrations[m.UPID] = m
}

// UpdateMigration updates migration progress
func (s *Store) UpdateMigration(upid string, progress int, status string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.migrations[upid]; ok {
		m.Progress = progress
		m.Status = status
		m.Error = errMsg
	}
}

// GetMigrations returns all active migrations
func (s *Store) GetMigrations() []pve.MigrationProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()

	migrations := make([]pve.MigrationProgress, 0, len(s.migrations))
	for _, m := range s.migrations {
		migrations = append(migrations, *m)
	}
	return migrations
}

// RemoveMigration removes a completed migration
func (s *Store) RemoveMigration(upid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.migrations, upid)
}

// DRS recommendation management

// SetDRSRecommendations sets recommendations for a cluster
func (s *Store) SetDRSRecommendations(cluster string, recs []pve.DRSRecommendation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drsRecommendations[cluster] = recs
}

// GetDRSRecommendations returns recommendations for a cluster
func (s *Store) GetDRSRecommendations(cluster string) []pve.DRSRecommendation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.drsRecommendations[cluster]
}

// GetAllDRSRecommendations returns all recommendations across clusters
func (s *Store) GetAllDRSRecommendations() []pve.DRSRecommendation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []pve.DRSRecommendation
	for _, recs := range s.drsRecommendations {
		all = append(all, recs...)
	}
	return all
}

// RemoveDRSRecommendation removes a specific recommendation
func (s *Store) RemoveDRSRecommendation(cluster, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	recs := s.drsRecommendations[cluster]
	for i, rec := range recs {
		if rec.ID == id {
			s.drsRecommendations[cluster] = append(recs[:i], recs[i+1:]...)
			return
		}
	}
}

// Legacy compatibility - these work on "default" cluster or first cluster

// GetNodes returns all nodes across all clusters (legacy)
func (s *Store) GetNodes() []pve.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []pve.Node
	for _, cs := range s.clusters {
		all = append(all, cs.GetNodes()...)
	}
	return all
}

// GetVMs returns all VMs across all clusters (legacy)
func (s *Store) GetVMs() []pve.VM {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []pve.VM
	for _, cs := range s.clusters {
		all = append(all, cs.GetVMs()...)
	}
	return all
}

// GetVM finds a VM by VMID across all clusters (legacy - may have collisions!)
func (s *Store) GetVM(vmid int) (pve.VM, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, cs := range s.clusters {
		if vm, ok := cs.GetVM(vmid); ok {
			return vm, true
		}
	}
	return pve.VM{}, false
}

// GetContainers returns all containers across all clusters (legacy)
func (s *Store) GetContainers() []pve.Container {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []pve.Container
	for _, cs := range s.clusters {
		all = append(all, cs.GetContainers()...)
	}
	return all
}

// GetContainer finds a container by VMID across all clusters (legacy)
func (s *Store) GetContainer(vmid int) (pve.Container, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, cs := range s.clusters {
		if ct, ok := cs.GetContainer(vmid); ok {
			return ct, true
		}
	}
	return pve.Container{}, false
}

// GetStorage returns all storage across all clusters (legacy)
func (s *Store) GetStorage(nodeName string) []pve.Storage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []pve.Storage
	for _, cs := range s.clusters {
		all = append(all, cs.GetStorage(nodeName)...)
	}
	return all
}

// GetCeph returns Ceph status from first cluster that has it (legacy)
func (s *Store) GetCeph() *pve.CephStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, cs := range s.clusters {
		if ceph := cs.GetCeph(); ceph != nil {
			return ceph
		}
	}
	return nil
}

// GetSummary returns global summary (legacy)
func (s *Store) GetSummary() Summary {
	return s.GetGlobalSummary().Total
}

// GetAllNodeStatuses returns statuses across all clusters (legacy)
func (s *Store) GetAllNodeStatuses() map[string]NodeStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := make(map[string]NodeStatus)
	for clusterName, cs := range s.clusters {
		for nodeName, status := range cs.GetAllNodeStatuses() {
			// Prefix with cluster name to avoid collisions
			key := clusterName + "/" + nodeName
			all[key] = status
		}
	}
	return all
}
