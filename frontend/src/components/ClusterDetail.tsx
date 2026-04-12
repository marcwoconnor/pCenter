import { useState, useEffect, useMemo } from 'react';
import { useCluster } from '../context/ClusterContext';
import { formatBytes } from '../api/client';
import { DRSPanel } from './DRSPanel';
import type { Node, Guest } from '../types';

interface Tab { id: string; label: string; }
const clusterTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'ha', label: 'HA' },
  { id: 'drs', label: 'DRS' },
  { id: 'configure', label: 'Configure' },
];

interface HAStatus {
  enabled: boolean;
  quorum: boolean;
  manager: { node: string; status: string };
  resources: Array<{ sid: string; type: string; status: string; node: string; state: string }>;
}
interface HAGroup {
  group: string; comment?: string; nodes: string[]; nofailback?: boolean; restricted?: boolean;
}

export function ClusterDetail({ clusterName, displayName, defaultTab }: { clusterName: string; displayName?: string; defaultTab?: string }) {
  const { nodes, guests, drsRecommendations, getCluster } = useCluster();
  const [activeTab, setActiveTab] = useState(defaultTab || 'summary');

  useEffect(() => { if (defaultTab) setActiveTab(defaultTab); }, [defaultTab]);

  const cluster = getCluster(clusterName);
  const cn = useMemo(() => (nodes || []).filter(n => n.cluster === clusterName), [nodes, clusterName]);
  const cg = useMemo(() => (guests || []).filter(g => g.cluster === clusterName), [guests, clusterName]);
  const cd = useMemo(() => (drsRecommendations || []).filter(r => r.cluster === clusterName), [drsRecommendations, clusterName]);

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 py-3 flex-shrink-0">
        <div className="flex items-center gap-3">
          <span className="text-2xl">🏢</span>
          <div>
            <h1 className="text-lg font-semibold text-gray-900 dark:text-white">{displayName || clusterName}</h1>
            <div className="text-sm text-gray-500">
              Proxmox Cluster &middot; {cn.length} nodes &middot;{' '}
              {cg.filter(g => g.type === 'qemu').length} VMs &middot;{' '}
              {cg.filter(g => g.type === 'lxc').length} CTs
            </div>
          </div>
        </div>
      </div>

      <div className="bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 flex-shrink-0">
        <div className="flex gap-1">
          {clusterTabs.map(tab => (
            <button key={tab.id} onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 text-sm font-medium transition-colors ${
                activeTab === tab.id
                  ? 'text-blue-600 border-b-2 border-blue-600 bg-white dark:bg-gray-700'
                  : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
              }`}>
              {tab.label}
              {tab.id === 'drs' && cd.length > 0 && (
                <span className="ml-1.5 px-1.5 py-0.5 bg-yellow-500 text-white text-xs rounded-full">{cd.length}</span>
              )}
            </button>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-auto p-4 bg-gray-50 dark:bg-gray-900">
        {activeTab === 'summary' && <ClusterSummary nodes={cn} guests={cg} ha={cluster?.ha} />}
        {activeTab === 'ha' && <ClusterHA clusterName={clusterName} />}
        {activeTab === 'drs' && <DRSPanel recommendations={cd} onRefresh={() => window.location.reload()} />}
        {activeTab === 'configure' && <ClusterConfigure clusterName={clusterName} cluster={cluster} />}
      </div>
    </div>
  );
}

function ClusterSummary({ nodes, guests, ha }: { nodes: Node[]; guests: Guest[]; ha?: { enabled: boolean; quorum: boolean; manager: string } }) {
  const onlineNodes = nodes.filter(n => n.status === 'online');
  const runningVMs = guests.filter(g => g.type === 'qemu' && g.status === 'running');
  const totalVMs = guests.filter(g => g.type === 'qemu');
  const runningCTs = guests.filter(g => g.type === 'lxc' && g.status === 'running');
  const totalCTs = guests.filter(g => g.type === 'lxc');
  const totalCPU = nodes.reduce((s, n) => s + n.maxcpu, 0);
  const usedCPU = nodes.reduce((s, n) => s + n.cpu * n.maxcpu, 0);
  const totalMem = nodes.reduce((s, n) => s + n.maxmem, 0);
  const usedMem = nodes.reduce((s, n) => s + n.mem, 0);
  const totalDisk = nodes.reduce((s, n) => s + n.maxdisk, 0);
  const usedDisk = nodes.reduce((s, n) => s + n.disk, 0);
  const cpuPct = totalCPU > 0 ? (usedCPU / totalCPU) * 100 : 0;
  const memPct = totalMem > 0 ? (usedMem / totalMem) * 100 : 0;
  const diskPct = totalDisk > 0 ? (usedDisk / totalDisk) * 100 : 0;
  const versions = [...new Set(nodes.map(n => n.pve_version).filter(Boolean))];

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <Card title="Nodes" value={`${onlineNodes.length}/${nodes.length}`} sub="online" color={onlineNodes.length === nodes.length ? 'green' : 'yellow'} />
        <Card title="VMs" value={`${runningVMs.length}/${totalVMs.length}`} sub="running" color="blue" />
        <Card title="Containers" value={`${runningCTs.length}/${totalCTs.length}`} sub="running" color="blue" />
        <Card title="HA" value={ha?.enabled ? (ha.quorum ? 'OK' : 'No Quorum') : 'Disabled'} sub={ha?.manager ? `mgr: ${ha.manager}` : ''} color={ha?.enabled ? (ha.quorum ? 'green' : 'red') : 'gray'} />
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Cluster Resources</h3>
        <div className="space-y-4">
          <Bar label="CPU" value={cpuPct} detail={`${usedCPU.toFixed(1)} / ${totalCPU} cores`} />
          <Bar label="Memory" value={memPct} detail={`${formatBytes(usedMem)} / ${formatBytes(totalMem)}`} />
          <Bar label="Storage" value={diskPct} detail={`${formatBytes(usedDisk)} / ${formatBytes(totalDisk)}`} />
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Nodes</h3>
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
              <th className="pb-2 font-medium">Node</th><th className="pb-2 font-medium">Status</th>
              <th className="pb-2 font-medium">CPU</th><th className="pb-2 font-medium">Memory</th>
              <th className="pb-2 font-medium">VMs</th><th className="pb-2 font-medium">CTs</th>
            </tr>
          </thead>
          <tbody>
            {nodes.map(node => {
              const nv = guests.filter(g => g.node === node.node && g.type === 'qemu');
              const nc = guests.filter(g => g.node === node.node && g.type === 'lxc');
              return (
                <tr key={node.node} className="border-b border-gray-100 dark:border-gray-700/50">
                  <td className="py-2 font-medium text-gray-900 dark:text-white">{node.node}</td>
                  <td className="py-2"><span className={`px-2 py-0.5 rounded text-xs ${node.status === 'online' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'}`}>{node.status}</span></td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{(node.cpu * 100).toFixed(1)}%</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{node.maxmem > 0 ? ((node.mem / node.maxmem) * 100).toFixed(1) : 0}%</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{nv.filter(g => g.status === 'running').length}/{nv.length}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{nc.filter(g => g.status === 'running').length}/{nc.length}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {versions.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Software</h3>
          <div className="text-sm">
            <span className="text-gray-500">PVE Version: </span>
            <span className={`text-gray-900 dark:text-white ${versions.length > 1 ? 'text-yellow-600 dark:text-yellow-400' : ''}`}>
              {versions.length === 1 ? versions[0] : `${versions.join(', ')} (drift!)`}
            </span>
          </div>
        </div>
      )}
    </div>
  );
}

function ClusterHA({ clusterName }: { clusterName: string }) {
  const [haStatus, setHaStatus] = useState<HAStatus | null>(null);
  const [haGroups, setHaGroups] = useState<HAGroup[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      fetch(`/api/clusters/${clusterName}/ha/status`, { credentials: 'include' }).then(r => r.ok ? r.json() : null).catch(() => null),
      fetch(`/api/clusters/${clusterName}/ha/groups`, { credentials: 'include' }).then(r => r.ok ? r.json() : []).catch(() => []),
    ]).then(([status, groups]) => { setHaStatus(status); setHaGroups(groups || []); })
      .finally(() => setLoading(false));
  }, [clusterName]);

  if (loading) return <div className="text-gray-500 p-4">Loading HA status...</div>;
  if (!haStatus) return <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6 text-center text-gray-500">HA not available</div>;

  return (
    <div className="space-y-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">HA Status</h3>
        <div className="grid md:grid-cols-3 gap-4 text-sm">
          <div><span className="text-gray-500">Enabled: </span><span className={haStatus.enabled ? 'text-green-600' : 'text-gray-400'}>{haStatus.enabled ? 'Yes' : 'No'}</span></div>
          <div><span className="text-gray-500">Quorum: </span><span className={haStatus.quorum ? 'text-green-600' : 'text-red-600'}>{haStatus.quorum ? 'OK' : 'Lost'}</span></div>
          <div><span className="text-gray-500">Manager: </span><span className="text-gray-900 dark:text-white">{haStatus.manager?.node || 'N/A'}</span></div>
        </div>
      </div>

      {haStatus.resources && haStatus.resources.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Managed Resources ({haStatus.resources.length})</h3>
          <table className="w-full text-sm">
            <thead><tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
              <th className="pb-2 font-medium">Resource</th><th className="pb-2 font-medium">Node</th><th className="pb-2 font-medium">Status</th>
            </tr></thead>
            <tbody>
              {haStatus.resources.map(r => (
                <tr key={r.sid} className="border-b border-gray-100 dark:border-gray-700/50">
                  <td className="py-2 text-gray-900 dark:text-white">{r.sid}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{r.node}</td>
                  <td className="py-2"><span className={`px-2 py-0.5 rounded text-xs ${r.status === 'started' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'}`}>{r.status}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {haGroups.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Failover Groups</h3>
          {haGroups.map(g => (
            <div key={g.group} className="border border-gray-200 dark:border-gray-700 rounded p-3 mb-2">
              <span className="font-medium text-gray-900 dark:text-white">{g.group}</span>
              {g.comment && <div className="text-sm text-gray-500">{g.comment}</div>}
              <div className="text-sm text-gray-700 dark:text-gray-300">Nodes: {g.nodes.join(' → ')}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function ClusterConfigure({ clusterName, cluster }: { clusterName: string; cluster: ReturnType<ReturnType<typeof useCluster>['getCluster']> }) {
  return (
    <div className="space-y-6">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-4 text-gray-900 dark:text-white pb-2 border-b border-gray-200 dark:border-gray-700">Connection</h3>
        <div className="grid md:grid-cols-2 gap-4 text-sm">
          <Row label="Cluster Name" value={clusterName} />
          <Row label="Nodes" value={`${cluster?.summary?.TotalNodes || 0} total`} />
        </div>
      </div>
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-4 text-gray-900 dark:text-white pb-2 border-b border-gray-200 dark:border-gray-700">DRS</h3>
        <p className="text-sm text-gray-500">DRS settings apply globally. Configure under pCenter → Configure → DRS.</p>
      </div>
    </div>
  );
}

function Card({ title, value, sub, color }: { title: string; value: string; sub: string; color: string }) {
  const c: Record<string, string> = { green: 'border-green-500', yellow: 'border-yellow-500', red: 'border-red-500', blue: 'border-blue-500', gray: 'border-gray-400' };
  return (
    <div className={`bg-white dark:bg-gray-800 rounded-lg shadow p-4 border-l-4 ${c[color] || c.gray}`}>
      <div className="text-sm text-gray-500">{title}</div>
      <div className="text-2xl font-bold text-gray-900 dark:text-white">{value}</div>
      <div className="text-xs text-gray-400">{sub}</div>
    </div>
  );
}

function Bar({ label, value, detail }: { label: string; value: number; detail: string }) {
  const color = value > 90 ? 'bg-red-500' : value > 70 ? 'bg-yellow-500' : 'bg-blue-500';
  return (
    <div>
      <div className="flex justify-between text-sm mb-1">
        <span className="text-gray-500">{label}</span>
        <span className="text-gray-900 dark:text-white">{value.toFixed(1)}% ({detail})</span>
      </div>
      <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded"><div className={`h-full rounded ${color}`} style={{ width: `${Math.min(value, 100)}%` }} /></div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (<div className="flex justify-between py-1.5"><span className="text-gray-500">{label}</span><span className="text-gray-900 dark:text-white">{value}</span></div>);
}
