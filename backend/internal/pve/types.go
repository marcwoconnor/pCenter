package pve

import "time"

// Node represents a Proxmox cluster node
type Node struct {
	Cluster        string  `json:"cluster,omitempty"` // populated by us
	Node           string  `json:"node"`
	Status         string  `json:"status"` // online, offline
	CPU            float64 `json:"cpu"`    // 0.0-1.0 usage
	MaxCPU         int     `json:"maxcpu"`
	Mem            int64   `json:"mem"`    // bytes used
	MaxMem         int64   `json:"maxmem"` // bytes total
	Disk           int64   `json:"disk"`
	MaxDisk        int64   `json:"maxdisk"`
	Uptime         int64   `json:"uptime"` // seconds
	SSLFingerprint string  `json:"ssl_fingerprint,omitempty"`
}

// VM represents a QEMU virtual machine
type VM struct {
	Cluster   string  `json:"cluster,omitempty"` // populated by us
	VMID      int     `json:"vmid"`
	Name      string  `json:"name"`
	Node      string  `json:"node,omitempty"` // populated by us
	Status    string  `json:"status"`         // running, stopped, paused
	CPU       float64 `json:"cpu"`
	CPUs      int     `json:"cpus"`
	Mem       int64   `json:"mem"`
	MaxMem    int64   `json:"maxmem"`
	Disk      int64   `json:"disk"`
	MaxDisk   int64   `json:"maxdisk"`
	Uptime    int64   `json:"uptime"`
	NetIn     int64   `json:"netin"`
	NetOut    int64   `json:"netout"`
	DiskRead  int64   `json:"diskread"`
	DiskWrite int64   `json:"diskwrite"`
	Template  bool    `json:"template"`
	Tags      string  `json:"tags,omitempty"`
	HAState   string  `json:"ha_state,omitempty"` // started, stopped, etc if HA managed
}

// Container represents an LXC container
type Container struct {
	Cluster string  `json:"cluster,omitempty"` // populated by us
	VMID    int     `json:"vmid"`
	Name    string  `json:"name"`
	Node    string  `json:"node,omitempty"` // populated by us
	Status  string  `json:"status"`         // running, stopped
	CPU     float64 `json:"cpu"`
	CPUs    int     `json:"cpus"`
	Mem     int64   `json:"mem"`
	MaxMem  int64   `json:"maxmem"`
	Swap    int64   `json:"swap"`
	MaxSwap int64   `json:"maxswap"`
	Disk    int64   `json:"disk"`
	MaxDisk int64   `json:"maxdisk"`
	Uptime  int64   `json:"uptime"`
	NetIn   int64   `json:"netin"`
	NetOut  int64   `json:"netout"`
	Type    string  `json:"type,omitempty"` // lxc
	Tags    string  `json:"tags,omitempty"`
	HAState string  `json:"ha_state,omitempty"` // started, stopped, etc if HA managed
}

// Storage represents a storage location
type Storage struct {
	Cluster string `json:"cluster,omitempty"` // populated by us
	Storage string `json:"storage"`
	Node    string `json:"node,omitempty"`
	Type    string `json:"type"`    // dir, lvm, zfspool, ceph, etc
	Status  string `json:"status"`  // available, unavailable
	Active  int    `json:"active"`  // 1 or 0
	Enabled int    `json:"enabled"` // 1 or 0
	Shared  int    `json:"shared"`  // 1 or 0
	Content string `json:"content"` // images,rootdir,vztmpl,iso,backup
	Used    int64  `json:"used"`
	Avail   int64  `json:"avail"`
	Total   int64  `json:"total"`
}

// ClusterResource is a unified resource from /cluster/resources
type ClusterResource struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"` // node, qemu, lxc, storage
	Node     string  `json:"node"`
	Status   string  `json:"status"`
	Name     string  `json:"name,omitempty"`
	VMID     int     `json:"vmid,omitempty"`
	CPU      float64 `json:"cpu,omitempty"`
	MaxCPU   int     `json:"maxcpu,omitempty"`
	Mem      int64   `json:"mem,omitempty"`
	MaxMem   int64   `json:"maxmem,omitempty"`
	Disk     int64   `json:"disk,omitempty"`
	MaxDisk  int64   `json:"maxdisk,omitempty"`
	Uptime   int64   `json:"uptime,omitempty"`
	Template int     `json:"template,omitempty"`
	Tags     string  `json:"tags,omitempty"`
}

// ClusterNode represents a node from /cluster/status
type ClusterNode struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	IP     string `json:"ip"`
	Online int    `json:"online"` // 1 or 0
	Local  int    `json:"local"`  // 1 if this is the node we queried
}

// Task represents a Proxmox task
type Task struct {
	UPID      string `json:"upid"`
	Node      string `json:"node"`
	PID       int    `json:"pid"`
	PStart    int64  `json:"pstart"`
	StartTime int64  `json:"starttime"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	User      string `json:"user"`
	Status    string `json:"status,omitempty"`    // running, stopped, OK, error
	ExitCode  string `json:"exitstatus,omitempty"`
}

// CephStatus represents Ceph cluster health
type CephStatus struct {
	Health struct {
		Status string `json:"status"` // HEALTH_OK, HEALTH_WARN, HEALTH_ERR
	} `json:"health"`
	PGMap struct {
		BytesUsed  int64 `json:"bytes_used"`
		BytesAvail int64 `json:"bytes_avail"`
		BytesTotal int64 `json:"bytes_total"`
	} `json:"pgmap"`
}

// HAStatus represents cluster HA manager status
type HAStatus struct {
	Enabled   bool              `json:"enabled"`
	Quorum    bool              `json:"quorum"`
	Manager   HAManagerStatus   `json:"manager"`
	Resources []HAResourceState `json:"resources"`
}

// HAManagerStatus is the HA manager node info
type HAManagerStatus struct {
	Node   string `json:"node"`
	Status string `json:"status"` // active, wait
}

// HAResourceState is the state of an HA-managed resource
type HAResourceState struct {
	SID    string `json:"sid"`    // vm:100 or ct:200
	Type   string `json:"type"`   // vm, ct
	Status string `json:"status"` // started, stopped, fence, error, etc
	Node   string `json:"node"`
	State  string `json:"state"` // enabled, disabled
}

// HAGroup is an HA failover group
type HAGroup struct {
	Group      string   `json:"group"`
	Comment    string   `json:"comment,omitempty"`
	Nodes      []string `json:"nodes"` // ordered by priority
	NoFailback bool     `json:"nofailback,omitempty"`
	Restricted bool     `json:"restricted,omitempty"`
}

// HAResource is an HA resource configuration
type HAResource struct {
	SID         string `json:"sid"` // vm:100 or ct:200
	Type        string `json:"type"`
	State       string `json:"state"` // started, stopped, disabled
	Group       string `json:"group,omitempty"`
	MaxRestart  int    `json:"max_restart,omitempty"`
	MaxRelocate int    `json:"max_relocate,omitempty"`
	Comment     string `json:"comment,omitempty"`
}

// MigrationProgress tracks an active migration
type MigrationProgress struct {
	UPID      string    `json:"upid"`
	Cluster   string    `json:"cluster"`
	VMID      int       `json:"vmid"`
	GuestName string    `json:"guest_name"`
	GuestType string    `json:"guest_type"` // vm, ct
	FromNode  string    `json:"from_node"`
	ToNode    string    `json:"to_node"`
	Online    bool      `json:"online"` // live migration
	StartedAt time.Time `json:"started_at"`
	Progress  int       `json:"progress"` // 0-100
	Status    string    `json:"status"`   // running, completed, failed
	Error     string    `json:"error,omitempty"`
}

// DRSRecommendation suggests a migration for load balancing
type DRSRecommendation struct {
	ID        string    `json:"id"`
	Cluster   string    `json:"cluster"`
	GuestType string    `json:"guest_type"` // vm, ct
	VMID      int       `json:"vmid"`
	GuestName string    `json:"guest_name"`
	FromNode  string    `json:"from_node"`
	ToNode    string    `json:"to_node"`
	Reason    string    `json:"reason"`
	Priority  int       `json:"priority"` // 1-5
	CreatedAt time.Time `json:"created_at"`
}

// APIResponse wraps all Proxmox API responses
type APIResponse[T any] struct {
	Data   T       `json:"data"`
	Errors *string `json:"errors,omitempty"`
}

// ClusterState holds aggregated state from all nodes in a cluster
type ClusterState struct {
	UpdatedAt  time.Time
	Nodes      []Node
	VMs        []VM
	Containers []Container
	Storage    []Storage
}
