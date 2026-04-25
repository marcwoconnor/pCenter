import { useState, useEffect, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Layout } from '../components/Layout';
import { useCluster } from '../context/ClusterContext';
import { api, formatBytes } from '../api/client';
import type {
  CephCluster,
  CephOSD,
  CephPool,
  CephMON,
  CephMGR,
  CephMDS,
  CephFlags,
  CephInstallPreflightResponse,
  CephJobSnapshot,
} from '../types';

type TabKey = 'status' | 'osds' | 'pools' | 'monitors' | 'cephfs' | 'crush' | 'flags';

const TABS: { key: TabKey; label: string }[] = [
  { key: 'status', label: 'Status' },
  { key: 'osds', label: 'OSDs' },
  { key: 'pools', label: 'Pools' },
  { key: 'monitors', label: 'Monitors' },
  { key: 'cephfs', label: 'CephFS' },
  { key: 'crush', label: 'CRUSH' },
  { key: 'flags', label: 'Flags' },
];

const POLL_INTERVAL_MS = 30_000;

export function CephPage() {
  const { clusters } = useCluster();
  const [searchParams, setSearchParams] = useSearchParams();

  // Cluster + tab driven by URL so links are deep-linkable.
  const clusterFromURL = searchParams.get('cluster') || '';
  const tabFromURL = (searchParams.get('tab') as TabKey) || 'status';
  const activeCluster = clusterFromURL || clusters[0]?.name || '';
  const activeTab: TabKey = TABS.some((t) => t.key === tabFromURL) ? tabFromURL : 'status';

  const [topology, setTopology] = useState<CephCluster | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchTopology = useCallback(async () => {
    if (!activeCluster) {
      setTopology(null);
      return;
    }
    setLoading(true);
    try {
      const data = await api.getClusterCeph(activeCluster);
      setTopology(data);
      setError(null);
    } catch (err) {
      setTopology(null);
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [activeCluster]);

  useEffect(() => {
    fetchTopology();
    const id = setInterval(fetchTopology, POLL_INTERVAL_MS);
    return () => clearInterval(id);
  }, [fetchTopology]);

  const setTab = (tab: TabKey) => {
    const next = new URLSearchParams(searchParams);
    next.set('tab', tab);
    if (activeCluster) next.set('cluster', activeCluster);
    setSearchParams(next);
  };

  const setCluster = (name: string) => {
    const next = new URLSearchParams(searchParams);
    next.set('cluster', name);
    setSearchParams(next);
  };

  return (
    <Layout>
      <div className="flex-1 overflow-auto bg-gray-50 dark:bg-gray-900">
        {/* Header: cluster picker */}
        <div className="border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-6 py-3">
          <div className="flex items-center gap-4">
            <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Ceph</h1>
            <label className="text-sm text-gray-600 dark:text-gray-400">
              Cluster:&nbsp;
              <select
                value={activeCluster}
                onChange={(e) => setCluster(e.target.value)}
                className="border border-gray-300 dark:border-gray-600 rounded px-2 py-1 text-sm bg-white dark:bg-gray-700 dark:text-gray-100"
                disabled={clusters.length === 0}
              >
                {clusters.length === 0 ? (
                  <option value="">No clusters</option>
                ) : (
                  clusters.map((c) => (
                    <option key={c.name} value={c.name}>
                      {c.name}
                    </option>
                  ))
                )}
              </select>
            </label>
            {topology?.last_updated && (
              <span className="text-xs text-gray-500 dark:text-gray-400">
                Last poll: {new Date(topology.last_updated).toLocaleTimeString()}
              </span>
            )}
            <button
              onClick={fetchTopology}
              disabled={loading || !activeCluster}
              className="ml-auto text-sm text-blue-600 hover:underline disabled:text-gray-400 disabled:no-underline"
            >
              {loading ? 'Refreshing…' : 'Refresh'}
            </button>
          </div>
        </div>

        {/* Tab bar */}
        <div className="border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
          <div className="flex gap-4 px-6">
            {TABS.map((t) => (
              <button
                key={t.key}
                onClick={() => setTab(t.key)}
                className={`py-3 px-1 text-sm font-medium border-b-2 ${
                  activeTab === t.key
                    ? 'border-blue-500 text-blue-500'
                    : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
                }`}
              >
                {t.label}
              </button>
            ))}
          </div>
        </div>

        {/* Content */}
        <div className="p-6">
          {!activeCluster ? (
            <EmptyState message="No cluster selected." />
          ) : error ? (
            <NotInstalled cluster={activeCluster} message={error} onInstalled={fetchTopology} />
          ) : loading && !topology ? (
            <div className="text-gray-500 dark:text-gray-400">Loading Ceph topology…</div>
          ) : !topology ? (
            <EmptyState message="No Ceph data available yet." />
          ) : (
            <>
              {activeTab === 'status' && <StatusTab topology={topology} />}
              {activeTab === 'osds' && (
                <OSDsTab cluster={activeCluster} topology={topology} onRefresh={fetchTopology} />
              )}
              {activeTab === 'pools' && (
                <PoolsTab cluster={activeCluster} topology={topology} onRefresh={fetchTopology} />
              )}
              {activeTab === 'monitors' && (
                <MonitorsTab cluster={activeCluster} topology={topology} onRefresh={fetchTopology} />
              )}
              {activeTab === 'cephfs' && (
                <CephFSTab cluster={activeCluster} topology={topology} onRefresh={fetchTopology} />
              )}
              {activeTab === 'crush' && <CrushTab cluster={activeCluster} topology={topology} />}
              {activeTab === 'flags' && (
                <FlagsTab cluster={activeCluster} topology={topology} onRefresh={fetchTopology} />
              )}
            </>
          )}
        </div>
      </div>
    </Layout>
  );
}

// --- Helpers ---

function EmptyState({ message }: { message: string }) {
  return <div className="text-gray-500 dark:text-gray-400">{message}</div>;
}

function NotInstalled({ cluster, message, onInstalled }: { cluster: string; message: string; onInstalled: () => void }) {
  const [showWizard, setShowWizard] = useState(false);
  // 404 from /ceph means Ceph isn't installed (or hasn't been polled). Surface
  // it as a friendly empty-state rather than a red error.
  const isNotInstalled = message.includes('404') || message.includes('Ceph not installed');
  if (isNotInstalled) {
    return (
      <>
        <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-8 text-center">
          <p className="text-gray-700 dark:text-gray-200 font-medium mb-2">
            Ceph is not installed on cluster <code className="font-mono">{cluster}</code>.
          </p>
          <p className="text-sm text-gray-500 dark:text-gray-400 mb-4">
            Run the install wizard to bootstrap Ceph (pveceph install + init + first MON + first MGR).
            Once it succeeds, OSDs and pools can be added from this page.
          </p>
          <button
            onClick={() => setShowWizard(true)}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 hover:bg-blue-700 text-white"
          >
            Install Ceph…
          </button>
        </div>
        {showWizard && (
          <CephInstallWizard
            cluster={cluster}
            onClose={() => setShowWizard(false)}
            onSuccess={() => { setShowWizard(false); onInstalled(); }}
          />
        )}
      </>
    );
  }
  return <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{message}</div>;
}

// --- Install wizard ---

type WizardStep = 'configure' | 'preflight' | 'running';

function CephInstallWizard({ cluster, onClose, onSuccess }: { cluster: string; onClose: () => void; onSuccess: () => void }) {
  const { getNodesByCluster } = useCluster();
  const clusterNodes = getNodesByCluster(cluster);
  const [step, setStep] = useState<WizardStep>('configure');
  const [selectedNodes, setSelectedNodes] = useState<string[]>(() => clusterNodes.map((n) => n.node));
  const [network, setNetwork] = useState('');
  const [clusterNetwork, setClusterNetwork] = useState('');
  const [poolSize, setPoolSize] = useState('3');
  const [minSize, setMinSize] = useState('2');
  const [preflight, setPreflight] = useState<CephInstallPreflightResponse | null>(null);
  const [jobId, setJobId] = useState<string | null>(null);
  const [job, setJob] = useState<CephJobSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const toggleNode = (name: string) => {
    setSelectedNodes((cur) => (cur.includes(name) ? cur.filter((n) => n !== name) : [...cur, name]));
  };

  const runPreflight = async () => {
    if (selectedNodes.length === 0) {
      setError('Pick at least one node.');
      return;
    }
    if (!network) {
      setError('Network CIDR is required (e.g. 10.0.0.0/24).');
      return;
    }
    setError(null);
    setBusy(true);
    try {
      const result = await api.preflightCephInstall(cluster, { nodes: selectedNodes });
      setPreflight(result);
      setStep('preflight');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const { job_id } = await api.startCephInstall(cluster, {
        nodes: selectedNodes,
        network,
        cluster_network: clusterNetwork || undefined,
        pool_size: Number(poolSize) || undefined,
        min_size: Number(minSize) || undefined,
      });
      setJobId(job_id);
      setStep('running');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  };

  // Poll the job once we have its ID.
  useEffect(() => {
    if (!jobId) return;
    let cancelled = false;
    const tick = async () => {
      try {
        const snap = await api.getCephJob(cluster, jobId);
        if (cancelled) return;
        setJob(snap);
        if (snap.state !== 'running') {
          // Stop polling on terminal state; user dismisses dialog manually.
          return;
        }
        setTimeout(tick, 2000);
      } catch (err) {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : String(err));
      }
    };
    tick();
    return () => { cancelled = true; };
  }, [cluster, jobId]);

  // Detect terminal success / failure to update parent.
  const isTerminal = job?.state === 'succeeded' || job?.state === 'failed';
  const handleClose = () => {
    if (job?.state === 'succeeded') onSuccess();
    else onClose();
  };

  return (
    <Modal title={`Install Ceph on ${cluster}`} onClose={busy && !isTerminal ? () => {} : handleClose}>
      {step === 'configure' && (
        <div className="space-y-4">
          <Field label="Nodes (founder is the first selected)">
            <div className="border border-gray-300 dark:border-gray-600 rounded p-2 max-h-40 overflow-y-auto">
              {clusterNodes.length === 0 ? (
                <p className="text-sm text-gray-500">No nodes discovered for this cluster.</p>
              ) : (
                clusterNodes.map((n) => (
                  <label key={n.node} className="flex items-center gap-2 text-sm py-0.5">
                    <input
                      type="checkbox"
                      checked={selectedNodes.includes(n.node)}
                      onChange={() => toggleNode(n.node)}
                    />
                    <span className="font-mono">{n.node}</span>
                    <span className={`text-xs ${n.status === 'online' ? 'text-green-600' : 'text-red-600'}`}>{n.status}</span>
                  </label>
                ))
              )}
            </div>
          </Field>
          <Field label="Public network CIDR">
            <input value={network} onChange={(e) => setNetwork(e.target.value)} className={inputCls} placeholder="10.0.0.0/24" />
          </Field>
          <Field label="Cluster network CIDR (optional, for OSD replication)">
            <input value={clusterNetwork} onChange={(e) => setClusterNetwork(e.target.value)} className={inputCls} placeholder="10.10.0.0/24" />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Default pool size"><input type="number" min={1} value={poolSize} onChange={(e) => setPoolSize(e.target.value)} className={inputCls} /></Field>
            <Field label="Default min_size"><input type="number" min={1} value={minSize} onChange={(e) => setMinSize(e.target.value)} className={inputCls} /></Field>
          </div>
          {error && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{error}</div>}
          <DialogButtons onCancel={onClose} onSubmit={runPreflight} submitLabel="Run preflight" submitting={busy} />
        </div>
      )}

      {step === 'preflight' && preflight && (
        <div className="space-y-4">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-900 text-xs uppercase text-gray-500 dark:text-gray-400">
              <tr>
                <th className="text-left px-2 py-1">Node</th>
                <th className="text-left px-2 py-1">PVE</th>
                <th className="text-left px-2 py-1">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {preflight.hosts.map((h) => (
                <tr key={h.node}>
                  <td className="px-2 py-1 font-mono">{h.node}</td>
                  <td className="px-2 py-1">{h.pve_version || '—'}</td>
                  <td className="px-2 py-1">
                    {h.blockers.length === 0 ? (
                      <span className="text-green-600">ready</span>
                    ) : (
                      <span className="text-red-600">{h.blockers.join('; ')}</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {preflight.message && (
            <div className="bg-yellow-50 text-yellow-800 px-3 py-2 rounded text-sm">{preflight.message}</div>
          )}
          {error && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{error}</div>}
          <div className="flex justify-between gap-2">
            <button onClick={() => setStep('configure')} disabled={busy} className="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200">
              Back
            </button>
            <div className="flex gap-2">
              <button onClick={onClose} disabled={busy} className="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200">Cancel</button>
              <button
                onClick={submit}
                disabled={busy || !preflight.can_proceed}
                title={preflight.can_proceed ? 'Start install' : 'Resolve preflight blockers first'}
                className="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-700 text-white disabled:bg-gray-400"
              >
                {busy ? 'Working…' : 'Start install'}
              </button>
            </div>
          </div>
        </div>
      )}

      {step === 'running' && (
        <div className="space-y-3">
          <div className="text-sm">
            Job state:{' '}
            <span className={
              job?.state === 'succeeded' ? 'text-green-600 font-medium' :
              job?.state === 'failed' ? 'text-red-600 font-medium' :
              'text-blue-600 font-medium'
            }>
              {job?.state || 'starting…'}
            </span>
          </div>
          {job?.error && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{job.error}</div>}
          <div className="border border-gray-200 dark:border-gray-700 rounded max-h-80 overflow-y-auto">
            <table className="w-full text-xs">
              <thead className="bg-gray-50 dark:bg-gray-900 text-gray-500 dark:text-gray-400">
                <tr>
                  <th className="text-left px-2 py-1">Phase</th>
                  <th className="text-left px-2 py-1">Host</th>
                  <th className="text-left px-2 py-1">State</th>
                  <th className="text-left px-2 py-1">Detail</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {(job?.steps || []).map((s, i) => (
                  <tr key={i}>
                    <td className="px-2 py-1 font-mono">{s.phase}</td>
                    <td className="px-2 py-1 font-mono">{s.host || '—'}</td>
                    <td className="px-2 py-1">
                      <span className={
                        s.state === 'succeeded' ? 'text-green-600' :
                        s.state === 'failed' ? 'text-red-600' :
                        s.state === 'running' ? 'text-blue-600' :
                        'text-gray-500'
                      }>{s.state}</span>
                    </td>
                    <td className="px-2 py-1 text-gray-600 dark:text-gray-400">{s.error || s.message || ''}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {error && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{error}</div>}
          <div className="flex justify-end">
            <button
              onClick={handleClose}
              disabled={!isTerminal && job !== null && job.state === 'running'}
              className="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-700 text-white disabled:bg-gray-400"
            >
              {isTerminal ? (job?.state === 'succeeded' ? 'Done' : 'Close') : 'Running…'}
            </button>
          </div>
        </div>
      )}
    </Modal>
  );
}

function HealthBadge({ status }: { status?: string }) {
  if (!status) return <span className="text-gray-500">—</span>;
  const color =
    status === 'HEALTH_OK'
      ? 'bg-green-100 text-green-800'
      : status === 'HEALTH_WARN'
      ? 'bg-yellow-100 text-yellow-800'
      : 'bg-red-100 text-red-800';
  return <span className={`px-2 py-0.5 rounded text-xs font-medium ${color}`}>{status}</span>;
}

function StatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
      <div className="text-xs uppercase text-gray-500 dark:text-gray-400">{label}</div>
      <div className="text-xl font-semibold text-gray-900 dark:text-gray-100 mt-1">{value}</div>
      {sub && <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">{sub}</div>}
    </div>
  );
}

// --- Tabs ---

function StatusTab({ topology }: { topology: CephCluster }) {
  const status = topology.status;
  const pgmap = status?.pgmap;
  const pct =
    pgmap && pgmap.bytes_total > 0 ? (pgmap.bytes_used / pgmap.bytes_total) * 100 : 0;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
        <StatCard label="Health" value={status?.health.status || 'UNKNOWN'} />
        <StatCard
          label="Used"
          value={pgmap ? formatBytes(pgmap.bytes_used) : '—'}
          sub={pgmap ? `${pct.toFixed(1)}% of ${formatBytes(pgmap.bytes_total)}` : undefined}
        />
        <StatCard label="OSDs" value={String(topology.osds.length)} sub={`${topology.osds.filter((o) => o.status === 'up' && o.in).length} up & in`} />
        <StatCard
          label="Pools"
          value={String(topology.pools.length)}
          sub={`${topology.mons.length} MONs · ${topology.mgrs.length} MGRs`}
        />
      </div>

      {status?.health.checks && Object.keys(status.health.checks).length > 0 && (
        <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
          <div className="px-4 py-2 border-b border-gray-200 dark:border-gray-700 text-sm font-medium text-gray-700 dark:text-gray-200">
            Health checks
          </div>
          <ul className="divide-y divide-gray-200 dark:divide-gray-700">
            {Object.entries(status.health.checks).map(([code, check]) => (
              <li key={code} className="px-4 py-3">
                <div className="flex items-center gap-2">
                  <HealthBadge status={check.severity} />
                  <span className="font-mono text-xs text-gray-600 dark:text-gray-400">{code}</span>
                  <span className="text-sm text-gray-900 dark:text-gray-100">
                    {check.summary.message}
                  </span>
                </div>
                {check.detail && check.detail.length > 0 && (
                  <ul className="mt-2 ml-6 text-xs text-gray-600 dark:text-gray-400 space-y-1">
                    {check.detail.slice(0, 5).map((d, i) => (
                      <li key={i} className="font-mono">{d.message}</li>
                    ))}
                    {check.detail.length > 5 && (
                      <li className="italic">… and {check.detail.length - 5} more</li>
                    )}
                  </ul>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function OSDsTab({ cluster, topology, onRefresh }: { cluster: string; topology: CephCluster; onRefresh: () => void }) {
  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const groups = groupByHost(topology.osds);

  const act = async (label: string, fn: () => Promise<unknown>) => {
    setBusy(label);
    setActionError(null);
    try {
      await fn();
      await onRefresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-4">
      {actionError && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{actionError}</div>}

      {groups.length === 0 ? (
        <EmptyState message="No OSDs." />
      ) : (
        groups.map(([host, osds]) => (
          <div key={host} className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
            <div className="px-4 py-2 border-b border-gray-200 dark:border-gray-700 text-sm font-medium text-gray-700 dark:text-gray-200">
              {host} <span className="text-gray-500 dark:text-gray-400 font-normal">· {osds.length} OSD{osds.length === 1 ? '' : 's'}</span>
            </div>
            <table className="w-full text-sm">
              <thead className="bg-gray-50 dark:bg-gray-900 text-xs uppercase text-gray-500 dark:text-gray-400">
                <tr>
                  <th className="text-left px-4 py-2">ID</th>
                  <th className="text-left px-4 py-2">Status</th>
                  <th className="text-left px-4 py-2">In</th>
                  <th className="text-left px-4 py-2">Class</th>
                  <th className="text-right px-4 py-2">Weight</th>
                  <th className="text-right px-4 py-2">Reweight</th>
                  <th className="text-right px-4 py-2">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {osds.map((osd) => (
                  <tr key={osd.id} className="text-gray-900 dark:text-gray-100">
                    <td className="px-4 py-2 font-mono">osd.{osd.id}</td>
                    <td className="px-4 py-2">
                      <span className={osd.status === 'up' ? 'text-green-600' : 'text-red-600'}>{osd.status}</span>
                    </td>
                    <td className="px-4 py-2">
                      {osd.in ? <span className="text-green-600">in</span> : <span className="text-yellow-600">out</span>}
                    </td>
                    <td className="px-4 py-2">{osd.device_class || '—'}</td>
                    <td className="px-4 py-2 text-right">{osd.crush_weight.toFixed(3)}</td>
                    <td className="px-4 py-2 text-right">{osd.reweight.toFixed(3)}</td>
                    <td className="px-4 py-2 text-right space-x-2">
                      {osd.in ? (
                        <button
                          disabled={busy !== null}
                          onClick={() => act(`out-${osd.id}`, () => api.setCephOSDOut(cluster, host, osd.id))}
                          className="text-xs text-yellow-700 hover:underline disabled:text-gray-400"
                        >
                          {busy === `out-${osd.id}` ? '…' : 'Out'}
                        </button>
                      ) : (
                        <button
                          disabled={busy !== null}
                          onClick={() => act(`in-${osd.id}`, () => api.setCephOSDIn(cluster, host, osd.id))}
                          className="text-xs text-green-700 hover:underline disabled:text-gray-400"
                        >
                          {busy === `in-${osd.id}` ? '…' : 'In'}
                        </button>
                      )}
                      <button
                        disabled={busy !== null}
                        onClick={() => act(`scrub-${osd.id}`, () => api.scrubCephOSD(cluster, host, osd.id, { deep: false }))}
                        className="text-xs text-blue-600 hover:underline disabled:text-gray-400"
                      >
                        Scrub
                      </button>
                      <button
                        disabled={busy !== null}
                        onClick={() => act(`deep-${osd.id}`, () => api.scrubCephOSD(cluster, host, osd.id, { deep: true }))}
                        className="text-xs text-blue-600 hover:underline disabled:text-gray-400"
                      >
                        Deep
                      </button>
                      <button
                        disabled={busy !== null || osd.in}
                        title={osd.in ? 'Mark OSD out before destroying' : 'Destroy OSD (with LVM cleanup)'}
                        onClick={() => {
                          if (window.confirm(`Destroy osd.${osd.id} on ${host}?\n\nThe OSD will be removed and its LVM volumes wiped. This cannot be undone.`)) {
                            act(`del-${osd.id}`, () => api.deleteCephOSD(cluster, host, osd.id, { cleanup: true }));
                          }
                        }}
                        className="text-xs text-red-600 hover:underline disabled:text-gray-400"
                      >
                        Destroy
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))
      )}
    </div>
  );
}

function PoolsTab({ cluster, topology, onRefresh }: { cluster: string; topology: CephCluster; onRefresh: () => void }) {
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<CephPool | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const deletePool = async (pool: CephPool) => {
    if (!window.confirm(`Delete pool ${pool.pool_name}?\n\nThe pool and any PVE Storage entries pointing at it will be removed. This cannot be undone.`)) return;
    setBusy(pool.pool_name);
    setActionError(null);
    try {
      await api.deleteCephPool(cluster, pool.pool_name, { remove_storages: true });
      await onRefresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <button
          onClick={() => setCreating(true)}
          className="text-sm bg-blue-600 hover:bg-blue-700 text-white rounded px-3 py-1.5"
        >
          + New pool
        </button>
      </div>

      {actionError && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{actionError}</div>}

      <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-900 text-xs uppercase text-gray-500 dark:text-gray-400">
            <tr>
              <th className="text-left px-4 py-2">Name</th>
              <th className="text-left px-4 py-2">Application</th>
              <th className="text-right px-4 py-2">Size / Min</th>
              <th className="text-right px-4 py-2">PG num</th>
              <th className="text-left px-4 py-2">Autoscale</th>
              <th className="text-right px-4 py-2">Used</th>
              <th className="text-right px-4 py-2">Avail</th>
              <th className="text-right px-4 py-2">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
            {topology.pools.length === 0 ? (
              <tr><td colSpan={8} className="px-4 py-6 text-center text-gray-500">No pools.</td></tr>
            ) : (
              topology.pools.map((p) => (
                <tr key={p.id} className="text-gray-900 dark:text-gray-100">
                  <td className="px-4 py-2 font-mono">{p.pool_name}</td>
                  <td className="px-4 py-2">{p.application || '—'}</td>
                  <td className="px-4 py-2 text-right">{p.size}/{p.min_size}</td>
                  <td className="px-4 py-2 text-right">{p.pg_num}</td>
                  <td className="px-4 py-2">{p.pg_autoscale_mode || '—'}</td>
                  <td className="px-4 py-2 text-right">{p.bytes_used != null ? formatBytes(p.bytes_used) : '—'}</td>
                  <td className="px-4 py-2 text-right">{p.max_avail != null ? formatBytes(p.max_avail) : '—'}</td>
                  <td className="px-4 py-2 text-right space-x-2">
                    <button
                      disabled={busy !== null}
                      onClick={() => setEditing(p)}
                      className="text-xs text-blue-600 hover:underline disabled:text-gray-400"
                    >
                      Edit
                    </button>
                    <button
                      disabled={busy !== null}
                      onClick={() => deletePool(p)}
                      className="text-xs text-red-600 hover:underline disabled:text-gray-400"
                    >
                      {busy === p.pool_name ? '…' : 'Delete'}
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {creating && (
        <PoolCreateDialog
          cluster={cluster}
          onClose={() => setCreating(false)}
          onSuccess={() => { setCreating(false); onRefresh(); }}
        />
      )}
      {editing && (
        <PoolEditDialog
          cluster={cluster}
          pool={editing}
          onClose={() => setEditing(null)}
          onSuccess={() => { setEditing(null); onRefresh(); }}
        />
      )}
    </div>
  );
}

function MonitorsTab({ cluster, topology, onRefresh }: { cluster: string; topology: CephCluster; onRefresh: () => void }) {
  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [adding, setAdding] = useState<{ kind: 'mon' | 'mgr' | 'mds' } | null>(null);

  const remove = async (kind: 'mon' | 'mgr' | 'mds', host: string, id: string) => {
    const friendly = kind === 'mon' ? 'monitor' : kind === 'mgr' ? 'manager' : 'metadata server';
    const warning =
      kind === 'mon'
        ? 'Removing the last MON will break the cluster — confirm quorum will survive.'
        : kind === 'mgr'
        ? 'Removing the active MGR triggers failover; the last MGR loses metrics but I/O continues.'
        : 'Removing the active MDS triggers failover; with no standbys, the FS goes degraded until a new MDS is created.';
    if (!window.confirm(`Remove ${friendly} ${id} from ${host}?\n\n${warning}`)) return;
    setBusy(`${kind}-${id}`);
    setActionError(null);
    try {
      if (kind === 'mon') await api.deleteCephMON(cluster, host, id);
      else if (kind === 'mgr') await api.deleteCephMGR(cluster, host, id);
      else await api.deleteCephMDS(cluster, host, id);
      await onRefresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-6">
      {actionError && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{actionError}</div>}

      <DaemonSection
        title="Monitors (MON)"
        addLabel="+ Add monitor"
        onAdd={() => setAdding({ kind: 'mon' })}
        rows={topology.mons.map<DaemonRow>((m: CephMON) => ({
          name: m.name,
          host: m.host,
          extra: `rank ${m.rank}`,
          state: m.state,
          quorum: m.quorum,
          onRemove: () => remove('mon', m.host, m.name),
          busy: busy === `mon-${m.name}`,
        }))}
      />
      <DaemonSection
        title="Managers (MGR)"
        addLabel="+ Add manager"
        onAdd={() => setAdding({ kind: 'mgr' })}
        rows={topology.mgrs.map<DaemonRow>((m: CephMGR) => ({
          name: m.name,
          host: m.host,
          extra: '',
          state: m.state,
          onRemove: () => remove('mgr', m.host, m.name),
          busy: busy === `mgr-${m.name}`,
        }))}
      />
      <DaemonSection
        title="Metadata servers (MDS)"
        addLabel="+ Add MDS"
        onAdd={() => setAdding({ kind: 'mds' })}
        rows={topology.mdss.map<DaemonRow>((m: CephMDS) => ({
          name: m.name,
          host: m.host,
          extra: m.rank ? `rank ${m.rank}` : '',
          state: m.state,
          onRemove: () => remove('mds', m.host, m.name),
          busy: busy === `mds-${m.name}`,
        }))}
      />

      {adding && (
        <DaemonAddDialog
          cluster={cluster}
          kind={adding.kind}
          onClose={() => setAdding(null)}
          onSuccess={() => { setAdding(null); onRefresh(); }}
        />
      )}
    </div>
  );
}

function CephFSTab({ cluster, topology, onRefresh }: { cluster: string; topology: CephCluster; onRefresh: () => void }) {
  const [creating, setCreating] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  // CephFS deletion needs a node — pick any MDS host, falling back to the
  // first MON host. This avoids asking the operator to choose; PVE doesn't
  // care which node receives the DELETE call.
  const pickHost = (): string | null => {
    if (topology.mdss.length > 0) return topology.mdss[0].host;
    if (topology.mons.length > 0) return topology.mons[0].host;
    return null;
  };

  const deleteFS = async (name: string) => {
    const host = pickHost();
    if (!host) {
      setActionError('No MDS or MON available to route the delete request through.');
      return;
    }
    if (!window.confirm(`Delete CephFS ${name}?\n\nThe filesystem, its PVE Storage entries, AND the underlying data + metadata pools will be removed. This cannot be undone.`)) return;
    setBusy(name);
    setActionError(null);
    try {
      await api.deleteCephFS(cluster, host, name, { remove_storages: true, remove_pools: true });
      await onRefresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  const createDisabled = topology.mdss.length === 0;

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <button
          onClick={() => setCreating(true)}
          disabled={createDisabled}
          title={createDisabled ? 'Create at least one MDS before creating a CephFS' : 'Create a new CephFS'}
          className="text-sm bg-blue-600 hover:bg-blue-700 disabled:bg-gray-400 text-white rounded px-3 py-1.5"
        >
          + New CephFS
        </button>
      </div>

      {actionError && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{actionError}</div>}

      <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-900 text-xs uppercase text-gray-500 dark:text-gray-400">
            <tr>
              <th className="text-left px-4 py-2">Name</th>
              <th className="text-left px-4 py-2">Metadata pool</th>
              <th className="text-left px-4 py-2">Data pools</th>
              <th className="text-right px-4 py-2">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
            {topology.fs.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-4 py-6 text-center text-gray-500">
                  No CephFS filesystems.{' '}
                  {topology.mdss.length === 0 && (
                    <span className="text-xs text-gray-400">
                      Add an MDS in the Monitors tab first.
                    </span>
                  )}
                </td>
              </tr>
            ) : (
              topology.fs.map((f) => (
                <tr key={f.name} className="text-gray-900 dark:text-gray-100">
                  <td className="px-4 py-2 font-mono">{f.name}</td>
                  <td className="px-4 py-2 font-mono">{f.metadata_pool || '—'}</td>
                  <td className="px-4 py-2 font-mono">{f.data_pools?.join(', ') || '—'}</td>
                  <td className="px-4 py-2 text-right">
                    <button
                      disabled={busy !== null}
                      onClick={() => deleteFS(f.name)}
                      className="text-xs text-red-600 hover:underline disabled:text-gray-400"
                    >
                      {busy === f.name ? '…' : 'Delete'}
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {creating && (
        <CephFSCreateDialog
          cluster={cluster}
          host={pickHost() || ''}
          onClose={() => setCreating(false)}
          onSuccess={() => { setCreating(false); onRefresh(); }}
        />
      )}
    </div>
  );
}

function CrushTab({ cluster, topology }: { cluster: string; topology: CephCluster }) {
  const [crushMap, setCrushMap] = useState<string | null>(null);
  const [crushError, setCrushError] = useState<string | null>(null);
  const [crushLoading, setCrushLoading] = useState(false);

  const loadMap = useCallback(async () => {
    setCrushLoading(true);
    setCrushError(null);
    try {
      setCrushMap(await api.getCephCrushMap(cluster));
    } catch (err) {
      setCrushError(err instanceof Error ? err.message : String(err));
    } finally {
      setCrushLoading(false);
    }
  }, [cluster]);

  useEffect(() => {
    loadMap();
  }, [loadMap]);

  return (
    <div className="space-y-4">
      <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
        <div className="px-4 py-2 border-b border-gray-200 dark:border-gray-700 text-sm font-medium text-gray-700 dark:text-gray-200">
          CRUSH rules
        </div>
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-900 text-xs uppercase text-gray-500 dark:text-gray-400">
            <tr>
              <th className="text-left px-4 py-2">ID</th>
              <th className="text-left px-4 py-2">Name</th>
              <th className="text-right px-4 py-2">Type</th>
              <th className="text-right px-4 py-2">Steps</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
            {topology.rules.length === 0 ? (
              <tr><td colSpan={4} className="px-4 py-6 text-center text-gray-500">No CRUSH rules.</td></tr>
            ) : (
              topology.rules.map((r) => (
                <tr key={r.rule_id} className="text-gray-900 dark:text-gray-100">
                  <td className="px-4 py-2">{r.rule_id}</td>
                  <td className="px-4 py-2 font-mono">{r.rule_name}</td>
                  <td className="px-4 py-2 text-right">{r.type === 1 ? 'replicated' : r.type === 3 ? 'erasure' : r.type ?? '—'}</td>
                  <td className="px-4 py-2 text-right">{r.steps_count ?? '—'}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
        <div className="px-4 py-2 border-b border-gray-200 dark:border-gray-700 text-sm font-medium text-gray-700 dark:text-gray-200 flex justify-between items-center">
          <span>Decompiled CRUSH map (read-only)</span>
          <button onClick={loadMap} disabled={crushLoading} className="text-sm text-blue-600 hover:underline disabled:text-gray-400">
            {crushLoading ? 'Loading…' : 'Reload'}
          </button>
        </div>
        {crushError && <div className="bg-red-50 text-red-700 px-3 py-2 m-3 rounded text-sm">{crushError}</div>}
        <pre className="p-4 text-xs font-mono overflow-x-auto whitespace-pre text-gray-800 dark:text-gray-200 max-h-[600px] overflow-y-auto">
          {crushMap || (crushLoading ? 'Loading…' : '(empty)')}
        </pre>
      </div>
    </div>
  );
}

interface DaemonRow {
  name: string;
  host: string;
  extra: string;
  state: string;
  quorum?: boolean;
  onRemove?: () => void;
  busy?: boolean;
}

function DaemonSection({ title, rows, addLabel, onAdd }: { title: string; rows: DaemonRow[]; addLabel?: string; onAdd?: () => void }) {
  return (
    <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
      <div className="px-4 py-2 border-b border-gray-200 dark:border-gray-700 flex justify-between items-center">
        <span className="text-sm font-medium text-gray-700 dark:text-gray-200">{title}</span>
        {addLabel && onAdd && (
          <button onClick={onAdd} className="text-sm text-blue-600 hover:underline">
            {addLabel}
          </button>
        )}
      </div>
      {rows.length === 0 ? (
        <div className="px-4 py-6 text-center text-gray-500">None.</div>
      ) : (
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-900 text-xs uppercase text-gray-500 dark:text-gray-400">
            <tr>
              <th className="text-left px-4 py-2">Name</th>
              <th className="text-left px-4 py-2">Host</th>
              <th className="text-left px-4 py-2">State</th>
              <th className="text-left px-4 py-2"></th>
              <th className="text-right px-4 py-2">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
            {rows.map((r) => (
              <tr key={`${r.host}-${r.name}`} className="text-gray-900 dark:text-gray-100">
                <td className="px-4 py-2 font-mono">{r.name}</td>
                <td className="px-4 py-2">{r.host}</td>
                <td className="px-4 py-2">
                  {r.state}{r.quorum != null && (r.quorum ? ' · in quorum' : ' · OUT of quorum')}
                </td>
                <td className="px-4 py-2 text-gray-500">{r.extra}</td>
                <td className="px-4 py-2 text-right">
                  {r.onRemove && (
                    <button
                      disabled={r.busy}
                      onClick={r.onRemove}
                      className="text-xs text-red-600 hover:underline disabled:text-gray-400"
                    >
                      {r.busy ? '…' : 'Remove'}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

const FLAG_DEFS: { key: keyof CephFlags; label: string; description: string }[] = [
  { key: 'noout', label: 'noout', description: 'Prevent OSDs from being marked out (use during maintenance).' },
  { key: 'noin', label: 'noin', description: 'Prevent OSDs from being marked in.' },
  { key: 'noup', label: 'noup', description: 'Prevent OSDs from being marked up.' },
  { key: 'nodown', label: 'nodown', description: 'Prevent OSDs from being marked down.' },
  { key: 'nobackfill', label: 'nobackfill', description: 'Pause backfills.' },
  { key: 'norebalance', label: 'norebalance', description: 'Pause CRUSH rebalancing.' },
  { key: 'norecover', label: 'norecover', description: 'Pause recovery.' },
  { key: 'noscrub', label: 'noscrub', description: 'Pause shallow scrubs.' },
  { key: 'nodeep-scrub', label: 'nodeep-scrub', description: 'Pause deep scrubs.' },
  { key: 'pause', label: 'pause', description: 'Pause all client I/O — destructive, use with care.' },
];

function FlagsTab({ cluster, topology, onRefresh }: { cluster: string; topology: CephCluster; onRefresh: () => void }) {
  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const toggle = async (flag: string, enable: boolean) => {
    setBusy(flag);
    setActionError(null);
    try {
      await api.toggleCephFlag(cluster, flag, enable);
      await onRefresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-4">
      {actionError && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{actionError}</div>}

      <div className="rounded border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-900 text-xs uppercase text-gray-500 dark:text-gray-400">
            <tr>
              <th className="text-left px-4 py-2">Flag</th>
              <th className="text-left px-4 py-2">Description</th>
              <th className="text-right px-4 py-2">State</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
            {FLAG_DEFS.map((f) => {
              const enabled = topology.flags[f.key];
              return (
                <tr key={f.key} className="text-gray-900 dark:text-gray-100">
                  <td className="px-4 py-2 font-mono">{f.label}</td>
                  <td className="px-4 py-2 text-sm text-gray-600 dark:text-gray-400">{f.description}</td>
                  <td className="px-4 py-2 text-right">
                    <button
                      disabled={busy !== null}
                      onClick={() => toggle(f.label, !enabled)}
                      className={`text-xs px-3 py-1 rounded ${
                        enabled ? 'bg-yellow-100 text-yellow-800 hover:bg-yellow-200' : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                      } disabled:opacity-50`}
                    >
                      {busy === f.label ? '…' : enabled ? 'Set — click to clear' : 'Click to set'}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// --- Dialogs ---

function PoolCreateDialog({ cluster, onClose, onSuccess }: { cluster: string; onClose: () => void; onSuccess: () => void }) {
  const [name, setName] = useState('');
  const [size, setSize] = useState('3');
  const [minSize, setMinSize] = useState('2');
  const [pgNum, setPgNum] = useState('128');
  const [autoscale, setAutoscale] = useState<'on' | 'off' | 'warn'>('on');
  const [application, setApplication] = useState<'rbd' | 'cephfs' | 'rgw' | ''>('rbd');
  const [addStorages, setAddStorages] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (!name) {
      setError('Name is required');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await api.createCephPool(cluster, {
        name,
        size: Number(size),
        min_size: Number(minSize),
        pg_num: Number(pgNum),
        pg_autoscale_mode: autoscale,
        application: application || undefined,
        add_storages: addStorages,
      });
      onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setSubmitting(false);
    }
  };

  return (
    <Modal title="Create pool" onClose={submitting ? () => {} : onClose}>
      <div className="space-y-3">
        <Field label="Name">
          <input value={name} onChange={(e) => setName(e.target.value)} className={inputCls} placeholder="rbd" />
        </Field>
        <div className="grid grid-cols-3 gap-3">
          <Field label="Size">
            <input type="number" min={1} value={size} onChange={(e) => setSize(e.target.value)} className={inputCls} />
          </Field>
          <Field label="Min size">
            <input type="number" min={1} value={minSize} onChange={(e) => setMinSize(e.target.value)} className={inputCls} />
          </Field>
          <Field label="PG num">
            <input type="number" min={1} value={pgNum} onChange={(e) => setPgNum(e.target.value)} className={inputCls} />
          </Field>
        </div>
        <Field label="Autoscale">
          <select value={autoscale} onChange={(e) => setAutoscale(e.target.value as 'on' | 'off' | 'warn')} className={inputCls}>
            <option value="on">on</option>
            <option value="warn">warn</option>
            <option value="off">off</option>
          </select>
        </Field>
        <Field label="Application">
          <select value={application} onChange={(e) => setApplication(e.target.value as 'rbd' | 'cephfs' | 'rgw' | '')} className={inputCls}>
            <option value="rbd">rbd</option>
            <option value="cephfs">cephfs</option>
            <option value="rgw">rgw</option>
            <option value="">(none)</option>
          </select>
        </Field>
        <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input type="checkbox" checked={addStorages} onChange={(e) => setAddStorages(e.target.checked)} />
          Also create a PVE Storage entry pointing at this pool
        </label>
        {error && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{error}</div>}
      </div>
      <DialogButtons onCancel={onClose} onSubmit={submit} submitLabel="Create" submitting={submitting} />
    </Modal>
  );
}

function PoolEditDialog({ cluster, pool, onClose, onSuccess }: { cluster: string; pool: CephPool; onClose: () => void; onSuccess: () => void }) {
  const [size, setSize] = useState(String(pool.size));
  const [minSize, setMinSize] = useState(String(pool.min_size));
  const [pgNum, setPgNum] = useState(String(pool.pg_num));
  const [autoscale, setAutoscale] = useState<string>(pool.pg_autoscale_mode || 'on');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await api.updateCephPool(cluster, pool.pool_name, {
        size: Number(size) !== pool.size ? Number(size) : undefined,
        min_size: Number(minSize) !== pool.min_size ? Number(minSize) : undefined,
        pg_num: Number(pgNum) !== pool.pg_num ? Number(pgNum) : undefined,
        pg_autoscale_mode: autoscale !== (pool.pg_autoscale_mode || 'on') ? (autoscale as 'on' | 'off' | 'warn') : undefined,
      });
      onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setSubmitting(false);
    }
  };

  return (
    <Modal title={`Edit pool ${pool.pool_name}`} onClose={submitting ? () => {} : onClose}>
      <div className="space-y-3">
        <div className="grid grid-cols-3 gap-3">
          <Field label="Size"><input type="number" min={1} value={size} onChange={(e) => setSize(e.target.value)} className={inputCls} /></Field>
          <Field label="Min size"><input type="number" min={1} value={minSize} onChange={(e) => setMinSize(e.target.value)} className={inputCls} /></Field>
          <Field label="PG num"><input type="number" min={1} value={pgNum} onChange={(e) => setPgNum(e.target.value)} className={inputCls} /></Field>
        </div>
        <Field label="Autoscale">
          <select value={autoscale} onChange={(e) => setAutoscale(e.target.value)} className={inputCls}>
            <option value="on">on</option>
            <option value="warn">warn</option>
            <option value="off">off</option>
          </select>
        </Field>
        {error && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{error}</div>}
      </div>
      <DialogButtons onCancel={onClose} onSubmit={submit} submitLabel="Save" submitting={submitting} />
    </Modal>
  );
}

function DaemonAddDialog({ cluster, kind, onClose, onSuccess }: { cluster: string; kind: 'mon' | 'mgr' | 'mds'; onClose: () => void; onSuccess: () => void }) {
  const [node, setNode] = useState('');
  const [monAddress, setMonAddress] = useState('');
  const [hotstandby, setHotstandby] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (!node) {
      setError('Node is required');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      if (kind === 'mon') {
        await api.createCephMON(cluster, node, monAddress ? { mon_address: monAddress } : undefined);
      } else if (kind === 'mgr') {
        await api.createCephMGR(cluster, node);
      } else {
        await api.createCephMDS(cluster, node, hotstandby ? { hotstandby: true } : undefined);
      }
      onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setSubmitting(false);
    }
  };

  const title = kind === 'mon' ? 'Add monitor' : kind === 'mgr' ? 'Add manager' : 'Add MDS';

  return (
    <Modal title={title} onClose={submitting ? () => {} : onClose}>
      <div className="space-y-3">
        <Field label="Node">
          <input value={node} onChange={(e) => setNode(e.target.value)} className={inputCls} placeholder="pve1" />
        </Field>
        {kind === 'mon' && (
          <Field label="Monitor address (optional)">
            <input value={monAddress} onChange={(e) => setMonAddress(e.target.value)} className={inputCls} placeholder="10.0.0.1" />
            <p className="text-xs text-gray-500 mt-1">Leave blank to let PVE auto-detect.</p>
          </Field>
        )}
        {kind === 'mds' && (
          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
            <input type="checkbox" checked={hotstandby} onChange={(e) => setHotstandby(e.target.checked)} />
            Hot standby (standby-replay — warmer cache, faster failover)
          </label>
        )}
        {error && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{error}</div>}
      </div>
      <DialogButtons onCancel={onClose} onSubmit={submit} submitLabel="Create" submitting={submitting} />
    </Modal>
  );
}

function CephFSCreateDialog({ cluster, host, onClose, onSuccess }: { cluster: string; host: string; onClose: () => void; onSuccess: () => void }) {
  const [name, setName] = useState('');
  const [pgNum, setPgNum] = useState('64');
  const [addStorage, setAddStorage] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (!name) {
      setError('Name is required');
      return;
    }
    if (!host) {
      setError('No MDS or MON host available to route the create through.');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await api.createCephFS(cluster, host, name, {
        pg_num: Number(pgNum) || undefined,
        add_storage: addStorage,
      });
      onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setSubmitting(false);
    }
  };

  return (
    <Modal title="Create CephFS" onClose={submitting ? () => {} : onClose}>
      <div className="space-y-3">
        <Field label="Name">
          <input value={name} onChange={(e) => setName(e.target.value)} className={inputCls} placeholder="cephfs" />
          <p className="text-xs text-gray-500 mt-1">PVE creates {`{name}_data`} and {`{name}_metadata`} pools automatically.</p>
        </Field>
        <Field label="PG num (data pool)">
          <input type="number" min={1} value={pgNum} onChange={(e) => setPgNum(e.target.value)} className={inputCls} />
        </Field>
        <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input type="checkbox" checked={addStorage} onChange={(e) => setAddStorage(e.target.checked)} />
          Also create a PVE Storage entry of type cephfs
        </label>
        {error && <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">{error}</div>}
      </div>
      <DialogButtons onCancel={onClose} onSubmit={submit} submitLabel="Create" submitting={submitting} />
    </Modal>
  );
}

// --- UI primitives ---

const inputCls = 'w-full border border-gray-300 dark:border-gray-600 rounded px-2 py-1 text-sm bg-white dark:bg-gray-700 dark:text-gray-100';

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="text-gray-700 dark:text-gray-300">{label}</span>
      <div className="mt-1">{children}</div>
    </label>
  );
}

function Modal({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">{title}</h2>
        {children}
      </div>
    </div>
  );
}

function DialogButtons({ onCancel, onSubmit, submitLabel, submitting }: { onCancel: () => void; onSubmit: () => void; submitLabel: string; submitting: boolean }) {
  return (
    <div className="flex justify-end gap-2 mt-6">
      <button
        onClick={onCancel}
        disabled={submitting}
        className="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-50"
      >
        Cancel
      </button>
      <button
        onClick={onSubmit}
        disabled={submitting}
        className="px-3 py-1.5 text-sm rounded bg-blue-600 hover:bg-blue-700 text-white disabled:bg-gray-400"
      >
        {submitting ? 'Working…' : submitLabel}
      </button>
    </div>
  );
}

// --- utils ---

function groupByHost(osds: CephOSD[]): Array<[string, CephOSD[]]> {
  const map = new Map<string, CephOSD[]>();
  for (const osd of osds) {
    const host = osd.host || '(unknown)';
    if (!map.has(host)) map.set(host, []);
    map.get(host)!.push(osd);
  }
  return Array.from(map.entries()).sort(([a], [b]) => a.localeCompare(b)).map(([h, list]) => [h, list.sort((x, y) => x.id - y.id)]);
}
