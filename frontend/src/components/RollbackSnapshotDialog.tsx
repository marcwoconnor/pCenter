import { useState } from 'react';
import { api } from '../api/client';
import type { Guest, Snapshot } from '../types';

interface RollbackSnapshotDialogProps {
  guest: Guest;
  snapshot: Snapshot;
  onClose: () => void;
  onSuccess: () => void;
}

export function RollbackSnapshotDialog({ guest, snapshot, onClose, onSuccess }: RollbackSnapshotDialogProps) {
  const [confirmed, setConfirmed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isVM = guest.type === 'qemu';
  const hasRAMState = snapshot.vmstate === 1;

  const handleRollback = async () => {
    if (!confirmed) {
      setError('You must confirm the rollback');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      if (isVM) {
        await api.rollbackVMSnapshot(guest.cluster, guest.vmid, snapshot.name);
      } else {
        await api.rollbackContainerSnapshot(guest.cluster, guest.vmid, snapshot.name);
      }
      onSuccess();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to rollback snapshot');
    } finally {
      setLoading(false);
    }
  };

  const formatDate = (timestamp?: number) => {
    if (!timestamp) return 'Unknown';
    return new Date(timestamp * 1000).toLocaleString();
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4 text-amber-600">
          Rollback Snapshot
        </h2>

        <div className="space-y-4">
          {/* Warning */}
          <div className="bg-amber-50 border border-amber-200 rounded p-3">
            <p className="text-sm text-amber-800 font-medium">
              Warning: This is a destructive action
            </p>
            <p className="text-sm text-amber-700 mt-1">
              Rolling back to a snapshot will discard all current state and restore
              the {isVM ? 'VM' : 'container'} to the state at the time of the snapshot.
            </p>
          </div>

          {/* Snapshot details */}
          <div className="bg-gray-50 rounded p-3 space-y-2">
            <div className="flex justify-between text-sm">
              <span className="text-gray-600">Guest:</span>
              <span className="font-medium">{guest.name}</span>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-gray-600">Snapshot:</span>
              <span className="font-medium">{snapshot.name}</span>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-gray-600">Created:</span>
              <span>{formatDate(snapshot.snaptime)}</span>
            </div>
            {hasRAMState && (
              <div className="flex justify-between text-sm">
                <span className="text-gray-600">RAM State:</span>
                <span className="text-green-600">Included</span>
              </div>
            )}
            {snapshot.description && (
              <div className="text-sm pt-2 border-t">
                <span className="text-gray-600">Description:</span>
                <p className="mt-1">{snapshot.description}</p>
              </div>
            )}
          </div>

          {/* Confirmation checkbox */}
          <div className="flex items-start gap-2">
            <input
              type="checkbox"
              id="confirm-rollback"
              checked={confirmed}
              onChange={(e) => setConfirmed(e.target.checked)}
              disabled={loading}
              className="rounded border-gray-300 mt-0.5"
            />
            <label htmlFor="confirm-rollback" className="text-sm text-gray-700">
              I understand that this will revert all changes made after this snapshot
              and any unsaved data will be lost.
            </label>
          </div>

          {/* Error message */}
          {error && (
            <div className="bg-red-50 text-red-700 px-3 py-2 rounded text-sm">
              {error}
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-3 mt-6">
          <button
            onClick={onClose}
            disabled={loading}
            className="px-4 py-2 text-gray-700 hover:bg-gray-100 rounded"
          >
            Cancel
          </button>
          <button
            onClick={handleRollback}
            disabled={loading || !confirmed}
            className="px-4 py-2 bg-amber-600 text-white rounded hover:bg-amber-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Rolling back...' : 'Rollback'}
          </button>
        </div>
      </div>
    </div>
  );
}
