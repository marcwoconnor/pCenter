import { useState, useEffect } from 'react';
import { api } from '../api/client';
import { useCluster } from '../context/ClusterContext';
import type { Guest, Storage } from '../types';

interface CloneDialogProps {
  guest: Guest;
  onClose: () => void;
  onSuccess: () => void;
}

export function CloneDialog({ guest, onClose, onSuccess }: CloneDialogProps) {
  const { startClone } = useCluster();
  const [nodes, setNodes] = useState<{ name: string; online: boolean }[]>([]);
  const [storages, setStorages] = useState<Storage[]>([]);
  const [newId, setNewId] = useState<number | null>(null);
  const [name, setName] = useState(`${guest.name}-clone`);
  const [targetNode, setTargetNode] = useState(guest.node);
  const [fullClone, setFullClone] = useState(true);
  const [storage, setStorage] = useState('');
  const [fetchingId, setFetchingId] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // Fetch available nodes and next VMID
    Promise.all([
      api.getMigrationTargets(guest.cluster),
      api.getNextVMID(guest.cluster),
      api.getStorage(),
    ])
      .then(([nodeList, { vmid }, storageList]) => {
        setNodes(nodeList);
        setNewId(vmid);
        setStorages(storageList.filter(s =>
          s.content.includes('images') || s.content.includes('rootdir')
        ));
        setFetchingId(false);
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : 'Failed to load data');
        setFetchingId(false);
      });
  }, [guest.cluster]);

  const handleClone = () => {
    if (!newId) {
      setError('Please enter a valid VM ID');
      return;
    }
    if (!name.trim()) {
      setError('Please enter a name');
      return;
    }

    // Start clone in background - tracked in Tasks bar
    startClone(guest, newId, name.trim(), {
      targetNode: targetNode !== guest.node ? targetNode : undefined,
      full: fullClone,
      storage: storage || undefined,
    });

    // Close immediately - progress shown in Tasks bar
    onSuccess();
    onClose();
  };

  const onlineNodes = nodes.filter(n => n.online);
  const isVM = guest.type === 'qemu';

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-gray-100">
          Clone {isVM ? 'VM' : 'Container'}: {guest.name}
        </h2>

        <div className="space-y-4">
          {/* Source info */}
          <div className="text-sm text-gray-600 dark:text-gray-400 bg-gray-100 dark:bg-gray-700 px-3 py-2 rounded">
            Source: {guest.vmid} on {guest.node}
          </div>

          {/* New VMID */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              New {isVM ? 'VM' : 'CT'} ID
            </label>
            <input
              type="number"
              value={newId ?? ''}
              onChange={(e) => setNewId(parseInt(e.target.value) || null)}
              disabled={fetchingId}
              className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
              placeholder={fetchingId ? 'Loading...' : 'Enter VM ID'}
            />
          </div>

          {/* Name */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              {isVM ? 'Name' : 'Hostname'}
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>

          {/* Target node */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Target Node
            </label>
            <select
              value={targetNode}
              onChange={(e) => setTargetNode(e.target.value)}
              className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              {onlineNodes.map((node) => (
                <option key={node.name} value={node.name}>
                  {node.name}{node.name === guest.node ? ' (current)' : ''}
                </option>
              ))}
            </select>
          </div>

          {/* Storage (optional) */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Storage <span className="text-gray-400 font-normal">(optional)</span>
            </label>
            <select
              value={storage}
              onChange={(e) => setStorage(e.target.value)}
              className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              <option value="">Same as source</option>
              {storages.map((s) => (
                <option key={s.storage} value={s.storage}>
                  {s.storage} ({s.type})
                </option>
              ))}
            </select>
          </div>

          {/* Full clone option */}
          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="full-clone"
              checked={fullClone}
              onChange={(e) => setFullClone(e.target.checked)}
              className="rounded border-gray-300 dark:border-gray-600"
            />
            <label htmlFor="full-clone" className="text-sm text-gray-700 dark:text-gray-300">
              Full clone (independent copy)
            </label>
          </div>
          <p className="text-xs text-gray-500 dark:text-gray-400 ml-6 -mt-2">
            {fullClone
              ? 'Creates independent disk - larger but standalone'
              : 'Linked clone - faster, uses less space, depends on source'}
          </p>

          {error && (
            <div className="bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400 px-3 py-2 rounded text-sm">
              {error}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button
            onClick={onClose}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
          >
            Cancel
          </button>
          <button
            onClick={handleClone}
            disabled={!newId || !name.trim()}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Clone
          </button>
        </div>
      </div>
    </div>
  );
}
