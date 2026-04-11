import { useState, useEffect, useMemo, memo } from 'react';
import { Link } from 'react-router-dom';
import { useCluster } from '../context/ClusterContext';
import { Layout } from '../components/Layout';
import { DRSPanel } from '../components/DRSPanel';
import { MetricsChart } from '../components/MetricsChart';
import { useMetrics } from '../hooks/useMetrics';
import { formatBytes } from '../api/client';
import type { QDeviceStatus, MaintenancePreflight, MaintenanceState } from '../types';

function ProgressBar({ value, color = 'blue' }: { value: number; color?: string }) {
  const colors: Record<string, string> = {
    blue: 'bg-blue-500',
    green: 'bg-green-500',
    red: 'bg-red-500',
    yellow: 'bg-yellow-500',
  };
  const barColor = value > 90 ? colors.red : value > 70 ? colors.yellow : colors[color] || colors.blue;
  return (
    <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
      <div className={`h-full rounded ${barColor}`} style={{ width: `${Math.min(value, 100)}%` }} />
    </div>
  );
}

// QDevice Status Banner
function QDeviceBanner({ cluster }: { cluster: string }) {
  const [qdevice, setQdevice] = useState<QDeviceStatus | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    fetch(`/api/clusters/${cluster}/qdevice`, { signal: controller.signal })
      .then(r => r.json())
      .then(setQdevice)
      .catch((e) => { if (e.name !== 'AbortError') setQdevice(null); });
    return () => controller.abort();
  }, [cluster]);

  if (!qdevice || !qdevice.configured) return null;

  return (
    <div className={`mb-4 p-3 rounded-lg flex items-center justify-between ${
      qdevice.connected
        ? 'bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800'
        : 'bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800'
    }`}>
      <div className="flex items-center gap-3">
        <div className={`w-3 h-3 rounded-full ${qdevice.connected ? 'bg-green-500' : 'bg-red-500'}`} />
        <div>
          <span className="font-medium text-gray-900 dark:text-white">QDevice: </span>
          <span className={qdevice.connected ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}>
            {qdevice.connected ? 'Connected' : 'Disconnected'}
          </span>
        </div>
      </div>
      {qdevice.host_vm_name && (
        <div className="text-sm text-gray-500">
          VM: <span className="font-mono">{qdevice.host_vm_name}</span> on <span className="font-medium">{qdevice.host_node}</span>
        </div>
      )}
    </div>
  );
}

// Maintenance Mode Modal
function MaintenanceModal({
  node,
  cluster,
  onClose,
}: {
  node: string;
  cluster: string;
  onClose: () => void;
}) {
  const [preflight, setPreflight] = useState<MaintenancePreflight | null>(null);
  const [state, setState] = useState<MaintenanceState | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const controller = new AbortController();
    // Fetch preflight checks
    fetch(`/api/clusters/${cluster}/maintenance/${node}/preflight`, { signal: controller.signal })
      .then(r => r.json())
      .then(setPreflight)
      .catch((e) => { if (e.name !== 'AbortError') console.error('preflight fetch failed:', e); })
      .finally(() => setLoading(false));

    // Fetch current state
    fetch(`/api/clusters/${cluster}/maintenance/${node}/state`, { signal: controller.signal })
      .then(r => r.json())
      .then(setState)
      .catch((e) => { if (e.name !== 'AbortError') console.error('state fetch failed:', e); });

    return () => controller.abort();
  }, [cluster, node]);

  // Poll for state updates when in maintenance
  useEffect(() => {
    if (!state?.in_maintenance) return;
    const controller = new AbortController();
    const interval = setInterval(() => {
      fetch(`/api/clusters/${cluster}/maintenance/${node}/state`, { signal: controller.signal })
        .then(r => r.json())
        .then(setState)
        .catch((e) => { if (e.name !== 'AbortError') console.error('maintenance poll failed:', e); });
    }, 2000);
    return () => { clearInterval(interval); controller.abort(); };
  }, [cluster, node, state?.in_maintenance]);

  const enterMaintenance = async () => {
    const res = await fetch(`/api/clusters/${cluster}/maintenance/${node}/enter`, { method: 'POST' });
    const newState = await res.json();
    setState(newState);
  };

  const exitMaintenance = async () => {
    await fetch(`/api/clusters/${cluster}/maintenance/${node}/exit`, { method: 'POST' });
    setState(null);
    onClose();
  };

  const getCheckIcon = (status: string) => {
    switch (status) {
      case 'ok': return '✓';
      case 'warning': return '⚠';
      case 'error': return '✗';
      default: return '?';
    }
  };

  const getCheckColor = (status: string) => {
    switch (status) {
      case 'ok': return 'text-green-500';
      case 'warning': return 'text-yellow-500';
      case 'error': return 'text-red-500';
      default: return 'text-gray-500';
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-2xl w-full mx-4 max-h-[80vh] overflow-auto" onClick={e => e.stopPropagation()}>
        <div className="p-4 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
          <h2 className="text-xl font-bold text-gray-900 dark:text-white">
            Maintenance Mode: {node}
          </h2>
          <button onClick={onClose} className="text-gray-500 hover:text-gray-700">✕</button>
        </div>

        <div className="p-4">
          {loading ? (
            <div className="text-center py-8 text-gray-500">Loading pre-flight checks...</div>
          ) : state?.in_maintenance ? (
            // In maintenance mode - show progress
            <div className="space-y-4">
              <div className={`p-4 rounded-lg ${
                state.phase === 'ready' ? 'bg-green-50 dark:bg-green-900/20' :
                state.phase === 'error' ? 'bg-red-50 dark:bg-red-900/20' :
                'bg-blue-50 dark:bg-blue-900/20'
              }`}>
                <div className="flex items-center justify-between mb-2">
                  <span className="font-medium text-gray-900 dark:text-white">
                    {state.phase === 'ready' ? 'Ready for Maintenance' :
                     state.phase === 'error' ? 'Error' :
                     'Evacuating Guests...'}
                  </span>
                  <span className="text-sm text-gray-500">{state.progress}%</span>
                </div>
                <ProgressBar value={state.progress} color={state.phase === 'ready' ? 'green' : state.phase === 'error' ? 'red' : 'blue'} />
                {state.message && (
                  <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">{state.message}</p>
                )}
              </div>

              {state.phase === 'ready' && (
                <div className="bg-yellow-50 dark:bg-yellow-900/20 p-4 rounded-lg">
                  <p className="text-yellow-800 dark:text-yellow-200 font-medium">
                    Host is ready for maintenance. You may now safely reboot or shut down the host.
                  </p>
                </div>
              )}

              <button
                onClick={exitMaintenance}
                className="w-full py-2 px-4 bg-gray-600 hover:bg-gray-700 text-white rounded-lg font-medium"
              >
                Exit Maintenance Mode
              </button>
            </div>
          ) : (
            // Pre-flight checks
            <div className="space-y-4">
              <div className="space-y-2">
                <h3 className="font-medium text-gray-900 dark:text-white">Pre-flight Checks</h3>
                {preflight?.checks.map((check, i) => (
                  <div key={i} className="flex items-start gap-2 p-2 bg-gray-50 dark:bg-gray-700 rounded">
                    <span className={`${getCheckColor(check.status)} font-bold`}>{getCheckIcon(check.status)}</span>
                    <div>
                      <div className="font-medium text-gray-900 dark:text-white">{check.name}</div>
                      <div className="text-sm text-gray-500">{check.message}</div>
                    </div>
                  </div>
                ))}
              </div>

              {preflight?.critical_guests && preflight.critical_guests.length > 0 && (
                <div className="bg-yellow-50 dark:bg-yellow-900/20 p-3 rounded-lg">
                  <h4 className="font-medium text-yellow-800 dark:text-yellow-200 mb-2">Critical Guests (migrate first)</h4>
                  {preflight.critical_guests.map(g => (
                    <div key={g.vmid} className="text-sm text-yellow-700 dark:text-yellow-300">
                      {g.name} ({g.vmid}) - {g.reason}
                    </div>
                  ))}
                </div>
              )}

              {preflight?.guests_to_move && preflight.guests_to_move.length > 0 && (
                <div>
                  <h4 className="font-medium text-gray-900 dark:text-white mb-2">
                    Guests to Migrate ({preflight.guests_to_move.length})
                  </h4>
                  <div className="max-h-40 overflow-auto text-sm text-gray-600 dark:text-gray-400">
                    {preflight.guests_to_move.map(g => (
                      <div key={g.vmid} className="py-1">
                        {g.name} ({g.vmid}) → {g.target_node}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <div className="flex gap-2">
                <button
                  onClick={onClose}
                  className="flex-1 py-2 px-4 bg-gray-200 hover:bg-gray-300 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-800 dark:text-white rounded-lg font-medium"
                >
                  Cancel
                </button>
                <button
                  onClick={enterMaintenance}
                  disabled={!preflight?.can_enter}
                  className={`flex-1 py-2 px-4 rounded-lg font-medium ${
                    preflight?.can_enter
                      ? 'bg-orange-500 hover:bg-orange-600 text-white'
                      : 'bg-gray-300 text-gray-500 cursor-not-allowed'
                  }`}
                >
                  Enter Maintenance Mode
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

type TimeRange = '1h' | '6h' | '24h' | '7d' | '30d';

// Type for metrics series
type MetricsSeries = { metric: string; resource_id: string; unit: string; data: { ts: number; value: number }[] };
type MetricsData = { series: MetricsSeries[] } | null;

// Aggregate network metrics by node (sum all VMs/CTs on each node)
function aggregateByNode(
  series: MetricsSeries[],
  metric: string,
  vmidToNode: Map<string, string>
): MetricsSeries[] {
  // Group series by node
  const nodeData = new Map<string, Map<number, number>>();

  for (const s of series) {
    if (s.metric !== metric) continue;
    const node = vmidToNode.get(s.resource_id) || 'unknown';

    if (!nodeData.has(node)) {
      nodeData.set(node, new Map());
    }
    const nodeMap = nodeData.get(node)!;

    for (const point of s.data) {
      const existing = nodeMap.get(point.ts) || 0;
      nodeMap.set(point.ts, existing + point.value);
    }
  }

  // Convert back to series format
  const result: MetricsSeries[] = [];
  for (const [node, dataMap] of nodeData) {
    const data = Array.from(dataMap.entries())
      .map(([ts, value]) => ({ ts, value }))
      .sort((a, b) => a.ts - b.ts);
    result.push({
      metric,
      resource_id: node,
      unit: 'bytes_per_sec',
      data,
    });
  }

  return result;
}

// Memoized metrics panel to prevent re-renders from WebSocket updates
const MetricsPanel = memo(function MetricsPanel({
  nodeMetrics,
  vmNetMetrics,
  ctNetMetrics,
  guests,
  loading,
  timeRange,
}: {
  nodeMetrics: MetricsData;
  vmNetMetrics: MetricsData;
  ctNetMetrics: MetricsData;
  guests: { vmid: number; node: string }[];
  loading: boolean;
  timeRange: TimeRange;
}) {
  // Build vmid -> node lookup
  const vmidToNode = useMemo(() => {
    const map = new Map<string, string>();
    for (const g of guests) {
      map.set(g.vmid.toString(), g.node);
    }
    return map;
  }, [guests]);

  // Node metrics
  const cpuSeries = useMemo(
    () => nodeMetrics?.series?.filter(s => s.metric === 'cpu') ?? [],
    [nodeMetrics?.series]
  );
  const pgpgInSeries = useMemo(
    () => nodeMetrics?.series?.filter(s => s.metric === 'pgpgin') ?? [],
    [nodeMetrics?.series]
  );
  const pgpgOutSeries = useMemo(
    () => nodeMetrics?.series?.filter(s => s.metric === 'pgpgout') ?? [],
    [nodeMetrics?.series]
  );

  // Guest I/O metrics - aggregate by node
  const allGuestSeries = useMemo(() => {
    const vmSeries = vmNetMetrics?.series ?? [];
    const ctSeries = ctNetMetrics?.series ?? [];
    return [...vmSeries, ...ctSeries];
  }, [vmNetMetrics?.series, ctNetMetrics?.series]);

  // Network I/O by node
  const netInByNode = useMemo(
    () => aggregateByNode(allGuestSeries, 'netin', vmidToNode),
    [allGuestSeries, vmidToNode]
  );
  const netOutByNode = useMemo(
    () => aggregateByNode(allGuestSeries, 'netout', vmidToNode),
    [allGuestSeries, vmidToNode]
  );

  // Disk I/O by node
  const diskReadByNode = useMemo(
    () => aggregateByNode(allGuestSeries, 'diskread', vmidToNode),
    [allGuestSeries, vmidToNode]
  );
  const diskWriteByNode = useMemo(
    () => aggregateByNode(allGuestSeries, 'diskwrite', vmidToNode),
    [allGuestSeries, vmidToNode]
  );

  if (loading) {
    return (
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <div className="text-sm text-gray-500 text-center py-8">Loading metrics...</div>
      </div>
    );
  }

  const hasNodeMetrics = nodeMetrics?.series && nodeMetrics.series.length > 0;
  const hasMemMetrics = pgpgInSeries.length > 0 || pgpgOutSeries.length > 0;
  const hasNetMetrics = netInByNode.length > 0 || netOutByNode.length > 0;
  const hasDiskMetrics = diskReadByNode.length > 0 || diskWriteByNode.length > 0;

  if (!hasNodeMetrics && !hasMemMetrics && !hasNetMetrics && !hasDiskMetrics) {
    return (
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <div className="text-sm text-gray-500 text-center py-8">
          No metrics data available. Enable metrics in config.yaml.
        </div>
      </div>
    );
  }

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <MetricsChart
          series={cpuSeries}
          timeRange={timeRange}
          title="CPU Usage (%)"
          height={160}
        />
        {hasMemMetrics && (
          <MetricsChart
            series={[...pgpgInSeries, ...pgpgOutSeries]}
            timeRange={timeRange}
            title="Memory I/O"
            height={160}
          />
        )}
        {hasDiskMetrics && (
          <MetricsChart
            series={[...diskReadByNode, ...diskWriteByNode]}
            timeRange={timeRange}
            title="Disk I/O"
            height={160}
          />
        )}
        {hasNetMetrics && (
          <MetricsChart
            series={[...netInByNode, ...netOutByNode]}
            timeRange={timeRange}
            title="Network I/O"
            height={160}
          />
        )}
      </div>
    </div>
  );
});

// Stable reference for metrics to fetch
const NODE_METRICS = ['cpu', 'pgpgin', 'pgpgout'] as const;
const GUEST_METRICS = ['netin', 'netout', 'diskread', 'diskwrite'] as const;

export function Home() {
  const { summary, nodes, guests, ceph, drsRecommendations, isLoading } = useCluster();
  const [maintenanceNode, setMaintenanceNode] = useState<{ node: string; cluster: string } | null>(null);
  const [timeRange, setTimeRange] = useState<TimeRange>('1h');

  // Memoize cluster name to prevent unnecessary refetches
  const clusterName = useMemo(() => nodes[0]?.cluster || '', [nodes[0]?.cluster]);

  // Fetch node metrics (cpu, mem, disk)
  const { data: nodeMetrics, loading: nodeMetricsLoading } = useMetrics({
    cluster: clusterName,
    resourceType: 'node',
    metrics: NODE_METRICS as unknown as string[],
    timeRange,
    enabled: !!clusterName,
  });

  // Fetch VM network metrics (aggregate across all VMs)
  const { data: vmNetMetrics, loading: vmNetLoading } = useMetrics({
    cluster: clusterName,
    resourceType: 'vm',
    metrics: GUEST_METRICS as unknown as string[],
    timeRange,
    enabled: !!clusterName,
  });

  // Fetch CT network metrics
  const { data: ctNetMetrics, loading: ctNetLoading } = useMetrics({
    cluster: clusterName,
    resourceType: 'ct',
    metrics: GUEST_METRICS as unknown as string[],
    timeRange,
    enabled: !!clusterName,
  });

  if (isLoading) {
    return (
      <Layout>
        <div className="flex-1 flex items-center justify-center">
          <div className="text-gray-500">Loading...</div>
        </div>
      </Layout>
    );
  }

  const safeGuests = guests || [];
  const safeNodes = nodes || [];
  const vms = safeGuests.filter(g => g.type === 'qemu');
  const cts = safeGuests.filter(g => g.type === 'lxc');
  const runningVMs = vms.filter(g => g.status === 'running').length;
  const runningCTs = cts.filter(g => g.status === 'running').length;
  const totalVMs = vms.length;
  const totalCTs = cts.length;

  // Get stopped guests
  const stoppedVMs = vms.filter(g => g.status !== 'running').slice(0, 3);
  const stoppedCTs = cts.filter(g => g.status !== 'running').slice(0, 3);

  // Get top CPU consumers (running only)
  const topCpuVM = vms.filter(g => g.status === 'running').sort((a, b) => b.cpu - a.cpu)[0];
  const topCpuCT = cts.filter(g => g.status === 'running').sort((a, b) => b.cpu - a.cpu)[0];

  // Get version stats - find most common version and count
  const pveVersionCounts = nodes.reduce((acc, n) => {
    const v = n.pve_version?.split('/')[1] || n.pve_version;
    if (v) acc[v] = (acc[v] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);
  const pveVersionEntries = Object.entries(pveVersionCounts).sort((a, b) => b[1] - a[1]);
  const topPveVersion = pveVersionEntries[0];
  const hasMixedPve = pveVersionEntries.length > 1;

  const kernelVersionCounts = nodes.reduce((acc, n) => {
    if (n.kernel_version) acc[n.kernel_version] = (acc[n.kernel_version] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);
  const kernelEntries = Object.entries(kernelVersionCounts).sort((a, b) => b[1] - a[1]);
  const topKernel = kernelEntries[0];
  const hasMixedKernel = kernelEntries.length > 1;

  return (
    <Layout>
      <div className="flex-1 overflow-auto p-6">
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">Datacenter Overview</h1>

        {/* Summary Cards */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
            <div className="text-sm text-gray-500 dark:text-gray-400">Nodes</div>
            <div className="text-2xl font-bold text-gray-900 dark:text-white">
              {summary?.OnlineNodes || 0}/{summary?.TotalNodes || nodes.length}
            </div>
            <div className="text-xs text-green-500">online</div>
            {topPveVersion && (
              <div className="mt-2 space-y-1 text-xs">
                <div className={hasMixedPve ? 'text-yellow-500' : 'text-gray-500'}>
                  {hasMixedPve
                    ? `${topPveVersion[1]}/${nodes.length} on PVE ${topPveVersion[0]}`
                    : `PVE ${topPveVersion[0]}`
                  }
                </div>
                {topKernel && (
                  <div className={`truncate ${hasMixedKernel ? 'text-yellow-500' : 'text-gray-500 dark:text-gray-400'}`}>
                    {hasMixedKernel
                      ? `${topKernel[1]}/${nodes.length} on ${topKernel[0]}`
                      : topKernel[0]
                    }
                  </div>
                )}
              </div>
            )}
          </div>
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
            <div className="text-sm text-gray-500 dark:text-gray-400">Virtual Machines</div>
            <div className="text-2xl font-bold text-gray-900 dark:text-white">
              {runningVMs}/{totalVMs}
            </div>
            <div className="text-xs text-green-500">running</div>
            <div className="mt-2 space-y-1 text-xs">
              {topCpuVM && (
                <div className={`${topCpuVM.cpu > 0.5 ? 'text-yellow-500' : 'text-gray-500'}`}>
                  {topCpuVM.name}: {(topCpuVM.cpu * 100).toFixed(0)}%
                </div>
              )}
              {stoppedVMs.length > 0 && (
                <div className="text-gray-500 dark:text-gray-400">
                  stopped: {stoppedVMs.map(v => v.name).join(', ')}
                </div>
              )}
            </div>
          </div>
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
            <div className="text-sm text-gray-500 dark:text-gray-400">Containers</div>
            <div className="text-2xl font-bold text-gray-900 dark:text-white">
              {runningCTs}/{totalCTs}
            </div>
            <div className="text-xs text-green-500">running</div>
            <div className="mt-2 space-y-1 text-xs">
              {topCpuCT && (
                <div className={`${topCpuCT.cpu > 0.5 ? 'text-yellow-500' : 'text-gray-500'}`}>
                  {topCpuCT.name}: {(topCpuCT.cpu * 100).toFixed(0)}%
                </div>
              )}
              {stoppedCTs.length > 0 && (
                <div className="text-gray-500 dark:text-gray-400">
                  stopped: {stoppedCTs.map(c => c.name).join(', ')}
                </div>
              )}
            </div>
          </div>
          {ceph && (
            <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
              <div className="text-sm text-gray-500 dark:text-gray-400">Ceph Health</div>
              <div className={`text-2xl font-bold ${
                ceph.health === 'HEALTH_OK' ? 'text-green-500' :
                ceph.health === 'HEALTH_WARN' ? 'text-yellow-500' : 'text-red-500'
              }`}>
                {ceph.health}
              </div>
              {ceph.checks && Object.keys(ceph.checks).length > 0 && (
                <div className="mt-2 space-y-1">
                  {Object.entries(ceph.checks).map(([name, check]) => (
                    <div key={name} className="text-xs">
                      <div className={`font-medium ${
                        check.severity === 'HEALTH_ERR' ? 'text-red-500' : 'text-yellow-500'
                      }`}>
                        {check.summary}
                      </div>
                      {check.detail && (
                        <div className="text-gray-500 dark:text-gray-400 pl-2">
                          {check.detail}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
              <div className="text-xs text-gray-500 mt-1">
                {formatBytes(ceph.bytes_used)} / {formatBytes(ceph.bytes_total)}
              </div>
              {ceph.health !== 'HEALTH_OK' && (
                <Link
                  to="/storage?tab=ceph"
                  className="mt-2 block text-center text-xs px-2 py-1 bg-blue-500 hover:bg-blue-600 text-white rounded"
                >
                  View Details
                </Link>
              )}
            </div>
          )}
        </div>

        {/* DRS Recommendations */}
        <div className="mb-6">
          <DRSPanel
            recommendations={drsRecommendations}
            onRefresh={() => window.location.reload()}
          />
        </div>

        {/* Nodes Grid */}
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Hosts</h2>
        {nodes.length > 0 && <QDeviceBanner cluster={nodes[0].cluster} />}
        <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
          {safeNodes.map((node) => {
            const cpuPercent = node.cpu * 100;
            const memPercent = (node.mem / node.maxmem) * 100;
            const nodeGuests = safeGuests.filter(g => g.node === node.node);
            const nodeRunning = nodeGuests.filter(g => g.status === 'running').length;

            return (
              <div key={node.node} className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <div className={`w-3 h-3 rounded-full ${node.status === 'online' ? 'bg-green-500' : 'bg-red-500'}`} />
                    <span className="font-semibold text-gray-900 dark:text-white">{node.node}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-gray-500">{nodeRunning}/{nodeGuests.length} guests</span>
                    <button
                      onClick={() => setMaintenanceNode({ node: node.node, cluster: node.cluster })}
                      className="text-xs px-2 py-1 bg-orange-100 hover:bg-orange-200 dark:bg-orange-900/30 dark:hover:bg-orange-900/50 text-orange-600 dark:text-orange-400 rounded"
                      title="Enter maintenance mode"
                    >
                      🔧
                    </button>
                  </div>
                </div>
                <div className="space-y-3">
                  <div>
                    <div className="flex justify-between text-xs mb-1">
                      <span className="text-gray-500">CPU ({node.maxcpu} cores)</span>
                      <span className="text-gray-700 dark:text-gray-300">{cpuPercent.toFixed(1)}%</span>
                    </div>
                    <ProgressBar value={cpuPercent} />
                  </div>
                  <div>
                    <div className="flex justify-between text-xs mb-1">
                      <span className="text-gray-500">Memory ({formatBytes(node.maxmem)})</span>
                      <span className="text-gray-700 dark:text-gray-300">{memPercent.toFixed(1)}%</span>
                    </div>
                    <ProgressBar value={memPercent} />
                  </div>
                </div>
              </div>
            );
          })}
        </div>

        {/* Resource Usage Charts */}
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Resource Usage</h2>
          <div className="flex gap-1">
            {(['1h', '6h', '24h', '7d', '30d'] as TimeRange[]).map((range) => (
              <button
                key={range}
                onClick={() => setTimeRange(range)}
                className={`px-2 py-1 text-xs rounded ${
                  timeRange === range
                    ? 'bg-blue-500 text-white'
                    : 'bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
                }`}
              >
                {range}
              </button>
            ))}
          </div>
        </div>
        <MetricsPanel
          nodeMetrics={nodeMetrics}
          vmNetMetrics={vmNetMetrics}
          ctNetMetrics={ctNetMetrics}
          guests={guests}
          loading={nodeMetricsLoading || vmNetLoading || ctNetLoading}
          timeRange={timeRange}
        />
      </div>

      {/* Maintenance Mode Modal */}
      {maintenanceNode && (
        <MaintenanceModal
          node={maintenanceNode.node}
          cluster={maintenanceNode.cluster}
          onClose={() => setMaintenanceNode(null)}
        />
      )}
    </Layout>
  );
}
