package agent

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // TODO: add token validation
	},
}

// Message types from agent protocol
const (
	MsgTypeRegister      = "register"
	MsgTypeHeartbeat     = "heartbeat"
	MsgTypeStatus        = "status"
	MsgTypeEvent         = "event"
	MsgTypeCommand       = "command"
	MsgTypeCommandResult = "command_result"
)

// Hub manages agent WebSocket connections
type Hub struct {
	store    *state.Store
	agents   map[string]*AgentConn // keyed by "cluster/node"
	commands *CommandTracker
	mu       sync.RWMutex
	onChange func() // callback when state changes
}

// AgentConn represents a connected agent
type AgentConn struct {
	hub       *Hub
	conn      *websocket.Conn
	node      string
	cluster   string
	version   string
	send      chan []byte
	lastSeen  time.Time
}

// NewHub creates a new agent hub
func NewHub(store *state.Store) *Hub {
	return &Hub{
		store:    store,
		agents:   make(map[string]*AgentConn),
		commands: NewCommandTracker(),
	}
}

// OnChange sets callback for state changes
func (h *Hub) OnChange(fn func()) {
	h.onChange = fn
}

// GetConnectedAgents returns list of connected agents
func (h *Hub) GetConnectedAgents() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	agents := make([]string, 0, len(h.agents))
	for key := range h.agents {
		agents = append(agents, key)
	}
	return agents
}

// HandleWebSocket handles agent WebSocket connections
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("agent websocket upgrade failed", "error", err)
		return
	}

	agent := &AgentConn{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 64),
		lastSeen: time.Now(),
	}

	go agent.readPump()
	go agent.writePump()
}

// readPump handles incoming messages from agent
func (a *AgentConn) readPump() {
	defer func() {
		a.hub.unregister(a)
		a.conn.Close()
	}()

	for {
		_, data, err := a.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Error("agent read error", "node", a.node, "error", err)
			}
			return
		}

		a.lastSeen = time.Now()
		a.handleMessage(data)
	}
}

// writePump handles outgoing messages to agent
func (a *AgentConn) writePump() {
	defer a.conn.Close()

	for message := range a.send {
		if err := a.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

// Message envelope
type Message struct {
	Type      string          `json:"type"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// RegisterData from agent
type RegisterData struct {
	Node       string `json:"node"`
	Cluster    string `json:"cluster"`
	Version    string `json:"version"`
	PVEVersion string `json:"pve_version,omitempty"`
}

// StatusData from agent
type StatusData struct {
	Node       string          `json:"node"`
	Cluster    string          `json:"cluster"`
	NodeStatus *NodeStatus     `json:"node_status"`
	VMs        []VMStatus      `json:"vms"`
	Containers []CTStatus      `json:"containers"`
	Storage    []StorageStatus `json:"storage,omitempty"`
	Ceph       *CephStatus     `json:"ceph,omitempty"`
	Metrics    *SystemMetrics  `json:"metrics,omitempty"`
}

type NodeStatus struct {
	Status     string   `json:"status"`
	CPU        float64  `json:"cpu"`
	MaxCPU     int      `json:"maxcpu"`
	Mem        int64    `json:"mem"`
	MaxMem     int64    `json:"maxmem"`
	Disk       int64    `json:"disk"`
	MaxDisk    int64    `json:"maxdisk"`
	Uptime     int64    `json:"uptime"`
	PVEVersion string   `json:"pveversion,omitempty"`
	KVersion   string   `json:"kversion,omitempty"`
	LoadAvg    []string `json:"loadavg,omitempty"`
}

type VMStatus struct {
	VMID      int     `json:"vmid"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	CPU       float64 `json:"cpu"`
	CPUs      int     `json:"cpus"`
	Mem       int64   `json:"mem"`
	MaxMem    int64   `json:"maxmem"`
	Disk      int64   `json:"disk"`
	MaxDisk   int64   `json:"maxdisk"`
	NetIn     int64   `json:"netin"`
	NetOut    int64   `json:"netout"`
	DiskRead  int64   `json:"diskread"`
	DiskWrite int64   `json:"diskwrite"`
	Uptime    int64   `json:"uptime"`
	Template  bool    `json:"template"`
}

type CTStatus struct {
	VMID      int     `json:"vmid"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	CPU       float64 `json:"cpu"`
	CPUs      int     `json:"cpus"`
	Mem       int64   `json:"mem"`
	MaxMem    int64   `json:"maxmem"`
	Disk      int64   `json:"disk"`
	MaxDisk   int64   `json:"maxdisk"`
	Swap      int64   `json:"swap"`
	MaxSwap   int64   `json:"maxswap"`
	NetIn     int64   `json:"netin"`
	NetOut    int64   `json:"netout"`
	DiskRead  int64   `json:"diskread"`
	DiskWrite int64   `json:"diskwrite"`
	Uptime    int64   `json:"uptime"`
	Template  bool    `json:"template"`
}

type StorageStatus struct {
	Storage string `json:"storage"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	Total   int64  `json:"total"`
	Used    int64  `json:"used"`
	Avail   int64  `json:"avail"`
	Shared  bool   `json:"shared"`
	Content string `json:"content"`
}

type CephStatus struct {
	Health       string        `json:"health"`
	HealthChecks []HealthCheck `json:"health_checks,omitempty"`
	PGMap        CephPGMap     `json:"pgmap"`
	OSDMap       CephOSDMap    `json:"osdmap"`
	MonMap       CephMonMap    `json:"monmap"`
}

type HealthCheck struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}

type CephPGMap struct {
	BytesTotal int64 `json:"bytes_total"`
	BytesUsed  int64 `json:"bytes_used"`
	BytesAvail int64 `json:"bytes_avail"`
}

type CephOSDMap struct {
	NumOSDs   int `json:"num_osds"`
	NumUpOSDs int `json:"num_up_osds"`
	NumInOSDs int `json:"num_in_osds"`
}

type CephMonMap struct {
	NumMons int `json:"num_mons"`
}

type SystemMetrics struct {
	PgpgIn     int64   `json:"pgpgin"`
	PgpgOut    int64   `json:"pgpgout"`
	PswpIn     int64   `json:"pswpin"`
	PswpOut    int64   `json:"pswpout"`
	PgFault    int64   `json:"pgfault"`
	PgMajFault int64   `json:"pgmajfault"`
	LoadAvg1   float64 `json:"loadavg_1m"`
	LoadAvg5   float64 `json:"loadavg_5m"`
	LoadAvg15  float64 `json:"loadavg_15m"`
}

func (a *AgentConn) handleMessage(data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("failed to parse agent message", "error", err)
		return
	}

	switch msg.Type {
	case MsgTypeRegister:
		a.handleRegister(msg.Data)
	case MsgTypeHeartbeat:
		// Just updates lastSeen
		slog.Debug("agent heartbeat", "node", a.node)
	case MsgTypeStatus:
		a.handleStatus(msg.Data)
	case MsgTypeCommandResult:
		a.handleCommandResult(msg.Data)
	default:
		slog.Warn("unknown message type", "type", msg.Type)
	}
}

func (a *AgentConn) handleRegister(data json.RawMessage) {
	var reg RegisterData
	if err := json.Unmarshal(data, &reg); err != nil {
		slog.Error("failed to parse register data", "error", err)
		return
	}

	a.node = reg.Node
	a.cluster = reg.Cluster
	a.version = reg.Version

	a.hub.register(a)

	slog.Info("agent registered", "node", a.node, "cluster", a.cluster, "version", a.version)
}

func (a *AgentConn) handleStatus(data json.RawMessage) {
	var status StatusData
	if err := json.Unmarshal(data, &status); err != nil {
		slog.Error("failed to parse status data", "error", err)
		return
	}

	// Update state store
	cs := a.hub.store.GetOrCreateCluster(status.Cluster)

	// Convert agent types to pve types and update store
	node := pve.Node{
		Node:    status.Node,
		Cluster: status.Cluster,
		Status:  "online",
	}

	if status.NodeStatus != nil {
		node.CPU = status.NodeStatus.CPU
		node.MaxCPU = status.NodeStatus.MaxCPU
		node.Mem = status.NodeStatus.Mem
		node.MaxMem = status.NodeStatus.MaxMem
		node.Disk = status.NodeStatus.Disk
		node.MaxDisk = status.NodeStatus.MaxDisk
		node.Uptime = status.NodeStatus.Uptime

		// Update node details
		cs.SetNodeDetails(status.Node, &pve.NodeStatus{
			PVEVersion: status.NodeStatus.PVEVersion,
			KernelVersion: status.NodeStatus.KVersion,
			LoadAvg: status.NodeStatus.LoadAvg,
		})
	}

	// Convert VMs
	vms := make([]pve.VM, len(status.VMs))
	for i, vm := range status.VMs {
		vms[i] = pve.VM{
			VMID:      vm.VMID,
			Name:      vm.Name,
			Node:      status.Node,
			Cluster:   status.Cluster,
			Status:    vm.Status,
			CPU:       vm.CPU,
			CPUs:      vm.CPUs,
			Mem:       vm.Mem,
			MaxMem:    vm.MaxMem,
			Disk:      vm.Disk,
			MaxDisk:   vm.MaxDisk,
			NetIn:     vm.NetIn,
			NetOut:    vm.NetOut,
			DiskRead:  vm.DiskRead,
			DiskWrite: vm.DiskWrite,
			Uptime:    vm.Uptime,
			Template:  vm.Template,
		}
	}

	// Convert containers
	cts := make([]pve.Container, len(status.Containers))
	for i, ct := range status.Containers {
		cts[i] = pve.Container{
			VMID:      ct.VMID,
			Name:      ct.Name,
			Node:      status.Node,
			Cluster:   status.Cluster,
			Status:    ct.Status,
			CPU:       ct.CPU,
			CPUs:      ct.CPUs,
			Mem:       ct.Mem,
			MaxMem:    ct.MaxMem,
			Disk:      ct.Disk,
			MaxDisk:   ct.MaxDisk,
			Swap:      ct.Swap,
			MaxSwap:   ct.MaxSwap,
			NetIn:     ct.NetIn,
			NetOut:    ct.NetOut,
			DiskRead:  ct.DiskRead,
			DiskWrite: ct.DiskWrite,
			Uptime:    ct.Uptime,
		}
	}

	// Convert storage
	storage := make([]pve.Storage, len(status.Storage))
	for i, s := range status.Storage {
		shared := 0
		if s.Shared {
			shared = 1
		}
		storage[i] = pve.Storage{
			Storage: s.Storage,
			Node:    status.Node,
			Cluster: status.Cluster,
			Type:    s.Type,
			Status:  s.Status,
			Total:   s.Total,
			Used:    s.Used,
			Avail:   s.Avail,
			Shared:  shared,
			Content: s.Content,
		}
	}

	// Convert Ceph if present
	var ceph *pve.CephStatus
	if status.Ceph != nil {
		ceph = &pve.CephStatus{}
		ceph.Health.Status = status.Ceph.Health
		ceph.PGMap.BytesTotal = status.Ceph.PGMap.BytesTotal
		ceph.PGMap.BytesUsed = status.Ceph.PGMap.BytesUsed
		ceph.PGMap.BytesAvail = status.Ceph.PGMap.BytesAvail
	}

	// Update the store
	cs.UpdateNode(status.Node, node, vms, cts, storage, ceph)

	slog.Debug("agent status received",
		"node", status.Node,
		"vms", len(vms),
		"containers", len(cts),
		"storage", len(storage))

	// Notify listeners
	if a.hub.onChange != nil {
		a.hub.onChange()
	}
}

func (h *Hub) register(a *AgentConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := a.cluster + "/" + a.node

	// Close existing connection if any
	if existing, ok := h.agents[key]; ok {
		close(existing.send)
	}

	h.agents[key] = a
	slog.Info("agent connected", "key", key, "total", len(h.agents))
}

func (h *Hub) unregister(a *AgentConn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if a.node == "" {
		return // Never registered
	}

	key := a.cluster + "/" + a.node
	if existing, ok := h.agents[key]; ok && existing == a {
		delete(h.agents, key)
		close(a.send)
		slog.Info("agent disconnected", "key", key, "total", len(h.agents))
	}
}
