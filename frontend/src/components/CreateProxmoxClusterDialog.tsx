import { useState, useEffect, useRef } from 'react';
import { api } from '../api/client';
import type {
  Datacenter,
  InventoryHost,
  PveClusterPreflightResponse,
  PveClusterJob,
  PveClusterJoinerInput,
} from '../types';

// Props: the datacenter whose standalone hosts are candidates for the new
// cluster. The parent filters out already-clustered hosts before passing them
// in via `datacenter`. On success, parent refreshes the tree.
interface CreateProxmoxClusterDialogProps {
  datacenter: Datacenter;
  onClose: () => void;
  onSuccess: () => Promise<void>;
}

type Step = 'pick' | 'preflight' | 'creds' | 'progress' | 'done';

export function CreateProxmoxClusterDialog({
  datacenter,
  onClose,
  onSuccess,
}: CreateProxmoxClusterDialogProps) {
  const standaloneHosts: InventoryHost[] = (datacenter.hosts || []).filter(
    (h) => h.status === 'online'
  );

  // --- Step 1: Pick founder, joiners, name ---
  const [step, setStep] = useState<Step>('pick');
  const [clusterName, setClusterName] = useState('');
  const [founderId, setFounderId] = useState<string>(
    standaloneHosts[0]?.id || ''
  );
  const [joinerIds, setJoinerIds] = useState<Set<string>>(new Set());
  const [pickError, setPickError] = useState<string | null>(null);

  // --- Step 2: Preflight ---
  const [preflight, setPreflight] = useState<PveClusterPreflightResponse | null>(
    null
  );
  const [preflightLoading, setPreflightLoading] = useState(false);
  const [preflightError, setPreflightError] = useState<string | null>(null);

  // --- Step 3: Credentials ---
  const [sharedPassword, setSharedPassword] = useState(true);
  const [passwords, setPasswords] = useState<Record<string, string>>({});
  const [credsError, setCredsError] = useState<string | null>(null);

  // --- Step 4: Progress ---
  // jobId is implicitly captured in the setInterval closure via startPolling's
  // argument; we don't need to store it in state.
  const [job, setJob] = useState<PveClusterJob | null>(null);
  const [progressError, setProgressError] = useState<string | null>(null);
  const pollRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current);
    };
  }, []);

  // --- Helpers ---
  const joiners = standaloneHosts.filter(
    (h) => h.id !== founderId && joinerIds.has(h.id)
  );
  const founder = standaloneHosts.find((h) => h.id === founderId);

  const toggleJoiner = (id: string) => {
    const next = new Set(joinerIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setJoinerIds(next);
  };

  // --- Step 1 → 2 ---
  const runPreflight = async () => {
    setPickError(null);
    const trimmed = clusterName.trim();
    if (!trimmed) {
      setPickError('Cluster name is required');
      return;
    }
    if (!founderId) {
      setPickError('Pick a founder host');
      return;
    }
    // Joiners are optional: Proxmox allows forming a 1-node cluster and
    // adding more nodes later. No check here.
    setPreflightLoading(true);
    setPreflightError(null);
    try {
      const resp = await api.preflightPveCluster({
        cluster_name: trimmed,
        founder_host_id: founderId,
        joiner_host_ids: joiners.map((j) => j.id),
      });
      setPreflight(resp);
      setStep('preflight');
    } catch (err) {
      setPreflightError(err instanceof Error ? err.message : 'preflight failed');
    } finally {
      setPreflightLoading(false);
    }
  };

  // --- Step 3 → 4: submit creation job ---
  const submit = async () => {
    setCredsError(null);
    const allHostIds = [founderId, ...joiners.map((j) => j.id)];
    // Don't trim — PVE passwords are passed verbatim (a leading/trailing
    // space could be intentional, even if rare). We only check for the
    // never-typed-anything case below.
    const effectivePw = (id: string) =>
      sharedPassword ? passwords['_shared'] || '' : passwords[id] || '';

    for (const id of allHostIds) {
      if (!effectivePw(id)) {
        setCredsError('Please enter the root password for every host');
        return;
      }
    }

    const joinerInputs: PveClusterJoinerInput[] = joiners.map((j) => ({
      host_id: j.id,
      password: effectivePw(j.id),
    }));

    try {
      const resp = await api.createPveCluster({
        cluster_name: clusterName.trim(),
        datacenter_id: datacenter.id,
        founder_host_id: founderId,
        founder_password: effectivePw(founderId),
        joiners: joinerInputs,
      });
      setStep('progress');
      startPolling(resp.job_id);
    } catch (err) {
      setCredsError(err instanceof Error ? err.message : 'start failed');
    }
  };

  const startPolling = (id: string) => {
    const poll = async () => {
      try {
        const j = await api.getPveClusterJob(id);
        setJob(j);
        if (j.state === 'succeeded') {
          if (pollRef.current) window.clearInterval(pollRef.current);
          // Tree needs a moment to catch up (poller just got AddCluster'd).
          setTimeout(async () => {
            await onSuccess();
            setStep('done');
          }, 800);
        } else if (j.state === 'failed') {
          if (pollRef.current) window.clearInterval(pollRef.current);
        }
      } catch (err) {
        // 404 means server restarted and lost job state.
        const msg = err instanceof Error ? err.message : '';
        if (msg.includes('not found')) {
          setProgressError(
            'Job state lost (pcenter may have restarted). Verify the cluster in the PVE web UI, then Rescan.'
          );
          if (pollRef.current) window.clearInterval(pollRef.current);
        }
      }
    };
    void poll();
    pollRef.current = window.setInterval(poll, 1500);
  };

  // --- Render ---
  const title = {
    pick: 'Create Proxmox Cluster — Select Hosts',
    preflight: 'Create Proxmox Cluster — Pre-flight Checks',
    creds: 'Create Proxmox Cluster — Root Passwords',
    progress: 'Creating Proxmox Cluster…',
    done: 'Cluster Created',
  }[step];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div
        className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-5 min-w-[520px] max-w-2xl max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between mb-4">
          <h3 className="text-lg font-medium text-gray-900 dark:text-gray-100">
            {title}
          </h3>
          <button
            className="text-gray-400 hover:text-gray-600 text-xl leading-none"
            onClick={onClose}
            aria-label="Close"
          >
            ×
          </button>
        </div>

        {/* Step 1 */}
        {step === 'pick' && (
          <div>
            {standaloneHosts.length === 0 && (
              <p className="text-sm text-amber-600 dark:text-amber-400 mb-3">
                No online standalone hosts in this datacenter. Add one first.
              </p>
            )}
            {standaloneHosts.length === 1 && (
              <p className="text-sm text-blue-600 dark:text-blue-400 mb-3">
                Only one standalone host. You'll form a 1-node cluster; add
                joiners later by repeating this wizard after adding more
                standalone hosts.
              </p>
            )}
            <div className="mb-3">
              <label className="block text-sm font-medium mb-1">
                Cluster name
              </label>
              <input
                type="text"
                value={clusterName}
                onChange={(e) => setClusterName(e.target.value)}
                placeholder="e.g. homelab1"
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
              />
              <p className="text-xs text-gray-500 mt-1">
                1–15 characters, letters/digits/dashes only (Proxmox's rule).
              </p>
            </div>

            <div className="mb-3">
              <label className="block text-sm font-medium mb-1">
                Founder (will create the cluster)
              </label>
              <select
                value={founderId}
                onChange={(e) => {
                  setFounderId(e.target.value);
                  // If the new founder was a joiner, unselect it.
                  const next = new Set(joinerIds);
                  next.delete(e.target.value);
                  setJoinerIds(next);
                }}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
              >
                {standaloneHosts.map((h) => (
                  <option key={h.id} value={h.id}>
                    {h.node_name || h.address} ({h.address})
                  </option>
                ))}
              </select>
            </div>

            <div className="mb-3">
              <label className="block text-sm font-medium mb-1">
                Joiners (optional, {joiners.length} selected)
              </label>
              <div className="border border-gray-300 dark:border-gray-600 rounded-md divide-y divide-gray-200 dark:divide-gray-700">
                {standaloneHosts
                  .filter((h) => h.id !== founderId)
                  .map((h) => (
                    <label
                      key={h.id}
                      className="flex items-center gap-2 p-2 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700/30"
                    >
                      <input
                        type="checkbox"
                        checked={joinerIds.has(h.id)}
                        onChange={() => toggleJoiner(h.id)}
                      />
                      <span className="text-sm">
                        {h.node_name || h.address}{' '}
                        <span className="text-gray-500">({h.address})</span>
                      </span>
                    </label>
                  ))}
                {standaloneHosts.length < 2 && (
                  <p className="text-xs text-gray-500 p-2">
                    No other standalone hosts available.
                  </p>
                )}
              </div>
            </div>

            {pickError && (
              <p className="text-sm text-red-500 mb-3">{pickError}</p>
            )}
            {preflightError && (
              <p className="text-sm text-red-500 mb-3">{preflightError}</p>
            )}

            <div className="flex justify-end gap-2">
              <button
                onClick={onClose}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
              >
                Cancel
              </button>
              <button
                onClick={runPreflight}
                disabled={preflightLoading || standaloneHosts.length < 1}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {preflightLoading ? 'Checking…' : 'Next: Pre-flight Checks'}
              </button>
            </div>
          </div>
        )}

        {/* Step 2: Preflight */}
        {step === 'preflight' && preflight && (
          <div>
            {!preflight.cluster_name_ok && (
              <p className="text-sm text-red-500 mb-3">
                Cluster name invalid: {preflight.cluster_name_message}
              </p>
            )}
            <div className="space-y-2 mb-4">
              {(preflight.hosts || []).map((h) => {
                const blockers = h.blockers || [];
                const ok = blockers.length === 0;
                return (
                  <div
                    key={h.host_id}
                    className={`border rounded p-3 ${
                      ok
                        ? 'border-green-300 dark:border-green-700 bg-green-50/50 dark:bg-green-900/10'
                        : 'border-red-300 dark:border-red-700 bg-red-50/50 dark:bg-red-900/10'
                    }`}
                  >
                    <div className="flex items-center gap-2 mb-1">
                      <span
                        className={ok ? 'text-green-600' : 'text-red-600'}
                      >
                        {ok ? '✓' : '✗'}
                      </span>
                      <span className="font-medium">
                        {h.node_name || h.address}{' '}
                        <span className="text-xs text-gray-500">
                          ({h.role}, PVE {h.pve_version || '?'})
                        </span>
                      </span>
                    </div>
                    <div className="text-xs text-gray-600 dark:text-gray-400 ml-5">
                      Reachable: {h.reachable ? 'yes' : 'no'} · VMs: {h.vm_count}{' '}
                      · CTs: {h.ct_count} · In cluster:{' '}
                      {h.already_in_cluster ? 'yes' : 'no'}
                    </div>
                    {blockers.map((b, i) => (
                      <div key={i} className="text-sm text-red-600 mt-1 ml-5">
                        • {b}
                      </div>
                    ))}
                  </div>
                );
              })}
            </div>
            <div className="flex justify-between gap-2">
              <button
                onClick={() => setStep('pick')}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
              >
                ← Back
              </button>
              <button
                onClick={() => setStep('creds')}
                disabled={!preflight.can_proceed}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                Next: Enter Passwords
              </button>
            </div>
          </div>
        )}

        {/* Step 3: Creds */}
        {step === 'creds' && (
          <div>
            <p className="text-sm text-gray-600 dark:text-gray-400 mb-3">
              Proxmox's join endpoint requires the{' '}
              <code className="bg-gray-100 dark:bg-gray-700 px-1 rounded">
                root@pam
              </code>{' '}
              password — API tokens are not accepted. Passwords are used once
              to form/join the cluster and to re-create the pcenter API token;
              they are not stored.
            </p>
            <label className="flex items-center gap-2 mb-3 cursor-pointer">
              <input
                type="checkbox"
                checked={sharedPassword}
                onChange={(e) => {
                  setSharedPassword(e.target.checked);
                  setPasswords({});
                }}
              />
              <span className="text-sm">Same root password for all hosts</span>
            </label>

            {sharedPassword ? (
              <div className="mb-3">
                <label className="block text-sm font-medium mb-1">
                  Root password
                </label>
                <input
                  type="password"
                  value={passwords['_shared'] || ''}
                  onChange={(e) =>
                    setPasswords({ _shared: e.target.value })
                  }
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
                  autoFocus
                />
              </div>
            ) : (
              <div className="space-y-2 mb-3">
                {[founder, ...joiners].filter(Boolean).map((h) => (
                  <div key={h!.id}>
                    <label className="block text-sm font-medium mb-1">
                      {h!.node_name || h!.address}
                    </label>
                    <input
                      type="password"
                      value={passwords[h!.id] || ''}
                      onChange={(e) =>
                        setPasswords((prev) => ({
                          ...prev,
                          [h!.id]: e.target.value,
                        }))
                      }
                      className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
                    />
                  </div>
                ))}
              </div>
            )}

            {credsError && (
              <p className="text-sm text-red-500 mb-3">{credsError}</p>
            )}

            <div className="flex justify-between gap-2">
              <button
                onClick={() => setStep('preflight')}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
              >
                ← Back
              </button>
              <button
                onClick={submit}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
              >
                Create Cluster
              </button>
            </div>
          </div>
        )}

        {/* Step 4: Progress */}
        {step === 'progress' && (
          <div>
            <p className="text-sm text-gray-600 dark:text-gray-400 mb-3">
              Forming cluster{' '}
              <span className="font-medium">{clusterName}</span>… this usually
              takes under 2 minutes.
            </p>
            {progressError ? (
              <p className="text-sm text-red-600 mb-3">{progressError}</p>
            ) : job ? (
              <ProgressSteps job={job} />
            ) : (
              <p className="text-sm text-gray-500">Starting…</p>
            )}
            {job?.state === 'failed' && (
              <div className="mt-4 border border-red-300 dark:border-red-700 bg-red-50/50 dark:bg-red-900/10 rounded p-3">
                <div className="text-sm font-medium text-red-700 dark:text-red-400 mb-1">
                  Cluster formation failed
                </div>
                <pre className="text-xs whitespace-pre-wrap text-red-700 dark:text-red-400 font-mono">
                  {job.error}
                </pre>
                <button
                  onClick={() => {
                    if (job.error) navigator.clipboard.writeText(job.error);
                  }}
                  className="mt-2 text-xs px-2 py-1 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300 rounded hover:bg-red-200"
                >
                  Copy error
                </button>
              </div>
            )}
            <div className="flex justify-end mt-4">
              <button
                onClick={onClose}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
              >
                {job?.state === 'failed' ? 'Close' : 'Hide (keeps running)'}
              </button>
            </div>
          </div>
        )}

        {/* Step 5: Done */}
        {step === 'done' && (
          <div>
            <p className="text-sm text-green-600 dark:text-green-400 mb-3">
              ✓ Cluster <span className="font-medium">{clusterName}</span>{' '}
              formed successfully. Your hosts have been grouped in the tree.
            </p>
            <div className="flex justify-end">
              <button
                onClick={onClose}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
              >
                Done
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ProgressSteps renders the vertical step list with per-step state icons.
function ProgressSteps({ job }: { job: PveClusterJob }) {
  const icon = (state: string) =>
    state === 'succeeded'
      ? '✓'
      : state === 'failed'
      ? '✗'
      : state === 'running'
      ? '↻'
      : '·';
  const color = (state: string) =>
    state === 'succeeded'
      ? 'text-green-600'
      : state === 'failed'
      ? 'text-red-600'
      : state === 'running'
      ? 'text-blue-600 animate-pulse'
      : 'text-gray-400';

  const label = (phase: string) =>
    ({
      create_cluster: 'Create cluster',
      fetch_join_info: 'Fetch join info',
      join: 'Join cluster',
      reauth_token: 'Refresh API token',
      update_inventory: 'Update pcenter inventory',
    }[phase] || phase);

  return (
    <ol className="space-y-1">
      {job.steps.map((s, i) => (
        <li
          key={i}
          className="flex items-start gap-2 text-sm py-1"
        >
          <span className={`w-4 text-center ${color(s.state)}`}>
            {icon(s.state)}
          </span>
          <span className="flex-1">
            <span className="font-medium">
              {s.node_name || s.address || 'host'}
            </span>{' '}
            <span className="text-gray-500">— {label(s.phase)}</span>
            {s.message && (
              <span className="text-xs text-gray-500 ml-2">{s.message}</span>
            )}
            {s.error && (
              <div className="text-xs text-red-600 mt-0.5">{s.error}</div>
            )}
          </span>
        </li>
      ))}
    </ol>
  );
}
