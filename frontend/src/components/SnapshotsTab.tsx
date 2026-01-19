import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';
import type { Guest, Snapshot } from '../types';
import { CreateSnapshotDialog } from './CreateSnapshotDialog';
import { RollbackSnapshotDialog } from './RollbackSnapshotDialog';

interface SnapshotsTabProps {
  guest: Guest;
}

export function SnapshotsTab({ guest }: SnapshotsTabProps) {
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  // Dialog state
  const [showCreate, setShowCreate] = useState(false);
  const [rollbackSnapshot, setRollbackSnapshot] = useState<Snapshot | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  const isVM = guest.type === 'qemu';

  const fetchSnapshots = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = isVM
        ? await api.getVMSnapshots(guest.cluster, guest.vmid)
        : await api.getContainerSnapshots(guest.cluster, guest.vmid);
      // Filter out 'current' pseudo-snapshot that Proxmox includes
      setSnapshots(data.filter(s => s.name !== 'current'));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load snapshots');
    } finally {
      setLoading(false);
    }
  }, [guest.cluster, guest.vmid, isVM]);

  useEffect(() => {
    fetchSnapshots();
  }, [fetchSnapshots]);

  const handleDelete = async (snapname: string) => {
    setActionLoading(snapname);
    try {
      if (isVM) {
        await api.deleteVMSnapshot(guest.cluster, guest.vmid, snapname);
      } else {
        await api.deleteContainerSnapshot(guest.cluster, guest.vmid, snapname);
      }
      setDeleteConfirm(null);
      // Optimistically remove from UI immediately
      setSnapshots(prev => prev.filter(s => s.name !== snapname));
      // Refresh after delay to sync with server
      setTimeout(fetchSnapshots, 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete snapshot');
    } finally {
      setActionLoading(null);
    }
  };

  const formatDate = (timestamp?: number) => {
    if (!timestamp) return '-';
    return new Date(timestamp * 1000).toLocaleString();
  };

  // Build tree structure from parent relationships
  const buildTree = (snaps: Snapshot[]): (Snapshot & { depth: number })[] => {
    const result: (Snapshot & { depth: number })[] = [];
    const byName = new Map(snaps.map(s => [s.name, s]));
    const visited = new Set<string>();

    const addWithDepth = (snap: Snapshot, depth: number) => {
      if (visited.has(snap.name)) return;
      visited.add(snap.name);
      result.push({ ...snap, depth });
      // Find children
      snaps
        .filter(s => s.parent === snap.name)
        .forEach(child => addWithDepth(child, depth + 1));
    };

    // Start from root snapshots (no parent or parent is 'current')
    snaps
      .filter(s => !s.parent || !byName.has(s.parent))
      .forEach(s => addWithDepth(s, 0));

    // Add any orphaned snapshots
    snaps.forEach(s => {
      if (!visited.has(s.name)) {
        result.push({ ...s, depth: 0 });
      }
    });

    return result;
  };

  const treeSnapshots = buildTree(snapshots);

  if (loading) {
    return (
      <div className="p-4 text-center text-gray-500 dark:text-gray-400">
        Loading snapshots...
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Header card */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <div className="flex justify-between items-center">
          <h3 className="font-medium text-gray-900 dark:text-white">Snapshots</h3>
          <button
            onClick={() => setShowCreate(true)}
            className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded hover:bg-blue-700"
          >
            Create Snapshot
          </button>
        </div>
      </div>

      {/* Error */}
      {error && (
        <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 px-4 py-3 rounded-lg text-sm">
          {error}
          <button
            onClick={() => setError(null)}
            className="ml-2 text-red-500 hover:text-red-700 dark:hover:text-red-300"
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Snapshot list */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
        {treeSnapshots.length === 0 ? (
          <div className="text-center py-8 text-gray-500 dark:text-gray-400">
            <p>No snapshots</p>
            <p className="text-sm mt-1">Create a snapshot to save the current state</p>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-700 border-b border-gray-200 dark:border-gray-600">
              <tr>
                <th className="text-left px-4 py-3 font-medium text-gray-700 dark:text-gray-300">Name</th>
                <th className="text-left px-4 py-3 font-medium text-gray-700 dark:text-gray-300">Created</th>
                <th className="text-left px-4 py-3 font-medium text-gray-700 dark:text-gray-300">Description</th>
                {isVM && (
                  <th className="text-center px-4 py-3 font-medium text-gray-700 dark:text-gray-300">RAM</th>
                )}
                <th className="text-right px-4 py-3 font-medium text-gray-700 dark:text-gray-300">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {treeSnapshots.map((snap) => (
                <tr key={snap.name} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                  <td className="px-4 py-3">
                    <span style={{ paddingLeft: `${snap.depth * 20}px` }} className="flex items-center gap-1">
                      {snap.depth > 0 && (
                        <span className="text-gray-400 dark:text-gray-500">└─</span>
                      )}
                      <span className="font-medium text-gray-900 dark:text-white">{snap.name}</span>
                    </span>
                  </td>
                  <td className="px-4 py-3 text-gray-600 dark:text-gray-400">
                    {formatDate(snap.snaptime)}
                  </td>
                  <td className="px-4 py-3 text-gray-600 dark:text-gray-400 max-w-xs truncate">
                    {snap.description || '-'}
                  </td>
                  {isVM && (
                    <td className="px-4 py-3 text-center">
                      {snap.vmstate === 1 ? (
                        <span className="inline-block px-2 py-0.5 bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 text-xs rounded">
                          Yes
                        </span>
                      ) : (
                        <span className="text-gray-400 dark:text-gray-500">-</span>
                      )}
                    </td>
                  )}
                  <td className="px-4 py-3 text-right">
                    <div className="flex justify-end gap-2">
                      <button
                        onClick={() => setRollbackSnapshot(snap)}
                        disabled={actionLoading === snap.name}
                        className="px-2 py-1 text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 rounded text-xs"
                        title="Rollback to this snapshot"
                      >
                        Rollback
                      </button>
                      {deleteConfirm === snap.name ? (
                        <span className="flex items-center gap-1">
                          <button
                            onClick={() => handleDelete(snap.name)}
                            disabled={actionLoading === snap.name}
                            className="px-2 py-1 bg-red-600 text-white rounded text-xs hover:bg-red-700 disabled:opacity-50"
                          >
                            {actionLoading === snap.name ? '...' : 'Confirm'}
                          </button>
                          <button
                            onClick={() => setDeleteConfirm(null)}
                            className="px-2 py-1 text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded text-xs"
                          >
                            Cancel
                          </button>
                        </span>
                      ) : (
                        <button
                          onClick={() => setDeleteConfirm(snap.name)}
                          disabled={actionLoading === snap.name}
                          className="px-2 py-1 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 rounded text-xs"
                          title="Delete snapshot"
                        >
                          Delete
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Create dialog */}
      {showCreate && (
        <CreateSnapshotDialog
          guest={guest}
          onClose={() => setShowCreate(false)}
          onSuccess={fetchSnapshots}
        />
      )}

      {/* Rollback dialog */}
      {rollbackSnapshot && (
        <RollbackSnapshotDialog
          guest={guest}
          snapshot={rollbackSnapshot}
          onClose={() => setRollbackSnapshot(null)}
          onSuccess={fetchSnapshots}
        />
      )}
    </div>
  );
}
