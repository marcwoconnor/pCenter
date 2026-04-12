import { useState } from 'react';
import { Layout } from '../components/Layout';
import { InventoryTree } from '../components/InventoryTree';
import { ObjectDetail } from '../components/ObjectDetail';
import { ErrorBoundary } from '../components/ErrorBoundary';
import { useCluster } from '../context/ClusterContext';

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
          <div className="flex-1 flex items-center justify-center text-gray-500">
            Select a VM or container from the tree
          </div>
        )}
      </div>
    </Layout>
  );
}
