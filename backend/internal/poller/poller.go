package poller

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/drs"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

// Poller polls multiple PVE clusters and updates state
type Poller struct {
	clusters map[string]*ClusterPoller // keyed by cluster name
	store    *state.Store
	interval time.Duration
	onChange func() // called when state changes

	// DRS scheduler
	drsScheduler *drs.Scheduler
	drsConfig    config.DRSConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ClusterPoller handles polling for a single cluster
type ClusterPoller struct {
	name         string
	config       config.ClusterConfig
	clients      map[string]*pve.Client // keyed by node name
	clusterStore *state.ClusterStore
	interval     time.Duration
	onChange     func()

	mu sync.RWMutex
}

// New creates a new multi-cluster poller
func New(store *state.Store, interval time.Duration, drsCfg config.DRSConfig) *Poller {
	p := &Poller{
		clusters:  make(map[string]*ClusterPoller),
		store:     store,
		interval:  interval,
		drsConfig: drsCfg,
	}
	if drsCfg.Enabled {
		p.drsScheduler = drs.NewScheduler(drsCfg, store)
	}
	return p
}

// AddCluster adds a cluster to poll
func (p *Poller) AddCluster(cfg config.ClusterConfig) *ClusterPoller {
	clusterStore := p.store.GetOrCreateCluster(cfg.Name)

	cp := &ClusterPoller{
		name:         cfg.Name,
		config:       cfg,
		clients:      make(map[string]*pve.Client),
		clusterStore: clusterStore,
		interval:     p.interval,
		onChange:     p.onChange,
	}

	p.clusters[cfg.Name] = cp
	return cp
}

// GetClusterClients returns all clients for a cluster
func (p *Poller) GetClusterClients(clusterName string) map[string]*pve.Client {
	if cp, ok := p.clusters[clusterName]; ok {
		cp.mu.RLock()
		defer cp.mu.RUnlock()
		// Return a copy
		clients := make(map[string]*pve.Client)
		for k, v := range cp.clients {
			clients[k] = v
		}
		return clients
	}
	return nil
}

// GetAllClients returns all clients across all clusters: map[cluster][node]*Client
func (p *Poller) GetAllClients() map[string]map[string]*pve.Client {
	result := make(map[string]map[string]*pve.Client)
	for name, cp := range p.clusters {
		result[name] = p.GetClusterClients(name)
		_ = cp // used
	}
	return result
}

// OnChange sets a callback for when state changes
func (p *Poller) OnChange(fn func()) {
	p.onChange = fn
	// Update all cluster pollers
	for _, cp := range p.clusters {
		cp.onChange = fn
	}
}

// Start begins polling all clusters
func (p *Poller) Start(ctx context.Context) {
	p.ctx, p.cancel = context.WithCancel(ctx)

	for _, cp := range p.clusters {
		p.wg.Add(1)
		go func(cluster *ClusterPoller) {
			defer p.wg.Done()
			cluster.run(p.ctx)
		}(cp)
	}

	// Start DRS loop if enabled
	if p.drsScheduler != nil {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.runDRS(p.ctx)
		}()
	}

	slog.Info("poller started", "clusters", len(p.clusters), "interval", p.interval, "drs", p.drsScheduler != nil)
}

// Stop stops all pollers
func (p *Poller) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	slog.Info("poller stopped")
}

// ForceRefresh triggers an immediate poll of all clusters
func (p *Poller) ForceRefresh() {
	for _, cp := range p.clusters {
		go cp.pollAll()
	}
}

// --- ClusterPoller ---

// run starts the cluster polling loop
func (cp *ClusterPoller) run(ctx context.Context) {
	// First, discover nodes in this cluster
	if err := cp.discoverNodes(ctx); err != nil {
		slog.Error("failed to discover cluster nodes", "cluster", cp.name, "error", err)
		return
	}

	// Initial poll of all nodes
	cp.pollAll()

	// Start per-node polling loops
	var wg sync.WaitGroup
	for nodeName, client := range cp.clients {
		wg.Add(1)
		go func(name string, c *pve.Client) {
			defer wg.Done()
			cp.pollNodeLoop(ctx, name, c)
		}(nodeName, client)
	}

	// Also poll HA status periodically
	wg.Add(1)
	go func() {
		defer wg.Done()
		cp.pollHALoop(ctx)
	}()

	wg.Wait()
}

// discoverNodes discovers all nodes in the cluster
func (cp *ClusterPoller) discoverNodes(ctx context.Context) error {
	// Create a discovery client
	discoveryClient := pve.NewClientFromClusterConfig(cp.config)

	nodes, err := discoveryClient.DiscoverClusterNodes(ctx)
	if err != nil {
		return err
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()

	for _, node := range nodes {
		// Create a client for each node
		client := pve.NewClientForNode(cp.config, node.Name, node.IP)
		cp.clients[node.Name] = client
		slog.Info("discovered node", "cluster", cp.name, "node", node.Name, "ip", node.IP, "online", node.Online)
	}

	if len(cp.clients) == 0 {
		// Fallback: use the discovery node itself
		discoveryClient.SetNodeName("unknown")
		// Try to get node name from the node list
		allNodes, err := discoveryClient.GetNodes(ctx)
		if err == nil && len(allNodes) > 0 {
			for _, n := range allNodes {
				client := pve.NewClientForNode(cp.config, n.Node, "")
				cp.clients[n.Node] = client
				slog.Info("discovered node via /nodes", "cluster", cp.name, "node", n.Node)
			}
		}
	}

	return nil
}

// pollAll polls all nodes once
func (cp *ClusterPoller) pollAll() {
	cp.mu.RLock()
	clients := make([]*pve.Client, 0, len(cp.clients))
	for _, c := range cp.clients {
		clients = append(clients, c)
	}
	cp.mu.RUnlock()

	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(c *pve.Client) {
			defer wg.Done()
			cp.fetchNode(context.Background(), c)
		}(client)
	}
	wg.Wait()
}

// pollNodeLoop runs the polling loop for a single node
func (cp *ClusterPoller) pollNodeLoop(ctx context.Context, nodeName string, client *pve.Client) {
	ticker := time.NewTicker(cp.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cp.fetchNode(ctx, client)
		}
	}
}

// pollHALoop polls HA status periodically
func (cp *ClusterPoller) pollHALoop(ctx context.Context) {
	// Poll HA less frequently - every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial poll
	cp.fetchHA(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cp.fetchHA(ctx)
		}
	}
}

// fetchNode fetches all data from a single node
func (cp *ClusterPoller) fetchNode(ctx context.Context, client *pve.Client) {
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	nodeName := client.NodeName()
	start := time.Now()

	// Fetch all data in parallel
	var (
		node    pve.Node
		vms     []pve.VM
		cts     []pve.Container
		storage []pve.Storage
		ceph    *pve.CephStatus

		nodeErr, vmErr, ctErr, storageErr, cephErr error
		wg                                          sync.WaitGroup
	)

	wg.Add(5)

	go func() {
		defer wg.Done()
		nodeStatus, err := client.GetNodeStatus(fetchCtx)
		if err != nil {
			nodeErr = err
		} else if nodeStatus != nil {
			node = *nodeStatus
		}
	}()

	go func() {
		defer wg.Done()
		vms, vmErr = client.GetVMs(fetchCtx)
	}()

	go func() {
		defer wg.Done()
		cts, ctErr = client.GetContainers(fetchCtx)
	}()

	go func() {
		defer wg.Done()
		storage, storageErr = client.GetStorage(fetchCtx)
	}()

	go func() {
		defer wg.Done()
		ceph, cephErr = client.GetCephStatus(fetchCtx)
		if cephErr != nil {
			ceph = nil
			cephErr = nil
		}
	}()

	wg.Wait()

	// Check for critical errors
	if nodeErr != nil {
		slog.Error("failed to fetch node status", "cluster", cp.name, "node", nodeName, "error", nodeErr)
		cp.clusterStore.SetNodeError(nodeName, nodeErr)
		return
	}

	// Log non-critical errors
	if vmErr != nil {
		slog.Warn("failed to fetch VMs", "cluster", cp.name, "node", nodeName, "error", vmErr)
	}
	if ctErr != nil {
		slog.Warn("failed to fetch containers", "cluster", cp.name, "node", nodeName, "error", ctErr)
	}
	if storageErr != nil {
		slog.Warn("failed to fetch storage", "cluster", cp.name, "node", nodeName, "error", storageErr)
	}

	// Update cluster state
	cp.clusterStore.UpdateNode(nodeName, node, vms, cts, storage, ceph)

	slog.Debug("polled node",
		"cluster", cp.name,
		"node", nodeName,
		"vms", len(vms),
		"containers", len(cts),
		"duration", time.Since(start),
	)

	// Notify listeners
	if cp.onChange != nil {
		cp.onChange()
	}
}

// fetchHA fetches HA status for the cluster
func (cp *ClusterPoller) fetchHA(ctx context.Context) {
	cp.mu.RLock()
	var client *pve.Client
	for _, c := range cp.clients {
		client = c
		break // Use first available client
	}
	cp.mu.RUnlock()

	if client == nil {
		return
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	haStatus, err := client.GetHAStatus(fetchCtx)
	if err != nil {
		slog.Debug("failed to fetch HA status", "cluster", cp.name, "error", err)
		return
	}

	cp.clusterStore.SetHAStatus(haStatus)

	if cp.onChange != nil {
		cp.onChange()
	}
}

// runDRS runs the DRS scheduler periodically
func (p *Poller) runDRS(ctx context.Context) {
	interval := time.Duration(p.drsConfig.CheckInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("DRS scheduler started", "interval", interval, "mode", p.drsConfig.Mode)

	// Wait a bit for initial data to be collected
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}

	// Initial run
	p.runDRSAnalysis()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runDRSAnalysis()
		}
	}
}

// runDRSAnalysis analyzes all clusters and updates recommendations
func (p *Poller) runDRSAnalysis() {
	for clusterName := range p.clusters {
		recs := p.drsScheduler.AnalyzeCluster(clusterName)
		p.store.SetDRSRecommendations(clusterName, recs)

		if len(recs) > 0 {
			slog.Info("DRS recommendations generated",
				"cluster", clusterName,
				"count", len(recs),
			)
		}
	}

	// Notify listeners of the update
	if p.onChange != nil {
		p.onChange()
	}
}
