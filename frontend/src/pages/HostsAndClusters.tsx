import { useState } from 'react';
import { Layout } from '../components/Layout';
import { InventoryTree } from '../components/InventoryTree';
import { ObjectDetail } from '../components/ObjectDetail';
import { useCluster } from '../context/ClusterContext';

export function HostsAndClusters() {
  const { selectedObject } = useCluster();
  const [filter, setFilter] = useState('');

  const sidebar = (
    <div className="flex flex-col h-full">
      <div className="p-2 border-b border-gray-200 dark:border-gray-700">
        <input
          type="text"
          placeholder="Search..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
        />
      </div>
      <div className="flex-1 overflow-y-auto">
        <InventoryTree view="hosts" filter={filter} />
      </div>
    </div>
  );

  return (
    <Layout sidebar={sidebar}>
      <div className="flex-1 overflow-auto">
        {selectedObject ? (
          <ObjectDetail />
        ) : (
          <div className="flex-1 flex items-center justify-center text-gray-500">
            Select an object from the tree
          </div>
        )}
      </div>
    </Layout>
  );
}
