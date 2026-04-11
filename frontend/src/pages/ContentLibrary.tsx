import { useState, useEffect, useCallback } from 'react';
import { Layout } from '../components/Layout';
import { useCluster } from '../context/ClusterContext';
import { api, formatBytes } from '../api/client';
import type { LibraryItem, LibraryItemType, CreateLibraryItemRequest, StorageVolume } from '../types';

const TYPE_LABELS: Record<LibraryItemType, string> = {
  'iso': 'ISO Image',
  'vztmpl': 'CT Template',
  'vm-template': 'VM Template',
  'ova': 'OVA/OVF',
  'snippet': 'Snippet',
};

const TYPE_ICONS: Record<LibraryItemType, string> = {
  'iso': '💿',
  'vztmpl': '📦',
  'vm-template': '🖥',
  'ova': '📁',
  'snippet': '📝',
};

const ALL_TYPES: LibraryItemType[] = ['iso', 'vztmpl', 'vm-template', 'ova', 'snippet'];

export function ContentLibrary() {
  const { clusters, nodes, storage } = useCluster();
  const [items, setItems] = useState<LibraryItem[]>([]);
  const [categories, setCategories] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState<string>('');
  const [categoryFilter, setCategoryFilter] = useState('');
  const [clusterFilter, setClusterFilter] = useState('');

  // Dialogs
  const [showAddDialog, setShowAddDialog] = useState(false);
  const [showDeployDialog, setShowDeployDialog] = useState<LibraryItem | null>(null);
  const [editingItem, setEditingItem] = useState<LibraryItem | null>(null);

  const fetchItems = useCallback(async () => {
    try {
      setError(null);
      const params: Record<string, string> = {};
      if (typeFilter) params.type = typeFilter;
      if (categoryFilter) params.category = categoryFilter;
      if (clusterFilter) params.cluster = clusterFilter;
      if (search) params.search = search;
      const data = await api.getLibraryItems(params);
      setItems(data);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [typeFilter, categoryFilter, clusterFilter, search]);

  const fetchCategories = useCallback(async () => {
    try {
      const cats = await api.getLibraryCategories();
      setCategories(cats);
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    fetchItems();
    fetchCategories();
  }, [fetchItems, fetchCategories]);

  const handleDelete = async (item: LibraryItem) => {
    if (!confirm(`Remove "${item.name}" from the content library?\n\nThis only removes the catalog entry — the actual file on storage is not deleted.`)) return;
    try {
      await api.deleteLibraryItem(item.id);
      fetchItems();
    } catch (err) {
      alert('Delete failed: ' + (err as Error).message);
    }
  };

  const handleDeploy = async (item: LibraryItem, targetCluster: string, targetNode: string, newName: string, newVMID: number, full: boolean) => {
    try {
      const result = await api.deployLibraryItem(item.id, {
        target_cluster: targetCluster,
        target_node: targetNode,
        new_name: newName,
        new_vmid: newVMID,
        full,
      });
      alert(`Deploy started: ${result.message}`);
      setShowDeployDialog(null);
    } catch (err) {
      alert('Deploy failed: ' + (err as Error).message);
    }
  };

  const clusterNames = clusters.map(c => c.name);

  return (
    <Layout>
      <div className="flex-1 overflow-auto p-4">
        <div className="flex items-center justify-between mb-4">
          <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Content Library</h1>
          <button
            onClick={() => setShowAddDialog(true)}
            className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            + Add Item
          </button>
        </div>

        {/* Filters */}
        <div className="flex flex-wrap gap-2 mb-4">
          <input
            type="text"
            placeholder="Search..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200 w-48"
          />
          <select
            value={typeFilter}
            onChange={(e) => setTypeFilter(e.target.value)}
            className="px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
          >
            <option value="">All Types</option>
            {ALL_TYPES.map(t => (
              <option key={t} value={t}>{TYPE_LABELS[t]}</option>
            ))}
          </select>
          <select
            value={categoryFilter}
            onChange={(e) => setCategoryFilter(e.target.value)}
            className="px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
          >
            <option value="">All Categories</option>
            {categories.map(c => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
          <select
            value={clusterFilter}
            onChange={(e) => setClusterFilter(e.target.value)}
            className="px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
          >
            <option value="">All Clusters</option>
            {clusterNames.map(c => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
          {(search || typeFilter || categoryFilter || clusterFilter) && (
            <button
              onClick={() => { setSearch(''); setTypeFilter(''); setCategoryFilter(''); setClusterFilter(''); }}
              className="px-2 py-1 text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400"
            >
              Clear
            </button>
          )}
        </div>

        {error && (
          <div className="mb-4 p-3 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300 rounded text-sm">
            {error}
          </div>
        )}

        {/* Items Table */}
        {loading ? (
          <div className="text-center py-8 text-gray-500">Loading...</div>
        ) : items.length === 0 ? (
          <div className="text-center py-12 text-gray-500 dark:text-gray-400">
            <div className="text-4xl mb-3">📚</div>
            <p className="text-lg mb-1">Content Library is empty</p>
            <p className="text-sm">Add ISOs, templates, and VM images to share across clusters.</p>
          </div>
        ) : (
          <div className="border rounded dark:border-gray-700 overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-gray-50 dark:bg-gray-800 text-left">
                  <th className="px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Name</th>
                  <th className="px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Type</th>
                  <th className="px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Category</th>
                  <th className="px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Cluster</th>
                  <th className="px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Storage</th>
                  <th className="px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Size</th>
                  <th className="px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Version</th>
                  <th className="px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Tags</th>
                  <th className="px-3 py-2 font-medium text-gray-600 dark:text-gray-300 w-24">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {items.map((item) => (
                  <tr key={item.id} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                    <td className="px-3 py-2">
                      <div className="flex items-center gap-2">
                        <span>{TYPE_ICONS[item.type] || '📄'}</span>
                        <div>
                          <div className="font-medium text-gray-900 dark:text-gray-100">{item.name}</div>
                          {item.description && (
                            <div className="text-xs text-gray-500 dark:text-gray-400 truncate max-w-xs">{item.description}</div>
                          )}
                        </div>
                      </div>
                    </td>
                    <td className="px-3 py-2 text-gray-600 dark:text-gray-300">{TYPE_LABELS[item.type]}</td>
                    <td className="px-3 py-2 text-gray-600 dark:text-gray-300">{item.category || '-'}</td>
                    <td className="px-3 py-2 text-gray-600 dark:text-gray-300">{item.cluster}</td>
                    <td className="px-3 py-2 text-gray-600 dark:text-gray-300">{item.storage}</td>
                    <td className="px-3 py-2 text-gray-600 dark:text-gray-300">{item.size ? formatBytes(item.size) : '-'}</td>
                    <td className="px-3 py-2 text-gray-600 dark:text-gray-300">{item.version || '-'}</td>
                    <td className="px-3 py-2">
                      {item.tags?.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {item.tags.map(tag => (
                            <span key={tag} className="px-1.5 py-0.5 text-xs bg-blue-100 dark:bg-blue-900/40 text-blue-700 dark:text-blue-300 rounded">
                              {tag}
                            </span>
                          ))}
                        </div>
                      ) : '-'}
                    </td>
                    <td className="px-3 py-2">
                      <div className="flex gap-1">
                        {item.type === 'vm-template' && (
                          <button
                            onClick={() => setShowDeployDialog(item)}
                            className="px-2 py-0.5 text-xs bg-green-600 text-white rounded hover:bg-green-700"
                            title="Deploy from template"
                          >
                            Deploy
                          </button>
                        )}
                        <button
                          onClick={() => setEditingItem(item)}
                          className="px-2 py-0.5 text-xs bg-gray-200 dark:bg-gray-600 text-gray-700 dark:text-gray-200 rounded hover:bg-gray-300 dark:hover:bg-gray-500"
                        >
                          Edit
                        </button>
                        <button
                          onClick={() => handleDelete(item)}
                          className="px-2 py-0.5 text-xs bg-red-100 dark:bg-red-900/40 text-red-600 dark:text-red-300 rounded hover:bg-red-200 dark:hover:bg-red-900/60"
                        >
                          Del
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Summary */}
        {items.length > 0 && (
          <div className="mt-2 text-xs text-gray-500 dark:text-gray-400">
            {items.length} item{items.length !== 1 ? 's' : ''}
            {typeFilter && ` (filtered by ${TYPE_LABELS[typeFilter as LibraryItemType]})`}
          </div>
        )}
      </div>

      {/* Add Item Dialog */}
      {showAddDialog && (
        <AddItemDialog
          clusters={clusterNames}
          storage={storage}
          onClose={() => setShowAddDialog(false)}
          onSave={async (req) => {
            await api.createLibraryItem(req);
            setShowAddDialog(false);
            fetchItems();
            fetchCategories();
          }}
        />
      )}

      {/* Edit Item Dialog */}
      {editingItem && (
        <EditItemDialog
          item={editingItem}
          onClose={() => setEditingItem(null)}
          onSave={async (req) => {
            await api.updateLibraryItem(editingItem.id, req);
            setEditingItem(null);
            fetchItems();
            fetchCategories();
          }}
        />
      )}

      {/* Deploy Dialog */}
      {showDeployDialog && (
        <DeployDialog
          item={showDeployDialog}
          clusters={clusterNames}
          nodes={nodes}
          onClose={() => setShowDeployDialog(null)}
          onDeploy={handleDeploy}
        />
      )}
    </Layout>
  );
}

// --- Add Item Dialog ---

function AddItemDialog({ clusters, storage, onClose, onSave }: {
  clusters: string[];
  storage: { storage: string; node: string; cluster?: string; content: string }[];
  onClose: () => void;
  onSave: (req: CreateLibraryItemRequest) => Promise<void>;
}) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [type, setType] = useState<LibraryItemType>('iso');
  const [category, setCategory] = useState('');
  const [version, setVersion] = useState('');
  const [tags, setTags] = useState('');
  const [cluster, setCluster] = useState(clusters[0] || '');
  const [storageName, setStorageName] = useState('');
  const [volume, setVolume] = useState('');
  const [vmid, setVmid] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  // Get storage pools for selected cluster that match the content type
  const contentMap: Record<LibraryItemType, string> = {
    'iso': 'iso',
    'vztmpl': 'vztmpl',
    'vm-template': 'images',
    'ova': 'iso',
    'snippet': 'snippets',
  };

  const filteredStorage = storage.filter(s =>
    (!cluster || s.cluster === cluster) &&
    s.content?.includes(contentMap[type])
  );

  // Fetch volumes when storage is selected
  const [volumes, setVolumes] = useState<StorageVolume[]>([]);
  const [loadingVolumes, setLoadingVolumes] = useState(false);

  useEffect(() => {
    if (!storageName) {
      setVolumes([]);
      return;
    }
    setLoadingVolumes(true);
    api.getStorageContent(storageName)
      .then(setVolumes)
      .catch(() => setVolumes([]))
      .finally(() => setLoadingVolumes(false));
  }, [storageName]);

  const handleSubmit = async () => {
    if (!name || !cluster || !storageName) {
      setError('Name, cluster, and storage are required');
      return;
    }
    setSaving(true);
    setError('');
    try {
      await onSave({
        name,
        description: description || undefined,
        type,
        category: category || undefined,
        version: version || undefined,
        tags: tags ? tags.split(',').map(t => t.trim()).filter(Boolean) : undefined,
        cluster,
        storage: storageName,
        volume: volume || `${storageName}:${type}/${name}`,
        vmid: vmid ? parseInt(vmid) : undefined,
      });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <DialogOverlay onClose={onClose}>
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-lg p-6">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-gray-100">Add to Content Library</h2>

        {error && <div className="mb-3 p-2 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300 rounded text-sm">{error}</div>}

        <div className="space-y-3">
          <Field label="Name">
            <input value={name} onChange={e => setName(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" placeholder="e.g. Ubuntu 24.04 Server" />
          </Field>

          <Field label="Type">
            <select value={type} onChange={e => { setType(e.target.value as LibraryItemType); setStorageName(''); setVolume(''); }} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200">
              {ALL_TYPES.map(t => <option key={t} value={t}>{TYPE_LABELS[t]}</option>)}
            </select>
          </Field>

          <Field label="Cluster">
            <select value={cluster} onChange={e => { setCluster(e.target.value); setStorageName(''); }} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200">
              {clusters.map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          </Field>

          <Field label="Storage">
            <select value={storageName} onChange={e => { setStorageName(e.target.value); setVolume(''); }} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200">
              <option value="">Select storage...</option>
              {filteredStorage.map(s => (
                <option key={`${s.cluster}-${s.node}-${s.storage}`} value={s.storage}>
                  {s.storage} ({s.node})
                </option>
              ))}
            </select>
          </Field>

          {storageName && type !== 'vm-template' && (
            <Field label="Volume">
              {loadingVolumes ? (
                <div className="text-sm text-gray-500">Loading volumes...</div>
              ) : volumes.length > 0 ? (
                <select value={volume} onChange={e => {
                  setVolume(e.target.value);
                  if (!name) {
                    const vol = volumes.find(v => v.volid === e.target.value);
                    if (vol) setName(vol.volid.split('/').pop()?.replace(/\.[^.]+$/, '') || '');
                  }
                }} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200">
                  <option value="">Select or enter manually...</option>
                  {volumes.map(v => (
                    <option key={v.volid} value={v.volid}>
                      {v.volid.split('/').pop()} ({formatBytes(v.size)})
                    </option>
                  ))}
                </select>
              ) : (
                <input value={volume} onChange={e => setVolume(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" placeholder="e.g. local:iso/ubuntu.iso" />
              )}
            </Field>
          )}

          {type === 'vm-template' && (
            <Field label="Source VMID">
              <input value={vmid} onChange={e => setVmid(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" placeholder="e.g. 9000" type="number" />
            </Field>
          )}

          <Field label="Description">
            <input value={description} onChange={e => setDescription(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" placeholder="Optional description" />
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field label="Category">
              <input value={category} onChange={e => setCategory(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" placeholder="e.g. Linux" />
            </Field>
            <Field label="Version">
              <input value={version} onChange={e => setVersion(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" placeholder="e.g. 24.04" />
            </Field>
          </div>

          <Field label="Tags (comma-separated)">
            <input value={tags} onChange={e => setTags(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" placeholder="e.g. ubuntu, server, lts" />
          </Field>
        </div>

        <div className="flex justify-end gap-2 mt-5">
          <button onClick={onClose} className="px-3 py-1.5 text-sm border rounded dark:border-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700">
            Cancel
          </button>
          <button onClick={handleSubmit} disabled={saving} className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
            {saving ? 'Adding...' : 'Add to Library'}
          </button>
        </div>
      </div>
    </DialogOverlay>
  );
}

// --- Edit Item Dialog ---

function EditItemDialog({ item, onClose, onSave }: {
  item: LibraryItem;
  onClose: () => void;
  onSave: (req: { name?: string; description?: string; category?: string; version?: string; tags?: string[] }) => Promise<void>;
}) {
  const [name, setName] = useState(item.name);
  const [description, setDescription] = useState(item.description || '');
  const [category, setCategory] = useState(item.category || '');
  const [version, setVersion] = useState(item.version || '');
  const [tags, setTags] = useState(item.tags.join(', '));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async () => {
    setSaving(true);
    setError('');
    try {
      await onSave({
        name,
        description,
        category,
        version,
        tags: tags ? tags.split(',').map(t => t.trim()).filter(Boolean) : [],
      });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <DialogOverlay onClose={onClose}>
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-lg p-6">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-gray-100">Edit Library Item</h2>

        {error && <div className="mb-3 p-2 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300 rounded text-sm">{error}</div>}

        <div className="space-y-3">
          <Field label="Name">
            <input value={name} onChange={e => setName(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" />
          </Field>
          <Field label="Description">
            <input value={description} onChange={e => setDescription(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Category">
              <input value={category} onChange={e => setCategory(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" />
            </Field>
            <Field label="Version">
              <input value={version} onChange={e => setVersion(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" />
            </Field>
          </div>
          <Field label="Tags (comma-separated)">
            <input value={tags} onChange={e => setTags(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" />
          </Field>

          <div className="text-xs text-gray-500 dark:text-gray-400 mt-2">
            Source: {item.cluster} / {item.storage} / {item.volume}
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-5">
          <button onClick={onClose} className="px-3 py-1.5 text-sm border rounded dark:border-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700">
            Cancel
          </button>
          <button onClick={handleSubmit} disabled={saving} className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>
    </DialogOverlay>
  );
}

// --- Deploy Dialog (VM Templates) ---

function DeployDialog({ item, clusters, nodes, onClose, onDeploy }: {
  item: LibraryItem;
  clusters: string[];
  nodes: { node: string; cluster: string; status: string }[];
  onClose: () => void;
  onDeploy: (item: LibraryItem, cluster: string, node: string, name: string, vmid: number, full: boolean) => void;
}) {
  const [targetCluster, setTargetCluster] = useState(item.cluster);
  const [targetNode, setTargetNode] = useState('');
  const [newName, setNewName] = useState(item.name);
  const [newVMID, setNewVMID] = useState('');
  const [full, setFull] = useState(true);

  const availableNodes = nodes.filter(n => n.cluster === targetCluster && n.status === 'online');

  useEffect(() => {
    if (availableNodes.length > 0 && !targetNode) {
      setTargetNode(availableNodes[0].node);
    }
  }, [targetCluster, availableNodes.length]);

  return (
    <DialogOverlay onClose={onClose}>
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-gray-100">Deploy Template</h2>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-4">
          Clone "{item.name}" (VMID {item.vmid}) to create a new VM.
        </p>

        <div className="space-y-3">
          <Field label="Target Cluster">
            <select value={targetCluster} onChange={e => { setTargetCluster(e.target.value); setTargetNode(''); }} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200">
              {clusters.map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          </Field>

          <Field label="Target Node">
            <select value={targetNode} onChange={e => setTargetNode(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200">
              {availableNodes.length === 0 && <option value="">No online nodes</option>}
              {availableNodes.map(n => <option key={n.node} value={n.node}>{n.node}</option>)}
            </select>
          </Field>

          <Field label="New VM Name">
            <input value={newName} onChange={e => setNewName(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" />
          </Field>

          <Field label="New VMID">
            <input value={newVMID} onChange={e => setNewVMID(e.target.value)} className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200" type="number" placeholder="Auto-assign if empty" />
          </Field>

          <div className="flex items-center gap-2">
            <input type="checkbox" id="full-clone" checked={full} onChange={e => setFull(e.target.checked)} />
            <label htmlFor="full-clone" className="text-sm text-gray-700 dark:text-gray-300">Full clone (independent copy)</label>
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-5">
          <button onClick={onClose} className="px-3 py-1.5 text-sm border rounded dark:border-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700">
            Cancel
          </button>
          <button
            onClick={() => onDeploy(item, targetCluster, targetNode, newName, newVMID ? parseInt(newVMID) : 0, full)}
            disabled={!targetNode}
            className="px-3 py-1.5 text-sm bg-green-600 text-white rounded hover:bg-green-700 disabled:opacity-50"
          >
            Deploy
          </button>
        </div>
      </div>
    </DialogOverlay>
  );
}

// --- Shared Components ---

function DialogOverlay({ children, onClose }: { children: React.ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div onClick={e => e.stopPropagation()}>
        {children}
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs font-medium text-gray-600 dark:text-gray-400 mb-1">{label}</label>
      {children}
    </div>
  );
}
