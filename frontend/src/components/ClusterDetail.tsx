import { useState, useEffect, useMemo } from 'react';
import { useCluster } from '../context/ClusterContext';
import { formatBytes } from '../api/client';
import { DRSPanel } from './DRSPanel';
import { MetricsChart } from './MetricsChart';
import { useMetrics } from '../hooks/useMetrics';
import type { DRSRecommendation, Node, Guest } from '../types';

type TimeRange = '1h' | '6h' | '24h' | '7d' | '30d';

interface Tab {
  id: string;
  label: string;
}

const clusterTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'ha', label: 'HA' },
  { id: 'drs', label: 'DRS' },
  { id: 'monitor', label: 'Monitor' },
  { id: 'configure', label: 'Configure' },
];

interface HAStatus {
  enabled: boolean;
  quorum: boolean;
  manager: { node: string; status: string };
  resources: Array<{
    sid: string;
    type: string;
    status: string;
    node: string;
    state: string;
  }>;
}

interface HAGroup {
  group: string;
  comment?: string;
  nodes: string[];
  nofailback?: boolean;
  restricted?: boolean;
}

export function ClusterDetail({ clusterName, defaultTab }: { clusterName: string; defaultTab?: string }) {
  const { nodes, guests, drsRecommendations, getCluster } = useCluster();
  const [activeTab, setActiveTab] = useState(defaultTab || 'summary');

  useEffect(() => {
    if (defaultTab) setActiveTab(defaultTab);
  }, [defaultTab]);

  const cluster = getCluster(clusterName);
  const clusterNodes = useMemo(() => nodes.filter(n => n.cluster === clusterName), [nodes, clusterName]);
  const clusterGuests = useMemo(() => guests.filter(g => g.cluster === clusterName), [guests, clusterName]);
  const clusterDRS = useMemo(
    () => (drsRecommendations || []).filter(r => r.cluster === clusterName),
    [drsRecommendations, clusterName]
  );

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Header */}
      <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 py-3 flex-shrink-0">
        <div className="flex items-center gap-3">
          <span className="text-2xl">🏢</span>
          <div>
            <h1 className="text-lg font-semibold text-gray-900 dark:text-white">{clusterName}</h1>
            <div className="text-sm text-gray-500">
              Proxmox Cluster &middot; {clusterNodes.length} nodes &middot;{' '}
              {clusterGuests.filter(g => g.type === 'qemu').length} VMs &middot;{' '}
              {clusterGuests.filter(g => g.type === 'lxc').length} CTs
            </div>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 flex-shrink-0">
        <div className="flex gap-1">
          {clusterTabs.map((tab) => (
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
              {tab.id === 'drs' && clusterDRS.length > 0 && (
                <span className="ml-1.5 px-1.5 py-0.5 bg-yellow-500 text-white text-xs rounded-full">
                  {clusterDRS.length}
                </span>
              )}
            </button>
          ))}
        </div>
      </div>

      {/* Tab Content */}
      <div className="flex-1 overflow-auto p-4 bg-gray-50 dark:bg-gray-900">
        {activeTab === 'summary' && (
          <ClusterSummaryTab
            nodes={clusterNodes}
            guests={clusterGuests}
            ha={cluster?.ha}
          />
        )}
        {activeTab === 'ha' && <ClusterHATab clusterName={clusterName} />}
        {activeTab === 'drs' && (
          <ClusterDRSTab recommendations={clusterDRS} />
        )}
        {activeTab === 'monitor' && <ClusterMonitorTab clusterName={clusterName} />}
        {activeTab === 'configure' && <ClusterConfigureTab clusterName={clusterName} />}
      </div>
    </div>
  );
}

// ─── Summary Tab ────────────────────────────────────────────────────────────

function ClusterSummaryTab({
  nodes,
  guests,
  ha,
}: {
  nodes: Node[];
  guests: Guest[];
  ha?: { enabled: boolean; quorum: boolean; manager: string };
}) {
  const onlineNodes = nodes.filter(n => n.status === 'online');
  const runningVMs = guests.filter(g => g.type === 'qemu' && g.status === 'running');
  const runningCTs = guests.filter(g => g.type === 'lxc' && g.status === 'running');
  const totalVMs = guests.filter(g => g.type === 'qemu');
  const totalCTs = guests.filter(g => g.type === 'lxc');

  // Aggregate resources from nodes
  const totalCPU = nodes.reduce((sum, n) => sum + n.maxcpu, 0);
  const usedCPU = nodes.reduce((sum, n) => sum + n.cpu * n.maxcpu, 0);
  const totalMem = nodes.reduce((sum, n) => sum + n.maxmem, 0);
  const usedMem = nodes.reduce((sum, n) => sum + n.mem, 0);
  const totalDisk = nodes.reduce((sum, n) => sum + n.maxdisk, 0);
  const usedDisk = nodes.reduce((sum, n) => sum + n.disk, 0);

  const cpuPercent = totalCPU > 0 ? ((usedCPU / totalCPU) * 100).toFixed(1) : '0';
  const memPercent = totalMem > 0 ? ((usedMem / totalMem) * 100).toFixed(1) : '0';
  const diskPercent = totalDisk > 0 ? ((usedDisk / totalDisk) * 100).toFixed(1) : '0';

  // Version info
  const versions = [...new Set(nodes.map(n => n.pve_version).filter(Boolean))];
  const kernels = [...new Set(nodes.map(n => n.kernel_version).filter(Boolean))];

  return (
    <div className="space-y-4">
      {/* Status Cards Row */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <StatusCard
          title="Nodes"
          value={`${onlineNodes.length}/${nodes.length}`}
          subtitle="online"
          color={onlineNodes.length === nodes.length ? 'green' : 'yellow'}
        />
        <StatusCard
          title="VMs"
          value={`${runningVMs.length}/${totalVMs.length}`}
          subtitle="running"
          color="blue"
        />
        <StatusCard
          title="Containers"
          value={`${runningCTs.length}/${totalCTs.length}`}
          subtitle="running"
          color="blue"
        />
        <StatusCard
          title="HA"
          value={ha?.enabled ? (ha.quorum ? 'OK' : 'No Quorum') : 'Disabled'}
          subtitle={ha?.manager ? `mgr: ${ha.manager}` : ''}
          color={ha?.enabled ? (ha.quorum ? 'green' : 'red') : 'gray'}
        />
      </div>

      {/* Resource Usage */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Cluster Resources</h3>
        <div className="space-y-4">
          <ResourceBar label="CPU" value={parseFloat(cpuPercent)} detail={`${usedCPU.toFixed(1)} / ${totalCPU} cores`} />
          <ResourceBar label="Memory" value={parseFloat(memPercent)} detail={`${formatBytes(usedMem)} / ${formatBytes(totalMem)}`} />
          <ResourceBar label="Storage" value={parseFloat(diskPercent)} detail={`${formatBytes(usedDisk)} / ${formatBytes(totalDisk)}`} />
        </div>
      </div>

      {/* Nodes Grid */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Nodes</h3>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
                <th className="pb-2 font-medium">Node</th>
                <th className="pb-2 font-medium">Status</th>
                <th className="pb-2 font-medium">CPU</th>
                <th className="pb-2 font-medium">Memory</th>
                <th className="pb-2 font-medium">VMs</th>
                <th className="pb-2 font-medium">CTs</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map(node => {
                const nodeVMs = guests.filter(g => g.node === node.node && g.type === 'qemu');
                const nodeCTs = guests.filter(g => g.node === node.node && g.type === 'lxc');
                const nodeCPU = (node.cpu * 100).toFixed(1);
                const nodeMem = node.maxmem > 0 ? ((node.mem / node.maxmem) * 100).toFixed(1) : '0';
                return (
                  <tr key={node.node} className="border-b border-gray-100 dark:border-gray-700/50">
                    <td className="py-2 text-gray-900 dark:text-white font-medium">{node.node}</td>
                    <td className="py-2">
                      <span className={`px-2 py-0.5 rounded text-xs ${
                        node.status === 'online'
                          ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                          : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
                      }`}>
                        {node.status}
                      </span>
                    </td>
                    <td className="py-2 text-gray-700 dark:text-gray-300">{nodeCPU}%</td>
                    <td className="py-2 text-gray-700 dark:text-gray-300">{nodeMem}%</td>
                    <td className="py-2 text-gray-700 dark:text-gray-300">
                      {nodeVMs.filter(g => g.status === 'running').length}/{nodeVMs.length}
                    </td>
                    <td className="py-2 text-gray-700 dark:text-gray-300">
                      {nodeCTs.filter(g => g.status === 'running').length}/{nodeCTs.length}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* Version Info */}
      {(versions.length > 0 || kernels.length > 0) && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Software</h3>
          <div className="grid md:grid-cols-2 gap-4 text-sm">
            {versions.length > 0 && (
              <div>
                <span className="text-gray-500">PVE Version: </span>
                <span className={`text-gray-900 dark:text-white ${versions.length > 1 ? 'text-yellow-600 dark:text-yellow-400' : ''}`}>
                  {versions.length === 1 ? versions[0] : `${versions.join(', ')} (drift!)`}
                </span>
              </div>
            )}
            {kernels.length > 0 && (
              <div>
                <span className="text-gray-500">Kernel: </span>
                <span className={`text-gray-900 dark:text-white ${kernels.length > 1 ? 'text-yellow-600 dark:text-yellow-400' : ''}`}>
                  {kernels.length === 1 ? kernels[0] : `${kernels.join(', ')} (drift!)`}
                </span>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// ─── HA Tab ─────────────────────────────────────────────────────────────────

function ClusterHATab({ clusterName }: { clusterName: string }) {
  const [haStatus, setHaStatus] = useState<HAStatus | null>(null);
  const [haGroups, setHaGroups] = useState<HAGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(null);

    Promise.all([
      fetch(`/api/clusters/${clusterName}/ha/status`, { signal: controller.signal, credentials: 'include' })
        .then(r => r.ok ? r.json() : null)
        .catch(() => null),
      fetch(`/api/clusters/${clusterName}/ha/groups`, { signal: controller.signal, credentials: 'include' })
        .then(r => r.ok ? r.json() : [])
        .catch(() => []),
    ]).then(([status, groups]) => {
      setHaStatus(status);
      setHaGroups(groups || []);
    }).catch(e => {
      if (e.name !== 'AbortError') setError(e.message);
    }).finally(() => setLoading(false));

    return () => controller.abort();
  }, [clusterName]);

  if (loading) {
    return <div className="text-gray-500 p-4">Loading HA status...</div>;
  }

  if (error) {
    return <div className="text-red-500 p-4">Error: {error}</div>;
  }

  if (!haStatus) {
    return (
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6 text-center text-gray-500">
        HA is not available for this cluster.
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* HA Status Overview */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">HA Status</h3>
        <div className="grid md:grid-cols-3 gap-4 text-sm">
          <div>
            <span className="text-gray-500">Enabled: </span>
            <span className={haStatus.enabled ? 'text-green-600' : 'text-gray-400'}>
              {haStatus.enabled ? 'Yes' : 'No'}
            </span>
          </div>
          <div>
            <span className="text-gray-500">Quorum: </span>
            <span className={haStatus.quorum ? 'text-green-600' : 'text-red-600'}>
              {haStatus.quorum ? 'OK' : 'Lost'}
            </span>
          </div>
          <div>
            <span className="text-gray-500">Manager: </span>
            <span className="text-gray-900 dark:text-white">
              {haStatus.manager?.node || 'N/A'} ({haStatus.manager?.status || 'unknown'})
            </span>
          </div>
        </div>
      </div>

      {/* HA Resources */}
      {haStatus.resources && haStatus.resources.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">
            Managed Resources ({haStatus.resources.length})
          </h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
                  <th className="pb-2 font-medium">Resource</th>
                  <th className="pb-2 font-medium">Type</th>
                  <th className="pb-2 font-medium">Node</th>
                  <th className="pb-2 font-medium">Status</th>
                  <th className="pb-2 font-medium">State</th>
                </tr>
              </thead>
              <tbody>
                {haStatus.resources.map(res => (
                  <tr key={res.sid} className="border-b border-gray-100 dark:border-gray-700/50">
                    <td className="py-2 text-gray-900 dark:text-white font-medium">{res.sid}</td>
                    <td className="py-2 text-gray-700 dark:text-gray-300">{res.type}</td>
                    <td className="py-2 text-gray-700 dark:text-gray-300">{res.node}</td>
                    <td className="py-2">
                      <span className={`px-2 py-0.5 rounded text-xs ${
                        res.status === 'started'
                          ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                          : res.status === 'error' || res.status === 'fence'
                          ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
                          : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300'
                      }`}>
                        {res.status}
                      </span>
                    </td>
                    <td className="py-2 text-gray-700 dark:text-gray-300">{res.state}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* HA Groups */}
      {haGroups.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">
            Failover Groups ({haGroups.length})
          </h3>
          <div className="space-y-3">
            {haGroups.map(group => (
              <div key={group.group} className="border border-gray-200 dark:border-gray-700 rounded p-3">
                <div className="flex items-center justify-between mb-2">
                  <span className="font-medium text-gray-900 dark:text-white">{group.group}</span>
                  <div className="flex gap-2 text-xs">
                    {group.restricted && (
                      <span className="px-2 py-0.5 bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400 rounded">
                        Restricted
                      </span>
                    )}
                    {group.nofailback && (
                      <span className="px-2 py-0.5 bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400 rounded">
                        No Failback
                      </span>
                    )}
                  </div>
                </div>
                {group.comment && (
                  <div className="text-sm text-gray-500 mb-2">{group.comment}</div>
                )}
                <div className="text-sm text-gray-700 dark:text-gray-300">
                  <span className="text-gray-500">Nodes: </span>
                  {group.nodes.join(' → ')}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ─── DRS Tab ────────────────────────────────────────────────────────────────

function ClusterDRSTab({
  recommendations,
}: {
  recommendations: DRSRecommendation[];
}) {
  return (
    <div className="space-y-4">
      <DRSPanel
        recommendations={recommendations}
        onRefresh={() => window.location.reload()}
      />
    </div>
  );
}

// ─── Monitor Tab ────────────────────────────────────────────────────────────

const CLUSTER_METRICS = ['cpu', 'mem_percent'];

function ClusterMonitorTab({ clusterName }: { clusterName: string }) {
  const [timeRange, setTimeRange] = useState<TimeRange>('1h');

  const timeRanges: { value: TimeRange; label: string }[] = [
    { value: '1h', label: '1 Hour' },
    { value: '6h', label: '6 Hours' },
    { value: '24h', label: '24 Hours' },
    { value: '7d', label: '7 Days' },
    { value: '30d', label: '30 Days' },
  ];

  const { data: metricsData, loading } = useMetrics({
    cluster: clusterName,
    metrics: CLUSTER_METRICS,
    timeRange,
  });

  const series = metricsData?.series || [];

  const cpuSeries = useMemo(
    () => series.filter(s => s.metric === 'cpu'),
    [series]
  );

  const memSeries = useMemo(
    () => series.filter(s => s.metric === 'mem_percent'),
    [series]
  );

  return (
    <div className="space-y-4">
      {/* Time range selector */}
      <div className="flex gap-2">
        {timeRanges.map(tr => (
          <button
            key={tr.value}
            onClick={() => setTimeRange(tr.value)}
            className={`px-3 py-1 text-sm rounded ${
              timeRange === tr.value
                ? 'bg-blue-600 text-white'
                : 'bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-400 border border-gray-300 dark:border-gray-600'
            }`}
          >
            {tr.label}
          </button>
        ))}
      </div>

      {loading && <div className="text-gray-500">Loading metrics...</div>}

      <div className="grid md:grid-cols-2 gap-4">
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">CPU Usage</h3>
          <MetricsChart series={cpuSeries} timeRange={timeRange} height={250} />
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Memory Usage</h3>
          <MetricsChart series={memSeries} timeRange={timeRange} height={250} />
        </div>
      </div>
    </div>
  );
}

// ─── Configure Tab ──────────────────────────────────────────────────────────

function ClusterConfigureTab({ clusterName }: { clusterName: string }) {
  const { getCluster } = useCluster();
  const cluster = getCluster(clusterName);

  return (
    <div className="space-y-6">
      {/* DRS Configuration */}
      <ConfigSection title="DRS (Distributed Resource Scheduler)">
        <div className="text-sm text-gray-500 mb-3">
          DRS settings are currently configured in <code className="bg-gray-100 dark:bg-gray-700 px-1 rounded">config.yaml</code>.
          In-app editing coming soon.
        </div>
        <div className="grid md:grid-cols-2 gap-4 text-sm">
          <ConfigRow label="Status" value="See config.yaml" />
          <ConfigRow label="Mode" value="See config.yaml" />
          <ConfigRow label="Check Interval" value="See config.yaml" />
          <ConfigRow label="CPU Threshold" value="See config.yaml" />
          <ConfigRow label="Memory Threshold" value="See config.yaml" />
          <ConfigRow label="Max Migrations" value="See config.yaml" />
        </div>
      </ConfigSection>

      {/* Cluster Connection Info */}
      <ConfigSection title="Connection">
        <div className="grid md:grid-cols-2 gap-4 text-sm">
          <ConfigRow label="Cluster Name" value={clusterName} />
          <ConfigRow label="Nodes" value={`${cluster?.summary?.TotalNodes || 0} total`} />
        </div>
      </ConfigSection>
    </div>
  );
}

// ─── Shared Components ──────────────────────────────────────────────────────

function StatusCard({
  title,
  value,
  subtitle,
  color,
}: {
  title: string;
  value: string;
  subtitle: string;
  color: string;
}) {
  const colors: Record<string, string> = {
    green: 'border-green-500',
    yellow: 'border-yellow-500',
    red: 'border-red-500',
    blue: 'border-blue-500',
    gray: 'border-gray-400',
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

function ConfigSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
      <h3 className="font-medium mb-4 text-gray-900 dark:text-white pb-2 border-b border-gray-200 dark:border-gray-700">
        {title}
      </h3>
      {children}
    </div>
  );
}

function ConfigRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between py-1.5">
      <span className="text-gray-500">{label}</span>
      <span className="text-gray-900 dark:text-white">{value}</span>
    </div>
  );
}
