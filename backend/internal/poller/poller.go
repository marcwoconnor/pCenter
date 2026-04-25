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

	// mu serializes the AddCluster / RemoveCluster / Reconcile triad against
	// each other. Read-mostly callers (GetClusterClients, runDRSAnalysis,
	// ForceRefresh, OnChange) intentionally do not take this lock — they're
	// invoked from contexts that don't race with reconciliation in practice,
	// and adding lock acquisitions in their hot paths would noticeably regress
	// the per-tick poll cost. The narrow contract: anything that mutates the
	// `clusters` map MUST hold mu; anything that snapshots its keys/values
	// for iteration MUST hold mu while building the snapshot.
	mu sync.Mutex
}

// ClusterPoller handles polling for a single cluster
type ClusterPoller struct {
	name         string
	config       config.ClusterConfig
	clients      map[string]*pve.Client // keyed by node name
	clusterStore *state.ClusterStore
	interval     time.Duration
	onChange     func()

	// cancel stops this cluster's polling goroutines. Set when AddCluster
	// is called after Start (or when Start fans out to pre-added clusters).
	// RemoveCluster invokes it to drop a single cluster without stopping the
	// whole poller.
	cancel context.CancelFunc

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

// SetDRSRulesDB sets the rules database on the DRS scheduler
func (p *Poller) SetDRSRulesDB(db *drs.RulesDB) {
	if p.drsScheduler != nil {
		p.drsScheduler.SetRulesDB(db)
	}
}

// GetDRSScheduler returns the DRS scheduler (for violation checking)
func (p *Poller) GetDRSScheduler() *drs.Scheduler {
	return p.drsScheduler
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

	p.mu.Lock()
	p.clusters[cfg.Name] = cp
	pollerCtx := p.ctx
	p.mu.Unlock()

	// If poller is already running, start this cluster's polling goroutine immediately
	if pollerCtx != nil {
		childCtx, cancel := context.WithCancel(pollerCtx)
		cp.cancel = cancel
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			cp.run(childCtx)
		}()
	}

	return cp
}

// RemoveCluster stops polling for the named cluster, removes it from the
// poller's registry, AND drops its accumulated state from the store. Used
// when standalone hosts get promoted into a real PVE cluster (the per-host
// "standalone:<id>" pseudo-clusters are no longer the right discovery path),
// or when forcing a re-discovery after a join.
//
// Clearing state.Store is essential: otherwise the renderer keeps seeing
// stale node/storage/VM entries from the removed cluster, which surface in
// the UI as duplicates.
//
// Safe to call on an unknown name (no-op). If the poller was never Start()ed
// (cancel == nil), the cluster is just removed from the map.
func (p *Poller) RemoveCluster(name string) {
	p.mu.Lock()
	cp, ok := p.clusters[name]
	if ok {
		delete(p.clusters, name)
	}
	p.mu.Unlock()

	if !ok {
		// Still drop any stale state for that name — there may be leftover
		// entries from a previous run that no longer have a poller.
		p.store.RemoveCluster(name)
		return
	}
	if cp.cancel != nil {
		cp.cancel()
	}
	p.store.RemoveCluster(name)
	slog.Info("removed cluster from poller", "cluster", name)
}

// Reconcile drops poller entries whose names are not present in `expected`.
// Used to garbage-collect orphan goroutines after out-of-band inventory
// changes (e.g. an operator SQL-DELETE'd a host row, or a test reset state) —
// without it, the standalone:<id> ClusterPoller for the gone host keeps
// running and 401'ing every interval until pcenter restarts. Issue #66.
//
// Pass the canonical set of expected poller keys (typically derived from
// inventory.GetClusterConfigs). Names in `expected` that are not currently
// registered are NOT added — Reconcile is one-directional cleanup. Adding
// stays the responsibility of explicit AddCluster calls (the API layer and
// the inventory reconciler's promote callback).
//
// Returns the names that were removed (possibly empty).
func (p *Poller) Reconcile(expected []string) []string {
	want := make(map[string]struct{}, len(expected))
	for _, n := range expected {
		want[n] = struct{}{}
	}

	p.mu.Lock()
	orphans := make([]string, 0)
	for name := range p.clusters {
		if _, ok := want[name]; !ok {
			orphans = append(orphans, name)
		}
	}
	p.mu.Unlock()

	for _, name := range orphans {
		slog.Info("reconcile: dropping orphan poller (not in inventory)", "cluster", name)
		p.RemoveCluster(name)
	}
	return orphans
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
	p.mu.Lock()
	p.ctx, p.cancel = context.WithCancel(ctx)
	startTargets := make([]*ClusterPoller, 0, len(p.clusters))
	for _, cp := range p.clusters {
		startTargets = append(startTargets, cp)
	}
	p.mu.Unlock()

	for _, cp := range startTargets {
		childCtx, cancel := context.WithCancel(p.ctx)
		cp.cancel = cancel
		p.wg.Add(1)
		go func(cluster *ClusterPoller, runCtx context.Context) {
			defer p.wg.Done()
			cluster.run(runCtx)
		}(cp, childCtx)
	}

	// Start DRS loop if enabled
	if p.drsScheduler != nil {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.runDRS(p.ctx)
		}()
	}

	// Start migration status polling loop
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.pollMigrationsLoop(p.ctx)
	}()

	slog.Info("poller started", "clusters", len(startTargets), "interval", p.interval, "drs", p.drsScheduler != nil)
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

	// Also poll SDN data periodically
	wg.Add(1)
	go func() {
		defer wg.Done()
		cp.pollSDNLoop(ctx)
	}()

	// Also poll QDevice status periodically
	wg.Add(1)
	go func() {
		defer wg.Done()
		cp.pollQDeviceLoop(ctx)
	}()

	// Also poll certificates periodically (certs don't change often; 5min is plenty)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cp.pollCertsLoop(ctx)
	}()

	// Also poll cluster-wide Ceph topology (OSDs, pools, MONs, ...).
	// fetchNode already polls /ceph/status per node; this loop populates
	// the topology view consumed by the day-2 management UI.
	wg.Add(1)
	go func() {
		defer wg.Done()
		cp.pollCephLoop(ctx)
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
		node        pve.Node
		nodeDetails *pve.NodeStatus
		vmStats     *pve.VmStats
		vms         []pve.VM
		cts         []pve.Container
		storage     []pve.Storage
		ceph        *pve.CephStatus
		netIfaces   []pve.NetworkInterface

		nodeErr, detailsErr, vmStatsErr, vmErr, ctErr, storageErr, cephErr, netErr error
		wg                                                                          sync.WaitGroup
	)

	wg.Add(8)

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
		nodeDetails, detailsErr = client.GetNodeDetails(fetchCtx)
	}()

	go func() {
		defer wg.Done()
		vmStats, vmStatsErr = client.GetVmStats(fetchCtx)
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

	go func() {
		defer wg.Done()
		netIfaces, netErr = client.GetNetworkInterfaces(fetchCtx)
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
	if netErr != nil {
		slog.Warn("failed to fetch network interfaces", "cluster", cp.name, "node", nodeName, "error", netErr)
	}
	if detailsErr != nil {
		slog.Warn("failed to fetch node details", "cluster", cp.name, "node", nodeName, "error", detailsErr)
	}
	if vmStatsErr != nil {
		slog.Warn("failed to fetch vmstats", "cluster", cp.name, "node", nodeName, "error", vmStatsErr)
	}

	// Update cluster state
	cp.clusterStore.UpdateNode(nodeName, node, vms, cts, storage, ceph)

	// Update network interfaces separately (node-specific)
	if netErr == nil {
		cp.clusterStore.UpdateNetworkInterfaces(nodeName, netIfaces)
	}

	// Update node details (version, kernel, etc.)
	if detailsErr == nil && nodeDetails != nil {
		cp.clusterStore.SetNodeDetails(nodeName, nodeDetails)
	}

	// Update vmstats (memory paging counters)
	if vmStatsErr == nil && vmStats != nil {
		cp.clusterStore.SetVmStats(nodeName, vmStats)
	}

	slog.Debug("polled node",
		"cluster", cp.name,
		"node", nodeName,
		"vms", len(vms),
		"containers", len(cts),
		"interfaces", len(netIfaces),
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

// pollQDeviceLoop polls qdevice status periodically
func (cp *ClusterPoller) pollQDeviceLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	cp.fetchQDevice(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cp.fetchQDevice(ctx)
		}
	}
}

// fetchQDevice fetches qdevice status via Proxmox API and finds hosting VM
func (cp *ClusterPoller) fetchQDevice(ctx context.Context) {
	cp.mu.RLock()
	clients := make(map[string]*pve.Client, len(cp.clients))
	for k, v := range cp.clients {
		clients[k] = v
	}
	cp.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Get qdevice status from any node (it's cluster-wide)
	var status *pve.QDeviceStatus
	for _, client := range clients {
		s, err := client.GetQDeviceStatus(fetchCtx)
		if err == nil && s != nil {
			status = s
			break
		}
	}

	if status == nil || !status.Configured {
		cp.clusterStore.SetQDeviceStatus(status)
		if cp.onChange != nil {
			cp.onChange()
		}
		return
	}

	// Find the qdevice VM for display info
	for nodeName, client := range clients {
		vmStatus, err := client.FindQDeviceVM(fetchCtx, "")
		if err == nil && vmStatus != nil {
			status.HostNode = nodeName
			status.HostVMID = vmStatus.HostVMID
			status.HostVMName = vmStatus.HostVMName
			break
		}
	}

	cp.clusterStore.SetQDeviceStatus(status)
	if cp.onChange != nil {
		cp.onChange()
	}
}

// pollCertsLoop polls per-node certificate info every 5 minutes.
// Low-frequency because certs only change on renewal events.
func (cp *ClusterPoller) pollCertsLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Initial poll (slight delay so nodes are discovered)
	time.Sleep(10 * time.Second)
	cp.fetchCerts(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cp.fetchCerts(ctx)
		}
	}
}

// fetchCerts fetches cert info from every online node in parallel.
func (cp *ClusterPoller) fetchCerts(ctx context.Context) {
	cp.mu.RLock()
	clients := make(map[string]*pve.Client, len(cp.clients))
	for k, v := range cp.clients {
		clients[k] = v
	}
	cp.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for nodeName, client := range clients {
		wg.Add(1)
		go func(n string, c *pve.Client) {
			defer wg.Done()
			certs, err := c.GetNodeCertificates(fetchCtx)
			if err != nil {
				slog.Debug("cert poll failed", "cluster", cp.name, "node", n, "error", err)
				return
			}
			cp.clusterStore.SetNodeCertificates(n, certs)
		}(nodeName, client)
	}
	wg.Wait()
}

// pollSDNLoop polls SDN data periodically
func (cp *ClusterPoller) pollSDNLoop(ctx context.Context) {
	// Poll SDN less frequently - every 60 seconds
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Initial poll
	cp.fetchSDN(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cp.fetchSDN(ctx)
		}
	}
}

// fetchSDN fetches SDN data for the cluster (zones, vnets, subnets, controllers)
func (cp *ClusterPoller) fetchSDN(ctx context.Context) {
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

	// Fetch all SDN data in parallel
	var (
		zones       []pve.SDNZone
		vnets       []pve.SDNVNet
		subnets     []pve.SDNSubnet
		controllers []pve.SDNController
		wg          sync.WaitGroup
	)

	wg.Add(4)

	go func() {
		defer wg.Done()
		var err error
		zones, err = client.GetSDNZones(fetchCtx)
		if err != nil {
			slog.Debug("SDN zones fetch failed (may not be configured)", "error", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		vnets, err = client.GetSDNVNets(fetchCtx)
		if err != nil {
			slog.Debug("SDN vnets fetch failed (may not be configured)", "error", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		subnets, err = client.GetSDNSubnets(fetchCtx)
		if err != nil {
			slog.Debug("SDN subnets fetch failed (may not be configured)", "error", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		controllers, err = client.GetSDNControllers(fetchCtx)
		if err != nil {
			slog.Debug("SDN controllers fetch failed (may not be configured)", "error", err)
		}
	}()

	wg.Wait()

	// Update cluster state
	cp.clusterStore.SetSDNData(zones, vnets, subnets, controllers)

	if len(zones) > 0 || len(vnets) > 0 {
		slog.Debug("polled SDN",
			"cluster", cp.name,
			"zones", len(zones),
			"vnets", len(vnets),
			"subnets", len(subnets),
		)
	}

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

// pollMigrationsLoop polls active migration task statuses
func (p *Poller) pollMigrationsLoop(ctx context.Context) {
	// Poll every 5 seconds - migrations can complete quickly
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkMigrationStatuses(ctx)
		}
	}
}

// checkMigrationStatuses checks all active migrations against Proxmox task API
func (p *Poller) checkMigrationStatuses(ctx context.Context) {
	migrations := p.store.GetMigrations()
	if len(migrations) == 0 {
		return
	}

	for _, m := range migrations {
		// Find a client for the source node's cluster
		clients := p.GetClusterClients(m.Cluster)
		if clients == nil {
			continue
		}

		// Get client for the source node (where task was started)
		client, ok := clients[m.FromNode]
		if !ok {
			// Try any client in the cluster
			for _, c := range clients {
				client = c
				break
			}
		}
		if client == nil {
			continue
		}

		// Query task status
		fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		task, err := client.GetTaskStatus(fetchCtx, m.UPID)
		cancel()

		if err != nil {
			slog.Debug("failed to get migration task status", "upid", m.UPID, "error", err)
			continue
		}

		if task == nil {
			continue
		}

		// Check if task is finished
		if task.Status == "stopped" {
			if task.ExitCode == "OK" {
				slog.Info("migration completed", "vmid", m.VMID, "from", m.FromNode, "to", m.ToNode)
				p.store.UpdateMigration(m.UPID, 100, "completed", "")
			} else {
				slog.Warn("migration failed", "vmid", m.VMID, "exitstatus", task.ExitCode)
				p.store.UpdateMigration(m.UPID, m.Progress, "failed", task.ExitCode)
			}
			// Remove after a short delay so UI can see the final status
			go func(upid string) {
				time.Sleep(10 * time.Second)
				p.store.RemoveMigration(upid)
				if p.onChange != nil {
					p.onChange()
				}
			}(m.UPID)
		}
	}
}

// pollCephLoop polls cluster-wide Ceph topology (OSDs, MONs, MGRs, MDSs,
// pools, rules, fs, flags) periodically. Topology data is identical from
// every MON, so we hit one node per tick rather than fanning out.
//
// Cadence is intentionally slower than fetchNode's per-node Ceph status
// poll — topology changes (pool create, OSD add) are operator-initiated
// and rare, while health status needs to track cluster events promptly.
func (cp *ClusterPoller) pollCephLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	cp.fetchCephTopology(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cp.fetchCephTopology(ctx)
		}
	}
}

// fetchCephTopology fetches a full Ceph topology snapshot from any node
// that has Ceph installed. If GetCephStatus returns an error from a node
// (typically 404 "Ceph not installed") we move on to the next; if all
// nodes fail, we set the topology to nil so the UI can render "no Ceph".
func (cp *ClusterPoller) fetchCephTopology(ctx context.Context) {
	cp.mu.RLock()
	clients := make([]*pve.Client, 0, len(cp.clients))
	for _, c := range cp.clients {
		clients = append(clients, c)
	}
	cp.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Pick a node that responds to /ceph/status — that's our signal Ceph is
	// installed and the node is in quorum. Try each client in turn.
	var probe *pve.Client
	var status *pve.CephStatus
	for _, client := range clients {
		s, err := client.GetCephStatus(fetchCtx)
		if err != nil || s == nil {
			continue
		}
		probe = client
		status = s
		break
	}
	if probe == nil {
		// No node has Ceph (or all are unreachable). Clear any stale topology.
		cp.clusterStore.SetCephTopology(nil)
		return
	}

	// Fetch each topology piece in parallel against the chosen node.
	var (
		mons    []pve.CephMON
		mgrs    []pve.CephMGR
		mdss    []pve.CephMDS
		osds    []pve.CephOSD
		pools   []pve.CephPool
		rules   []pve.CephRule
		fs      []pve.CephFSEntry
		flags   pve.CephFlags
		monErr, mgrErr, mdsErr, osdErr, poolErr, ruleErr, fsErr, flagErr error
		wg      sync.WaitGroup
	)
	wg.Add(8)
	go func() { defer wg.Done(); mons, monErr = probe.ListCephMONs(fetchCtx) }()
	go func() { defer wg.Done(); mgrs, mgrErr = probe.ListCephMGRs(fetchCtx) }()
	go func() { defer wg.Done(); mdss, mdsErr = probe.ListCephMDSs(fetchCtx) }()
	go func() { defer wg.Done(); osds, osdErr = probe.ListCephOSDs(fetchCtx) }()
	go func() { defer wg.Done(); pools, poolErr = probe.ListCephPools(fetchCtx) }()
	go func() { defer wg.Done(); rules, ruleErr = probe.GetCephRules(fetchCtx) }()
	go func() { defer wg.Done(); fs, fsErr = probe.ListCephFS(fetchCtx) }()
	go func() { defer wg.Done(); flags, flagErr = probe.GetCephFlags(fetchCtx) }()
	wg.Wait()

	// Log non-fatal errors but proceed with whatever we got — partial data
	// is better than no data, and a single endpoint hiccup shouldn't blank
	// the whole topology view.
	for _, e := range []struct {
		what string
		err  error
	}{
		{"mons", monErr}, {"mgrs", mgrErr}, {"mdss", mdsErr}, {"osds", osdErr},
		{"pools", poolErr}, {"rules", ruleErr}, {"fs", fsErr}, {"flags", flagErr},
	} {
		if e.err != nil {
			slog.Debug("ceph topology partial fetch error", "cluster", cp.name, "node", probe.NodeName(), "what", e.what, "error", e.err)
		}
	}

	cp.clusterStore.SetCephTopology(&pve.CephCluster{
		Status:      status,
		MONs:        mons,
		MGRs:        mgrs,
		MDSs:        mdss,
		OSDs:        osds,
		Pools:       pools,
		Rules:       rules,
		FS:          fs,
		Flags:       flags,
		LastUpdated: time.Now(),
	})

	if cp.onChange != nil {
		cp.onChange()
	}
}
