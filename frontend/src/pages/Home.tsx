import { useCluster } from '../context/ClusterContext';
import { Layout } from '../components/Layout';
import { DRSPanel } from '../components/DRSPanel';
import { formatBytes } from '../api/client';

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

export function Home() {
  const { summary, nodes, guests, ceph, drsRecommendations, isLoading } = useCluster();

  if (isLoading) {
    return (
      <Layout>
        <div className="flex-1 flex items-center justify-center">
          <div className="text-gray-500">Loading...</div>
        </div>
      </Layout>
    );
  }

  const runningVMs = guests.filter(g => g.type === 'qemu' && g.status === 'running').length;
  const runningCTs = guests.filter(g => g.type === 'lxc' && g.status === 'running').length;
  const totalVMs = guests.filter(g => g.type === 'qemu').length;
  const totalCTs = guests.filter(g => g.type === 'lxc').length;

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
          </div>
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
            <div className="text-sm text-gray-500 dark:text-gray-400">Virtual Machines</div>
            <div className="text-2xl font-bold text-gray-900 dark:text-white">
              {runningVMs}/{totalVMs}
            </div>
            <div className="text-xs text-green-500">running</div>
          </div>
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
            <div className="text-sm text-gray-500 dark:text-gray-400">Containers</div>
            <div className="text-2xl font-bold text-gray-900 dark:text-white">
              {runningCTs}/{totalCTs}
            </div>
            <div className="text-xs text-green-500">running</div>
          </div>
          {ceph && (
            <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
              <div className="text-sm text-gray-500 dark:text-gray-400">Ceph Health</div>
              <div className={`text-2xl font-bold ${
                ceph.health === 'HEALTH_OK' ? 'text-green-500' :
                ceph.health === 'HEALTH_WARN' ? 'text-yellow-500' : 'text-red-500'
              }`}>
                {ceph.health.replace('HEALTH_', '')}
              </div>
              <div className="text-xs text-gray-500">
                {formatBytes(ceph.bytes_used)} / {formatBytes(ceph.bytes_total)}
              </div>
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
        <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
          {nodes.map((node) => {
            const cpuPercent = node.cpu * 100;
            const memPercent = (node.mem / node.maxmem) * 100;
            const nodeGuests = guests.filter(g => g.node === node.node);
            const nodeRunning = nodeGuests.filter(g => g.status === 'running').length;

            return (
              <div key={node.node} className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <div className={`w-3 h-3 rounded-full ${node.status === 'online' ? 'bg-green-500' : 'bg-red-500'}`} />
                    <span className="font-semibold text-gray-900 dark:text-white">{node.node}</span>
                  </div>
                  <span className="text-sm text-gray-500">{nodeRunning}/{nodeGuests.length} guests</span>
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

        {/* Recent Activity placeholder */}
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Resource Usage</h2>
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <div className="text-sm text-gray-500 text-center py-8">
            Resource graphs coming soon...
          </div>
        </div>
      </div>
    </Layout>
  );
}
