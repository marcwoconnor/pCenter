import type {
  LoginRequest,
  LoginResponse,
  TOTPRequiredResponse,
  VerifyTOTPRequest,
  RegisterRequest,
  ChangePasswordRequest,
  TOTPSetupResponse,
  TOTPVerifySetupRequest,
  CreateUserRequest,
  MeResponse,
  AuthCheckResponse,
  UserCountResponse,
  User,
  Session,
  AuthEvent,
} from '../types/auth';

const BASE_URL = '/api';

// Store CSRF token for requests
let csrfToken: string | null = null;

export function setCSRFToken(token: string | null) {
  csrfToken = token;
}

export function getCSRFToken(): string | null {
  return csrfToken;
}

async function fetchAuth<T>(
  path: string,
  options?: RequestInit & { skipCSRF?: boolean }
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string>),
  };

  // Add CSRF token for state-changing requests
  if (
    csrfToken &&
    !options?.skipCSRF &&
    options?.method &&
    ['POST', 'PUT', 'DELETE'].includes(options.method)
  ) {
    headers['X-CSRF-Token'] = csrfToken;
  }

  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
    credentials: 'include', // Important: include cookies
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(error.error || 'API request failed');
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json();
}

// Type guard for TOTP required response
function isTOTPRequired(
  response: LoginResponse | TOTPRequiredResponse
): response is TOTPRequiredResponse {
  return 'requires_totp' in response && response.requires_totp;
}

export const authApi = {
  // Check user count (for first-user detection)
  getUserCount: () => fetchAuth<UserCountResponse>('/auth/user-count'),

  // Check current auth status
  checkAuth: () => fetchAuth<AuthCheckResponse>('/auth/check'),

  // Login
  login: async (req: LoginRequest): Promise<LoginResponse | TOTPRequiredResponse> => {
    const response = await fetchAuth<LoginResponse | TOTPRequiredResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify(req),
      skipCSRF: true, // Login doesn't need CSRF
    });

    // Store CSRF token from response
    if ('csrf_token' in response && response.csrf_token) {
      setCSRFToken(response.csrf_token);
    }

    return response;
  },

  // Verify TOTP code
  verifyTOTP: async (req: VerifyTOTPRequest): Promise<LoginResponse> => {
    const response = await fetchAuth<LoginResponse>('/auth/verify-totp', {
      method: 'POST',
      body: JSON.stringify(req),
    });
    if (response.csrf_token) {
      setCSRFToken(response.csrf_token);
    }
    return response;
  },

  // Logout
  logout: () =>
    fetchAuth<{ success: boolean }>('/auth/logout', { method: 'POST' }),

  // Register first user
  register: async (req: RegisterRequest): Promise<LoginResponse> => {
    const response = await fetchAuth<LoginResponse>('/auth/register', {
      method: 'POST',
      body: JSON.stringify(req),
      skipCSRF: true, // Registration doesn't need CSRF
    });
    if (response.csrf_token) {
      setCSRFToken(response.csrf_token);
    }
    return response;
  },

  // Get current user
  getMe: async (): Promise<MeResponse> => {
    const response = await fetchAuth<MeResponse>('/auth/me');
    if (response.csrf_token) {
      setCSRFToken(response.csrf_token);
    }
    return response;
  },

  // Change password
  changePassword: (req: ChangePasswordRequest) =>
    fetchAuth<{ success: boolean }>('/auth/password', {
      method: 'PUT',
      body: JSON.stringify(req),
    }),

  // TOTP setup
  setupTOTP: () =>
    fetchAuth<TOTPSetupResponse>('/auth/totp/setup', { method: 'POST' }),

  // Verify TOTP setup
  verifyTOTPSetup: (req: TOTPVerifySetupRequest) =>
    fetchAuth<{ success: boolean }>('/auth/totp/verify-setup', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  // Disable TOTP
  disableTOTP: () =>
    fetchAuth<{ success: boolean }>('/auth/totp', { method: 'DELETE' }),

  // List sessions
  listSessions: () => fetchAuth<Session[]>('/auth/sessions'),

  // Revoke session
  revokeSession: (id: string) =>
    fetchAuth<{ success: boolean }>(`/auth/sessions/${id}`, { method: 'DELETE' }),

  // Admin: List users
  listUsers: () => fetchAuth<User[]>('/users'),

  // Admin: Create user
  createUser: (req: CreateUserRequest) =>
    fetchAuth<User>('/users', {
      method: 'POST',
      body: JSON.stringify(req),
    }),

  // Admin: Delete user
  deleteUser: (id: string) =>
    fetchAuth<{ success: boolean }>(`/users/${id}`, { method: 'DELETE' }),

  // Admin: List auth events
  listAuthEvents: () => fetchAuth<AuthEvent[]>('/auth/events'),
};

// Export helper to check if response needs TOTP
export { isTOTPRequired };
