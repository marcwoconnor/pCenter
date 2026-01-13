import { useState, useMemo, useEffect } from 'react';
import { useCluster } from '../context/ClusterContext';
import type { SelectedObject } from '../context/ClusterContext';
import type { Guest, Storage, ClusterInfo, NetworkInterface } from '../types';
import { ContextMenu, type MenuItem } from './ContextMenu';
import { MigrateDialog } from './MigrateDialog';
import { api } from '../api/client';

interface ContextMenuState {
  x: number;
  y: number;
  items: MenuItem[];
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
}

function TreeNode({ icon, label, status, badge, isSelected, onClick, onContextMenu, children, defaultExpanded = false, count }: TreeNodeProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const hasChildren = Boolean(children);

  const statusColors = {
    online: 'bg-green-500',
    running: 'bg-green-500',
    stopped: 'bg-gray-400',
    warning: 'bg-yellow-500',
  };

  return (
    <div>
      <div
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
          isSelected
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

export function InventoryTree({ view, filter = '' }: InventoryTreeProps) {
  const { clusters, nodes, guests, storage, selectedObject, setSelectedObject, performAction, openConsole } = useCluster();
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [migrateGuest, setMigrateGuest] = useState<Guest | null>(null);
  const [networkInterfaces, setNetworkInterfaces] = useState<NetworkInterface[]>([]);

  const filterLower = filter.toLowerCase();

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

  // Context menu builders
  const getGuestMenuItems = (guest: Guest): MenuItem[] => {
    const isRunning = guest.status === 'running';
    const type = guest.type === 'qemu' ? 'vm' : 'ct';

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

  const showContextMenu = (e: React.MouseEvent, items: MenuItem[]) => {
    setContextMenu({ x: e.clientX, y: e.clientY, items });
  };

  const closeContextMenu = () => setContextMenu(null);

  // Render guest node with context menu
  const renderGuestNode = (guest: Guest, type: 'vm' | 'ct') => (
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
      onContextMenu={(e) => showContextMenu(e, getGuestMenuItems(guest))}
    />
  );

  // Hosts & Clusters View - Now with cluster hierarchy
  if (view === 'hosts') {
    const totalNodes = sortedNodes.length;

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
        <TreeNode
          icon="🏢"
          label="pCenter"
          defaultExpanded
          count={totalNodes}
          isSelected={isSelected({ type: 'datacenter', id: 'datacenter', name: 'pCenter' })}
          onClick={() => setSelectedObject({ type: 'datacenter', id: 'datacenter', name: 'pCenter' })}
        >
          {sortedClusters.length === 0 ? (
            // Fallback if no clusters defined - show nodes directly
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
                      {vms.map((vm) => renderGuestNode(vm, 'vm'))}
                    </TreeNode>
                  )}
                  {cts.length > 0 && (
                    <TreeNode icon="📁" label="Containers" count={cts.length} defaultExpanded>
                      {cts.map((ct) => renderGuestNode(ct, 'ct'))}
                    </TreeNode>
                  )}
                </TreeNode>
              );
            })
          ) : (
            // Multi-cluster view
            sortedClusters.map((cluster) => {
              const clusterNodes = nodesByCluster[cluster.name] || [];

              return (
                <TreeNode
                  key={cluster.name}
                  icon="🏛"
                  label={cluster.name}
                  badge={<HABadge ha={cluster.ha} />}
                  isSelected={isSelected({ type: 'cluster', id: cluster.name, name: cluster.name, cluster: cluster.name })}
                  onClick={() => setSelectedObject({ type: 'cluster', id: cluster.name, name: cluster.name, cluster: cluster.name })}
                  onContextMenu={(e) => showContextMenu(e, getClusterMenuItems(cluster.name))}
                  defaultExpanded
                  count={clusterNodes.length}
                >
                  {clusterNodes.map((node) => {
                    const nodeGuests = (guestsByClusterNode[cluster.name]?.[node.node] || []).filter(filterGuest);
                    const vms = nodeGuests.filter(g => g.type === 'qemu');
                    const cts = nodeGuests.filter(g => g.type === 'lxc');

                    return (
                      <TreeNode
                        key={`${cluster.name}-${node.node}`}
                        icon="🖥"
                        label={node.node}
                        status={node.status === 'online' ? 'online' : 'stopped'}
                        isSelected={isSelected({ type: 'node', id: node.node, name: node.node, cluster: cluster.name })}
                        onClick={() => setSelectedObject({ type: 'node', id: node.node, name: node.node, cluster: cluster.name })}
                        onContextMenu={(e) => showContextMenu(e, getNodeMenuItems(node.node, cluster.name))}
                        defaultExpanded
                        count={nodeGuests.length}
                      >
                        {vms.length > 0 && (
                          <TreeNode icon="📁" label="Virtual Machines" count={vms.length} defaultExpanded>
                            {vms.map((vm) => renderGuestNode(vm, 'vm'))}
                          </TreeNode>
                        )}
                        {cts.length > 0 && (
                          <TreeNode icon="📁" label="Containers" count={cts.length} defaultExpanded>
                            {cts.map((ct) => renderGuestNode(ct, 'ct'))}
                          </TreeNode>
                        )}
                      </TreeNode>
                    );
                  })}
                </TreeNode>
              );
            })
          )}
        </TreeNode>
      </div>
    );
  }

  // VMs & Templates View
  if (view === 'vms') {
    const vms = sortedGuests.filter(g => g.type === 'qemu').filter(filterGuest);
    const cts = sortedGuests.filter(g => g.type === 'lxc').filter(filterGuest);

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
        <TreeNode icon="📁" label="Virtual Machines" defaultExpanded count={vms.length}>
          {vms.map((vm) => renderGuestNode(vm, 'vm'))}
        </TreeNode>
        <TreeNode icon="📁" label="Containers" defaultExpanded count={cts.length}>
          {cts.map((ct) => renderGuestNode(ct, 'ct'))}
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
