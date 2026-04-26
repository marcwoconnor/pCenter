import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';
import type { Pool, PoolDetail } from '../types';

export function PoolsTab({ clusterName }: { clusterName: string }) {
  const [pools, setPools] = useState<Pool[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [expanded, setExpanded] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await api.listPools(clusterName);
      setPools(data);
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'load failed');
      setPools([]);
    }
  }, [clusterName]);

  // eslint-disable-next-line react-hooks/set-state-in-effect -- fetch on mount; load is stable via useCallback
  useEffect(() => { load() }, [load]);

  const onDelete = async (poolID: string) => {
    if (!confirm(`Delete pool "${poolID}"?\n\nPool must be empty — remove all members first.`)) return;
    try {
      await api.deletePool(clusterName, poolID);
      load();
    } catch (e) {
      alert('Delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  };

  if (pools === null) return <div className="text-gray-500 p-4">Loading pools…</div>;
  if (err) return <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 rounded p-4">{err}</div>;

  return (
    <div className="space-y-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <div className="flex items-center justify-between mb-3">
          <div>
            <h3 className="font-medium text-gray-900 dark:text-white">Resource Pools ({pools.length})</h3>
            <p className="text-xs text-gray-500">Group VMs, containers, and storage into logical pools for tagging, permissions, or reporting.</p>
          </div>
          <button onClick={() => setShowAdd(true)} className="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded">+ Add Pool</button>
        </div>

        {pools.length === 0 ? (
          <p className="text-sm text-gray-500">No pools yet. Click <em>Add Pool</em> to create one.</p>
        ) : (
          <table className="w-full text-sm">
            <thead><tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
              <th className="pb-2 font-medium w-8"></th>
              <th className="pb-2 font-medium">Pool ID</th>
              <th className="pb-2 font-medium">Comment</th>
              <th className="pb-2 font-medium w-32">Actions</th>
            </tr></thead>
            <tbody>
              {pools.map(p => (
                <PoolRow
                  key={p.poolid}
                  pool={p}
                  clusterName={clusterName}
                  expanded={expanded === p.poolid}
                  onToggle={() => setExpanded(expanded === p.poolid ? null : p.poolid)}
                  onDelete={() => onDelete(p.poolid)}
                  onChange={load}
                />
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showAdd && (
        <AddPoolDialog
          clusterName={clusterName}
          onClose={() => setShowAdd(false)}
          onSuccess={() => { setShowAdd(false); load(); }}
        />
      )}
    </div>
  );
}

function PoolRow({
  pool, clusterName, expanded, onToggle, onDelete, onChange,
}: {
  pool: Pool;
  clusterName: string;
  expanded: boolean;
  onToggle: () => void;
  onDelete: () => void;
  onChange: () => void;
}) {
  const [detail, setDetail] = useState<PoolDetail | null>(null);

  useEffect(() => {
    if (!expanded) return;
    (async () => {
      try {
        const d = await api.getPool(clusterName, pool.poolid);
        setDetail(d);
      } catch {
        setDetail({ members: [] });
      }
    })();
  }, [expanded, clusterName, pool.poolid]);

  const removeMember = async (m: { type: string; vmid?: number; storage?: string }) => {
    const vms = (m.type === 'qemu' || m.type === 'lxc') && m.vmid ? [String(m.vmid)] : [];
    const storage = m.type === 'storage' && m.storage ? [m.storage] : [];
    try {
      await api.updatePool(clusterName, pool.poolid, { vms, storage, delete: true });
      // Refresh detail
      const d = await api.getPool(clusterName, pool.poolid);
      setDetail(d);
      onChange();
    } catch (e) {
      alert('Remove failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  };

  return (
    <>
      <tr className="border-b border-gray-100 dark:border-gray-700/50">
        <td className="py-2">
          <button onClick={onToggle} className="text-gray-500 hover:text-gray-700 dark:hover:text-gray-300">
            {expanded ? '▾' : '▸'}
          </button>
        </td>
        <td className="py-2 text-gray-900 dark:text-white">{pool.poolid}</td>
        <td className="py-2 text-gray-700 dark:text-gray-300">{pool.comment || '—'}</td>
        <td className="py-2">
          <button onClick={onDelete} className="text-red-600 hover:text-red-800 text-xs">Delete</button>
        </td>
      </tr>
      {expanded && (
        <tr>
          <td></td>
          <td colSpan={3} className="pb-4">
            {detail === null ? (
              <div className="text-xs text-gray-500">Loading members…</div>
            ) : detail.members.length === 0 ? (
              <div className="text-xs text-gray-500 italic">No members in this pool.</div>
            ) : (
              <div className="bg-gray-50 dark:bg-gray-900/50 rounded p-2 space-y-1">
                {detail.members.map(m => (
                  <div key={m.id} className="flex items-center gap-3 text-xs">
                    <span className={`px-1.5 py-0.5 rounded font-mono ${
                      m.type === 'qemu' ? 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300' :
                      m.type === 'lxc' ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300' :
                      'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300'
                    }`}>{m.type}</span>
                    <span className="text-gray-900 dark:text-white">{m.name || m.id}</span>
                    {m.node && <span className="text-gray-500">@ {m.node}</span>}
                    {m.status && <span className="text-gray-500">· {m.status}</span>}
                    <button onClick={() => removeMember(m)}
                      className="ml-auto text-red-600 hover:text-red-800">Remove</button>
                  </div>
                ))}
              </div>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

function AddPoolDialog({
  clusterName, onClose, onSuccess,
}: {
  clusterName: string;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [poolID, setPoolID] = useState('');
  const [comment, setComment] = useState('');
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const onSave = async () => {
    setErr(null);
    if (!poolID.trim()) { setErr('Pool ID is required'); return }
    if (!/^[A-Za-z0-9_-]+$/.test(poolID)) {
      setErr('Pool ID can only contain letters, digits, dashes, and underscores');
      return;
    }
    setLoading(true);
    try {
      await api.createPool(clusterName, { poolid: poolID.trim(), comment: comment.trim() || undefined });
      onSuccess();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'create failed');
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-white">Create Resource Pool</h2>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Pool ID <span className="text-red-500">*</span></label>
            <input type="text" value={poolID} onChange={e => setPoolID(e.target.value)} disabled={loading}
              placeholder="e.g. production"
              className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Comment</label>
            <input type="text" value={comment} onChange={e => setComment(e.target.value)} disabled={loading}
              placeholder="optional description"
              className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2" />
          </div>
          {err && <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 px-3 py-2 rounded text-sm">{err}</div>}
        </div>
        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} disabled={loading}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded">Cancel</button>
          <button onClick={onSave} disabled={loading}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
            {loading ? 'Creating…' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  );
}
