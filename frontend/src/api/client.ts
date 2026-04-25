import type {
  Node,
  Guest,
  Summary,
  Storage,
  StorageVolume,
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
  VMConfig,
  ContainerConfig,
  ConfigResponse,
  ActivityEntry,
  Datacenter,
  InventoryCluster,
  InventoryHost,
  CreateDatacenterRequest,
  UpdateDatacenterRequest,
  CreateClusterRequest,
  UpdateClusterRequest,
  AddHostRequest,
  UpdateHostRequest,
  DatacenterTreeResponse,
  PveClusterPreflightRequest,
  PveClusterPreflightResponse,
  CreatePveClusterRequest,
  PveClusterJob,
  PveClusterJoinPreflightRequest,
  JoinPveClusterRequest,
  CreateVMRequest,
  CreateContainerRequest,
  Snapshot,
  CreateSnapshotRequest,
  LibraryItem,
  CreateLibraryItemRequest,
  UpdateLibraryItemRequest,
  DeployLibraryItemRequest,
  Tag,
  TagAssignment,
  CreateTagRequest,
  AssignTagRequest,
  DRSRule,
  DRSRuleViolation,
  AlarmInstance,
  AlarmDefinition,
  NotificationChannel,
  NodeConfig,
  ScheduledTask,
  TaskRun,
  CreateScheduledTaskRequest,
  RBACRole,
  RBACRoleAssignment,
  CreateRoleRequest,
  AssignRoleRequest,
  NodeCertificate,
  ACMEAccount,
  ACMEPlugin,
  ACMEDirectory,
  ACMEChallengeSchema,
  NodeACMEDomain,
  CreateACMEAccountRequest,
  CreateACMEPluginRequest,
  Pool,
  PoolDetail,
  CreatePoolRequest,
  UpdatePoolRequest,
  CreateBackupRequest,
  WebhookEndpoint,
  CreateWebhookRequest,
  CreateWebhookResponse,
  UpdateWebhookRequest,
} from '../types';

import { getCSRFToken } from './auth';

const BASE_URL = '/api';

// Debounce 401 redirects: when multiple concurrent requests all get 401,
// only the first one triggers the redirect. Without this, N concurrent fetches
// returning 401 cause N rapid window.location.href assignments.
let redirecting = false;

const DEFAULT_TIMEOUT_MS = 30000; // 30s fetch timeout

async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string>),
  };

  // Add CSRF token for state-changing requests
  const csrfToken = getCSRFToken();
  if (
    csrfToken &&
    options?.method &&
    ['POST', 'PUT', 'DELETE'].includes(options.method)
  ) {
    headers['X-CSRF-Token'] = csrfToken;
  }

  // Add timeout via AbortSignal
  const timeoutSignal = AbortSignal.timeout(DEFAULT_TIMEOUT_MS);
  const signal = options?.signal
    ? AbortSignal.any([options.signal, timeoutSignal])
    : timeoutSignal;

  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
    signal,
    credentials: 'include', // Include cookies for session
  });

  // Handle 401 - notify auth context to redirect (preserves React state)
  if (res.status === 401) {
    if (!redirecting && !window.location.pathname.includes('/login')) {
      redirecting = true;
      // Dispatch custom event so AuthContext can handle via React Router
      window.dispatchEvent(new CustomEvent('auth:session-expired'));
      // Reset after short delay to allow re-trigger if needed
      setTimeout(() => { redirecting = false; }, 2000);
    }
    throw new Error('Session expired');
  }

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
  getStorageContent: (storage: string, node?: string) =>
    fetchAPI<StorageVolume[]>(node ? `/storage/${storage}/content?node=${node}` : `/storage/${storage}/content`),
  getCeph: () => fetchAPI<CephStatus>('/ceph'),
  getMigrations: () => fetchAPI<MigrationProgress[]>('/migrations'),
  getDRSRecommendations: () => fetchAPI<DRSRecommendation[]>('/drs/recommendations'),

  // Cluster-specific
  getClusterSummary: (cluster: string) =>
    fetchAPI<Summary>(`/clusters/${cluster}/summary`),
  getClusterNodes: (cluster: string) =>
    fetchAPI<Node[]>(`/clusters/${cluster}/nodes`),
  getNodeConfig: (cluster: string, node: string) =>
    fetchAPI<NodeConfig>(`/clusters/${cluster}/nodes/${node}/config`),
  updateNodeDNS: (cluster: string, node: string, dns: { search: string; dns1: string; dns2?: string; dns3?: string }) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/dns`, { method: 'PUT', body: JSON.stringify(dns) }),
  updateNodeTimezone: (cluster: string, node: string, timezone: string) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/time`, { method: 'PUT', body: JSON.stringify({ timezone }) }),
  updateNodeHosts: (cluster: string, node: string, data: string, digest: string) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/hosts`, { method: 'PUT', body: JSON.stringify({ data, digest }) }),
  createNodeNetworkInterface: (cluster: string, node: string, params: Record<string, string>) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/network`, { method: 'POST', body: JSON.stringify(params) }),
  updateNodeNetworkInterface: (cluster: string, node: string, iface: string, params: Record<string, string>) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/network/${iface}`, { method: 'PUT', body: JSON.stringify(params) }),
  deleteNodeNetworkInterface: (cluster: string, node: string, iface: string) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/network/${iface}`, { method: 'DELETE' }),
  applyNodeNetwork: (cluster: string, node: string) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/network-apply`, { method: 'POST' }),
  revertNodeNetwork: (cluster: string, node: string) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/network-revert`, { method: 'POST' }),
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

  // Configuration (cluster-specific)
  getVMConfig: (cluster: string, vmid: number) =>
    fetchAPI<ConfigResponse<VMConfig>>(`/clusters/${cluster}/vms/${vmid}/config`),
  getContainerConfig: (cluster: string, vmid: number) =>
    fetchAPI<ConfigResponse<ContainerConfig>>(`/clusters/${cluster}/containers/${vmid}/config`),
  updateVMConfig: (cluster: string, vmid: number, digest: string, changes: Record<string, unknown>, deleteKeys?: string[]) =>
    fetchAPI<{ message: string }>(`/clusters/${cluster}/vms/${vmid}/config`, {
      method: 'PUT',
      body: JSON.stringify({ digest, changes, delete: deleteKeys }),
    }),
  updateContainerConfig: (cluster: string, vmid: number, digest: string, changes: Record<string, unknown>, deleteKeys?: string[]) =>
    fetchAPI<{ message: string }>(`/clusters/${cluster}/containers/${vmid}/config`, {
      method: 'PUT',
      body: JSON.stringify({ digest, changes, delete: deleteKeys }),
    }),

  // VM Snapshots
  getVMSnapshots: (cluster: string, vmid: number) =>
    fetchAPI<Snapshot[]>(`/clusters/${cluster}/vms/${vmid}/snapshots`),
  createVMSnapshot: (cluster: string, vmid: number, req: CreateSnapshotRequest) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/vms/${vmid}/snapshots`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  rollbackVMSnapshot: (cluster: string, vmid: number, snapname: string) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/vms/${vmid}/snapshots/${encodeURIComponent(snapname)}/rollback`, {
      method: 'POST',
    }),
  deleteVMSnapshot: (cluster: string, vmid: number, snapname: string) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/vms/${vmid}/snapshots/${encodeURIComponent(snapname)}`, {
      method: 'DELETE',
    }),

  // Container Snapshots
  getContainerSnapshots: (cluster: string, vmid: number) =>
    fetchAPI<Snapshot[]>(`/clusters/${cluster}/containers/${vmid}/snapshots`),
  createContainerSnapshot: (cluster: string, vmid: number, req: CreateSnapshotRequest) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/containers/${vmid}/snapshots`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  rollbackContainerSnapshot: (cluster: string, vmid: number, snapname: string) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/containers/${vmid}/snapshots/${encodeURIComponent(snapname)}/rollback`, {
      method: 'POST',
    }),
  deleteContainerSnapshot: (cluster: string, vmid: number, snapname: string) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/containers/${vmid}/snapshots/${encodeURIComponent(snapname)}`, {
      method: 'DELETE',
    }),

  // Create VM/Container
  getNextVMID: (cluster: string) =>
    fetchAPI<{ vmid: number }>(`/clusters/${cluster}/nextid`),
  createVM: (cluster: string, node: string, req: CreateVMRequest) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/nodes/${node}/vms`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  createContainer: (cluster: string, node: string, req: CreateContainerRequest) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/nodes/${node}/containers`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  // Delete VM/Container
  deleteVM: (cluster: string, vmid: number, purge?: boolean) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/vms/${vmid}${purge ? '?purge=1' : ''}`, {
      method: 'DELETE',
    }),
  deleteContainer: (cluster: string, vmid: number, purge?: boolean) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/containers/${vmid}${purge ? '?purge=1' : ''}`, {
      method: 'DELETE',
    }),

  // Task status
  getTaskStatus: (cluster: string, upid: string) =>
    fetchAPI<{
      upid: string;
      node: string;
      status: string;  // "running" | "stopped"
      exitstatus?: string;  // "OK" | error message
      type: string;
      id: string;
    }>(`/clusters/${cluster}/tasks/${encodeURIComponent(upid)}`),

  // Clone VM/Container
  cloneVM: (cluster: string, vmid: number, opts: {
    new_id: number;
    name?: string;
    target_node?: string;
    full?: boolean;
    storage?: string;
    description?: string;
  }) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/vms/${vmid}/clone`, {
      method: 'POST',
      body: JSON.stringify(opts),
    }),
  cloneContainer: (cluster: string, vmid: number, opts: {
    new_id: number;
    name?: string;
    target_node?: string;
    full?: boolean;
    storage?: string;
    description?: string;
  }) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/containers/${vmid}/clone`, {
      method: 'POST',
      body: JSON.stringify(opts),
    }),

  // Convert VM/Container to Template (Proxmox native)
  convertVMToTemplate: (cluster: string, vmid: number) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/vms/${vmid}/template`, {
      method: 'POST',
    }),
  convertContainerToTemplate: (cluster: string, vmid: number) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/containers/${vmid}/template`, {
      method: 'POST',
    }),

  // ACME / certificates
  getNodeCertificates: (cluster: string, node: string) =>
    fetchAPI<NodeCertificate[]>(`/clusters/${cluster}/nodes/${node}/certificates`),
  renewNodeACMECertificate: (cluster: string, node: string) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/nodes/${node}/certificates/acme/renew`, {
      method: 'POST',
    }),
  uploadNodeCustomCertificate: (cluster: string, node: string, req: { certificates: string; key?: string; force?: boolean; restart?: boolean }) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/certificates/custom`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  deleteNodeCustomCertificate: (cluster: string, node: string, restart = true) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/certificates/custom${restart ? '?restart=1' : ''}`, {
      method: 'DELETE',
    }),
  listACMEAccounts: (cluster: string) =>
    fetchAPI<ACMEAccount[]>(`/clusters/${cluster}/acme/accounts`),
  listACMEPlugins: (cluster: string) =>
    fetchAPI<ACMEPlugin[]>(`/clusters/${cluster}/acme/plugins`),
  listACMEDirectories: (cluster: string) =>
    fetchAPI<ACMEDirectory[]>(`/clusters/${cluster}/acme/directories`),
  getACMETOSURL: (cluster: string, directory: string) =>
    fetchAPI<{ tos: string }>(`/clusters/${cluster}/acme/tos?directory=${encodeURIComponent(directory)}`),
  listACMEChallengeSchemas: (cluster: string) =>
    fetchAPI<ACMEChallengeSchema[]>(`/clusters/${cluster}/acme/challenge-schema`),

  createACMEAccount: (cluster: string, req: CreateACMEAccountRequest) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/acme/accounts`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  updateACMEAccount: (cluster: string, name: string, contact: string) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/acme/accounts/${encodeURIComponent(name)}`, {
      method: 'PUT',
      body: JSON.stringify({ contact }),
    }),
  deleteACMEAccount: (cluster: string, name: string) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/acme/accounts/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),

  createACMEPlugin: (cluster: string, req: CreateACMEPluginRequest) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/acme/plugins`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  updateACMEPlugin: (cluster: string, id: string, data: Record<string, string>) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/acme/plugins/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify({ data }),
    }),
  deleteACMEPlugin: (cluster: string, id: string) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/acme/plugins/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  getNodeACMEDomains: (cluster: string, node: string) =>
    fetchAPI<NodeACMEDomain[]>(`/clusters/${cluster}/nodes/${node}/acme/domains`),
  setNodeACMEDomains: (cluster: string, node: string, domains: NodeACMEDomain[]) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/nodes/${node}/acme/domains`, {
      method: 'PUT',
      body: JSON.stringify({ domains }),
    }),

  // Resource pools
  listPools: (cluster: string) =>
    fetchAPI<Pool[]>(`/clusters/${cluster}/pools`),
  getPool: (cluster: string, poolID: string) =>
    fetchAPI<PoolDetail>(`/clusters/${cluster}/pools/${encodeURIComponent(poolID)}`),
  createPool: (cluster: string, req: CreatePoolRequest) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/pools`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  updatePool: (cluster: string, poolID: string, req: UpdatePoolRequest) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/pools/${encodeURIComponent(poolID)}`, {
      method: 'PUT',
      body: JSON.stringify(req),
    }),
  deletePool: (cluster: string, poolID: string) =>
    fetchAPI<{ status: string }>(`/clusters/${cluster}/pools/${encodeURIComponent(poolID)}`, {
      method: 'DELETE',
    }),

  // Backup (vzdump)
  createBackup: (cluster: string, node: string, req: CreateBackupRequest) =>
    fetchAPI<{ upid: string }>(`/clusters/${cluster}/nodes/${node}/backup`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),

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

  // Activity
  getActivity: (params?: { limit?: number; offset?: number; resource_type?: string; resource_id?: string; cluster?: string; action?: string }) => {
    const query = new URLSearchParams();
    if (params?.limit) query.set('limit', params.limit.toString());
    if (params?.offset) query.set('offset', params.offset.toString());
    if (params?.resource_type) query.set('resource_type', params.resource_type);
    if (params?.resource_id) query.set('resource_id', params.resource_id);
    if (params?.cluster) query.set('cluster', params.cluster);
    if (params?.action) query.set('action', params.action);
    const qs = query.toString();
    return fetchAPI<ActivityEntry[]>(`/activity${qs ? `?${qs}` : ''}`);
  },

  // Tags
  getTags: () => fetchAPI<Tag[]>('/tags'),
  getTagCategories: () => fetchAPI<string[]>('/tags/categories'),
  createTag: (req: CreateTagRequest) =>
    fetchAPI<Tag>('/tags', { method: 'POST', body: JSON.stringify(req) }),
  updateTag: (id: string, req: Partial<CreateTagRequest>) =>
    fetchAPI<void>(`/tags/${id}`, { method: 'PUT', body: JSON.stringify(req) }),
  deleteTag: (id: string) =>
    fetchAPI<void>(`/tags/${id}`, { method: 'DELETE' }),
  getTagAssignments: () => fetchAPI<TagAssignment[]>('/tags/assignments'),
  assignTag: (req: AssignTagRequest) =>
    fetchAPI<void>('/tags/assign', { method: 'POST', body: JSON.stringify(req) }),
  unassignTag: (req: AssignTagRequest) =>
    fetchAPI<void>('/tags/assign', { method: 'DELETE', body: JSON.stringify(req) }),
  bulkAssignTags: (tagIds: string[], objects: Array<{ object_type: string; object_id: string; cluster: string }>) =>
    fetchAPI<void>('/tags/bulk-assign', { method: 'POST', body: JSON.stringify({ tag_ids: tagIds, objects }) }),

  // DRS Rules
  getDRSRules: (cluster: string) =>
    fetchAPI<DRSRule[]>(`/clusters/${cluster}/drs/rules`),
  createDRSRule: (cluster: string, req: { name: string; type: string; members: number[]; host_node?: string }) =>
    fetchAPI<DRSRule>(`/clusters/${cluster}/drs/rules`, { method: 'POST', body: JSON.stringify(req) }),
  updateDRSRule: (cluster: string, id: string, rule: Partial<DRSRule>) =>
    fetchAPI<void>(`/clusters/${cluster}/drs/rules/${id}`, { method: 'PUT', body: JSON.stringify(rule) }),
  deleteDRSRule: (cluster: string, id: string) =>
    fetchAPI<void>(`/clusters/${cluster}/drs/rules/${id}`, { method: 'DELETE' }),
  getDRSViolations: (cluster: string) =>
    fetchAPI<DRSRuleViolation[]>(`/clusters/${cluster}/drs/violations`),

  // Alarms
  getActiveAlarms: () => fetchAPI<AlarmInstance[]>('/alarms'),
  getAlarmDefinitions: () => fetchAPI<AlarmDefinition[]>('/alarms/definitions'),
  createAlarmDefinition: (req: Partial<AlarmDefinition>) =>
    fetchAPI<AlarmDefinition>('/alarms/definitions', { method: 'POST', body: JSON.stringify(req) }),
  updateAlarmDefinition: (id: string, req: Partial<AlarmDefinition>) =>
    fetchAPI<void>(`/alarms/definitions/${id}`, { method: 'PUT', body: JSON.stringify(req) }),
  deleteAlarmDefinition: (id: string) =>
    fetchAPI<void>(`/alarms/definitions/${id}`, { method: 'DELETE' }),
  acknowledgeAlarm: (id: string, user: string) =>
    fetchAPI<void>(`/alarms/${id}/acknowledge`, { method: 'POST', body: JSON.stringify({ user }) }),
  getAlarmHistory: (limit?: number) =>
    fetchAPI<Array<Record<string, unknown>>>(`/alarms/history${limit ? `?limit=${limit}` : ''}`),
  getAlarmChannels: () => fetchAPI<NotificationChannel[]>('/alarms/channels'),
  createAlarmChannel: (req: { name: string; type: string; config: string }) =>
    fetchAPI<NotificationChannel>('/alarms/channels', { method: 'POST', body: JSON.stringify(req) }),
  deleteAlarmChannel: (id: string) =>
    fetchAPI<void>(`/alarms/channels/${id}`, { method: 'DELETE' }),
  testAlarmChannel: (id: string) =>
    fetchAPI<void>(`/alarms/channels/${id}/test`, { method: 'POST' }),

  // Datacenters
  getDatacenters: () => fetchAPI<Datacenter[]>('/datacenters'),
  getDatacenter: (id: string) => fetchAPI<Datacenter>(`/datacenters/${id}`),
  createDatacenter: (req: CreateDatacenterRequest) =>
    fetchAPI<Datacenter>('/datacenters', { method: 'POST', body: JSON.stringify(req) }),
  updateDatacenter: (id: string, req: UpdateDatacenterRequest) =>
    fetchAPI<void>(`/datacenters/${id}`, { method: 'PUT', body: JSON.stringify(req) }),
  deleteDatacenter: (id: string) =>
    fetchAPI<void>(`/datacenters/${id}`, { method: 'DELETE' }),
  getDatacenterTree: () => fetchAPI<DatacenterTreeResponse>('/datacenters/tree'),
  addDatacenterHost: (datacenterID: string, req: AddHostRequest) =>
    fetchAPI<InventoryHost>(`/datacenters/${datacenterID}/hosts`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  // Inventory Clusters (configuration)
  getInventoryClusters: () => fetchAPI<InventoryCluster[]>('/inventory/clusters'),
  getInventoryCluster: (name: string) => fetchAPI<InventoryCluster>(`/inventory/clusters/${name}`),
  createInventoryCluster: (req: CreateClusterRequest) =>
    fetchAPI<InventoryCluster>('/inventory/clusters', { method: 'POST', body: JSON.stringify(req) }),
  updateInventoryCluster: (name: string, req: UpdateClusterRequest) =>
    fetchAPI<void>(`/inventory/clusters/${name}`, { method: 'PUT', body: JSON.stringify(req) }),
  deleteInventoryCluster: (name: string) =>
    fetchAPI<void>(`/inventory/clusters/${name}`, { method: 'DELETE' }),
  moveClusterToDatacenter: (name: string, datacenterId?: string) =>
    fetchAPI<void>(`/inventory/clusters/${name}/move`, {
      method: 'POST',
      body: JSON.stringify({ datacenter_id: datacenterId }),
    }),

  // Inventory Hosts (per-cluster)
  getClusterHosts: (clusterName: string) =>
    fetchAPI<InventoryHost[]>(`/inventory/clusters/${clusterName}/hosts`),
  addClusterHost: (clusterName: string, req: AddHostRequest) =>
    fetchAPI<InventoryHost>(`/inventory/clusters/${clusterName}/hosts`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  getHost: (id: string) => fetchAPI<InventoryHost>(`/inventory/hosts/${id}`),
  updateHost: (id: string, req: UpdateHostRequest) =>
    fetchAPI<void>(`/inventory/hosts/${id}`, { method: 'PUT', body: JSON.stringify(req) }),
  deleteHost: (id: string) =>
    fetchAPI<void>(`/inventory/hosts/${id}`, { method: 'DELETE' }),
  moveHostToCluster: (hostId: string, clusterId: string) =>
    fetchAPI<{ status: string }>(`/inventory/hosts/${hostId}/move`, { method: 'POST', body: JSON.stringify({ cluster_id: clusterId }) }),

  // PVE cluster formation — turn 2+ standalone hosts into a real Corosync cluster.
  preflightPveCluster: (req: PveClusterPreflightRequest) =>
    fetchAPI<PveClusterPreflightResponse>('/inventory/pve-cluster/preflight', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  createPveCluster: (req: CreatePveClusterRequest) =>
    fetchAPI<{ job_id: string }>('/inventory/pve-cluster', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  getPveClusterJob: (jobId: string) =>
    fetchAPI<PveClusterJob>(`/inventory/pve-cluster-jobs/${jobId}`),

  // Add new member nodes to an already-existing PVE cluster.
  preflightJoinPveCluster: (req: PveClusterJoinPreflightRequest) =>
    fetchAPI<PveClusterPreflightResponse>('/inventory/pve-cluster/join/preflight', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  joinPveCluster: (req: JoinPveClusterRequest) =>
    fetchAPI<{ job_id: string }>('/inventory/pve-cluster/join', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  // Host setup actions
  setupHostSSH: (id: string, sshPassword: string) =>
    fetchAPI<{ success: boolean; message: string }>(`/inventory/hosts/${id}/setup-ssh`, {
      method: 'POST',
      body: JSON.stringify({ ssh_password: sshPassword }),
    }),
  deployAgent: (id: string) =>
    fetchAPI<{ success: boolean; message: string; token_secret?: string }>(`/inventory/hosts/${id}/deploy-agent`, {
      method: 'POST',
    }),

  // Content Library
  getLibraryItems: (params?: { type?: string; category?: string; cluster?: string; search?: string; tag?: string }) => {
    const query = new URLSearchParams();
    if (params?.type) query.set('type', params.type);
    if (params?.category) query.set('category', params.category);
    if (params?.cluster) query.set('cluster', params.cluster);
    if (params?.search) query.set('search', params.search);
    if (params?.tag) query.set('tag', params.tag);
    const qs = query.toString();
    return fetchAPI<LibraryItem[]>(`/library${qs ? `?${qs}` : ''}`);
  },
  getLibraryItem: (id: string) =>
    fetchAPI<LibraryItem>(`/library/${id}`),
  getLibraryCategories: () =>
    fetchAPI<string[]>('/library/categories'),
  createLibraryItem: (req: CreateLibraryItemRequest) =>
    fetchAPI<LibraryItem>('/library', { method: 'POST', body: JSON.stringify(req) }),
  updateLibraryItem: (id: string, req: UpdateLibraryItemRequest) =>
    fetchAPI<{ message: string }>(`/library/${id}`, { method: 'PUT', body: JSON.stringify(req) }),
  deleteLibraryItem: (id: string) =>
    fetchAPI<{ message: string }>(`/library/${id}`, { method: 'DELETE' }),
  deployLibraryItem: (id: string, req: DeployLibraryItemRequest) =>
    fetchAPI<{ upid: string; message: string }>(`/library/${id}/deploy`, { method: 'POST', body: JSON.stringify(req) }),

  // Scheduler
  getScheduledTasks: () =>
    fetchAPI<ScheduledTask[]>('/scheduler/tasks'),
  createScheduledTask: (req: CreateScheduledTaskRequest) =>
    fetchAPI<ScheduledTask>('/scheduler/tasks', { method: 'POST', body: JSON.stringify(req) }),
  updateScheduledTask: (id: string, req: { name: string; cron_expr: string; params?: string; enabled: boolean }) =>
    fetchAPI<{ status: string }>(`/scheduler/tasks/${id}`, { method: 'PUT', body: JSON.stringify(req) }),
  deleteScheduledTask: (id: string) =>
    fetchAPI<{ status: string }>(`/scheduler/tasks/${id}`, { method: 'DELETE' }),
  getTaskRuns: (taskId?: string, limit?: number) => {
    const qs = new URLSearchParams();
    if (taskId) qs.set('task_id', taskId);
    if (limit) qs.set('limit', String(limit));
    const q = qs.toString();
    return fetchAPI<TaskRun[]>(`/scheduler/runs${q ? '?' + q : ''}`);
  },

  // Version/update check
  getVersion: () =>
    fetchAPI<any>('/version'),

  // RBAC
  getRoles: () =>
    fetchAPI<RBACRole[]>('/rbac/roles'),
  createRole: (req: CreateRoleRequest) =>
    fetchAPI<RBACRole>('/rbac/roles', { method: 'POST', body: JSON.stringify(req) }),
  updateRole: (id: string, req: CreateRoleRequest) =>
    fetchAPI<{ status: string }>(`/rbac/roles/${id}`, { method: 'PUT', body: JSON.stringify(req) }),
  deleteRole: (id: string) =>
    fetchAPI<{ status: string }>(`/rbac/roles/${id}`, { method: 'DELETE' }),
  getRoleAssignments: (params?: { user_id?: string; object_type?: string; object_id?: string }) => {
    const qs = new URLSearchParams();
    if (params?.user_id) qs.set('user_id', params.user_id);
    if (params?.object_type) qs.set('object_type', params.object_type);
    if (params?.object_id) qs.set('object_id', params.object_id);
    const q = qs.toString();
    return fetchAPI<RBACRoleAssignment[]>(`/rbac/assignments${q ? '?' + q : ''}`);
  },
  createRoleAssignment: (req: AssignRoleRequest) =>
    fetchAPI<RBACRoleAssignment>('/rbac/assignments', { method: 'POST', body: JSON.stringify(req) }),
  deleteRoleAssignment: (id: string) =>
    fetchAPI<{ status: string }>(`/rbac/assignments/${id}`, { method: 'DELETE' }),
  getMyPermissions: (objectType?: string, objectId?: string) => {
    const qs = new URLSearchParams();
    if (objectType) qs.set('object_type', objectType);
    if (objectId) qs.set('object_id', objectId);
    const q = qs.toString();
    return fetchAPI<string[]>(`/rbac/my-permissions${q ? '?' + q : ''}`);
  },
  getAllPermissions: () =>
    fetchAPI<string[]>('/rbac/permissions'),

  // Webhooks (admin)
  listWebhooks: () =>
    fetchAPI<WebhookEndpoint[]>('/webhooks'),
  createWebhook: (req: CreateWebhookRequest) =>
    fetchAPI<CreateWebhookResponse>('/webhooks', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  updateWebhook: (id: string, req: UpdateWebhookRequest) =>
    fetchAPI<WebhookEndpoint>(`/webhooks/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(req),
    }),
  deleteWebhook: (id: string) =>
    fetchAPI<void>(`/webhooks/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),
  testWebhook: (id: string) =>
    fetchAPI<{ status: string }>(`/webhooks/${encodeURIComponent(id)}/test`, {
      method: 'POST',
    }),
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
