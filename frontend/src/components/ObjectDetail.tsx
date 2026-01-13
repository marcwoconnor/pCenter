import { useState } from 'react';
import { useCluster } from '../context/ClusterContext';
import { formatBytes, formatUptime } from '../api/client';

interface Tab {
  id: string;
  label: string;
}

const nodeTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'vms', label: 'Virtual Machines' },
  { id: 'monitor', label: 'Monitor' },
];

const guestTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'monitor', label: 'Monitor' },
  { id: 'configure', label: 'Configure' },
];

const storageTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'vms', label: 'Virtual Machines' },
];

export function ObjectDetail() {
  const { selectedObject, nodes, guests, storage, performAction } = useCluster();
  const [activeTab, setActiveTab] = useState('summary');

  if (!selectedObject) {
    return (
      <div className="flex-1 flex items-center justify-center text-gray-500">
        Select an object from the inventory tree
      </div>
    );
  }

  const tabs = selectedObject.type === 'node' ? nodeTabs :
    selectedObject.type === 'storage' ? storageTabs : guestTabs;

  // Get the actual object data
  const node = selectedObject.type === 'node'
    ? nodes.find((n) => n.node === selectedObject.id)
    : null;

  const guest = (selectedObject.type === 'vm' || selectedObject.type === 'ct')
    ? guests.find((g) => g.vmid === selectedObject.id)
    : null;

  const storageItem = selectedObject.type === 'storage'
    ? storage.find((s) => `${s.node}-${s.storage}` === selectedObject.id)
    : null;

  const handleAction = async (action: 'start' | 'stop' | 'shutdown') => {
    if (!guest) return;
    try {
      await performAction(guest.type === 'qemu' ? 'vm' : 'ct', guest.vmid, action);
    } catch {
      // Error is shown in tasks bar
    }
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Object Header */}
      <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 py-3 flex-shrink-0">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="text-2xl">
              {selectedObject.type === 'node' && '🖥'}
              {selectedObject.type === 'vm' && '💻'}
              {selectedObject.type === 'ct' && '📦'}
              {selectedObject.type === 'storage' && '💾'}
            </span>
            <div>
              <h1 className="text-lg font-semibold text-gray-900 dark:text-white">
                {selectedObject.name}
              </h1>
              <div className="text-sm text-gray-500">
                {selectedObject.type === 'node' && 'Proxmox Node'}
                {selectedObject.type === 'vm' && `VM ${selectedObject.id} on ${selectedObject.node}`}
                {selectedObject.type === 'ct' && `Container ${selectedObject.id} on ${selectedObject.node}`}
                {selectedObject.type === 'storage' && `Storage on ${selectedObject.node}`}
              </div>
            </div>
          </div>

          {/* Actions */}
          {guest && (
            <div className="flex gap-2">
              {guest.status === 'stopped' ? (
                <button
                  onClick={() => handleAction('start')}
                  className="px-3 py-1.5 bg-green-600 text-white text-sm rounded hover:bg-green-700"
                >
                  Start
                </button>
              ) : (
                <>
                  <button
                    onClick={() => handleAction('shutdown')}
                    className="px-3 py-1.5 bg-yellow-600 text-white text-sm rounded hover:bg-yellow-700"
                  >
                    Shutdown
                  </button>
                  <button
                    onClick={() => handleAction('stop')}
                    className="px-3 py-1.5 bg-red-600 text-white text-sm rounded hover:bg-red-700"
                  >
                    Stop
                  </button>
                </>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 flex-shrink-0">
        <div className="flex gap-1">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 text-sm font-medium transition-colors ${
                activeTab === tab.id
                  ? 'text-blue-600 border-b-2 border-blue-600 bg-white dark:bg-gray-700'
                  : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Tab Content */}
      <div className="flex-1 overflow-auto p-4 bg-gray-50 dark:bg-gray-900">
        {activeTab === 'summary' && node && <NodeSummary node={node} />}
        {activeTab === 'summary' && guest && <GuestSummary guest={guest} />}
        {activeTab === 'summary' && storageItem && <StorageSummary storage={storageItem} />}
        {activeTab === 'vms' && node && <NodeVMs nodeId={node.node} />}
        {activeTab === 'monitor' && <MonitorTab />}
        {activeTab === 'configure' && <ConfigureTab />}
      </div>
    </div>
  );
}

function NodeSummary({ node }: { node: any }) {
  const cpuPercent = (node.cpu * 100).toFixed(1);
  const memPercent = ((node.mem / node.maxmem) * 100).toFixed(1);

  return (
    <div className="grid md:grid-cols-2 gap-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Status</h3>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">Status</span>
            <span className={node.status === 'online' ? 'text-green-600' : 'text-red-600'}>
              {node.status}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Uptime</span>
            <span className="text-gray-900 dark:text-white">{formatUptime(node.uptime)}</span>
          </div>
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Resources</h3>
        <div className="space-y-3">
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">CPU ({node.maxcpu} cores)</span>
              <span className="text-gray-900 dark:text-white">{cpuPercent}%</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className={`h-full rounded ${parseFloat(cpuPercent) > 80 ? 'bg-red-500' : 'bg-blue-500'}`}
                style={{ width: `${cpuPercent}%` }}
              />
            </div>
          </div>
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">Memory ({formatBytes(node.maxmem)})</span>
              <span className="text-gray-900 dark:text-white">{memPercent}%</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className={`h-full rounded ${parseFloat(memPercent) > 80 ? 'bg-red-500' : 'bg-green-500'}`}
                style={{ width: `${memPercent}%` }}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function GuestSummary({ guest }: { guest: any }) {
  const cpuPercent = (guest.cpu * 100).toFixed(1);
  const memPercent = guest.maxmem > 0 ? ((guest.mem / guest.maxmem) * 100).toFixed(1) : '0';

  return (
    <div className="grid md:grid-cols-2 gap-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Status</h3>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">Status</span>
            <span className={guest.status === 'running' ? 'text-green-600' : 'text-gray-600'}>
              {guest.status}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Type</span>
            <span className="text-gray-900 dark:text-white">{guest.type === 'qemu' ? 'Virtual Machine' : 'Container'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Node</span>
            <span className="text-gray-900 dark:text-white">{guest.node}</span>
          </div>
          {guest.status === 'running' && (
            <div className="flex justify-between">
              <span className="text-gray-500">Uptime</span>
              <span className="text-gray-900 dark:text-white">{formatUptime(guest.uptime)}</span>
            </div>
          )}
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Resources</h3>
        <div className="space-y-3">
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">CPU ({guest.cpus} cores)</span>
              <span className="text-gray-900 dark:text-white">{guest.status === 'running' ? `${cpuPercent}%` : '-'}</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className="h-full rounded bg-blue-500"
                style={{ width: guest.status === 'running' ? `${cpuPercent}%` : '0%' }}
              />
            </div>
          </div>
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">Memory ({formatBytes(guest.maxmem)})</span>
              <span className="text-gray-900 dark:text-white">{guest.status === 'running' ? `${memPercent}%` : '-'}</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className="h-full rounded bg-green-500"
                style={{ width: guest.status === 'running' ? `${memPercent}%` : '0%' }}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function StorageSummary({ storage }: { storage: any }) {
  const usedPercent = storage.total > 0 ? ((storage.used / storage.total) * 100).toFixed(1) : '0';

  return (
    <div className="grid md:grid-cols-2 gap-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Details</h3>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">Type</span>
            <span className="text-gray-900 dark:text-white">{storage.type}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Node</span>
            <span className="text-gray-900 dark:text-white">{storage.node}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Shared</span>
            <span className="text-gray-900 dark:text-white">{storage.shared ? 'Yes' : 'No'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">Content</span>
            <span className="text-gray-900 dark:text-white">{storage.content}</span>
          </div>
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Capacity</h3>
        <div className="space-y-3">
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-500">Used</span>
              <span className="text-gray-900 dark:text-white">{formatBytes(storage.used)} / {formatBytes(storage.total)}</span>
            </div>
            <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
              <div
                className={`h-full rounded ${parseFloat(usedPercent) > 80 ? 'bg-red-500' : 'bg-blue-500'}`}
                style={{ width: `${usedPercent}%` }}
              />
            </div>
          </div>
          <div className="text-sm text-gray-500">
            {formatBytes(storage.avail)} available ({(100 - parseFloat(usedPercent)).toFixed(1)}%)
          </div>
        </div>
      </div>
    </div>
  );
}

function NodeVMs({ nodeId }: { nodeId: string }) {
  const { guests, performAction } = useCluster();
  const nodeGuests = guests.filter((g) => g.node === nodeId);

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
      <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
        <thead className="bg-gray-50 dark:bg-gray-700">
          <tr>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">ID</th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">CPU</th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Memory</th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
          {nodeGuests.map((g) => (
            <tr key={g.vmid} className="hover:bg-gray-50 dark:hover:bg-gray-700">
              <td className="px-4 py-2 text-sm">{g.vmid}</td>
              <td className="px-4 py-2 text-sm font-medium">{g.name}</td>
              <td className="px-4 py-2 text-sm">{g.type === 'qemu' ? 'VM' : 'CT'}</td>
              <td className="px-4 py-2 text-sm">
                <span className={`px-2 py-0.5 rounded text-xs ${g.status === 'running' ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800'}`}>
                  {g.status}
                </span>
              </td>
              <td className="px-4 py-2 text-sm">{g.status === 'running' ? `${(g.cpu * 100).toFixed(0)}%` : '-'}</td>
              <td className="px-4 py-2 text-sm">{formatBytes(g.mem)} / {formatBytes(g.maxmem)}</td>
              <td className="px-4 py-2 text-sm">
                {g.status === 'stopped' ? (
                  <button onClick={() => performAction(g.type === 'qemu' ? 'vm' : 'ct', g.vmid, 'start')} className="text-green-600 hover:underline text-xs">Start</button>
                ) : (
                  <button onClick={() => performAction(g.type === 'qemu' ? 'vm' : 'ct', g.vmid, 'stop')} className="text-red-600 hover:underline text-xs">Stop</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function MonitorTab() {
  return (
    <div className="text-gray-500 text-center py-8">
      Performance monitoring coming soon...
    </div>
  );
}

function ConfigureTab() {
  return (
    <div className="text-gray-500 text-center py-8">
      Configuration options coming soon...
    </div>
  );
}
