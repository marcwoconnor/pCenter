import { useState, useEffect, useMemo } from 'react';
import { useCluster } from '../context/ClusterContext';
import { api, formatBytes } from '../api/client';
import type { Datacenter, Node, Guest } from '../types';

interface Tab { id: string; label: string; }
const dcTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'configure', label: 'Configure' },
];

export function DatacenterDetail({ datacenterId, datacenterName, defaultTab }: {
  datacenterId: string; datacenterName: string; defaultTab?: string;
}) {
  const { nodes, guests } = useCluster();
  const [activeTab, setActiveTab] = useState(defaultTab || 'summary');
  const [datacenter, setDatacenter] = useState<Datacenter | null>(null);

  useEffect(() => { if (defaultTab) setActiveTab(defaultTab); }, [defaultTab]);

  useEffect(() => {
    api.getDatacenter(datacenterId).then(setDatacenter).catch(() => {});
  }, [datacenterId]);

  const dcClusterNames = useMemo(() => {
    if (!datacenter?.clusters) return [];
    return datacenter.clusters.map(c => c.agent_name || c.name);
  }, [datacenter]);

  const dcNodes = useMemo(() =>
    dcClusterNames.length > 0 ? (nodes || []).filter(n => dcClusterNames.includes(n.cluster)) : [],
    [nodes, dcClusterNames]);
  const dcGuests = useMemo(() =>
    dcClusterNames.length > 0 ? (guests || []).filter(g => dcClusterNames.includes(g.cluster)) : [],
    [guests, dcClusterNames]);

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 py-3 flex-shrink-0">
        <div className="flex items-center gap-3">
          <span className="text-2xl">🏛</span>
          <div>
            <h1 className="text-lg font-semibold text-gray-900 dark:text-white">{datacenterName}</h1>
            <div className="text-sm text-gray-500">
              Datacenter &middot; {datacenter?.clusters?.length || 0} clusters &middot; {datacenter?.hosts?.length || 0} standalone hosts
            </div>
          </div>
        </div>
      </div>

      <div className="bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 flex-shrink-0">
        <div className="flex gap-1">
          {dcTabs.map(tab => (
            <button key={tab.id} onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 text-sm font-medium transition-colors ${
                activeTab === tab.id
                  ? 'text-blue-600 border-b-2 border-blue-600 bg-white dark:bg-gray-700'
                  : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
              }`}>{tab.label}</button>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-auto p-4 bg-gray-50 dark:bg-gray-900">
        {activeTab === 'summary' && <DCSummary datacenter={datacenter} nodes={dcNodes} guests={dcGuests} />}
        {activeTab === 'configure' && <DCConfigure datacenter={datacenter} />}
      </div>
    </div>
  );
}

function DCSummary({ datacenter, nodes, guests }: { datacenter: Datacenter | null; nodes: Node[]; guests: Guest[] }) {
  const onlineNodes = nodes.filter(n => n.status === 'online');
  const runningVMs = guests.filter(g => g.type === 'qemu' && g.status === 'running');
  const totalVMs = guests.filter(g => g.type === 'qemu');
  const runningCTs = guests.filter(g => g.type === 'lxc' && g.status === 'running');
  const totalCTs = guests.filter(g => g.type === 'lxc');
  const totalCPU = nodes.reduce((s, n) => s + n.maxcpu, 0);
  const usedCPU = nodes.reduce((s, n) => s + n.cpu * n.maxcpu, 0);
  const totalMem = nodes.reduce((s, n) => s + n.maxmem, 0);
  const usedMem = nodes.reduce((s, n) => s + n.mem, 0);
  const cpuPct = totalCPU > 0 ? (usedCPU / totalCPU) * 100 : 0;
  const memPct = totalMem > 0 ? (usedMem / totalMem) * 100 : 0;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <Card title="Clusters" value={String(datacenter?.clusters?.length || 0)} sub="configured" color="blue" />
        <Card title="Nodes" value={`${onlineNodes.length}/${nodes.length}`} sub="online" color={onlineNodes.length === nodes.length ? 'green' : 'yellow'} />
        <Card title="VMs" value={`${runningVMs.length}/${totalVMs.length}`} sub="running" color="blue" />
        <Card title="Containers" value={`${runningCTs.length}/${totalCTs.length}`} sub="running" color="blue" />
      </div>
      {nodes.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Resources</h3>
          <div className="space-y-4">
            <Bar label="CPU" value={cpuPct} detail={`${usedCPU.toFixed(1)} / ${totalCPU} cores`} />
            <Bar label="Memory" value={memPct} detail={`${formatBytes(usedMem)} / ${formatBytes(totalMem)}`} />
          </div>
        </div>
      )}
      {datacenter?.clusters && datacenter.clusters.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Clusters</h3>
          <div className="space-y-2">
            {datacenter.clusters.map(c => (
              <div key={c.id} className="flex items-center justify-between p-2 border border-gray-200 dark:border-gray-700 rounded">
                <span className="font-medium text-gray-900 dark:text-white">{c.name}</span>
                <span className={`px-2 py-0.5 rounded text-xs ${
                  c.status === 'active' ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
                }`}>{c.status}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function DCConfigure({ datacenter }: { datacenter: Datacenter | null }) {
  if (!datacenter) return <div className="text-gray-500">Loading...</div>;
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
      <h3 className="font-medium mb-4 text-gray-900 dark:text-white pb-2 border-b border-gray-200 dark:border-gray-700">General</h3>
      <div className="space-y-2 text-sm">
        <Row label="Name" value={datacenter.name} />
        <Row label="Description" value={datacenter.description || '(none)'} />
        <Row label="Created" value={new Date(datacenter.created_at).toLocaleString()} />
        <Row label="Clusters" value={String(datacenter.clusters?.length || 0)} />
        <Row label="Standalone Hosts" value={String(datacenter.hosts?.length || 0)} />
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
      <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
        <div className={`h-full rounded ${color}`} style={{ width: `${Math.min(value, 100)}%` }} />
      </div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between py-1.5">
      <span className="text-gray-500">{label}</span>
      <span className="text-gray-900 dark:text-white">{value}</span>
    </div>
  );
}
