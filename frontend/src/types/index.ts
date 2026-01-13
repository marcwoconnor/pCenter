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
  tags?: string;
  ha_state?: string;
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
  tags?: string;
  ha_state?: string;
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
