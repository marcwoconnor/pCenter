import { useState, useEffect, useMemo } from 'react';
import { api, formatBytes } from '../api/client';
import type { Guest, Storage } from '../types';

interface MoveStorageDialogProps {
  guest: Guest;
  onClose: () => void;
  onSuccess: () => void;
}

interface DiskOption {
  key: string;       // scsi0, virtio0, rootfs, mp0, ...
  storage: string;   // local-lvm
  volid: string;     // local-lvm:vm-100-disk-0
  sizeHuman?: string;
}

// parseDisks walks a VM/CT raw_config dict and extracts movable disks/volumes.
// VM disk keys: scsi\d+ / virtio\d+ / ide\d+ / sata\d+. CT volume keys: rootfs / mp\d+.
// Lines with `media=cdrom` or a `none` reference are skipped — not movable.
function parseDisks(config: Record<string, unknown>, isVM: boolean): DiskOption[] {
  const diskKeyRe = isVM
    ? /^(?:scsi|virtio|ide|sata)\d+$/
    : /^(?:rootfs|mp\d+)$/;
  const out: DiskOption[] = [];
  for (const [key, raw] of Object.entries(config)) {
    if (!diskKeyRe.test(key)) continue;
    if (typeof raw !== 'string') continue;
    if (raw.includes('media=cdrom')) continue;
    const firstField = raw.split(',')[0];
    if (!firstField || firstField === 'none') continue;
    const colon = firstField.indexOf(':');
    if (colon <= 0) continue;
    const storage = firstField.slice(0, colon);
    const sizeMatch = raw.match(/(?:^|,)size=([^,]+)/);
    out.push({ key, storage, volid: firstField, sizeHuman: sizeMatch?.[1] });
  }
  out.sort((a, b) => a.key.localeCompare(b.key, undefined, { numeric: true }));
  return out;
}

export function MoveStorageDialog({ guest, onClose, onSuccess }: MoveStorageDialogProps) {
  const isVM = guest.type === 'qemu';
  const isRunning = guest.status === 'running';
  const ctBlocked = !isVM && isRunning; // PVE rejects online LXC volume moves

  const [disks, setDisks] = useState<DiskOption[]>([]);
  const [storages, setStorages] = useState<Storage[]>([]);
  const [selectedDisk, setSelectedDisk] = useState('');
  const [targetStorage, setTargetStorage] = useState('');
  const [deleteSource, setDeleteSource] = useState(true);
  const [format, setFormat] = useState('');
  const [configLoading, setConfigLoading] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [configResp, allStorages] = await Promise.all([
          isVM
            ? api.getVMConfig(guest.cluster, guest.vmid)
            : api.getContainerConfig(guest.cluster, guest.vmid),
          api.getStorage(guest.node),
        ]);
        if (cancelled) return;
        const raw = (configResp.config.raw_config ?? {}) as Record<string, unknown>;
        setDisks(parseDisks(raw, isVM));
        setStorages(allStorages.filter(s => s.cluster === guest.cluster));
      } catch (e) {
        if (!cancelled) {
          setError('Failed to load disks/storage: ' + (e instanceof Error ? e.message : String(e)));
        }
      } finally {
        if (!cancelled) setConfigLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [guest.cluster, guest.vmid, guest.node, isVM]);

  const currentStorage = disks.find(d => d.key === selectedDisk)?.storage ?? '';
  const contentNeeded = isVM ? 'images' : 'rootdir';

  const eligibleStorages = useMemo(() => {
    return storages.filter(s => {
      if (!s.active) return false;
      if (!s.content.split(',').map(c => c.trim()).includes(contentNeeded)) return false;
      if (currentStorage && s.storage === currentStorage) return false;
      return true;
    });
  }, [storages, currentStorage, contentNeeded]);

  const handleSubmit = async () => {
    if (ctBlocked) {
      setError('Container must be stopped before moving its volume.');
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const req = {
        disk: selectedDisk,
        target_storage: targetStorage,
        delete_source: deleteSource,
        from_storage: currentStorage,
        ...(isVM && format ? { format } : {}),
      };
      if (isVM) {
        await api.moveVMDisk(guest.cluster, guest.vmid, req);
      } else {
        await api.moveContainerVolume(guest.cluster, guest.vmid, req);
      }
      onSuccess();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Move failed');
    } finally {
      setLoading(false);
    }
  };

  const canSubmit = !loading && !ctBlocked && !!selectedDisk && !!targetStorage;

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4">
          Move Storage: {guest.name}
        </h2>

        {configLoading ? (
          <div className="text-sm text-gray-500 py-4">Loading disks…</div>
        ) : (
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                {isVM ? 'Disk' : 'Volume'}
              </label>
              <select
                value={selectedDisk}
                onChange={(e) => {
                  setSelectedDisk(e.target.value);
                  setTargetStorage('');
                }}
                disabled={loading || disks.length === 0}
                className="w-full border border-gray-300 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                <option value="">Select {isVM ? 'disk' : 'volume'}…</option>
                {disks.map(d => (
                  <option key={d.key} value={d.key}>
                    {d.key} — {d.volid}{d.sizeHuman ? ` (${d.sizeHuman})` : ''}
                  </option>
                ))}
              </select>
              {!configLoading && disks.length === 0 && (
                <p className="text-sm text-amber-600 mt-1">
                  No movable {isVM ? 'disks' : 'volumes'} found on this guest.
                </p>
              )}
            </div>

            {currentStorage && (
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  From Storage
                </label>
                <div className="text-sm text-gray-600 bg-gray-100 px-3 py-2 rounded">
                  {currentStorage}
                </div>
              </div>
            )}

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Target Storage
              </label>
              <select
                value={targetStorage}
                onChange={(e) => setTargetStorage(e.target.value)}
                disabled={loading || !selectedDisk}
                className="w-full border border-gray-300 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                <option value="">Select target storage…</option>
                {eligibleStorages.map(s => (
                  <option key={s.storage} value={s.storage}>
                    {s.storage} ({formatBytes(s.avail)} free)
                  </option>
                ))}
              </select>
              {selectedDisk && eligibleStorages.length === 0 && (
                <p className="text-sm text-amber-600 mt-1">
                  No eligible target storage — must be active and accept {contentNeeded}.
                </p>
              )}
            </div>

            {isVM && (
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Format <span className="text-gray-500 text-xs">(optional)</span>
                </label>
                <select
                  value={format}
                  onChange={(e) => setFormat(e.target.value)}
                  disabled={loading}
                  className="w-full border border-gray-300 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
                >
                  <option value="">Keep current / PVE default</option>
                  <option value="raw">raw</option>
                  <option value="qcow2">qcow2</option>
                  <option value="vmdk">vmdk</option>
                </select>
              </div>
            )}

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="move-delete-source"
                checked={deleteSource}
                onChange={(e) => setDeleteSource(e.target.checked)}
                disabled={loading}
                className="rounded border-gray-300"
              />
              <label htmlFor="move-delete-source" className="text-sm text-gray-700">
                Delete source after move
              </label>
            </div>

            {isVM && isRunning && (
              <p className="text-sm text-gray-500">
                VM is running — move will happen online with no downtime.
              </p>
            )}

            {ctBlocked && (
              <div className="bg-amber-50 text-amber-800 px-3 py-2 rounded text-sm">
                Container must be stopped before moving its volume (PVE does not support online LXC volume moves).
              </div>
            )}

            {error && (
              <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">
                {error}
              </div>
            )}
          </div>
        )}

        <div className="flex justify-end gap-3 mt-6">
          <button
            onClick={onClose}
            disabled={loading}
            className="px-4 py-2 text-gray-700 hover:bg-gray-100 rounded"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={!canSubmit}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Starting move…' : 'Move'}
          </button>
        </div>
      </div>
    </div>
  );
}
