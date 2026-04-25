import { useState, useMemo, useEffect, useCallback, memo } from 'react';
import { useCluster } from '../context/ClusterContext';
import { useFolders } from '../context/FolderContext';
import type { SelectedObject } from '../context/ClusterContext';
import type { Guest, Storage, ClusterInfo, NetworkInterface, Folder, TreeView, Datacenter, InventoryCluster, InventoryHost } from '../types';
import { ContextMenu, type MenuItem } from './ContextMenu';
import { MigrateDialog } from './MigrateDialog';
import { CloneDialog } from './CloneDialog';
import { BackupDialog } from './BackupDialog';
import { FolderDialog } from './FolderDialog';
import { DatacenterDialog } from './DatacenterDialog';
import { AddHostDialog } from './AddHostDialog';
import { CreateProxmoxClusterDialog } from './CreateProxmoxClusterDialog';
import { JoinPveClusterDialog } from './JoinPveClusterDialog';
import { UploadDialog } from './UploadDialog';
import { CreateVMDialog } from './CreateVMDialog';
import { CreateContainerDialog } from './CreateContainerDialog';
import { api } from '../api/client';

interface ContextMenuState {
  x: number;
  y: number;
  items: MenuItem[];
}

// Drag data format
interface DragData {
  type: 'resource' | 'folder';
  resourceType?: 'vm' | 'ct' | 'node' | 'storage';
  resourceId?: string;
  cluster?: string;
  folderId?: string;
}

interface TreeNodeProps {
  icon: string;
  iconTitle?: string;
  label: string;
  status?: 'online' | 'running' | 'stopped' | 'warning';
  badge?: React.ReactNode;
  isSelected?: boolean;
  onClick?: () => void;
  onContextMenu?: (e: React.MouseEvent) => void;
  children?: React.ReactNode;
  defaultExpanded?: boolean;
  count?: number;
  // Drag-and-drop props
  draggable?: boolean;
  dragData?: DragData;
  droppable?: boolean;
  onDrop?: (data: DragData) => void;
}

function TreeNode({
  icon,
  iconTitle,
  label,
  status,
  badge,
  isSelected,
  onClick,
  onContextMenu,
  children,
  defaultExpanded = false,
  count,
  draggable = false,
  dragData,
  droppable = false,
  onDrop,
}: TreeNodeProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [isDragOver, setIsDragOver] = useState(false);
  const hasChildren = Boolean(children);

  const statusColors = {
    online: 'bg-green-500',
    running: 'bg-green-500',
    stopped: 'bg-gray-400',
    warning: 'bg-yellow-500',
  };

  const handleDragStart = (e: React.DragEvent) => {
    if (!draggable || !dragData) return;
    e.dataTransfer.setData('application/json', JSON.stringify(dragData));
    e.dataTransfer.effectAllowed = 'move';
  };

  const handleDragOver = (e: React.DragEvent) => {
    if (!droppable) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    setIsDragOver(true);
  };

  const handleDragLeave = () => {
    setIsDragOver(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    if (!droppable || !onDrop) return;
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
    try {
      const data = JSON.parse(e.dataTransfer.getData('application/json')) as DragData;
      onDrop(data);
    } catch {
      // Invalid drag data
    }
  };

  return (
    <div>
      <div
        draggable={draggable}
        onDragStart={handleDragStart}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => {
          onClick?.();
        }}
        onContextMenu={(e) => {
          if (onContextMenu) {
            e.preventDefault();
            e.stopPropagation();
            onContextMenu(e);
          }
        }}
        className={`flex items-center gap-2 px-2 py-1.5 cursor-pointer text-sm ${
          isDragOver
            ? 'bg-blue-200 dark:bg-blue-800 border-2 border-blue-400 border-dashed'
            : isSelected
              ? 'bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200'
              : 'hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300'
        }`}
      >
        {hasChildren ? (
          <span className="w-4 text-center text-gray-400 hover:text-white cursor-pointer" onClick={(e) => { e.stopPropagation(); setExpanded(!expanded); }}>{expanded ? '▼' : '▶'}</span>
        ) : (
          <span className="w-4" />
        )}
        <span title={iconTitle}>{icon}</span>
        {status && <div className={`w-2 h-2 rounded-full ${statusColors[status]}`} />}
        <span className="flex-1 truncate">{label}</span>
        {badge}
        {count !== undefined && (
          <span className="text-xs text-gray-400">{count}</span>
        )}
      </div>
      {expanded && children && (
        <div className="ml-4">{children}</div>
      )}
    </div>
  );
}

// HA Status Badge component
//
// A 1-node cluster has Proxmox's HA service running and quorate (1 vote =
// majority of 1), so the raw flags would proudly say "OK" — but failover is
// physically impossible without a second node. Hide the badge entirely in
// that case rather than display a misleading green.
function HABadge({ ha, nodeCount }: { ha?: ClusterInfo['ha']; nodeCount?: number }) {
  if (!ha?.enabled) return null;
  if (nodeCount !== undefined && nodeCount < 2) return null;

  const isOk = ha.quorum;
  return (
    <span className={`text-xs px-1.5 py-0.5 rounded ${
      isOk
        ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
        : 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300'
    }`}>
      {isOk ? 'HA: OK' : 'HA: No Quorum'}
    </span>
  );
}

interface InventoryTreeProps {
  view: 'hosts' | 'vms' | 'storage' | 'network';
  filter?: string;
}

// Folder dialog state
interface FolderDialogState {
  mode: 'create' | 'rename';
  parentId?: string;
  folderId?: string;
  initialName?: string;
  treeView: TreeView;
}

// Datacenter dialog state
interface DatacenterDialogState {
  mode: 'create-dc' | 'edit-dc' | 'create-cluster' | 'edit-cluster';
  datacenter?: Datacenter;
  cluster?: InventoryCluster;
  parentDatacenterId?: string;
}

// SSH Setup Dialog - prompts for root password to deploy SSH key
function SSHSetupDialog({ hostAddress, onSubmit, onClose }: {
  hostAddress: string;
  onSubmit: (password: string) => Promise<void>;
  onClose: () => void;
}) {
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!password.trim()) return;
    setLoading(true);
    try {
      await onSubmit(password);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-gray-800 rounded-lg p-6 w-96 shadow-xl" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-semibold mb-4">Setup SSH Key</h2>
        <p className="text-sm text-gray-400 mb-4">
          Deploy pCenter's SSH key to <span className="text-blue-400">{hostAddress}</span>
        </p>
        <form onSubmit={handleSubmit}>
          <label className="block text-sm text-gray-400 mb-1">Root Password</label>
          <input
            type="password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white mb-4"
            placeholder="Enter root password"
            autoFocus
            disabled={loading}
          />
          <div className="flex gap-2 justify-end">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 bg-gray-700 hover:bg-gray-600 rounded"
              disabled={loading}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="px-4 py-2 bg-blue-600 hover:bg-blue-500 rounded disabled:opacity-50"
              disabled={!password.trim() || loading}
            >
              {loading ? 'Setting up...' : 'Setup SSH'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export const InventoryTree = memo(function InventoryTree({ view, filter = '' }: InventoryTreeProps) {
  const { clusters, nodes, guests, storage, selectedObject, setSelectedObject, performAction, openConsole } = useCluster();
  const { hostsTree, vmsTree, createFolder, renameFolder, deleteFolder, moveFolder, moveResource } = useFolders();
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [migrateGuest, setMigrateGuest] = useState<Guest | null>(null);
  const [cloneGuest, setCloneGuest] = useState<Guest | null>(null);
  const [backupGuest, setBackupGuest] = useState<Guest | null>(null);
  const [networkInterfaces, setNetworkInterfaces] = useState<NetworkInterface[]>([]);
  const [folderDialog, setFolderDialog] = useState<FolderDialogState | null>(null);

  // Datacenter/cluster inventory state
  const [datacenters, setDatacenters] = useState<Datacenter[]>([]);
  const [orphanClusters, setOrphanClusters] = useState<InventoryCluster[]>([]);
  const [datacenterDialog, setDatacenterDialog] = useState<DatacenterDialogState | null>(null);
  const [addHostDialog, setAddHostDialog] = useState<{
    mode: 'cluster' | 'datacenter';
    clusterName?: string;
    datacenterId?: string;
    datacenterName?: string;
  } | null>(null);
  const [uploadDialog, setUploadDialog] = useState<{ storage: string; node: string; contentType: 'iso' | 'vztmpl' } | null>(null);
  const [createVMDialog, setCreateVMDialog] = useState<{ cluster: string; node: string } | null>(null);
  const [createContainerDialog, setCreateContainerDialog] = useState<{ cluster: string; node: string } | null>(null);
  const [sshSetupDialog, setSshSetupDialog] = useState<{ hostId: string; hostAddress: string } | null>(null);
  const [deployingAgent, setDeployingAgent] = useState<string | null>(null); // hostId being deployed
  // Open state for the Create Proxmox Cluster wizard, scoped to a datacenter.
  const [createPveClusterDialog, setCreatePveClusterDialog] = useState<Datacenter | null>(null);
  // Open state for the Add Member Node wizard, scoped to an existing cluster.
  const [joinPveClusterDialog, setJoinPveClusterDialog] = useState<InventoryCluster | null>(null);
  // When true, the next successfully-created datacenter auto-chains into the Add Host dialog.
  // Used by the "Add Host" root-menu item and the top banner when no datacenter exists yet.
  const [chainAddHostAfterDC, setChainAddHostAfterDC] = useState(false);

  const filterLower = filter.toLowerCase();

  // Fetch datacenter tree for hosts view
  const fetchDatacenterTree = useCallback(async () => {
    try {
      const tree = await api.getDatacenterTree();
      setDatacenters(tree.datacenters || []);
      setOrphanClusters(tree.orphan_clusters || []);
    } catch (e) {
      console.error('Failed to fetch datacenter tree:', e);
    }
  }, []);

  useEffect(() => {
    if (view === 'hosts') {
      fetchDatacenterTree();
    }
  }, [view, fetchDatacenterTree]);

  // Unified "Add Host" entry point — used by the root context menu and the top banner.
  // Fresh install (0 datacenters): create a datacenter first, then chain into Add Host.
  // Otherwise: open Add Host scoped to the first datacenter (user can right-click a specific one to target).
  const handleAddHostIntent = useCallback(() => {
    if (datacenters.length === 0) {
      setChainAddHostAfterDC(true);
      setDatacenterDialog({ mode: 'create-dc' });
    } else {
      const dc = datacenters[0];
      setAddHostDialog({ mode: 'datacenter', datacenterId: dc.id, datacenterName: dc.name });
    }
  }, [datacenters]);

  // Let non-tree UI (e.g. the top banner in Layout) trigger the Add Host flow.
  useEffect(() => {
    if (view !== 'hosts') return;
    const handler = () => handleAddHostIntent();
    window.addEventListener('pcenter:add-host', handler);
    return () => window.removeEventListener('pcenter:add-host', handler);
  }, [view, handleAddHostIntent]);

  // Collect all resource IDs that are in folders (to exclude from default lists)
  const collectFolderMembers = (folders: Folder[]): Set<string> => {
    const members = new Set<string>();
    const traverse = (folderList: Folder[]) => {
      for (const folder of folderList) {
        if (folder.members) {
          for (const m of folder.members) {
            // Key format: "type:id:cluster"
            members.add(`${m.resource_type}:${m.resource_id}:${m.cluster}`);
          }
        }
        if (folder.children) {
          traverse(folder.children);
        }
      }
    };
    traverse(folders);
    return members;
  };

  const hostsFolderMembers = useMemo(() => collectFolderMembers(hostsTree), [hostsTree]);
  const vmsFolderMembers = useMemo(() => collectFolderMembers(vmsTree), [vmsTree]);

  // Check if a guest is in a folder
  const isGuestInFolder = (guest: Guest, tree: TreeView): boolean => {
    const members = tree === 'hosts' ? hostsFolderMembers : vmsFolderMembers;
    const type = guest.type === 'qemu' ? 'vm' : 'ct';
    return members.has(`${type}:${guest.vmid}:${guest.cluster}`);
  };

  // Derive stable cluster names for network fetch
  const clusterNamesKey = useMemo(() => {
    let names: string[] = [];
    if (clusters && clusters.length > 0) {
      names = clusters.map(c => c.name);
    } else if (nodes && nodes.length > 0) {
      names = [...new Set(nodes.map(n => n.cluster).filter(Boolean))];
    }
    return names.sort().join(',');
  }, [clusters, nodes]);

  // Fetch network interfaces ONCE when switching to network view
  // No polling needed - config data only changes on user action
  useEffect(() => {
    if (view !== 'network' || !clusterNamesKey) return;

    const clusterNames = clusterNamesKey.split(',');

    async function fetchNetworkData() {
      const allInterfaces: NetworkInterface[] = [];
      for (const clusterName of clusterNames) {
        try {
          const ifaces = await api.getClusterNetworkInterfaces(clusterName);
          allInterfaces.push(...ifaces);
        } catch (e) {
          console.error(`Failed to fetch network for ${clusterName}:`, e);
        }
      }
      setNetworkInterfaces(allInterfaces);
    }

    fetchNetworkData();
  }, [view, clusterNamesKey]);

  // Sort clusters by name
  const sortedClusters = useMemo(() =>
    [...clusters].sort((a, b) => a.name.localeCompare(b.name)),
    [clusters]
  );

  // Sort nodes by name for stable ordering
  const sortedNodes = useMemo(() =>
    [...nodes].sort((a, b) => a.node.localeCompare(b.node)),
    [nodes]
  );

  // Sort guests by vmid for stable ordering
  const sortedGuests = useMemo(() =>
    [...guests].sort((a, b) => a.vmid - b.vmid),
    [guests]
  );

  // Group guests by cluster and node
  const guestsByClusterNode = useMemo(() => {
    const map: Record<string, Record<string, Guest[]>> = {};
    for (const g of sortedGuests) {
      const cluster = g.cluster || 'default';
      if (!map[cluster]) map[cluster] = {};
      if (!map[cluster][g.node]) map[cluster][g.node] = [];
      map[cluster][g.node].push(g);
    }
    return map;
  }, [sortedGuests]);

  // Group nodes by cluster
  const nodesByCluster = useMemo(() => {
    const map: Record<string, typeof sortedNodes> = {};
    for (const n of sortedNodes) {
      const cluster = n.cluster || 'default';
      if (!map[cluster]) map[cluster] = [];
      map[cluster].push(n);
    }
    return map;
  }, [sortedNodes]);

  const isSelected = (obj: SelectedObject) =>
    selectedObject?.type === obj.type && selectedObject?.id === obj.id && selectedObject?.cluster === obj.cluster;

  const filterGuest = (g: Guest) =>
    !filterLower ||
    g.name.toLowerCase().includes(filterLower) ||
    g.vmid.toString().includes(filterLower);

  // Flatten folder tree for "Move to" submenu
  const flattenFolders = (folders: Folder[], prefix = ''): { id: string; name: string; path: string }[] => {
    const result: { id: string; name: string; path: string }[] = [];
    for (const folder of folders) {
      const path = prefix ? `${prefix} / ${folder.name}` : folder.name;
      result.push({ id: folder.id, name: folder.name, path });
      if (folder.children) {
        result.push(...flattenFolders(folder.children, path));
      }
    }
    return result;
  };

  // Context menu builders
  const getGuestMenuItems = (guest: Guest, treeView: TreeView): MenuItem[] => {
    const isRunning = guest.status === 'running';
    const type = guest.type === 'qemu' ? 'vm' : 'ct';

    // "Move to" only available in VMs & Templates view (vCenter behavior)
    const flatFolders = treeView === 'vms' ? flattenFolders(vmsTree) : [];
    const moveToSubmenu: MenuItem[] = treeView === 'vms' ? [
      {
        label: '(Root)',
        icon: '🏢',
        action: async () => {
          try {
            await moveResource({
              resource_type: type,
              resource_id: guest.vmid.toString(),
              cluster: guest.cluster,
              to_folder_id: undefined,
            }, treeView);
          } catch (err) {
            alert('Move failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
          }
        },
      },
      { label: '', action: () => {}, divider: true },
      ...flatFolders.map(f => ({
        label: f.path,
        icon: '📁',
        action: async () => {
          try {
            await moveResource({
              resource_type: type,
              resource_id: guest.vmid.toString(),
              cluster: guest.cluster,
              to_folder_id: f.id,
            }, treeView);
          } catch (err) {
            alert('Move failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
          }
        },
      })),
    ] : [];

    return [
      {
        label: 'Start',
        icon: '▶',
        action: () => performAction(type, guest.vmid, 'start', guest.cluster),
        disabled: isRunning,
      },
      {
        label: 'Shutdown',
        icon: '⏹',
        action: () => performAction(type, guest.vmid, 'shutdown', guest.cluster),
        disabled: !isRunning,
      },
      {
        label: 'Stop',
        icon: '⏻',
        action: () => performAction(type, guest.vmid, 'stop', guest.cluster),
        disabled: !isRunning,
        danger: true,
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Console',
        icon: '🖥',
        action: () => openConsole(type, guest.vmid, guest.name, guest.cluster),
      },
      {
        label: 'Migrate',
        icon: '↔',
        action: () => setMigrateGuest(guest),
      },
      // "Move to" only in VMs & Templates view (vCenter behavior)
      ...(treeView === 'vms' ? [{
        label: 'Move to',
        icon: '📂',
        action: () => {},
        submenu: moveToSubmenu,
      }] : []),
      { label: '', action: () => {}, divider: true },
      {
        label: 'Snapshots',
        icon: '📷',
        action: () => setSelectedObject({
          type,
          id: guest.vmid,
          name: guest.name,
          node: guest.node,
          cluster: guest.cluster,
          defaultTab: 'snapshots',
        }),
      },
      {
        label: 'Clone',
        icon: '📋',
        action: () => setCloneGuest(guest),
      },
      {
        label: 'Backup Now',
        icon: '💾',
        action: () => setBackupGuest(guest),
      },
      ...(guest.template ? [] : [{
        label: 'Convert to Template',
        icon: '🖼',
        disabled: isRunning,
        action: async () => {
          const msg = `Convert ${guest.name} (${guest.vmid}) to a template?\n\nThis is permanent — templates cannot be converted back to regular ${type === 'vm' ? 'VMs' : 'containers'}.`;
          if (!confirm(msg)) return;
          try {
            if (type === 'vm') {
              await api.convertVMToTemplate(guest.cluster, guest.vmid);
            } else {
              await api.convertContainerToTemplate(guest.cluster, guest.vmid);
            }
          } catch (err) {
            alert('Convert failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
          }
        },
      }]),
      { label: '', action: () => {}, divider: true },
      guest.ha_state ? {
        label: 'Disable HA',
        icon: '🛡',
        action: async () => {
          if (confirm(`Disable HA protection for ${guest.name}?`)) {
            try {
              await api.disableHA(guest.cluster, type, guest.vmid);
              alert('HA disabled. Changes will reflect after next poll.');
            } catch (err) {
              alert('Failed to disable HA: ' + (err instanceof Error ? err.message : 'Unknown error'));
            }
          }
        },
      } : {
        label: 'Enable HA',
        icon: '🛡',
        action: async () => {
          try {
            await api.enableHA(guest.cluster, type, guest.vmid, { state: 'started' });
            alert('HA enabled. Changes will reflect after next poll.');
          } catch (err) {
            alert('Failed to enable HA: ' + (err instanceof Error ? err.message : 'Unknown error'));
          }
        },
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Delete',
        icon: '🗑',
        danger: true,
        disabled: isRunning,
        action: async () => {
          if (!confirm(`Are you sure you want to delete ${guest.name} (${guest.vmid})?\n\nThis action cannot be undone.`)) {
            return;
          }
          const purge = confirm('Also remove all disks and unreferenced files?');
          try {
            if (type === 'vm') {
              await api.deleteVM(guest.cluster, guest.vmid, purge);
            } else {
              await api.deleteContainer(guest.cluster, guest.vmid, purge);
            }
          } catch (err) {
            alert('Delete failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
          }
        },
      },
    ];
  };

  // Find inventory host by cluster and node name
  const findInventoryHost = (clusterName: string, nodeName: string): InventoryHost | undefined => {
    // Search in datacenters
    for (const dc of datacenters) {
      for (const cluster of dc.clusters || []) {
        if (cluster.name === clusterName || cluster.agent_name === clusterName) {
          return cluster.hosts?.find(h => h.node_name === nodeName || h.address.startsWith(nodeName));
        }
      }
    }
    // Search in orphan clusters
    for (const cluster of orphanClusters) {
      if (cluster.name === clusterName || cluster.agent_name === clusterName) {
        return cluster.hosts?.find(h => h.node_name === nodeName || h.address.startsWith(nodeName));
      }
    }
    return undefined;
  };

  const getNodeMenuItems = (nodeName: string, cluster: string): MenuItem[] => {
    const inventoryHost = findInventoryHost(cluster, nodeName);

    const items: MenuItem[] = [
      {
        label: 'Shell',
        icon: '🖥',
        action: () => alert('Node shell not yet implemented'),
      },
    ];

    // Add SSH/Deploy options if we have an inventory host
    if (inventoryHost) {
      items.push(
        { label: '', action: () => {}, divider: true },
        {
          label: 'Setup SSH Key',
          icon: '🔑',
          action: () => setSshSetupDialog({ hostId: inventoryHost.id, hostAddress: inventoryHost.address }),
        },
        {
          label: 'Deploy Agent',
          icon: '📦',
          action: async () => {
            if (deployingAgent) {
              alert('Already deploying an agent. Please wait.');
              return;
            }
            if (!confirm(`Deploy pve-agent to "${inventoryHost.address}"?\n\nThis requires SSH key access (run Setup SSH first).`)) {
              return;
            }
            setDeployingAgent(inventoryHost.id);
            try {
              const result = await api.deployAgent(inventoryHost.id);
              alert(result.message);
              fetchDatacenterTree();
            } catch (err) {
              alert('Deploy failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
            } finally {
              setDeployingAgent(null);
            }
          },
        }
      );
    }

    items.push(
      { label: '', action: () => {}, divider: true },
      {
        label: 'Create VM',
        icon: '💻',
        action: () => setCreateVMDialog({ cluster, node: nodeName }),
      },
      {
        label: 'Create Container',
        icon: '📦',
        action: () => setCreateContainerDialog({ cluster, node: nodeName }),
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Reboot',
        icon: '🔄',
        action: () => alert('Node reboot not yet implemented'),
        danger: true,
      }
    );

    return items;
  };

  // Get best node for a cluster (node with most free memory)
  const getBestNode = (cluster: string): string | null => {
    const clusterNodes = nodes.filter(n => n.cluster === cluster && n.status === 'online');
    if (clusterNodes.length === 0) return null;
    // Pick node with most available memory
    return clusterNodes.sort((a, b) => (b.maxmem - b.mem) - (a.maxmem - a.mem))[0].node;
  };

  const getClusterMenuItems = (clusterName: string): MenuItem[] => {
    const bestNode = getBestNode(clusterName);
    return [
      {
        label: 'DRS Recommendations',
        icon: '⚖',
        action: () => alert('DRS UI coming in Phase 4'),
      },
      {
        label: 'HA Status',
        icon: '🛡',
        action: () => alert('HA UI coming in Phase 5'),
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Create VM',
        icon: '💻',
        action: () => {
          if (bestNode) {
            setCreateVMDialog({ cluster: clusterName, node: bestNode });
          } else {
            alert('No online nodes available in this cluster');
          }
        },
        disabled: !bestNode,
      },
      {
        label: 'Create Container',
        icon: '📦',
        action: () => {
          if (bestNode) {
            setCreateContainerDialog({ cluster: clusterName, node: bestNode });
          } else {
            alert('No online nodes available in this cluster');
          }
        },
        disabled: !bestNode,
      },
    ];
  };

  // Get datacenter context menu items
  const getDatacenterMenuItems = (dc: Datacenter): MenuItem[] => {
    // "Create Proxmox Cluster" surfaces whenever there's ≥1 online standalone
    // host — Proxmox supports forming a 1-node cluster and adding joiners
    // later. No artificial ≥2 gate; the wizard adapts to the number of hosts.
    const onlineStandaloneHosts = (dc.hosts || []).filter(h => h.status === 'online');
    const canFormPveCluster = onlineStandaloneHosts.length >= 1;

    return [
      {
        label: 'Add Host',
        icon: '🖥️',
        action: () => setAddHostDialog({
          mode: 'datacenter',
          datacenterId: dc.id,
          datacenterName: dc.name,
        }),
      },
      {
        label: 'Add Cluster (label only)',
        icon: '➕',
        action: () => setDatacenterDialog({
          mode: 'create-cluster',
          parentDatacenterId: dc.id,
        }),
      },
      ...(canFormPveCluster ? [{
        label: 'Create Proxmox Cluster…',
        icon: '🏗️',
        action: () => setCreatePveClusterDialog(dc),
      }] : []),
      { label: '', action: () => {}, divider: true },
      {
        label: 'Edit Datacenter',
        icon: '✏️',
        action: () => setDatacenterDialog({
          mode: 'edit-dc',
          datacenter: dc,
        }),
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Delete Datacenter',
        icon: '🗑️',
        action: async () => {
          if (confirm(`Delete datacenter "${dc.name}"? Clusters will become unassigned.`)) {
            try {
              await api.deleteDatacenter(dc.id);
              fetchDatacenterTree();
            } catch (err) {
              alert('Failed to delete: ' + (err instanceof Error ? err.message : 'Unknown error'));
            }
          }
        },
        danger: true,
      },
    ];
  };

  // Get inventory cluster context menu items (for configuration)
  const getInventoryClusterMenuItems = (cluster: InventoryCluster): MenuItem[] => {
    // Build "Move to Datacenter" submenu
    const moveSubmenu: MenuItem[] = [
      {
        label: '(Unassigned)',
        icon: '📤',
        action: async () => {
          try {
            await api.moveClusterToDatacenter(cluster.name, undefined);
            fetchDatacenterTree();
          } catch (err) {
            alert('Move failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
          }
        },
      },
      { label: '', action: () => {}, divider: true },
      ...datacenters.map(dc => ({
        label: dc.name,
        icon: '🏢',
        action: async () => {
          try {
            await api.moveClusterToDatacenter(cluster.name, dc.id);
            fetchDatacenterTree();
          } catch (err) {
            alert('Move failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
          }
        },
      })),
    ];

    // Collect all online standalone hosts across datacenters as candidates
    // for joining this cluster. Filtering happens inside the dialog too,
    // but we precompute here to decide whether to show the menu entry.
    const standalonePool: InventoryHost[] = datacenters
      .flatMap(dc => dc.hosts || [])
      .filter(h => h.status === 'online' && !h.cluster_id);

    return [
      ...(cluster.status === 'active' && standalonePool.length > 0 ? [{
        label: 'Add Member Node…',
        icon: '🔗',
        action: () => setJoinPveClusterDialog(cluster),
      }] : []),
      {
        label: 'Add Host (inventory record only)',
        icon: '➕',
        action: () => setAddHostDialog({ mode: 'cluster', clusterName: cluster.name }),
      },
      {
        label: 'Edit Cluster',
        icon: '✏️',
        action: () => setDatacenterDialog({
          mode: 'edit-cluster',
          cluster,
        }),
      },
      {
        label: 'Move to Datacenter',
        icon: '↔️',
        action: () => {},
        submenu: moveSubmenu,
      },
      { label: '', action: () => {}, divider: true },
      {
        label: cluster.enabled ? 'Disable' : 'Enable',
        icon: cluster.enabled ? '⏸️' : '▶️',
        action: async () => {
          try {
            await api.updateInventoryCluster(cluster.name, {
              ...cluster,
              enabled: !cluster.enabled,
            });
            fetchDatacenterTree();
          } catch (err) {
            alert('Failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
          }
        },
      },
      {
        label: 'Delete Cluster',
        icon: '🗑️',
        action: async () => {
          if (confirm(`Delete cluster "${cluster.name}"? This cannot be undone.`)) {
            try {
              await api.deleteInventoryCluster(cluster.name);
              fetchDatacenterTree();
            } catch (err) {
              alert('Failed to delete: ' + (err instanceof Error ? err.message : 'Unknown error'));
            }
          }
        },
        danger: true,
      },
    ];
  };

  // Get inventory host context menu items
  const getInventoryHostMenuItems = (host: InventoryHost, clusterOrDcName: string): MenuItem[] => {
    const items: MenuItem[] = [
      {
        label: 'Setup SSH Key',
        icon: '🔑',
        action: () => setSshSetupDialog({ hostId: host.id, hostAddress: host.address }),
      },
      {
        label: 'Deploy Agent',
        icon: '📦',
        action: async () => {
          if (deployingAgent) {
            alert('Already deploying an agent. Please wait.');
            return;
          }
          if (!confirm(`Deploy pve-agent to "${host.address}"?\n\nThis requires SSH key access (run Setup SSH first).`)) {
            return;
          }
          setDeployingAgent(host.id);
          try {
            const result = await api.deployAgent(host.id);
            alert(result.message);
            fetchDatacenterTree();
          } catch (err) {
            alert('Deploy failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
          } finally {
            setDeployingAgent(null);
          }
        },
      },
    ];

    // "Move to Cluster" submenu — collect all clusters from all datacenters
    const allClusters: { id: string; name: string }[] = [];
    for (const dc of datacenters) {
      for (const c of (dc.clusters || [])) {
        if (c.id !== host.cluster_id) {
          allClusters.push({ id: c.id, name: c.name });
        }
      }
    }
    for (const c of orphanClusters) {
      if (c.id !== host.cluster_id) {
        allClusters.push({ id: c.id, name: c.name });
      }
    }

    if (allClusters.length > 0) {
      items.push({
        label: 'Move to Cluster',
        icon: '🏛️',
        action: () => {},
        submenu: allClusters.map(c => ({
          label: c.name,
          icon: '🏛️',
          action: async () => {
            if (confirm(`Move "${host.node_name || host.address}" to cluster "${c.name}"?`)) {
              try {
                await api.moveHostToCluster(host.id, c.id);
                fetchDatacenterTree();
              } catch (err) {
                alert('Move failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
              }
            }
          },
        })),
      });
    }

    items.push({ label: '', action: () => {}, divider: true });
    items.push({
      label: 'Delete Host',
      icon: '🗑️',
      action: async () => {
        if (confirm(`Remove host "${host.address}" from "${clusterOrDcName}"?`)) {
          try {
            await api.deleteHost(host.id);
            fetchDatacenterTree();
          } catch (err) {
            alert('Failed to delete: ' + (err instanceof Error ? err.message : 'Unknown error'));
          }
        }
      },
      danger: true,
    });

    return items;
  };

  // Get root menu items for hosts view
  const getRootHostsMenuItems = (): MenuItem[] => {
    return [
      {
        label: 'Add Host',
        icon: '🖥️',
        action: handleAddHostIntent,
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'New Folder',
        icon: '📁',
        action: () => setFolderDialog({ mode: 'create', treeView: 'hosts' }),
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Add Datacenter',
        icon: '🏢',
        action: () => setDatacenterDialog({ mode: 'create-dc' }),
      },
      {
        label: 'Add Cluster',
        icon: '🏛️',
        action: () => setDatacenterDialog({ mode: 'create-cluster' }),
      },
    ];
  };

  const getStorageMenuItems = (s: Storage): MenuItem[] => {
    return [
      {
        label: 'Browse Content',
        icon: '📂',
        action: () => {
          setSelectedObject({
            type: 'storage',
            id: `${s.node}-${s.storage}`,
            name: s.storage,
            node: s.node,
            cluster: s.cluster,
            defaultTab: 'vms',
          });
        },
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Upload ISO',
        icon: '📀',
        action: () => setUploadDialog({ storage: s.storage, node: s.node, contentType: 'iso' }),
      },
      {
        label: 'Upload Template',
        icon: '📄',
        action: () => setUploadDialog({ storage: s.storage, node: s.node, contentType: 'vztmpl' }),
      },
    ];
  };

  // Get folder context menu items
  const getFolderMenuItems = (folder: Folder, treeView: TreeView): MenuItem[] => {
    return [
      {
        label: 'New Folder',
        icon: '📁',
        action: () => setFolderDialog({
          mode: 'create',
          parentId: folder.id,
          treeView,
        }),
      },
      {
        label: 'Rename',
        icon: '✏️',
        action: () => setFolderDialog({
          mode: 'rename',
          folderId: folder.id,
          initialName: folder.name,
          treeView,
        }),
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Delete',
        icon: '🗑️',
        action: async () => {
          if (confirm(`Delete folder "${folder.name}"? Contents will be moved to parent.`)) {
            try {
              await deleteFolder(folder.id);
            } catch (err) {
              alert('Failed to delete folder: ' + (err instanceof Error ? err.message : 'Unknown error'));
            }
          }
        },
        danger: true,
      },
    ];
  };

  // Get best cluster/node combo across all clusters (for VMs view)
  const getBestClusterNode = (): { cluster: string; node: string } | null => {
    const onlineNodes = nodes.filter(n => n.status === 'online');
    if (onlineNodes.length === 0) return null;
    // Pick node with most available memory across all clusters
    const best = onlineNodes.sort((a, b) => (b.maxmem - b.mem) - (a.maxmem - a.mem))[0];
    return { cluster: best.cluster, node: best.node };
  };

  // Get datacenter/root context menu items (folders only in VMs view per vCenter)
  const getRootMenuItems = (treeView: TreeView): MenuItem[] => {
    if (treeView !== 'vms') return [];
    const bestTarget = getBestClusterNode();
    return [
      {
        label: 'New Folder',
        icon: '📁',
        action: () => setFolderDialog({
          mode: 'create',
          treeView,
        }),
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Create VM',
        icon: '💻',
        action: () => {
          if (bestTarget) {
            setCreateVMDialog(bestTarget);
          } else {
            alert('No online nodes available');
          }
        },
        disabled: !bestTarget,
      },
      {
        label: 'Create Container',
        icon: '📦',
        action: () => {
          if (bestTarget) {
            setCreateContainerDialog(bestTarget);
          } else {
            alert('No online nodes available');
          }
        },
        disabled: !bestTarget,
      },
    ];
  };

  // Handle folder dialog submit
  const handleFolderDialogSubmit = async (name: string) => {
    if (!folderDialog) return;
    try {
      if (folderDialog.mode === 'create') {
        await createFolder(name, folderDialog.treeView, folderDialog.parentId);
      } else if (folderDialog.folderId) {
        await renameFolder(folderDialog.folderId, name);
      }
      setFolderDialog(null);
    } catch (err) {
      alert('Failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
    }
  };

  // Handle drop on a folder
  const handleFolderDrop = async (targetFolderId: string, treeView: TreeView, data: DragData) => {
    try {
      if (data.type === 'folder' && data.folderId) {
        // Move folder to new parent
        if (data.folderId === targetFolderId) return; // Can't drop on itself
        await moveFolder(data.folderId, targetFolderId);
      } else if (data.type === 'resource' && data.resourceType && data.resourceId && data.cluster) {
        // Move resource to folder
        await moveResource({
          resource_type: data.resourceType,
          resource_id: data.resourceId,
          cluster: data.cluster,
          to_folder_id: targetFolderId,
        }, treeView);
      }
    } catch (err) {
      alert('Move failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
    }
  };

  // Handle drop on root (remove from folder)
  const handleRootDrop = async (treeView: TreeView, data: DragData) => {
    try {
      if (data.type === 'folder' && data.folderId) {
        // Move folder to root
        await moveFolder(data.folderId, undefined);
      } else if (data.type === 'resource' && data.resourceType && data.resourceId && data.cluster) {
        // Remove resource from folder (move to root)
        await moveResource({
          resource_type: data.resourceType,
          resource_id: data.resourceId,
          cluster: data.cluster,
          to_folder_id: undefined,
        }, treeView);
      }
    } catch (err) {
      alert('Move failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
    }
  };

  const showContextMenu = (e: React.MouseEvent, items: MenuItem[]) => {
    setContextMenu({ x: e.clientX, y: e.clientY, items });
  };

  const closeContextMenu = () => setContextMenu(null);

  // Render a folder node with its children
  const renderFolderNode = (folder: Folder, treeView: TreeView) => (
    <TreeNode
      key={folder.id}
      icon="📁"
      label={folder.name}
      defaultExpanded
      count={(folder.children?.length || 0) + (folder.members?.length || 0)}
      onContextMenu={(e) => showContextMenu(e, getFolderMenuItems(folder, treeView))}
      draggable
      dragData={{ type: 'folder', folderId: folder.id }}
      droppable
      onDrop={(data) => handleFolderDrop(folder.id, treeView, data)}
    >
      {/* Render child folders */}
      {folder.children?.map((child) => renderFolderNode(child, treeView))}
      {/* Render folder members (resources) - this requires matching with actual guests/nodes */}
      {folder.members?.map((member) => {
        if (member.resource_type === 'vm' || member.resource_type === 'ct') {
          const guest = guests.find(g =>
            g.vmid.toString() === member.resource_id &&
            g.cluster === member.cluster
          );
          if (guest) {
            return renderGuestNode(guest, member.resource_type === 'vm' ? 'vm' : 'ct', treeView);
          }
        }
        if (member.resource_type === 'node') {
          const node = nodes.find(n =>
            n.node === member.resource_id &&
            n.cluster === member.cluster
          );
          if (node) {
            const nodeGuests = (guestsByClusterNode[node.cluster]?.[node.node] || []).filter(filterGuest);
            const vms = nodeGuests.filter(g => g.type === 'qemu');
            const cts = nodeGuests.filter(g => g.type === 'lxc');
            return (
              <TreeNode
                key={`${node.cluster}-${node.node}`}
                icon="🖥"
                label={node.node}
                status={node.status === 'online' ? 'online' : 'stopped'}
                isSelected={isSelected({ type: 'node', id: node.node, name: node.node, cluster: node.cluster })}
                onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, cluster: node.cluster })}
                onContextMenu={(e) => showContextMenu(e, getNodeMenuItems(node.node, node.cluster))}
                defaultExpanded
                count={nodeGuests.length}
              >
                {vms.length > 0 && (
                  <TreeNode icon="📁" label="Virtual Machines" count={vms.length} defaultExpanded
                    onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, cluster: node.cluster, defaultTab: 'only-vms' })}>
                    {vms.map((vm) => renderGuestNode(vm, 'vm', treeView))}
                  </TreeNode>
                )}
                {cts.length > 0 && (
                  <TreeNode icon="📁" label="Containers" count={cts.length} defaultExpanded
                    onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, cluster: node.cluster, defaultTab: 'only-cts' })}>
                    {cts.map((ct) => renderGuestNode(ct, 'ct', treeView))}
                  </TreeNode>
                )}
              </TreeNode>
            );
          }
        }
        return null;
      })}
    </TreeNode>
  );

  // Render guest node with context menu - draggable only in VMs view
  const renderGuestNode = (guest: Guest, type: 'vm' | 'ct', treeView: TreeView) => (
    <TreeNode
      key={`${guest.cluster}-${guest.vmid}`}
      icon={type === 'vm' ? '💻' : '📦'}
      label={`${guest.vmid} - ${guest.name}`}
      status={guest.status === 'running' ? 'running' : 'stopped'}
      badge={guest.ha_state ? (
        <span className={`text-xs px-1 rounded ${
          guest.ha_state === 'started' ? 'bg-green-600/20 text-green-400' :
          guest.ha_state === 'stopped' ? 'bg-gray-600/20 text-gray-400' :
          guest.ha_state === 'error' || guest.ha_state === 'fence' ? 'bg-red-600/20 text-red-400' :
          'bg-blue-600/20 text-blue-400'
        }`} title={`HA: ${guest.ha_state}`}>HA</span>
      ) : undefined}
      isSelected={isSelected({ type, id: guest.vmid, name: guest.name, node: guest.node, cluster: guest.cluster })}
      onClick={() => setSelectedObject({ type, id: guest.vmid, name: guest.name, node: guest.node, cluster: guest.cluster })}
      onContextMenu={(e) => showContextMenu(e, getGuestMenuItems(guest, treeView))}
      draggable={treeView === 'vms'}
      dragData={treeView === 'vms' ? {
        type: 'resource',
        resourceType: type,
        resourceId: guest.vmid.toString(),
        cluster: guest.cluster,
      } : undefined}
    />
  );

  // Get status color for inventory host
  const getHostStatusColor = (status: string) => {
    switch (status) {
      case 'online': return 'online';
      case 'connecting': return 'warning';
      case 'offline':
      case 'staged':
      case 'error':
      default: return 'stopped';
    }
  };

  // Helper to render a cluster node with its hosts
  const renderClusterWithNodes = (clusterName: string, inventoryCluster?: InventoryCluster) => {
    // Use agent_name for runtime data lookups (what agents report), display name for UI
    const lookupName = inventoryCluster?.agent_name || clusterName;
    const clusterNodes = nodesByCluster[lookupName] || [];
    const runtimeCluster = clusters.find(c => c.name === lookupName);
    const inventoryHosts = inventoryCluster?.hosts || [];

    // Determine what to show under the cluster - prefer runtime nodes if available
    const hasRuntimeNodes = clusterNodes.length > 0;
    const hasInventoryHosts = inventoryHosts.length > 0;

    // Status badge based on cluster status
    const getClusterStatusBadge = () => {
      if (!inventoryCluster) return null;
      switch (inventoryCluster.status) {
        case 'empty':
          return <span className="text-xs px-1.5 py-0.5 rounded bg-gray-600 text-gray-300">empty</span>;
        case 'pending':
          return <span className="text-xs px-1.5 py-0.5 rounded bg-yellow-600 text-yellow-100">pending</span>;
        case 'error':
          return <span className="text-xs px-1.5 py-0.5 rounded bg-red-600 text-red-100">error</span>;
        default:
          return null;
      }
    };

    return (
      <TreeNode
        key={clusterName}
        icon="🏛"
        label={clusterName}
        badge={<>
          {inventoryCluster && !inventoryCluster.enabled && (
            <span className="text-xs px-1.5 py-0.5 rounded bg-gray-600 text-gray-300">disabled</span>
          )}
          {getClusterStatusBadge()}
          <HABadge ha={runtimeCluster?.ha} nodeCount={clusterNodes.length} />
        </>}
        isSelected={isSelected({ type: 'cluster', id: clusterName, name: clusterName, cluster: lookupName })}
        onClick={() => setSelectedObject({ type: 'cluster', id: clusterName, name: clusterName, cluster: lookupName })}
        onContextMenu={(e) => {
          // Show inventory menu if we have cluster config, otherwise runtime menu
          if (inventoryCluster) {
            showContextMenu(e, getInventoryClusterMenuItems(inventoryCluster));
          } else {
            showContextMenu(e, getClusterMenuItems(clusterName));
          }
        }}
        defaultExpanded
        count={hasRuntimeNodes ? clusterNodes.length : inventoryHosts.length}
      >
        {hasRuntimeNodes ? (
          // Show runtime nodes with guests (normal operational view)
          clusterNodes.map((node) => {
            const nodeGuests = (guestsByClusterNode[lookupName]?.[node.node] || []).filter(filterGuest);
            const vms = nodeGuests.filter(g => g.type === 'qemu');
            const cts = nodeGuests.filter(g => g.type === 'lxc');

            return (
              <TreeNode
                key={`${clusterName}-${node.node}`}
                icon="🖥"
                label={node.node}
                status={node.status === 'online' ? 'online' : 'stopped'}
                isSelected={isSelected({ type: 'node', id: node.node, name: node.node, cluster: node.cluster })}
                onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, cluster: node.cluster })}
                onContextMenu={(e) => showContextMenu(e, getNodeMenuItems(node.node, clusterName))}
                defaultExpanded
                count={nodeGuests.length}
              >
                {vms.length > 0 && (
                  <TreeNode icon="📁" label="Virtual Machines" count={vms.length} defaultExpanded
                    onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, cluster: node.cluster, defaultTab: 'only-vms' })}>
                    {vms.map((vm) => renderGuestNode(vm, 'vm', 'hosts'))}
                  </TreeNode>
                )}
                {cts.length > 0 && (
                  <TreeNode icon="📁" label="Containers" count={cts.length} defaultExpanded
                    onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, cluster: node.cluster, defaultTab: 'only-cts' })}>
                    {cts.map((ct) => renderGuestNode(ct, 'ct', 'hosts'))}
                  </TreeNode>
                )}
              </TreeNode>
            );
          })
        ) : hasInventoryHosts ? (
          // Show inventory hosts (configuration view - cluster not yet active)
          inventoryHosts.map((host) => (
            <TreeNode
              key={host.id}
              icon={host.status === 'online' ? '🟢' : (host.status === 'error' || host.status === 'offline') ? '🔴' : '⚪'}
              iconTitle={host.status === 'error' ? host.error : host.status}
              label={host.node_name || host.address}
              status={getHostStatusColor(host.status) as 'online' | 'stopped' | 'warning'}
              badge={host.error ? (
                <span className="text-xs px-1.5 py-0.5 rounded bg-red-600 text-red-100" title={host.error}>!</span>
              ) : undefined}
              onContextMenu={(e) => showContextMenu(e, getInventoryHostMenuItems(host, clusterName))}
            />
          ))
        ) : (
          // Empty cluster - show hint
          <div className="px-6 py-2 text-xs text-gray-500 italic">
            No hosts. Right-click to add one.
          </div>
        )}
      </TreeNode>
    );
  };

  // Background right-click handler for empty space in the tree
  const handleTreeBackground = (e: React.MouseEvent) => {
    e.preventDefault();
    const items = view === 'hosts' ? getRootHostsMenuItems() : getRootMenuItems(view as TreeView);
    if (items.length > 0) showContextMenu(e, items);
  };

  // Hosts & Clusters View - with datacenter hierarchy
  if (view === 'hosts') {
    const totalNodes = sortedNodes.length;
    const hasDatacenters = datacenters.length > 0 || orphanClusters.length > 0;

    return (
      <div className="py-2 min-h-full" onContextMenu={handleTreeBackground}>
        {contextMenu && (
          <ContextMenu
            x={contextMenu.x}
            y={contextMenu.y}
            items={contextMenu.items}
            onClose={closeContextMenu}
          />
        )}
        {migrateGuest && (
          <MigrateDialog
            guest={migrateGuest}
            onClose={() => setMigrateGuest(null)}
            onSuccess={() => {
              // Migration started - state will update via WebSocket
            }}
          />
        )}
        {cloneGuest && (
          <CloneDialog
            guest={cloneGuest}
            onClose={() => setCloneGuest(null)}
            onSuccess={() => {
              // Clone started - state will update via WebSocket
            }}
          />
        )}
        {backupGuest && (
          <BackupDialog
            guest={backupGuest}
            onClose={() => setBackupGuest(null)}
            onSuccess={() => setBackupGuest(null)}
          />
        )}
        {datacenterDialog && (
          <DatacenterDialog
            mode={datacenterDialog.mode}
            datacenter={datacenterDialog.datacenter}
            cluster={datacenterDialog.cluster}
            parentDatacenterId={datacenterDialog.parentDatacenterId}
            datacenters={datacenters}
            onSubmit={async (created) => {
              const shouldChain = chainAddHostAfterDC && datacenterDialog.mode === 'create-dc';
              setChainAddHostAfterDC(false);
              setDatacenterDialog(null);
              await fetchDatacenterTree();
              if (shouldChain && created && 'name' in created) {
                const dc = created as Datacenter;
                setAddHostDialog({ mode: 'datacenter', datacenterId: dc.id, datacenterName: dc.name });
              }
            }}
            onClose={() => {
              setChainAddHostAfterDC(false);
              setDatacenterDialog(null);
            }}
          />
        )}
        {addHostDialog && (
          <AddHostDialog
            mode={addHostDialog.mode}
            clusterName={addHostDialog.clusterName}
            datacenterId={addHostDialog.datacenterId}
            datacenterName={addHostDialog.datacenterName}
            onSubmit={async () => {
              setAddHostDialog(null);
              await fetchDatacenterTree();
            }}
            onClose={() => setAddHostDialog(null)}
          />
        )}
        {createPveClusterDialog && (
          <CreateProxmoxClusterDialog
            datacenter={createPveClusterDialog}
            onClose={() => setCreatePveClusterDialog(null)}
            onSuccess={fetchDatacenterTree}
          />
        )}
        {joinPveClusterDialog && (
          <JoinPveClusterDialog
            cluster={joinPveClusterDialog}
            availableHosts={datacenters.flatMap(dc => dc.hosts || [])}
            onClose={() => setJoinPveClusterDialog(null)}
            onSuccess={fetchDatacenterTree}
          />
        )}
        {createVMDialog && (
          <CreateVMDialog
            cluster={createVMDialog.cluster}
            node={createVMDialog.node}
            onClose={() => setCreateVMDialog(null)}
            onSuccess={() => {
              // VM created - state will update via WebSocket
            }}
          />
        )}
        {createContainerDialog && (
          <CreateContainerDialog
            cluster={createContainerDialog.cluster}
            node={createContainerDialog.node}
            onClose={() => setCreateContainerDialog(null)}
            onSuccess={() => {
              // Container created - state will update via WebSocket
            }}
          />
        )}
        {sshSetupDialog && (
          <SSHSetupDialog
            hostAddress={sshSetupDialog.hostAddress}
            onSubmit={async (password) => {
              try {
                const result = await api.setupHostSSH(sshSetupDialog.hostId, password);
                alert(result.message);
                setSshSetupDialog(null);
              } catch (err) {
                alert('SSH setup failed: ' + (err instanceof Error ? err.message : 'Unknown error'));
              }
            }}
            onClose={() => setSshSetupDialog(null)}
          />
        )}
        <TreeNode
          icon="🏢"
          label="pCenter"
          defaultExpanded
          count={totalNodes}
          isSelected={isSelected({ type: 'datacenter', id: 'root', name: 'pCenter' })}
          onClick={() => setSelectedObject({ type: 'datacenter', id: 'root', name: 'pCenter' })}
          onContextMenu={(e) => showContextMenu(e, getRootHostsMenuItems())}
        >
          {hasDatacenters ? (
            <>
              {/* Render datacenters with their clusters */}
              {datacenters.map((dc) => (
                <TreeNode
                  key={dc.id}
                  icon="🏢"
                  label={dc.name}
                  defaultExpanded
                  count={(dc.clusters?.length || 0) + (dc.hosts?.length || 0)}
                  isSelected={isSelected({ type: 'datacenter', id: dc.id, name: dc.name })}
                  onClick={() => setSelectedObject({ type: 'datacenter', id: dc.id, name: dc.name })}
                  onContextMenu={(e) => showContextMenu(e, getDatacenterMenuItems(dc))}
                >
                  {/* Render clusters */}
                  {dc.clusters?.map((invCluster) =>
                    renderClusterWithNodes(invCluster.name, invCluster)
                  )}
                  {/* Render standalone hosts (not in a cluster) */}
                  {dc.hosts?.map((host) => (
                    <TreeNode
                      key={host.id}
                      icon={host.status === 'online' ? '🟢' : (host.status === 'error' || host.status === 'offline') ? '🔴' : '⚪'}
                      iconTitle={host.status === 'error' ? host.error : host.status}
                      label={host.node_name || host.address}
                      status={host.status === 'online' ? 'online' : (host.status === 'error' || host.status === 'offline') ? 'stopped' : 'stopped'}
                      isSelected={isSelected({ type: 'node', id: host.node_name || host.address, name: host.node_name || host.address })}
                      onClick={() => setSelectedObject({ type: 'node', id: host.node_name || host.address, name: host.node_name || host.address })}
                      onContextMenu={(e) => showContextMenu(e, getInventoryHostMenuItems(host, dc.name))}
                    />
                  ))}
                </TreeNode>
              ))}

              {/* Render orphan clusters (no datacenter) */}
              {orphanClusters.length > 0 && (
                <TreeNode
                  icon="📤"
                  label="Unassigned"
                  defaultExpanded
                  count={orphanClusters.length}
                >
                  {orphanClusters.map((invCluster) =>
                    renderClusterWithNodes(invCluster.name, invCluster)
                  )}
                </TreeNode>
              )}
            </>
          ) : sortedClusters.length === 0 ? (
            // Fallback if no clusters at all - show nodes directly
            sortedNodes.map((node) => {
              const nodeGuests = (guestsByClusterNode['default']?.[node.node] || []).filter(filterGuest);
              const vms = nodeGuests.filter(g => g.type === 'qemu');
              const cts = nodeGuests.filter(g => g.type === 'lxc');

              return (
                <TreeNode
                  key={node.node}
                  icon="🖥"
                  label={node.node}
                  status={node.status === 'online' ? 'online' : 'stopped'}
                  isSelected={isSelected({ type: 'node', id: node.node, name: node.node })}
                  onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node })}
                  onContextMenu={(e) => showContextMenu(e, getNodeMenuItems(node.node, 'default'))}
                  defaultExpanded
                  count={nodeGuests.length}
                >
                  {vms.length > 0 && (
                    <TreeNode icon="📁" label="Virtual Machines" count={vms.length} defaultExpanded
                      onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, defaultTab: 'only-vms' })}>
                      {vms.map((vm) => renderGuestNode(vm, 'vm', 'hosts'))}
                    </TreeNode>
                  )}
                  {cts.length > 0 && (
                    <TreeNode icon="📁" label="Containers" count={cts.length} defaultExpanded
                      onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, defaultTab: 'only-cts' })}>
                      {cts.map((ct) => renderGuestNode(ct, 'ct', 'hosts'))}
                    </TreeNode>
                  )}
                </TreeNode>
              );
            })
          ) : (
            // Runtime clusters only (no inventory datacenters yet)
            sortedClusters.map((cluster) => renderClusterWithNodes(cluster.name))
          )}
        </TreeNode>
      </div>
    );
  }

  // VMs & Templates View - Now with folder support
  if (view === 'vms') {
    // Filter out guests that are already in folders
    const vms = sortedGuests
      .filter(g => g.type === 'qemu')
      .filter(filterGuest)
      .filter(g => !isGuestInFolder(g, 'vms'));
    const cts = sortedGuests
      .filter(g => g.type === 'lxc')
      .filter(filterGuest)
      .filter(g => !isGuestInFolder(g, 'vms'));

    return (
      <div className="py-2 min-h-full" onContextMenu={handleTreeBackground}>
        {contextMenu && (
          <ContextMenu
            x={contextMenu.x}
            y={contextMenu.y}
            items={contextMenu.items}
            onClose={closeContextMenu}
          />
        )}
        {migrateGuest && (
          <MigrateDialog
            guest={migrateGuest}
            onClose={() => setMigrateGuest(null)}
            onSuccess={() => {}}
          />
        )}
        {cloneGuest && (
          <CloneDialog
            guest={cloneGuest}
            onClose={() => setCloneGuest(null)}
            onSuccess={() => {}}
          />
        )}
        {backupGuest && (
          <BackupDialog
            guest={backupGuest}
            onClose={() => setBackupGuest(null)}
            onSuccess={() => setBackupGuest(null)}
          />
        )}
        {folderDialog && (
          <FolderDialog
            mode={folderDialog.mode}
            initialName={folderDialog.initialName}
            onSubmit={handleFolderDialogSubmit}
            onClose={() => setFolderDialog(null)}
          />
        )}
        {createVMDialog && (
          <CreateVMDialog
            cluster={createVMDialog.cluster}
            node={createVMDialog.node}
            onClose={() => setCreateVMDialog(null)}
            onSuccess={() => {}}
          />
        )}
        {createContainerDialog && (
          <CreateContainerDialog
            cluster={createContainerDialog.cluster}
            node={createContainerDialog.node}
            onClose={() => setCreateContainerDialog(null)}
            onSuccess={() => {}}
          />
        )}
        <TreeNode
          icon="🏢"
          label="VMs & Templates"
          defaultExpanded
          count={vms.length + cts.length + vmsTree.length}
          onContextMenu={(e) => showContextMenu(e, getRootMenuItems('vms'))}
          droppable
          onDrop={(data) => handleRootDrop('vms', data)}
        >
          {/* Render folders first */}
          {vmsTree.map((folder) => renderFolderNode(folder, 'vms'))}
          <TreeNode icon="💻" label="Virtual Machines" defaultExpanded count={vms.length}>
            {vms.map((vm) => renderGuestNode(vm, 'vm', 'vms'))}
          </TreeNode>
          <TreeNode icon="📦" label="Containers" defaultExpanded count={cts.length}>
            {cts.map((ct) => renderGuestNode(ct, 'ct', 'vms'))}
          </TreeNode>
        </TreeNode>
      </div>
    );
  }

  // Sort storage by name for stable ordering
  const sortedStorage = useMemo(() =>
    [...storage].sort((a, b) => a.storage.localeCompare(b.storage)),
    [storage]
  );

  // Get storage icon and tooltip based on type
  const getStorageIcon = (type: string): { icon: string; title: string } => {
    switch (type) {
      case 'rbd':
        return { icon: '🐙', title: 'Ceph RBD (RADOS Block Device)' };
      case 'cephfs':
        return { icon: '🐙', title: 'CephFS (Ceph Filesystem)' };
      case 'zfspool':
      case 'zfs':
        return { icon: '🗃️', title: 'ZFS Pool' };
      case 'nfs':
        return { icon: '🌐', title: 'NFS (Network File System)' };
      case 'cifs':
        return { icon: '🌐', title: 'CIFS/SMB Share' };
      case 'glusterfs':
        return { icon: '🌐', title: 'GlusterFS' };
      case 'lvm':
        return { icon: '📊', title: 'LVM (Logical Volume Manager)' };
      case 'lvmthin':
        return { icon: '📊', title: 'LVM-thin (Thin Provisioned)' };
      case 'iscsi':
      case 'iscsidirect':
        return { icon: '🔗', title: 'iSCSI Target' };
      case 'pbs':
        return { icon: '🛡️', title: 'Proxmox Backup Server' };
      case 'dir':
        return { icon: '📁', title: 'Directory Storage' };
      case 'btrfs':
        return { icon: '🌲', title: 'Btrfs Filesystem' };
      default:
        return { icon: '💿', title: `Storage (${type})` };
    }
  };

  // Storage View
  if (view === 'storage') {
    const sharedStorage = sortedStorage.filter(s => s.shared);
    const localByNode = useMemo(() => {
      const map: Record<string, Storage[]> = {};
      for (const s of sortedStorage.filter(s => !s.shared)) {
        if (!map[s.node]) map[s.node] = [];
        map[s.node].push(s);
      }
      return map;
    }, [sortedStorage]);

    return (
      <div className="py-2 min-h-full" onContextMenu={(e) => e.preventDefault()}>
        {contextMenu && (
          <ContextMenu
            x={contextMenu.x}
            y={contextMenu.y}
            items={contextMenu.items}
            onClose={closeContextMenu}
          />
        )}
        {sharedStorage.length > 0 && (
          <TreeNode icon="☁" label="Shared Storage" defaultExpanded count={sharedStorage.length}>
            {sharedStorage.map((s) => {
              const { icon, title } = getStorageIcon(s.type);
              return (
                <TreeNode
                  key={`${s.cluster}-${s.node}-${s.storage}`}
                  icon={icon}
                  iconTitle={title}
                  label={`${s.storage} (${s.node})`}
                  status={s.status === 'available' ? 'online' : 'warning'}
                  isSelected={isSelected({ type: 'storage', id: `${s.node}-${s.storage}`, name: s.storage, cluster: s.cluster })}
                  onClick={() => setSelectedObject({ type: 'storage', id: `${s.node}-${s.storage}`, name: s.storage, node: s.node, cluster: s.cluster })}
                  onContextMenu={(e) => showContextMenu(e, getStorageMenuItems(s))}
                />
              );
            })}
          </TreeNode>
        )}
        {sortedNodes.map((node) => {
          const nodeStorage = localByNode[node.node] || [];
          if (nodeStorage.length === 0) return null;
          return (
            <TreeNode key={`${node.cluster}-${node.node}`} icon="🖥" label={node.node} defaultExpanded count={nodeStorage.length}>
              {nodeStorage.map((s) => {
                const { icon, title } = getStorageIcon(s.type);
                return (
                  <TreeNode
                    key={`${s.cluster}-${s.node}-${s.storage}`}
                    icon={icon}
                    iconTitle={title}
                    label={s.storage}
                    status={s.status === 'available' ? 'online' : 'warning'}
                    isSelected={isSelected({ type: 'storage', id: `${s.node}-${s.storage}`, name: s.storage, cluster: s.cluster })}
                    onClick={() => setSelectedObject({ type: 'storage', id: `${s.node}-${s.storage}`, name: s.storage, node: s.node, cluster: s.cluster })}
                    onContextMenu={(e) => showContextMenu(e, getStorageMenuItems(s))}
                  />
                );
              })}
            </TreeNode>
          );
        })}
        {uploadDialog && (
          <UploadDialog
            storage={uploadDialog.storage}
            node={uploadDialog.node}
            contentType={uploadDialog.contentType}
            onClose={() => setUploadDialog(null)}
          />
        )}
      </div>
    );
  }

  // Network View - real interfaces
  if (view === 'network') {
    // Group interfaces by node, then by type
    const interfacesByNode = networkInterfaces.reduce((acc, iface) => {
      const key = `${iface.cluster}-${iface.node}`;
      if (!acc[key]) acc[key] = { node: iface.node, cluster: iface.cluster, interfaces: [] };
      acc[key].interfaces.push(iface);
      return acc;
    }, {} as Record<string, { node: string; cluster: string; interfaces: NetworkInterface[] }>);

    // Icon and tooltip based on interface type
    const getIfaceIcon = (type: string): { icon: string; title: string } => {
      switch (type) {
        case 'bridge': return { icon: '🌉', title: 'Linux Bridge' };
        case 'bond': return { icon: '🔗', title: 'Network Bond (Link Aggregation)' };
        case 'vlan': return { icon: '🏷️', title: 'VLAN Interface' };
        case 'eth': return { icon: '🔌', title: 'Physical Ethernet' };
        case 'OVSBridge': return { icon: '🌐', title: 'Open vSwitch Bridge' };
        case 'OVSIntPort': return { icon: '🔀', title: 'OVS Internal Port' };
        case 'OVSPort': return { icon: '🔀', title: 'OVS Port' };
        default: return { icon: '📡', title: `Network Interface (${type})` };
      }
    };

    // Format interface label with IP if available
    const getIfaceLabel = (iface: NetworkInterface) => {
      let label = iface.iface;
      if (iface.address) label += ` - ${iface.address}`;
      else if (iface.cidr) label += ` - ${iface.cidr}`;
      return label;
    };

    // Group interfaces by type for better organization
    const groupByType = (interfaces: NetworkInterface[]) => {
      const bridges = interfaces.filter(i => i.type === 'bridge' || i.type === 'OVSBridge');
      const bonds = interfaces.filter(i => i.type === 'bond');
      const vlans = interfaces.filter(i => i.type === 'vlan');
      const physical = interfaces.filter(i => i.type === 'eth');
      const other = interfaces.filter(i => !['bridge', 'OVSBridge', 'bond', 'vlan', 'eth'].includes(i.type));
      return { bridges, bonds, vlans, physical, other };
    };

    // Render a single interface node
    const renderInterface = (iface: NetworkInterface) => {
      const { icon, title } = getIfaceIcon(iface.type);
      return (
        <TreeNode
          key={`${iface.cluster}-${iface.node}-${iface.iface}`}
          icon={icon}
          iconTitle={title}
          label={getIfaceLabel(iface)}
          status={iface.active === 1 ? 'online' : 'stopped'}
          isSelected={isSelected({ type: 'network', id: `${iface.node}-${iface.iface}`, name: iface.iface, node: iface.node, cluster: iface.cluster })}
          onClick={() => setSelectedObject({ type: 'network', id: `${iface.node}-${iface.iface}`, name: iface.iface, node: iface.node, cluster: iface.cluster })}
        />
      );
    };

    return (
      <div className="py-2">
        <TreeNode icon="🌐" label="Networks" defaultExpanded count={networkInterfaces.length}>
          {Object.values(interfacesByNode)
            .sort((a, b) => a.node.localeCompare(b.node))
            .map(({ node, cluster, interfaces }) => {
              const grouped = groupByType(interfaces);
              return (
                <TreeNode
                  key={`${cluster}-${node}`}
                  icon="🖥"
                  label={node}
                  count={interfaces.length}
                  defaultExpanded
                >
                  {grouped.bridges.length > 0 && (
                    <TreeNode icon="🌉" label="Bridges" count={grouped.bridges.length} defaultExpanded>
                      {grouped.bridges.sort((a, b) => a.iface.localeCompare(b.iface)).map(renderInterface)}
                    </TreeNode>
                  )}
                  {grouped.bonds.length > 0 && (
                    <TreeNode icon="🔗" label="Bonds" count={grouped.bonds.length} defaultExpanded>
                      {grouped.bonds.sort((a, b) => a.iface.localeCompare(b.iface)).map(renderInterface)}
                    </TreeNode>
                  )}
                  {grouped.vlans.length > 0 && (
                    <TreeNode icon="🏷️" label="VLANs" count={grouped.vlans.length} defaultExpanded>
                      {grouped.vlans.sort((a, b) => a.iface.localeCompare(b.iface)).map(renderInterface)}
                    </TreeNode>
                  )}
                  {grouped.physical.length > 0 && (
                    <TreeNode icon="🔌" label="Physical" count={grouped.physical.length} defaultExpanded>
                      {grouped.physical.sort((a, b) => a.iface.localeCompare(b.iface)).map(renderInterface)}
                    </TreeNode>
                  )}
                  {grouped.other.length > 0 && (
                    <TreeNode icon="📡" label="Other" count={grouped.other.length} defaultExpanded>
                      {grouped.other.sort((a, b) => a.iface.localeCompare(b.iface)).map(renderInterface)}
                    </TreeNode>
                  )}
                </TreeNode>
              );
            })}
        </TreeNode>
      </div>
    );
  }

  return null;
});
