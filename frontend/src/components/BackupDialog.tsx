import { useState, useMemo } from 'react';
import { api } from '../api/client';
import { useCluster } from '../context/ClusterContext';
import type { Guest } from '../types';

interface Props {
  guest: Guest;
  onClose: () => void;
  onSuccess: () => void;
}

export function BackupDialog({ guest, onClose, onSuccess }: Props) {
  const { storage } = useCluster();
  const [selectedStorage, setSelectedStorage] = useState('');
  const [mode, setMode] = useState<'snapshot' | 'suspend' | 'stop'>('snapshot');
  const [compress, setCompress] = useState<'zstd' | 'gzip' | 'lzo' | '0'>('zstd');
  const [notes, setNotes] = useState('');
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Backup-capable storages for this guest's cluster + node
  const eligibleStorage = useMemo(() => {
    return (storage || [])
      .filter(s => s.cluster === guest.cluster)
      .filter(s => (s.content || '').includes('backup'))
      .filter(s =>
        // Storage is accessible from guest's node: either shared, or on same node
        s.shared === 1 || s.node === guest.node
      );
  }, [storage, guest.cluster, guest.node]);

  // Pick first eligible storage by default if none selected
  if (!selectedStorage && eligibleStorage.length > 0 && !loading) {
    setSelectedStorage(eligibleStorage[0].storage);
  }

  const onSubmit = async () => {
    setErr(null);
    if (!selectedStorage) { setErr('Select a storage target'); return }
    setLoading(true);
    try {
      await api.createBackup(guest.cluster, guest.node, {
        vmids: [guest.vmid],
        storage: selectedStorage,
        mode,
        compress,
        notes: notes || undefined,
      });
      onSuccess();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'backup failed');
      setLoading(false);
    }
  };

  const isRunning = guest.status === 'running';

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-white">
          Backup {guest.type === 'qemu' ? 'VM' : 'Container'}: {guest.name} ({guest.vmid})
        </h2>

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Target Storage <span className="text-red-500">*</span></label>
            {eligibleStorage.length === 0 ? (
              <div className="text-sm text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-900/20 px-3 py-2 rounded">
                No backup-capable storage accessible from {guest.node}. Add one in Proxmox with <code>content: backup</code>.
              </div>
            ) : (
              <select value={selectedStorage} onChange={e => setSelectedStorage(e.target.value)} disabled={loading}
                className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2">
                {eligibleStorage.map(s => (
                  <option key={s.storage} value={s.storage}>
                    {s.storage} ({s.type}{s.shared === 1 ? ', shared' : ''})
                  </option>
                ))}
              </select>
            )}
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Mode</label>
              <select value={mode} onChange={e => setMode(e.target.value as typeof mode)} disabled={loading}
                className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2">
                <option value="snapshot">Snapshot (live, no downtime)</option>
                <option value="suspend">Suspend (brief pause)</option>
                <option value="stop">Stop (full downtime)</option>
              </select>
              {!isRunning && (
                <p className="text-xs text-gray-500 mt-1">Guest is stopped — all modes behave the same.</p>
              )}
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Compression</label>
              <select value={compress} onChange={e => setCompress(e.target.value as typeof compress)} disabled={loading}
                className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2">
                <option value="zstd">zstd (fast, default)</option>
                <option value="gzip">gzip</option>
                <option value="lzo">lzo</option>
                <option value="0">none</option>
              </select>
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Notes</label>
            <input type="text" value={notes} onChange={e => setNotes(e.target.value)} disabled={loading}
              placeholder="optional — shown in backup list"
              className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2" />
          </div>

          {err && <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 px-3 py-2 rounded text-sm">{err}</div>}
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} disabled={loading}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded">Cancel</button>
          <button onClick={onSubmit} disabled={loading || !selectedStorage}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
            {loading ? 'Starting…' : 'Start Backup'}
          </button>
        </div>
      </div>
    </div>
  );
}
