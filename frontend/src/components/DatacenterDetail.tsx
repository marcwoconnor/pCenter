import { useState, useEffect, useMemo } from 'react';
import { useCluster } from '../context/ClusterContext';
import { api, formatBytes } from '../api/client';
import type { Datacenter, Node, Guest } from '../types';

interface Tab {
  id: string;
  label: string;
}

const datacenterTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'configure', label: 'Configure' },
];

interface Props {
  datacenterId: string;
  datacenterName: string;
  defaultTab?: string;
}

export function DatacenterDetail({ datacenterId, datacenterName, defaultTab }: Props) {
  const { nodes, guests } = useCluster();
  const [activeTab, setActiveTab] = useState(defaultTab || 'summary');
  const [datacenter, setDatacenter] = useState<Datacenter | null>(null);

  useEffect(() => {
    if (defaultTab) setActiveTab(defaultTab);
  }, [defaultTab]);

  // Load datacenter details from inventory API
  useEffect(() => {
    if (datacenterId === 'root') return;
    const controller = new AbortController();
    api.getDatacenter(datacenterId)
      .then(setDatacenter)
      .catch(() => {});
    return () => controller.abort();
  }, [datacenterId]);

  // Find which clusters belong to this datacenter
  const dcClusterNames = useMemo(() => {
    if (!datacenter?.clusters) return [];
    return datacenter.clusters.map(c => c.agent_name || c.name);
  }, [datacenter]);

  // Filter nodes/guests to this datacenter's clusters
  const dcNodes = useMemo(
    () => dcClusterNames.length > 0
      ? nodes.filter(n => dcClusterNames.includes(n.cluster))
      : [],
    [nodes, dcClusterNames]
  );
  const dcGuests = useMemo(
    () => dcClusterNames.length > 0
      ? guests.filter(g => dcClusterNames.includes(g.cluster))
      : [],
    [guests, dcClusterNames]
  );

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Header */}
      <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 py-3 flex-shrink-0">
        <div className="flex items-center gap-3">
          <span className="text-2xl">🏛</span>
          <div>
            <h1 className="text-lg font-semibold text-gray-900 dark:text-white">{datacenterName}</h1>
            <div className="text-sm text-gray-500">
              Datacenter &middot; {datacenter?.clusters?.length || 0} clusters &middot;{' '}
              {datacenter?.hosts?.length || 0} standalone hosts
            </div>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 flex-shrink-0">
        <div className="flex gap-1">
          {datacenterTabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 text-sm font-medium transition-colors ${
                activeTab === tab.id
                  ? 'text-blue-600 border-b-2 border-blue-600 bg-white dark:bg-gray-700'
                  : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Tab Content */}
      <div className="flex-1 overflow-auto p-4 bg-gray-50 dark:bg-gray-900">
        {activeTab === 'summary' && (
          <DatacenterSummaryTab
            datacenter={datacenter}
            nodes={dcNodes}
            guests={dcGuests}
          />
        )}
        {activeTab === 'configure' && (
          <DatacenterConfigureTab datacenter={datacenter} />
        )}
      </div>
    </div>
  );
}

// ─── Summary Tab ────────────────────────────────────────────────────────────

function DatacenterSummaryTab({
  datacenter,
  nodes,
  guests,
}: {
  datacenter: Datacenter | null;
  nodes: Node[];
  guests: Guest[];
}) {
  const onlineNodes = nodes.filter(n => n.status === 'online');
  const runningVMs = guests.filter(g => g.type === 'qemu' && g.status === 'running');
  const runningCTs = guests.filter(g => g.type === 'lxc' && g.status === 'running');
  const totalVMs = guests.filter(g => g.type === 'qemu');
  const totalCTs = guests.filter(g => g.type === 'lxc');

  const totalCPU = nodes.reduce((sum, n) => sum + n.maxcpu, 0);
  const usedCPU = nodes.reduce((sum, n) => sum + n.cpu * n.maxcpu, 0);
  const totalMem = nodes.reduce((sum, n) => sum + n.maxmem, 0);
  const usedMem = nodes.reduce((sum, n) => sum + n.mem, 0);

  const cpuPercent = totalCPU > 0 ? ((usedCPU / totalCPU) * 100).toFixed(1) : '0';
  const memPercent = totalMem > 0 ? ((usedMem / totalMem) * 100).toFixed(1) : '0';

  return (
    <div className="space-y-4">
      {/* Status Cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <StatusCard title="Clusters" value={String(datacenter?.clusters?.length || 0)} subtitle="configured" color="blue" />
        <StatusCard title="Nodes" value={`${onlineNodes.length}/${nodes.length}`} subtitle="online" color={onlineNodes.length === nodes.length ? 'green' : 'yellow'} />
        <StatusCard title="VMs" value={`${runningVMs.length}/${totalVMs.length}`} subtitle="running" color="blue" />
        <StatusCard title="Containers" value={`${runningCTs.length}/${totalCTs.length}`} subtitle="running" color="blue" />
      </div>

      {/* Resources */}
      {nodes.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Aggregate Resources</h3>
          <div className="space-y-4">
            <ResourceBar label="CPU" value={parseFloat(cpuPercent)} detail={`${usedCPU.toFixed(1)} / ${totalCPU} cores`} />
            <ResourceBar label="Memory" value={parseFloat(memPercent)} detail={`${formatBytes(usedMem)} / ${formatBytes(totalMem)}`} />
          </div>
        </div>
      )}

      {/* Clusters List */}
      {datacenter?.clusters && datacenter.clusters.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Clusters</h3>
          <div className="space-y-2">
            {datacenter.clusters.map(cluster => (
              <div key={cluster.id} className="flex items-center justify-between p-2 border border-gray-200 dark:border-gray-700 rounded">
                <div>
                  <span className="font-medium text-gray-900 dark:text-white">{cluster.name}</span>
                  {cluster.agent_name && cluster.agent_name !== cluster.name && (
                    <span className="ml-2 text-xs text-gray-500">({cluster.agent_name})</span>
                  )}
                </div>
                <span className={`px-2 py-0.5 rounded text-xs ${
                  cluster.status === 'active'
                    ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
                }`}>
                  {cluster.status}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Standalone Hosts */}
      {datacenter?.hosts && datacenter.hosts.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Standalone Hosts</h3>
          <div className="space-y-2">
            {datacenter.hosts.map(host => (
              <div key={host.id} className="flex items-center justify-between p-2 border border-gray-200 dark:border-gray-700 rounded">
                <div>
                  <span className="font-medium text-gray-900 dark:text-white">{host.node_name || host.address}</span>
                  <span className="ml-2 text-xs text-gray-500">{host.address}</span>
                </div>
                <span className={`px-2 py-0.5 rounded text-xs ${
                  host.status === 'online'
                    ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
                }`}>
                  {host.status}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Configure Tab ──────────────────────────────────────────────────────────

function DatacenterConfigureTab({ datacenter }: { datacenter: Datacenter | null }) {
  if (!datacenter) {
    return <div className="text-gray-500">Loading datacenter configuration...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-4 text-gray-900 dark:text-white pb-2 border-b border-gray-200 dark:border-gray-700">
          General
        </h3>
        <div className="space-y-3 text-sm">
          <div className="flex justify-between py-1.5">
            <span className="text-gray-500">Name</span>
            <span className="text-gray-900 dark:text-white">{datacenter.name}</span>
          </div>
          <div className="flex justify-between py-1.5">
            <span className="text-gray-500">Description</span>
            <span className="text-gray-900 dark:text-white">{datacenter.description || '(none)'}</span>
          </div>
          <div className="flex justify-between py-1.5">
            <span className="text-gray-500">Created</span>
            <span className="text-gray-900 dark:text-white">{new Date(datacenter.created_at).toLocaleString()}</span>
          </div>
          <div className="flex justify-between py-1.5">
            <span className="text-gray-500">Clusters</span>
            <span className="text-gray-900 dark:text-white">{datacenter.clusters?.length || 0}</span>
          </div>
          <div className="flex justify-between py-1.5">
            <span className="text-gray-500">Standalone Hosts</span>
            <span className="text-gray-900 dark:text-white">{datacenter.hosts?.length || 0}</span>
          </div>
        </div>
      </div>
    </div>
  );
}

// ─── Shared Components ──────────────────────────────────────────────────────

function StatusCard({ title, value, subtitle, color }: { title: string; value: string; subtitle: string; color: string }) {
  const colors: Record<string, string> = {
    green: 'border-green-500', yellow: 'border-yellow-500', red: 'border-red-500', blue: 'border-blue-500', gray: 'border-gray-400',
  };
  return (
    <div className={`bg-white dark:bg-gray-800 rounded-lg shadow p-4 border-l-4 ${colors[color] || colors.gray}`}>
      <div className="text-sm text-gray-500">{title}</div>
      <div className="text-2xl font-bold text-gray-900 dark:text-white">{value}</div>
      <div className="text-xs text-gray-400">{subtitle}</div>
    </div>
  );
}

function ResourceBar({ label, value, detail }: { label: string; value: number; detail: string }) {
  const barColor = value > 90 ? 'bg-red-500' : value > 70 ? 'bg-yellow-500' : 'bg-blue-500';
  return (
    <div>
      <div className="flex justify-between text-sm mb-1">
        <span className="text-gray-500">{label}</span>
        <span className="text-gray-900 dark:text-white">{value.toFixed(1)}% ({detail})</span>
      </div>
      <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
        <div className={`h-full rounded ${barColor}`} style={{ width: `${Math.min(value, 100)}%` }} />
      </div>
    </div>
  );
}
