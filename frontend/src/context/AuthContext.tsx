import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from 'react';
import {
  authApi,
  isTOTPRequired,
  setCSRFToken,
  getCSRFToken,
} from '../api/auth';
import type { AuthState } from '../types/auth';

interface AuthContextValue extends AuthState {
  login: (username: string, password: string) => Promise<void>;
  verifyTOTP: (code: string) => Promise<void>;
  logout: () => Promise<void>;
  register: (username: string, password: string, email?: string) => Promise<void>;
  refreshUser: () => Promise<void>;
  clearError: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const [state, setState] = useState<AuthState>({
    user: null,
    csrfToken: null,
    isAuthenticated: false,
    isLoading: true,
    requiresTOTP: false,
    needsFirstUser: null,
    error: null,
  });

  // Check auth status on mount
  useEffect(() => {
    const checkAuth = async () => {
      try {
        // First check if any users exist
        const countRes = await authApi.getUserCount();
        if (countRes.count === 0) {
          setState((s) => ({
            ...s,
            isLoading: false,
            needsFirstUser: true,
          }));
          return;
        }

        // Try to get current user
        const meRes = await authApi.getMe();
        setState({
          user: meRes.user,
          csrfToken: meRes.csrf_token,
          isAuthenticated: true,
          isLoading: false,
          requiresTOTP: false,
          needsFirstUser: false,
          error: null,
        });
      } catch {
        // Not authenticated
        setState((s) => ({
          ...s,
          user: null,
          csrfToken: null,
          isAuthenticated: false,
          isLoading: false,
          needsFirstUser: false,
          error: null,
        }));
      }
    };

    checkAuth();

    // Listen for session expiry from fetchAPI (avoids window.location.href)
    const handleSessionExpired = () => {
      setState({
        user: null,
        csrfToken: null,
        isAuthenticated: false,
        isLoading: false,
        requiresTOTP: false,
        needsFirstUser: null,
        error: null,
      });
    };
    window.addEventListener('auth:session-expired', handleSessionExpired);
    return () => window.removeEventListener('auth:session-expired', handleSessionExpired);
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    setState((s) => ({ ...s, error: null, isLoading: true }));

    try {
      const response = await authApi.login({ username, password });

      if (isTOTPRequired(response)) {
        // Need 2FA verification
        setState((s) => ({
          ...s,
          isLoading: false,
          requiresTOTP: true,
          csrfToken: response.csrf_token,
        }));
      } else {
        // Login complete
        setState({
          user: response.user,
          csrfToken: response.csrf_token,
          isAuthenticated: true,
          isLoading: false,
          requiresTOTP: false,
          needsFirstUser: false,
          error: null,
        });
      }
    } catch (e) {
      setState((s) => ({
        ...s,
        isLoading: false,
        error: e instanceof Error ? e.message : 'Login failed',
      }));
      throw e;
    }
  }, []);

  const verifyTOTP = useCallback(async (code: string) => {
    setState((s) => ({ ...s, error: null, isLoading: true }));

    try {
      const response = await authApi.verifyTOTP({ code });
      setState({
        user: response.user,
        csrfToken: response.csrf_token,
        isAuthenticated: true,
        isLoading: false,
        requiresTOTP: false,
        needsFirstUser: false,
        error: null,
      });
    } catch (e) {
      setState((s) => ({
        ...s,
        isLoading: false,
        error: e instanceof Error ? e.message : 'Verification failed',
      }));
      throw e;
    }
  }, []);

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } finally {
      setCSRFToken(null);
      setState({
        user: null,
        csrfToken: null,
        isAuthenticated: false,
        isLoading: false,
        requiresTOTP: false,
        needsFirstUser: false,
        error: null,
      });
    }
  }, []);

  const register = useCallback(
    async (username: string, password: string, email?: string) => {
      setState((s) => ({ ...s, error: null, isLoading: true }));

      try {
        const response = await authApi.register({ username, password, email });
        setState({
          user: response.user,
          csrfToken: response.csrf_token,
          isAuthenticated: true,
          isLoading: false,
          requiresTOTP: false,
          needsFirstUser: false,
          error: null,
        });
      } catch (e) {
        setState((s) => ({
          ...s,
          isLoading: false,
          error: e instanceof Error ? e.message : 'Registration failed',
        }));
        throw e;
      }
    },
    []
  );

  const refreshUser = useCallback(async () => {
    try {
      const meRes = await authApi.getMe();
      setState((s) => ({
        ...s,
        user: meRes.user,
        csrfToken: meRes.csrf_token,
      }));
    } catch {
      // Session expired
      setCSRFToken(null);
      setState((s) => ({
        ...s,
        user: null,
        csrfToken: null,
        isAuthenticated: false,
        requiresTOTP: false,
      }));
    }
  }, []);

  const clearError = useCallback(() => {
    setState((s) => ({ ...s, error: null }));
  }, []);

  return (
    <AuthContext.Provider
      value={{
        ...state,
        login,
        verifyTOTP,
        logout,
        register,
        refreshUser,
        clearError,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

// Export for use in api client
export { getCSRFToken };
