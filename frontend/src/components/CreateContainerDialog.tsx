import { useState, useEffect } from 'react';
import { api } from '../api/client';
import type { Storage, StorageVolume } from '../types';

interface CreateContainerDialogProps {
  cluster: string;
  node: string;
  onClose: () => void;
  onSuccess: () => void;
}

export function CreateContainerDialog({ cluster, node, onClose, onSuccess }: CreateContainerDialogProps) {
  // Form state
  const [vmid, setVmid] = useState<number>(0);
  const [hostname, setHostname] = useState('');
  const [template, setTemplate] = useState('');
  const [cores, setCores] = useState(1);
  const [memory, setMemory] = useState(512);
  const [swap, setSwap] = useState(512);
  const [storage, setStorage] = useState('');
  const [diskSize, setDiskSize] = useState(8);
  const [password, setPassword] = useState('');
  const [unprivileged, setUnprivileged] = useState(true);
  const [startAfter, setStartAfter] = useState(false);

  // Data state
  const [storageList, setStorageList] = useState<Storage[]>([]);
  const [templateList, setTemplateList] = useState<StorageVolume[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadingData, setLoadingData] = useState(true);

  // Fetch initial data
  useEffect(() => {
    const fetchData = async () => {
      try {
        // Get next VMID
        const { vmid: nextId } = await api.getNextVMID(cluster);
        setVmid(nextId);

        // Get storage (filter for rootdir content - for container root filesystems)
        const allStorage = await api.getStorage(node);
        const rootStorage = allStorage.filter(s =>
          s.content.includes('rootdir') && s.active === 1
        );
        setStorageList(rootStorage);
        if (rootStorage.length > 0) {
          setStorage(rootStorage[0].storage);
        }

        // Get templates from all storage that supports vztmpl content
        const tmplStorage = allStorage.filter(s => s.content.includes('vztmpl'));
        const allTemplates: StorageVolume[] = [];
        for (const st of tmplStorage) {
          try {
            const content = await api.getStorageContent(st.storage, node);
            const templates = content.filter(v => v.content === 'vztmpl');
            allTemplates.push(...templates);
          } catch {
            // Ignore errors for individual storage
          }
        }
        setTemplateList(allTemplates);
        if (allTemplates.length > 0) {
          setTemplate(allTemplates[0].volid);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load data');
      } finally {
        setLoadingData(false);
      }
    };

    fetchData();
  }, [cluster, node]);

  const handleCreate = async () => {
    if (!hostname.trim()) {
      setError('Hostname is required');
      return;
    }
    if (!template) {
      setError('Template is required');
      return;
    }
    if (!storage) {
      setError('Storage is required');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      await api.createContainer(cluster, node, {
        vmid,
        hostname: hostname.trim(),
        ostemplate: template,
        cores,
        memory,
        swap,
        storage,
        disk_size: diskSize,
        password: password || undefined,
        unprivileged,
        start: startAfter,
      });
      onSuccess();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create container');
    } finally {
      setLoading(false);
    }
  };

  // Extract template name for display
  const getTemplateName = (volid: string) => {
    const parts = volid.split('/');
    return parts[parts.length - 1];
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md p-6 max-h-[90vh] overflow-y-auto">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-white">
          Create Container
        </h2>

        {loadingData ? (
          <div className="text-center py-8 text-gray-500">Loading...</div>
        ) : (
          <div className="space-y-4">
            {/* Node */}
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Node
              </label>
              <div className="text-sm text-gray-600 dark:text-gray-400 bg-gray-100 dark:bg-gray-700 px-3 py-2 rounded">
                {node}
              </div>
            </div>

            {/* VMID */}
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                CT ID
              </label>
              <input
                type="number"
                value={vmid}
                onChange={(e) => setVmid(parseInt(e.target.value) || 0)}
                className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                disabled={loading}
                min={100}
              />
            </div>

            {/* Hostname */}
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Hostname
              </label>
              <input
                type="text"
                value={hostname}
                onChange={(e) => setHostname(e.target.value)}
                placeholder="my-container"
                className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                disabled={loading}
              />
            </div>

            {/* Template */}
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Template
              </label>
              {templateList.length === 0 ? (
                <div className="text-sm text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-900/20 px-3 py-2 rounded">
                  No templates available. Upload a template first.
                </div>
              ) : (
                <select
                  value={template}
                  onChange={(e) => setTemplate(e.target.value)}
                  className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                  disabled={loading}
                >
                  {templateList.map((t) => (
                    <option key={t.volid} value={t.volid}>
                      {getTemplateName(t.volid)}
                    </option>
                  ))}
                </select>
              )}
            </div>

            {/* Resources row */}
            <div className="grid grid-cols-3 gap-3">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Cores
                </label>
                <input
                  type="number"
                  value={cores}
                  onChange={(e) => setCores(parseInt(e.target.value) || 1)}
                  className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                  disabled={loading}
                  min={1}
                  max={128}
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Memory (MB)
                </label>
                <input
                  type="number"
                  value={memory}
                  onChange={(e) => setMemory(parseInt(e.target.value) || 256)}
                  className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                  disabled={loading}
                  min={64}
                  step={64}
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Swap (MB)
                </label>
                <input
                  type="number"
                  value={swap}
                  onChange={(e) => setSwap(parseInt(e.target.value) || 0)}
                  className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                  disabled={loading}
                  min={0}
                  step={64}
                />
              </div>
            </div>

            {/* Storage row */}
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Storage
                </label>
                <select
                  value={storage}
                  onChange={(e) => setStorage(e.target.value)}
                  className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                  disabled={loading}
                >
                  {storageList.map((s) => (
                    <option key={s.storage} value={s.storage}>
                      {s.storage}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Disk Size (GB)
                </label>
                <input
                  type="number"
                  value={diskSize}
                  onChange={(e) => setDiskSize(parseInt(e.target.value) || 4)}
                  className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                  disabled={loading}
                  min={1}
                />
              </div>
            </div>

            {/* Password */}
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Root Password (optional)
              </label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Leave blank for no password"
                className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                disabled={loading}
              />
            </div>

            {/* Checkboxes */}
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="unprivileged"
                  checked={unprivileged}
                  onChange={(e) => setUnprivileged(e.target.checked)}
                  disabled={loading}
                  className="rounded border-gray-300 dark:border-gray-600"
                />
                <label htmlFor="unprivileged" className="text-sm text-gray-700 dark:text-gray-300">
                  Unprivileged container (recommended)
                </label>
              </div>
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="start-after-ct"
                  checked={startAfter}
                  onChange={(e) => setStartAfter(e.target.checked)}
                  disabled={loading}
                  className="rounded border-gray-300 dark:border-gray-600"
                />
                <label htmlFor="start-after-ct" className="text-sm text-gray-700 dark:text-gray-300">
                  Start after creation
                </label>
              </div>
            </div>

            {/* Error */}
            {error && (
              <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 px-3 py-2 rounded text-sm">
                {error}
              </div>
            )}
          </div>
        )}

        {/* Actions */}
        <div className="flex justify-end gap-3 mt-6">
          <button
            onClick={onClose}
            disabled={loading}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded"
          >
            Cancel
          </button>
          <button
            onClick={handleCreate}
            disabled={loading || loadingData || !hostname.trim() || !template || !storage}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Creating...' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  );
}
