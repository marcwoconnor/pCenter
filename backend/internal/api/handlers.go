package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/agent"
	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/folders"
	"github.com/moconnor/pcenter/internal/inventory"
	"github.com/moconnor/pcenter/internal/metrics"
	"github.com/moconnor/pcenter/internal/poller"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

// Handler holds dependencies for API handlers
type Handler struct {
	store     *state.Store
	poller    *poller.Poller
	metrics   *metrics.QueryService
	folders   *folders.Service
	activity  *activity.Service
	inventory *inventory.Service
	agentHub  *agent.Hub
	clusters  []config.ClusterConfig // For on-demand client creation
	onChange  func()                 // Callback to broadcast state changes
}

// NewHandler creates a new API handler
func NewHandler(store *state.Store, p *poller.Poller) *Handler {
	return &Handler{
		store:  store,
		poller: p,
	}
}

// SetMetricsService sets the metrics query service
func (h *Handler) SetMetricsService(m *metrics.QueryService) {
	h.metrics = m
}

// SetFoldersService sets the folders service
func (h *Handler) SetFoldersService(f *folders.Service) {
	h.folders = f
}

// SetActivityService sets the activity logging service
func (h *Handler) SetActivityService(a *activity.Service) {
	h.activity = a
}

// SetInventoryService sets the inventory service for datacenter/cluster management
func (h *Handler) SetInventoryService(inv *inventory.Service) {
	h.inventory = inv
}

// SetOnChange sets a callback for state changes (broadcasts to WebSocket)
func (h *Handler) SetOnChange(fn func()) {
	h.onChange = fn
}

// SetAgentHub sets the agent hub for command execution
func (h *Handler) SetAgentHub(hub *agent.Hub) {
	h.agentHub = hub
}

// SetClusters sets the cluster configs for on-demand client creation
func (h *Handler) SetClusters(clusters []config.ClusterConfig) {
	h.clusters = clusters
}

// getClient returns the PVE client for a cluster/node combination
func (h *Handler) getClient(cluster, node string) (*pve.Client, bool) {
	// Try poller first
	if h.poller != nil {
		clients := h.poller.GetClusterClients(cluster)
		if clients != nil {
			if client, ok := clients[node]; ok {
				return client, true
			}
		}
	}

	// Fall back to on-demand client creation for agent-only mode
	return h.createOnDemandClient(cluster, node)
}

// createOnDemandClient creates a PVE client on-demand from cluster config
func (h *Handler) createOnDemandClient(clusterName, node string) (*pve.Client, bool) {
	// Find cluster config
	var clusterCfg *config.ClusterConfig
	for _, c := range h.clusters {
		if c.Name == clusterName {
			clusterCfg = &c
			break
		}
	}
	if clusterCfg == nil {
		return nil, false
	}

	// Create client using discovery node (Proxmox will route to correct node)
	client := pve.NewClientForNode(*clusterCfg, node, "")
	return client, true
}

// pollerAvailable returns true if the poller is running
func (h *Handler) pollerAvailable() bool {
	return h.poller != nil
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

// GetStorageContent returns volumes on a specific storage
func (h *Handler) GetStorageContent(w http.ResponseWriter, r *http.Request) {
	storageName := r.PathValue("storage")
	node := r.URL.Query().Get("node") // optional: specific node

	// Find the storage to determine which node to query
	allStorage := h.store.GetStorage("")
	var targetNode, cluster string
	for _, s := range allStorage {
		if s.Storage == storageName {
			if node != "" && s.Node != node {
				continue
			}
			targetNode = s.Node
			cluster = s.Cluster
			break
		}
	}

	if targetNode == "" {
		writeError(w, http.StatusNotFound, "storage not found")
		return
	}

	// Try poller first (direct connection mode)
	if h.poller != nil {
		allClients := h.poller.GetAllClients()
		for _, clients := range allClients {
			for _, c := range clients {
				if c.NodeName() == targetNode {
					content, err := c.GetStorageContent(r.Context(), storageName)
					if err != nil {
						writeError(w, http.StatusInternalServerError, err.Error())
						return
					}
					writeJSON(w, content)
					return
				}
			}
		}
	}

	// Fall back to agent mode
	if h.agentHub != nil {
		cmd := &agent.CommandData{
			ID:     fmt.Sprintf("storage-%d", time.Now().UnixNano()),
			Action: "storage_content",
			Params: map[string]interface{}{
				"storage": storageName,
			},
		}

		resultCh, err := h.agentHub.SendCommand(cluster, targetNode, cmd)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		select {
		case result := <-resultCh:
			if !result.Success {
				writeError(w, http.StatusInternalServerError, result.Error)
				return
			}
			// Result.Output is JSON, parse and return
			var content []pve.StorageVolume
			if err := json.Unmarshal([]byte(result.Output), &content); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to parse agent response")
				return
			}
			writeJSON(w, content)
			return

		case <-ctx.Done():
			writeError(w, http.StatusGatewayTimeout, "agent command timed out")
			return
		}
	}

	writeError(w, http.StatusServiceUnavailable, "no connection to node "+targetNode)
}

// UploadToStorage handles file uploads to storage (ISO, templates, etc.)
func (h *Handler) UploadToStorage(w http.ResponseWriter, r *http.Request) {
	storageName := r.PathValue("storage")
	node := r.URL.Query().Get("node")
	contentType := r.URL.Query().Get("content") // iso, vztmpl, etc.

	if contentType == "" {
		contentType = "iso" // default to ISO
	}

	// Parse multipart form (max 10GB)
	if err := r.ParseMultipartForm(10 << 30); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "no file provided: "+err.Error())
		return
	}
	defer file.Close()

	// Find the storage to determine which node to use
	allStorage := h.store.GetStorage("")
	var targetNode string
	for _, s := range allStorage {
		if s.Storage == storageName {
			if node != "" && s.Node != node {
				continue
			}
			targetNode = s.Node
			break
		}
	}

	if targetNode == "" {
		writeError(w, http.StatusNotFound, "storage not found")
		return
	}

	if h.poller == nil {
		writeError(w, http.StatusServiceUnavailable, "upload requires direct cluster connection (not available in agent-only mode)")
		return
	}

	// Find client for target node
	allClients := h.poller.GetAllClients()
	for _, clients := range allClients {
		for _, c := range clients {
			if c.NodeName() == targetNode {
				upid, err := c.UploadToStorage(r.Context(), storageName, contentType, header.Filename, file, header.Size)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				writeJSON(w, map[string]string{"upid": upid, "filename": header.Filename})
				return
			}
		}
	}

	writeError(w, http.StatusServiceUnavailable, "no client for node "+targetNode)
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

// CephCommandRequest is the request body for running a Ceph command
type CephCommandRequest struct {
	Command string `json:"command"` // e.g., "pg_repair"
	PgID    string `json:"pg_id,omitempty"`
}

// CephCommandResponse is the response from running a Ceph command
type CephCommandResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// RunCephCommand executes a whitelisted Ceph command on a cluster node
func (h *Handler) RunCephCommand(w http.ResponseWriter, r *http.Request) {
	var req CephCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get any available client to run the command
	var client *pve.Client
	if h.poller != nil {
		allClients := h.poller.GetAllClients()
		for _, clients := range allClients {
			for _, c := range clients {
				client = c
				break
			}
			if client != nil {
				break
			}
		}
	}

	if client == nil {
		writeError(w, http.StatusServiceUnavailable, "no cluster connection available (agent-only mode)")
		return
	}

	// Execute the command
	output, err := client.RunCephCommand(r.Context(), req.Command, req.PgID)
	if err != nil {
		writeJSON(w, CephCommandResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	writeJSON(w, CephCommandResponse{
		Success: true,
		Output:  output,
	})
}

// GetSmart returns SMART data for all disks across all nodes
func (h *Handler) GetSmart(w http.ResponseWriter, r *http.Request) {
	if h.poller == nil {
		writeJSON(w, []pve.SmartDisk{})
		return
	}
	allClients := h.poller.GetAllClients()

	var allDisks []pve.SmartDisk
	for _, clients := range allClients {
		for _, client := range clients {
			disks, err := client.GetSmartData(r.Context())
			if err != nil {
				continue // Log and skip failed nodes
			}
			allDisks = append(allDisks, disks...)
		}
	}

	writeJSON(w, allDisks)
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

// --- Maintenance Mode ---

// GetQDeviceStatus returns the qdevice status for a cluster
func (h *Handler) GetQDeviceStatus(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")

	if h.poller == nil {
		writeError(w, http.StatusServiceUnavailable, "feature unavailable in agent-only mode")
		return
	}
	clients := h.poller.GetClusterClients(clusterName)
	if clients == nil {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	// Get qdevice status from any node
	var qdeviceStatus *pve.QDeviceStatus
	var qdeviceVMNode string
	var qdeviceVM *pve.QDeviceStatus

	for nodeName, client := range clients {
		status, err := client.GetQDeviceStatus(r.Context())
		if err == nil && status != nil && status.Configured {
			qdeviceStatus = status
		}

		// Check if this node has the qdevice VM
		vmStatus, err := client.FindQDeviceVM(r.Context(), "")
		if err == nil && vmStatus != nil {
			qdeviceVMNode = nodeName
			qdeviceVM = vmStatus
		}
	}

	if qdeviceStatus == nil {
		qdeviceStatus = &pve.QDeviceStatus{Configured: false}
	}

	// Merge qdevice VM info
	if qdeviceVM != nil {
		qdeviceStatus.HostNode = qdeviceVMNode
		qdeviceStatus.HostVMID = qdeviceVM.HostVMID
		qdeviceStatus.HostVMName = qdeviceVM.HostVMName
	}

	writeJSON(w, qdeviceStatus)
}

// GetMaintenancePreflight returns pre-flight checks for entering maintenance mode
func (h *Handler) GetMaintenancePreflight(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	nodeName := r.PathValue("node")

	if h.poller == nil {
		writeError(w, http.StatusServiceUnavailable, "feature unavailable in agent-only mode")
		return
	}
	clients := h.poller.GetClusterClients(clusterName)
	if clients == nil {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	client, ok := clients[nodeName]
	if !ok {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	// Get all node names
	var allNodes []string
	for n := range clients {
		allNodes = append(allNodes, n)
	}

	preflight, err := client.GetMaintenancePreflight(r.Context(), nodeName, allNodes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Find guests on this node
	cs, ok := h.store.GetCluster(clusterName)
	if ok {
		vms := cs.GetVMs()
		cts := cs.GetContainers()

		var otherNode string
		for _, n := range allNodes {
			if n != nodeName {
				otherNode = n
				break
			}
		}

		for _, vm := range vms {
			if vm.Node == nodeName {
				guest := pve.GuestToMove{
					VMID:       vm.VMID,
					Name:       vm.Name,
					Type:       "qemu",
					Status:     vm.Status,
					TargetNode: otherNode,
				}
				// Check if this is the qdevice VM
				if strings.Contains(strings.ToLower(vm.Name), "osd-mon") ||
					strings.Contains(strings.ToLower(vm.Name), "qdevice") {
					guest.IsCritical = true
					guest.Reason = "QDevice/Ceph Monitor - must migrate first"
					preflight.CriticalGuests = append(preflight.CriticalGuests, guest)
				}
				preflight.GuestsToMove = append(preflight.GuestsToMove, guest)
			}
		}

		for _, ct := range cts {
			if ct.Node == nodeName {
				guest := pve.GuestToMove{
					VMID:       ct.VMID,
					Name:       ct.Name,
					Type:       "lxc",
					Status:     ct.Status,
					TargetNode: otherNode,
				}
				preflight.GuestsToMove = append(preflight.GuestsToMove, guest)
			}
		}
	}

	// Add check for guests count
	if len(preflight.GuestsToMove) > 0 {
		preflight.Checks = append(preflight.Checks, pve.MaintenancePreflightCheck{
			Name:    "Guests to Migrate",
			Status:  "warning",
			Message: fmt.Sprintf("%d guests will be migrated", len(preflight.GuestsToMove)),
		})
	}

	if len(preflight.CriticalGuests) > 0 {
		preflight.Checks = append(preflight.Checks, pve.MaintenancePreflightCheck{
			Name:    "Critical Guests",
			Status:  "warning",
			Message: fmt.Sprintf("%d critical guests (qdevice) will be migrated first", len(preflight.CriticalGuests)),
		})
	}

	writeJSON(w, preflight)
}

// GetMaintenanceState returns the current maintenance state for a node
func (h *Handler) GetMaintenanceState(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	nodeName := r.PathValue("node")

	state := h.store.GetMaintenanceState(clusterName, nodeName)
	if state == nil {
		state = &pve.MaintenanceState{
			Node:          nodeName,
			InMaintenance: false,
		}
	}
	writeJSON(w, state)
}

// EnterMaintenanceMode starts the maintenance mode process for a node
func (h *Handler) EnterMaintenanceMode(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	nodeName := r.PathValue("node")

	if h.poller == nil {
		writeError(w, http.StatusServiceUnavailable, "feature unavailable in agent-only mode")
		return
	}
	clients := h.poller.GetClusterClients(clusterName)
	if clients == nil {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	client, ok := clients[nodeName]
	if !ok {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	// Set initial maintenance state
	state := &pve.MaintenanceState{
		Node:          nodeName,
		InMaintenance: true,
		EnteredAt:     time.Now(),
		Phase:         "starting",
		Progress:      0,
		Message:       "Setting Ceph noout flag...",
	}
	h.store.SetMaintenanceState(clusterName, nodeName, state)

	// Set Ceph noout flag
	if err := client.SetCephNoout(r.Context(), true); err != nil {
		state.Phase = "error"
		state.Message = fmt.Sprintf("Failed to set noout: %v", err)
		h.store.SetMaintenanceState(clusterName, nodeName, state)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	state.Phase = "evacuating"
	state.Progress = 10
	state.Message = "Starting guest evacuation..."
	h.store.SetMaintenanceState(clusterName, nodeName, state)

	// Start evacuation in background
	go h.evacuateNode(clusterName, nodeName, clients)

	writeJSON(w, state)
}

// evacuateNode migrates all guests from a node (runs in background)
func (h *Handler) evacuateNode(clusterName, nodeName string, clients map[string]*pve.Client) {
	ctx := context.Background()
	client := clients[nodeName]

	// Find target node
	var targetNode string
	var targetClient *pve.Client
	for n, c := range clients {
		if n != nodeName {
			targetNode = n
			targetClient = c
			break
		}
	}

	if targetNode == "" {
		state := h.store.GetMaintenanceState(clusterName, nodeName)
		state.Phase = "error"
		state.Message = "No target node available"
		h.store.SetMaintenanceState(clusterName, nodeName, state)
		return
	}

	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		return
	}

	vms := cs.GetVMs()
	cts := cs.GetContainers()

	var guestsToMigrate []pve.GuestToMove

	// Critical guests first (qdevice VM)
	for _, vm := range vms {
		if vm.Node == nodeName {
			guest := pve.GuestToMove{VMID: vm.VMID, Name: vm.Name, Type: "qemu", Status: vm.Status}
			if strings.Contains(strings.ToLower(vm.Name), "osd-mon") {
				guest.IsCritical = true
				guestsToMigrate = append([]pve.GuestToMove{guest}, guestsToMigrate...)
			} else {
				guestsToMigrate = append(guestsToMigrate, guest)
			}
		}
	}

	for _, ct := range cts {
		if ct.Node == nodeName {
			guestsToMigrate = append(guestsToMigrate, pve.GuestToMove{
				VMID: ct.VMID, Name: ct.Name, Type: "lxc", Status: ct.Status,
			})
		}
	}

	total := len(guestsToMigrate)
	for i, guest := range guestsToMigrate {
		state := h.store.GetMaintenanceState(clusterName, nodeName)
		state.Progress = 10 + (80 * i / max(total, 1))
		state.Message = fmt.Sprintf("Migrating %s (%d/%d)...", guest.Name, i+1, total)
		h.store.SetMaintenanceState(clusterName, nodeName, state)

		// Perform migration
		var upid string
		var err error
		online := guest.Status == "running"

		if guest.Type == "qemu" {
			upid, err = client.MigrateVM(ctx, guest.VMID, targetNode, online)
		} else {
			upid, err = client.MigrateContainer(ctx, guest.VMID, targetNode, online)
		}

		if err != nil {
			state.Message = fmt.Sprintf("Failed to migrate %s: %v", guest.Name, err)
			// Continue with next guest
			continue
		}

		// Wait for migration to complete
		_ = targetClient // Use target client to check task status
		_ = upid
		time.Sleep(5 * time.Second) // Simple wait - could poll task status
	}

	// Evacuation complete
	state := h.store.GetMaintenanceState(clusterName, nodeName)
	state.Phase = "ready"
	state.Progress = 100
	state.Message = "Maintenance mode ready - host can be rebooted"
	h.store.SetMaintenanceState(clusterName, nodeName, state)
}

// ExitMaintenanceMode exits maintenance mode for a node
func (h *Handler) ExitMaintenanceMode(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	nodeName := r.PathValue("node")

	if h.poller == nil {
		writeError(w, http.StatusServiceUnavailable, "feature unavailable in agent-only mode")
		return
	}
	clients := h.poller.GetClusterClients(clusterName)
	if clients == nil {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	client, ok := clients[nodeName]
	if !ok {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	// Clear Ceph noout flag
	if err := client.SetCephNoout(r.Context(), false); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to unset noout: %v", err))
		return
	}

	// Clear maintenance state
	h.store.SetMaintenanceState(clusterName, nodeName, nil)

	writeJSON(w, map[string]string{"status": "ok", "message": "Exited maintenance mode"})
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

// ClearMigration removes a stale migration from tracking
func (h *Handler) ClearMigration(w http.ResponseWriter, r *http.Request) {
	upid := r.PathValue("upid")
	if upid == "" {
		writeError(w, http.StatusBadRequest, "upid required")
		return
	}
	h.store.RemoveMigration(upid)
	writeJSON(w, map[string]string{"message": "migration cleared"})
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

	// Log activity
	if h.activity != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"from_node": vm.Node,
			"to_node":   req.TargetNode,
			"online":    req.Online,
			"upid":      upid,
		})
		h.activity.Log(activity.Entry{
			Action:       activity.ActionMigrate,
			ResourceType: "vm",
			ResourceID:   vmidStr,
			ResourceName: vm.Name,
			Cluster:      vm.Cluster,
			Details:      string(details),
			Status:       "started",
		})
	}

	// Broadcast state so UI shows migration immediately
	if h.onChange != nil {
		h.onChange()
	}

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

	// Log activity
	if h.activity != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"from_node": ct.Node,
			"to_node":   req.TargetNode,
			"online":    req.Online,
			"upid":      upid,
		})
		h.activity.Log(activity.Entry{
			Action:       activity.ActionMigrate,
			ResourceType: "ct",
			ResourceID:   vmidStr,
			ResourceName: ct.Name,
			Cluster:      ct.Cluster,
			Details:      string(details),
			Status:       "started",
		})
	}

	// Broadcast state so UI shows migration immediately
	if h.onChange != nil {
		h.onChange()
	}

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

	// Log activity
	if h.activity != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"from_node": vm.Node,
			"to_node":   req.TargetNode,
			"online":    req.Online,
			"upid":      upid,
		})
		h.activity.Log(activity.Entry{
			Action:       activity.ActionMigrate,
			ResourceType: "vm",
			ResourceID:   vmidStr,
			ResourceName: vm.Name,
			Cluster:      clusterName,
			Details:      string(details),
			Status:       "started",
		})
	}

	// Broadcast state so UI shows migration immediately
	if h.onChange != nil {
		h.onChange()
	}

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

	// Log activity
	if h.activity != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"from_node": ct.Node,
			"to_node":   req.TargetNode,
			"online":    req.Online,
			"upid":      upid,
		})
		h.activity.Log(activity.Entry{
			Action:       activity.ActionMigrate,
			ResourceType: "ct",
			ResourceID:   vmidStr,
			ResourceName: ct.Name,
			Cluster:      clusterName,
			Details:      string(details),
			Status:       "started",
		})
	}

	// Broadcast state so UI shows migration immediately
	if h.onChange != nil {
		h.onChange()
	}

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

	// Log activity
	if h.activity != nil {
		details, _ := json.Marshal(map[string]interface{}{
			"from_node":   found.FromNode,
			"to_node":     found.ToNode,
			"online":      online,
			"upid":        upid,
			"drs_reason":  found.Reason,
		})
		h.activity.Log(activity.Entry{
			Action:       activity.ActionDRSApply,
			ResourceType: found.GuestType,
			ResourceID:   strconv.Itoa(found.VMID),
			ResourceName: found.GuestName,
			Cluster:      clusterName,
			Details:      string(details),
			Status:       "started",
		})
	}

	// Broadcast state so UI shows migration immediately
	if h.onChange != nil {
		h.onChange()
	}

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

// --- Network/SDN Handlers ---

// GetClusterNetworkInterfaces returns all network interfaces for a cluster
func (h *Handler) GetClusterNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	// Optional node filter
	node := r.URL.Query().Get("node")
	writeJSON(w, cs.GetNetworkInterfaces(node))
}

// GetClusterSDNZones returns all SDN zones for a cluster
func (h *Handler) GetClusterSDNZones(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}
	writeJSON(w, cs.GetSDNZones())
}

// GetClusterSDNVNets returns all SDN virtual networks for a cluster
func (h *Handler) GetClusterSDNVNets(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}
	writeJSON(w, cs.GetSDNVNets())
}

// GetClusterSDNSubnets returns all SDN subnets for a cluster
func (h *Handler) GetClusterSDNSubnets(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}
	writeJSON(w, cs.GetSDNSubnets())
}

// NetworkOverview provides aggregated network data
type NetworkOverview struct {
	Interfaces  []pve.NetworkInterface `json:"interfaces"`
	SDNZones    []pve.SDNZone          `json:"sdn_zones"`
	SDNVNets    []pve.SDNVNet          `json:"sdn_vnets"`
	SDNSubnets  []pve.SDNSubnet        `json:"sdn_subnets"`
}

// GetClusterNetwork returns aggregated network data for a cluster
func (h *Handler) GetClusterNetwork(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	cs, ok := h.store.GetCluster(clusterName)
	if !ok {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	overview := NetworkOverview{
		Interfaces:  cs.GetNetworkInterfaces(""),
		SDNZones:    cs.GetSDNZones(),
		SDNVNets:    cs.GetSDNVNets(),
		SDNSubnets:  cs.GetSDNSubnets(),
	}

	// Ensure non-nil slices for JSON
	if overview.Interfaces == nil {
		overview.Interfaces = []pve.NetworkInterface{}
	}
	if overview.SDNZones == nil {
		overview.SDNZones = []pve.SDNZone{}
	}
	if overview.SDNVNets == nil {
		overview.SDNVNets = []pve.SDNVNet{}
	}
	if overview.SDNSubnets == nil {
		overview.SDNSubnets = []pve.SDNSubnet{}
	}

	writeJSON(w, overview)
}

// --- Metrics Handlers ---

// parseMetricQuery parses common metrics query parameters from a request
func (h *Handler) parseMetricQuery(r *http.Request) metrics.MetricQuery {
	query := metrics.MetricQuery{
		Resolution: r.URL.Query().Get("resolution"),
	}

	// Parse time range
	if start := r.URL.Query().Get("start"); start != "" {
		if ts, err := strconv.ParseInt(start, 10, 64); err == nil {
			query.StartTime = time.Unix(ts, 0)
		}
	}
	if query.StartTime.IsZero() {
		query.StartTime = time.Now().Add(-time.Hour) // Default: last hour
	}

	if end := r.URL.Query().Get("end"); end != "" {
		if ts, err := strconv.ParseInt(end, 10, 64); err == nil {
			query.EndTime = time.Unix(ts, 0)
		}
	}
	if query.EndTime.IsZero() {
		query.EndTime = time.Now()
	}

	// Parse metric types
	if metricsParam := r.URL.Query().Get("metrics"); metricsParam != "" {
		query.MetricTypes = strings.Split(metricsParam, ",")
	} else {
		query.MetricTypes = []string{"cpu", "mem_percent"} // Default metrics
	}

	return query
}

// GetMetrics handles general metrics queries
func (h *Handler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	if h.metrics == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics not enabled")
		return
	}

	query := h.parseMetricQuery(r)
	query.Cluster = r.URL.Query().Get("cluster")
	query.ResourceType = r.URL.Query().Get("resource_type")
	query.ResourceID = r.URL.Query().Get("resource_id")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := h.metrics.Query(ctx, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, result)
}

// GetNodeMetrics returns metrics for a specific node
func (h *Handler) GetNodeMetrics(w http.ResponseWriter, r *http.Request) {
	if h.metrics == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics not enabled")
		return
	}

	query := h.parseMetricQuery(r)
	query.ResourceType = "node"
	query.ResourceID = r.PathValue("node")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := h.metrics.Query(ctx, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, result)
}

// GetVMMetrics returns metrics for a specific VM
func (h *Handler) GetVMMetrics(w http.ResponseWriter, r *http.Request) {
	if h.metrics == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics not enabled")
		return
	}

	query := h.parseMetricQuery(r)
	query.ResourceType = "vm"
	query.ResourceID = r.PathValue("vmid")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := h.metrics.Query(ctx, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, result)
}

// GetContainerMetrics returns metrics for a specific container
func (h *Handler) GetContainerMetrics(w http.ResponseWriter, r *http.Request) {
	if h.metrics == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics not enabled")
		return
	}

	query := h.parseMetricQuery(r)
	query.ResourceType = "ct"
	query.ResourceID = r.PathValue("vmid")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := h.metrics.Query(ctx, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, result)
}

// GetClusterMetrics returns metrics for all resources in a cluster
func (h *Handler) GetClusterMetrics(w http.ResponseWriter, r *http.Request) {
	if h.metrics == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics not enabled")
		return
	}

	query := h.parseMetricQuery(r)
	query.Cluster = r.PathValue("cluster")
	query.ResourceType = r.URL.Query().Get("resource_type")
	query.ResourceID = r.URL.Query().Get("resource_id")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := h.metrics.Query(ctx, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, result)
}

// --- Folder Handlers ---

// GetFolderTree returns the folder tree for a specific view (hosts or vms)
func (h *Handler) GetFolderTree(w http.ResponseWriter, r *http.Request) {
	if h.folders == nil {
		writeError(w, http.StatusServiceUnavailable, "folders not enabled")
		return
	}

	tree := r.PathValue("tree")
	var treeView folders.TreeView
	switch tree {
	case "hosts":
		treeView = folders.TreeViewHosts
	case "vms":
		treeView = folders.TreeViewVMs
	default:
		writeError(w, http.StatusBadRequest, "invalid tree: must be 'hosts' or 'vms'")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := h.folders.GetFolderTree(ctx, treeView)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, result)
}

// CreateFolder creates a new folder
func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	if h.folders == nil {
		writeError(w, http.StatusServiceUnavailable, "folders not enabled")
		return
	}

	var req folders.CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	folder, err := h.folders.CreateFolder(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, folder)
}

// RenameFolder renames a folder
func (h *Handler) RenameFolder(w http.ResponseWriter, r *http.Request) {
	if h.folders == nil {
		writeError(w, http.StatusServiceUnavailable, "folders not enabled")
		return
	}

	id := r.PathValue("id")
	var req folders.RenameFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.folders.RenameFolder(ctx, id, req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteFolder deletes a folder
func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	if h.folders == nil {
		writeError(w, http.StatusServiceUnavailable, "folders not enabled")
		return
	}

	id := r.PathValue("id")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.folders.DeleteFolder(ctx, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// MoveFolder moves a folder to a new parent
func (h *Handler) MoveFolder(w http.ResponseWriter, r *http.Request) {
	if h.folders == nil {
		writeError(w, http.StatusServiceUnavailable, "folders not enabled")
		return
	}

	id := r.PathValue("id")
	var req folders.MoveFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.folders.MoveFolder(ctx, id, req.ParentID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AddFolderMember adds a resource to a folder
func (h *Handler) AddFolderMember(w http.ResponseWriter, r *http.Request) {
	if h.folders == nil {
		writeError(w, http.StatusServiceUnavailable, "folders not enabled")
		return
	}

	folderID := r.PathValue("id")
	var req folders.AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.folders.AddMember(ctx, folderID, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveFolderMember removes a resource from a folder
func (h *Handler) RemoveFolderMember(w http.ResponseWriter, r *http.Request) {
	if h.folders == nil {
		writeError(w, http.StatusServiceUnavailable, "folders not enabled")
		return
	}

	folderID := r.PathValue("id")
	var req folders.RemoveMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.folders.RemoveMember(ctx, folderID, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// MoveResource moves a resource to a folder
func (h *Handler) MoveResource(w http.ResponseWriter, r *http.Request) {
	if h.folders == nil {
		writeError(w, http.StatusServiceUnavailable, "folders not enabled")
		return
	}

	var req folders.MoveResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Determine tree view from query param or default to hosts
	tree := r.URL.Query().Get("tree")
	var treeView folders.TreeView
	switch tree {
	case "vms":
		treeView = folders.TreeViewVMs
	default:
		treeView = folders.TreeViewHosts
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.folders.MoveResource(ctx, req, treeView); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Configuration Handlers ---

// GetClusterVMConfig returns the full configuration for a VM
func (h *Handler) GetClusterVMConfig(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	vmidStr := r.PathValue("vmid")

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
		writeError(w, http.StatusServiceUnavailable, "node client not available")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	config, err := client.GetVMConfig(ctx, vmid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return wrapped response with metadata
	writeJSON(w, pve.ConfigResponse{
		Config: config,
		Digest: config.Digest,
		Node:   vm.Node,
		VMID:   vmid,
	})
}

// GetClusterContainerConfig returns the full configuration for a container
func (h *Handler) GetClusterContainerConfig(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	vmidStr := r.PathValue("vmid")

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
		writeError(w, http.StatusServiceUnavailable, "node client not available")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	config, err := client.GetContainerConfig(ctx, vmid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return wrapped response with metadata
	writeJSON(w, pve.ConfigResponse{
		Config: config,
		Digest: config.Digest,
		Node:   ct.Node,
		VMID:   vmid,
	})
}

// UpdateClusterVMConfig updates VM configuration with optimistic locking
func (h *Handler) UpdateClusterVMConfig(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	vmidStr := r.PathValue("vmid")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	var req pve.ConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Digest == "" {
		writeError(w, http.StatusBadRequest, "digest required for config updates")
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
		writeError(w, http.StatusServiceUnavailable, "node client not available")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := client.UpdateVMConfig(ctx, vmid, &req); err != nil {
		// Check for digest mismatch (412 Precondition Failed from Proxmox)
		if strings.Contains(err.Error(), "412") || strings.Contains(err.Error(), "digest") {
			writeError(w, http.StatusConflict, "configuration changed by another user, please refresh")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		changedKeys := make([]string, 0, len(req.Changes))
		for k := range req.Changes {
			changedKeys = append(changedKeys, k)
		}
		details, _ := json.Marshal(map[string]interface{}{"changed": changedKeys})
		h.activity.Log(activity.Entry{
			Action:       activity.ActionConfigUpdate,
			ResourceType: "vm",
			ResourceID:   vmidStr,
			ResourceName: vm.Name,
			Cluster:      clusterName,
			Details:      string(details),
		})
	}

	writeJSON(w, map[string]string{"message": "configuration updated"})
}

// UpdateClusterContainerConfig updates container configuration with optimistic locking
func (h *Handler) UpdateClusterContainerConfig(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	vmidStr := r.PathValue("vmid")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	var req pve.ConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Digest == "" {
		writeError(w, http.StatusBadRequest, "digest required for config updates")
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
		writeError(w, http.StatusServiceUnavailable, "node client not available")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := client.UpdateContainerConfig(ctx, vmid, &req); err != nil {
		// Check for digest mismatch (412 Precondition Failed from Proxmox)
		if strings.Contains(err.Error(), "412") || strings.Contains(err.Error(), "digest") {
			writeError(w, http.StatusConflict, "configuration changed by another user, please refresh")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		changedKeys := make([]string, 0, len(req.Changes))
		for k := range req.Changes {
			changedKeys = append(changedKeys, k)
		}
		details, _ := json.Marshal(map[string]interface{}{"changed": changedKeys})
		h.activity.Log(activity.Entry{
			Action:       activity.ActionConfigUpdate,
			ResourceType: "ct",
			ResourceID:   vmidStr,
			ResourceName: ct.Name,
			Cluster:      clusterName,
			Details:      string(details),
		})
	}

	writeJSON(w, map[string]string{"message": "configuration updated"})
}

// GetActivity retrieves activity log entries
func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {
	if h.activity == nil {
		writeError(w, http.StatusServiceUnavailable, "activity logging not enabled")
		return
	}

	params := activity.QueryParams{
		Limit:        50,
		ResourceType: r.URL.Query().Get("resource_type"),
		ResourceID:   r.URL.Query().Get("resource_id"),
		Cluster:      r.URL.Query().Get("cluster"),
		Action:       r.URL.Query().Get("action"),
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = limit
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			params.Offset = offset
		}
	}

	entries, err := h.activity.Query(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if entries == nil {
		entries = []activity.Entry{}
	}

	writeJSON(w, entries)
}

// === Datacenter Handlers ===

// ListDatacenters returns all datacenters
func (h *Handler) ListDatacenters(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	datacenters, err := h.inventory.ListDatacenters(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if datacenters == nil {
		datacenters = []inventory.Datacenter{}
	}

	writeJSON(w, datacenters)
}

// GetDatacenter returns a datacenter by ID
func (h *Handler) GetDatacenter(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "datacenter id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	dc, err := h.inventory.GetDatacenter(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dc == nil {
		writeError(w, http.StatusNotFound, "datacenter not found")
		return
	}

	writeJSON(w, dc)
}

// CreateDatacenter creates a new datacenter
func (h *Handler) CreateDatacenter(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	var req inventory.CreateDatacenterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	dc, err := h.inventory.CreateDatacenter(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "datacenter_create",
			ResourceType: "datacenter",
			ResourceID:   dc.ID,
			ResourceName: dc.Name,
		})
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, dc)
}

// UpdateDatacenter updates a datacenter
func (h *Handler) UpdateDatacenter(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "datacenter id is required")
		return
	}

	var req inventory.UpdateDatacenterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.inventory.UpdateDatacenter(ctx, id, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "datacenter_update",
			ResourceType: "datacenter",
			ResourceID:   id,
			ResourceName: req.Name,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteDatacenter deletes a datacenter
func (h *Handler) DeleteDatacenter(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "datacenter id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get DC name for logging before delete
	dc, _ := h.inventory.GetDatacenter(ctx, id)
	dcName := ""
	if dc != nil {
		dcName = dc.Name
	}

	if err := h.inventory.DeleteDatacenter(ctx, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "datacenter_delete",
			ResourceType: "datacenter",
			ResourceID:   id,
			ResourceName: dcName,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetDatacenterTree returns datacenters with their clusters
func (h *Handler) GetDatacenterTree(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	datacenters, orphanClusters, err := h.inventory.GetDatacenterTree(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"datacenters":     datacenters,
		"orphan_clusters": orphanClusters,
	})
}

// === Cluster Inventory Handlers ===

// ListInventoryClusters returns all clusters from inventory
func (h *Handler) ListInventoryClusters(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	clusters, err := h.inventory.ListClusters(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if clusters == nil {
		clusters = []inventory.Cluster{}
	}

	writeJSON(w, clusters)
}

// GetInventoryCluster returns a cluster by name
func (h *Handler) GetInventoryCluster(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "cluster name is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cluster, err := h.inventory.GetClusterByName(ctx, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cluster == nil {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	writeJSON(w, cluster)
}

// CreateInventoryCluster creates a new cluster
func (h *Handler) CreateInventoryCluster(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	var req inventory.CreateClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cluster, err := h.inventory.CreateCluster(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "cluster_create",
			ResourceType: "cluster",
			ResourceID:   cluster.ID,
			ResourceName: cluster.Name,
		})
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, cluster)
}

// UpdateInventoryCluster updates a cluster
func (h *Handler) UpdateInventoryCluster(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "cluster name is required")
		return
	}

	var req inventory.UpdateClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.inventory.UpdateClusterByName(ctx, name, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "cluster_update",
			ResourceType: "cluster",
			ResourceID:   name,
			ResourceName: req.Name,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteInventoryCluster deletes a cluster
func (h *Handler) DeleteInventoryCluster(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "cluster name is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.inventory.DeleteClusterByName(ctx, name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "cluster_delete",
			ResourceType: "cluster",
			ResourceID:   name,
			ResourceName: name,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// MoveClusterToDatacenter moves a cluster to a datacenter
func (h *Handler) MoveClusterToDatacenter(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "cluster name is required")
		return
	}

	var req struct {
		DatacenterID *string `json:"datacenter_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.inventory.MoveClusterToDatacenter(ctx, name, req.DatacenterID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		dcName := "(orphan)"
		if req.DatacenterID != nil {
			if dc, _ := h.inventory.GetDatacenter(ctx, *req.DatacenterID); dc != nil {
				dcName = dc.Name
			}
		}
		h.activity.Log(activity.Entry{
			Action:       "cluster_move",
			ResourceType: "cluster",
			ResourceID:   name,
			ResourceName: name,
			Details:      fmt.Sprintf(`{"datacenter":"%s"}`, dcName),
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// === Inventory Host Handlers ===

// ListClusterHosts returns hosts for a cluster
func (h *Handler) ListClusterHosts(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	clusterName := r.PathValue("name")
	if clusterName == "" {
		writeError(w, http.StatusBadRequest, "cluster name is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cluster, err := h.inventory.GetClusterByName(ctx, clusterName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cluster == nil {
		writeError(w, http.StatusNotFound, "cluster not found")
		return
	}

	hosts, err := h.inventory.ListHostsByCluster(ctx, cluster.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if hosts == nil {
		hosts = []inventory.InventoryHost{}
	}

	writeJSON(w, hosts)
}

// AddClusterHost adds a host to a cluster
func (h *Handler) AddClusterHost(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	clusterName := r.PathValue("name")
	if clusterName == "" {
		writeError(w, http.StatusBadRequest, "cluster name is required")
		return
	}

	var req inventory.AddHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	host, err := h.inventory.AddHostByClusterName(ctx, clusterName, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "host_add",
			ResourceType: "host",
			ResourceID:   host.ID,
			ResourceName: req.Address,
			Cluster:      clusterName,
		})
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, host)
}

// GetHost returns a host by ID
func (h *Handler) GetHost(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	hostID := r.PathValue("id")
	if hostID == "" {
		writeError(w, http.StatusBadRequest, "host ID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	host, err := h.inventory.GetHost(ctx, hostID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if host == nil {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}

	writeJSON(w, host)
}

// UpdateHost updates a host
func (h *Handler) UpdateHost(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	hostID := r.PathValue("id")
	if hostID == "" {
		writeError(w, http.StatusBadRequest, "host ID is required")
		return
	}

	var req inventory.UpdateHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.inventory.UpdateHost(ctx, hostID, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "host_update",
			ResourceType: "host",
			ResourceID:   hostID,
			ResourceName: req.Address,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteHost deletes a host
func (h *Handler) DeleteHost(w http.ResponseWriter, r *http.Request) {
	if h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "inventory not enabled")
		return
	}

	hostID := r.PathValue("id")
	if hostID == "" {
		writeError(w, http.StatusBadRequest, "host ID is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	host, err := h.inventory.GetHost(ctx, hostID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if host == nil {
		writeError(w, http.StatusNotFound, "host not found")
		return
	}

	if err := h.inventory.DeleteHost(ctx, hostID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Log activity
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "host_delete",
			ResourceType: "host",
			ResourceID:   hostID,
			ResourceName: host.Address,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}
