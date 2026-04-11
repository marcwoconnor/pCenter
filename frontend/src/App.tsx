import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider, useAuth } from './context/AuthContext';
import { ClusterProvider } from './context/ClusterContext';
import { FolderProvider } from './context/FolderContext';
import { Home } from './pages/Home';
import { HostsAndClusters } from './pages/HostsAndClusters';
import { VMsAndTemplates } from './pages/VMsAndTemplates';
import { StoragePage } from './pages/Storage';
import { NetworkPage } from './pages/Network';
import { ContentLibrary } from './pages/ContentLibrary';
import { ConsolePage } from './pages/ConsolePage';
import { Settings } from './pages/Settings';
import { Login } from './pages/Login';
import type { ReactNode } from 'react';

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
