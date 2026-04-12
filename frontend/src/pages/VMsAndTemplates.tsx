import { useState, useMemo } from 'react';
import { Layout } from '../components/Layout';
import { InventoryTree } from '../components/InventoryTree';
import { ObjectDetail } from '../components/ObjectDetail';
import { ErrorBoundary } from '../components/ErrorBoundary';
import { useCluster } from '../context/ClusterContext';
import type { Guest } from '../types';

export function VMsAndTemplates() {
  const { selectedObject } = useCluster();
  const [filter, setFilter] = useState('');

  const errorKey = selectedObject
    ? `${selectedObject.type}-${selectedObject.id}-${selectedObject.cluster || ''}`
    : 'none';

  const sidebar = (
    <div className="flex flex-col h-full">
      <div className="p-2 border-b border-gray-200 dark:border-gray-700">
        <input
          type="text"
          placeholder="Search VMs/CTs..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
        />
      </div>
      <div className="flex-1 overflow-y-auto">
        <InventoryTree view="vms" filter={filter} />
      </div>
    </div>
  );

  return (
    <Layout sidebar={sidebar}>
      <div className="flex-1 overflow-auto">
        {selectedObject ? (
          <ErrorBoundary key={errorKey}>
            <ObjectDetail />
          </ErrorBoundary>
        ) : (
          <AllGuestsGrid filter={filter} />
        )}
      </div>
    </Layout>
  );
}

function AllGuestsGrid({ filter }: { filter: string }) {
  const { guests, setSelectedObject, tags, tagAssignments } = useCluster();
  const [sortBy, setSortBy] = useState<'name' | 'status' | 'node' | 'type'>('name');

  const filterLower = filter.toLowerCase();

  const sorted = useMemo(() => {
    let filtered = guests;
    if (filterLower) {
      filtered = guests.filter(g =>
        g.name.toLowerCase().includes(filterLower) ||
        String(g.vmid).includes(filterLower) ||
        g.node.toLowerCase().includes(filterLower)
      );
    }

    return [...filtered].sort((a, b) => {
      // Running always first
      if (a.status === 'running' && b.status !== 'running') return -1;
      if (a.status !== 'running' && b.status === 'running') return 1;

      switch (sortBy) {
        case 'status': return a.status.localeCompare(b.status);
        case 'node': return a.node.localeCompare(b.node);
        case 'type': return (a.type === 'qemu' ? 'VM' : 'CT').localeCompare(b.type === 'qemu' ? 'VM' : 'CT');
        default: return a.name.localeCompare(b.name);
      }
    });
  }, [guests, filterLower, sortBy]);

  const handleClick = (g: Guest) => {
    setSelectedObject({
      type: g.type === 'qemu' ? 'vm' : 'ct',
      id: g.vmid,
      name: g.name,
      node: g.node,
      cluster: g.cluster,
    });
  };

  const running = sorted.filter(g => g.status === 'running').length;

  return (
    <div className="p-4">
      <div className="flex items-center justify-between mb-4">
        <div>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
            All Virtual Machines & Containers
          </h2>
          <div className="text-sm text-gray-500">
            {running} running / {sorted.length} total
          </div>
        </div>
        <div className="flex items-center gap-2 text-sm">
          <span className="text-gray-500">Sort:</span>
          {(['name', 'status', 'node', 'type'] as const).map(s => (
            <button key={s} onClick={() => setSortBy(s)}
              className={`px-2 py-0.5 rounded text-xs ${
                sortBy === s
                  ? 'bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300'
                  : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'
              }`}>
              {s}
            </button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-2">
        {sorted.map((g) => {
          const isRunning = g.status === 'running';
          const isVM = g.type === 'qemu';
          const objType = isVM ? 'vm' : 'ct';
          const guestTags = tagAssignments
            .filter(a => a.object_type === objType && a.object_id === String(g.vmid) && a.cluster === g.cluster)
            .map(a => tags.find(t => t.id === a.tag_id))
            .filter(Boolean);

          return (
            <div
              key={`${g.cluster}-${g.vmid}`}
              onClick={() => handleClick(g)}
              className={`rounded-lg border-2 p-2 cursor-pointer transition-all hover:scale-105 hover:shadow-md relative
                ${isRunning
                  ? 'border-green-500/40 bg-green-50 dark:bg-green-900/10'
                  : 'border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-800/50 opacity-60'
                }`}
              title={`${g.name} (${g.vmid}) on ${g.node} - ${g.status}`}
            >
              <div className="flex items-center gap-1 mb-1">
                <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${isRunning ? 'bg-green-500' : 'bg-gray-400'}`} />
                <span className="text-[10px] text-gray-400 dark:text-gray-500">{isVM ? 'VM' : 'CT'}</span>
                <span className="text-[10px] text-gray-400 dark:text-gray-500 ml-auto truncate">{g.node}</span>
              </div>
              <div className="font-medium text-xs text-gray-900 dark:text-white truncate leading-tight">
                {g.name}
              </div>
              <div className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">{g.vmid}</div>
              {guestTags.length > 0 && (
                <div className="absolute bottom-1.5 right-1.5 flex gap-0.5">
                  {guestTags.map(t => (
                    <span key={t!.id} className="w-2 h-2 rounded-full" style={{ backgroundColor: t!.color }} title={`${t!.category}: ${t!.name}`} />
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
