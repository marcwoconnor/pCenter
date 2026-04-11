import { useState, useEffect } from 'react';
import { api } from '../api/client';
import type { Guest } from '../types';

interface MigrateDialogProps {
  guest: Guest;
  onClose: () => void;
  onSuccess: () => void;
}

export function MigrateDialog({ guest, onClose, onSuccess }: MigrateDialogProps) {
  const [nodes, setNodes] = useState<{ name: string; online: boolean }[]>([]);
  const [targetNode, setTargetNode] = useState('');
  const [online, setOnline] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // Fetch available nodes
    api.getMigrationTargets(guest.cluster).then(setNodes).catch(console.error);
  }, [guest.cluster]);

  const handleMigrate = async () => {
    if (!targetNode) {
      setError('Please select a target node');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      if (guest.type === 'qemu') {
        await api.clusterMigrateVM(guest.cluster, guest.vmid, targetNode, online);
      } else {
        await api.clusterMigrateContainer(guest.cluster, guest.vmid, targetNode, online);
      }
      onSuccess();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Migration failed');
    } finally {
      setLoading(false);
    }
  };

  const availableNodes = nodes.filter(n => n.name !== guest.node && n.online);
  const isVMRunning = guest.status === 'running';

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4">
          Migrate {guest.type === 'qemu' ? 'VM' : 'Container'}: {guest.name}
        </h2>

        <div className="space-y-4">
          {/* Current location */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Current Node
            </label>
            <div className="text-sm text-gray-600 bg-gray-100 px-3 py-2 rounded">
              {guest.node}
            </div>
          </div>

          {/* Target node selection */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Target Node
            </label>
            <select
              value={targetNode}
              onChange={(e) => setTargetNode(e.target.value)}
              className="w-full border border-gray-300 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500"
              disabled={loading}
            >
              <option value="">Select target node...</option>
              {availableNodes.map((node) => (
                <option key={node.name} value={node.name}>
                  {node.name}
                </option>
              ))}
            </select>
            {availableNodes.length === 0 && (
              <p className="text-sm text-amber-600 mt-1">
                No other online nodes available for migration
              </p>
            )}
          </div>

          {/* Live migration option */}
          {isVMRunning && (
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="online-migration"
                checked={online}
                onChange={(e) => setOnline(e.target.checked)}
                disabled={loading}
                className="rounded border-gray-300"
              />
              <label htmlFor="online-migration" className="text-sm text-gray-700">
                Live migration (no downtime)
              </label>
            </div>
          )}

          {!isVMRunning && (
            <p className="text-sm text-gray-500">
              Guest is stopped - will perform offline migration
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
            className="px-4 py-2 text-gray-700 hover:bg-gray-100 rounded"
          >
            Cancel
          </button>
          <button
            onClick={handleMigrate}
            disabled={loading || !targetNode || availableNodes.length === 0}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Migrating...' : 'Migrate'}
          </button>
        </div>
      </div>
    </div>
  );
}
