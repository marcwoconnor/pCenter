import { useState, useMemo, useEffect, useRef } from 'react';
import { useCluster } from '../context/ClusterContext';
import { formatBytes, formatUptime, api } from '../api/client';
import { useMetrics } from '../hooks/useMetrics';
import { useConfigEditor, type UseConfigEditorReturn } from '../hooks/useConfigEditor';
import { MetricsChart } from './MetricsChart';
import type { MetricSeries, VMConfig, ContainerConfig, NetworkInterface, Node, Guest, Storage, StorageVolume, Tag, TagAssignment, NodeConfig } from '../types';
import { NetworkTopology } from './NetworkTopology';
import { SnapshotsTab } from './SnapshotsTab';
import { ErrorBoundary } from './ErrorBoundary';
import { TagPicker } from './TagPicker';
import { PCenterRootDetail } from './PCenterRootDetail';
import { DatacenterDetail } from './DatacenterDetail';
import { ClusterDetail } from './ClusterDetail';
import { NodeCertificatesTab } from './ACMEPanels';

type TimeRange = '1h' | '6h' | '24h' | '7d' | '30d';

// Minimal type for noVNC RFB instance. The @novnc/novnc package doesn't
// ship TypeScript declarations, so we define the subset we actually use.
interface RFBInstance {
  scaleViewport: boolean;
  clipViewport: boolean;
  resizeSession: boolean;
  _screen?: { width: number; height: number };
  addEventListener(type: string, listener: (e: CustomEvent) => void): void;
  disconnect(): void;
  sendKey(keysym: number, code: string, down: boolean): void;
  focus(): void;
}

interface Tab {
  id: string;
  label: string;
}

const nodeTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'vms', label: 'Virtual Machines' },
  { id: 'configure', label: 'Configure' },
  { id: 'certificates', label: 'Certificates' },
  { id: 'monitor', label: 'Monitor' },
];

const guestTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'console', label: 'Console' },
  { id: 'snapshots', label: 'Snapshots' },
  { id: 'monitor', label: 'Monitor' },
  { id: 'configure', label: 'Configure' },
];

const storageTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'vms', label: 'Content' },
];

const networkTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'topology', label: 'Virtual Switches' },
];

export function ObjectDetail() {
  const { selectedObject, nodes, guests, storage, performAction, tags, tagAssignments, refreshTags } = useCluster();
  const [activeTab, setActiveTab] = useState('summary');

  // Handle defaultTab from context menu navigation
  /* eslint-disable react-hooks/set-state-in-effect --
     sync defaultTab prop into tab state on object change; parent deep-links via defaultTab */
  useEffect(() => {
    if (selectedObject?.defaultTab) {
      setActiveTab(selectedObject.defaultTab);
    } else {
      setActiveTab('summary');
    }
  }, [selectedObject?.id, selectedObject?.defaultTab]);
  /* eslint-enable react-hooks/set-state-in-effect */

  // Memoize the NetworkTopology element. Declared above the early returns so
  // React's rules-of-hooks is satisfied — returns null for non-network or
  // null selectedObject, which the actual render site handles.
  const networkTopologyElement = useMemo(() => {
    if (!selectedObject || selectedObject.type !== 'network') return null;
    const node = selectedObject.node || '';
    const cluster = selectedObject.cluster || '';
    return <NetworkTopology node={node} cluster={cluster} />;
  }, [selectedObject]);

  if (!selectedObject) {
    return (
      <div className="flex-1 flex items-center justify-center text-gray-500">
        Select an object from the inventory tree
      </div>
    );
  }

  // Delegate to specialized detail components for higher-level objects
  if (selectedObject.type === 'datacenter') {
    if (selectedObject.id === 'root') {
      return <PCenterRootDetail defaultTab={selectedObject.defaultTab} />;
    }
    return <DatacenterDetail datacenterId={String(selectedObject.id)} datacenterName={selectedObject.name} defaultTab={selectedObject.defaultTab} />;
  }
  if (selectedObject.type === 'cluster') {
    return <ClusterDetail clusterName={selectedObject.cluster || String(selectedObject.id)} displayName={selectedObject.name} defaultTab={selectedObject.defaultTab} />;
  }

  const tabs = selectedObject.type === 'node' ? nodeTabs :
    selectedObject.type === 'storage' ? storageTabs :
    selectedObject.type === 'network' ? networkTabs : guestTabs;

  // Get the actual object data
  // Cluster comparison optional - inventory hosts/orphan nodes don't have cluster set
  const node = selectedObject.type === 'node'
    ? nodes.find((n) => n.node === selectedObject.id && (!selectedObject.cluster || n.cluster === selectedObject.cluster))
    : null;

  const guest = (selectedObject.type === 'vm' || selectedObject.type === 'ct')
    ? guests.find((g) => g.vmid === selectedObject.id && (!selectedObject.cluster || g.cluster === selectedObject.cluster))
    : null;

  const storageItem = selectedObject.type === 'storage'
    ? storage.find((s) => `${s.node}-${s.storage}` === selectedObject.id)
    : null;

  const handleAction = async (action: 'start' | 'stop' | 'shutdown') => {
    if (!guest) return;
    try {
      await performAction(guest.type === 'qemu' ? 'vm' : 'ct', guest.vmid, action);
    } catch {
      // Error is shown in tasks bar
    }
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Object Header */}
      <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="text-2xl">
              {selectedObject.type === 'node' && '🖥'}
              {selectedObject.type === 'vm' && '💻'}
              {selectedObject.type === 'ct' && '📦'}
              {selectedObject.type === 'storage' && '💾'}
              {selectedObject.type === 'network' && '🌐'}
            </span>
            <div>
              <h1 className="text-lg font-semibold text-gray-900 dark:text-white">
                {selectedObject.name}
              </h1>
              <div className="text-sm text-gray-500">
                {selectedObject.type === 'node' && 'Proxmox Node'}
                {selectedObject.type === 'vm' && `VM ${selectedObject.id} on ${selectedObject.node}`}
                {selectedObject.type === 'ct' && `Container ${selectedObject.id} on ${selectedObject.node}`}
                {selectedObject.type === 'storage' && `Storage on ${selectedObject.node}`}
                {selectedObject.type === 'network' && `Network Interface on ${selectedObject.node}`}
              </div>
            </div>
          </div>

          {/* Actions */}
          {guest && (
            <div className="flex gap-2">
              {guest.status === 'stopped' ? (
                <button
                  onClick={() => handleAction('start')}
                  className="px-3 py-1.5 bg-green-600 text-white text-sm rounded hover:bg-green-700"
                >
                  Start
                </button>
              ) : (
                <>
                  <button
                    onClick={() => handleAction('shutdown')}
                    className="px-3 py-1.5 bg-yellow-600 text-white text-sm rounded hover:bg-yellow-700"
                  >
                    Shutdown
                  </button>
                  <button
                    onClick={() => handleAction('stop')}
                    className="px-3 py-1.5 bg-red-600 text-white text-sm rounded hover:bg-red-700"
                  >
                    Stop
                  </button>
                </>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 flex-shrink-0">
        <div className="flex gap-1">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 text-sm font-medium transition-colors ${
                activeTab === tab.id || (tab.id === 'vms' && (activeTab === 'only-vms' || activeTab === 'only-cts'))
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
        {activeTab === 'summary' && node && <NodeSummary node={node} />}
        {activeTab === 'summary' && guest && (
          <GuestSummary
            guest={guest}
            tags={tags}
            tagAssignments={tagAssignments}
            onTagsChanged={refreshTags}
          />
        )}
        {activeTab === 'summary' && storageItem && <StorageSummary storage={storageItem} />}
        {activeTab === 'summary' && selectedObject.type === 'network' && (
          <NetworkSummary ifaceName={selectedObject.name} node={selectedObject.node || ''} cluster={selectedObject.cluster || ''} />
        )}
        {/* Memoized NetworkTopology to prevent re-renders from parent context updates */}
        {networkTopologyElement && (
          <div className={activeTab === 'topology' ? 'block' : 'hidden'}>
            {networkTopologyElement}
          </div>
        )}
        {activeTab === 'console' && guest && (
          <ErrorBoundary fallback={
            <div className="flex items-center justify-center h-64 text-gray-500">
              Console failed to initialize. The VM may need to be running.
            </div>
          }>
            <ConsoleTab
              vmid={guest.vmid}
              type={guest.type === 'qemu' ? 'vm' : 'ct'}
              name={guest.name}
              isRunning={guest.status === 'running'}
            />
          </ErrorBoundary>
        )}
        {(activeTab === 'vms' || activeTab === 'only-vms' || activeTab === 'only-cts') && node && (
          <NodeVMs nodeId={node.node} typeFilter={activeTab === 'only-vms' ? 'qemu' : activeTab === 'only-cts' ? 'lxc' : undefined} />
        )}
        {activeTab === 'vms' && storageItem && <StorageVMs storage={storageItem} />}
        {activeTab === 'configure' && node && (
          <NodeConfigureTab node={node.node} cluster={node.cluster} />
        )}
        {activeTab === 'certificates' && node && (
          <NodeCertificatesTab node={node.node} cluster={node.cluster} />
        )}
        {activeTab === 'monitor' && node && <NodeMonitorTab node={node.node} />}
        {activeTab === 'monitor' && guest && (
          <GuestMonitorTab
            vmid={guest.vmid}
            type={guest.type === 'qemu' ? 'vm' : 'ct'}
            isRunning={guest.status === 'running'}
          />
        )}
        {activeTab === 'snapshots' && guest && (
          <SnapshotsTab guest={guest} />
        )}
        {activeTab === 'configure' && guest && (
          <ConfigureTab
            vmid={guest.vmid}
            type={guest.type === 'qemu' ? 'vm' : 'ct'}
            cluster={guest.cluster}
          />
        )}
      </div>
    </div>
  );
}

function NodeSummary({ node }: { node: Node }) {
  const cpuPercent = (node.cpu * 100).toFixed(1);
  const memPercent = ((node.mem / node.maxmem) * 100).toFixed(1);

  return (
    <div className="grid md:grid-cols-2 gap-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Status</h3>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">Status</span>
            <span className={node.status === 'online' ? 'text-green-600' : 'text-red-600'}>
              {node.status}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Uptime</span>
            <span className="text-gray-900 dark:text-white">{formatUptime(node.uptime)}</span>
          </div>
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Resources</h3>
        <div className="space-y-3">
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">CPU ({node.maxcpu} cores)</span>
              <span className="text-gray-900 dark:text-white">{cpuPercent}%</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className={`h-full rounded ${parseFloat(cpuPercent) > 80 ? 'bg-red-500' : 'bg-blue-500'}`}
                style={{ width: `${cpuPercent}%` }}
              />
            </div>
          </div>
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">Memory ({formatBytes(node.maxmem)})</span>
              <span className="text-gray-900 dark:text-white">{memPercent}%</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className={`h-full rounded ${parseFloat(memPercent) > 80 ? 'bg-red-500' : 'bg-green-500'}`}
                style={{ width: `${Math.min(parseFloat(memPercent), 100)}%` }}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function NodeConfigureTab({ node, cluster }: { node: string; cluster: string }) {
  const [config, setConfig] = useState<NodeConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const reload = () => {
    setLoading(true);
    setError(null);
    api.getNodeConfig(cluster, node)
      .then(setConfig)
      .catch((err) => setError(err.message || 'Failed to load config'))
      .finally(() => setLoading(false));
  };

  /* eslint-disable react-hooks/set-state-in-effect --
     fetch-on-mount for node config with cancellation guard */
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    api.getNodeConfig(cluster, node)
      .then((data) => { if (!cancelled) setConfig(data); })
      .catch((err) => { if (!cancelled) setError(err.message || 'Failed to load config'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [node, cluster]);
  /* eslint-enable react-hooks/set-state-in-effect */

  if (loading) {
    return <div className="text-center py-8 text-gray-500">Loading configuration...</div>;
  }
  if (error) {
    return <div className="text-center py-8 text-red-500">Error: {error}</div>;
  }
  if (!config) return null;

  return (
    <div className="space-y-4">
      {/* System Info (read-only) */}
      {config.status && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">System</h3>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-500">PVE Version</span>
              <span className="text-gray-900 dark:text-white">{config.status.pveversion}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">Kernel</span>
              <span className="text-gray-900 dark:text-white">{config.status.kversion}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">CPU Model</span>
              <span className="text-gray-900 dark:text-white">{config.status.cpu_model}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">CPU Topology</span>
              <span className="text-gray-900 dark:text-white">{config.status.cpu_sockets}s / {config.status.cpu_cores}c</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">Boot Mode</span>
              <span className="text-gray-900 dark:text-white">{config.status.boot_mode}</span>
            </div>
            {config.status.loadavg && (
              <div className="flex justify-between">
                <span className="text-gray-500">Load Average</span>
                <span className="text-gray-900 dark:text-white">{config.status.loadavg.join(', ')}</span>
              </div>
            )}
          </div>
        </div>
      )}

      <div className="grid md:grid-cols-2 gap-4">
        <NodeDNSCard dns={config.dns} cluster={cluster} node={node} onSaved={reload} />
        <NodeTimeCard time={config.time} cluster={cluster} node={node} onSaved={reload} />
      </div>

      {/* Subscription (read-only) */}
      {config.subscription && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Subscription</h3>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-500">Status</span>
              <span className={config.subscription.status === 'active' ? 'text-green-600' : 'text-yellow-600'}>
                {config.subscription.status}
              </span>
            </div>
            {config.subscription.level && (
              <div className="flex justify-between">
                <span className="text-gray-500">Level</span>
                <span className="text-gray-900 dark:text-white">{config.subscription.level}</span>
              </div>
            )}
            {config.subscription.serverid && (
              <div className="flex justify-between">
                <span className="text-gray-500">Server ID</span>
                <span className="text-gray-900 dark:text-white font-mono text-xs">{config.subscription.serverid}</span>
              </div>
            )}
            {config.subscription.nextduedate && (
              <div className="flex justify-between">
                <span className="text-gray-500">Next Due</span>
                <span className="text-gray-900 dark:text-white">{config.subscription.nextduedate}</span>
              </div>
            )}
          </div>
        </div>
      )}

      <NodeNetworkCard network={config.network} cluster={cluster} node={node} onSaved={reload} />
      <NodeHostsCard hosts={config.hosts} cluster={cluster} node={node} onSaved={reload} />

      {/* APT Repositories (read-only) */}
      {config.apt_repos && config.apt_repos.files && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white">APT Repositories</h3>
          <div className="space-y-3">
            {config.apt_repos.files.map((file) =>
              file.repositories.map((repo, idx) => (
                <div
                  key={`${file.path}-${idx}`}
                  className={`text-sm rounded p-3 border ${
                    repo.Enabled
                      ? 'bg-gray-50 dark:bg-gray-900 border-gray-200 dark:border-gray-700'
                      : 'bg-gray-100 dark:bg-gray-900/50 border-gray-300 dark:border-gray-600 opacity-60'
                  }`}
                >
                  <div className="flex items-center gap-2 mb-1">
                    <span className={`inline-block w-2 h-2 rounded-full ${repo.Enabled ? 'bg-green-500' : 'bg-gray-400'}`} />
                    <span className="font-mono text-xs text-gray-500">{file.path}</span>
                  </div>
                  <div className="font-mono text-xs text-gray-900 dark:text-gray-300">
                    {repo.Types?.join(' ')} {repo.URIs?.join(' ')} {repo.Suites?.join(' ')} {repo.Components?.join(' ')}
                  </div>
                  {repo.Comment && (
                    <div className="text-xs text-gray-500 mt-1">{repo.Comment}</div>
                  )}
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// --- Editable sub-components for NodeConfigureTab ---

function NodeDNSCard({ dns, cluster, node, onSaved }: {
  dns: NodeConfig['dns']; cluster: string; node: string; onSaved: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');
  const [form, setForm] = useState({ search: '', dns1: '', dns2: '', dns3: '' });

  const startEdit = () => {
    if (dns) setForm({ search: dns.search, dns1: dns.dns1, dns2: dns.dns2 || '', dns3: dns.dns3 || '' });
    setErr('');
    setEditing(true);
  };

  const save = async () => {
    setSaving(true);
    setErr('');
    try {
      await api.updateNodeDNS(cluster, node, form);
      setEditing(false);
      onSaved();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  if (!dns && !editing) return null;

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="font-medium text-gray-900 dark:text-white">DNS</h3>
        {!editing && (
          <button onClick={startEdit} className="text-xs text-blue-600 hover:text-blue-700">Edit</button>
        )}
      </div>
      {editing ? (
        <div className="space-y-2">
          <label className="block text-sm">
            <span className="text-gray-500">Search Domain</span>
            <input value={form.search} onChange={e => setForm(f => ({ ...f, search: e.target.value }))}
              className="mt-1 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white" />
          </label>
          <label className="block text-sm">
            <span className="text-gray-500">DNS Server 1</span>
            <input value={form.dns1} onChange={e => setForm(f => ({ ...f, dns1: e.target.value }))}
              className="mt-1 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
          </label>
          <label className="block text-sm">
            <span className="text-gray-500">DNS Server 2</span>
            <input value={form.dns2} onChange={e => setForm(f => ({ ...f, dns2: e.target.value }))}
              className="mt-1 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
          </label>
          <label className="block text-sm">
            <span className="text-gray-500">DNS Server 3</span>
            <input value={form.dns3} onChange={e => setForm(f => ({ ...f, dns3: e.target.value }))}
              className="mt-1 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
          </label>
          {err && <div className="text-red-500 text-xs">{err}</div>}
          <div className="flex gap-2 pt-1">
            <button onClick={save} disabled={saving}
              className="px-3 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
              {saving ? 'Saving...' : 'Save'}
            </button>
            <button onClick={() => setEditing(false)} className="px-3 py-1 text-xs text-gray-600 dark:text-gray-400 hover:text-gray-900">
              Cancel
            </button>
          </div>
        </div>
      ) : (
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">Search Domain</span>
            <span className="text-gray-900 dark:text-white">{dns!.search}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">DNS Server 1</span>
            <span className="text-gray-900 dark:text-white font-mono">{dns!.dns1}</span>
          </div>
          {dns!.dns2 && (
            <div className="flex justify-between">
              <span className="text-gray-500">DNS Server 2</span>
              <span className="text-gray-900 dark:text-white font-mono">{dns!.dns2}</span>
            </div>
          )}
          {dns!.dns3 && (
            <div className="flex justify-between">
              <span className="text-gray-500">DNS Server 3</span>
              <span className="text-gray-900 dark:text-white font-mono">{dns!.dns3}</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function NodeTimeCard({ time, cluster, node, onSaved }: {
  time: NodeConfig['time']; cluster: string; node: string; onSaved: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');
  const [timezone, setTimezone] = useState('');

  const startEdit = () => {
    if (time) setTimezone(time.timezone);
    setErr('');
    setEditing(true);
  };

  const save = async () => {
    setSaving(true);
    setErr('');
    try {
      await api.updateNodeTimezone(cluster, node, timezone);
      setEditing(false);
      onSaved();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  if (!time && !editing) return null;

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="font-medium text-gray-900 dark:text-white">Time</h3>
        {!editing && (
          <button onClick={startEdit} className="text-xs text-blue-600 hover:text-blue-700">Edit</button>
        )}
      </div>
      {editing ? (
        <div className="space-y-2">
          <label className="block text-sm">
            <span className="text-gray-500">Timezone</span>
            <input value={timezone} onChange={e => setTimezone(e.target.value)}
              placeholder="e.g. America/New_York"
              className="mt-1 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white" />
          </label>
          {err && <div className="text-red-500 text-xs">{err}</div>}
          <div className="flex gap-2 pt-1">
            <button onClick={save} disabled={saving}
              className="px-3 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
              {saving ? 'Saving...' : 'Save'}
            </button>
            <button onClick={() => setEditing(false)} className="px-3 py-1 text-xs text-gray-600 dark:text-gray-400 hover:text-gray-900">
              Cancel
            </button>
          </div>
        </div>
      ) : (
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">Timezone</span>
            <span className="text-gray-900 dark:text-white">{time!.timezone}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Local Time</span>
            <span className="text-gray-900 dark:text-white">{new Date(time!.localtime * 1000).toLocaleString()}</span>
          </div>
        </div>
      )}
    </div>
  );
}

function NodeHostsCard({ hosts, cluster, node, onSaved }: {
  hosts: string; cluster: string; node: string; onSaved: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');
  const [content, setContent] = useState('');

  const startEdit = () => {
    setContent(hosts);
    setErr('');
    setEditing(true);
  };

  const save = async () => {
    setSaving(true);
    setErr('');
    try {
      await api.updateNodeHosts(cluster, node, content, '');
      setEditing(false);
      onSaved();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="font-medium text-gray-900 dark:text-white">/etc/hosts</h3>
        {!editing && (
          <button onClick={startEdit} className="text-xs text-blue-600 hover:text-blue-700">Edit</button>
        )}
      </div>
      {editing ? (
        <div className="space-y-2">
          <textarea value={content} onChange={e => setContent(e.target.value)} rows={10}
            className="block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-3 py-2 text-sm font-mono text-gray-900 dark:text-white" />
          {err && <div className="text-red-500 text-xs">{err}</div>}
          <div className="flex gap-2">
            <button onClick={save} disabled={saving}
              className="px-3 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
              {saving ? 'Saving...' : 'Save'}
            </button>
            <button onClick={() => setEditing(false)} className="px-3 py-1 text-xs text-gray-600 dark:text-gray-400 hover:text-gray-900">
              Cancel
            </button>
          </div>
        </div>
      ) : (
        hosts ? (
          <pre className="text-sm font-mono bg-gray-50 dark:bg-gray-900 rounded p-3 overflow-x-auto text-gray-900 dark:text-gray-300 whitespace-pre">
            {hosts}
          </pre>
        ) : (
          <div className="text-sm text-gray-500">No hosts data available</div>
        )
      )}
    </div>
  );
}

function NodeNetworkCard({ network, cluster, node, onSaved }: {
  network: NetworkInterface[]; cluster: string; node: string; onSaved: () => void;
}) {
  const [editIface, setEditIface] = useState<NetworkInterface | null>(null);
  const [creating, setCreating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [applying, setApplying] = useState(false);
  const [err, setErr] = useState('');
  const [form, setForm] = useState<Record<string, string>>({});

  const IFACE_TYPES = ['bridge', 'bond', 'vlan', 'OVSBridge', 'OVSBond', 'OVSPort', 'OVSIntPort'];

  const openEdit = (iface: NetworkInterface) => {
    setEditIface(iface);
    setCreating(false);
    setForm({
      type: iface.type || '',
      address: iface.address || '',
      netmask: iface.netmask || '',
      gateway: iface.gateway || '',
      cidr: iface.cidr || '',
      bridge_ports: iface.bridge_ports || '',
      slaves: iface.slaves || '',
      bond_mode: iface.bond_mode || '',
      mtu: iface.mtu ? String(iface.mtu) : '',
      autostart: iface.autostart ? '1' : '0',
      comments: iface.comments || '',
      'vlan-raw-device': iface['vlan-raw-device'] || '',
    });
    setErr('');
  };

  const openCreate = () => {
    setEditIface(null);
    setCreating(true);
    setForm({ iface: '', type: 'bridge', address: '', netmask: '', gateway: '', bridge_ports: '', autostart: '1', comments: '' });
    setErr('');
  };

  const saveEdit = async () => {
    setSaving(true);
    setErr('');
    try {
      // Filter out empty values
      const params: Record<string, string> = {};
      for (const [k, v] of Object.entries(form)) {
        if (v !== '') params[k] = v;
      }
      if (creating) {
        await api.createNodeNetworkInterface(cluster, node, params);
      } else if (editIface) {
        await api.updateNodeNetworkInterface(cluster, node, editIface.iface, params);
      }
      setEditIface(null);
      setCreating(false);
      onSaved();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  const deleteIface = async (iface: string) => {
    if (!confirm(`Delete interface ${iface}? Changes won't take effect until applied.`)) return;
    try {
      await api.deleteNodeNetworkInterface(cluster, node, iface);
      onSaved();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Delete failed');
    }
  };

  const applyChanges = async () => {
    if (!confirm('Apply network changes? This will restart networking and may briefly interrupt connectivity.')) return;
    setApplying(true);
    try {
      await api.applyNodeNetwork(cluster, node);
      onSaved();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Apply failed');
    } finally {
      setApplying(false);
    }
  };

  const revertChanges = async () => {
    if (!confirm('Revert all pending network changes?')) return;
    try {
      await api.revertNodeNetwork(cluster, node);
      onSaved();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Revert failed');
    }
  };

  const isEditing = editIface !== null || creating;

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="font-medium text-gray-900 dark:text-white">Network Interfaces</h3>
        <div className="flex gap-2">
          <button onClick={revertChanges} className="text-xs text-yellow-600 hover:text-yellow-700">Revert</button>
          <button onClick={applyChanges} disabled={applying}
            className="text-xs text-green-600 hover:text-green-700 disabled:opacity-50">
            {applying ? 'Applying...' : 'Apply'}
          </button>
          <button onClick={openCreate} className="text-xs text-blue-600 hover:text-blue-700">+ Add</button>
        </div>
      </div>

      {err && <div className="text-red-500 text-xs mb-2">{err}</div>}

      {/* Edit/Create form */}
      {isEditing && (
        <div className="mb-4 p-3 border border-blue-200 dark:border-blue-800 rounded bg-blue-50/50 dark:bg-blue-900/20">
          <h4 className="text-sm font-medium mb-2 text-gray-900 dark:text-white">
            {creating ? 'Create Interface' : `Edit ${editIface!.iface}`}
          </h4>
          <div className="grid grid-cols-2 gap-2 text-sm">
            {creating && (
              <label className="block">
                <span className="text-gray-500 text-xs">Interface Name</span>
                <input value={form.iface || ''} onChange={e => setForm(f => ({ ...f, iface: e.target.value }))}
                  placeholder="e.g. vmbr1" className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
              </label>
            )}
            <label className="block">
              <span className="text-gray-500 text-xs">Type</span>
              <select value={form.type || ''} onChange={e => setForm(f => ({ ...f, type: e.target.value }))}
                className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white">
                {IFACE_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
              </select>
            </label>
            <label className="block">
              <span className="text-gray-500 text-xs">IPv4/CIDR</span>
              <input value={form.cidr || ''} onChange={e => setForm(f => ({ ...f, cidr: e.target.value }))}
                placeholder="e.g. 10.0.0.1/24" className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
            </label>
            <label className="block">
              <span className="text-gray-500 text-xs">Gateway</span>
              <input value={form.gateway || ''} onChange={e => setForm(f => ({ ...f, gateway: e.target.value }))}
                className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
            </label>
            <label className="block">
              <span className="text-gray-500 text-xs">Bridge Ports</span>
              <input value={form.bridge_ports || ''} onChange={e => setForm(f => ({ ...f, bridge_ports: e.target.value }))}
                placeholder="e.g. eno1" className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
            </label>
            <label className="block">
              <span className="text-gray-500 text-xs">Bond Slaves</span>
              <input value={form.slaves || ''} onChange={e => setForm(f => ({ ...f, slaves: e.target.value }))}
                className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
            </label>
            <label className="block">
              <span className="text-gray-500 text-xs">Bond Mode</span>
              <input value={form.bond_mode || ''} onChange={e => setForm(f => ({ ...f, bond_mode: e.target.value }))}
                className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white" />
            </label>
            <label className="block">
              <span className="text-gray-500 text-xs">VLAN Raw Device</span>
              <input value={form['vlan-raw-device'] || ''} onChange={e => setForm(f => ({ ...f, 'vlan-raw-device': e.target.value }))}
                className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
            </label>
            <label className="block">
              <span className="text-gray-500 text-xs">MTU</span>
              <input value={form.mtu || ''} onChange={e => setForm(f => ({ ...f, mtu: e.target.value }))}
                className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
            </label>
            <label className="flex items-center gap-2 text-sm pt-4">
              <input type="checkbox" checked={form.autostart === '1'} onChange={e => setForm(f => ({ ...f, autostart: e.target.checked ? '1' : '0' }))} />
              <span className="text-gray-500">Autostart</span>
            </label>
            <label className="block col-span-2">
              <span className="text-gray-500 text-xs">Comments</span>
              <input value={form.comments || ''} onChange={e => setForm(f => ({ ...f, comments: e.target.value }))}
                className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white" />
            </label>
          </div>
          <div className="flex gap-2 mt-3">
            <button onClick={saveEdit} disabled={saving}
              className="px-3 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
              {saving ? 'Saving...' : 'Save'}
            </button>
            <button onClick={() => { setEditIface(null); setCreating(false); }}
              className="px-3 py-1 text-xs text-gray-600 dark:text-gray-400 hover:text-gray-900">
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Interface table */}
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
              <th className="pb-2 pr-4">Interface</th>
              <th className="pb-2 pr-4">Type</th>
              <th className="pb-2 pr-4">CIDR / Address</th>
              <th className="pb-2 pr-4">Gateway</th>
              <th className="pb-2 pr-4">Ports / Slaves</th>
              <th className="pb-2 pr-4">Active</th>
              <th className="pb-2 pr-4">Comments</th>
              <th className="pb-2"></th>
            </tr>
          </thead>
          <tbody className="text-gray-900 dark:text-white">
            {[...network].sort((a, b) => a.iface.localeCompare(b.iface)).map((iface) => (
              <tr key={iface.iface} className="border-b border-gray-100 dark:border-gray-700/50 group">
                <td className="py-2 pr-4 font-mono">{iface.iface}</td>
                <td className="py-2 pr-4">{iface.type}</td>
                <td className="py-2 pr-4 font-mono">{iface.cidr || iface.address || '-'}</td>
                <td className="py-2 pr-4 font-mono">{iface.gateway || '-'}</td>
                <td className="py-2 pr-4 font-mono text-xs">{iface.bridge_ports || iface.slaves || '-'}</td>
                <td className="py-2 pr-4">
                  <span className={`inline-block w-2 h-2 rounded-full ${iface.active ? 'bg-green-500' : 'bg-gray-400'}`} />
                </td>
                <td className="py-2 pr-4 text-gray-500 text-xs">{iface.comments || ''}</td>
                <td className="py-2 text-right opacity-0 group-hover:opacity-100 transition-opacity">
                  <button onClick={() => openEdit(iface)} className="text-xs text-blue-600 hover:text-blue-700 mr-2">Edit</button>
                  <button onClick={() => deleteIface(iface.iface)} className="text-xs text-red-600 hover:text-red-700">Del</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function GuestSummary({ guest, tags, tagAssignments, onTagsChanged }: {
  guest: Guest;
  tags: Tag[];
  tagAssignments: TagAssignment[];
  onTagsChanged: () => void;
}) {
  const cpuPercent = (guest.cpu * 100).toFixed(1);
  const memPercentRaw = guest.maxmem > 0 ? (guest.mem / guest.maxmem) * 100 : 0;
  const memPercent = memPercentRaw.toFixed(1);

  const objectType = guest.type === 'qemu' ? 'vm' : 'ct';
  const objectId = String(guest.vmid);

  // Find assigned tags for this guest
  const assignedTagIds = tagAssignments
    .filter(a => a.object_type === objectType && a.object_id === objectId && a.cluster === guest.cluster)
    .map(a => a.tag_id);
  const assignedTags = tags.filter(t => assignedTagIds.includes(t.id));

  const handleAssign = async (tagId: string) => {
    try {
      await api.assignTag({ tag_id: tagId, object_type: objectType, object_id: objectId, cluster: guest.cluster });
      onTagsChanged();
    } catch { /* tag assign is best-effort — the UI will refresh and show the real state on next load */ }
  };

  const handleUnassign = async (tagId: string) => {
    try {
      await api.unassignTag({ tag_id: tagId, object_type: objectType, object_id: objectId, cluster: guest.cluster });
      onTagsChanged();
    } catch { /* tag unassign is best-effort — the UI will refresh and show the real state on next load */ }
  };

  return (
    <div className="space-y-4">
    <div className="grid md:grid-cols-2 gap-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Status</h3>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">Status</span>
            <span className={guest.status === 'running' ? 'text-green-600' : 'text-gray-600'}>
              {guest.status}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Type</span>
            <span className="text-gray-900 dark:text-white">{guest.type === 'qemu' ? 'Virtual Machine' : 'Container'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Node</span>
            <span className="text-gray-900 dark:text-white">{guest.node}</span>
          </div>
          {guest.status === 'running' && (
            <div className="flex justify-between">
              <span className="text-gray-500">Uptime</span>
              <span className="text-gray-900 dark:text-white">{formatUptime(guest.uptime)}</span>
            </div>
          )}
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Resources</h3>
        <div className="space-y-3">
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">CPU ({guest.cpus} cores)</span>
              <span className="text-gray-900 dark:text-white">{guest.status === 'running' ? `${cpuPercent}%` : '-'}</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className="h-full rounded bg-blue-500"
                style={{ width: guest.status === 'running' ? `${cpuPercent}%` : '0%' }}
              />
            </div>
          </div>
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">Memory ({formatBytes(guest.maxmem)})</span>
              <span className={`text-gray-900 dark:text-white ${memPercentRaw > 100 ? 'text-red-500 font-semibold' : ''}`}>{guest.status === 'running' ? `${memPercent}%` : '-'}</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className={`h-full rounded ${memPercentRaw > 90 ? 'bg-red-500' : memPercentRaw > 70 ? 'bg-yellow-500' : 'bg-green-500'}`}
                style={{ width: guest.status === 'running' ? `${Math.min(memPercentRaw, 100)}%` : '0%' }}
              />
            </div>
          </div>
        </div>
      </div>
    </div>

      {/* Tags */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Tags</h3>
        <TagPicker
          objectType={objectType}
          objectId={objectId}
          cluster={guest.cluster}
          tags={assignedTags}
          allTags={tags}
          onAssign={handleAssign}
          onUnassign={handleUnassign}
          onTagCreated={() => onTagsChanged()}
        />
      </div>
    </div>
  );
}

function StorageSummary({ storage }: { storage: Storage }) {
  const usedPercent = storage.total > 0 ? ((storage.used / storage.total) * 100).toFixed(1) : '0';

  return (
    <div className="grid md:grid-cols-2 gap-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Details</h3>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">Type</span>
            <span className="text-gray-900 dark:text-white">{storage.type}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Node</span>
            <span className="text-gray-900 dark:text-white">{storage.node}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Shared</span>
            <span className="text-gray-900 dark:text-white">{storage.shared ? 'Yes' : 'No'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Content</span>
            <span className="text-gray-900 dark:text-white">{storage.content}</span>
          </div>
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Capacity</h3>
        <div className="space-y-3">
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">Used</span>
              <span className="text-gray-900 dark:text-white">{formatBytes(storage.used)} / {formatBytes(storage.total)}</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className={`h-full rounded ${parseFloat(usedPercent) > 80 ? 'bg-red-500' : 'bg-blue-500'}`}
                style={{ width: `${usedPercent}%` }}
              />
            </div>
          </div>
          <div className="text-sm text-gray-500">
            {formatBytes(storage.avail)} available ({(100 - parseFloat(usedPercent)).toFixed(1)}%)
          </div>
        </div>
      </div>
    </div>
  );
}

function NodeVMs({ nodeId, typeFilter }: { nodeId: string; typeFilter?: 'qemu' | 'lxc' }) {
  const { guests, setSelectedObject, tags, tagAssignments } = useCluster();
  const nodeGuests = useMemo(
    () => guests
      .filter((g) => g.node === nodeId && (!typeFilter || g.type === typeFilter))
      .sort((a, b) => {
        // Running first, then by name
        if (a.status === 'running' && b.status !== 'running') return -1;
        if (a.status !== 'running' && b.status === 'running') return 1;
        return a.name.localeCompare(b.name);
      }),
    [guests, nodeId, typeFilter]
  );

  const handleClick = (g: typeof nodeGuests[0]) => {
    setSelectedObject({
      type: g.type === 'qemu' ? 'vm' : 'ct',
      id: g.vmid,
      name: g.name,
      node: g.node,
      cluster: g.cluster,
    });
  };

  if (nodeGuests.length === 0) {
    return (
      <div className="text-center py-8 text-gray-500 dark:text-gray-400">
        No virtual machines or containers on this node
      </div>
    );
  }

  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(110px,1fr))] gap-2">
      {nodeGuests.map((g) => {
        const isRunning = g.status === 'running';
        const isVM = g.type === 'qemu';
        const objType = isVM ? 'vm' : 'ct';
        const guestTags = tagAssignments
          .filter(a => a.object_type === objType && a.object_id === String(g.vmid) && a.cluster === g.cluster)
          .map(a => tags.find(t => t.id === a.tag_id))
          .filter(Boolean);

        return (
          <div
            key={g.vmid}
            onClick={() => handleClick(g)}
            className={`rounded-lg border-2 p-2 cursor-pointer transition-all hover:scale-105 hover:shadow-md relative
              ${isRunning
                ? 'border-green-500/40 bg-green-50 dark:bg-green-900/10'
                : 'border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-800/50 opacity-60'
              }`}
            title={`${g.name} (${g.vmid}) - ${g.status}`}
          >
            <div className="flex items-center gap-1 mb-1">
              <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${isRunning ? 'bg-green-500' : 'bg-gray-400'}`} />
              <span className="text-[10px] text-gray-400 dark:text-gray-500">{isVM ? 'VM' : 'CT'}</span>
            </div>
            <div className="font-medium text-xs text-gray-900 dark:text-white truncate leading-tight">
              {g.name}
            </div>
            <div className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">{g.vmid}</div>
            {guestTags.length > 0 && (
              <div className="absolute bottom-1.5 right-1.5 flex gap-0.5">
                {guestTags.map(t => (
                  <span key={t!.id} className="w-2 h-2 rounded-full" style={{ backgroundColor: t!.color }} title={`${t!.category}: ${t!.name}`} />
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

type SortField = 'name' | 'vmid' | 'type' | 'size' | 'format';
type SortDir = 'asc' | 'desc';

function StorageVMs({ storage }: { storage: Storage }) {
  const { guests, setSelectedObject } = useCluster();
  const [volumes, setVolumes] = useState<StorageVolume[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sortField, setSortField] = useState<SortField>('name');
  const [sortDir, setSortDir] = useState<SortDir>('asc');

  useEffect(() => {
    const fetchContent = async () => {
      setLoading(true);
      setError(null);
      try {
        const res = await fetch(`/api/storage/${storage.storage}/content?node=${storage.node}`);
        if (!res.ok) throw new Error('Failed to fetch storage content');
        const data = await res.json();
        setVolumes(data || []);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unknown error');
      } finally {
        setLoading(false);
      }
    };
    fetchContent();
  }, [storage.storage, storage.node]);

  // Build guest lookup map
  const guestMap = useMemo(() => {
    const map: Record<number, typeof guests[0]> = {};
    for (const g of guests) {
      map[g.vmid] = g;
    }
    return map;
  }, [guests]);

  // Extract disk name from volid
  const getDiskName = (volid: string) => {
    const parts = volid.split(':');
    return parts.length > 1 ? parts[1] : volid;
  };

  // Sort volumes
  const sortedVolumes = useMemo(() => {
    const sorted = [...volumes].sort((a, b) => {
      let cmp = 0;
      switch (sortField) {
        case 'name':
          cmp = getDiskName(a.volid).localeCompare(getDiskName(b.volid));
          break;
        case 'vmid':
          cmp = (a.vmid || 0) - (b.vmid || 0);
          break;
        case 'type':
          cmp = (a.content || '').localeCompare(b.content || '');
          break;
        case 'size':
          cmp = (a.size || 0) - (b.size || 0);
          break;
        case 'format':
          cmp = (a.format || '').localeCompare(b.format || '');
          break;
      }
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return sorted;
  }, [volumes, sortField, sortDir]);

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir(sortDir === 'asc' ? 'desc' : 'asc');
    } else {
      setSortField(field);
      setSortDir('asc');
    }
  };

  const SortHeader = ({ field, label, align }: { field: SortField; label: string; align?: 'right' }) => (
    <th
      className={`pb-2 font-medium cursor-pointer hover:text-gray-700 dark:hover:text-gray-200 select-none ${align === 'right' ? 'text-right' : ''}`}
      onClick={() => handleSort(field)}
    >
      {label}
      {sortField === field && (
        <span className="ml-1">{sortDir === 'asc' ? '▲' : '▼'}</span>
      )}
    </th>
  );

  const handleGuestClick = (vmid: number) => {
    const g = guestMap[vmid];
    if (g) {
      setSelectedObject({
        type: g.type === 'qemu' ? 'vm' : 'ct',
        id: g.vmid,
        name: g.name,
        node: g.node,
        cluster: g.cluster,
      });
    }
  };

  // Get content type icon
  const getContentIcon = (content: string, format: string) => {
    if (content === 'iso') return '💿';
    if (content === 'backup') return '📦';
    if (content === 'vztmpl') return '📋';
    if (content === 'snippets') return '📝';
    if (content === 'rootdir') return '🗂️';
    // For images, differentiate by format
    if (format === 'raw') return '🖴';
    if (format === 'qcow2') return '🖴';
    return '📄';
  };

  if (loading) {
    return (
      <div className="text-center py-8 text-gray-500 dark:text-gray-400">
        Loading...
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center py-8 text-red-500">
        Error: {error}
      </div>
    );
  }

  if (sortedVolumes.length === 0) {
    return (
      <div className="text-center py-8 text-gray-500 dark:text-gray-400">
        Empty
      </div>
    );
  }

  const totalSize = sortedVolumes.reduce((sum, v) => sum + (v.size || 0), 0);

  return (
    <div>
      <div className="flex items-center justify-between text-sm text-gray-500 dark:text-gray-400 mb-3">
        <span>{sortedVolumes.length} item{sortedVolumes.length !== 1 ? 's' : ''}</span>
        <span>{formatBytes(totalSize)}</span>
      </div>
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-gray-700">
            <SortHeader field="name" label="Name" />
            <SortHeader field="vmid" label="Owner" />
            <SortHeader field="type" label="Type" />
            <SortHeader field="size" label="Size" align="right" />
            <th className="w-4"></th>
            <SortHeader field="format" label="Format" />
          </tr>
        </thead>
        <tbody className="text-gray-700 dark:text-gray-300">
          {sortedVolumes.map((vol) => {
            const guest = vol.vmid ? guestMap[vol.vmid] : null;
            return (
              <tr
                key={vol.volid}
                className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50"
              >
                <td className="py-1.5">
                  <span className="mr-2">{getContentIcon(vol.content, vol.format)}</span>
                  <span className="font-mono text-xs">{getDiskName(vol.volid)}</span>
                </td>
                <td className="py-1.5">
                  {guest ? (
                    <button
                      onClick={() => handleGuestClick(vol.vmid!)}
                      className="hover:underline"
                    >
                      {guest.name}
                    </button>
                  ) : vol.vmid ? (
                    <span className="text-gray-400 dark:text-gray-500">VM {vol.vmid}</span>
                  ) : (
                    <span className="text-gray-400 dark:text-gray-500">—</span>
                  )}
                </td>
                <td className="py-1.5 text-gray-500 dark:text-gray-400">{vol.content}</td>
                <td className="py-1.5 text-right font-mono text-xs">{formatBytes(vol.size)}</td>
                <td></td>
                <td className="py-1.5 text-gray-500 dark:text-gray-400">{vol.format}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// Time range options for metrics charts
const TIME_RANGES: { value: TimeRange; label: string }[] = [
  { value: '1h', label: '1 Hour' },
  { value: '6h', label: '6 Hours' },
  { value: '24h', label: '24 Hours' },
  { value: '7d', label: '7 Days' },
  { value: '30d', label: '30 Days' },
];

// Stable metric arrays to prevent re-renders
const GUEST_CPU_MEM_METRICS = ['cpu', 'mem_percent'];
const GUEST_NET_METRICS = ['netin', 'netout'];
const GUEST_DISK_METRICS = ['diskread', 'diskwrite'];
const CT_SWAP_METRICS = ['swap_percent'];
const NODE_CPU_MEM_METRICS = ['cpu', 'mem_percent'];
const NODE_LOAD_METRICS = ['loadavg_1m', 'loadavg_5m', 'loadavg_15m'];

function GuestMonitorTab({
  vmid,
  type,
  isRunning,
}: {
  vmid: number;
  type: 'vm' | 'ct';
  isRunning: boolean;
}) {
  const [timeRange, setTimeRange] = useState<TimeRange>('1h');

  // Fetch all metrics in one request
  const { data, loading, error } = useMetrics({
    resourceType: type,
    resourceId: vmid.toString(),
    metrics: type === 'ct'
      ? [...GUEST_CPU_MEM_METRICS, ...GUEST_NET_METRICS, ...GUEST_DISK_METRICS, ...CT_SWAP_METRICS]
      : [...GUEST_CPU_MEM_METRICS, ...GUEST_NET_METRICS, ...GUEST_DISK_METRICS],
    timeRange,
    enabled: isRunning,
  });

  // Split series by metric type for separate charts
  const cpuSeries = useMemo(
    () => data?.series?.filter((s: MetricSeries) => s.metric === 'cpu') || [],
    [data?.series]
  );
  const memSeries = useMemo(
    () => data?.series?.filter((s: MetricSeries) => s.metric === 'mem_percent') || [],
    [data?.series]
  );
  const netSeries = useMemo(
    () => data?.series?.filter((s: MetricSeries) => s.metric === 'netin' || s.metric === 'netout') || [],
    [data?.series]
  );
  const diskSeries = useMemo(
    () => data?.series?.filter((s: MetricSeries) => s.metric === 'diskread' || s.metric === 'diskwrite') || [],
    [data?.series]
  );
  const swapSeries = useMemo(
    () => data?.series?.filter((s: MetricSeries) => s.metric === 'swap_percent') || [],
    [data?.series]
  );

  if (!isRunning) {
    return (
      <div className="text-gray-500 dark:text-gray-400 text-center py-8">
        {type === 'vm' ? 'VM' : 'Container'} is not running. Start it to view performance metrics.
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Time Range Selector */}
      <div className="flex justify-between items-center">
        <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300">
          Performance
        </h3>
        <div className="flex gap-1">
          {TIME_RANGES.map((tr) => (
            <button
              key={tr.value}
              onClick={() => setTimeRange(tr.value)}
              className={`px-3 py-1 text-xs rounded transition-colors ${
                timeRange === tr.value
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
              }`}
            >
              {tr.label}
            </button>
          ))}
        </div>
      </div>

      {error && (
        <div className="text-red-500 text-sm p-2 bg-red-100 dark:bg-red-900/20 rounded">
          {error}
        </div>
      )}

      {loading && !data && (
        <div className="text-gray-500 text-center py-8">Loading metrics...</div>
      )}

      {/* Charts Grid - vCenter style 2x2 layout */}
      <div className="grid md:grid-cols-2 gap-4">
        {/* CPU Chart */}
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <MetricsChart
            series={cpuSeries}
            timeRange={timeRange}
            title="CPU Usage"
            height={180}
            showLegend={false}
          />
        </div>

        {/* Memory Chart */}
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <MetricsChart
            series={memSeries}
            timeRange={timeRange}
            title="Memory Usage"
            height={180}
            showLegend={false}
          />
        </div>

        {/* Network I/O Chart */}
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <MetricsChart
            series={netSeries}
            timeRange={timeRange}
            title="Network I/O"
            height={180}
            showLegend={true}
          />
        </div>

        {/* Disk I/O Chart */}
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <MetricsChart
            series={diskSeries}
            timeRange={timeRange}
            title="Disk I/O"
            height={180}
            showLegend={true}
          />
        </div>

        {/* Swap Chart - containers only */}
        {type === 'ct' && swapSeries.length > 0 && (
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
            <MetricsChart
              series={swapSeries}
              timeRange={timeRange}
              title="Swap Usage"
              height={180}
              showLegend={false}
            />
          </div>
        )}
      </div>
    </div>
  );
}

// Helper to get time range start timestamp
function getStartTimestamp(range: TimeRange): number {
  const now = Math.floor(Date.now() / 1000);
  switch (range) {
    case '1h': return now - 3600;
    case '6h': return now - 6 * 3600;
    case '24h': return now - 24 * 3600;
    case '7d': return now - 7 * 24 * 3600;
    case '30d': return now - 30 * 24 * 3600;
    default: return now - 3600;
  }
}

function NodeMonitorTab({ node }: { node: string }) {
  const [timeRange, setTimeRange] = useState<TimeRange>('1h');
  const [selectedGuests, setSelectedGuests] = useState<Set<number>>(new Set());
  const [guestMetrics, setGuestMetrics] = useState<MetricSeries[]>([]);
  const [showGuestPicker, setShowGuestPicker] = useState(false);
  const { guests } = useCluster();

  // Get running guests on this node, sorted by name for stable ordering
  const nodeGuests = useMemo(
    () => guests
      .filter(g => g.node === node && g.status === 'running')
      .sort((a, b) => a.name.localeCompare(b.name)),
    [guests, node]
  );

  // Fetch node metrics
  const { data, loading, error } = useMetrics({
    resourceType: 'node',
    resourceId: node,
    metrics: [...NODE_CPU_MEM_METRICS, ...NODE_LOAD_METRICS],
    timeRange,
  });

  // Fetch metrics for selected guests
  /* eslint-disable react-hooks/set-state-in-effect --
     fetch-on-selection-change + clear-when-empty; no external system to subscribe to here */
  useEffect(() => {
    if (selectedGuests.size === 0) {
      setGuestMetrics([]);
      return;
    }

    const fetchGuestMetrics = async () => {
      const start = getStartTimestamp(timeRange);
      const end = Math.floor(Date.now() / 1000);
      const results: MetricSeries[] = [];

      await Promise.all(
        Array.from(selectedGuests).map(async (vmid) => {
          const guest = nodeGuests.find(g => g.vmid === vmid);
          if (!guest) return;

          const type = guest.type === 'qemu' ? 'vm' : 'ct';
          try {
            const res = await fetch(
              `/api/metrics/${type}/${vmid}?start=${start}&end=${end}&metrics=cpu&resolution=auto`
            );
            if (res.ok) {
              const data = await res.json();
              // Relabel series with guest name
              data.series?.forEach((s: MetricSeries) => {
                results.push({
                  ...s,
                  resource_id: guest.name,
                });
              });
            }
          } catch {
            // Ignore fetch errors for individual guests
          }
        })
      );

      setGuestMetrics(results);
    };

    fetchGuestMetrics();
    const interval = setInterval(fetchGuestMetrics, 30000);
    return () => clearInterval(interval);
  }, [selectedGuests, timeRange, nodeGuests]);
  /* eslint-enable react-hooks/set-state-in-effect */

  // Toggle guest selection
  const toggleGuest = (vmid: number) => {
    setSelectedGuests(prev => {
      const next = new Set(prev);
      if (next.has(vmid)) {
        next.delete(vmid);
      } else {
        next.add(vmid);
      }
      return next;
    });
  };

  // Combine node CPU with guest CPU series
  const nodeCpuSeries = useMemo(
    () => data?.series?.filter((s: MetricSeries) => s.metric === 'cpu').map(s => ({
      ...s,
      resource_id: `${node} (Host)`,
    })) || [],
    [data?.series, node]
  );

  const combinedCpuSeries = useMemo(
    () => [...nodeCpuSeries, ...guestMetrics],
    [nodeCpuSeries, guestMetrics]
  );

  const memSeries = useMemo(
    () => data?.series?.filter((s: MetricSeries) => s.metric === 'mem_percent') || [],
    [data?.series]
  );
  const loadSeries = useMemo(
    () => data?.series?.filter((s: MetricSeries) => s.metric.startsWith('loadavg')) || [],
    [data?.series]
  );

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300">
          Performance
        </h3>
        <div className="flex gap-1">
          {TIME_RANGES.map((tr) => (
            <button
              key={tr.value}
              onClick={() => setTimeRange(tr.value)}
              className={`px-3 py-1 text-xs rounded transition-colors ${
                timeRange === tr.value
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
              }`}
            >
              {tr.label}
            </button>
          ))}
        </div>
      </div>

      {error && (
        <div className="text-red-500 text-sm p-2 bg-red-100 dark:bg-red-900/20 rounded">
          {error}
        </div>
      )}

      {loading && !data && (
        <div className="text-gray-500 text-center py-8">Loading metrics...</div>
      )}

      <div className="grid md:grid-cols-2 gap-4">
        {/* CPU Chart with guest overlay option */}
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <div className="flex justify-between items-center mb-2">
            <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300">
              CPU Usage
            </h4>
            {nodeGuests.length > 0 && (
              <button
                onClick={() => setShowGuestPicker(!showGuestPicker)}
                className="text-xs text-blue-600 hover:text-blue-800 dark:text-blue-400"
              >
                {showGuestPicker ? 'Hide' : 'Overlay'} Guests ({selectedGuests.size})
              </button>
            )}
          </div>

          {/* Guest picker dropdown */}
          {showGuestPicker && (
            <div className="mb-3 p-2 bg-gray-50 dark:bg-gray-700 rounded text-xs max-h-40 overflow-y-auto">
              <div className="grid grid-cols-2 gap-1">
                {nodeGuests.map(g => (
                  <label
                    key={g.vmid}
                    className="flex items-center gap-1.5 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-600 p-1 rounded"
                  >
                    <input
                      type="checkbox"
                      checked={selectedGuests.has(g.vmid)}
                      onChange={() => toggleGuest(g.vmid)}
                      className="w-3 h-3"
                    />
                    <span className="truncate" title={g.name}>
                      {g.type === 'qemu' ? '💻' : '📦'} {g.name}
                    </span>
                  </label>
                ))}
              </div>
              {selectedGuests.size > 0 && (
                <button
                  onClick={() => setSelectedGuests(new Set())}
                  className="mt-2 text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
                >
                  Clear all
                </button>
              )}
            </div>
          )}

          <MetricsChart
            series={combinedCpuSeries}
            timeRange={timeRange}
            height={180}
            showLegend={combinedCpuSeries.length > 1}
          />
        </div>

        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <MetricsChart
            series={memSeries}
            timeRange={timeRange}
            title="Memory Usage"
            height={180}
            showLegend={false}
          />
        </div>

        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4 md:col-span-2">
          <MetricsChart
            series={loadSeries}
            timeRange={timeRange}
            title="Load Average"
            height={180}
            showLegend={true}
          />
        </div>
      </div>
    </div>
  );
}

type ConfigSubTab = 'hardware' | 'options' | 'network' | 'storage';

const CONFIG_SUB_TABS: { id: ConfigSubTab; label: string }[] = [
  { id: 'hardware', label: 'Hardware' },
  { id: 'options', label: 'Options' },
  { id: 'network', label: 'Network' },
  { id: 'storage', label: 'Storage' },
];

function ConfigureTab({
  vmid,
  type,
  cluster,
}: {
  vmid: number;
  type: 'vm' | 'ct';
  cluster: string;
}) {
  const [activeSubTab, setActiveSubTab] = useState<ConfigSubTab>('hardware');
  const [config, setConfig] = useState<VMConfig | ContainerConfig | null>(null);
  const [digest, setDigest] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState<string | null>(null);

  // Fetch config on mount
  useEffect(() => {
    setLoading(true);
    setFetchError(null);

    const fetchConfig = async () => {
      try {
        if (type === 'vm') {
          const res = await api.getVMConfig(cluster, vmid);
          setConfig(res.config);
          setDigest(res.digest);
        } else {
          const res = await api.getContainerConfig(cluster, vmid);
          setConfig(res.config);
          setDigest(res.digest);
        }
      } catch (err) {
        setFetchError(err instanceof Error ? err.message : 'Failed to load config');
      } finally {
        setLoading(false);
      }
    };

    fetchConfig();
  }, [vmid, type, cluster]);

  if (loading) {
    return (
      <div className="text-gray-500 dark:text-gray-400 text-center py-8">
        Loading configuration...
      </div>
    );
  }

  if (fetchError) {
    return (
      <div className="text-red-500 text-center py-8">
        Error: {fetchError}
      </div>
    );
  }

  if (!config) {
    return (
      <div className="text-gray-500 dark:text-gray-400 text-center py-8">
        No configuration data available
      </div>
    );
  }

  return (
    <ConfigureTabContent
      vmid={vmid}
      type={type}
      cluster={cluster}
      config={config}
      digest={digest}
      activeSubTab={activeSubTab}
      setActiveSubTab={setActiveSubTab}
    />
  );
}

// Separate component to use the hook after config is loaded
function ConfigureTabContent({
  vmid,
  type,
  cluster,
  config,
  digest,
  activeSubTab,
  setActiveSubTab,
}: {
  vmid: number;
  type: 'vm' | 'ct';
  cluster: string;
  config: VMConfig | ContainerConfig;
  digest: string;
  activeSubTab: ConfigSubTab;
  setActiveSubTab: (tab: ConfigSubTab) => void;
}) {
  const editor = useConfigEditor({
    vmid,
    type,
    cluster,
    initialConfig: config,
    initialDigest: digest,
  });

  return (
    <div className="space-y-4">
      {/* Sub-tabs */}
      <div className="flex gap-1 border-b border-gray-200 dark:border-gray-700 pb-2">
        {CONFIG_SUB_TABS.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveSubTab(tab.id)}
            className={`px-3 py-1.5 text-xs font-medium rounded-t transition-colors ${
              activeSubTab === tab.id
                ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 border border-b-0 border-gray-200 dark:border-gray-700'
                : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Sub-tab content */}
      {activeSubTab === 'hardware' && (
        <HardwareSection config={config} type={type} editor={editor} />
      )}
      {activeSubTab === 'options' && (
        <OptionsSection config={config} type={type} editor={editor} />
      )}
      {activeSubTab === 'network' && (
        <NetworkSection config={config} />
      )}
      {activeSubTab === 'storage' && (
        <StorageSection config={config} type={type} />
      )}

      {/* Pending Changes Panel */}
      {editor.isDirty && (
        <PendingChangesPanel editor={editor} />
      )}
    </div>
  );
}

function HardwareSection({
  config,
  type,
  editor,
}: {
  config: VMConfig | ContainerConfig;
  type: 'vm' | 'ct';
  editor: UseConfigEditorReturn;
}) {
  const isVM = type === 'vm';
  const vmConfig = isVM ? (config as VMConfig) : null;
  const ctConfig = !isVM ? (config as ContainerConfig) : null;

  return (
    <div className="grid md:grid-cols-2 gap-4">
      {/* CPU */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h4 className="font-medium mb-3 text-gray-900 dark:text-white flex items-center gap-2">
          <span>🔲</span> CPU
        </h4>
        <div className="space-y-2 text-sm">
          <EditableNumberRow
            label="Cores"
            configKey="cores"
            editor={editor}
            min={1}
            max={128}
          />
          {isVM && (
            <EditableNumberRow
              label="Sockets"
              configKey="sockets"
              editor={editor}
              min={1}
              max={4}
            />
          )}
          {vmConfig && (
            <ConfigRow label="Type" value={vmConfig.cpu || 'kvm64'} />
          )}
          {ctConfig && ctConfig.cpulimit && (
            <ConfigRow label="CPU Limit" value={ctConfig.cpulimit} />
          )}
        </div>
      </div>

      {/* Memory */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h4 className="font-medium mb-3 text-gray-900 dark:text-white flex items-center gap-2">
          <span>💾</span> Memory
        </h4>
        <div className="space-y-2 text-sm">
          <EditableNumberRow
            label="Memory (MB)"
            configKey="memory"
            editor={editor}
            min={16}
            max={1048576}
            step={128}
          />
          {ctConfig && (
            <EditableNumberRow
              label="Swap (MB)"
              configKey="swap"
              editor={editor}
              min={0}
              max={131072}
              step={128}
            />
          )}
          {vmConfig && vmConfig.balloon !== undefined && (
            <ConfigRow
              label="Ballooning"
              value={vmConfig.balloon === 0 ? 'Disabled' : `Min ${vmConfig.balloon} MB`}
            />
          )}
        </div>
      </div>

      {/* BIOS/Machine (VM only) */}
      {vmConfig && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h4 className="font-medium mb-3 text-gray-900 dark:text-white flex items-center gap-2">
            <span>⚙️</span> System
          </h4>
          <div className="space-y-2 text-sm">
            <ConfigRow label="BIOS" value={vmConfig.bios || 'SeaBIOS'} />
            <ConfigRow label="Machine" value={vmConfig.machine || 'i440fx'} />
            <ConfigRow label="OS Type" value={vmConfig.ostype || 'other'} />
          </div>
        </div>
      )}

      {/* Display (VM only) */}
      {vmConfig && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h4 className="font-medium mb-3 text-gray-900 dark:text-white flex items-center gap-2">
            <span>🖥️</span> Display
          </h4>
          <div className="space-y-2 text-sm">
            <ConfigRow label="VGA" value={vmConfig.vga || 'std'} />
          </div>
        </div>
      )}

      {/* Container Features */}
      {ctConfig && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h4 className="font-medium mb-3 text-gray-900 dark:text-white flex items-center gap-2">
            <span>📦</span> Container
          </h4>
          <div className="space-y-2 text-sm">
            <ConfigRow label="OS Type" value={ctConfig.ostype || 'unmanaged'} />
            <ConfigRow label="Arch" value={ctConfig.arch || 'amd64'} />
            <ConfigRow
              label="Unprivileged"
              value={ctConfig.unprivileged === 1 ? 'Yes' : 'No'}
            />
            {ctConfig.features && (
              <ConfigRow label="Features" value={ctConfig.features} />
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function OptionsSection({
  config,
  type,
  editor,
}: {
  config: VMConfig | ContainerConfig;
  type: 'vm' | 'ct';
  editor: UseConfigEditorReturn;
}) {
  const isVM = type === 'vm';
  const vmConfig = isVM ? (config as VMConfig) : null;
  const ctConfig = !isVM ? (config as ContainerConfig) : null;

  return (
    <div className="grid md:grid-cols-2 gap-4">
      {/* General */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h4 className="font-medium mb-3 text-gray-900 dark:text-white">General</h4>
        <div className="space-y-2 text-sm">
          <ConfigRow
            label="Name"
            value={vmConfig?.name || ctConfig?.hostname || `${type}`}
          />
          {config.description && (
            <div className="flex flex-col">
              <span className="text-gray-500 dark:text-gray-400">Description</span>
              <span className="text-gray-900 dark:text-white text-xs mt-1 whitespace-pre-wrap">
                {config.description}
              </span>
            </div>
          )}
        </div>
      </div>

      {/* Boot Options */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h4 className="font-medium mb-3 text-gray-900 dark:text-white">Boot Options</h4>
        <div className="space-y-2 text-sm">
          <EditableCheckboxRow
            label="Start at boot"
            configKey="onboot"
            editor={editor}
          />
          {vmConfig?.boot && <ConfigRow label="Boot order" value={vmConfig.boot} />}
          {vmConfig?.bootdisk && <ConfigRow label="Boot disk" value={vmConfig.bootdisk} />}
          {ctConfig?.startup && <ConfigRow label="Startup" value={ctConfig.startup} />}
        </div>
      </div>

      {/* Protection */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h4 className="font-medium mb-3 text-gray-900 dark:text-white">Protection</h4>
        <div className="space-y-2 text-sm">
          <EditableCheckboxRow
            label="Protection"
            configKey="protection"
            editor={editor}
          />
        </div>
      </div>

      {/* QEMU Guest Agent (VM only) */}
      {vmConfig && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h4 className="font-medium mb-3 text-gray-900 dark:text-white">Guest Agent</h4>
          <div className="space-y-2 text-sm">
            <ConfigRow
              label="QEMU Agent"
              value={vmConfig.agent?.includes('enabled=1') ? 'Enabled' : 'Disabled'}
            />
          </div>
        </div>
      )}

      {/* Cloud-init (VM only) */}
      {vmConfig && (vmConfig.ciuser || vmConfig.sshkeys || vmConfig.ipconfig0) && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4 md:col-span-2">
          <h4 className="font-medium mb-3 text-gray-900 dark:text-white">Cloud-init</h4>
          <div className="space-y-2 text-sm">
            {vmConfig.ciuser && <ConfigRow label="User" value={vmConfig.ciuser} />}
            {vmConfig.sshkeys && (
              <ConfigRow label="SSH Keys" value="(configured)" />
            )}
            {vmConfig.ipconfig0 && <ConfigRow label="IP Config" value={vmConfig.ipconfig0} />}
            {vmConfig.nameserver && <ConfigRow label="DNS" value={vmConfig.nameserver} />}
            {vmConfig.searchdomain && (
              <ConfigRow label="Search Domain" value={vmConfig.searchdomain} />
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function NetworkSection({
  config,
}: {
  config: VMConfig | ContainerConfig;
}) {
  // Extract network interfaces from raw_config
  const networks = useMemo(() => {
    const raw = config.raw_config || {};
    const nets: { key: string; value: string }[] = [];

    Object.entries(raw).forEach(([key, value]) => {
      if (key.startsWith('net') && typeof value === 'string') {
        nets.push({ key, value });
      }
    });

    return nets.sort((a, b) => a.key.localeCompare(b.key));
  }, [config.raw_config]);

  if (networks.length === 0) {
    return (
      <div className="text-gray-500 dark:text-gray-400 text-center py-8">
        No network interfaces configured
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {networks.map(({ key, value }) => (
        <div key={key} className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h4 className="font-medium mb-3 text-gray-900 dark:text-white flex items-center gap-2">
            <span>🔌</span> {key.toUpperCase()}
          </h4>
          <div className="text-sm text-gray-600 dark:text-gray-400 font-mono break-all">
            {value}
          </div>
        </div>
      ))}
    </div>
  );
}

function StorageSection({
  config,
  type,
}: {
  config: VMConfig | ContainerConfig;
  type: 'vm' | 'ct';
}) {
  const isVM = type === 'vm';
  const ctConfig = !isVM ? (config as ContainerConfig) : null;

  // Extract storage devices from raw_config
  const storage = useMemo(() => {
    const raw = config.raw_config || {};
    const disks: { key: string; value: string }[] = [];

    // VM disk types
    const vmDiskPrefixes = ['scsi', 'sata', 'ide', 'virtio', 'efidisk', 'tpmstate'];
    // CT mount point prefix
    const ctPrefix = 'mp';

    Object.entries(raw).forEach(([key, value]) => {
      if (typeof value !== 'string') return;

      if (isVM) {
        if (vmDiskPrefixes.some(p => key.startsWith(p))) {
          disks.push({ key, value });
        }
      } else {
        if (key.startsWith(ctPrefix) || key === 'rootfs') {
          disks.push({ key, value });
        }
      }
    });

    return disks.sort((a, b) => a.key.localeCompare(b.key));
  }, [config.raw_config, isVM]);

  // Add rootfs for containers. React Compiler can't infer preservation across
  // the `.find` + conditional spread, so the manual memoization stays —
  // behavior-equivalent, just not compiler-optimized.
  // eslint-disable-next-line react-hooks/preserve-manual-memoization
  const allStorage = useMemo(() => {
    if (ctConfig?.rootfs && !storage.find(s => s.key === 'rootfs')) {
      return [{ key: 'rootfs', value: ctConfig.rootfs }, ...storage];
    }
    return storage;
  }, [storage, ctConfig?.rootfs]);

  if (allStorage.length === 0) {
    return (
      <div className="text-gray-500 dark:text-gray-400 text-center py-8">
        No storage devices configured
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {allStorage.map(({ key, value }) => (
        <div key={key} className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h4 className="font-medium mb-3 text-gray-900 dark:text-white flex items-center gap-2">
            <span>💿</span> {key.toUpperCase()}
          </h4>
          <div className="text-sm text-gray-600 dark:text-gray-400 font-mono break-all">
            {value}
          </div>
        </div>
      ))}
    </div>
  );
}

// Helper component for config rows
function ConfigRow({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="flex justify-between">
      <span className="text-gray-500 dark:text-gray-400">{label}</span>
      <span className="text-gray-900 dark:text-white font-medium">{value}</span>
    </div>
  );
}

// Editable number input row
function EditableNumberRow({
  label,
  configKey,
  editor,
  min,
  max,
  step = 1,
}: {
  label: string;
  configKey: string;
  editor: UseConfigEditorReturn;
  min?: number;
  max?: number;
  step?: number;
}) {
  const value = editor.getValue(configKey);
  const numValue = typeof value === 'number' ? value : parseInt(String(value) || '0', 10);

  return (
    <div className="flex justify-between items-center">
      <span className="text-gray-500 dark:text-gray-400">{label}</span>
      <input
        type="number"
        value={numValue}
        min={min}
        max={max}
        step={step}
        onChange={(e) => editor.setValue(configKey, parseInt(e.target.value, 10), label)}
        className="w-24 px-2 py-1 text-sm text-right bg-gray-100 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded focus:outline-none focus:ring-1 focus:ring-blue-500 text-gray-900 dark:text-white"
      />
    </div>
  );
}

// Editable checkbox row (for boolean flags like onboot, protection)
function EditableCheckboxRow({
  label,
  configKey,
  editor,
}: {
  label: string;
  configKey: string;
  editor: UseConfigEditorReturn;
}) {
  const value = editor.getValue(configKey);
  const checked = value === 1 || value === '1' || value === true;

  return (
    <div className="flex justify-between items-center">
      <span className="text-gray-500 dark:text-gray-400">{label}</span>
      <label className="relative inline-flex items-center cursor-pointer">
        <input
          type="checkbox"
          checked={checked}
          onChange={(e) => editor.setValue(configKey, e.target.checked ? 1 : 0, label)}
          className="sr-only peer"
        />
        <div className="w-9 h-5 bg-gray-200 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-blue-300 dark:peer-focus:ring-blue-800 rounded-full peer dark:bg-gray-700 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all dark:border-gray-600 peer-checked:bg-blue-600"></div>
      </label>
    </div>
  );
}

// Pending changes panel with Apply/Discard buttons
function PendingChangesPanel({ editor }: { editor: UseConfigEditorReturn }) {
  return (
    <div className="fixed bottom-4 left-1/2 transform -translate-x-1/2 bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 p-4 max-w-lg w-full mx-4 z-50">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <h4 className="font-medium text-gray-900 dark:text-white text-sm mb-2">
            Pending Changes ({editor.pendingChanges.length})
          </h4>
          <div className="space-y-1 text-xs max-h-32 overflow-y-auto">
            {editor.pendingChanges.map((change) => (
              <div key={change.key} className="flex gap-2 text-gray-600 dark:text-gray-400">
                <span className="font-medium">{change.label}:</span>
                <span className="text-red-500 line-through">{String(change.oldValue ?? 'unset')}</span>
                <span>→</span>
                <span className="text-green-500">{String(change.newValue)}</span>
              </div>
            ))}
          </div>
          {editor.error && (
            <div className="mt-2 text-xs text-red-500">
              {editor.conflict ? '⚠️ ' : ''}
              {editor.error}
            </div>
          )}
        </div>
        <div className="flex gap-2 flex-shrink-0">
          <button
            onClick={editor.discard}
            disabled={editor.applying}
            className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white disabled:opacity-50"
          >
            Discard
          </button>
          <button
            onClick={editor.apply}
            disabled={editor.applying}
            className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 flex items-center gap-1"
          >
            {editor.applying ? (
              <>
                <span className="animate-spin">⏳</span>
                Applying...
              </>
            ) : (
              'Apply'
            )}
          </button>
        </div>
      </div>
    </div>
  );
}

// Console tab with inline VNC viewer
function ConsoleTab({
  vmid,
  type,
  name,
  isRunning,
}: {
  vmid: number;
  type: 'vm' | 'ct';
  name: string;
  isRunning: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFBInstance | null>(null);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'error' | 'closed'>('connecting');
  const [error, setError] = useState<string | null>(null);
  const [scaleMode, setScaleMode] = useState<'fit' | '1:1'>('fit');
  const [vncSize, setVncSize] = useState<{ width: number; height: number } | null>(null);

  useEffect(() => {
    if (!isRunning || !containerRef.current) return;

    let rfb: RFBInstance | null = null;
    let mounted = true;

    const connect = async () => {
      try {
        const ticketResp = await fetch(`/api/console/${type}/${vmid}/ticket`);
        if (!ticketResp.ok) {
          throw new Error(`Failed to get ticket: ${ticketResp.statusText}`);
        }
        const { ticket, port } = await ticketResp.json();

        if (!mounted || !containerRef.current) return;

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/api/console/${type}/${vmid}/ws?ticket=${encodeURIComponent(ticket)}&port=${port}`;

        const module = await import('@novnc/novnc/lib/rfb.js');
        if (!mounted || !containerRef.current) return;

        const RFB = module.default;

        rfb = new RFB(containerRef.current, wsUrl, {
          credentials: { password: ticket },
        });

        rfbRef.current = rfb;
        rfb.scaleViewport = scaleMode === 'fit';
        rfb.clipViewport = true;
        rfb.resizeSession = false;

        rfb.addEventListener('connect', () => {
          if (mounted) {
            setStatus('connected');
            // Capture VNC native size and apply scaling
            setTimeout(() => {
              if (rfbRef.current) {
                const screen = rfbRef.current._screen;
                if (screen) {
                  setVncSize({ width: screen.width, height: screen.height });
                }
                rfbRef.current.scaleViewport = scaleMode === 'fit';
              }
            }, 100);

            // Remap Backspace to send DEL (^?) instead of ^H
            const canvas = containerRef.current?.querySelector('canvas');
            if (canvas) {
              canvas.addEventListener('keydown', (e: KeyboardEvent) => {
                if (e.key === 'Backspace') {
                  e.preventDefault();
                  e.stopPropagation();
                  // Send Ctrl+? which produces DEL (ASCII 127)
                  rfb?.sendKey(0x7f, 'Backspace', true);
                  rfb?.sendKey(0x7f, 'Backspace', false);
                }
              }, true);
            }

            rfb?.focus();
          }
        });

        rfb.addEventListener('disconnect', (e: CustomEvent) => {
          if (mounted) {
            if (e.detail.clean) {
              setStatus('closed');
            } else {
              setStatus('error');
              setError('Connection lost');
            }
          }
        });

        rfb.addEventListener('securityfailure', (e: CustomEvent) => {
          if (mounted) {
            setStatus('error');
            setError(`Security error: ${e.detail.reason}`);
          }
        });
      } catch (err) {
        if (mounted) {
          setStatus('error');
          setError(err instanceof Error ? err.message : 'Failed to initialize VNC');
        }
      }
    };

    connect();

    // Handle container resize
    const resizeObserver = new ResizeObserver(() => {
      if (rfbRef.current) {
        rfbRef.current.scaleViewport = true;
      }
    });
    if (containerRef.current) {
      resizeObserver.observe(containerRef.current);
    }

    return () => {
      mounted = false;
      resizeObserver.disconnect();
      if (rfbRef.current) {
        rfbRef.current.disconnect();
        rfbRef.current = null;
      }
    };
    // scaleMode is intentionally omitted: the connection should not tear down
    // on scale toggles. A separate effect below picks up scaleMode changes
    // and updates rfb.scaleViewport in place.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [type, vmid, isRunning]);

  // Reset state when switching VMs
  useEffect(() => {
    setStatus('connecting');
    setError(null);
  }, [vmid]);

  // Update scale mode on existing connection
  useEffect(() => {
    if (rfbRef.current) {
      rfbRef.current.scaleViewport = scaleMode === 'fit';
    }
  }, [scaleMode]);

  if (!isRunning) {
    return (
      <div className="flex items-center justify-center h-full text-gray-500 dark:text-gray-400">
        <div className="text-center">
          <p className="text-lg mb-2">{type === 'vm' ? 'VM' : 'Container'} is not running</p>
          <p className="text-sm">Start the {type === 'vm' ? 'VM' : 'container'} to access the console</p>
        </div>
      </div>
    );
  }

  const handlePopout = () => {
    const url = `/console/${type}/${vmid}/${encodeURIComponent(name)}`;
    window.open(url, `console-${vmid}`, 'width=1024,height=768');
  };

  return (
    <div className="flex flex-col" style={{ height: 'calc(100vh - 220px)' }}>
      {/* Console toolbar */}
      <div className="flex items-center justify-between px-3 py-2 bg-gray-800 rounded-t-lg flex-shrink-0">
        <div className="flex items-center gap-3">
          <span className={`px-2 py-0.5 text-xs rounded ${
            status === 'connected' ? 'bg-green-600 text-white' :
            status === 'connecting' ? 'bg-yellow-600 text-white' :
            status === 'error' ? 'bg-red-600 text-white' :
            'bg-gray-600 text-white'
          }`}>
            {status}
          </span>
          <div className="flex gap-1 ml-2">
            <button
              onClick={() => setScaleMode('fit')}
              className={`px-2 py-0.5 text-xs rounded ${
                scaleMode === 'fit'
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
              }`}
              title="Scale to fit container"
            >
              Fit
            </button>
            <button
              onClick={() => setScaleMode('1:1')}
              className={`px-2 py-0.5 text-xs rounded ${
                scaleMode === '1:1'
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
              }`}
              title="Native resolution (1:1 pixels)"
            >
              1:1
            </button>
          </div>
        </div>
        <button
          onClick={handlePopout}
          className="text-gray-400 hover:text-white hover:bg-blue-600 px-2 py-1 rounded text-sm"
          title="Pop out to new window"
        >
          ⧉ Pop out
        </button>
      </div>

      {/* VNC container */}
      <div
        className={`flex-1 bg-black rounded-b-lg ${
          scaleMode === '1:1' ? 'overflow-auto flex items-start justify-center' : 'overflow-hidden'
        }`}
        onClick={() => rfbRef.current?.focus()}
      >
        {error ? (
          <div className="flex items-center justify-center h-full text-red-400">
            {error}
          </div>
        ) : (
          <div
            ref={containerRef}
            style={scaleMode === '1:1' && vncSize
              ? { width: vncSize.width, height: vncSize.height }
              : { width: '100%', height: '100%' }
            }
          />
        )}
      </div>
    </div>
  );
}

// Network interface summary
function NetworkSummary({ ifaceName, node, cluster }: { ifaceName: string; node: string; cluster: string }) {
  const [iface, setIface] = useState<NetworkInterface | null>(null);
  const [loading, setLoading] = useState(true);
  const [connectedGuests, setConnectedGuests] = useState<{ vmid: number; name: string; type: string }[]>([]);
  const { guests } = useCluster();

  useEffect(() => {
    async function fetchInterface() {
      setLoading(true);
      try {
        const interfaces = await api.getClusterNetworkInterfaces(cluster, node);
        const found = interfaces.find((i: NetworkInterface) => i.iface === ifaceName);
        setIface(found || null);
      } catch (err) {
        console.error('Failed to fetch interface:', err);
        setIface(null);
      } finally {
        setLoading(false);
      }
    }
    fetchInterface();
  }, [ifaceName, node, cluster]);

  // Find guests on this node that might use this interface
  useEffect(() => {
    if (!iface || iface.type !== 'bridge') return;
    const nodeGuests = guests.filter(g => g.node === node);
    setConnectedGuests(nodeGuests.map(g => ({
      vmid: g.vmid,
      name: g.name,
      type: g.type === 'qemu' ? 'VM' : 'CT',
    })));
  }, [iface, guests, node]);

  if (loading) {
    return <div className="text-gray-500 dark:text-gray-400 text-center py-8">Loading...</div>;
  }

  if (!iface) {
    return <div className="text-gray-500 dark:text-gray-400 text-center py-8">Interface not found</div>;
  }

  // Icon based on type
  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'bridge': return '🌉';
      case 'bond': return '🔗';
      case 'vlan': return '🏷️';
      case 'eth': return '🔌';
      case 'OVSBridge': return '🌐';
      default: return '📡';
    }
  };

  return (
    <div className="space-y-6">
      {/* Interface Info Card */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center gap-2">
          <span>{getTypeIcon(iface.type)}</span>
          Interface Details
        </h3>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <span className="text-gray-500 dark:text-gray-400 text-sm">Name</span>
            <div className="text-gray-900 dark:text-white font-medium">{iface.iface}</div>
          </div>
          <div>
            <span className="text-gray-500 dark:text-gray-400 text-sm">Type</span>
            <div className="text-gray-900 dark:text-white font-medium">{iface.type}</div>
          </div>
          <div>
            <span className="text-gray-500 dark:text-gray-400 text-sm">Status</span>
            <div className={`font-medium ${iface.active === 1 ? 'text-green-600' : 'text-gray-500'}`}>
              {iface.active === 1 ? 'Active' : 'Inactive'}
            </div>
          </div>
          <div>
            <span className="text-gray-500 dark:text-gray-400 text-sm">Autostart</span>
            <div className="text-gray-900 dark:text-white font-medium">
              {iface.autostart === 1 ? 'Yes' : 'No'}
            </div>
          </div>
        </div>
      </div>

      {/* IPv4 Configuration */}
      {(iface.address || iface.cidr || iface.gateway) && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
            IPv4 Configuration
          </h3>
          <div className="grid grid-cols-2 gap-4">
            {iface.method && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">Method</span>
                <div className="text-gray-900 dark:text-white font-medium">{iface.method}</div>
              </div>
            )}
            {(iface.address || iface.cidr) && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">Address</span>
                <div className="text-gray-900 dark:text-white font-medium font-mono">
                  {iface.cidr || `${iface.address}/${iface.netmask}`}
                </div>
              </div>
            )}
            {iface.gateway && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">Gateway</span>
                <div className="text-gray-900 dark:text-white font-medium font-mono">{iface.gateway}</div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* IPv6 Configuration */}
      {(iface.address6 || iface.gateway6) && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
            IPv6 Configuration
          </h3>
          <div className="grid grid-cols-2 gap-4">
            {iface.method6 && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">Method</span>
                <div className="text-gray-900 dark:text-white font-medium">{iface.method6}</div>
              </div>
            )}
            {iface.address6 && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">Address</span>
                <div className="text-gray-900 dark:text-white font-medium font-mono">
                  {iface.address6}/{iface.netmask6}
                </div>
              </div>
            )}
            {iface.gateway6 && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">Gateway</span>
                <div className="text-gray-900 dark:text-white font-medium font-mono">{iface.gateway6}</div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Bridge Details */}
      {iface.type === 'bridge' && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
            Bridge Configuration
          </h3>
          <div className="grid grid-cols-2 gap-4">
            {iface.bridge_ports && (
              <div className="col-span-2">
                <span className="text-gray-500 dark:text-gray-400 text-sm">Bridge Ports</span>
                <div className="text-gray-900 dark:text-white font-medium font-mono">{iface.bridge_ports}</div>
              </div>
            )}
            {iface.bridge_stp && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">STP</span>
                <div className="text-gray-900 dark:text-white font-medium">{iface.bridge_stp}</div>
              </div>
            )}
            {iface.bridge_fd && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">Forward Delay</span>
                <div className="text-gray-900 dark:text-white font-medium">{iface.bridge_fd}</div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Bond Details */}
      {iface.type === 'bond' && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
            Bond Configuration
          </h3>
          <div className="grid grid-cols-2 gap-4">
            {iface.slaves && (
              <div className="col-span-2">
                <span className="text-gray-500 dark:text-gray-400 text-sm">Slave Interfaces</span>
                <div className="text-gray-900 dark:text-white font-medium font-mono">{iface.slaves}</div>
              </div>
            )}
            {iface.bond_mode && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">Bond Mode</span>
                <div className="text-gray-900 dark:text-white font-medium">{iface.bond_mode}</div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* VLAN Details */}
      {iface.type === 'vlan' && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
            VLAN Configuration
          </h3>
          <div className="grid grid-cols-2 gap-4">
            {iface['vlan-raw-device'] && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">Parent Interface</span>
                <div className="text-gray-900 dark:text-white font-medium">{iface['vlan-raw-device']}</div>
              </div>
            )}
            {iface['vlan-id'] && (
              <div>
                <span className="text-gray-500 dark:text-gray-400 text-sm">VLAN ID</span>
                <div className="text-gray-900 dark:text-white font-medium">{String(iface['vlan-id'])}</div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Connected Guests (for bridges) */}
      {iface.type === 'bridge' && connectedGuests.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
            Guests on this Node ({connectedGuests.length})
          </h3>
          <div className="text-xs text-gray-500 dark:text-gray-400 mb-2">
            Note: Shows all guests on this node. Actual bridge connections depend on VM/CT network config.
          </div>
          <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
            {connectedGuests.slice(0, 12).map(g => (
              <div key={g.vmid} className="flex items-center gap-2 text-sm">
                <span>{g.type === 'VM' ? '💻' : '📦'}</span>
                <span className="text-gray-900 dark:text-white">{g.vmid}</span>
                <span className="text-gray-500 dark:text-gray-400 truncate">{g.name}</span>
              </div>
            ))}
            {connectedGuests.length > 12 && (
              <div className="text-gray-500 dark:text-gray-400 text-sm">
                +{connectedGuests.length - 12} more...
              </div>
            )}
          </div>
        </div>
      )}

      {/* MTU if set */}
      {iface.mtu && iface.mtu > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">
            Additional Settings
          </h3>
          <div>
            <span className="text-gray-500 dark:text-gray-400 text-sm">MTU</span>
            <div className="text-gray-900 dark:text-white font-medium">{iface.mtu}</div>
          </div>
        </div>
      )}

      {/* Comments */}
      {iface.comments && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">
            Comments
          </h3>
          <div className="text-gray-600 dark:text-gray-400 whitespace-pre-wrap">{iface.comments}</div>
        </div>
      )}
    </div>
  );
}
