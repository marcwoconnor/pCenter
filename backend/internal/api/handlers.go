package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/moconnor/pcenter/internal/poller"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

// Handler holds dependencies for API handlers
type Handler struct {
	store  *state.Store
	poller *poller.Poller
}

// NewHandler creates a new API handler
func NewHandler(store *state.Store, p *poller.Poller) *Handler {
	return &Handler{
		store:  store,
		poller: p,
	}
}

// getClient returns the PVE client for a cluster/node combination
func (h *Handler) getClient(cluster, node string) (*pve.Client, bool) {
	clients := h.poller.GetClusterClients(cluster)
	if clients == nil {
		return nil, false
	}
	client, ok := clients[node]
	return client, ok
}

// JSON helper
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// --- Global (legacy) Handlers ---

// GetSummary returns global summary (all clusters)
func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetSummary())
}

// GetGlobalSummary returns detailed summary per cluster
func (h *Handler) GetGlobalSummary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetGlobalSummary())
}

// GetClusters returns list of cluster names and summaries
func (h *Handler) GetClusters(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetGlobalSummary())
}

// GetNodes returns all nodes (across all clusters)
func (h *Handler) GetNodes(w http.ResponseWriter, r *http.Request) {
	nodes := h.store.GetNodes()
	statuses := h.store.GetAllNodeStatuses()

	type NodeWithStatus struct {
		pve.Node
		LastUpdate int64  `json:"last_update"`
		Error      string `json:"error,omitempty"`
	}

	result := make([]NodeWithStatus, 0, len(nodes))
	for _, n := range nodes {
		nws := NodeWithStatus{Node: n}
		// Try cluster/node key first (new format)
		key := n.Cluster + "/" + n.Node
		if status, ok := statuses[key]; ok {
			nws.LastUpdate = status.LastUpdate.Unix()
			if status.Error != nil {
				nws.Error = status.Error.Error()
			}
		} else if status, ok := statuses[n.Node]; ok {
			// Fallback to node-only key (legacy)
			nws.LastUpdate = status.LastUpdate.Unix()
			if status.Error != nil {
				nws.Error = status.Error.Error()
			}
		}
		result = append(result, nws)
	}

	writeJSON(w, result)
}

// GetVMs returns all VMs (across all clusters)
func (h *Handler) GetVMs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetVMs())
}

// GetVM returns a single VM (searches all clusters)
func (h *Handler) GetVM(w http.ResponseWriter, r *http.Request) {
	vmidStr := r.PathValue("vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	vm, ok := h.store.GetVM(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "VM not found")
		return
	}

	writeJSON(w, vm)
}

// GetContainers returns all containers (across all clusters)
func (h *Handler) GetContainers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetContainers())
}

// GetContainer returns a single container (searches all clusters)
func (h *Handler) GetContainer(w http.ResponseWriter, r *http.Request) {
	vmidStr := r.PathValue("vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	ct, ok := h.store.GetContainer(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}

	writeJSON(w, ct)
}

// GetAllGuests returns all VMs and containers combined (across all clusters)
func (h *Handler) GetAllGuests(w http.ResponseWriter, r *http.Request) {
	type Guest struct {
		Cluster string  `json:"cluster"`
		VMID    int     `json:"vmid"`
		Name    string  `json:"name"`
		Node    string  `json:"node"`
		Type    string  `json:"type"` // "qemu" or "lxc"
		Status  string  `json:"status"`
		CPU     float64 `json:"cpu"`
		CPUs    int     `json:"cpus"`
		Mem     int64   `json:"mem"`
		MaxMem  int64   `json:"maxmem"`
		Disk    int64   `json:"disk"`
		MaxDisk int64   `json:"maxdisk"`
		Uptime  int64   `json:"uptime"`
		Tags    string  `json:"tags,omitempty"`
		HAState string  `json:"ha_state,omitempty"`
	}

	var guests []Guest

	for _, vm := range h.store.GetVMs() {
		guests = append(guests, Guest{
			Cluster: vm.Cluster,
			VMID:    vm.VMID,
			Name:    vm.Name,
			Node:    vm.Node,
			Type:    "qemu",
			Status:  vm.Status,
			CPU:     vm.CPU,
			CPUs:    vm.CPUs,
			Mem:     vm.Mem,
			MaxMem:  vm.MaxMem,
			Disk:    vm.Disk,
			MaxDisk: vm.MaxDisk,
			Uptime:  vm.Uptime,
			Tags:    vm.Tags,
			HAState: vm.HAState,
		})
	}

	for _, ct := range h.store.GetContainers() {
		guests = append(guests, Guest{
			Cluster: ct.Cluster,
			VMID:    ct.VMID,
			Name:    ct.Name,
			Node:    ct.Node,
			Type:    "lxc",
			Status:  ct.Status,
			CPU:     ct.CPU,
			CPUs:    ct.CPUs,
			Mem:     ct.Mem,
			MaxMem:  ct.MaxMem,
			Disk:    ct.Disk,
			MaxDisk: ct.MaxDisk,
			Uptime:  ct.Uptime,
			Tags:    ct.Tags,
			HAState: ct.HAState,
		})
	}

	writeJSON(w, guests)
}

// GetStorage returns all storage (across all clusters)
func (h *Handler) GetStorage(w http.ResponseWriter, r *http.Request) {
	node := r.URL.Query().Get("node")
	writeJSON(w, h.store.GetStorage(node))
}

// GetCeph returns Ceph status (from first available cluster)
func (h *Handler) GetCeph(w http.ResponseWriter, r *http.Request) {
	ceph := h.store.GetCeph()
	if ceph == nil {
		writeError(w, http.StatusNotFound, "Ceph not available")
		return
	}
	writeJSON(w, ceph)
}

// --- Cluster-specific handlers ---

// GetClusterSummary returns summary for a specific cluster
func (h *Handler) GetClusterSummary(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}
	writeJSON(w, cs.GetSummary())
}

// GetClusterNodes returns nodes for a specific cluster
func (h *Handler) GetClusterNodes(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	nodes := cs.GetNodes()
	statuses := cs.GetAllNodeStatuses()

	type NodeWithStatus struct {
		pve.Node
		LastUpdate int64  `json:"last_update"`
		Error      string `json:"error,omitempty"`
	}

	result := make([]NodeWithStatus, 0, len(nodes))
	for _, n := range nodes {
		nws := NodeWithStatus{Node: n}
		if status, ok := statuses[n.Node]; ok {
			nws.LastUpdate = status.LastUpdate.Unix()
			if status.Error != nil {
				nws.Error = status.Error.Error()
			}
		}
		result = append(result, nws)
	}

	writeJSON(w, result)
}

// GetClusterGuests returns all guests for a specific cluster
func (h *Handler) GetClusterGuests(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	type Guest struct {
		Cluster string  `json:"cluster"`
		VMID    int     `json:"vmid"`
		Name    string  `json:"name"`
		Node    string  `json:"node"`
		Type    string  `json:"type"`
		Status  string  `json:"status"`
		CPU     float64 `json:"cpu"`
		CPUs    int     `json:"cpus"`
		Mem     int64   `json:"mem"`
		MaxMem  int64   `json:"maxmem"`
		Disk    int64   `json:"disk"`
		MaxDisk int64   `json:"maxdisk"`
		Uptime  int64   `json:"uptime"`
		Tags    string  `json:"tags,omitempty"`
		HAState string  `json:"ha_state,omitempty"`
	}

	var guests []Guest

	for _, vm := range cs.GetVMs() {
		guests = append(guests, Guest{
			Cluster: vm.Cluster,
			VMID:    vm.VMID,
			Name:    vm.Name,
			Node:    vm.Node,
			Type:    "qemu",
			Status:  vm.Status,
			CPU:     vm.CPU,
			CPUs:    vm.CPUs,
			Mem:     vm.Mem,
			MaxMem:  vm.MaxMem,
			Disk:    vm.Disk,
			MaxDisk: vm.MaxDisk,
			Uptime:  vm.Uptime,
			Tags:    vm.Tags,
			HAState: vm.HAState,
		})
	}

	for _, ct := range cs.GetContainers() {
		guests = append(guests, Guest{
			Cluster: ct.Cluster,
			VMID:    ct.VMID,
			Name:    ct.Name,
			Node:    ct.Node,
			Type:    "lxc",
			Status:  ct.Status,
			CPU:     ct.CPU,
			CPUs:    ct.CPUs,
			Mem:     ct.Mem,
			MaxMem:  ct.MaxMem,
			Disk:    ct.Disk,
			MaxDisk: ct.MaxDisk,
			Uptime:  ct.Uptime,
			Tags:    ct.Tags,
			HAState: ct.HAState,
		})
	}

	writeJSON(w, guests)
}

// GetClusterHA returns HA status for a specific cluster
func (h *Handler) GetClusterHA(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	haStatus := cs.GetHAStatus()
	if haStatus == nil {
		writeError(w, http.StatusNotFound, "HA not available")
		return
	}
	writeJSON(w, haStatus)
}

// --- Actions ---

// VMAction handles start/stop/shutdown for a VM (legacy - searches all clusters)
func (h *Handler) VMAction(w http.ResponseWriter, r *http.Request) {
	vmidStr := r.PathValue("vmid")
	action := r.PathValue("action")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	vm, ok := h.store.GetVM(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "VM not found")
		return
	}

	client, ok := h.getClient(vm.Cluster, vm.Node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "node client not found")
		return
	}

	var upid string
	switch action {
	case "start":
		upid, err = client.StartVM(r.Context(), vmid)
	case "stop":
		upid, err = client.StopVM(r.Context(), vmid)
	case "shutdown":
		upid, err = client.ShutdownVM(r.Context(), vmid)
	default:
		writeError(w, http.StatusBadRequest, "invalid action")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]string{"upid": upid})
}

// ContainerAction handles start/stop/shutdown for a container (legacy)
func (h *Handler) ContainerAction(w http.ResponseWriter, r *http.Request) {
	vmidStr := r.PathValue("vmid")
	action := r.PathValue("action")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	ct, ok := h.store.GetContainer(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}

	client, ok := h.getClient(ct.Cluster, ct.Node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "node client not found")
		return
	}

	var upid string
	switch action {
	case "start":
		upid, err = client.StartContainer(r.Context(), vmid)
	case "stop":
		upid, err = client.StopContainer(r.Context(), vmid)
	case "shutdown":
		upid, err = client.ShutdownContainer(r.Context(), vmid)
	default:
		writeError(w, http.StatusBadRequest, "invalid action")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]string{"upid": upid})
}

// ClusterVMAction handles actions for a VM in a specific cluster
func (h *Handler) ClusterVMAction(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	vmidStr := r.PathValue("vmid")
	action := r.PathValue("action")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	vm, ok := cs.GetVM(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "VM not found in cluster")
		return
	}

	client, ok := h.getClient(clusterName, vm.Node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "node client not found")
		return
	}

	var upid string
	switch action {
	case "start":
		upid, err = client.StartVM(r.Context(), vmid)
	case "stop":
		upid, err = client.StopVM(r.Context(), vmid)
	case "shutdown":
		upid, err = client.ShutdownVM(r.Context(), vmid)
	default:
		writeError(w, http.StatusBadRequest, "invalid action")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]string{"upid": upid})
}

// ClusterContainerAction handles actions for a container in a specific cluster
func (h *Handler) ClusterContainerAction(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	vmidStr := r.PathValue("vmid")
	action := r.PathValue("action")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	ct, ok := cs.GetContainer(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "container not found in cluster")
		return
	}

	client, ok := h.getClient(clusterName, ct.Node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "node client not found")
		return
	}

	var upid string
	switch action {
	case "start":
		upid, err = client.StartContainer(r.Context(), vmid)
	case "stop":
		upid, err = client.StopContainer(r.Context(), vmid)
	case "shutdown":
		upid, err = client.ShutdownContainer(r.Context(), vmid)
	default:
		writeError(w, http.StatusBadRequest, "invalid action")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]string{"upid": upid})
}

// GetConsoleURL returns the Proxmox console URL for a VM or container (legacy)
func (h *Handler) GetConsoleURL(w http.ResponseWriter, r *http.Request) {
	guestType := r.PathValue("type") // "vm" or "ct"
	vmidStr := r.PathValue("vmid")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	var node string
	var cluster string
	var consoleType string

	if guestType == "vm" {
		vm, ok := h.store.GetVM(vmid)
		if !ok {
			writeError(w, http.StatusNotFound, "VM not found")
			return
		}
		node = vm.Node
		cluster = vm.Cluster
		consoleType = "kvm"
	} else if guestType == "ct" {
		ct, ok := h.store.GetContainer(vmid)
		if !ok {
			writeError(w, http.StatusNotFound, "container not found")
			return
		}
		node = ct.Node
		cluster = ct.Cluster
		consoleType = "lxc"
	} else {
		writeError(w, http.StatusBadRequest, "invalid type, must be 'vm' or 'ct'")
		return
	}

	client, ok := h.getClient(cluster, node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "node client not found")
		return
	}

	// Build the Proxmox console URL
	pveType := "qemu"
	if consoleType == "lxc" {
		pveType = "lxc"
	}
	consoleURL := "https://" + client.Host() + "/#v1:0:=" + pveType + "%2F" + vmidStr + ":4"

	writeJSON(w, map[string]string{"url": consoleURL})
}

// GetMigrations returns all active migrations
func (h *Handler) GetMigrations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetMigrations())
}

// GetDRSRecommendations returns all DRS recommendations
func (h *Handler) GetDRSRecommendations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.GetAllDRSRecommendations())
}

// GetClusterDRS returns DRS recommendations for a specific cluster
func (h *Handler) GetClusterDRS(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	_, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}
	writeJSON(w, h.store.GetDRSRecommendations(clusterName))
}

// --- Migration Handlers ---

// MigrateRequest is the request body for migration
type MigrateRequest struct {
	TargetNode string `json:"target_node"`
	Online     bool   `json:"online"` // live migration
}

// MigrateVM initiates a VM migration (searches all clusters by VMID)
func (h *Handler) MigrateVM(w http.ResponseWriter, r *http.Request) {
	vmidStr := r.PathValue("vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	var req MigrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TargetNode == "" {
		writeError(w, http.StatusBadRequest, "target_node required")
		return
	}

	vm, ok := h.store.GetVM(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "VM not found")
		return
	}

	if vm.Node == req.TargetNode {
		writeError(w, http.StatusBadRequest, "VM already on target node")
		return
	}

	client, ok := h.getClient(vm.Cluster, vm.Node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "source node client not found")
		return
	}

	upid, err := client.MigrateVM(r.Context(), vmid, req.TargetNode, req.Online)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "migration failed: "+err.Error())
		return
	}

	// Track the migration
	h.store.AddMigration(&pve.MigrationProgress{
		UPID:      upid,
		Cluster:   vm.Cluster,
		VMID:      vmid,
		GuestName: vm.Name,
		GuestType: "vm",
		FromNode:  vm.Node,
		ToNode:    req.TargetNode,
		Online:    req.Online,
		StartedAt: time.Now(),
		Progress:  0,
		Status:    "running",
	})

	writeJSON(w, map[string]string{"upid": upid})
}

// MigrateContainer initiates a container migration
func (h *Handler) MigrateContainer(w http.ResponseWriter, r *http.Request) {
	vmidStr := r.PathValue("vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	var req MigrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TargetNode == "" {
		writeError(w, http.StatusBadRequest, "target_node required")
		return
	}

	ct, ok := h.store.GetContainer(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "container not found")
		return
	}

	if ct.Node == req.TargetNode {
		writeError(w, http.StatusBadRequest, "container already on target node")
		return
	}

	client, ok := h.getClient(ct.Cluster, ct.Node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "source node client not found")
		return
	}

	upid, err := client.MigrateContainer(r.Context(), vmid, req.TargetNode, req.Online)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "migration failed: "+err.Error())
		return
	}

	// Track the migration
	h.store.AddMigration(&pve.MigrationProgress{
		UPID:      upid,
		Cluster:   ct.Cluster,
		VMID:      vmid,
		GuestName: ct.Name,
		GuestType: "ct",
		FromNode:  ct.Node,
		ToNode:    req.TargetNode,
		Online:    req.Online,
		StartedAt: time.Now(),
		Progress:  0,
		Status:    "running",
	})

	writeJSON(w, map[string]string{"upid": upid})
}

// ClusterMigrateVM initiates a VM migration in a specific cluster
func (h *Handler) ClusterMigrateVM(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	vmidStr := r.PathValue("vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	var req MigrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TargetNode == "" {
		writeError(w, http.StatusBadRequest, "target_node required")
		return
	}

	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	vm, ok := cs.GetVM(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "VM not found in cluster")
		return
	}

	if vm.Node == req.TargetNode {
		writeError(w, http.StatusBadRequest, "VM already on target node")
		return
	}

	client, ok := h.getClient(clusterName, vm.Node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "source node client not found")
		return
	}

	upid, err := client.MigrateVM(r.Context(), vmid, req.TargetNode, req.Online)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "migration failed: "+err.Error())
		return
	}

	h.store.AddMigration(&pve.MigrationProgress{
		UPID:      upid,
		Cluster:   clusterName,
		VMID:      vmid,
		GuestName: vm.Name,
		GuestType: "vm",
		FromNode:  vm.Node,
		ToNode:    req.TargetNode,
		Online:    req.Online,
		StartedAt: time.Now(),
		Progress:  0,
		Status:    "running",
	})

	writeJSON(w, map[string]string{"upid": upid})
}

// ClusterMigrateContainer initiates a container migration in a specific cluster
func (h *Handler) ClusterMigrateContainer(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	vmidStr := r.PathValue("vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	var req MigrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TargetNode == "" {
		writeError(w, http.StatusBadRequest, "target_node required")
		return
	}

	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	ct, ok := cs.GetContainer(vmid)
	if !ok {
		writeError(w, http.StatusNotFound, "container not found in cluster")
		return
	}

	if ct.Node == req.TargetNode {
		writeError(w, http.StatusBadRequest, "container already on target node")
		return
	}

	client, ok := h.getClient(clusterName, ct.Node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "source node client not found")
		return
	}

	upid, err := client.MigrateContainer(r.Context(), vmid, req.TargetNode, req.Online)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "migration failed: "+err.Error())
		return
	}

	h.store.AddMigration(&pve.MigrationProgress{
		UPID:      upid,
		Cluster:   clusterName,
		VMID:      vmid,
		GuestName: ct.Name,
		GuestType: "ct",
		FromNode:  ct.Node,
		ToNode:    req.TargetNode,
		Online:    req.Online,
		StartedAt: time.Now(),
		Progress:  0,
		Status:    "running",
	})

	writeJSON(w, map[string]string{"upid": upid})
}

// GetClusterNodes returns nodes for target selection in migrations
func (h *Handler) GetClusterNodesForMigration(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	nodes := cs.GetNodes()
	type NodeOption struct {
		Name   string `json:"name"`
		Online bool   `json:"online"`
	}

	options := make([]NodeOption, 0, len(nodes))
	for _, n := range nodes {
		options = append(options, NodeOption{
			Name:   n.Node,
			Online: n.Status == "online",
		})
	}

	writeJSON(w, options)
}

// --- DRS Handlers ---

// ApplyDRSRecommendation executes a DRS recommendation (initiates migration)
func (h *Handler) ApplyDRSRecommendation(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	recID := r.PathValue("id")

	// Find the recommendation
	recs := h.store.GetDRSRecommendations(clusterName)
	var found *pve.DRSRecommendation
	for i := range recs {
		if recs[i].ID == recID {
			found = &recs[i]
			break
		}
	}

	if found == nil {
		writeError(w, http.StatusNotFound, "recommendation not found")
		return
	}

	// Get the appropriate client
	client, ok := h.getClient(clusterName, found.FromNode)
	if !ok {
		writeError(w, http.StatusInternalServerError, "source node client not found")
		return
	}

	// Execute the migration
	var upid string
	var err error
	online := true // DRS always does live migration for running guests

	if found.GuestType == "vm" {
		upid, err = client.MigrateVM(r.Context(), found.VMID, found.ToNode, online)
	} else {
		upid, err = client.MigrateContainer(r.Context(), found.VMID, found.ToNode, online)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "migration failed: "+err.Error())
		return
	}

	// Track the migration
	h.store.AddMigration(&pve.MigrationProgress{
		UPID:      upid,
		Cluster:   clusterName,
		VMID:      found.VMID,
		GuestName: found.GuestName,
		GuestType: found.GuestType,
		FromNode:  found.FromNode,
		ToNode:    found.ToNode,
		Online:    online,
		StartedAt: time.Now(),
		Progress:  0,
		Status:    "running",
	})

	// Remove the recommendation since we've acted on it
	h.store.RemoveDRSRecommendation(clusterName, recID)

	writeJSON(w, map[string]string{"upid": upid, "message": "migration started"})
}

// DismissDRSRecommendation removes a DRS recommendation without acting on it
func (h *Handler) DismissDRSRecommendation(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	recID := r.PathValue("id")

	h.store.RemoveDRSRecommendation(clusterName, recID)

	writeJSON(w, map[string]string{"message": "recommendation dismissed"})
}

// --- HA Management Handlers ---

// HAEnableRequest is the request body for enabling HA
type HAEnableRequest struct {
	State       string `json:"state,omitempty"`        // started, stopped
	Group       string `json:"group,omitempty"`        // HA group name
	MaxRestart  int    `json:"max_restart,omitempty"`  // 0-10
	MaxRelocate int    `json:"max_relocate,omitempty"` // 0-10
	Comment     string `json:"comment,omitempty"`
}

// EnableHA enables HA for a VM or container
func (h *Handler) EnableHA(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	guestType := r.PathValue("type") // "vm" or "ct"
	vmidStr := r.PathValue("vmid")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	// Validate guest type
	if guestType != "vm" && guestType != "ct" {
		writeError(w, http.StatusBadRequest, "type must be 'vm' or 'ct'")
		return
	}

	var req HAEnableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && r.ContentLength > 0 {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Find the guest to get its node
	var node string
	if guestType == "vm" {
		cs, ok := h.store.GetCluster(clusterName)
		if !ok {
			writeError(w, http.StatusNotFound, "cluster not found")
			return
		}
		vm, ok := cs.GetVM(vmid)
		if !ok {
			writeError(w, http.StatusNotFound, "VM not found")
			return
		}
		node = vm.Node
	} else {
		cs, ok := h.store.GetCluster(clusterName)
		if !ok {
			writeError(w, http.StatusNotFound, "cluster not found")
			return
		}
		ct, ok := cs.GetContainer(vmid)
		if !ok {
			writeError(w, http.StatusNotFound, "container not found")
			return
		}
		node = ct.Node
	}

	// Get client for any node (HA is cluster-wide)
	client, ok := h.getClient(clusterName, node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "node client not found")
		return
	}

	// Enable HA
	cfg := pve.HAResourceConfig{
		State:       req.State,
		Group:       req.Group,
		MaxRestart:  req.MaxRestart,
		MaxRelocate: req.MaxRelocate,
		Comment:     req.Comment,
	}

	if err := client.EnableHA(r.Context(), guestType, vmid, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enable HA: "+err.Error())
		return
	}

	writeJSON(w, map[string]string{"message": "HA enabled"})
}

// DisableHA disables HA for a VM or container
func (h *Handler) DisableHA(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	guestType := r.PathValue("type")
	vmidStr := r.PathValue("vmid")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	if guestType != "vm" && guestType != "ct" {
		writeError(w, http.StatusBadRequest, "type must be 'vm' or 'ct'")
		return
	}

	// Find the guest to get its node
	var node string
	if guestType == "vm" {
		cs, ok := h.store.GetCluster(clusterName)
		if !ok {
			writeError(w, http.StatusNotFound, "cluster not found")
			return
		}
		vm, ok := cs.GetVM(vmid)
		if !ok {
			writeError(w, http.StatusNotFound, "VM not found")
			return
		}
		node = vm.Node
	} else {
		cs, ok := h.store.GetCluster(clusterName)
		if !ok {
			writeError(w, http.StatusNotFound, "cluster not found")
			return
		}
		ct, ok := cs.GetContainer(vmid)
		if !ok {
			writeError(w, http.StatusNotFound, "container not found")
			return
		}
		node = ct.Node
	}

	client, ok := h.getClient(clusterName, node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "node client not found")
		return
	}

	if err := client.DisableHA(r.Context(), guestType, vmid); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable HA: "+err.Error())
		return
	}

	writeJSON(w, map[string]string{"message": "HA disabled"})
}

// GetHAGroups returns available HA groups for a cluster
func (h *Handler) GetHAGroups(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")

	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	// Get any node's client to query cluster-wide HA groups
	nodes := cs.GetNodes()
	if len(nodes) == 0 {
		writeJSON(w, []interface{}{})
		return
	}

	client, ok := h.getClient(clusterName, nodes[0].Node)
	if !ok {
		writeError(w, http.StatusInternalServerError, "node client not found")
		return
	}

	groups, err := client.GetHAGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get HA groups: "+err.Error())
		return
	}

	writeJSON(w, groups)
}
