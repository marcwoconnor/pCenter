// Auth types matching backend responses

export interface User {
  id: string;
  username: string;
  email?: string;
  role: 'admin' | 'user';
  totp_enabled: boolean;
  last_login?: string;
  created_at: string;
  updated_at: string;
}

export interface Session {
  id: string;
  user_id: string;
  ip_address: string;
  created_at: string;
  expires_at: string;
  last_seen_at: string;
  current?: boolean; // Added by frontend to mark current session
}

export interface AuthEvent {
  id: number;
  timestamp: string;
  user_id?: string;
  username?: string;
  event_type: string;
  ip_address: string;
  details?: string;
  success: boolean;
}

// API Request/Response types

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  user: User;
  csrf_token: string;
}

export interface TOTPRequiredResponse {
  requires_totp: boolean;
  csrf_token: string;
}

export interface VerifyTOTPRequest {
  code: string;
}

export interface RegisterRequest {
  username: string;
  password: string;
  email?: string;
}

export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
}

export interface TOTPSetupResponse {
  secret: string;
  qr_code_data_url: string;
  recovery_codes: string[];
}

export interface TOTPVerifySetupRequest {
  code: string;
}

export interface CreateUserRequest {
  username: string;
  password: string;
  email?: string;
  role?: 'admin' | 'user';
}

export interface MeResponse {
  user: User;
  csrf_token: string;
}

export interface AuthCheckResponse {
  authenticated: boolean;
  requires_totp?: boolean;
  user?: User;
}

export interface UserCountResponse {
  count: number;
}

// Auth state
export interface AuthState {
  user: User | null;
  csrfToken: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  requiresTOTP: boolean;
  needsFirstUser: boolean | null;
  error: string | null;
}
