// API response types matching Go backend

export interface Node {
  cluster: string;
  node: string;
  status: string;
  cpu: number;
  maxcpu: number;
  mem: number;
  maxmem: number;
  disk: number;
  maxdisk: number;
  uptime: number;
  last_update?: number;
  error?: string;
  pve_version?: string;
  kernel_version?: string;
  cpu_model?: string;
  loadavg?: string[];
}

export interface GuestNIC {
  name: string;    // net0, net1, etc.
  bridge: string;  // vmbr0, vmbr1, etc.
  mac?: string;
  model?: string;  // virtio, e1000, etc.
  tag?: number;    // VLAN tag
}

export interface Guest {
  cluster: string;
  vmid: number;
  name: string;
  node: string;
  type: 'qemu' | 'lxc';
  status: string;
  cpu: number;
  cpus: number;
  mem: number;
  maxmem: number;
  disk: number;
  maxdisk: number;
  uptime: number;
  template?: boolean;
  tags?: string;
  ha_state?: string;
  nics?: GuestNIC[];
}

export interface VM {
  cluster: string;
  vmid: number;
  name: string;
  node: string;
  status: string;
  cpu: number;
  cpus: number;
  mem: number;
  maxmem: number;
  disk: number;
  maxdisk: number;
  uptime: number;
  netin: number;
  netout: number;
  diskread: number;
  diskwrite: number;
  template: boolean;
  tags?: string;
  ha_state?: string;
}

export interface Container {
  cluster: string;
  vmid: number;
  name: string;
  node: string;
  status: string;
  cpu: number;
  cpus: number;
  mem: number;
  maxmem: number;
  swap: number;
  maxswap: number;
  disk: number;
  maxdisk: number;
  uptime: number;
  netin: number;
  netout: number;
  type: string;
  template: boolean;
  tags?: string;
  ha_state?: string;
}

export interface NodeCertificate {
  filename: string;
  fingerprint?: string;
  issuer?: string;
  notafter?: number;   // Unix seconds
  notbefore?: number;
  public_key_bits?: number;
  public_key_type?: string;
  san?: string[];
  subject?: string;
  pem?: string;
}

export interface ACMEAccount {
  name: string;
  directory?: string;
  tos?: string;
}

export interface ACMEPlugin {
  plugin: string;
  type: string;
  api?: string;
  disable?: number;
}

export interface Storage {
  cluster: string;
  storage: string;
  node: string;
  type: string;
  status: string;
  active: number;
  enabled: number;
  shared: number | boolean;
  content: string;
  used: number;
  avail: number;
  total: number;
}

export interface StorageVolume {
  volid: string;      // e.g., "local-lvm:vm-100-disk-0"
  format: string;     // raw, qcow2, subvol, etc
  size: number;       // size in bytes
  used?: number;      // used space (for thin)
  vmid?: number;      // VM ID if this is a VM disk
  content: string;    // images, rootdir, iso, vztmpl, backup
  ctime?: number;     // creation time
  parent?: string;    // parent snapshot
  notes?: string;     // description/notes
}

export interface Summary {
  TotalNodes: number;
  OnlineNodes: number;
  TotalVMs: number;
  RunningVMs: number;
  TotalContainers: number;
  RunningCTs: number;
  TotalCPU: number;
  UsedCPU: number;
  TotalMemGB: number;
  UsedMemGB: number;
}

export interface CephHealthCheckDetail {
  message: string;
}

export interface CephHealthCheckSummary {
  count: number;
  message: string;
}

export interface CephHealthCheck {
  severity: string;
  summary: CephHealthCheckSummary;
  detail: CephHealthCheckDetail[];
  muted: boolean;
}

export interface CephStatus {
  health: {
    status: string;
    checks?: Record<string, CephHealthCheck>;
  };
  pgmap: {
    bytes_used: number;
    bytes_avail: number;
    bytes_total: number;
  };
}

// Multi-cluster types

export interface HAInfo {
  enabled: boolean;
  quorum: boolean;
  manager: string;
}

export interface ClusterInfo {
  name: string;
  summary: Summary;
  ha?: HAInfo;
}

export interface MigrationProgress {
  upid: string;
  cluster: string;
  vmid: number;
  guest_name: string;
  guest_type: 'vm' | 'ct';
  from_node: string;
  to_node: string;
  online: boolean;
  started_at: string;
  progress: number;
  status: 'running' | 'completed' | 'failed';
  error?: string;
}

export interface DRSRecommendation {
  id: string;
  cluster: string;
  guest_type: 'vm' | 'ct';
  vmid: number;
  guest_name: string;
  from_node: string;
  to_node: string;
  reason: string;
  priority: number;
  created_at: string;
}

export interface GlobalSummary {
  clusters: ClusterInfo[];
  total: Summary;
}

// Network/SDN types

export interface NetworkInterface {
  cluster: string;
  node: string;
  iface: string;
  type: string;
  active: number;
  autostart: number;
  method?: string;
  method6?: string;
  address?: string;
  netmask?: string;
  gateway?: string;
  cidr?: string;
  address6?: string;
  netmask6?: string;
  gateway6?: string;
  bridge_ports?: string;
  bridge_stp?: string;
  bridge_fd?: string;
  bridge_vlan_aware?: number;
  slaves?: string;
  bond_mode?: string;
  'bond-primary'?: string;
  'vlan-raw-device'?: string;
  'vlan-id'?: string | number;
  mtu?: number;
  comments?: string;
}

export interface SDNZone {
  cluster: string;
  zone: string;
  type: string;
  state?: string;
  pending?: number;
  nodes?: string;
  ipam?: string;
  dns?: string;
  reversedns?: string;
  dnszone?: string;
  bridge?: string;
  tag?: number;
  'vlan-protocol'?: string;
  mtu?: number;
  peers?: string;
}

export interface SDNVNet {
  cluster: string;
  vnet: string;
  zone: string;
  type?: string;
  state?: string;
  pending?: number;
  alias?: string;
  tag?: number;
  vlanaware?: number;
}

export interface SDNSubnet {
  cluster: string;
  subnet: string;
  vnet: string;
  zone?: string;
  type?: string;
  state?: string;
  gateway?: string;
  snat?: number;
  dnszoneprefix?: string;
}

export interface NetworkOverview {
  interfaces: NetworkInterface[];
  sdn_zones: SDNZone[];
  sdn_vnets: SDNVNet[];
  sdn_subnets: SDNSubnet[];
}

// SMART disk monitoring types

export interface SmartAttribute {
  id: number;
  name: string;
  value: number;
  worst: number;
  threshold: number;
  raw: number;
  flags: string;
  when_failed?: string;
  critical: boolean;
}

export interface NVMeHealth {
  critical_warning: number;
  available_spare: number;
  available_spare_thresh: number;
  percent_used: number;
  data_units_read: number;
  data_units_written: number;
  power_cycles: number;
  unsafe_shutdowns: number;
  media_errors: number;
  error_log_entries: number;
}

export interface SmartDisk {
  node: string;
  cluster?: string;
  device: string;
  model: string;
  serial: string;
  capacity: number;
  type: 'hdd' | 'ssd' | 'nvme';
  protocol: string;
  health: 'PASSED' | 'WARNING' | 'FAILED' | 'UNKNOWN';
  power_on_hours: number;
  temperature: number;
  attributes?: SmartAttribute[];
  nvme_health?: NVMeHealth;
}

// Maintenance mode types

export interface QDeviceStatus {
  configured: boolean;
  connected: boolean;
  host_node: string;
  host_vmid: number;
  host_vm_name: string;
  qnetd_address: string;
  algorithm: string;
  state: string;
}

export interface MaintenancePreflightCheck {
  name: string;
  status: 'ok' | 'warning' | 'error';
  message: string;
  blocking: boolean;
}

export interface GuestToMove {
  vmid: number;
  name: string;
  type: 'qemu' | 'lxc';
  status: string;
  target_node: string;
  is_critical: boolean;
  reason?: string;
}

export interface MaintenancePreflight {
  node: string;
  can_enter: boolean;
  checks: MaintenancePreflightCheck[];
  guests_to_move: GuestToMove[];
  critical_guests: GuestToMove[];
}

export interface MaintenanceState {
  node: string;
  in_maintenance: boolean;
  entered_at?: string;
  phase?: 'preflight' | 'evacuating' | 'ready' | 'exiting' | 'error';
  progress: number;
  message?: string;
}

// Metrics types

export interface MetricDataPoint {
  ts: number;
  value: number;
  min?: number;
  max?: number;
}

export interface MetricSeries {
  metric: string;
  resource_id: string;
  unit: string;
  data: MetricDataPoint[];
}

export interface MetricsMeta {
  start: number;
  end: number;
  resolution: string;
  point_count: number;
}

export interface MetricsResponse {
  series: MetricSeries[];
  meta: MetricsMeta;
}

// Folder types for organizational hierarchy

export type TreeView = 'hosts' | 'vms';

export interface Folder {
  id: string;
  name: string;
  parent_id?: string;
  tree_view: TreeView;
  cluster?: string;
  sort_order: number;
  created_at: string;
  updated_at: string;
  children?: Folder[];
  members?: FolderMember[];
}

export interface FolderMember {
  folder_id: string;
  resource_type: 'vm' | 'ct' | 'node' | 'storage';
  resource_id: string;
  cluster: string;
  added_at: string;
}

export interface CreateFolderRequest {
  name: string;
  parent_id?: string;
  tree_view: TreeView;
  cluster?: string;
}

export interface MoveResourceRequest {
  resource_type: 'vm' | 'ct' | 'node' | 'storage';
  resource_id: string;
  cluster: string;
  to_folder_id?: string;
}

// Tag types

export interface Tag {
  id: string;
  category: string;
  name: string;
  color: string;
  created_at: string;
}

export interface TagAssignment {
  tag_id: string;
  object_type: string; // vm, ct, node, storage
  object_id: string;
  cluster: string;
}

export interface CreateTagRequest {
  category: string;
  name: string;
  color: string;
}

export interface AssignTagRequest {
  tag_id: string;
  object_type: string;
  object_id: string;
  cluster: string;
}

// Alarm types

export interface AlarmInstance {
  id: string;
  definition_id: string;
  definition_name: string;
  cluster: string;
  resource_type: string;
  resource_id: string;
  resource_name: string;
  state: 'normal' | 'warning' | 'critical';
  current_value: number;
  threshold: number;
  triggered_at?: number;
  last_evaluated_at: number;
  acknowledged_by?: string;
  acknowledged_at?: number;
  consecutive_count: number;
}

export interface AlarmDefinition {
  id: string;
  name: string;
  enabled: boolean;
  metric_type: string;
  resource_type: string;
  scope: string;
  scope_target: string;
  condition: string;
  warning_threshold: number;
  critical_threshold: number;
  clear_threshold: number;
  duration_samples: number;
  notify_channels: string[];
  created_at: number;
}

export interface NotificationChannel {
  id: string;
  name: string;
  type: string;
  config: string;
  enabled: boolean;
}

// DRS Rules types

export interface DRSRule {
  id: string;
  cluster: string;
  name: string;
  type: 'affinity' | 'anti-affinity' | 'host-pin';
  enabled: boolean;
  members: number[];
  host_node: string;
}

export interface DRSRuleViolation {
  rule_id: string;
  rule_name: string;
  rule_type: string;
  cluster: string;
  message: string;
}

// Configuration types for VM/Container settings

export interface VMConfig {
  digest: string;
  name?: string;
  description?: string;

  // Hardware
  cores?: number;
  sockets?: number;
  cpu?: string;
  memory?: number;
  balloon?: number;
  numa?: number;
  bios?: string;
  machine?: string;

  // Boot
  boot?: string;
  bootdisk?: string;

  // Options
  onboot?: number;
  protection?: number;
  agent?: string;
  ostype?: string;

  // Cloud-init
  ciuser?: string;
  cipassword?: string;
  sshkeys?: string;
  ipconfig0?: string;
  ipconfig1?: string;
  nameserver?: string;
  searchdomain?: string;

  // VGA
  vga?: string;

  // Dynamic fields (scsi0, net0, ide0, etc.)
  raw_config?: Record<string, unknown>;
}

export interface ContainerConfig {
  digest: string;
  hostname?: string;
  description?: string;

  // Resources
  cores?: number;
  cpulimit?: number;
  cpuunits?: number;
  memory?: number;
  swap?: number;

  // Root filesystem
  rootfs?: string;

  // Options
  onboot?: number;
  protection?: number;
  unprivileged?: number;
  ostype?: string;
  arch?: string;

  // Features
  features?: string;

  // Startup
  startup?: string;

  // Dynamic fields (net0, mp0, etc.)
  raw_config?: Record<string, unknown>;
}

export interface ConfigResponse<T> {
  config: T;
  digest: string;
  node: string;
  vmid: number;
}

export interface Snapshot {
  name: string;
  description?: string;
  snaptime?: number;
  vmstate?: number;
  parent?: string;
}

export interface CreateSnapshotRequest {
  name: string;
  description?: string;
  vmstate?: boolean; // Include RAM state (VM only)
}

export interface FirewallRule {
  pos?: number;
  type: string;
  action: string;
  enable?: number;
  source?: string;
  dest?: string;
  sport?: string;
  dport?: string;
  proto?: string;
  macro?: string;
  iface?: string;
  log?: string;
  comment?: string;
}

export interface FirewallOptions {
  enable?: number;
  dhcp?: number;
  dhcp6?: number;
  ipfilter?: number;
  log_level_in?: string;
  log_level_out?: string;
  macfilter?: number;
  ndp?: number;
  policy_in?: string;
  policy_out?: string;
  radv?: number;
}

// Activity log entry
export interface ActivityEntry {
  id: number;
  timestamp: string;
  action: string;
  resource_type: string;
  resource_id: string;
  resource_name?: string;
  cluster: string;
  details?: string;
  status: string;
}

// Datacenter/Cluster inventory types

export type ClusterStatus = 'empty' | 'pending' | 'active' | 'error';
export type HostStatus = 'staged' | 'connecting' | 'online' | 'offline' | 'error';

export interface Datacenter {
  id: string;
  name: string;
  description?: string;
  created_at: string;
  updated_at: string;
  clusters?: InventoryCluster[];
  hosts?: InventoryHost[];  // Standalone hosts (not in a cluster)
}

export interface InventoryCluster {
  id: string;
  name: string;
  agent_name?: string;  // What agents report as (for matching runtime data)
  datacenter_id?: string;
  datacenter_name?: string;
  status: ClusterStatus;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  hosts?: InventoryHost[];
}

export interface InventoryHost {
  id: string;
  cluster_id?: string;      // Empty for standalone hosts
  datacenter_id?: string;   // Set for standalone hosts
  address: string;
  token_id: string;
  insecure: boolean;
  status: HostStatus;
  error?: string;
  node_name?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateDatacenterRequest {
  name: string;
  description?: string;
}

export interface UpdateDatacenterRequest {
  name: string;
  description?: string;
}

export interface CreateClusterRequest {
  name: string;
  datacenter_id?: string;
}

export interface UpdateClusterRequest {
  name: string;
  datacenter_id?: string;
  enabled: boolean;
}

export interface AddHostRequest {
  address: string;
  insecure: boolean;
  // Method 1: Username/password (preferred - auto-creates token)
  username?: string;
  password?: string;
  // Method 2: Existing token (backward compat)
  token_id?: string;
  token_secret?: string;
}

export interface UpdateHostRequest {
  address: string;
  token_id: string;
  token_secret?: string;
  insecure: boolean;
}

export interface DatacenterTreeResponse {
  datacenters: Datacenter[];
  orphan_clusters: InventoryCluster[];
}

// VM/Container creation types

export interface CreateVMRequest {
  vmid: number;
  name: string;
  cores: number;
  memory: number;      // MB
  storage: string;     // e.g., "local-lvm"
  disk_size: number;   // GB
  iso?: string;        // e.g., "local:iso/ubuntu.iso"
  ostype?: string;     // l26, win10, etc.
  network?: string;    // bridge name (e.g., vmbr0)
  start?: boolean;
}

export interface CreateContainerRequest {
  vmid: number;
  hostname: string;
  ostemplate: string;  // REQUIRED: e.g., "local:vztmpl/ubuntu.tar.gz"
  cores: number;
  memory: number;      // MB
  swap: number;        // MB
  storage: string;     // root storage
  disk_size: number;   // GB
  network?: string;    // bridge name
  password?: string;
  ssh_public_keys?: string;
  start?: boolean;
  unprivileged: boolean;
}

// Content Library types

export type LibraryItemType = 'iso' | 'vztmpl' | 'vm-template' | 'ova' | 'snippet';

export interface LibraryItem {
  id: string;
  name: string;
  description?: string;
  type: LibraryItemType;
  category?: string;
  version?: string;
  tags: string[];

  cluster: string;
  node?: string;
  storage: string;
  volume: string;

  size: number;
  format?: string;

  vmid?: number;
  os_type?: string;
  cores?: number;
  memory?: number;

  created_at: string;
  updated_at: string;
  created_by?: string;
}

export interface CreateLibraryItemRequest {
  name: string;
  description?: string;
  type: LibraryItemType;
  category?: string;
  version?: string;
  tags?: string[];
  cluster: string;
  node?: string;
  storage: string;
  volume: string;
  size?: number;
  format?: string;
  vmid?: number;
  os_type?: string;
  cores?: number;
  memory?: number;
}

export interface UpdateLibraryItemRequest {
  name?: string;
  description?: string;
  category?: string;
  version?: string;
  tags?: string[];
}

export interface DeployLibraryItemRequest {
  target_cluster: string;
  target_node: string;
  target_storage?: string;
  new_name?: string;
  new_vmid?: number;
  full: boolean;
}

// Node (host) configuration types

export interface NodeDNS {
  search: string;
  dns1: string;
  dns2?: string;
  dns3?: string;
}

export interface NodeTime {
  timezone: string;
  localtime: number;
  time: number;
}

export interface NodeSubscription {
  status: string;
  serverid?: string;
  productname?: string;
  level?: string;
  nextduedate?: string;
}

export interface APTRepository {
  Path: string;
  Number: number;
  FileType: string;
  Enabled: boolean;
  Types: string[];
  URIs: string[];
  Suites: string[];
  Components: string[];
  Comment?: string;
}

export interface APTRepositoryFile {
  path: string;
  'file-type': string;
  repositories: APTRepository[];
}

export interface APTRepositoryInfo {
  files: APTRepositoryFile[];
  digest: string;
}

export interface NodeConfig {
  dns: NodeDNS | null;
  time: NodeTime | null;
  hosts: string;
  network: NetworkInterface[];
  subscription: NodeSubscription | null;
  apt_repos: APTRepositoryInfo | null;
  status: {
    pveversion: string;
    kversion: string;
    cpu_model: string;
    cpu_cores: number;
    cpu_sockets: number;
    boot_mode: string;
    loadavg: string[];
  } | null;
}

// RBAC types

export interface RBACRole {
  id: string;
  name: string;
  description: string;
  builtin: boolean;
  permissions: string[];
  created_at: string;
  updated_at: string;
}

export interface RBACRoleAssignment {
  id: string;
  user_id: string;
  username?: string;
  role_id: string;
  role_name?: string;
  object_type: string;
  object_id: string;
  propagate: boolean;
  created_at: string;
}

export interface CreateRoleRequest {
  name: string;
  description: string;
  permissions: string[];
}

export interface AssignRoleRequest {
  user_id: string;
  role_id: string;
  object_type: string;
  object_id: string;
  propagate: boolean;
}

// Scheduler types

export interface ScheduledTask {
  id: string;
  name: string;
  enabled: boolean;
  task_type: string;
  target_type: string;
  target_id: number;
  cluster: string;
  cron_expr: string;
  params?: string;
  last_run?: string;
  next_run?: string;
  created_at: string;
  updated_at: string;
}

export interface TaskRun {
  id: string;
  task_id: string;
  task_name?: string;
  started_at: string;
  duration_ms: number;
  success: boolean;
  upid?: string;
  error?: string;
}

export interface CreateScheduledTaskRequest {
  name: string;
  task_type: string;
  target_type: string;
  target_id: number;
  cluster: string;
  cron_expr: string;
  params?: string;
  enabled: boolean;
}
