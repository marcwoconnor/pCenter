import { useState } from 'react';
import { Layout } from '../components/Layout';
import { InventoryTree } from '../components/InventoryTree';
import { useCluster } from '../context/ClusterContext';

export function NetworkPage() {
  const { nodes } = useCluster();
  const [filter, setFilter] = useState('');

  const sidebar = (
    <div className="flex flex-col h-full">
      <div className="p-2 border-b border-gray-200 dark:border-gray-700">
        <input
          type="text"
          placeholder="Search networks..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
        />
      </div>
      <div className="flex-1 overflow-y-auto">
        <InventoryTree view="network" filter={filter} />
      </div>
    </div>
  );

  return (
    <Layout sidebar={sidebar}>
      <div className="flex-1 overflow-auto p-6">
        <h1 className="text-xl font-bold text-gray-900 dark:text-white mb-4">Network Configuration</h1>

        <div className="bg-white dark:bg-gray-800 rounded-lg shadow">
          <table className="min-w-full">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Node</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Interface</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Type</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {nodes.map((node) => (
                <>
                  <tr key={`${node.node}-vmbr0`} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100">{node.node}</td>
                    <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">vmbr0</td>
                    <td className="px-4 py-3 text-sm text-gray-500">Linux Bridge</td>
                    <td className="px-4 py-3 text-sm">
                      <span className="px-2 py-0.5 rounded text-xs bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
                        active
                      </span>
                    </td>
                  </tr>
                  <tr key={`${node.node}-vmbr1`} className="hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <td className="px-4 py-3 text-sm text-gray-900 dark:text-gray-100">{node.node}</td>
                    <td className="px-4 py-3 text-sm font-medium text-gray-900 dark:text-gray-100">vmbr1</td>
                    <td className="px-4 py-3 text-sm text-gray-500">Linux Bridge</td>
                    <td className="px-4 py-3 text-sm">
                      <span className="px-2 py-0.5 rounded text-xs bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
                        active
                      </span>
                    </td>
                  </tr>
                </>
              ))}
            </tbody>
          </table>
        </div>

        <p className="mt-4 text-sm text-gray-500 dark:text-gray-400">
          Network configuration is a placeholder. Full network management coming in future milestone.
        </p>
      </div>
    </Layout>
  );
}
