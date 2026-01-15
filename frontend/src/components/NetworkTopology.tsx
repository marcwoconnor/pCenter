import { useState, useEffect, useMemo } from 'react';
import { useCluster } from '../context/ClusterContext';
import { api } from '../api/client';
import type { NetworkInterface } from '../types';

interface NetworkTopologyProps {
  node: string;
  cluster: string;
}

interface BridgeData {
  name: string;
  type: string;
  address?: string;
  cidr?: string;
  gateway?: string;
  bridgePorts: string[];
  active: boolean;
  vlanAware?: boolean;
  connectedGuests: { vmid: number; name: string; type: 'vm' | 'ct'; nic: string; vlan?: number }[];
}

export function NetworkTopology({ node, cluster }: NetworkTopologyProps) {
  const [interfaces, setInterfaces] = useState<NetworkInterface[]>([]);
  const [loading, setLoading] = useState(true);
  const { guests } = useCluster();

  // Fetch network interfaces
  useEffect(() => {
    async function fetchData() {
      setLoading(true);
      try {
        const ifaces = await api.getClusterNetworkInterfaces(cluster, node);
        setInterfaces(ifaces);
      } catch (err) {
        console.error('Failed to fetch interfaces:', err);
      } finally {
        setLoading(false);
      }
    }
    fetchData();
  }, [node, cluster]);

  // Build bridge topology data
  const bridges = useMemo(() => {
    const bridgeList: BridgeData[] = [];
    const nodeGuests = guests.filter(g => g.node === node);

    // Find all bridges
    const bridgeIfaces = interfaces.filter(i => i.type === 'bridge' || i.type === 'OVSBridge');

    for (const bridge of bridgeIfaces) {
      // Parse bridge_ports - can be space-separated list
      const ports = bridge.bridge_ports?.split(/\s+/).filter(Boolean) || [];

      // Find connected guests (simplified - we'd need VM configs for accurate info)
      // For now, assume guests on same node may connect to any bridge
      const connected = nodeGuests.map(g => ({
        vmid: g.vmid,
        name: g.name,
        type: (g.type === 'qemu' ? 'vm' : 'ct') as 'vm' | 'ct',
        nic: 'net0', // placeholder
      }));

      bridgeList.push({
        name: bridge.iface,
        type: bridge.type,
        address: bridge.address,
        cidr: bridge.cidr,
        gateway: bridge.gateway,
        bridgePorts: ports,
        active: bridge.active === 1,
        connectedGuests: connected,
      });
    }

    return bridgeList;
  }, [interfaces, guests, node]);

  // Get physical NICs
  const physicalNics = useMemo(() => {
    return interfaces.filter(i => i.type === 'eth');
  }, [interfaces]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-500 dark:text-gray-400">
        Loading topology...
      </div>
    );
  }

  if (bridges.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-500 dark:text-gray-400">
        No virtual switches found on this node
      </div>
    );
  }

  return (
    <div className="space-y-8 p-4">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white flex items-center gap-2">
        <span>🌐</span> Virtual Switches - {node}
      </h2>

      <div className="flex flex-wrap gap-6">
        {bridges.map((bridge) => (
          <VirtualSwitchDiagram
            key={bridge.name}
            bridge={bridge}
            physicalNics={physicalNics}
          />
        ))}
      </div>

      {/* Legend */}
      <div className="mt-8 p-4 bg-gray-100 dark:bg-gray-800 rounded-lg">
        <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Legend</h4>
        <div className="flex flex-wrap gap-4 text-xs text-gray-600 dark:text-gray-400">
          <div className="flex items-center gap-1">
            <div className="w-4 h-4 bg-blue-500 rounded" /> Virtual Switch (Bridge)
          </div>
          <div className="flex items-center gap-1">
            <div className="w-4 h-4 bg-green-500 rounded" /> Physical NIC (Uplink)
          </div>
          <div className="flex items-center gap-1">
            <div className="w-4 h-4 bg-purple-500 rounded" /> VM / Container
          </div>
        </div>
      </div>
    </div>
  );
}

// Individual virtual switch diagram
function VirtualSwitchDiagram({
  bridge,
  physicalNics,
}: {
  bridge: BridgeData;
  physicalNics: NetworkInterface[];
}) {
  // Get physical NICs that are uplinks for this bridge
  const uplinks = physicalNics.filter(nic => bridge.bridgePorts.includes(nic.iface));
  // Also include non-physical ports (bonds, vlans)
  const otherPorts = bridge.bridgePorts.filter(p => !physicalNics.find(n => n.iface === p));

  // Limit displayed guests
  const displayedGuests = bridge.connectedGuests.slice(0, 6);
  const moreGuests = bridge.connectedGuests.length - 6;

  return (
    <div className="flex flex-col items-center min-w-[280px]">
      {/* Connected VMs/CTs (top) */}
      <div className="flex flex-wrap justify-center gap-2 mb-2 min-h-[60px]">
        {displayedGuests.map((guest) => (
          <div
            key={guest.vmid}
            className="flex flex-col items-center"
          >
            <div className="w-16 h-12 bg-purple-600 dark:bg-purple-700 rounded-t-lg flex flex-col items-center justify-center text-white text-xs shadow-md">
              <span>{guest.type === 'vm' ? '💻' : '📦'}</span>
              <span className="font-mono">{guest.vmid}</span>
            </div>
            {/* Connector line */}
            <div className="w-0.5 h-3 bg-purple-400" />
          </div>
        ))}
        {moreGuests > 0 && (
          <div className="flex flex-col items-center">
            <div className="w-16 h-12 bg-purple-400 dark:bg-purple-600 rounded-t-lg flex items-center justify-center text-white text-xs shadow-md">
              +{moreGuests} more
            </div>
            <div className="w-0.5 h-3 bg-purple-400" />
          </div>
        )}
      </div>

      {/* Virtual Switch (middle) */}
      <div className="relative w-full">
        {/* Connection lines from VMs to switch */}
        <div className="absolute -top-0 left-0 right-0 h-0.5 bg-gray-300 dark:bg-gray-600" />

        <div className={`w-full p-4 rounded-lg shadow-lg ${
          bridge.active
            ? 'bg-blue-600 dark:bg-blue-700'
            : 'bg-gray-500 dark:bg-gray-600'
        }`}>
          <div className="flex items-center justify-between text-white">
            <div className="flex items-center gap-2">
              <span className="text-xl">🌉</span>
              <div>
                <div className="font-semibold">{bridge.name}</div>
                <div className="text-xs opacity-80">
                  {bridge.type === 'OVSBridge' ? 'Open vSwitch' : 'Linux Bridge'}
                </div>
              </div>
            </div>
            <div className={`w-3 h-3 rounded-full ${bridge.active ? 'bg-green-400' : 'bg-red-400'}`} />
          </div>

          {/* IP Address if configured */}
          {(bridge.address || bridge.cidr) && (
            <div className="mt-2 text-xs text-blue-100 font-mono">
              {bridge.cidr || bridge.address}
              {bridge.gateway && ` gw ${bridge.gateway}`}
            </div>
          )}
        </div>

        {/* Connection lines to physical NICs */}
        <div className="absolute -bottom-0 left-0 right-0 h-0.5 bg-gray-300 dark:bg-gray-600" />
      </div>

      {/* Physical NICs / Uplinks (bottom) */}
      <div className="flex flex-wrap justify-center gap-2 mt-2">
        {uplinks.length === 0 && otherPorts.length === 0 ? (
          <div className="text-xs text-gray-500 dark:text-gray-400 py-2">
            No uplinks configured
          </div>
        ) : (
          <>
            {uplinks.map((nic) => (
              <div key={nic.iface} className="flex flex-col items-center">
                {/* Connector line */}
                <div className="w-0.5 h-3 bg-green-400" />
                <div className={`w-20 h-14 rounded-b-lg flex flex-col items-center justify-center text-white text-xs shadow-md ${
                  nic.active === 1 ? 'bg-green-600 dark:bg-green-700' : 'bg-gray-500'
                }`}>
                  <span>🔌</span>
                  <span className="font-mono">{nic.iface}</span>
                  <span className={`w-2 h-2 rounded-full mt-1 ${nic.active === 1 ? 'bg-green-300' : 'bg-red-400'}`} />
                </div>
              </div>
            ))}
            {otherPorts.map((port) => (
              <div key={port} className="flex flex-col items-center">
                {/* Connector line */}
                <div className="w-0.5 h-3 bg-yellow-400" />
                <div className="w-20 h-14 bg-yellow-600 dark:bg-yellow-700 rounded-b-lg flex flex-col items-center justify-center text-white text-xs shadow-md">
                  <span>🔗</span>
                  <span className="font-mono">{port}</span>
                </div>
              </div>
            ))}
          </>
        )}
      </div>
    </div>
  );
}
