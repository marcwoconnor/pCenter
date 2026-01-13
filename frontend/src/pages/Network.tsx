import { useState, useEffect, useRef } from 'react';
import { Layout } from '../components/Layout';
import { InventoryTree } from '../components/InventoryTree';
import { useCluster } from '../context/ClusterContext';
import { api } from '../api/client';
import type { NetworkInterface, SDNZone, SDNVNet, SDNSubnet } from '../types';

type TabType = 'interfaces' | 'zones' | 'vnets';

export function NetworkPage() {
  const { clusters, nodes } = useCluster();
  const [filter, setFilter] = useState('');
  const [activeTab, setActiveTab] = useState<TabType>('interfaces');
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [interfaces, setInterfaces] = useState<NetworkInterface[]>([]);
  const [zones, setZones] = useState<SDNZone[]>([]);
  const [vnets, setVNets] = useState<SDNVNet[]>([]);
  const [subnets, setSubnets] = useState<SDNSubnet[]>([]);
  const hasFetched = useRef(false);

  useEffect(() => {
    async function fetchNetworkData() {
      // Get cluster names from clusters array or derive from nodes
      let clusterNames: string[] = [];
      if (clusters && clusters.length > 0) {
        clusterNames = clusters.map(c => c.name);
      } else if (nodes && nodes.length > 0) {
        // Derive unique cluster names from nodes
        clusterNames = [...new Set(nodes.map(n => n.cluster).filter(Boolean))];
      }

      if (clusterNames.length === 0) {
        // No clusters yet, try default
        clusterNames = ['default'];
      }

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
  }, [clusters, nodes]);

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

  const tabs: { id: TabType; label: string; count: number }[] = [
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
                {tab.count > 0 && (
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
