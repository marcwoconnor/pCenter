package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// Hub manages WebSocket connections and broadcasts
type Hub struct {
	store   *state.Store
	clients map[*Client]bool
	mu      sync.RWMutex

	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

// Client represents a WebSocket connection
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// NewHub creates a new WebSocket hub
func NewHub(store *state.Store) *Hub {
	return &Hub{
		store:      store,
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			slog.Debug("websocket client connected", "clients", len(h.clients))

			// Send initial state
			h.sendStateTo(client)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			slog.Debug("websocket client disconnected", "clients", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, disconnect
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastState sends current state to all clients
func (h *Hub) BroadcastState() {
	msg := h.buildStateMessage()
	if msg == nil {
		return
	}

	h.mu.RLock()
	clientCount := len(h.clients)
	h.mu.RUnlock()

	if clientCount > 0 {
		h.broadcast <- msg
		slog.Debug("broadcast state", "clients", clientCount)
	}
}

// BroadcastActivity sends an activity entry to all clients
func (h *Hub) BroadcastActivity(entry activity.Entry) {
	msg := WSMessage{
		Type:    "activity",
		Payload: entry,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("failed to marshal activity", "error", err)
		return
	}

	h.mu.RLock()
	clientCount := len(h.clients)
	h.mu.RUnlock()

	if clientCount > 0 {
		h.broadcast <- data
		slog.Debug("broadcast activity", "action", entry.Action, "clients", clientCount)
	}
}

func (h *Hub) sendStateTo(client *Client) {
	msg := h.buildStateMessage()
	if msg != nil {
		select {
		case client.send <- msg:
		default:
		}
	}
}

// WSMessage is the WebSocket message format
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// StatePayload contains full cluster state
type StatePayload struct {
	Clusters      []ClusterInfo        `json:"clusters"`
	Summary       state.Summary        `json:"summary"`
	Nodes         []NodeWithStatus     `json:"nodes"`
	Guests        []Guest              `json:"guests"`
	Storage       []StorageInfo        `json:"storage"`
	CephStatus    *CephInfo            `json:"ceph,omitempty"`
	Migrations    []pve.MigrationProgress `json:"migrations,omitempty"`
	DRS           []pve.DRSRecommendation `json:"drs_recommendations,omitempty"`
}

// ClusterInfo holds cluster summary
type ClusterInfo struct {
	Name    string        `json:"name"`
	Summary state.Summary `json:"summary"`
	HA      *HAInfo       `json:"ha,omitempty"`
}

// HAInfo holds HA status for a cluster
type HAInfo struct {
	Enabled bool   `json:"enabled"`
	Quorum  bool   `json:"quorum"`
	Manager string `json:"manager"` // manager node
}

// NodeWithStatus extends Node with polling status and details
type NodeWithStatus struct {
	Cluster       string   `json:"cluster"`
	Node          string   `json:"node"`
	Status        string   `json:"status"`
	CPU           float64  `json:"cpu"`
	MaxCPU        int      `json:"maxcpu"`
	Mem           int64    `json:"mem"`
	MaxMem        int64    `json:"maxmem"`
	Disk          int64    `json:"disk"`
	MaxDisk       int64    `json:"maxdisk"`
	Uptime        int64    `json:"uptime"`
	LastUpdate    int64    `json:"last_update"`
	Error         string   `json:"error,omitempty"`
	PVEVersion    string   `json:"pve_version,omitempty"`
	KernelVersion string   `json:"kernel_version,omitempty"`
	CPUModel      string   `json:"cpu_model,omitempty"`
	LoadAvg       []string `json:"loadavg,omitempty"`
}

// Guest is a unified VM/CT representation
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
	NICs    []pve.GuestNIC `json:"nics,omitempty"`
}

// StorageInfo contains storage details
type StorageInfo struct {
	Cluster string `json:"cluster"`
	Storage string `json:"storage"`
	Node    string `json:"node"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	Shared  bool   `json:"shared"`
	Content string `json:"content"`
	Used    int64  `json:"used"`
	Avail   int64  `json:"avail"`
	Total   int64  `json:"total"`
}

// CephHealthCheck contains details about a Ceph health check
type CephHealthCheck struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail,omitempty"`
}

// CephInfo contains Ceph cluster status
type CephInfo struct {
	Health     string                     `json:"health"`
	Checks     map[string]CephHealthCheck `json:"checks,omitempty"`
	BytesUsed  int64                      `json:"bytes_used"`
	BytesAvail int64                      `json:"bytes_avail"`
	BytesTotal int64                      `json:"bytes_total"`
}

func (h *Hub) buildStateMessage() []byte {
	globalSummary := h.store.GetGlobalSummary()

	// Build clusters list with HA info
	clusters := make([]ClusterInfo, 0, len(globalSummary.Clusters))
	for _, cs := range globalSummary.Clusters {
		ci := ClusterInfo{
			Name:    cs.Name,
			Summary: cs.Summary,
		}
		// Get HA status if available
		if cluster, ok := h.store.GetCluster(cs.Name); ok {
			if ha := cluster.GetHAStatus(); ha != nil {
				ci.HA = &HAInfo{
					Enabled: ha.Enabled,
					Quorum:  ha.Quorum,
					Manager: ha.Manager.Node,
				}
			}
		}
		clusters = append(clusters, ci)
	}

	// Build nodes list with details
	nodes := h.store.GetNodes()
	statuses := h.store.GetAllNodeStatuses()

	// Collect node details from all clusters
	allNodeDetails := make(map[string]*pve.NodeStatus) // keyed by "cluster/node"
	for _, cs := range globalSummary.Clusters {
		if cluster, ok := h.store.GetCluster(cs.Name); ok {
			for nodeName, details := range cluster.GetNodeDetails() {
				allNodeDetails[cs.Name+"/"+nodeName] = details
			}
		}
	}

	nodeList := make([]NodeWithStatus, 0, len(nodes))
	for _, n := range nodes {
		nws := NodeWithStatus{
			Cluster: n.Cluster,
			Node:    n.Node,
			Status:  n.Status,
			CPU:     n.CPU,
			MaxCPU:  n.MaxCPU,
			Mem:     n.Mem,
			MaxMem:  n.MaxMem,
			Disk:    n.Disk,
			MaxDisk: n.MaxDisk,
			Uptime:  n.Uptime,
		}
		// Try cluster/node key first
		key := n.Cluster + "/" + n.Node
		if status, ok := statuses[key]; ok {
			nws.LastUpdate = status.LastUpdate.Unix()
			if status.Error != nil {
				nws.Error = status.Error.Error()
			}
		} else if status, ok := statuses[n.Node]; ok {
			// Fallback to node-only key
			nws.LastUpdate = status.LastUpdate.Unix()
			if status.Error != nil {
				nws.Error = status.Error.Error()
			}
		}
		// Add node details (version, kernel, etc.)
		if details, ok := allNodeDetails[key]; ok && details != nil {
			nws.PVEVersion = details.PVEVersion
			nws.KernelVersion = details.KernelVersion
			nws.CPUModel = details.CPUModel
			nws.LoadAvg = details.LoadAvg
		}
		nodeList = append(nodeList, nws)
	}

	// Build guests list
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
			NICs:    vm.NICs,
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
			NICs:    ct.NICs,
		})
	}

	// IMPORTANT: Sort guests by VMID for consistent ordering.
	// Go map iteration order is random, so without sorting, the guest order
	// changes on every WebSocket broadcast. This caused the Network page's
	// Virtual Switches view to constantly rearrange VMs, making the UI unusable.
	// The frontend receives this data via useCluster() context, and any order
	// change triggers React re-renders with elements in new positions.
	sort.Slice(guests, func(i, j int) bool {
		return guests[i].VMID < guests[j].VMID
	})

	// Build storage list
	var storageList []StorageInfo
	for _, s := range h.store.GetStorage("") {
		storageList = append(storageList, StorageInfo{
			Cluster: s.Cluster,
			Storage: s.Storage,
			Node:    s.Node,
			Type:    s.Type,
			Status:  s.Status,
			Shared:  s.Shared == 1,
			Content: s.Content,
			Used:    s.Used,
			Avail:   s.Avail,
			Total:   s.Total,
		})
	}

	// Get Ceph status
	var cephInfo *CephInfo
	if ceph := h.store.GetCeph(); ceph != nil {
		cephInfo = &CephInfo{
			Health:     ceph.Health.Status,
			BytesUsed:  ceph.PGMap.BytesUsed,
			BytesAvail: ceph.PGMap.BytesAvail,
			BytesTotal: ceph.PGMap.BytesTotal,
		}
		// Map health checks if present
		if len(ceph.Health.Checks) > 0 {
			cephInfo.Checks = make(map[string]CephHealthCheck)
			for name, check := range ceph.Health.Checks {
				detail := ""
				if len(check.Detail) > 0 {
					detail = check.Detail[0].Message
				}
				cephInfo.Checks[name] = CephHealthCheck{
					Severity: check.Severity,
					Summary:  check.Summary.Message,
					Detail:   detail,
				}
			}
		}
	}

	payload := StatePayload{
		Clusters:   clusters,
		Summary:    globalSummary.Total,
		Nodes:      nodeList,
		Guests:     guests,
		Storage:    storageList,
		CephStatus: cephInfo,
		Migrations: h.store.GetMigrations(),
		DRS:        h.store.GetAllDRSRecommendations(),
	}

	msg := WSMessage{
		Type:    "state",
		Payload: payload,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("failed to marshal state", "error", err)
		return nil
	}
	return data
}

// HandleWebSocket handles WebSocket upgrade requests
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// We don't expect client messages, just keep connection alive
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()

	for message := range c.send {
		err := c.conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			return
		}
	}
}
