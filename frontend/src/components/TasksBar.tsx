import { useState, useEffect } from 'react';
import { useCluster } from '../context/ClusterContext';

export function TasksBar() {
  const { tasks, migrations } = useCluster();
  const [expanded, setExpanded] = useState(false);

  // Clock kept in state so "X seconds ago" labels stay fresh and render
  // output is a pure function of props+state — react-hooks/purity flags
  // Date.now() directly in render as impure.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const recentTasks = tasks.slice(0, expanded ? 10 : 3);
  const runningCount = tasks.filter((t) => t.status === 'running').length;
  const runningMigrations = migrations.filter((m) => m.status === 'running');

  const formatTime = (timestamp: number) => {
    const seconds = Math.floor((now - timestamp) / 1000);
    if (seconds < 60) return `${seconds}s ago`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
    return `${Math.floor(seconds / 3600)}h ago`;
  };

  return (
    <div className="bg-gray-800 border-t border-gray-700 flex-shrink-0">
      {/* Header */}
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full h-8 px-4 flex items-center justify-between text-sm text-gray-300 hover:bg-gray-700"
      >
        <div className="flex items-center gap-2">
          <span>Tasks</span>
          {runningCount > 0 && (
            <span className="px-1.5 py-0.5 bg-blue-600 text-white text-xs rounded">
              {runningCount} running
            </span>
          )}
          {runningMigrations.length > 0 && (
            <span className="px-1.5 py-0.5 bg-purple-600 text-white text-xs rounded flex items-center gap-1">
              <span className="inline-block w-2 h-2 border border-white border-t-transparent rounded-full animate-spin" />
              {runningMigrations.length} migrating
            </span>
          )}
        </div>
        <span>{expanded ? '▼' : '▲'}</span>
      </button>

      {/* Active Migrations */}
      {expanded && runningMigrations.length > 0 && (
        <div className="border-t border-gray-700">
          <div className="px-4 py-1 text-xs text-gray-500 uppercase tracking-wide bg-gray-750">
            Active Migrations
          </div>
          {runningMigrations.map((m) => (
            <div
              key={m.upid}
              className="px-4 py-2 flex items-center gap-3 text-sm border-b border-gray-700"
            >
              <div className="flex-shrink-0">
                <div className="w-4 h-4 border-2 border-purple-500 border-t-transparent rounded-full animate-spin" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="text-gray-200 truncate">
                  {m.guest_type === 'vm' ? '💻' : '📦'} {m.guest_name} (VMID {m.vmid})
                </div>
                <div className="text-gray-500 text-xs truncate">
                  {m.from_node} → {m.to_node} {m.online && '(live)'}
                </div>
              </div>
              <div className="flex-shrink-0 w-16">
                <div className="h-1.5 bg-gray-700 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-purple-500 transition-all duration-300"
                    style={{ width: `${m.progress}%` }}
                  />
                </div>
                <div className="text-xs text-gray-500 text-center mt-0.5">{m.progress}%</div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Task List */}
      {expanded && recentTasks.length > 0 && (
        <div className="border-t border-gray-700 max-h-48 overflow-y-auto">
          {recentTasks.map((task) => (
            <div
              key={task.id}
              className="px-4 py-2 flex items-center gap-3 text-sm border-b border-gray-700 last:border-0"
            >
              {/* Status Icon */}
              <div className="flex-shrink-0">
                {task.status === 'running' && (
                  <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
                )}
                {task.status === 'completed' && (
                  <div className="w-4 h-4 rounded-full bg-green-500 flex items-center justify-center text-white text-xs">✓</div>
                )}
                {task.status === 'failed' && (
                  <div className="w-4 h-4 rounded-full bg-red-500 flex items-center justify-center text-white text-xs">✕</div>
                )}
              </div>

              {/* Task Info */}
              <div className="flex-1 min-w-0">
                <div className="text-gray-200 truncate">{task.type}</div>
                <div className="text-gray-500 text-xs truncate">{task.target}</div>
              </div>

              {/* Time */}
              <div className="text-gray-500 text-xs flex-shrink-0">
                {formatTime(task.startTime)}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Empty state */}
      {expanded && recentTasks.length === 0 && (
        <div className="px-4 py-3 text-sm text-gray-500 text-center border-t border-gray-700">
          No recent tasks
        </div>
      )}
    </div>
  );
}
