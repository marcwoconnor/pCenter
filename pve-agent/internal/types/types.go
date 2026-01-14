package types

// Message types for agent <-> pCenter communication
const (
	MsgTypeRegister      = "register"
	MsgTypeHeartbeat     = "heartbeat"
	MsgTypeStatus        = "status"
	MsgTypeEvent         = "event"
	MsgTypeCommand       = "command"
	MsgTypeCommandResult = "command_result"
)

// Message is the envelope for all WebSocket messages
type Message struct {
	Type      string      `json:"type"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// RegisterData is sent when agent first connects
type RegisterData struct {
	Node       string `json:"node"`
	Cluster    string `json:"cluster"`
	Version    string `json:"version"`
	PVEVersion string `json:"pve_version,omitempty"`
}

// HeartbeatData is sent periodically to maintain connection
type HeartbeatData struct {
	Node   string `json:"node"`
	Uptime int64  `json:"uptime"`
}

// StatusData contains the full node state
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

// NodeStatus contains node-level information
type NodeStatus struct {
	Status     string  `json:"status"` // online, offline
	CPU        float64 `json:"cpu"`
	MaxCPU     int     `json:"maxcpu"`
	Mem        int64   `json:"mem"`
	MaxMem     int64   `json:"maxmem"`
	Disk       int64   `json:"disk"`
	MaxDisk    int64   `json:"maxdisk"`
	Uptime     int64   `json:"uptime"`
	PVEVersion string  `json:"pveversion,omitempty"`
	KVersion   string  `json:"kversion,omitempty"`
	LoadAvg    []string `json:"loadavg,omitempty"`
}

// VMStatus contains VM information
type VMStatus struct {
	VMID      int     `json:"vmid"`
	Name      string  `json:"name"`
	Status    string  `json:"status"` // running, stopped, paused
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
	HAState   string  `json:"ha_state,omitempty"`
}

// CTStatus contains container information
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
	HAState   string  `json:"ha_state,omitempty"`
}

// StorageStatus contains storage information
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

// CephStatus contains Ceph cluster status
type CephStatus struct {
	Health       string         `json:"health"`
	HealthChecks []HealthCheck  `json:"health_checks,omitempty"`
	PGMap        CephPGMap      `json:"pgmap"`
	OSDMap       CephOSDMap     `json:"osdmap"`
	MonMap       CephMonMap     `json:"monmap"`
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

// SystemMetrics contains /proc-based metrics
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

// EventData is sent when something changes
type EventData struct {
	Node      string      `json:"node"`
	EventType string      `json:"event_type"` // vm_started, vm_stopped, etc.
	Resource  string      `json:"resource"`   // vm:100, ct:200
	Details   interface{} `json:"details,omitempty"`
}

// CommandData is sent from pCenter to agent
type CommandData struct {
	ID     string                 `json:"id"`
	Action string                 `json:"action"` // vm_start, vm_stop, etc.
	Params map[string]interface{} `json:"params"`
}

// CommandResultData is the response to a command
type CommandResultData struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	UPID    string `json:"upid,omitempty"`
	Output  string `json:"output,omitempty"` // For ceph commands
	Error   string `json:"error,omitempty"`
}
