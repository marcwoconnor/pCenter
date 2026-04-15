package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/alarms"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

// Hub manages WebSocket connections and broadcasts
type Hub struct {
	store    *state.Store
	alarms   *alarms.Service
	clients  map[*Client]bool
	mu       sync.RWMutex
	upgrader websocket.Upgrader

	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	done       chan struct{} // closed to signal Run() to stop
}

// SetAlarmsService sets the alarms service on the hub for state broadcasts
func (h *Hub) SetAlarmsService(s *alarms.Service) {
	h.alarms = s
}

// Client represents a WebSocket connection
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// NewHub creates a new WebSocket hub.
// allowedOrigins controls which cross-origin WebSocket connections are accepted.
// If empty, only same-origin connections are allowed (gorilla/websocket default).
// This prevents cross-site WebSocket hijacking (CSWSH) where a malicious page
// opens a WebSocket to pCenter and receives real-time cluster state.
func NewHub(store *state.Store, allowedOrigins []string) *Hub {
	originSet := make(map[string]bool)
	for _, o := range allowedOrigins {
		originSet[o] = true
	}

	return &Hub{
		store:   store,
		clients: make(map[*Client]bool),
		upgrader: websocket.Upgrader{
			// CheckOrigin validates the Origin header on WebSocket upgrade requests.
			// SECURITY: Browsers send Origin on cross-origin WS requests but do NOT
			// enforce the server's response (unlike CORS for HTTP). So the server MUST
			// reject unauthorized origins here. If no origins configured, we use
			// gorilla/websocket's default behavior: require Origin == Host (same-origin).
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true // No origin = same-origin or non-browser client
				}
				if len(originSet) == 0 {
					return false // No configured origins = reject all cross-origin
				}
				if !originSet[origin] {
					slog.Warn("websocket origin rejected", "origin", origin)
				}
				return originSet[origin]
			},
		},
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan struct{}),
	}
}

// Stop gracefully shuts down the hub, closing all client connections.
func (h *Hub) Stop() {
	close(h.done)
}

// Run starts the hub's main loop. Stops when Stop() is called.
func (h *Hub) Run() {
	for {
		select {
		case <-h.done:
			// Graceful shutdown: close all client connections
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			slog.Info("websocket hub stopped")
			return

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
			// Collect slow clients under read lock, then clean up under write lock.
			// Previously this did close()+delete() under RLock which is unsafe:
			// RLock allows concurrent readers, but delete() mutates the map and
			// close() can race with writePump sending on the channel.
			h.mu.RLock()
			var slow []*Client
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					slow = append(slow, client)
				}
			}
			h.mu.RUnlock()

			// Clean up slow clients under write lock
			if len(slow) > 0 {
				h.mu.Lock()
				for _, client := range slow {
					if _, ok := h.clients[client]; ok {
						delete(h.clients, client)
						close(client.send)
					}
				}
				h.mu.Unlock()
			}
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
	Migrations    []pve.MigrationProgress  `json:"migrations,omitempty"`
	DRS           []pve.DRSRecommendation  `json:"drs_recommendations,omitempty"`
	Alarms        []alarms.AlarmInstance   `json:"alarms,omitempty"`
	QDeviceStatus *pve.QDeviceStatus       `json:"qdevice_status,omitempty"`
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

	// Sort nodes by name for consistent ordering across broadcasts
	sort.Slice(nodeList, func(i, j int) bool {
		return nodeList[i].Node < nodeList[j].Node
	})

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
				// Join all detail messages (limit to 50 to avoid huge payloads)
				var details []string
				maxDetails := 50
				for i, d := range check.Detail {
					if i >= maxDetails {
						details = append(details, fmt.Sprintf("... and %d more", len(check.Detail)-maxDetails))
						break
					}
					details = append(details, d.Message)
				}
				cephInfo.Checks[name] = CephHealthCheck{
					Severity: check.Severity,
					Summary:  check.Summary.Message,
					Detail:   strings.Join(details, "\n"),
				}
			}
		}
	}

	// Ensure slices are never nil — Go encodes nil slices as JSON null,
	// which crashes the frontend if it calls .filter() on them
	if nodeList == nil {
		nodeList = []NodeWithStatus{}
	}
	if guests == nil {
		guests = []Guest{}
	}
	if storageList == nil {
		storageList = []StorageInfo{}
	}
	if clusters == nil {
		clusters = []ClusterInfo{}
	}

	migrations := h.store.GetMigrations()
	if migrations == nil {
		migrations = []pve.MigrationProgress{}
	}

	drs := h.store.GetAllDRSRecommendations()
	if drs == nil {
		drs = []pve.DRSRecommendation{}
	}

	// Get active alarms
	var activeAlarms []alarms.AlarmInstance
	if h.alarms != nil {
		activeAlarms, _ = h.alarms.GetActiveAlarms(context.Background())
	}
	if activeAlarms == nil {
		activeAlarms = []alarms.AlarmInstance{}
	}

	// Get qdevice status from first cluster that has it
	var qdeviceStatus *pve.QDeviceStatus
	for _, cs := range globalSummary.Clusters {
		if cluster, ok := h.store.GetCluster(cs.Name); ok {
			if qs := cluster.GetQDeviceStatus(); qs != nil && qs.Configured {
				qdeviceStatus = qs
				break
			}
		}
	}

	payload := StatePayload{
		Clusters:      clusters,
		Summary:       globalSummary.Total,
		Nodes:         nodeList,
		Guests:        guests,
		Storage:       storageList,
		CephStatus:    cephInfo,
		Migrations:    migrations,
		DRS:           drs,
		Alarms:        activeAlarms,
		QDeviceStatus: qdeviceStatus,
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
	conn, err := h.upgrader.Upgrade(w, r, nil)
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
