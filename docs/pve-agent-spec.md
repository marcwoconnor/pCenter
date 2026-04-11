# PVE Agent Specification

Complete API surface the pve-agent must support to provide full Proxmox functionality to pCenter.

## API Endpoint Categories

### 1. Node Information (`/nodes/{node}/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/status` | GET/POST | Node status, reboot, shutdown |
| `/nodes/{node}/version` | GET | PVE version info |
| `/nodes/{node}/time` | GET/PUT | System time config |
| `/nodes/{node}/dns` | GET/PUT | DNS configuration |
| `/nodes/{node}/hosts` | GET/POST | /etc/hosts management |
| `/nodes/{node}/config` | GET/PUT | Node configuration |
| `/nodes/{node}/netstat` | GET | Network statistics |
| `/nodes/{node}/report` | GET | System report |
| `/nodes/{node}/syslog` | GET | System log |
| `/nodes/{node}/journal` | GET | Systemd journal |
| `/nodes/{node}/rrd` | GET | RRD graph data |
| `/nodes/{node}/rrddata` | GET | RRD statistics |
| `/nodes/{node}/subscription` | GET/PUT/POST/DELETE | Subscription status |
| `/nodes/{node}/aplinfo` | GET/POST | Appliance info |
| `/nodes/{node}/wakeonlan` | POST | Wake-on-LAN |

### 2. Virtual Machines (`/nodes/{node}/qemu/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/qemu` | GET/POST | List/Create VMs |
| `/nodes/{node}/qemu/{vmid}` | GET/DELETE | VM info/Delete |
| `/nodes/{node}/qemu/{vmid}/config` | GET/PUT/POST | VM configuration |
| `/nodes/{node}/qemu/{vmid}/pending` | GET | Pending config changes |
| `/nodes/{node}/qemu/{vmid}/unlink` | PUT | Unlink disk images |
| `/nodes/{node}/qemu/{vmid}/resize` | PUT | Resize disk |
| `/nodes/{node}/qemu/{vmid}/move` | POST | Move disk |
| `/nodes/{node}/qemu/{vmid}/clone` | POST | Clone VM |
| `/nodes/{node}/qemu/{vmid}/template` | POST | Convert to template |
| `/nodes/{node}/qemu/{vmid}/cloudinit` | GET/PUT | Cloud-init config |
| `/nodes/{node}/qemu/{vmid}/rrd` | GET | RRD graph data |
| `/nodes/{node}/qemu/{vmid}/rrddata` | GET | RRD statistics |

#### VM Status/Power Actions

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/qemu/{vmid}/status/current` | GET | Current status |
| `/nodes/{node}/qemu/{vmid}/status/start` | POST | Start VM |
| `/nodes/{node}/qemu/{vmid}/status/stop` | POST | Stop VM (hard) |
| `/nodes/{node}/qemu/{vmid}/status/shutdown` | POST | Shutdown VM (ACPI) |
| `/nodes/{node}/qemu/{vmid}/status/reboot` | POST | Reboot VM |
| `/nodes/{node}/qemu/{vmid}/status/reset` | POST | Reset VM (hard) |
| `/nodes/{node}/qemu/{vmid}/status/suspend` | POST | Suspend VM |
| `/nodes/{node}/qemu/{vmid}/status/resume` | POST | Resume VM |

#### VM Migration

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/qemu/{vmid}/migrate` | GET/POST | Migration info/Start migration |
| `/nodes/{node}/qemu/{vmid}/remote_migrate` | POST | Remote cluster migration |
| `/nodes/{node}/qemu/{vmid}/mtunnel` | POST | Migration tunnel |

#### VM Snapshots

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/qemu/{vmid}/snapshot` | GET/POST | List/Create snapshots |
| `/nodes/{node}/qemu/{vmid}/snapshot/{snap}` | GET/DELETE | Snapshot info/Delete |
| `/nodes/{node}/qemu/{vmid}/snapshot/{snap}/config` | GET/PUT | Snapshot config |
| `/nodes/{node}/qemu/{vmid}/snapshot/{snap}/rollback` | POST | Rollback to snapshot |

#### VM Console Access

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/qemu/{vmid}/vncproxy` | POST | VNC proxy ticket |
| `/nodes/{node}/qemu/{vmid}/vncwebsocket` | GET | VNC WebSocket |
| `/nodes/{node}/qemu/{vmid}/termproxy` | POST | Terminal proxy |
| `/nodes/{node}/qemu/{vmid}/spiceproxy` | POST | SPICE proxy |
| `/nodes/{node}/qemu/{vmid}/monitor` | POST | QEMU monitor command |
| `/nodes/{node}/qemu/{vmid}/sendkey` | PUT | Send key to VM |

#### VM Firewall

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/qemu/{vmid}/firewall/rules` | GET/POST | Firewall rules |
| `/nodes/{node}/qemu/{vmid}/firewall/rules/{pos}` | GET/PUT/DELETE | Rule by position |
| `/nodes/{node}/qemu/{vmid}/firewall/aliases` | GET/POST | Aliases |
| `/nodes/{node}/qemu/{vmid}/firewall/ipset` | GET/POST | IP sets |
| `/nodes/{node}/qemu/{vmid}/firewall/options` | GET/PUT | Firewall options |
| `/nodes/{node}/qemu/{vmid}/firewall/log` | GET | Firewall log |
| `/nodes/{node}/qemu/{vmid}/firewall/refs` | GET | References |

#### QEMU Guest Agent

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/qemu/{vmid}/agent/ping` | POST | Ping agent |
| `/nodes/{node}/qemu/{vmid}/agent/info` | GET | Agent info |
| `/nodes/{node}/qemu/{vmid}/agent/get-osinfo` | GET | OS information |
| `/nodes/{node}/qemu/{vmid}/agent/get-host-name` | GET | Hostname |
| `/nodes/{node}/qemu/{vmid}/agent/get-time` | GET | Guest time |
| `/nodes/{node}/qemu/{vmid}/agent/get-timezone` | GET | Timezone |
| `/nodes/{node}/qemu/{vmid}/agent/get-vcpus` | GET | vCPU info |
| `/nodes/{node}/qemu/{vmid}/agent/get-users` | GET | Logged in users |
| `/nodes/{node}/qemu/{vmid}/agent/get-fsinfo` | GET | Filesystem info |
| `/nodes/{node}/qemu/{vmid}/agent/get-memory-blocks` | GET | Memory blocks |
| `/nodes/{node}/qemu/{vmid}/agent/get-memory-block-info` | GET | Memory block info |
| `/nodes/{node}/qemu/{vmid}/agent/network-get-interfaces` | GET | Network interfaces |
| `/nodes/{node}/qemu/{vmid}/agent/exec` | POST | Execute command |
| `/nodes/{node}/qemu/{vmid}/agent/exec-status` | GET | Command status |
| `/nodes/{node}/qemu/{vmid}/agent/file-read` | GET | Read file |
| `/nodes/{node}/qemu/{vmid}/agent/file-write` | POST | Write file |
| `/nodes/{node}/qemu/{vmid}/agent/set-user-password` | POST | Set password |
| `/nodes/{node}/qemu/{vmid}/agent/shutdown` | POST | Shutdown via agent |
| `/nodes/{node}/qemu/{vmid}/agent/suspend-ram` | POST | Suspend to RAM |
| `/nodes/{node}/qemu/{vmid}/agent/suspend-disk` | POST | Suspend to disk |
| `/nodes/{node}/qemu/{vmid}/agent/suspend-hybrid` | POST | Hybrid suspend |
| `/nodes/{node}/qemu/{vmid}/agent/fsfreeze-freeze` | POST | Freeze filesystems |
| `/nodes/{node}/qemu/{vmid}/agent/fsfreeze-thaw` | POST | Thaw filesystems |
| `/nodes/{node}/qemu/{vmid}/agent/fsfreeze-status` | POST | Freeze status |
| `/nodes/{node}/qemu/{vmid}/agent/fstrim` | POST | Trim filesystems |

### 3. Containers (`/nodes/{node}/lxc/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/lxc` | GET/POST | List/Create containers |
| `/nodes/{node}/lxc/{vmid}` | GET/DELETE | Container info/Delete |
| `/nodes/{node}/lxc/{vmid}/config` | GET/PUT | Container config |
| `/nodes/{node}/lxc/{vmid}/pending` | GET | Pending changes |
| `/nodes/{node}/lxc/{vmid}/resize` | PUT | Resize volume |
| `/nodes/{node}/lxc/{vmid}/clone` | POST | Clone container |
| `/nodes/{node}/lxc/{vmid}/template` | POST | Convert to template |
| `/nodes/{node}/lxc/{vmid}/interfaces` | GET | Network interfaces |
| `/nodes/{node}/lxc/{vmid}/rrd` | GET | RRD graph |
| `/nodes/{node}/lxc/{vmid}/rrddata` | GET | RRD data |

#### Container Status/Power

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/lxc/{vmid}/status/current` | GET | Current status |
| `/nodes/{node}/lxc/{vmid}/status/start` | POST | Start container |
| `/nodes/{node}/lxc/{vmid}/status/stop` | POST | Stop container |
| `/nodes/{node}/lxc/{vmid}/status/shutdown` | POST | Shutdown container |
| `/nodes/{node}/lxc/{vmid}/status/reboot` | POST | Reboot container |
| `/nodes/{node}/lxc/{vmid}/status/suspend` | POST | Suspend (checkpoint) |
| `/nodes/{node}/lxc/{vmid}/status/resume` | POST | Resume |

#### Container Migration & Snapshots

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/lxc/{vmid}/migrate` | GET/POST | Migration |
| `/nodes/{node}/lxc/{vmid}/snapshot` | GET/POST | Snapshots |
| `/nodes/{node}/lxc/{vmid}/snapshot/{snap}` | GET/DELETE | Snapshot ops |
| `/nodes/{node}/lxc/{vmid}/snapshot/{snap}/rollback` | POST | Rollback |

#### Container Console

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/lxc/{vmid}/vncproxy` | POST | VNC proxy |
| `/nodes/{node}/lxc/{vmid}/vncwebsocket` | GET | VNC WebSocket |
| `/nodes/{node}/lxc/{vmid}/termproxy` | POST | Terminal proxy |
| `/nodes/{node}/lxc/{vmid}/spiceproxy` | POST | SPICE proxy |

#### Container Firewall

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/lxc/{vmid}/firewall/rules` | GET/POST | Rules |
| `/nodes/{node}/lxc/{vmid}/firewall/options` | GET/PUT | Options |
| `/nodes/{node}/lxc/{vmid}/firewall/log` | GET | Log |

### 4. Storage (`/nodes/{node}/storage/` & `/storage/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/storage` | GET/POST | List/Create storage |
| `/storage/{storage}` | GET/PUT/DELETE | Storage config |
| `/nodes/{node}/storage` | GET | Node storage list |
| `/nodes/{node}/storage/{storage}/status` | GET | Storage status |
| `/nodes/{node}/storage/{storage}/rrd` | GET | RRD graph |
| `/nodes/{node}/storage/{storage}/rrddata` | GET | RRD data |
| `/nodes/{node}/storage/{storage}/content` | GET/POST | Content list/Upload |
| `/nodes/{node}/storage/{storage}/content/{volume}` | GET/PUT/DELETE | Volume ops |
| `/nodes/{node}/storage/{storage}/upload` | POST | Upload file |
| `/nodes/{node}/storage/{storage}/download-url` | POST | Download from URL |
| `/nodes/{node}/storage/{storage}/prunebackups` | GET/DELETE | Backup pruning |
| `/nodes/{node}/storage/{storage}/import-metadata` | GET | Import metadata |
| `/nodes/{node}/storage/{storage}/file-restore` | GET | PBS file restore |

### 5. Ceph (`/nodes/{node}/ceph/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/ceph/status` | GET | Cluster status |
| `/nodes/{node}/ceph/init` | POST | Initialize Ceph |
| `/nodes/{node}/ceph/start` | POST | Start Ceph services |
| `/nodes/{node}/ceph/stop` | POST | Stop Ceph services |
| `/nodes/{node}/ceph/restart` | POST | Restart Ceph |
| `/nodes/{node}/ceph/log` | GET | Ceph log |
| `/nodes/{node}/ceph/crush` | GET | CRUSH map |
| `/nodes/{node}/ceph/cmd-safety` | GET | Command safety check |

#### Ceph OSDs

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/ceph/osd` | GET/POST | List/Create OSDs |
| `/nodes/{node}/ceph/osd/{osdid}` | DELETE | Delete OSD |
| `/nodes/{node}/ceph/osd/{osdid}/in` | POST | Mark OSD in |
| `/nodes/{node}/ceph/osd/{osdid}/out` | POST | Mark OSD out |
| `/nodes/{node}/ceph/osd/{osdid}/scrub` | POST | Scrub OSD |
| `/nodes/{node}/ceph/osd/{osdid}/lv-info` | GET | LV info |
| `/nodes/{node}/ceph/osd/{osdid}/metadata` | GET | OSD metadata |

#### Ceph Monitors

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/ceph/mon` | GET/POST | List/Create monitors |
| `/nodes/{node}/ceph/mon/{monid}` | DELETE | Delete monitor |

#### Ceph Managers

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/ceph/mgr` | GET/POST | List/Create managers |
| `/nodes/{node}/ceph/mgr/{id}` | DELETE | Delete manager |

#### Ceph MDS (CephFS)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/ceph/mds` | GET/POST | List/Create MDS |
| `/nodes/{node}/ceph/mds/{name}` | DELETE | Delete MDS |
| `/nodes/{node}/ceph/fs` | GET/POST | List/Create CephFS |
| `/nodes/{node}/ceph/fs/{name}` | DELETE | Delete CephFS |

#### Ceph Pools

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/ceph/pool` | GET/POST | List/Create pools |
| `/nodes/{node}/ceph/pool/{name}` | GET/PUT/DELETE | Pool operations |
| `/nodes/{node}/ceph/pool/{name}/status` | GET | Pool status |

#### Ceph Rules

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/ceph/rules` | GET | CRUSH rules |

### 6. Network (`/nodes/{node}/network/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/network` | GET/POST/PUT/DELETE | Network interfaces |
| `/nodes/{node}/network/{iface}` | GET/PUT/DELETE | Interface config |

### 7. Services (`/nodes/{node}/services/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/services` | GET | List services |
| `/nodes/{node}/services/{service}/state` | GET | Service state |
| `/nodes/{node}/services/{service}/start` | POST | Start service |
| `/nodes/{node}/services/{service}/stop` | POST | Stop service |
| `/nodes/{node}/services/{service}/restart` | POST | Restart service |
| `/nodes/{node}/services/{service}/reload` | POST | Reload service |

**Available services:**
- chrony, corosync, cron, ksmtuned, lxcfs
- postfix, proxmox-firewall, pve-cluster
- pve-firewall, pve-ha-crm, pve-ha-lrm
- pve-lxc-syscalld, pvedaemon, pvefw-logger
- pveproxy, pvescheduler, pvestatd
- qmeventd, spiceproxy, sshd, syslog
- systemd-journald, systemd-timesyncd

### 8. Disks (`/nodes/{node}/disks/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/disks/list` | GET | List disks |
| `/nodes/{node}/disks/smart` | GET | SMART data |
| `/nodes/{node}/disks/initgpt` | POST | Initialize GPT |
| `/nodes/{node}/disks/wipedisk` | PUT | Wipe disk |
| `/nodes/{node}/disks/directory` | GET/POST | Directory storage |
| `/nodes/{node}/disks/lvm` | GET/POST | LVM management |
| `/nodes/{node}/disks/lvmthin` | GET/POST | LVM thin pools |
| `/nodes/{node}/disks/zfs` | GET/POST | ZFS pools |
| `/nodes/{node}/disks/zfs/{name}` | GET/DELETE | ZFS pool ops |

### 9. Tasks (`/nodes/{node}/tasks/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/tasks` | GET | List tasks |
| `/nodes/{node}/tasks/{upid}` | GET/DELETE | Task info/Stop |
| `/nodes/{node}/tasks/{upid}/log` | GET | Task log |
| `/nodes/{node}/tasks/{upid}/status` | GET | Task status |

### 10. Certificates (`/nodes/{node}/certificates/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/certificates/info` | GET | Certificate info |
| `/nodes/{node}/certificates/custom` | POST/DELETE | Custom certificate |
| `/nodes/{node}/certificates/acme/certificate` | POST/PUT/DELETE | ACME cert |

### 11. Hardware (`/nodes/{node}/hardware/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/hardware/pci` | GET | PCI devices |
| `/nodes/{node}/hardware/pci/{pciid}` | GET | PCI device info |
| `/nodes/{node}/hardware/pci/{pciid}/mdev` | GET | Mediated devices |
| `/nodes/{node}/hardware/usb` | GET | USB devices |

### 12. APT (`/nodes/{node}/apt/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/apt/update` | GET/POST | Update status/Run update |
| `/nodes/{node}/apt/changelog` | GET | Package changelog |
| `/nodes/{node}/apt/versions` | GET | Package versions |
| `/nodes/{node}/apt/repositories` | GET/PUT/POST | Repo config |

### 13. Firewall (`/nodes/{node}/firewall/` & `/cluster/firewall/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/firewall/rules` | GET/POST | Node rules |
| `/nodes/{node}/firewall/options` | GET/PUT | Node FW options |
| `/nodes/{node}/firewall/log` | GET | Firewall log |
| `/cluster/firewall/rules` | GET/POST | Cluster rules |
| `/cluster/firewall/groups` | GET/POST | Security groups |
| `/cluster/firewall/aliases` | GET/POST | Aliases |
| `/cluster/firewall/ipset` | GET/POST | IP sets |
| `/cluster/firewall/options` | GET/PUT | Cluster FW options |
| `/cluster/firewall/macros` | GET | Available macros |
| `/cluster/firewall/refs` | GET | References |

### 14. Cluster (`/cluster/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/cluster/status` | GET | Cluster status |
| `/cluster/resources` | GET | All cluster resources |
| `/cluster/tasks` | GET | Cluster-wide tasks |
| `/cluster/options` | GET/PUT | Cluster options |
| `/cluster/log` | GET | Cluster log |
| `/cluster/nextid` | GET | Next free VMID |

#### Cluster Config

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/cluster/config` | GET/POST | Cluster config |
| `/cluster/config/join` | GET/POST | Join info/Join cluster |
| `/cluster/config/nodes` | GET | Cluster nodes |
| `/cluster/config/nodes/{node}` | POST/DELETE | Add/Remove node |
| `/cluster/config/totem` | GET | Corosync totem |
| `/cluster/config/qdevice` | GET | QDevice status |
| `/cluster/config/apiversion` | GET | API version |

#### High Availability

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/cluster/ha/status` | GET | HA status |
| `/cluster/ha/status/current` | GET | Current HA state |
| `/cluster/ha/status/manager_status` | GET | HA manager status |
| `/cluster/ha/resources` | GET/POST | HA resources |
| `/cluster/ha/resources/{sid}` | GET/PUT/DELETE | HA resource ops |
| `/cluster/ha/resources/{sid}/migrate` | POST | Migrate HA resource |
| `/cluster/ha/resources/{sid}/relocate` | POST | Relocate HA resource |
| `/cluster/ha/groups` | GET/POST | HA groups |
| `/cluster/ha/groups/{group}` | GET/PUT/DELETE | HA group ops |

#### Backup

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/cluster/backup` | GET/POST | Backup jobs |
| `/cluster/backup/{id}` | GET/PUT/DELETE | Backup job ops |
| `/cluster/backup/{id}/included_volumes` | GET | Included volumes |
| `/cluster/backup/{id}/run` | POST | Run backup now |
| `/cluster/backup-info/not-backed-up` | GET | VMs not backed up |

#### Replication

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/cluster/replication` | GET/POST | Replication jobs |
| `/cluster/replication/{id}` | GET/PUT/DELETE | Job operations |
| `/cluster/replication/{id}/schedule_now` | POST | Run now |

#### SDN (Software Defined Networking)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/cluster/sdn` | GET/PUT | SDN status/Apply |
| `/cluster/sdn/zones` | GET/POST | Zones |
| `/cluster/sdn/zones/{zone}` | GET/PUT/DELETE | Zone ops |
| `/cluster/sdn/vnets` | GET/POST | Virtual networks |
| `/cluster/sdn/vnets/{vnet}` | GET/PUT/DELETE | VNet ops |
| `/cluster/sdn/vnets/{vnet}/subnets` | GET/POST | Subnets |
| `/cluster/sdn/controllers` | GET/POST | Controllers |
| `/cluster/sdn/ipams` | GET/POST | IPAM |
| `/cluster/sdn/dns` | GET/POST | DNS |

#### Notifications

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/cluster/notifications/endpoints` | GET | Notification endpoints |
| `/cluster/notifications/endpoints/{type}` | GET/POST | Endpoint by type |
| `/cluster/notifications/matchers` | GET/POST | Matchers |
| `/cluster/notifications/targets` | GET | Targets |

#### Metrics

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/cluster/metrics/server` | GET/POST | Metrics servers |
| `/cluster/metrics/server/{id}` | GET/PUT/DELETE | Server ops |

#### Mapping (PCI/USB passthrough)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/cluster/mapping/pci` | GET/POST | PCI mappings |
| `/cluster/mapping/usb` | GET/POST | USB mappings |

### 15. Access Control (`/access/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/access/ticket` | GET/POST | Auth ticket |
| `/access/password` | PUT | Change password |
| `/access/acl` | GET/PUT | ACL |
| `/access/users` | GET/POST | Users |
| `/access/users/{userid}` | GET/PUT/DELETE | User ops |
| `/access/users/{userid}/token` | GET/POST | API tokens |
| `/access/users/{userid}/token/{tokenid}` | GET/PUT/DELETE | Token ops |
| `/access/users/{userid}/tfa` | GET | User TFA |
| `/access/groups` | GET/POST | Groups |
| `/access/groups/{groupid}` | GET/PUT/DELETE | Group ops |
| `/access/roles` | GET/POST | Roles |
| `/access/roles/{roleid}` | GET/PUT/DELETE | Role ops |
| `/access/domains` | GET/POST | Auth domains |
| `/access/domains/{realm}` | GET/PUT/DELETE | Domain ops |
| `/access/domains/{realm}/sync` | POST | Sync realm |
| `/access/tfa` | GET/POST | TFA |
| `/access/tfa/{userid}` | GET | User TFA list |
| `/access/tfa/{userid}/{id}` | GET/PUT/DELETE | TFA entry |
| `/access/openid/auth-url` | POST | OpenID auth URL |
| `/access/openid/login` | POST | OpenID login |

### 16. Pools (`/pools/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/pools` | GET/POST | Resource pools |
| `/pools/{poolid}` | GET/PUT/DELETE | Pool ops |

### 17. Bulk Operations (`/nodes/{node}/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/startall` | POST | Start all VMs/CTs |
| `/nodes/{node}/stopall` | POST | Stop all VMs/CTs |
| `/nodes/{node}/suspendall` | POST | Suspend all |
| `/nodes/{node}/migrateall` | POST | Migrate all to another node |
| `/nodes/{node}/vzdump` | POST | Backup (vzdump) |

### 18. Scanning/Discovery (`/nodes/{node}/scan/`)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/scan/nfs` | GET | Scan NFS exports |
| `/nodes/{node}/scan/cifs` | GET | Scan CIFS shares |
| `/nodes/{node}/scan/iscsi` | GET | Scan iSCSI targets |
| `/nodes/{node}/scan/lvm` | GET | Scan LVM |
| `/nodes/{node}/scan/zfs` | GET | Scan ZFS pools |
| `/nodes/{node}/scan/pbs` | GET | Scan PBS |

### 19. Console/Shell Access

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/nodes/{node}/vncshell` | POST | Node VNC shell |
| `/nodes/{node}/termproxy` | POST | Node terminal |
| `/nodes/{node}/spiceshell` | POST | Node SPICE shell |

---

## System Data (via /proc, not API)

The agent should also collect system metrics not exposed via API:

| Source | Data |
|--------|------|
| `/proc/vmstat` | pgpgin, pgpgout, pswpin, pswpout, pgfault, pgmajfault |
| `/proc/meminfo` | Detailed memory stats |
| `/proc/loadavg` | Load averages |
| `/proc/stat` | CPU statistics |
| `/proc/diskstats` | Disk I/O statistics |
| `/proc/net/dev` | Network interface stats |
| `/sys/class/thermal` | Temperature sensors |
| `smartctl` | SMART disk health |

---

## Agent Implementation Phases

### Phase 1: Core Monitoring (Read-only)
- Node status, version, hardware
- VM/CT list and status
- Storage status
- Ceph status
- Metrics (RRD data)
- System metrics (/proc)

### Phase 2: Power Management
- VM/CT start, stop, shutdown, reboot
- Node reboot/shutdown
- Bulk operations (startall, stopall)

### Phase 3: Configuration
- VM/CT config read/write
- Network configuration
- Storage management
- Firewall rules

### Phase 4: Advanced Operations
- Migration (live and offline)
- Snapshots
- Cloning
- Backup/Restore
- HA management

### Phase 5: Full Feature Parity
- QEMU guest agent operations
- Certificate management
- SDN
- Ceph management
- Access control
