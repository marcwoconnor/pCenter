import { useState, useMemo, useEffect, useCallback } from 'react';
import { useCluster } from '../context/ClusterContext';
import { useFolders } from '../context/FolderContext';
import type { SelectedObject } from '../context/ClusterContext';
import type { Guest, Storage, ClusterInfo, NetworkInterface, Folder, TreeView, Datacenter, InventoryCluster } from '../types';
import { ContextMenu, type MenuItem } from './ContextMenu';
import { MigrateDialog } from './MigrateDialog';
import { FolderDialog } from './FolderDialog';
import { DatacenterDialog } from './DatacenterDialog';
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
          if (hasChildren) setExpanded(!expanded);
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
          <span className="w-4 text-center text-gray-400">{expanded ? '▼' : '▶'}</span>
        ) : (
          <span className="w-4" />
        )}
        <span>{icon}</span>
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
function HABadge({ ha }: { ha?: ClusterInfo['ha'] }) {
  if (!ha?.enabled) return null;

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

export function InventoryTree({ view, filter = '' }: InventoryTreeProps) {
  const { clusters, nodes, guests, storage, selectedObject, setSelectedObject, performAction, openConsole } = useCluster();
  const { hostsTree, vmsTree, createFolder, renameFolder, deleteFolder, moveFolder, moveResource } = useFolders();
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [migrateGuest, setMigrateGuest] = useState<Guest | null>(null);
  const [networkInterfaces, setNetworkInterfaces] = useState<NetworkInterface[]>([]);
  const [folderDialog, setFolderDialog] = useState<FolderDialogState | null>(null);

  // Datacenter/cluster inventory state
  const [datacenters, setDatacenters] = useState<Datacenter[]>([]);
  const [orphanClusters, setOrphanClusters] = useState<InventoryCluster[]>([]);
  const [datacenterDialog, setDatacenterDialog] = useState<DatacenterDialogState | null>(null);

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

  // Fetch network interfaces when in network view
  useEffect(() => {
    if (view !== 'network') return;

    async function fetchNetworkData() {
      // Derive cluster names from nodes
      let clusterNames: string[] = [];
      if (clusters && clusters.length > 0) {
        clusterNames = clusters.map(c => c.name);
      } else if (nodes && nodes.length > 0) {
        clusterNames = [...new Set(nodes.map(n => n.cluster).filter(Boolean))];
      }
      if (clusterNames.length === 0) return;

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
    const interval = setInterval(fetchNetworkData, 30000);
    return () => clearInterval(interval);
  }, [view, clusters, nodes]);

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
        label: 'Snapshot',
        icon: '📷',
        action: () => alert('Snapshots not yet implemented'),
      },
      {
        label: 'Clone',
        icon: '📋',
        action: () => alert('Clone not yet implemented'),
      },
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
    ];
  };

  const getNodeMenuItems = (_nodeName: string, _cluster: string): MenuItem[] => {
    return [
      {
        label: 'Shell',
        icon: '🖥',
        action: () => alert('Node shell not yet implemented'),
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Create VM',
        icon: '💻',
        action: () => alert('Create VM not yet implemented'),
      },
      {
        label: 'Create Container',
        icon: '📦',
        action: () => alert('Create Container not yet implemented'),
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Reboot',
        icon: '🔄',
        action: () => alert('Node reboot not yet implemented'),
        danger: true,
      },
    ];
  };

  const getClusterMenuItems = (_clusterName: string): MenuItem[] => {
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
        action: () => alert('Create VM not yet implemented'),
      },
      {
        label: 'Create Container',
        icon: '📦',
        action: () => alert('Create Container not yet implemented'),
      },
    ];
  };

  // Get datacenter context menu items
  const getDatacenterMenuItems = (dc: Datacenter): MenuItem[] => {
    return [
      {
        label: 'Add Cluster',
        icon: '➕',
        action: () => setDatacenterDialog({
          mode: 'create-cluster',
          parentDatacenterId: dc.id,
        }),
      },
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

    return [
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

  // Get root menu items for hosts view
  const getRootHostsMenuItems = (): MenuItem[] => {
    return [
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

  const getStorageMenuItems = (_s: Storage): MenuItem[] => {
    return [
      {
        label: 'Upload ISO',
        icon: '📀',
        action: () => alert('ISO upload not yet implemented'),
      },
      {
        label: 'Upload Template',
        icon: '📄',
        action: () => alert('Template upload not yet implemented'),
      },
      { label: '', action: () => {}, divider: true },
      {
        label: 'Browse Content',
        icon: '📂',
        action: () => alert('Storage browser not yet implemented'),
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

  // Get datacenter/root context menu items (folders only in VMs view per vCenter)
  const getRootMenuItems = (treeView: TreeView): MenuItem[] => {
    if (treeView !== 'vms') return [];
    return [
      {
        label: 'New Folder',
        icon: '📁',
        action: () => setFolderDialog({
          mode: 'create',
          treeView,
        }),
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
                  <TreeNode icon="📁" label="Virtual Machines" count={vms.length} defaultExpanded>
                    {vms.map((vm) => renderGuestNode(vm, 'vm', treeView))}
                  </TreeNode>
                )}
                {cts.length > 0 && (
                  <TreeNode icon="📁" label="Containers" count={cts.length} defaultExpanded>
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

  // Helper to render a cluster node with its hosts
  const renderClusterWithNodes = (clusterName: string, inventoryCluster?: InventoryCluster) => {
    const clusterNodes = nodesByCluster[clusterName] || [];
    const runtimeCluster = clusters.find(c => c.name === clusterName);

    return (
      <TreeNode
        key={clusterName}
        icon="🏛"
        label={clusterName}
        badge={<>
          {inventoryCluster && !inventoryCluster.enabled && (
            <span className="text-xs px-1.5 py-0.5 rounded bg-gray-600 text-gray-300">disabled</span>
          )}
          <HABadge ha={runtimeCluster?.ha} />
        </>}
        isSelected={isSelected({ type: 'cluster', id: clusterName, name: clusterName, cluster: clusterName })}
        onClick={() => setSelectedObject({ type: 'cluster', id: clusterName, name: clusterName, cluster: clusterName })}
        onContextMenu={(e) => {
          // Show inventory menu if we have cluster config, otherwise runtime menu
          if (inventoryCluster) {
            showContextMenu(e, getInventoryClusterMenuItems(inventoryCluster));
          } else {
            showContextMenu(e, getClusterMenuItems(clusterName));
          }
        }}
        defaultExpanded
        count={clusterNodes.length}
      >
        {clusterNodes.map((node) => {
          const nodeGuests = (guestsByClusterNode[clusterName]?.[node.node] || []).filter(filterGuest);
          const vms = nodeGuests.filter(g => g.type === 'qemu');
          const cts = nodeGuests.filter(g => g.type === 'lxc');

          return (
            <TreeNode
              key={`${clusterName}-${node.node}`}
              icon="🖥"
              label={node.node}
              status={node.status === 'online' ? 'online' : 'stopped'}
              isSelected={isSelected({ type: 'node', id: node.node, name: node.node, cluster: clusterName })}
              onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, cluster: clusterName })}
              onContextMenu={(e) => showContextMenu(e, getNodeMenuItems(node.node, clusterName))}
              defaultExpanded
              count={nodeGuests.length}
            >
              {vms.length > 0 && (
                <TreeNode icon="📁" label="Virtual Machines" count={vms.length} defaultExpanded>
                  {vms.map((vm) => renderGuestNode(vm, 'vm', 'hosts'))}
                </TreeNode>
              )}
              {cts.length > 0 && (
                <TreeNode icon="📁" label="Containers" count={cts.length} defaultExpanded>
                  {cts.map((ct) => renderGuestNode(ct, 'ct', 'hosts'))}
                </TreeNode>
              )}
            </TreeNode>
          );
        })}
      </TreeNode>
    );
  };

  // Hosts & Clusters View - with datacenter hierarchy
  if (view === 'hosts') {
    const totalNodes = sortedNodes.length;
    const hasDatacenters = datacenters.length > 0 || orphanClusters.length > 0;

    return (
      <div className="py-2">
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
        {datacenterDialog && (
          <DatacenterDialog
            mode={datacenterDialog.mode}
            datacenter={datacenterDialog.datacenter}
            cluster={datacenterDialog.cluster}
            parentDatacenterId={datacenterDialog.parentDatacenterId}
            datacenters={datacenters}
            onSubmit={async () => {
              setDatacenterDialog(null);
              await fetchDatacenterTree();
            }}
            onClose={() => setDatacenterDialog(null)}
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
                  count={dc.clusters?.length || 0}
                  isSelected={isSelected({ type: 'datacenter', id: dc.id, name: dc.name })}
                  onClick={() => setSelectedObject({ type: 'datacenter', id: dc.id, name: dc.name })}
                  onContextMenu={(e) => showContextMenu(e, getDatacenterMenuItems(dc))}
                >
                  {dc.clusters?.map((invCluster) =>
                    renderClusterWithNodes(invCluster.name, invCluster)
                  )}
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
                    <TreeNode icon="📁" label="Virtual Machines" count={vms.length} defaultExpanded>
                      {vms.map((vm) => renderGuestNode(vm, 'vm', 'hosts'))}
                    </TreeNode>
                  )}
                  {cts.length > 0 && (
                    <TreeNode icon="📁" label="Containers" count={cts.length} defaultExpanded>
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
      <div className="py-2">
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
        {folderDialog && (
          <FolderDialog
            mode={folderDialog.mode}
            initialName={folderDialog.initialName}
            onSubmit={handleFolderDialogSubmit}
            onClose={() => setFolderDialog(null)}
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
      <div className="py-2">
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
            {sharedStorage.map((s) => (
              <TreeNode
                key={`${s.cluster}-${s.node}-${s.storage}`}
                icon="💾"
                label={s.storage}
                status={s.status === 'available' ? 'online' : 'warning'}
                isSelected={isSelected({ type: 'storage', id: `${s.node}-${s.storage}`, name: s.storage, cluster: s.cluster })}
                onClick={() => setSelectedObject({ type: 'storage', id: `${s.node}-${s.storage}`, name: s.storage, node: s.node, cluster: s.cluster })}
                onContextMenu={(e) => showContextMenu(e, getStorageMenuItems(s))}
              />
            ))}
          </TreeNode>
        )}
        {sortedNodes.map((node) => {
          const nodeStorage = localByNode[node.node] || [];
          if (nodeStorage.length === 0) return null;
          return (
            <TreeNode key={`${node.cluster}-${node.node}`} icon="🖥" label={node.node} defaultExpanded count={nodeStorage.length}>
              {nodeStorage.map((s) => (
                <TreeNode
                  key={`${s.cluster}-${s.node}-${s.storage}`}
                  icon="💾"
                  label={s.storage}
                  status={s.status === 'available' ? 'online' : 'warning'}
                  isSelected={isSelected({ type: 'storage', id: `${s.node}-${s.storage}`, name: s.storage, cluster: s.cluster })}
                  onClick={() => setSelectedObject({ type: 'storage', id: `${s.node}-${s.storage}`, name: s.storage, node: s.node, cluster: s.cluster })}
                  onContextMenu={(e) => showContextMenu(e, getStorageMenuItems(s))}
                />
              ))}
            </TreeNode>
          );
        })}
      </div>
    );
  }

  // Network View - real interfaces
  if (view === 'network') {
    // Group interfaces by node
    const interfacesByNode = networkInterfaces.reduce((acc, iface) => {
      const key = `${iface.cluster}-${iface.node}`;
      if (!acc[key]) acc[key] = { node: iface.node, cluster: iface.cluster, interfaces: [] };
      acc[key].interfaces.push(iface);
      return acc;
    }, {} as Record<string, { node: string; cluster: string; interfaces: NetworkInterface[] }>);

    // Icon based on interface type
    const getIfaceIcon = (type: string) => {
      switch (type) {
        case 'bridge': return '🌉';
        case 'bond': return '🔗';
        case 'vlan': return '🏷️';
        case 'eth': return '🔌';
        case 'OVSBridge': return '🌐';
        default: return '📡';
      }
    };

    // Format interface label
    const getIfaceLabel = (iface: NetworkInterface) => {
      let label = `${iface.iface} (${iface.type})`;
      if (iface.address) label += ` - ${iface.address}`;
      else if (iface.cidr) label += ` - ${iface.cidr}`;
      return label;
    };

    return (
      <div className="py-2">
        <TreeNode icon="🌐" label="Networks" defaultExpanded count={networkInterfaces.length}>
          {Object.values(interfacesByNode)
            .sort((a, b) => a.node.localeCompare(b.node))
            .map(({ node, cluster, interfaces }) => (
              <TreeNode
                key={`${cluster}-${node}`}
                icon="🖥"
                label={node}
                count={interfaces.length}
                defaultExpanded
              >
                {interfaces
                  .sort((a, b) => a.iface.localeCompare(b.iface))
                  .map((iface) => (
                    <TreeNode
                      key={`${cluster}-${node}-${iface.iface}`}
                      icon={getIfaceIcon(iface.type)}
                      label={getIfaceLabel(iface)}
                      status={iface.active ? 'online' : 'stopped'}
                    />
                  ))}
              </TreeNode>
            ))}
        </TreeNode>
      </div>
    );
  }

  return null;
}
