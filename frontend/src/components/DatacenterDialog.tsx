import { useState, useEffect, useRef } from 'react';
import { api } from '../api/client';
import type { Datacenter, InventoryCluster } from '../types';

interface DatacenterDialogProps {
  mode: 'create-dc' | 'edit-dc' | 'create-cluster' | 'edit-cluster';
  datacenter?: Datacenter;
  cluster?: InventoryCluster;
  parentDatacenterId?: string;
  datacenters: Datacenter[];
  // For create-dc / create-cluster, `created` is the newly-created entity so callers can chain follow-up actions.
  onSubmit: (created?: Datacenter | InventoryCluster) => Promise<void>;
  onClose: () => void;
}

export function DatacenterDialog({
  mode,
  datacenter,
  cluster,
  parentDatacenterId,
  datacenters,
  onSubmit,
  onClose,
}: DatacenterDialogProps) {
  const isDatacenter = mode === 'create-dc' || mode === 'edit-dc';
  const isCreate = mode === 'create-dc' || mode === 'create-cluster';

  // Datacenter fields
  const [dcName, setDcName] = useState(datacenter?.name || '');
  const [dcDescription, setDcDescription] = useState(datacenter?.description || '');

  // Cluster fields
  const [clusterName, setClusterName] = useState(cluster?.name || '');
  const [clusterDcId, setClusterDcId] = useState(cluster?.datacenter_id || parentDatacenterId || '');
  const [clusterEnabled, setClusterEnabled] = useState(cluster?.enabled ?? true);

  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSaving(true);

    try {
      let created: Datacenter | InventoryCluster | undefined;
      if (isDatacenter) {
        const name = dcName.trim();
        if (!name) {
          setError('Name is required');
          setSaving(false);
          return;
        }

        if (isCreate) {
          created = await api.createDatacenter({ name, description: dcDescription.trim() || undefined });
        } else if (datacenter) {
          await api.updateDatacenter(datacenter.id, { name, description: dcDescription.trim() || undefined });
        }
      } else {
        // Cluster - just name, datacenter, and enabled
        const name = clusterName.trim();
        if (!name) {
          setError('Name is required');
          setSaving(false);
          return;
        }

        if (isCreate) {
          created = await api.createInventoryCluster({
            name,
            datacenter_id: clusterDcId || undefined,
          });
        } else if (cluster) {
          await api.updateInventoryCluster(cluster.name, {
            name,
            datacenter_id: clusterDcId || undefined,
            enabled: clusterEnabled,
          });
        }
      }

      await onSubmit(created);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Operation failed');
      setSaving(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose();
    }
  };

  const title = {
    'create-dc': 'New Datacenter',
    'edit-dc': 'Edit Datacenter',
    'create-cluster': 'New Cluster',
    'edit-cluster': 'Edit Cluster',
  }[mode];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-4 min-w-[400px] max-w-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="text-lg font-medium text-gray-900 dark:text-gray-100 mb-4">
          {title}
        </h3>
        <form onSubmit={handleSubmit} onKeyDown={handleKeyDown}>
          {isDatacenter ? (
            <>
              <div className="mb-3">
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Name
                </label>
                <input
                  ref={inputRef}
                  type="text"
                  value={dcName}
                  onChange={(e) => setDcName(e.target.value)}
                  placeholder="Datacenter name"
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
              <div className="mb-3">
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Description
                </label>
                <input
                  type="text"
                  value={dcDescription}
                  onChange={(e) => setDcDescription(e.target.value)}
                  placeholder="Optional description"
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
            </>
          ) : (
            <>
              <div className="mb-3">
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Cluster Name
                </label>
                <input
                  ref={inputRef}
                  type="text"
                  value={clusterName}
                  onChange={(e) => setClusterName(e.target.value)}
                  placeholder="e.g., production"
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
              <div className="mb-3">
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Datacenter
                </label>
                <select
                  value={clusterDcId}
                  onChange={(e) => setClusterDcId(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
                >
                  <option value="">(None - Orphan)</option>
                  {datacenters.map((dc) => (
                    <option key={dc.id} value={dc.id}>
                      {dc.name}
                    </option>
                  ))}
                </select>
              </div>
              {!isCreate && (
                <div className="mb-3">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={clusterEnabled}
                      onChange={(e) => setClusterEnabled(e.target.checked)}
                      className="rounded border-gray-300 dark:border-gray-600"
                    />
                    <span className="text-sm text-gray-700 dark:text-gray-300">
                      Enabled
                    </span>
                  </label>
                  <p className="text-xs text-gray-500 mt-1">
                    Disabled clusters are not polled
                  </p>
                </div>
              )}
              {isCreate && (
                <p className="text-xs text-gray-500 mb-3">
                  After creating the cluster, add hosts by dragging them into the cluster in the tree.
                </p>
              )}
            </>
          )}

          {error && (
            <p className="text-red-500 text-sm mb-3">{error}</p>
          )}

          <div className="flex justify-end gap-2 mt-4">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving}
              className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
            >
              {saving ? 'Saving...' : isCreate ? 'Create' : 'Save'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
