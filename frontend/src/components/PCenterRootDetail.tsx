import { useState, useEffect, useMemo } from 'react';
import { useCluster } from '../context/ClusterContext';
import { formatBytes } from '../api/client';
import { DRSPanel } from './DRSPanel';
import { MetricsChart } from './MetricsChart';
import { useMetrics } from '../hooks/useMetrics';

type TimeRange = '1h' | '6h' | '24h' | '7d' | '30d';

interface Tab {
  id: string;
  label: string;
}

const rootTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'monitor', label: 'Monitor' },
  { id: 'configure', label: 'Configure' },
];

export function PCenterRootDetail({ defaultTab }: { defaultTab?: string }) {
  const { nodes, guests, clusters } = useCluster();
  const [activeTab, setActiveTab] = useState(defaultTab || 'summary');

  useEffect(() => {
    if (defaultTab) setActiveTab(defaultTab);
  }, [defaultTab]);

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Header */}
      <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 py-3 flex-shrink-0">
        <div className="flex items-center gap-3">
          <span className="text-2xl">🖧</span>
          <div>
            <h1 className="text-lg font-semibold text-gray-900 dark:text-white">pCenter</h1>
            <div className="text-sm text-gray-500">
              {clusters.length} cluster{clusters.length !== 1 ? 's' : ''} &middot;{' '}
              {nodes.length} nodes &middot;{' '}
              {guests.length} guests
            </div>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 flex-shrink-0">
        <div className="flex gap-1">
          {rootTabs.map((tab) => (
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
        {activeTab === 'summary' && <RootSummaryTab />}
        {activeTab === 'monitor' && <RootMonitorTab />}
        {activeTab === 'configure' && <RootConfigureTab />}
      </div>
    </div>
  );
}

// ─── Summary Tab ────────────────────────────────────────────────────────────

function RootSummaryTab() {
  const { nodes, guests, clusters, drsRecommendations, ceph } = useCluster();

  const onlineNodes = nodes.filter(n => n.status === 'online');
  const runningVMs = guests.filter(g => g.type === 'qemu' && g.status === 'running');
  const totalVMs = guests.filter(g => g.type === 'qemu');
  const runningCTs = guests.filter(g => g.type === 'lxc' && g.status === 'running');
  const totalCTs = guests.filter(g => g.type === 'lxc');

  const totalCPU = nodes.reduce((sum, n) => sum + n.maxcpu, 0);
  const usedCPU = nodes.reduce((sum, n) => sum + n.cpu * n.maxcpu, 0);
  const totalMem = nodes.reduce((sum, n) => sum + n.maxmem, 0);
  const usedMem = nodes.reduce((sum, n) => sum + n.mem, 0);
  const totalDisk = nodes.reduce((sum, n) => sum + n.maxdisk, 0);
  const usedDisk = nodes.reduce((sum, n) => sum + n.disk, 0);

  const cpuPercent = totalCPU > 0 ? (usedCPU / totalCPU) * 100 : 0;
  const memPercent = totalMem > 0 ? (usedMem / totalMem) * 100 : 0;
  const diskPercent = totalDisk > 0 ? (usedDisk / totalDisk) * 100 : 0;

  // Context ceph is flat: { health: string, bytes_used, bytes_avail, bytes_total }
  const cephHealth = ceph?.health || 'N/A';

  return (
    <div className="space-y-4">
      {/* Status Cards */}
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <StatusCard title="Clusters" value={String(clusters.length)} subtitle="configured" color="blue" />
        <StatusCard title="Nodes" value={`${onlineNodes.length}/${nodes.length}`} subtitle="online" color={onlineNodes.length === nodes.length ? 'green' : 'yellow'} />
        <StatusCard title="VMs" value={`${runningVMs.length}/${totalVMs.length}`} subtitle="running" color="blue" />
        <StatusCard title="Containers" value={`${runningCTs.length}/${totalCTs.length}`} subtitle="running" color="blue" />
        <StatusCard
          title="Ceph"
          value={cephHealth === 'HEALTH_OK' ? 'Healthy' : cephHealth === 'HEALTH_WARN' ? 'Warning' : cephHealth === 'HEALTH_ERR' ? 'Error' : cephHealth}
          subtitle={ceph ? `${formatBytes(ceph.bytes_used || 0)} used` : ''}
          color={cephHealth === 'HEALTH_OK' ? 'green' : cephHealth === 'HEALTH_WARN' ? 'yellow' : cephHealth === 'HEALTH_ERR' ? 'red' : 'gray'}
        />
      </div>

      {/* Resources */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Global Resources</h3>
        <div className="space-y-4">
          <ResourceBar label="CPU" value={cpuPercent} detail={`${usedCPU.toFixed(1)} / ${totalCPU} cores`} />
          <ResourceBar label="Memory" value={memPercent} detail={`${formatBytes(usedMem)} / ${formatBytes(totalMem)}`} />
          <ResourceBar label="Storage" value={diskPercent} detail={`${formatBytes(usedDisk)} / ${formatBytes(totalDisk)}`} />
        </div>
      </div>

      {/* Per-Cluster Overview */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Clusters</h3>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
                <th className="pb-2 font-medium">Cluster</th>
                <th className="pb-2 font-medium">Nodes</th>
                <th className="pb-2 font-medium">VMs</th>
                <th className="pb-2 font-medium">CTs</th>
                <th className="pb-2 font-medium">HA</th>
              </tr>
            </thead>
            <tbody>
              {clusters.map(c => (
                <tr key={c.name} className="border-b border-gray-100 dark:border-gray-700/50">
                  <td className="py-2 font-medium text-gray-900 dark:text-white">{c.name}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">
                    {c.summary?.OnlineNodes || 0}/{c.summary?.TotalNodes || 0}
                  </td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">
                    {c.summary?.RunningVMs || 0}/{c.summary?.TotalVMs || 0}
                  </td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">
                    {c.summary?.RunningCTs || 0}/{c.summary?.TotalContainers || 0}
                  </td>
                  <td className="py-2">
                    {c.ha ? (
                      <span className={`px-2 py-0.5 rounded text-xs ${
                        c.ha.quorum
                          ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                          : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
                      }`}>
                        {c.ha.quorum ? 'Quorum' : 'No Quorum'}
                      </span>
                    ) : (
                      <span className="text-gray-400">N/A</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* DRS Recommendations */}
      {drsRecommendations && drsRecommendations.length > 0 && (
        <DRSPanel recommendations={drsRecommendations} onRefresh={() => window.location.reload()} />
      )}
    </div>
  );
}

// ─── Monitor Tab ────────────────────────────────────────────────────────────

const GLOBAL_METRICS = ['cpu', 'mem_percent'];

function RootMonitorTab() {
  const [timeRange, setTimeRange] = useState<TimeRange>('1h');

  const timeRanges: { value: TimeRange; label: string }[] = [
    { value: '1h', label: '1 Hour' },
    { value: '6h', label: '6 Hours' },
    { value: '24h', label: '24 Hours' },
    { value: '7d', label: '7 Days' },
    { value: '30d', label: '30 Days' },
  ];

  const { data: metricsData, loading } = useMetrics({
    metrics: GLOBAL_METRICS,
    timeRange,
  });

  const series = metricsData?.series || [];

  const cpuSeries = useMemo(() => series.filter(s => s.metric === 'cpu'), [series]);
  const memSeries = useMemo(() => series.filter(s => s.metric === 'mem_percent'), [series]);

  return (
    <div className="space-y-4">
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
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">CPU Usage (All Nodes)</h3>
          <MetricsChart series={cpuSeries} timeRange={timeRange} height={250} />
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Memory Usage (All Nodes)</h3>
          <MetricsChart series={memSeries} timeRange={timeRange} height={250} />
        </div>
      </div>
    </div>
  );
}

// ─── Configure Tab ──────────────────────────────────────────────────────────

function RootConfigureTab() {
  return (
    <div className="space-y-6">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-4 text-gray-900 dark:text-white pb-2 border-b border-gray-200 dark:border-gray-700">
          Server
        </h3>
        <div className="text-sm text-gray-500 mb-3">
          Global pCenter server settings are configured in <code className="bg-gray-100 dark:bg-gray-700 px-1 rounded">config.yaml</code>.
          In-app editing coming soon.
        </div>
        <div className="grid md:grid-cols-2 gap-4 text-sm">
          <ConfigRow label="Configuration" value="config.yaml" />
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-4 text-gray-900 dark:text-white pb-2 border-b border-gray-200 dark:border-gray-700">
          Metrics
        </h3>
        <div className="text-sm text-gray-500">
          Metrics collection, retention, and rollup settings are in <code className="bg-gray-100 dark:bg-gray-700 px-1 rounded">config.yaml</code> under the <code className="bg-gray-100 dark:bg-gray-700 px-1 rounded">metrics:</code> section.
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-4 text-gray-900 dark:text-white pb-2 border-b border-gray-200 dark:border-gray-700">
          Authentication
        </h3>
        <div className="text-sm text-gray-500">
          Auth, session, lockout, TOTP, and rate-limiting settings are in <code className="bg-gray-100 dark:bg-gray-700 px-1 rounded">config.yaml</code> under the <code className="bg-gray-100 dark:bg-gray-700 px-1 rounded">auth:</code> section.
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

function ConfigRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between py-1.5">
      <span className="text-gray-500">{label}</span>
      <span className="text-gray-900 dark:text-white">{value}</span>
    </div>
  );
}
