import { lazy, Suspense } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider, useAuth } from './context/AuthContext';
import { ClusterProvider } from './context/ClusterContext';
import { FolderProvider } from './context/FolderContext';
import { Login } from './pages/Login';
import type { ReactNode } from 'react';

// Route-level pages are lazy-loaded so the initial JS bundle stays under
// the 500 kB warning threshold (#67). Each page becomes its own chunk;
// the heavy ones (Ceph @ ~1.7k LOC, Settings @ ~1.3k LOC, ObjectDetail
// @ ~2.9k LOC pulled in via Home/Hosts/VMs) only download when the user
// navigates there. Login stays eager because it's the entry point —
// splitting it would just add a round-trip on first visit.
//
// React.lazy expects a `{ default: Component }` shape, but our pages
// use named exports — the `.then(m => ({ default: m.X }))` adapter
// preserves the named-export convention without rewriting every page.
const Home = lazy(() => import('./pages/Home').then(m => ({ default: m.Home })));
const HostsAndClusters = lazy(() => import('./pages/HostsAndClusters').then(m => ({ default: m.HostsAndClusters })));
const VMsAndTemplates = lazy(() => import('./pages/VMsAndTemplates').then(m => ({ default: m.VMsAndTemplates })));
const StoragePage = lazy(() => import('./pages/Storage').then(m => ({ default: m.StoragePage })));
const CephPage = lazy(() => import('./pages/Ceph').then(m => ({ default: m.CephPage })));
const NetworkPage = lazy(() => import('./pages/Network').then(m => ({ default: m.NetworkPage })));
const ContentLibrary = lazy(() => import('./pages/ContentLibrary').then(m => ({ default: m.ContentLibrary })));
const ConsolePage = lazy(() => import('./pages/ConsolePage').then(m => ({ default: m.ConsolePage })));
const Settings = lazy(() => import('./pages/Settings').then(m => ({ default: m.Settings })));

// Loading spinner component
function LoadingScreen() {
  return (
    <div className="min-h-screen bg-slate-900 flex items-center justify-center">
      <div className="text-center">
        <div className="w-12 h-12 border-4 border-blue-600 border-t-transparent rounded-full animate-spin mx-auto mb-4" />
        <p className="text-slate-400">Loading...</p>
      </div>
    </div>
  );
}

// Protected route wrapper
function ProtectedRoute({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading, requiresTOTP, needsFirstUser } = useAuth();

  if (isLoading) {
    return <LoadingScreen />;
  }

  // Redirect to login if not authenticated
  if (!isAuthenticated || requiresTOTP || needsFirstUser) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

// Login route wrapper (redirect if already authenticated)
function LoginRoute() {
  const { isAuthenticated, isLoading, requiresTOTP } = useAuth();

  if (isLoading) {
    return <LoadingScreen />;
  }

  // If fully authenticated, redirect to home
  if (isAuthenticated && !requiresTOTP) {
    return <Navigate to="/" replace />;
  }

  // Show login (handles needsFirstUser, requiresTOTP, and normal login)
  return <Login />;
}

// Inner app with auth-aware routing
function AppRoutes() {
  return (
    <BrowserRouter>
      <Suspense fallback={<LoadingScreen />}>
        <Routes>
          <Route path="/login" element={<LoginRoute />} />
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <ClusterProvider>
                  <FolderProvider>
                    <Home />
                  </FolderProvider>
                </ClusterProvider>
              </ProtectedRoute>
            }
          />
          <Route
            path="/hosts"
            element={
              <ProtectedRoute>
                <ClusterProvider>
                  <FolderProvider>
                    <HostsAndClusters />
                  </FolderProvider>
                </ClusterProvider>
              </ProtectedRoute>
            }
          />
          <Route
            path="/vms"
            element={
              <ProtectedRoute>
                <ClusterProvider>
                  <FolderProvider>
                    <VMsAndTemplates />
                  </FolderProvider>
                </ClusterProvider>
              </ProtectedRoute>
            }
          />
          <Route
            path="/storage"
            element={
              <ProtectedRoute>
                <ClusterProvider>
                  <FolderProvider>
                    <StoragePage />
                  </FolderProvider>
                </ClusterProvider>
              </ProtectedRoute>
            }
          />
          <Route
            path="/ceph"
            element={
              <ProtectedRoute>
                <ClusterProvider>
                  <FolderProvider>
                    <CephPage />
                  </FolderProvider>
                </ClusterProvider>
              </ProtectedRoute>
            }
          />
          <Route
            path="/network"
            element={
              <ProtectedRoute>
                <ClusterProvider>
                  <FolderProvider>
                    <NetworkPage />
                  </FolderProvider>
                </ClusterProvider>
              </ProtectedRoute>
            }
          />
          <Route
            path="/library"
            element={
              <ProtectedRoute>
                <ClusterProvider>
                  <FolderProvider>
                    <ContentLibrary />
                  </FolderProvider>
                </ClusterProvider>
              </ProtectedRoute>
            }
          />
          <Route
            path="/console/:type/:vmid/:name"
            element={
              <ProtectedRoute>
                <ClusterProvider>
                  <FolderProvider>
                    <ConsolePage />
                  </FolderProvider>
                </ClusterProvider>
              </ProtectedRoute>
            }
          />
          <Route
            path="/settings"
            element={
              <ProtectedRoute>
                <ClusterProvider>
                  <FolderProvider>
                    <Settings />
                  </FolderProvider>
                </ClusterProvider>
              </ProtectedRoute>
            }
          />
        </Routes>
      </Suspense>
    </BrowserRouter>
  );
}

function App() {
  return (
    <AuthProvider>
      <AppRoutes />
    </AuthProvider>
  );
}

export default App;
