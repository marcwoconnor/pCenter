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

export interface CephStatus {
  health: {
    status: string;
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
