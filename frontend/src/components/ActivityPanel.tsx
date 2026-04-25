import { useState, useCallback, useEffect, useRef } from 'react';
import { useCluster } from '../context/ClusterContext';

const ACTION_LABELS: Record<string, string> = {
  config_update: 'Config Updated',
  vm_start: 'VM Started',
  vm_stop: 'VM Stopped',
  vm_shutdown: 'VM Shutdown',
  ct_start: 'Container Started',
  ct_stop: 'Container Stopped',
  ct_shutdown: 'Container Shutdown',
  migrate: 'Migration',
  ha_enable: 'HA Enabled',
  ha_disable: 'HA Disabled',
  drs_apply: 'DRS Applied',
  drs_dismiss: 'DRS Dismissed',
  folder_create: 'Folder Created',
  folder_rename: 'Folder Renamed',
  folder_delete: 'Folder Deleted',
  folder_move: 'Folder Moved',
  resource_move: 'Resource Moved',
};

const ACTION_COLORS: Record<string, string> = {
  config_update: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
  vm_start: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
  vm_stop: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
  vm_shutdown: 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200',
  ct_start: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
  ct_stop: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
  ct_shutdown: 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200',
  migrate: 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200',
  ha_enable: 'bg-teal-100 text-teal-800 dark:bg-teal-900 dark:text-teal-200',
  ha_disable: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
  drs_apply: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200',
  drs_dismiss: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
  folder_create: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
  folder_rename: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
  folder_delete: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
  folder_move: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
  resource_move: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900 dark:text-cyan-200',
};

const DEFAULT_HEIGHT = 150;
const MIN_HEIGHT = 80;
const MAX_HEIGHT = 400;

export function ActivityPanel() {
  const { activityEntries } = useCluster();
  const [isOpen, setIsOpen] = useState(() => {
    try { return localStorage.getItem('pcenter-activity-open') !== 'false'; }
    catch { return true; }
  });
  const [height, setHeight] = useState(() => {
    try {
      const saved = localStorage.getItem('pcenter-activity-height');
      return saved ? parseInt(saved, 10) : DEFAULT_HEIGHT;
    } catch { return DEFAULT_HEIGHT; }
  });
  const [isResizing, setIsResizing] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);

  // Save state to localStorage (try-catch for disabled/full storage)
  useEffect(() => {
    try { localStorage.setItem('pcenter-activity-open', isOpen.toString()); } catch { /* localStorage can fail silently (quota, private mode) — non-essential persistence */ }
  }, [isOpen]);

  useEffect(() => {
    try { localStorage.setItem('pcenter-activity-height', height.toString()); } catch { /* localStorage can fail silently (quota, private mode) — non-essential persistence */ }
  }, [height]);

  // Resize handling
  const startResizing = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    setIsResizing(true);
  }, []);

  const stopResizing = useCallback(() => {
    setIsResizing(false);
  }, []);

  const resize = useCallback((e: MouseEvent) => {
    if (!isResizing || !panelRef.current) return;
    const rect = panelRef.current.getBoundingClientRect();
    const newHeight = rect.bottom - e.clientY;
    if (newHeight >= MIN_HEIGHT && newHeight <= MAX_HEIGHT) {
      setHeight(newHeight);
    }
  }, [isResizing]);

  useEffect(() => {
    if (isResizing) {
      window.addEventListener('mousemove', resize);
      window.addEventListener('mouseup', stopResizing);
      document.body.style.cursor = 'row-resize';
      document.body.style.userSelect = 'none';
    }
    return () => {
      window.removeEventListener('mousemove', resize);
      window.removeEventListener('mouseup', stopResizing);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
  }, [isResizing, resize, stopResizing]);

  const formatTime = (timestamp: string) => {
    const date = new Date(timestamp);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  };

  const formatDetails = (entry: typeof activityEntries[0]) => {
    if (!entry.details) return null;
    try {
      const parsed = JSON.parse(entry.details);

      // Config changes
      if (parsed.changed) {
        return `Changed: ${parsed.changed.join(', ')}`;
      }

      // Migrations
      if (parsed.from_node && parsed.to_node) {
        let text = `${parsed.from_node} → ${parsed.to_node}`;
        if (parsed.duration) {
          text += ` (${parsed.duration})`;
        }
        if (parsed.error) {
          text += ` - ${parsed.error}`;
        }
        return text;
      }

      // Fallback
      return JSON.stringify(parsed);
    } catch {
      return entry.details;
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'success':
        return <span className="text-green-600 dark:text-green-400">✓</span>;
      case 'error':
        return <span className="text-red-600 dark:text-red-400">✗</span>;
      case 'started':
        return <span className="text-blue-600 dark:text-blue-400">●</span>;
      default:
        return null;
    }
  };

  return (
    <div
      ref={panelRef}
      className="bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700 flex-shrink-0"
    >
      {/* Resize Handle */}
      {isOpen && (
        <div
          onMouseDown={startResizing}
          className={`h-1 cursor-row-resize hover:bg-blue-500 transition-colors ${
            isResizing ? 'bg-blue-500' : 'bg-gray-200 dark:bg-gray-700'
          }`}
        />
      )}

      {/* Header */}
      <div
        onClick={() => setIsOpen(!isOpen)}
        className="h-8 flex items-center justify-between px-3 cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700 border-b border-gray-200 dark:border-gray-700"
      >
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500 dark:text-gray-400">
            {isOpen ? '▼' : '▶'}
          </span>
          <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
            Activity Log
          </span>
          {activityEntries.length > 0 && (
            <span className="text-xs text-gray-400 dark:text-gray-500">
              ({activityEntries.length})
            </span>
          )}
        </div>
        {!isOpen && activityEntries.length > 0 && (
          <span className="text-xs text-gray-500 dark:text-gray-400 truncate max-w-md">
            {ACTION_LABELS[activityEntries[0].action] || activityEntries[0].action}:{' '}
            {activityEntries[0].resource_name || `${activityEntries[0].resource_type}/${activityEntries[0].resource_id}`}
          </span>
        )}
      </div>

      {/* Content */}
      {isOpen && (
        <div
          style={{ height }}
          className="overflow-y-auto"
        >
          {activityEntries.length === 0 ? (
            <div className="flex items-center justify-center h-full text-gray-400 dark:text-gray-500 text-sm">
              No activity yet
            </div>
          ) : (
            <table className="w-full text-xs">
              <thead className="sticky top-0 bg-gray-50 dark:bg-gray-700">
                <tr className="text-left text-gray-500 dark:text-gray-400">
                  <th className="px-3 py-1.5 font-medium w-20">Time</th>
                  <th className="px-3 py-1.5 font-medium w-6"></th>
                  <th className="px-3 py-1.5 font-medium w-28">Action</th>
                  <th className="px-3 py-1.5 font-medium">Resource</th>
                  <th className="px-3 py-1.5 font-medium">Details</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-700">
                {activityEntries.map((entry) => (
                  <tr
                    key={entry.id}
                    className={`hover:bg-gray-50 dark:hover:bg-gray-700/50 ${
                      entry.status === 'error' ? 'bg-red-50 dark:bg-red-900/20' : ''
                    }`}
                  >
                    <td className="px-3 py-1.5 text-gray-500 dark:text-gray-400 whitespace-nowrap">
                      {formatTime(entry.timestamp)}
                    </td>
                    <td className="px-1 py-1.5 text-center">
                      {getStatusBadge(entry.status)}
                    </td>
                    <td className="px-3 py-1.5">
                      <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-medium ${
                        ACTION_COLORS[entry.action] || 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
                      }`}>
                        {ACTION_LABELS[entry.action] || entry.action}
                      </span>
                    </td>
                    <td className="px-3 py-1.5 text-gray-700 dark:text-gray-300">
                      <span className="text-gray-400 dark:text-gray-500">
                        {entry.resource_type === 'vm' ? 'VM' : entry.resource_type === 'ct' ? 'CT' : entry.resource_type}
                      </span>{' '}
                      {entry.resource_name || entry.resource_id}
                      <span className="text-gray-400 dark:text-gray-500 ml-1">
                        @ {entry.cluster}
                      </span>
                    </td>
                    <td className={`px-3 py-1.5 truncate max-w-md ${
                      entry.status === 'error'
                        ? 'text-red-600 dark:text-red-400'
                        : 'text-gray-500 dark:text-gray-400'
                    }`}>
                      {formatDetails(entry)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}
