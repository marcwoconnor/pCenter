import { useState, useRef, useCallback, useEffect, type ReactNode } from 'react';
import { NavLink, useLocation, useNavigate } from 'react-router-dom';
import { useCluster } from '../context/ClusterContext';
import { useAuth } from '../context/AuthContext';
import { TasksBar } from './TasksBar';
import { AlarmBadge } from './AlarmBadge';
import { Console } from './Console';
import { ActivityPanel } from './ActivityPanel';

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
  const { isConnected, summary, error, consoles, closeConsole, focusConsole, updateConsole } = useCluster();
  const { user, logout } = useAuth();
  const navigate = useNavigate();

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
          <div className="px-4 border-r border-gray-700">
            <div className="font-bold text-lg leading-tight">pCenter</div>
            <div className="text-[10px] text-gray-500 leading-none">{__APP_VERSION__}</div>
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
              {isConnected ? 'Connected' : 'Reconnecting...'}
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

      {/* Error Banner */}
      {error && (
        <div className="bg-yellow-500 text-yellow-900 px-4 py-2 text-sm flex-shrink-0">
          {error}
        </div>
      )}

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
