import type {
  Node,
  Guest,
  Summary,
  Storage,
  CephStatus,
  GlobalSummary,
  ClusterInfo,
  MigrationProgress,
  DRSRecommendation,
  NetworkInterface,
  SDNZone,
  SDNVNet,
  SDNSubnet,
  NetworkOverview,
  Folder,
  TreeView,
  CreateFolderRequest,
  MoveResourceRequest,
} from '../types';

const BASE_URL = '/api';

async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(error.error || 'API request failed');
  }

  // Handle 204 No Content responses
  if (res.status === 204) {
    return undefined as T;
  }

  return res.json();
}

// GET endpoints
export const api = {
  // Global (all clusters)
  getSummary: () => fetchAPI<Summary>('/summary'),
  getClusters: () => fetchAPI<GlobalSummary>('/clusters'),
  getNodes: () => fetchAPI<Node[]>('/nodes'),
  getGuests: () => fetchAPI<Guest[]>('/guests'),
  getVMs: () => fetchAPI<Guest[]>('/vms'),
  getContainers: () => fetchAPI<Guest[]>('/containers'),
  getStorage: (node?: string) =>
    fetchAPI<Storage[]>(node ? `/storage?node=${node}` : '/storage'),
  getCeph: () => fetchAPI<CephStatus>('/ceph'),
  getMigrations: () => fetchAPI<MigrationProgress[]>('/migrations'),
  getDRSRecommendations: () => fetchAPI<DRSRecommendation[]>('/drs/recommendations'),

  // Cluster-specific
  getClusterSummary: (cluster: string) =>
    fetchAPI<Summary>(`/clusters/${cluster}/summary`),
  getClusterNodes: (cluster: string) =>
    fetchAPI<Node[]>(`/clusters/${cluster}/nodes`),
  getClusterGuests: (cluster: string) =>
    fetchAPI<Guest[]>(`/clusters/${cluster}/guests`),
  getClusterHA: (cluster: string) =>
    fetchAPI<ClusterInfo['ha']>(`/clusters/${cluster}/ha/status`),
  getClusterDRS: (cluster: string) =>
    fetchAPI<DRSRecommendation[]>(`/clusters/${cluster}/drs/recommendations`),

  // Actions (global - searches all clusters)
  vmAction: (vmid: number, action: 'start' | 'stop' | 'shutdown') =>
    fetchAPI<{ upid: string }>(`/vms/${vmid}/${action}`, { method: 'POST' }),
  containerAction: (vmid: number, action: 'start' | 'stop' | 'shutdown') =>
    fetchAPI<{ upid: string }>(`/containers/${vmid}/${action}`, { method: 'POST' }),

  // Actions (cluster-specific)
  clusterVMAction: (cluster: string, vmid: number, action: 'start' | 'stop' | 'shutdown') =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/vms/${vmid}/${action}`, { method: 'POST' }),
  clusterContainerAction: (cluster: string, vmid: number, action: 'start' | 'stop' | 'shutdown') =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/containers/${vmid}/${action}`, { method: 'POST' }),

  // Console
  getConsoleURL: (type: 'vm' | 'ct', vmid: number) =>
    fetchAPI<{ url: string }>(`/console/${type}/${vmid}`),

  // Migration
  migrateVM: (vmid: number, targetNode: string, online: boolean) =>
    fetchAPI<{ upid: string }>(`/vms/${vmid}/migrate`, {
      method: 'POST',
      body: JSON.stringify({ target_node: targetNode, online }),
    }),
  migrateContainer: (vmid: number, targetNode: string, online: boolean) =>
    fetchAPI<{ upid: string }>(`/containers/${vmid}/migrate`, {
      method: 'POST',
      body: JSON.stringify({ target_node: targetNode, online }),
    }),
  clusterMigrateVM: (cluster: string, vmid: number, targetNode: string, online: boolean) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/vms/${vmid}/migrate`, {
      method: 'POST',
      body: JSON.stringify({ target_node: targetNode, online }),
    }),
  clusterMigrateContainer: (cluster: string, vmid: number, targetNode: string, online: boolean) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/containers/${vmid}/migrate`, {
      method: 'POST',
      body: JSON.stringify({ target_node: targetNode, online }),
    }),
  getMigrationTargets: (cluster: string) =>
    fetchAPI<{ name: string; online: boolean }[]>(`/clusters/${cluster}/nodes/migration-targets`),

  // DRS
  applyDRSRecommendation: (cluster: string, id: string) =>
    fetchAPI<{ upid: string; message: string }>(`/clusters/${cluster}/drs/apply/${id}`, { method: 'POST' }),
  dismissDRSRecommendation: (cluster: string, id: string) =>
    fetchAPI<{ message: string }>(`/clusters/${cluster}/drs/recommendations/${id}`, { method: 'DELETE' }),

  // HA Management
  enableHA: (cluster: string, type: 'vm' | 'ct', vmid: number, options?: {
    state?: 'started' | 'stopped';
    group?: string;
    max_restart?: number;
    max_relocate?: number;
    comment?: string;
  }) =>
    fetchAPI<{ message: string }>(`/clusters/${cluster}/ha/${type}/${vmid}/enable`, {
      method: 'POST',
      body: JSON.stringify(options || {}),
    }),
  disableHA: (cluster: string, type: 'vm' | 'ct', vmid: number) =>
    fetchAPI<{ message: string }>(`/clusters/${cluster}/ha/${type}/${vmid}`, { method: 'DELETE' }),
  getHAGroups: (cluster: string) =>
    fetchAPI<{ group: string; comment?: string; nodes?: string }[]>(`/clusters/${cluster}/ha/groups`),

  // Network/SDN
  getClusterNetwork: (cluster: string) =>
    fetchAPI<NetworkOverview>(`/clusters/${cluster}/network`),
  getClusterNetworkInterfaces: (cluster: string, node?: string) =>
    fetchAPI<NetworkInterface[]>(`/clusters/${cluster}/network/interfaces${node ? `?node=${node}` : ''}`),
  getClusterSDNZones: (cluster: string) =>
    fetchAPI<SDNZone[]>(`/clusters/${cluster}/sdn/zones`),
  getClusterSDNVNets: (cluster: string) =>
    fetchAPI<SDNVNet[]>(`/clusters/${cluster}/sdn/vnets`),
  getClusterSDNSubnets: (cluster: string) =>
    fetchAPI<SDNSubnet[]>(`/clusters/${cluster}/sdn/subnets`),

  // Folders
  getFolderTree: (tree: TreeView) =>
    fetchAPI<Folder[]>(`/folders/${tree}`),
  createFolder: (req: CreateFolderRequest) =>
    fetchAPI<Folder>('/folders', { method: 'POST', body: JSON.stringify(req) }),
  renameFolder: (id: string, name: string) =>
    fetchAPI<void>(`/folders/${id}`, { method: 'PUT', body: JSON.stringify({ name }) }),
  deleteFolder: (id: string) =>
    fetchAPI<void>(`/folders/${id}`, { method: 'DELETE' }),
  moveFolder: (id: string, parentId?: string) =>
    fetchAPI<void>(`/folders/${id}/move`, { method: 'POST', body: JSON.stringify({ parent_id: parentId }) }),
  addFolderMember: (folderId: string, resourceType: string, resourceId: string, cluster: string) =>
    fetchAPI<void>(`/folders/${folderId}/members`, {
      method: 'POST',
      body: JSON.stringify({ resource_type: resourceType, resource_id: resourceId, cluster }),
    }),
  removeFolderMember: (folderId: string, resourceType: string, resourceId: string, cluster: string) =>
    fetchAPI<void>(`/folders/${folderId}/members`, {
      method: 'DELETE',
      body: JSON.stringify({ resource_type: resourceType, resource_id: resourceId, cluster }),
    }),
  moveResource: (req: MoveResourceRequest, tree: TreeView) =>
    fetchAPI<void>(`/resources/move?tree=${tree}`, { method: 'POST', body: JSON.stringify(req) }),
};

// Helper functions
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

export function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}

export function formatPercent(value: number, max: number): string {
  if (max === 0) return '0%';
  return `${((value / max) * 100).toFixed(1)}%`;
}
