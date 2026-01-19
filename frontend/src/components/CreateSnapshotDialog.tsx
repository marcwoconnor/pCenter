import { useState } from 'react';
import { api } from '../api/client';
import type { Guest } from '../types';
import { useCluster } from '../context/ClusterContext';

interface CreateSnapshotDialogProps {
  guest: Guest;
  onClose: () => void;
  onSuccess: () => void;
}

export function CreateSnapshotDialog({ guest, onClose, onSuccess }: CreateSnapshotDialogProps) {
  const { addTask, updateTask } = useCluster();
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [includeRAM, setIncludeRAM] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isVM = guest.type === 'qemu';
  const isRunning = guest.status === 'running';

  const handleCreate = async () => {
    if (!name.trim()) {
      setError('Snapshot name is required');
      return;
    }

    // Validate name (alphanumeric, dash, underscore)
    if (!/^[a-zA-Z0-9_-]+$/.test(name)) {
      setError('Name must contain only letters, numbers, dashes, and underscores');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const req = {
        name: name.trim(),
        description: description.trim() || undefined,
        vmstate: isVM && isRunning ? includeRAM : undefined,
      };

      const taskId = `snap-create-${guest.vmid}-${Date.now()}`;

      if (isVM) {
        await api.createVMSnapshot(guest.cluster, guest.vmid, req);
      } else {
        await api.createContainerSnapshot(guest.cluster, guest.vmid, req);
      }

      // Add task to tasks bar
      addTask({
        id: taskId,
        type: 'Create Snapshot',
        status: 'running',
        target: `${guest.name}: ${name.trim()}`,
        startTime: Date.now(),
      });

      // Close immediately
      onClose();

      // Refresh list after delay and mark task complete
      setTimeout(() => {
        updateTask(taskId, { status: 'completed' });
        onSuccess();
      }, 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create snapshot');
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4">
          Create Snapshot: {guest.name}
        </h2>

        <div className="space-y-4">
          {/* Snapshot name */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Name <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g., before-update"
              className="w-full border border-gray-300 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
              disabled={loading}
              autoFocus
            />
            <p className="text-xs text-gray-500 mt-1">
              Letters, numbers, dashes, and underscores only
            </p>
          </div>

          {/* Description */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Description
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description..."
              rows={2}
              className="w-full border border-gray-300 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
              disabled={loading}
            />
          </div>

          {/* Include RAM (VM only, running only) */}
          {isVM && isRunning && (
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="include-ram"
                checked={includeRAM}
                onChange={(e) => setIncludeRAM(e.target.checked)}
                disabled={loading}
                className="rounded border-gray-300"
              />
              <label htmlFor="include-ram" className="text-sm text-gray-700">
                Include RAM state
              </label>
              <span className="text-xs text-gray-500">(larger snapshot, faster restore)</span>
            </div>
          )}

          {/* Status info */}
          {!isRunning && (
            <p className="text-sm text-gray-500 bg-gray-50 px-3 py-2 rounded">
              Guest is stopped - snapshot will include disk state only
            </p>
          )}

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
            className="px-4 py-2 text-gray-700 hover:bg-gray-100 rounded disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={handleCreate}
            disabled={loading || !name.trim()}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Creating...' : 'Create Snapshot'}
          </button>
        </div>
      </div>
    </div>
  );
}
