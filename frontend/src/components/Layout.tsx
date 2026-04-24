import { useState, useRef, useCallback, useEffect, type ReactNode } from 'react';
import { NavLink, useLocation, useNavigate } from 'react-router-dom';
import { useCluster } from '../context/ClusterContext';
import { useAuth } from '../context/AuthContext';
import { api } from '../api/client';
import { TasksBar } from './TasksBar';
import { AlarmBadge } from './AlarmBadge';
import { Console } from './Console';
import { ActivityPanel } from './ActivityPanel';

interface UpdateInfo {
  current_version: string;
  latest_version?: string;
  update_available: boolean;
  release_notes?: string;
  release_url?: string;
  release_name?: string;
  published_at?: string;
}

const navItems = [
  { path: '/', label: 'Home', icon: '⌂' },
  { path: '/hosts', label: 'Hosts & Clusters', icon: '▦' },
  { path: '/vms', label: 'VMs & Templates', icon: '◫' },
  { path: '/storage', label: 'Storage', icon: '▤' },
  { path: '/network', label: 'Network', icon: '◈' },
  { path: '/library', label: 'Library', icon: '📚' },
];

const MIN_SIDEBAR_WIDTH = 150;
const MAX_SIDEBAR_WIDTH = 500;
const DEFAULT_SIDEBAR_WIDTH = 256;

interface LayoutProps {
  children: ReactNode;
  sidebar?: ReactNode;
}

export function Layout({ children, sidebar }: LayoutProps) {
  const { isConnected, summary, clusters, error, consoles, closeConsole, focusConsole, updateConsole } = useCluster();
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
  const [showUpdatePanel, setShowUpdatePanel] = useState(false);

  useEffect(() => {
    api.getVersion().then(setUpdateInfo).catch(() => {});
    // Re-check every 30 minutes client-side
    const interval = setInterval(() => {
      api.getVersion().then(setUpdateInfo).catch(() => {});
    }, 30 * 60 * 1000);
    return () => clearInterval(interval);
  }, []);

  const handleLogout = async () => {
    await logout();
    navigate('/login');
  };
  const [sidebarWidth, setSidebarWidth] = useState(() => {
    try {
      const saved = localStorage.getItem('pcenter-sidebar-width');
      return saved ? parseInt(saved, 10) : DEFAULT_SIDEBAR_WIDTH;
    } catch { return DEFAULT_SIDEBAR_WIDTH; }
  });
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [isResizing, setIsResizing] = useState(false);
  const location = useLocation();
  const sidebarRef = useRef<HTMLDivElement>(null);

  // Save width to localStorage (try-catch for disabled/full storage)
  useEffect(() => {
    if (!sidebarCollapsed) {
      try { localStorage.setItem('pcenter-sidebar-width', sidebarWidth.toString()); } catch {}
    }
  }, [sidebarWidth, sidebarCollapsed]);

  const startResizing = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    setIsResizing(true);
  }, []);

  const stopResizing = useCallback(() => {
    setIsResizing(false);
  }, []);

  const resize = useCallback((e: MouseEvent) => {
    if (!isResizing || !sidebarRef.current) return;

    const newWidth = e.clientX - sidebarRef.current.getBoundingClientRect().left;
    if (newWidth >= MIN_SIDEBAR_WIDTH && newWidth <= MAX_SIDEBAR_WIDTH) {
      setSidebarWidth(newWidth);
      setSidebarCollapsed(false);
    } else if (newWidth < MIN_SIDEBAR_WIDTH / 2) {
      setSidebarCollapsed(true);
    }
  }, [isResizing]);

  useEffect(() => {
    if (isResizing) {
      window.addEventListener('mousemove', resize);
      window.addEventListener('mouseup', stopResizing);
      document.body.style.cursor = 'col-resize';
      document.body.style.userSelect = 'none';
    }
    return () => {
      window.removeEventListener('mousemove', resize);
      window.removeEventListener('mouseup', stopResizing);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
  }, [isResizing, resize, stopResizing]);

  const handleDoubleClick = () => {
    if (sidebarCollapsed) {
      setSidebarCollapsed(false);
    } else {
      setSidebarWidth(DEFAULT_SIDEBAR_WIDTH);
    }
  };

  return (
    <div className="h-screen flex flex-col bg-gray-100 dark:bg-gray-900">
      {/* Top Navigation Bar */}
      <header className="bg-gray-800 text-white shadow-lg flex-shrink-0">
        <div className="flex items-center h-12">
          {/* Logo */}
          <div className="px-4 border-r border-gray-700 relative">
            <div className="font-bold text-lg leading-tight">pCenter</div>
            <div className="flex items-center gap-1">
              <span className="text-[10px] text-gray-500 leading-none">{__APP_VERSION__}</span>
              {updateInfo?.update_available && (
                <button
                  onClick={() => setShowUpdatePanel(!showUpdatePanel)}
                  className="text-[9px] bg-green-600 text-white px-1 rounded leading-tight hover:bg-green-700"
                  title={`Update available: v${updateInfo.latest_version}`}
                >
                  NEW
                </button>
              )}
            </div>
            {/* Update panel dropdown */}
            {showUpdatePanel && updateInfo?.update_available && (
              <div className="absolute top-full left-0 mt-1 w-96 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-xl z-50 p-4">
                <div className="flex items-center justify-between mb-2">
                  <h3 className="font-semibold text-gray-900 dark:text-white text-sm">Update Available</h3>
                  <button onClick={() => setShowUpdatePanel(false)} className="text-gray-400 hover:text-gray-600 text-lg leading-none">&times;</button>
                </div>
                <div className="text-sm space-y-2">
                  <div className="flex justify-between text-xs">
                    <span className="text-gray-500">Current</span>
                    <span className="text-gray-900 dark:text-white font-mono">v{updateInfo.current_version}</span>
                  </div>
                  <div className="flex justify-between text-xs">
                    <span className="text-gray-500">Latest</span>
                    <span className="text-green-600 font-mono font-semibold">v{updateInfo.latest_version}</span>
                  </div>
                  {updateInfo.release_name && (
                    <div className="flex justify-between text-xs">
                      <span className="text-gray-500">Release</span>
                      <span className="text-gray-900 dark:text-white">{updateInfo.release_name}</span>
                    </div>
                  )}
                  {updateInfo.published_at && (
                    <div className="flex justify-between text-xs">
                      <span className="text-gray-500">Published</span>
                      <span className="text-gray-900 dark:text-white">{new Date(updateInfo.published_at).toLocaleDateString()}</span>
                    </div>
                  )}
                  {updateInfo.release_notes && (
                    <div className="mt-3 border-t border-gray-200 dark:border-gray-700 pt-2">
                      <div className="text-xs text-gray-500 mb-1">Release Notes</div>
                      <pre className="text-xs text-gray-900 dark:text-gray-300 whitespace-pre-wrap font-mono bg-gray-50 dark:bg-gray-900 p-2 rounded max-h-48 overflow-y-auto">
                        {updateInfo.release_notes}
                      </pre>
                    </div>
                  )}
                  {updateInfo.release_url && (
                    <a href={updateInfo.release_url} target="_blank" rel="noopener noreferrer"
                      className="block text-center mt-2 px-3 py-1.5 bg-blue-600 text-white text-xs rounded hover:bg-blue-700">
                      View on GitHub
                    </a>
                  )}
                </div>
              </div>
            )}
          </div>

          {/* Main Navigation */}
          <nav className="flex-1 flex items-center">
            {navItems.map((item) => (
              <NavLink
                key={item.path}
                to={item.path}
                className={({ isActive }) =>
                  `px-4 h-12 flex items-center gap-2 text-sm transition-colors ${
                    isActive
                      ? 'bg-gray-700 text-white border-b-2 border-blue-500'
                      : 'text-gray-300 hover:bg-gray-700 hover:text-white'
                  }`
                }
              >
                <span>{item.icon}</span>
                <span>{item.label}</span>
              </NavLink>
            ))}
          </nav>

          {/* Status indicators */}
          <div className="flex items-center gap-4 px-4">
            {summary && (
              <div className="text-xs text-gray-400">
                {summary.OnlineNodes}/{summary.TotalNodes} nodes |{' '}
                {summary.RunningVMs + summary.RunningCTs}/{summary.TotalVMs + summary.TotalContainers} guests
              </div>
            )}
            <div className={`flex items-center gap-1.5 text-xs ${isConnected ? 'text-green-400' : 'text-yellow-400'}`}>
              <div className={`w-2 h-2 rounded-full ${isConnected ? 'bg-green-400' : 'bg-yellow-400 animate-pulse'}`} />
              {isConnected ? 'Connected' : 'Connecting...'}
            </div>

            {/* Alarms */}
            <AlarmBadge />

            {/* User menu */}
            {user && (
              <div className="flex items-center gap-3 border-l border-gray-700 pl-4">
                <span className="text-xs text-gray-300">{user.username}</span>
                <NavLink
                  to="/settings"
                  className="text-xs text-gray-400 hover:text-white transition-colors"
                >
                  Settings
                </NavLink>
                <button
                  onClick={handleLogout}
                  className="text-xs text-gray-400 hover:text-white transition-colors"
                  title="Sign out"
                >
                  Sign out
                </button>
              </div>
            )}
          </div>
        </div>
      </header>

      {/* Status Banner — onboarding takes priority over transient connection errors
          when there's nothing to connect to yet. A misleading "connection lost"
          on a freshly installed / un-configured instance sends the wrong signal. */}
      {clusters.length === 0 ? (
        <div className="bg-blue-100 dark:bg-blue-900/40 text-blue-900 dark:text-blue-100 px-4 py-2 text-sm flex-shrink-0 flex items-center justify-between">
          <span>
            No Proxmox hosts connected yet. Add one to start managing nodes, VMs, and containers.
          </span>
          <button
            type="button"
            onClick={() => {
              const fire = () => window.dispatchEvent(new Event('pcenter:add-host'));
              if (location.pathname !== '/hosts') {
                navigate('/hosts');
                // InventoryTree subscribes on mount; wait a tick so the listener is attached.
                setTimeout(fire, 0);
              } else {
                fire();
              }
            }}
            className="ml-4 px-3 py-1 rounded bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium"
          >
            Add a host →
          </button>
        </div>
      ) : error ? (
        <div className="bg-yellow-500 text-yellow-900 px-4 py-2 text-sm flex-shrink-0">
          {error}
        </div>
      ) : null}

      {/* Main Content Area */}
      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar */}
        {sidebar && (
          <aside
            ref={sidebarRef}
            style={{ width: sidebarCollapsed ? 0 : sidebarWidth }}
            className={`bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 flex-shrink-0 flex flex-col ${
              sidebarCollapsed ? 'overflow-hidden' : ''
            }`}
          >
            {/* Sidebar Header */}
            <div className="h-10 flex items-center justify-between px-3 border-b border-gray-200 dark:border-gray-700 flex-shrink-0">
              <span className="text-sm font-medium text-gray-700 dark:text-gray-300 truncate">
                {location.pathname === '/hosts' && 'Hosts & Clusters'}
                {location.pathname === '/vms' && 'VMs & Templates'}
                {location.pathname === '/storage' && 'Storage'}
                {location.pathname === '/network' && 'Network'}
                {location.pathname === '/' && 'Overview'}
              </span>
            </div>

            {/* Sidebar Content */}
            <div className="flex-1 overflow-y-auto">
              {sidebar}
            </div>
          </aside>
        )}

        {/* Resize Handle */}
        {sidebar && (
          <div
            onMouseDown={startResizing}
            onDoubleClick={handleDoubleClick}
            className={`w-1 flex-shrink-0 cursor-col-resize hover:bg-blue-500 transition-colors ${
              isResizing ? 'bg-blue-500' : 'bg-gray-300 dark:bg-gray-600'
            }`}
            title="Drag to resize, double-click to reset"
          />
        )}

        {/* Main Content */}
        <main className="flex-1 overflow-hidden flex flex-col">
          {children}
        </main>
      </div>

      {/* Activity Panel */}
      <ActivityPanel />

      {/* Tasks Bar */}
      <TasksBar />

      {/* Console Windows */}
      {consoles.map((win) => (
        <Console
          key={win.id}
          console={win}
          onClose={() => closeConsole(win.id)}
          onFocus={() => focusConsole(win.id)}
          onUpdate={(updates) => updateConsole(win.id, updates)}
        />
      ))}
    </div>
  );
}
