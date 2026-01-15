import { useState, useEffect } from 'react';
import { api } from '../api/client';
import type { Storage, StorageVolume } from '../types';

interface CreateVMDialogProps {
  cluster: string;
  node: string;
  onClose: () => void;
  onSuccess: () => void;
}

export function CreateVMDialog({ cluster, node, onClose, onSuccess }: CreateVMDialogProps) {
  // Form state
  const [vmid, setVmid] = useState<number>(0);
  const [name, setName] = useState('');
  const [cores, setCores] = useState(2);
  const [memory, setMemory] = useState(2048);
  const [storage, setStorage] = useState('');
  const [diskSize, setDiskSize] = useState(32);
  const [iso, setIso] = useState('');
  const [ostype, setOstype] = useState('l26');
  const [startAfter, setStartAfter] = useState(false);

  // Data state
  const [storageList, setStorageList] = useState<Storage[]>([]);
  const [isoList, setIsoList] = useState<StorageVolume[]>([]);
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

        // Get storage (filter for images content)
        const allStorage = await api.getStorage(node);
        const imageStorage = allStorage.filter(s =>
          s.content.includes('images') && s.active === 1
        );
        setStorageList(imageStorage);
        if (imageStorage.length > 0) {
          setStorage(imageStorage[0].storage);
        }

        // Get ISOs from all storage that supports iso content
        const isoStorage = allStorage.filter(s => s.content.includes('iso'));
        const allIsos: StorageVolume[] = [];
        for (const st of isoStorage) {
          try {
            const content = await api.getStorageContent(st.storage, node);
            const isos = content.filter(v => v.content === 'iso');
            allIsos.push(...isos);
          } catch {
            // Ignore errors for individual storage
          }
        }
        setIsoList(allIsos);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load data');
      } finally {
        setLoadingData(false);
      }
    };

    fetchData();
  }, [cluster, node]);

  const handleCreate = async () => {
    if (!name.trim()) {
      setError('Name is required');
      return;
    }
    if (!storage) {
      setError('Storage is required');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      await api.createVM(cluster, node, {
        vmid,
        name: name.trim(),
        cores,
        memory,
        storage,
        disk_size: diskSize,
        iso: iso || undefined,
        ostype,
        start: startAfter,
      });
      onSuccess();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create VM');
    } finally {
      setLoading(false);
    }
  };

  const OS_TYPES = [
    { value: 'l26', label: 'Linux 2.6+' },
    { value: 'l24', label: 'Linux 2.4' },
    { value: 'win11', label: 'Windows 11' },
    { value: 'win10', label: 'Windows 10' },
    { value: 'win8', label: 'Windows 8' },
    { value: 'win7', label: 'Windows 7' },
    { value: 'other', label: 'Other' },
  ];

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-white">
          Create Virtual Machine
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
                VM ID
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

            {/* Name */}
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Name
              </label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="my-vm"
                className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                disabled={loading}
              />
            </div>

            {/* Resources row */}
            <div className="grid grid-cols-2 gap-4">
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
                  onChange={(e) => setMemory(parseInt(e.target.value) || 512)}
                  className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                  disabled={loading}
                  min={128}
                  step={128}
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
                  onChange={(e) => setDiskSize(parseInt(e.target.value) || 8)}
                  className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                  disabled={loading}
                  min={1}
                />
              </div>
            </div>

            {/* OS Type */}
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                OS Type
              </label>
              <select
                value={ostype}
                onChange={(e) => setOstype(e.target.value)}
                className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                disabled={loading}
              >
                {OS_TYPES.map((os) => (
                  <option key={os.value} value={os.value}>
                    {os.label}
                  </option>
                ))}
              </select>
            </div>

            {/* ISO */}
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                ISO Image (optional)
              </label>
              <select
                value={iso}
                onChange={(e) => setIso(e.target.value)}
                className="w-full border border-gray-300 dark:border-gray-600 rounded px-3 py-2 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:bg-gray-700 dark:text-white"
                disabled={loading}
              >
                <option value="">None</option>
                {isoList.map((i) => (
                  <option key={i.volid} value={i.volid}>
                    {i.volid.split('/').pop()}
                  </option>
                ))}
              </select>
            </div>

            {/* Start after creation */}
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="start-after"
                checked={startAfter}
                onChange={(e) => setStartAfter(e.target.checked)}
                disabled={loading}
                className="rounded border-gray-300 dark:border-gray-600"
              />
              <label htmlFor="start-after" className="text-sm text-gray-700 dark:text-gray-300">
                Start after creation
              </label>
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
            disabled={loading || loadingData || !name.trim() || !storage}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? 'Creating...' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  );
}
