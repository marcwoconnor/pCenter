package pve

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

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

// NodeStatus contains detailed node status from /nodes/{node}/status
type NodeStatus struct {
	PVEVersion    string   `json:"pveversion"`
	KernelVersion string   `json:"kversion"`
	CPUModel      string   `json:"cpu_model"`
	CPUCores      int      `json:"cpu_cores"`
	CPUSockets    int      `json:"cpu_sockets"`
	BootMode      string   `json:"boot_mode"`
	LoadAvg       []string `json:"loadavg"`
}

// NodeStatusResponse is the raw response from Proxmox /nodes/{node}/status
type NodeStatusResponse struct {
	PVEVersion string `json:"pveversion"`
	KVersion   string `json:"kversion"`
	CPUInfo    struct {
		Model   string `json:"model"`
		Cores   int    `json:"cores"`
		Sockets int    `json:"sockets"`
	} `json:"cpuinfo"`
	BootInfo struct {
		Mode string `json:"mode"`
	} `json:"boot-info"`
	LoadAvg []string `json:"loadavg"`
}

// GuestNIC represents a network interface on a VM/CT
type GuestNIC struct {
	Name   string `json:"name"`             // net0, net1, etc.
	Bridge string `json:"bridge"`           // vmbr0, vmbr1, etc.
	MAC    string `json:"mac,omitempty"`    // MAC address
	Model  string `json:"model,omitempty"`  // virtio, e1000, etc. (VMs only)
	Tag    int    `json:"tag,omitempty"`    // VLAN tag
}

// VM represents a QEMU virtual machine
type VM struct {
	Cluster   string     `json:"cluster,omitempty"` // populated by us
	VMID      int        `json:"vmid"`
	Name      string     `json:"name"`
	Node      string     `json:"node,omitempty"` // populated by us
	Status    string     `json:"status"`         // running, stopped, paused
	CPU       float64    `json:"cpu"`
	CPUs      int        `json:"cpus"`
	Mem       int64      `json:"mem"`
	MaxMem    int64      `json:"maxmem"`
	Disk      int64      `json:"disk"`
	MaxDisk   int64      `json:"maxdisk"`
	Uptime    int64      `json:"uptime"`
	NetIn     int64      `json:"netin"`
	NetOut    int64      `json:"netout"`
	DiskRead  int64      `json:"diskread"`
	DiskWrite int64      `json:"diskwrite"`
	Template  bool       `json:"template"`
	Tags      string     `json:"tags,omitempty"`
	HAState   string     `json:"ha_state,omitempty"` // started, stopped, etc if HA managed
	NICs      []GuestNIC `json:"nics,omitempty"`     // network interfaces
}

// Container represents an LXC container
type Container struct {
	Cluster   string     `json:"cluster,omitempty"` // populated by us
	VMID      int        `json:"vmid"`
	Name      string     `json:"name"`
	Node      string     `json:"node,omitempty"` // populated by us
	Status    string     `json:"status"`         // running, stopped
	CPU       float64    `json:"cpu"`
	CPUs      int        `json:"cpus"`
	Mem       int64      `json:"mem"`
	MaxMem    int64      `json:"maxmem"`
	Swap      int64      `json:"swap"`
	MaxSwap   int64      `json:"maxswap"`
	Disk      int64      `json:"disk"`
	MaxDisk   int64      `json:"maxdisk"`
	Uptime    int64      `json:"uptime"`
	NetIn     int64      `json:"netin"`
	NetOut    int64      `json:"netout"`
	DiskRead  int64      `json:"diskread"`
	DiskWrite int64      `json:"diskwrite"`
	Type      string     `json:"type,omitempty"` // lxc
	Template  bool       `json:"template"`
	Tags      string     `json:"tags,omitempty"`
	HAState   string     `json:"ha_state,omitempty"` // started, stopped, etc if HA managed
	NICs      []GuestNIC `json:"nics,omitempty"`     // network interfaces
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

// StorageVolume represents a volume on a storage (disk image, ISO, etc)
type StorageVolume struct {
	Volid   string      `json:"volid"`             // e.g., "local-lvm:vm-100-disk-0"
	Format  string      `json:"format"`            // raw, qcow2, subvol, etc
	Size    int64       `json:"size"`              // size in bytes
	Used    int64       `json:"used,omitempty"`    // used space (for thin)
	VMID    int         `json:"vmid,omitempty"`    // VM ID if this is a VM disk
	Content string      `json:"content"`           // images, rootdir, iso, vztmpl, backup
	Ctime   interface{} `json:"ctime,omitempty"`   // creation time (can be int64 or string)
	Parent  string      `json:"parent,omitempty"`  // parent snapshot
	Notes   string      `json:"notes,omitempty"`   // description/notes
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

// CephHealthCheckDetail is a single detail message within a health check
type CephHealthCheckDetail struct {
	Message string `json:"message"`
}

// CephHealthCheckSummary is the summary of a health check
type CephHealthCheckSummary struct {
	Count   int    `json:"count"`
	Message string `json:"message"`
}

// CephHealthCheck represents a single Ceph health check
type CephHealthCheck struct {
	Severity string                  `json:"severity"` // HEALTH_OK, HEALTH_WARN, HEALTH_ERR
	Summary  CephHealthCheckSummary  `json:"summary"`
	Detail   []CephHealthCheckDetail `json:"detail"`
	Muted    bool                    `json:"muted"`
}

// CephStatus represents Ceph cluster health
type CephStatus struct {
	Health struct {
		Status string                     `json:"status"` // HEALTH_OK, HEALTH_WARN, HEALTH_ERR
		Checks map[string]CephHealthCheck `json:"checks,omitempty"`
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

// DiskMoveProgress tracks an active storage vMotion (per-disk / per-volume
// move between storage pools). Used for both qemu `move_disk` and lxc
// `move_volume` — GuestType distinguishes them. Node stays the same across
// the operation; what changes is the backing storage of one disk/volume.
type DiskMoveProgress struct {
	UPID         string    `json:"upid"`
	Cluster      string    `json:"cluster"`
	VMID         int       `json:"vmid"`
	GuestName    string    `json:"guest_name"`
	GuestType    string    `json:"guest_type"` // vm, ct
	Node         string    `json:"node"`
	Disk         string    `json:"disk"`           // e.g. scsi0 (VM) or rootfs/mp0 (CT)
	FromStorage  string    `json:"from_storage"`   // resolved at initiation (for display)
	ToStorage    string    `json:"to_storage"`
	DeleteSource bool      `json:"delete_source"`
	StartedAt    time.Time `json:"started_at"`
	Progress     int       `json:"progress"` // 0-100
	Status       string    `json:"status"`   // running, completed, failed
	Error        string    `json:"error,omitempty"`
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

// NetworkInterface represents a node's network interface
type NetworkInterface struct {
	Cluster       string `json:"cluster,omitempty"`       // populated by us
	Node          string `json:"node,omitempty"`          // populated by us
	Iface         string `json:"iface"`                   // interface name (eth0, vmbr0, etc)
	Type          string `json:"type"`                    // bridge, bond, eth, vlan, OVSBridge, etc
	Active        int    `json:"active"`                  // 1 or 0
	Autostart     int    `json:"autostart"`               // 1 or 0
	Method        string `json:"method,omitempty"`        // static, dhcp, manual
	Method6       string `json:"method6,omitempty"`       // IPv6 method
	Address       string `json:"address,omitempty"`       // IPv4 address
	Netmask       string `json:"netmask,omitempty"`       // IPv4 netmask
	Gateway       string `json:"gateway,omitempty"`       // IPv4 gateway
	CIDR          string `json:"cidr,omitempty"`          // CIDR notation
	Address6      string `json:"address6,omitempty"`      // IPv6 address
	Netmask6      string `json:"netmask6,omitempty"`      // IPv6 prefix
	Gateway6      string `json:"gateway6,omitempty"`      // IPv6 gateway
	BridgePorts   string `json:"bridge_ports,omitempty"`  // bridge members
	BridgeSTP     string `json:"bridge_stp,omitempty"`    // spanning tree
	BridgeFD      string `json:"bridge_fd,omitempty"`     // forward delay
	BridgeVlanAware int  `json:"bridge_vlan_aware,omitempty"` // VLAN aware bridge
	BondSlaves    string `json:"slaves,omitempty"`        // bond members
	BondMode      string `json:"bond_mode,omitempty"`     // bonding mode
	BondPrimary   string `json:"bond-primary,omitempty"`  // primary interface
	VLANRawDevice string      `json:"vlan-raw-device,omitempty"` // parent for VLAN
	VLANID        interface{} `json:"vlan-id,omitempty"`         // VLAN tag (can be string or int)
	MTU           int    `json:"mtu,omitempty"`           // MTU size
	Comments      string `json:"comments,omitempty"`      // description
}

// SDNZone represents an SDN zone (cluster-wide)
type SDNZone struct {
	Cluster      string `json:"cluster,omitempty"` // populated by us
	Zone         string `json:"zone"`              // zone identifier
	Type         string `json:"type"`              // simple, vlan, qinq, vxlan, evpn
	State        string `json:"state,omitempty"`   // active, pending
	Pending      int    `json:"pending,omitempty"` // has pending changes
	Nodes        string `json:"nodes,omitempty"`   // restrict to nodes
	IPAM         string `json:"ipam,omitempty"`    // IPAM plugin
	DNS          string `json:"dns,omitempty"`     // DNS plugin
	ReverseDNS   string `json:"reversedns,omitempty"` // reverse DNS zone
	DNSZone      string `json:"dnszone,omitempty"` // DNS zone name
	Bridge       string `json:"bridge,omitempty"`  // bridge for simple/vlan zones
	Tag          int    `json:"tag,omitempty"`     // default VLAN tag
	VLANProtocol string `json:"vlan-protocol,omitempty"` // 802.1q or 802.1ad
	MTU          int    `json:"mtu,omitempty"`     // MTU size
	Peers        string `json:"peers,omitempty"`   // peer list for vxlan/evpn
}

// SDNVNet represents a virtual network within an SDN zone
type SDNVNet struct {
	Cluster   string `json:"cluster,omitempty"` // populated by us
	VNet      string `json:"vnet"`              // vnet identifier
	Zone      string `json:"zone"`              // parent zone
	Type      string `json:"type,omitempty"`    // vnet type
	State     string `json:"state,omitempty"`   // active, pending
	Pending   int    `json:"pending,omitempty"` // has pending changes
	Alias     string `json:"alias,omitempty"`   // display name
	Tag       int    `json:"tag,omitempty"`     // VLAN/VXLAN tag
	VLANAware int    `json:"vlanaware,omitempty"` // VLAN aware
}

// SDNSubnet represents a subnet within an SDN vnet
type SDNSubnet struct {
	Cluster       string `json:"cluster,omitempty"` // populated by us
	Subnet        string `json:"subnet"`            // CIDR notation
	VNet          string `json:"vnet"`              // parent vnet
	Zone          string `json:"zone,omitempty"`    // parent zone (from vnet)
	Type          string `json:"type,omitempty"`    // subnet type
	State         string `json:"state,omitempty"`   // active, pending
	Gateway       string `json:"gateway,omitempty"` // gateway IP
	SNAT          int    `json:"snat,omitempty"`    // enable SNAT
	DNSZonePrefix string `json:"dnszoneprefix,omitempty"` // DNS prefix
}

// SDNController represents an SDN controller (for EVPN)
type SDNController struct {
	Cluster    string `json:"cluster,omitempty"` // populated by us
	Controller string `json:"controller"`        // controller identifier
	Type       string `json:"type"`              // evpn, faucet, etc
	State      string `json:"state,omitempty"`   // active, pending
	Pending    int    `json:"pending,omitempty"` // has pending changes
	ASN        int    `json:"asn,omitempty"`     // BGP ASN
	Peers      string `json:"peers,omitempty"`   // BGP peers
}

// SmartDisk represents a disk with SMART data
type SmartDisk struct {
	Node         string           `json:"node"`
	Cluster      string           `json:"cluster,omitempty"`
	Device       string           `json:"device"`       // /dev/sda
	Model        string           `json:"model"`        // drive model
	Serial       string           `json:"serial"`       // serial number
	Capacity     int64            `json:"capacity"`     // bytes
	Type         string           `json:"type"`         // hdd, ssd, nvme
	Protocol     string           `json:"protocol"`     // ATA, NVMe
	Health       string           `json:"health"`       // PASSED, FAILED, UNKNOWN
	PowerOnHours int64            `json:"power_on_hours"`
	Temperature  int              `json:"temperature"`  // Celsius
	Attributes   []SmartAttribute `json:"attributes,omitempty"`   // HDD/SSD SMART attrs
	NVMeHealth   *NVMeHealth      `json:"nvme_health,omitempty"`  // NVMe specific
}

// SmartAttribute is a single SMART attribute (for HDD/SSD)
type SmartAttribute struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Value      int    `json:"value"`
	Worst      int    `json:"worst"`
	Threshold  int    `json:"threshold"`
	Raw        int64  `json:"raw"`
	Flags      string `json:"flags"`
	WhenFailed string `json:"when_failed,omitempty"`
	Critical   bool   `json:"critical"` // highlighted as critical for health
}

// NVMeHealth contains NVMe-specific health data
type NVMeHealth struct {
	CriticalWarning     int   `json:"critical_warning"`
	AvailableSpare      int   `json:"available_spare"`       // percent
	AvailableSpareThresh int  `json:"available_spare_thresh"` // percent
	PercentUsed         int   `json:"percent_used"`          // wear level
	DataUnitsRead       int64 `json:"data_units_read"`
	DataUnitsWritten    int64 `json:"data_units_written"`
	PowerCycles         int64 `json:"power_cycles"`
	UnsafeShutdowns     int64 `json:"unsafe_shutdowns"`
	MediaErrors         int64 `json:"media_errors"`
	ErrorLogEntries     int64 `json:"error_log_entries"`
}

// QDeviceStatus represents the Proxmox cluster qdevice status
type QDeviceStatus struct {
	Configured   bool   `json:"configured"`
	Connected    bool   `json:"connected"`
	HostNode     string `json:"host_node"`      // Node where qdevice VM runs
	HostVMID     int    `json:"host_vmid"`      // VMID of qdevice VM
	HostVMName   string `json:"host_vm_name"`   // Name of qdevice VM
	QNetdAddress string `json:"qnetd_address"`  // IP:port of qnetd server
	Algorithm    string `json:"algorithm"`      // e.g., "Fifty-Fifty split"
	State        string `json:"state"`          // Connected, Disconnected, etc.
}

// MaintenancePreflightCheck represents a single pre-flight check result
type MaintenancePreflightCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`   // ok, warning, error
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"` // If true, blocks maintenance mode
}

// MaintenancePreflight represents the full pre-flight check results
type MaintenancePreflight struct {
	Node           string                      `json:"node"`
	CanEnter       bool                        `json:"can_enter"`
	Checks         []MaintenancePreflightCheck `json:"checks"`
	GuestsToMove   []GuestToMove               `json:"guests_to_move"`
	CriticalGuests []GuestToMove               `json:"critical_guests"` // osd-mon01, etc.
}

// GuestToMove represents a VM/CT that needs to be migrated
type GuestToMove struct {
	VMID       int    `json:"vmid"`
	Name       string `json:"name"`
	Type       string `json:"type"`        // qemu, lxc
	Status     string `json:"status"`      // running, stopped
	TargetNode string `json:"target_node"`
	IsCritical bool   `json:"is_critical"` // qdevice VM, etc.
	Reason     string `json:"reason,omitempty"`
}

// MaintenanceState tracks a node's maintenance status
type MaintenanceState struct {
	Node          string    `json:"node"`
	InMaintenance bool      `json:"in_maintenance"`
	EnteredAt     time.Time `json:"entered_at,omitempty"`
	Phase         string    `json:"phase,omitempty"` // preflight, evacuating, ready, exiting
	Progress      int       `json:"progress"`        // 0-100
	Message       string    `json:"message,omitempty"`
}

// EvacuationStatus tracks the evacuation progress
type EvacuationStatus struct {
	Node            string        `json:"node"`
	TotalGuests     int           `json:"total_guests"`
	MigratedGuests  int           `json:"migrated_guests"`
	CurrentGuest    string        `json:"current_guest,omitempty"`
	CurrentProgress int           `json:"current_progress"`
	Errors          []string      `json:"errors,omitempty"`
	Guests          []GuestToMove `json:"guests"`
}

// VMConfig represents full VM configuration from Proxmox /nodes/{node}/qemu/{vmid}/config
type VMConfig struct {
	Digest      string `json:"digest"`                // For optimistic locking
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	// Hardware
	Cores   int     `json:"cores,omitempty"`
	Sockets int     `json:"sockets,omitempty"`
	CPU     string  `json:"cpu,omitempty"`     // CPU type (host, kvm64, etc)
	Memory  int     `json:"memory,omitempty"`  // MB
	Balloon int     `json:"balloon,omitempty"` // Memory ballooning (MB), 0 to disable
	Numa    int     `json:"numa,omitempty"`    // Enable NUMA
	BIOS    string  `json:"bios,omitempty"`    // seabios, ovmf
	Machine string  `json:"machine,omitempty"` // q35, i440fx

	// Boot
	Boot     string `json:"boot,omitempty"`     // Boot order (order=scsi0;ide2;net0)
	Bootdisk string `json:"bootdisk,omitempty"` // Default boot disk

	// Options
	Onboot     int    `json:"onboot,omitempty"`     // Start at boot (0 or 1)
	Protection int    `json:"protection,omitempty"` // Prevent deletion (0 or 1)
	Agent      string `json:"agent,omitempty"`      // QEMU guest agent (enabled=1)
	Ostype     string `json:"ostype,omitempty"`     // OS type (l26, win10, etc)

	// Cloud-init
	CIUser       string `json:"ciuser,omitempty"`
	CIPassword   string `json:"cipassword,omitempty"` // Will be hidden/masked
	SSHKeys      string `json:"sshkeys,omitempty"`    // URL-encoded
	IPConfig0    string `json:"ipconfig0,omitempty"`
	IPConfig1    string `json:"ipconfig1,omitempty"`
	Nameserver   string `json:"nameserver,omitempty"`
	Searchdomain string `json:"searchdomain,omitempty"`

	// VGA
	VGA string `json:"vga,omitempty"` // std, cirrus, vmware, qxl, serial0, virtio

	// Storage - dynamic fields stored in RawConfig
	// Network - dynamic fields stored in RawConfig

	// All raw config data (for dynamic fields like scsi0, net0, etc)
	RawConfig map[string]interface{} `json:"raw_config,omitempty"`
}

// ContainerConfig represents full LXC container configuration from Proxmox
type ContainerConfig struct {
	Digest      string `json:"digest"`                // For optimistic locking
	Hostname    string `json:"hostname,omitempty"`
	Description string `json:"description,omitempty"`

	// Resources
	Cores    int     `json:"cores,omitempty"`    // Number of cores
	CPULimit float64 `json:"cpulimit,omitempty"` // CPU limit (0-128)
	CPUUnits int     `json:"cpuunits,omitempty"` // CPU weight (0-500000)
	Memory   int     `json:"memory,omitempty"`   // MB
	Swap     int     `json:"swap,omitempty"`     // MB

	// Root filesystem
	Rootfs string `json:"rootfs,omitempty"` // storage:size format

	// Options
	Onboot       int    `json:"onboot,omitempty"`       // Start at boot
	Protection   int    `json:"protection,omitempty"`   // Prevent deletion
	Unprivileged int    `json:"unprivileged,omitempty"` // Unprivileged container
	Ostype       string `json:"ostype,omitempty"`       // debian, ubuntu, centos, etc
	Arch         string `json:"arch,omitempty"`         // amd64, i386, arm64, armhf

	// Features
	Features string `json:"features,omitempty"` // nesting=1,keyctl=1,fuse=1

	// Startup/Shutdown
	Startup  string `json:"startup,omitempty"`  // Startup order

	// Network - stored as net0, net1, etc in RawConfig
	// Mount points - stored as mp0, mp1, etc in RawConfig

	// All raw config data
	RawConfig map[string]interface{} `json:"raw_config,omitempty"`
}

// ConfigUpdateRequest for applying configuration changes
type ConfigUpdateRequest struct {
	Digest  string                 `json:"digest"`           // Required for optimistic locking
	Changes map[string]interface{} `json:"changes"`          // Key-value pairs to update
	Delete  []string               `json:"delete,omitempty"` // Keys to delete
}

// ConfigResponse wraps config with metadata
type ConfigResponse struct {
	Config interface{} `json:"config"`
	Digest string      `json:"digest"`
	Node   string      `json:"node"`
	VMID   int         `json:"vmid"`
}

// Snapshot represents a VM or container snapshot
type Snapshot struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SnapTime    int64  `json:"snaptime,omitempty"`  // Unix timestamp
	VMState     int    `json:"vmstate,omitempty"`   // 1 if includes RAM state (VM only)
	Parent      string `json:"parent,omitempty"`    // Parent snapshot name
}

// FirewallRule represents a firewall rule
type FirewallRule struct {
	Pos     int    `json:"pos,omitempty"`     // Position in rule list
	Type    string `json:"type"`              // in, out, group
	Action  string `json:"action"`            // ACCEPT, DROP, REJECT
	Enable  int    `json:"enable,omitempty"`  // 0 or 1
	Source  string `json:"source,omitempty"`
	Dest    string `json:"dest,omitempty"`
	Sport   string `json:"sport,omitempty"`   // Source port
	Dport   string `json:"dport,omitempty"`   // Destination port
	Proto   string `json:"proto,omitempty"`   // tcp, udp, icmp, etc
	Macro   string `json:"macro,omitempty"`   // Predefined macro (SSH, HTTP, etc)
	IFace   string `json:"iface,omitempty"`   // Interface
	Log     string `json:"log,omitempty"`     // Log level
	Comment string `json:"comment,omitempty"`
}

// FirewallOptions represents firewall options for a guest
type FirewallOptions struct {
	Enable       int    `json:"enable,omitempty"`       // Enable firewall
	DHCPv4       int    `json:"dhcp,omitempty"`         // Allow DHCP
	DHCPv6       int    `json:"dhcp6,omitempty"`        // Allow DHCPv6
	IPFilter     int    `json:"ipfilter,omitempty"`     // IP filter
	LogLevelIn   string `json:"log_level_in,omitempty"` // Log level for incoming
	LogLevelOut  string `json:"log_level_out,omitempty"`// Log level for outgoing
	MACFilter    int    `json:"macfilter,omitempty"`    // MAC filter
	NDP          int    `json:"ndp,omitempty"`          // Allow NDP
	PolicyIn     string `json:"policy_in,omitempty"`    // Default incoming policy
	PolicyOut    string `json:"policy_out,omitempty"`   // Default outgoing policy
	RadV         int    `json:"radv,omitempty"`         // Allow router advertisements
}

// CreateVMRequest for creating a new QEMU virtual machine
type CreateVMRequest struct {
	VMID     int    `json:"vmid"`
	Name     string `json:"name"`
	Cores    int    `json:"cores"`
	Memory   int    `json:"memory"`              // MB
	Storage  string `json:"storage"`             // e.g., "local-lvm"
	DiskSize int    `json:"disk_size"`           // GB
	ISO      string `json:"iso,omitempty"`       // e.g., "local:iso/ubuntu.iso"
	OSType   string `json:"ostype,omitempty"`    // l26, win10, etc.
	Network  string `json:"network,omitempty"`   // bridge name (e.g., vmbr0)
	Start    bool   `json:"start,omitempty"`
}

// CreateContainerRequest for creating a new LXC container
type CreateContainerRequest struct {
	VMID         int    `json:"vmid"`
	Hostname     string `json:"hostname"`
	Template     string `json:"ostemplate"`           // REQUIRED: e.g., "local:vztmpl/ubuntu.tar.gz"
	Cores        int    `json:"cores"`
	Memory       int    `json:"memory"`               // MB
	Swap         int    `json:"swap"`                 // MB
	Storage      string `json:"storage"`              // root storage
	DiskSize     int    `json:"disk_size"`            // GB
	Network      string `json:"network,omitempty"`    // bridge name
	Password     string `json:"password,omitempty"`
	SSHKeys      string `json:"ssh_public_keys,omitempty"`
	Start        bool   `json:"start,omitempty"`
	Unprivileged bool   `json:"unprivileged"`
}

// NodeDNS represents DNS configuration from /nodes/{node}/dns
type NodeDNS struct {
	Search string `json:"search"` // search domain
	DNS1   string `json:"dns1"`
	DNS2   string `json:"dns2,omitempty"`
	DNS3   string `json:"dns3,omitempty"`
}

// NodeTime represents time/timezone from /nodes/{node}/time
type NodeTime struct {
	Timezone  string `json:"timezone"`
	Localtime int64  `json:"localtime"`
	UTCTime   int64  `json:"time"` // UTC epoch
}

// NodeHosts represents /etc/hosts content from /nodes/{node}/hosts
type NodeHosts struct {
	Data   string `json:"data"`   // raw /etc/hosts content
	Digest string `json:"digest"` // config digest
}

// NodeSubscription represents subscription info from /nodes/{node}/subscription
type NodeSubscription struct {
	Status     string `json:"status"`               // active, notfound, new, invalid
	ServerID   string `json:"serverid,omitempty"`
	ProductName string `json:"productname,omitempty"`
	Level      string `json:"level,omitempty"`       // community, basic, standard, premium
	NextDue    string `json:"nextduedate,omitempty"`
}

// APTRepository represents a single APT repository entry
type APTRepository struct {
	Path      string   `json:"Path"`
	Index     int      `json:"Number"`
	FileType  string   `json:"FileType"`
	Enabled   bool     `json:"Enabled"`
	Types     []string `json:"Types"`
	URIs      []string `json:"URIs"`
	Suites    []string `json:"Suites"`
	Components []string `json:"Components"`
	Comment   string   `json:"Comment,omitempty"`
}

// APTRepositoryFile represents a file containing APT repos
type APTRepositoryFile struct {
	Path         string          `json:"path"`
	FileType     string          `json:"file-type"`
	Repositories []APTRepository `json:"repositories"`
}

// APTRepositoryInfo is the response from /nodes/{node}/apt/repositories
type APTRepositoryInfo struct {
	Files   []APTRepositoryFile `json:"files"`
	Digest  string              `json:"digest"`
}

// NodeConfig is the combined host-level configuration
type NodeConfig struct {
	DNS          *NodeDNS            `json:"dns"`
	Time         *NodeTime           `json:"time"`
	Hosts        string              `json:"hosts"`         // raw /etc/hosts
	Network      []NetworkInterface  `json:"network"`
	Subscription *NodeSubscription   `json:"subscription"`
	APTRepos     *APTRepositoryInfo  `json:"apt_repos"`
	Status       *NodeStatus         `json:"status"`        // PVE version, kernel, CPU info
}

// NodeCertificate represents a certificate installed on a node.
// From GET /nodes/{node}/certificates/info.
// Fields below `PEM` are populated by pCenter-side parsing (not PVE).
type NodeCertificate struct {
	Filename      string   `json:"filename"`
	Fingerprint   string   `json:"fingerprint,omitempty"`
	Issuer        string   `json:"issuer,omitempty"`
	NotAfter      int64    `json:"notafter,omitempty"`  // Unix seconds
	NotBefore     int64    `json:"notbefore,omitempty"` // Unix seconds
	PublicKeyBits int      `json:"public_key_bits,omitempty"`
	PublicKeyType string   `json:"public_key_type,omitempty"`
	SAN           []string `json:"san,omitempty"`
	Subject       string   `json:"subject,omitempty"`
	PEM           string   `json:"pem,omitempty"`

	// Populated server-side by parsing `PEM` — optional on the wire.
	Serial             string   `json:"serial,omitempty"`
	SignatureAlgorithm string   `json:"signature_algorithm,omitempty"`
	KeyUsage           []string `json:"key_usage,omitempty"`
	ExtendedKeyUsage   []string `json:"extended_key_usage,omitempty"`
	IsCA               bool     `json:"is_ca,omitempty"`
	IsSelfSigned       bool     `json:"is_self_signed,omitempty"`
}

// ACMEAccount is an ACME account registered at the cluster level.
// From GET /cluster/acme/account
type ACMEAccount struct {
	Name      string `json:"name"`
	Directory string `json:"directory,omitempty"`
	TOSURL    string `json:"tos,omitempty"`
}

// ACMEPlugin is an ACME DNS/HTTP challenge plugin configured at the cluster level.
// From GET /cluster/acme/plugins
type ACMEPlugin struct {
	Plugin          string            `json:"plugin"` // id
	Type            string            `json:"type"`   // "dns" or "standalone"
	API             string            `json:"api,omitempty"`
	Disable         int               `json:"disable,omitempty"` // 1 if disabled
	Data            map[string]string `json:"data,omitempty"`    // provider-specific fields (parsed from PVE's \n-separated string form)
	Digest          string            `json:"digest,omitempty"`
	ValidationDelay int               `json:"validation-delay,omitempty"`
}

// UnmarshalJSON parses ACMEPlugin responses from Proxmox, where the `data`
// field is a newline-separated key=value string (not a JSON object).
func (p *ACMEPlugin) UnmarshalJSON(b []byte) error {
	var raw struct {
		Plugin          string          `json:"plugin"`
		Type            string          `json:"type"`
		API             string          `json:"api,omitempty"`
		Disable         int             `json:"disable,omitempty"`
		Data            json.RawMessage `json:"data,omitempty"`
		Digest          string          `json:"digest,omitempty"`
		ValidationDelay int             `json:"validation-delay,omitempty"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	p.Plugin = raw.Plugin
	p.Type = raw.Type
	p.API = raw.API
	p.Disable = raw.Disable
	p.Digest = raw.Digest
	p.ValidationDelay = raw.ValidationDelay
	p.Data = parsePluginData(raw.Data)
	return nil
}

// parsePluginData handles PVE's two return shapes for the data field:
// a JSON string of "k=v\nk=v" pairs, or a nested object, or absent.
func parsePluginData(raw json.RawMessage) map[string]string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	// Try string form first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return parseDataLines(s)
	}
	// Fall back to object form
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err == nil {
		out := make(map[string]string, len(m))
		for k, v := range m {
			out[k] = fmt.Sprintf("%v", v)
		}
		return out
	}
	return nil
}

func parseDataLines(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		m[line[:eq]] = line[eq+1:]
	}
	return m
}

// ACMEDirectory is a published ACME directory (Let's Encrypt, ZeroSSL, etc).
// From GET /cluster/acme/directories
type ACMEDirectory struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ACMEChallengeSchema describes one DNS/standalone plugin type with its required fields.
// From GET /cluster/acme/challenge-schema
type ACMEChallengeSchema struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Type   string                 `json:"type"`   // "dns" or "standalone"
	Schema map[string]interface{} `json:"schema"` // per-field metadata: description, type, etc.
}

// ACMEAccountDetail is the full account body returned by GET /cluster/acme/account/{name}.
type ACMEAccountDetail struct {
	Location  string                 `json:"location,omitempty"`
	Directory string                 `json:"directory,omitempty"`
	TOSURL    string                 `json:"tos,omitempty"`
	Account   map[string]interface{} `json:"account,omitempty"`
}

// NodeACMEDomain is one domain entry in a node's ACME config (parsed from `acme`/`acmedomain[0-4]` fields).
type NodeACMEDomain struct {
	Domain string `json:"domain"`
	Plugin string `json:"plugin,omitempty"`
}

// Pool is a Proxmox resource pool (cluster-wide grouping of VMs/CTs/storage).
// From GET /pools
type Pool struct {
	PoolID  string `json:"poolid"`
	Comment string `json:"comment,omitempty"`
}

// PoolMember is one member of a resource pool.
// `type` is "qemu", "lxc", or "storage". VMID is populated for qemu/lxc; Storage for storage type.
type PoolMember struct {
	Type    string `json:"type"`
	ID      string `json:"id"`   // e.g. "qemu/100", "storage/local-lvm"
	Node    string `json:"node,omitempty"`
	VMID    int    `json:"vmid,omitempty"`
	Storage string `json:"storage,omitempty"`
	Name    string `json:"name,omitempty"`   // VM/CT display name or storage name
	Status  string `json:"status,omitempty"` // running/stopped for guests
}

// PoolDetail is the full pool body returned by GET /pools/{poolid}.
type PoolDetail struct {
	Comment string       `json:"comment,omitempty"`
	Members []PoolMember `json:"members,omitempty"`
}
