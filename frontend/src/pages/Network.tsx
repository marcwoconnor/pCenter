import { useState, useEffect, useRef, useMemo } from 'react';
import { Layout } from '../components/Layout';
import { InventoryTree } from '../components/InventoryTree';
import { useCluster } from '../context/ClusterContext';
import { api } from '../api/client';
import type { NetworkInterface, SDNZone, SDNVNet, SDNSubnet, Guest } from '../types';

type TabType = 'topology' | 'interfaces' | 'zones' | 'vnets';

export function NetworkPage() {
  const { clusters, nodes, guests } = useCluster();
  const [filter, setFilter] = useState('');
  const [activeTab, setActiveTab] = useState<TabType>('topology');
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [interfaces, setInterfaces] = useState<NetworkInterface[]>([]);
  const [zones, setZones] = useState<SDNZone[]>([]);
  const [vnets, setVNets] = useState<SDNVNet[]>([]);
  const [subnets, setSubnets] = useState<SDNSubnet[]>([]);
  const hasFetched = useRef(false);

  // Stable cluster names string to avoid effect re-running on every WebSocket update
  const clusterNamesKey = useMemo(() => {
    let names: string[] = [];
    if (clusters && clusters.length > 0) {
      names = clusters.map(c => c.name);
    } else if (nodes && nodes.length > 0) {
      names = [...new Set(nodes.map(n => n.cluster).filter(Boolean))];
    }
    return names.length > 0 ? names.sort().join(',') : 'default';
  }, [clusters, nodes]);

  useEffect(() => {
    const clusterNames = clusterNamesKey.split(',');

    async function fetchNetworkData() {
      // Only show loading on initial fetch, not refreshes
      if (!hasFetched.current) {
        setIsLoading(true);
      }
      setError(null);

      try {
        // Fetch network data from all clusters
        const allInterfaces: NetworkInterface[] = [];
        const allZones: SDNZone[] = [];
        const allVNets: SDNVNet[] = [];
        const allSubnets: SDNSubnet[] = [];

        for (const clusterName of clusterNames) {
          try {
            const data = await api.getClusterNetwork(clusterName);
            allInterfaces.push(...data.interfaces);
            allZones.push(...data.sdn_zones);
            allVNets.push(...data.sdn_vnets);
            allSubnets.push(...data.sdn_subnets);
          } catch (e) {
            console.warn(`Failed to fetch network data for ${clusterName}:`, e);
          }
        }

        // Sort interfaces for stable rendering order
        allInterfaces.sort((a, b) =>
          a.node.localeCompare(b.node) || a.iface.localeCompare(b.iface)
        );

        setInterfaces(allInterfaces);
        setZones(allZones);
        setVNets(allVNets);
        setSubnets(allSubnets);
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Failed to fetch network data');
      } finally {
        if (!hasFetched.current) {
          setIsLoading(false);
          hasFetched.current = true;
        }
      }
    }

    fetchNetworkData();
    const interval = setInterval(fetchNetworkData, 30000);
    return () => clearInterval(interval);
  }, [clusterNamesKey]);

  const sidebar = (
    <div className="flex flex-col h-full">
      <div className="p-2 border-b border-gray-200 dark:border-gray-700">
        <input
          type="text"
          placeholder="Search networks..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
        />
      </div>
      <div className="flex-1 overflow-y-auto">
        <InventoryTree view="network" filter={filter} />
      </div>
    </div>
  );

  // Get unique nodes for topology view
  const nodeNames = useMemo(() => {
    return [...new Set(interfaces.map(i => i.node))].sort();
  }, [interfaces]);

  const tabs: { id: TabType; label: string; count?: number }[] = [
    { id: 'topology', label: 'Virtual Switches' },
    { id: 'interfaces', label: 'Interfaces', count: interfaces.length },
    { id: 'zones', label: 'SDN Zones', count: zones.length },
    { id: 'vnets', label: 'VNets', count: vnets.length },
  ];

  // Filter interfaces by search
  const filteredInterfaces = interfaces.filter(i =>
    !filter ||
    i.iface.toLowerCase().includes(filter.toLowerCase()) ||
    i.node.toLowerCase().includes(filter.toLowerCase()) ||
    i.type.toLowerCase().includes(filter.toLowerCase())
  );

  // Group interfaces by type
  const bridges = filteredInterfaces.filter(i => i.type === 'bridge');
  const bonds = filteredInterfaces.filter(i => i.type === 'bond');
  const vlans = filteredInterfaces.filter(i => i.type === 'vlan');
  const physical = filteredInterfaces.filter(i => i.type === 'eth');
  const other = filteredInterfaces.filter(i => !['bridge', 'bond', 'vlan', 'eth'].includes(i.type));

  return (
    <Layout sidebar={sidebar}>
      <div className="flex-1 overflow-auto p-6">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white mb-4">Network Configuration</h1>

        {/* Tabs */}
        <div className="border-b border-gray-200 dark:border-gray-700 mb-4">
          <nav className="-mb-px flex space-x-6">
            {tabs.map(tab => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`py-2 px-1 border-b-2 font-medium text-sm ${
                  activeTab === tab.id
                    ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                    : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
                }`}
              >
                {tab.label}
                {tab.count !== undefined && tab.count > 0 && (
                  <span className="ml-2 px-2 py-0.5 text-xs rounded-full bg-gray-100 dark:bg-gray-700">
                    {tab.count}
                  </span>
                )}
              </button>
            ))}
          </nav>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <div className="text-gray-500">Loading network data...</div>
          </div>
        ) : error ? (
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
            <div className="text-red-800 dark:text-red-200">{error}</div>
          </div>
        ) : (
          <>
            {/* Virtual Switches Topology Tab */}
            {activeTab === 'topology' && (
              <div className="space-y-8">
                {nodeNames.length === 0 ? (
                  <div className="text-center py-8 text-gray-500">
                    No network interfaces found
                  </div>
                ) : (
                  nodeNames.map(nodeName => (
                    <NodeTopology
                      key={nodeName}
                      nodeName={nodeName}
                      interfaces={interfaces.filter(i => i.node === nodeName)}
                      guests={guests.filter(g => g.node === nodeName)}
                    />
                  ))
                )}
              </div>
            )}

            {/* Interfaces Tab */}
            {activeTab === 'interfaces' && (
              <div className="space-y-6">
                {bridges.length > 0 && (
                  <InterfaceSection title="Bridges" interfaces={bridges} />
                )}
                {bonds.length > 0 && (
                  <InterfaceSection title="Bonds" interfaces={bonds} />
                )}
                {vlans.length > 0 && (
                  <InterfaceSection title="VLANs" interfaces={vlans} />
                )}
                {physical.length > 0 && (
                  <InterfaceSection title="Physical" interfaces={physical} />
                )}
                {other.length > 0 && (
                  <InterfaceSection title="Other" interfaces={other} />
                )}
                {filteredInterfaces.length === 0 && (
                  <div className="text-center py-8 text-gray-500">
                    No network interfaces found
                  </div>
                )}
              </div>
            )}

            {/* SDN Zones Tab */}
            {activeTab === 'zones' && (
              <div className="space-y-4">
                {zones.length === 0 ? (
                  <div className="text-center py-8 text-gray-500">
                    No SDN zones configured. SDN can be enabled in Proxmox under Datacenter → SDN.
                  </div>
                ) : (
                  <div className="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
                    <table className="min-w-full">
                      <thead className="bg-gray-50 dark:bg-gray-700">
                        <tr>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Zone</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Type</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Bridge</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Nodes</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">IPAM</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">MTU</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                        {zones.map((zone) => (
                          <tr key={`${zone.cluster}-${zone.zone}`} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                            <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">{zone.zone}</td>
                            <td className="px-4 py-3 text-sm">
                              <ZoneTypeBadge type={zone.type} />
                            </td>
                            <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{zone.bridge || '-'}</td>
                            <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{zone.nodes || 'all'}</td>
                            <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{zone.ipam || '-'}</td>
                            <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{zone.mtu || 'default'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            )}

            {/* VNets Tab */}
            {activeTab === 'vnets' && (
              <div className="space-y-4">
                {vnets.length === 0 ? (
                  <div className="text-center py-8 text-gray-500">
                    No VNets configured. Create VNets within SDN zones.
                  </div>
                ) : (
                  <div className="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
                    <table className="min-w-full">
                      <thead className="bg-gray-50 dark:bg-gray-700">
                        <tr>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">VNet</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Zone</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Tag</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Alias</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Subnets</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                        {vnets.map((vnet) => {
                          const vnetSubnets = subnets.filter(s => s.vnet === vnet.vnet);
                          return (
                            <tr key={`${vnet.cluster}-${vnet.vnet}`} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                              <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">{vnet.vnet}</td>
                              <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{vnet.zone}</td>
                              <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{vnet.tag || '-'}</td>
                              <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">{vnet.alias || '-'}</td>
                              <td className="px-4 py-3 text-sm">
                                {vnetSubnets.length > 0 ? (
                                  <div className="space-y-1">
                                    {vnetSubnets.map(s => (
                                      <div key={s.subnet} className="text-xs">
                                        <span className="font-mono">{s.subnet}</span>
                                        {s.gateway && <span className="text-gray-400 ml-1">gw: {s.gateway}</span>}
                                      </div>
                                    ))}
                                  </div>
                                ) : (
                                  <span className="text-gray-400">-</span>
                                )}
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </Layout>
  );
}

// Helper components

function InterfaceSection({ title, interfaces }: { title: string; interfaces: NetworkInterface[] }) {
  return (
    <div>
      <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">{title}</h3>
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
        <table className="min-w-full">
          <thead className="bg-gray-50 dark:bg-gray-700">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Node</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Interface</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Type</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">IP/CIDR</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Details</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Status</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
            {interfaces.map((iface) => (
              <tr key={`${iface.cluster}-${iface.node}-${iface.iface}`} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100">{iface.node}</td>
                <td className="px-4 py-3 text-sm font-medium font-mono text-gray-900 dark:text-gray-100">{iface.iface}</td>
                <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                  <InterfaceTypeBadge type={iface.type} />
                </td>
                <td className="px-4 py-3 text-sm font-mono text-gray-500 dark:text-gray-400">
                  {iface.cidr || iface.address || '-'}
                </td>
                <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                  <InterfaceDetails iface={iface} />
                </td>
                <td className="px-4 py-3 text-sm">
                  <span className={`px-2 py-0.5 rounded text-xs ${
                    iface.active
                      ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
                      : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
                  }`}>
                    {iface.active ? 'active' : 'inactive'}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function InterfaceTypeBadge({ type }: { type: string }) {
  const colors: Record<string, string> = {
    bridge: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
    bond: 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200',
    vlan: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
    eth: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
    OVSBridge: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200',
  };
  return (
    <span className={`px-2 py-0.5 rounded text-xs ${colors[type] || colors.eth}`}>
      {type}
    </span>
  );
}

function ZoneTypeBadge({ type }: { type: string }) {
  const colors: Record<string, string> = {
    simple: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
    vlan: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
    qinq: 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200',
    vxlan: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
    evpn: 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200',
  };
  return (
    <span className={`px-2 py-0.5 rounded text-xs ${colors[type] || colors.simple}`}>
      {type}
    </span>
  );
}

function InterfaceDetails({ iface }: { iface: NetworkInterface }) {
  const details: string[] = [];

  if (iface.bridge_ports) {
    details.push(`ports: ${iface.bridge_ports}`);
  }
  if (iface.slaves) {
    details.push(`slaves: ${iface.slaves}`);
  }
  if (iface.bond_mode) {
    details.push(`mode: ${iface.bond_mode}`);
  }
  if (iface['vlan-raw-device']) {
    details.push(`parent: ${iface['vlan-raw-device']}`);
  }
  if (iface['vlan-id']) {
    details.push(`vlan: ${iface['vlan-id']}`);
  }
  if (iface.mtu && iface.mtu !== 1500) {
    details.push(`mtu: ${iface.mtu}`);
  }
  if (iface.gateway) {
    details.push(`gw: ${iface.gateway}`);
  }

  return details.length > 0 ? (
    <span className="text-xs">{details.join(', ')}</span>
  ) : (
    <span className="text-gray-400">-</span>
  );
}

// Node topology diagram - vCenter-style virtual switch view
function NodeTopology({
  nodeName,
  interfaces,
  guests,
}: {
  nodeName: string;
  interfaces: NetworkInterface[];
  guests: Guest[];
}) {
  // Get bridges, VLANs, and physical NICs - sorted for stable render order
  const bridgeIfaces = interfaces
    .filter(i => i.type === 'bridge' || i.type === 'OVSBridge')
    .sort((a, b) => a.iface.localeCompare(b.iface));
  const vlanIfaces = interfaces
    .filter(i => i.type === 'vlan')
    .sort((a, b) => a.iface.localeCompare(b.iface));
  const physicalNics = interfaces.filter(i => i.type === 'eth');

  // Group guests by bridge they're connected to
  const guestsByBridge = useMemo(() => {
    const map: Record<string, Guest[]> = {};
    for (const guest of guests) {
      if (guest.nics) {
        for (const nic of guest.nics) {
          if (nic.bridge) {
            if (!map[nic.bridge]) map[nic.bridge] = [];
            // Avoid duplicates if guest has multiple NICs on same bridge
            if (!map[nic.bridge].find(g => g.vmid === guest.vmid)) {
              map[nic.bridge].push(guest);
            }
          }
        }
      }
    }
    return map;
  }, [guests]);

  if (bridgeIfaces.length === 0) {
    return null;
  }

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow-lg p-6">
      <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-6 flex items-center gap-2">
        <span>🖥</span> {nodeName}
      </h3>

      <div className="space-y-8">
        {bridgeIfaces.map(bridge => {
          // Parse bridge_ports
          const ports = bridge.bridge_ports?.split(/\s+/).filter(Boolean) || [];
          const uplinks = physicalNics.filter(nic => ports.includes(nic.iface));
          const otherPorts = ports.filter(p => !physicalNics.find(n => n.iface === p));
          // Find VLANs attached to this bridge (vlan-raw-device matches bridge name)
          const bridgeVlans = vlanIfaces.filter(v => v['vlan-raw-device'] === bridge.iface);
          // Get guests connected to this bridge - sorted by VMID for stable order.
          // IMPORTANT: Even though the backend now sorts guests, we sort again here
          // because guestsByBridge groups guests by bridge and the grouping process
          // can change order. Sorting by VMID ensures consistent visual positioning
          // and prevents VMs from jumping around on WebSocket updates.
          const bridgeGuests = (guestsByBridge[bridge.iface] || [])
            .slice()
            .sort((a, b) => a.vmid - b.vmid);

          const leftItems = bridgeVlans.length + Math.max(1, bridgeGuests.length);
          const rightItems = Math.max(1, uplinks.length + otherPorts.length);

          return (
            <div key={bridge.iface} className="relative">
              {/* Bridge label - outside, above */}
              <div className="mb-2 flex items-center gap-3">
                <div className={`w-3 h-3 rounded-full ${bridge.active === 1 ? 'bg-green-500' : 'bg-red-500'}`} />
                <span className="font-bold text-gray-900 dark:text-white">{bridge.iface}</span>
                <span className="text-sm text-gray-500 dark:text-gray-400">
                  {bridge.type === 'OVSBridge' ? 'Open vSwitch' : 'Linux Bridge'}
                </span>
                {(bridge.address || bridge.cidr) && (
                  <span className="text-sm font-mono text-gray-500 dark:text-gray-400">
                    {bridge.cidr || bridge.address}
                  </span>
                )}
                {bridgeGuests.length > 0 && (
                  <span className="text-sm text-purple-600 dark:text-purple-400">
                    ({bridgeGuests.length} guest{bridgeGuests.length !== 1 ? 's' : ''})
                  </span>
                )}
              </div>

              {/* Main diagram */}
              <div className="flex items-stretch">
                {/* Left: VLANs + VMs/CTs */}
                <div className="flex flex-col gap-1 min-w-[140px]">
                  {bridgeVlans.map((vlan) => (
                    <div key={vlan.iface} className="flex items-center justify-end h-10">
                      <div className={`px-3 py-1 rounded text-xs text-right ${
                        vlan.active === 1
                          ? 'bg-cyan-600 text-white'
                          : 'bg-gray-400 text-white'
                      }`}>
                        <div className="font-mono font-semibold">{vlan.iface}</div>
                        {(vlan.cidr || vlan.address) && (
                          <div className="text-[10px] opacity-80">{vlan.cidr || vlan.address}</div>
                        )}
                      </div>
                    </div>
                  ))}
                  {/* Display all guests */}
                  {bridgeGuests.length > 0 ? (
                    bridgeGuests.map((guest) => (
                      <div key={guest.vmid} className="flex items-center justify-end h-10">
                        <div className={`px-3 py-1 rounded text-xs text-right flex items-center gap-1 ${
                          guest.status === 'running'
                            ? 'bg-purple-600 text-white'
                            : 'bg-gray-400 text-white'
                        }`}>
                          <span>{guest.type === 'qemu' ? '💻' : '📦'}</span>
                          <div>
                            <div className="font-semibold">{guest.vmid}</div>
                            <div className="text-[10px] opacity-80 truncate max-w-[80px]">{guest.name}</div>
                          </div>
                        </div>
                      </div>
                    ))
                  ) : (
                    <div className="flex items-center justify-end h-10">
                      <div className="px-3 py-1 rounded bg-gray-300 dark:bg-gray-600 text-gray-600 dark:text-gray-300 text-xs italic">
                        No VMs
                      </div>
                    </div>
                  )}
                </div>

                {/* SVG connecting lines + switch */}
                <div className="relative w-40" style={{ height: `${Math.max(leftItems, rightItems) * 44}px` }}>
                  <svg className="absolute inset-0 w-full h-full">
                    {/* Left horizontal lines to switch */}
                    {Array.from({ length: leftItems }).map((_, idx) => {
                      const y = (idx * 44) + 20;
                      return (
                        <line
                          key={`l-${idx}`}
                          x1="0" y1={y}
                          x2="50%" y2={y}
                          stroke="#6b7280" strokeWidth="2"
                        />
                      );
                    })}
                    {/* Vertical bus inside switch */}
                    <line
                      x1="50%" y1="10"
                      x2="50%" y2={Math.max(leftItems, rightItems) * 44 - 10}
                      stroke="#3b82f6" strokeWidth="4"
                    />
                    {/* Right horizontal lines from switch */}
                    {Array.from({ length: rightItems }).map((_, idx) => {
                      const y = (idx * 44) + 20;
                      return (
                        <line
                          key={`r-${idx}`}
                          x1="50%" y1={y}
                          x2="100%" y2={y}
                          stroke="#6b7280" strokeWidth="2"
                        />
                      );
                    })}
                  </svg>
                  {/* Switch box overlay */}
                  <div className={`absolute left-1/2 top-0 bottom-0 w-6 -ml-3 rounded ${
                    bridge.active === 1 ? 'bg-blue-600' : 'bg-gray-500'
                  }`} />
                </div>

                {/* Right: Physical NICs */}
                <div className="flex flex-col gap-1 min-w-[120px]">
                  {uplinks.length === 0 && otherPorts.length === 0 ? (
                    <div className="flex items-center h-10">
                      <div className="text-xs text-gray-400 italic">No uplinks</div>
                    </div>
                  ) : (
                    <>
                      {uplinks.map(nic => (
                        <div key={nic.iface} className="flex items-center h-10">
                          <div className={`px-3 py-1 rounded flex items-center gap-2 ${
                            nic.active === 1 ? 'bg-green-600' : 'bg-gray-500'
                          } text-white`}>
                            <span className="font-mono text-sm">{nic.iface}</span>
                            <div className={`w-2 h-2 rounded-full ${nic.active === 1 ? 'bg-green-300' : 'bg-red-400'}`} />
                          </div>
                        </div>
                      ))}
                      {otherPorts.map(port => (
                        <div key={port} className="flex items-center h-10">
                          <div className="px-3 py-1 rounded bg-yellow-600 text-white">
                            <span className="font-mono text-sm">{port}</span>
                          </div>
                        </div>
                      ))}
                    </>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* Legend */}
      <div className="mt-6 pt-4 border-t border-gray-200 dark:border-gray-700">
        <div className="flex flex-wrap gap-4 text-xs text-gray-500 dark:text-gray-400">
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 bg-blue-600 rounded" /> Virtual Switch (Bridge)
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 bg-cyan-600 rounded" /> VLAN Interface (vmk)
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 bg-green-600 rounded" /> Physical NIC (Uplink)
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 bg-yellow-600 rounded" /> Bond / Other
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 bg-purple-600 rounded" /> VM / Container
          </div>
        </div>
      </div>
    </div>
  );
}
