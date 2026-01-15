import { useState, useEffect, memo } from 'react';
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

// Module-level cache to persist across component remounts
const topologyCache = new Map<string, { bridges: BridgeData[]; physicalNics: NetworkInterface[] }>();
// Track which keys have been fetched to prevent duplicate fetches
const fetchingKeys = new Set<string>();

export const NetworkTopology = memo(function NetworkTopology({ node, cluster }: NetworkTopologyProps) {
  const cacheKey = `${cluster}/${node}`;
  const [, forceUpdate] = useState(0);

  // Read directly from cache
  const cached = topologyCache.get(cacheKey);
  const bridges = cached?.bridges || [];
  const physicalNics = cached?.physicalNics || [];
  const loading = !cached && !fetchingKeys.has(cacheKey);

  // Fetch network interfaces and guests ONCE (skip if cached or already fetching)
  useEffect(() => {
    // Already have cached data or already fetching, skip
    if (topologyCache.has(cacheKey) || fetchingKeys.has(cacheKey)) return;
    fetchingKeys.add(cacheKey);

    async function fetchData() {
      try {
        // Fetch interfaces and guests snapshot (not live updates)
        const [ifaces, allGuests] = await Promise.all([
          api.getClusterNetworkInterfaces(cluster, node),
          api.getGuests(),
        ]);

        // Filter guests for this node
        const nodeGuests = allGuests.filter(g => g.node === node);

        // Find all bridges
        const bridgeIfaces = ifaces.filter(i => i.type === 'bridge' || i.type === 'OVSBridge');
        const bridgeList: BridgeData[] = [];

        for (const bridge of bridgeIfaces) {
          const ports = bridge.bridge_ports?.split(/\s+/).filter(Boolean) || [];

          // Find guests connected to this bridge via their NICs
          const connected: BridgeData['connectedGuests'] = [];
          for (const guest of nodeGuests) {
            if (!guest.nics) continue;
            for (const nic of guest.nics) {
              if (nic.bridge === bridge.iface) {
                connected.push({
                  vmid: guest.vmid,
                  name: guest.name,
                  type: (guest.type === 'qemu' ? 'vm' : 'ct') as 'vm' | 'ct',
                  nic: nic.name,
                  vlan: nic.tag,
                });
              }
            }
          }

          connected.sort((a, b) => a.vmid - b.vmid);

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

        bridgeList.sort((a, b) => a.name.localeCompare(b.name));
        const physNics = ifaces.filter(i => i.type === 'eth');

        // Cache the data
        topologyCache.set(cacheKey, { bridges: bridgeList, physicalNics: physNics });

        // Trigger re-render to show cached data
        forceUpdate(n => n + 1);
      } catch (err) {
        console.error('Failed to fetch network topology:', err);
      }
    }
    fetchData();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cacheKey]);

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
});

// Individual virtual switch diagram - memoized to prevent re-renders
const VirtualSwitchDiagram = memo(function VirtualSwitchDiagram({
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

  // Sort guests by vmid for stable display order
  const sortedGuests = [...bridge.connectedGuests].sort((a, b) => a.vmid - b.vmid);

  return (
    <div className="flex flex-col items-center min-w-[280px]">
      {/* Connected VMs/CTs (top) - use grid with order for stable positioning */}
      <div className="grid grid-cols-[repeat(auto-fit,minmax(64px,max-content))] justify-center gap-2 mb-2 min-h-[60px] w-full">
        {sortedGuests.map((guest) => (
          <div
            key={`${guest.vmid}-${guest.nic}`}
            className="flex flex-col items-center"
            style={{ order: guest.vmid }}
          >
            <div className="w-16 h-12 bg-purple-600 dark:bg-purple-700 rounded-t-lg flex flex-col items-center justify-center text-white text-xs shadow-md">
              <span>{guest.type === 'vm' ? '💻' : '📦'}</span>
              <span className="font-mono">{guest.vmid}</span>
            </div>
            {/* Connector line */}
            <div className="w-0.5 h-3 bg-purple-400" />
          </div>
        ))}
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
});
