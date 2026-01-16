package auth

import "time"

// User represents a pCenter user
type User struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	Email          string    `json:"email,omitempty"`
	PasswordHash   string    `json:"-"` // never expose
	Role           string    `json:"role"`
	TOTPEnabled    bool      `json:"totp_enabled"`
	TOTPSecret     string    `json:"-"` // encrypted, never expose
	FailedAttempts int       `json:"-"`
	LockedUntil    time.Time `json:"-"`
	LastLogin      time.Time `json:"last_login,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Session represents an active session
type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	TokenHash    string    `json:"-"` // never expose
	CSRFToken    string    `json:"-"` // exposed only via /me endpoint
	TOTPVerified bool      `json:"totp_verified"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
}

// RecoveryCode represents a TOTP recovery code
type RecoveryCode struct {
	ID        string    `json:"id"`
	UserID    string    `json:"-"`
	CodeHash  string    `json:"-"` // bcrypt hash
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

// AuthEvent represents an audit log entry
type AuthEvent struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"user_id,omitempty"`
	Username  string    `json:"username,omitempty"`
	EventType string    `json:"event_type"`
	IPAddress string    `json:"ip_address"`
	Details   string    `json:"details,omitempty"`
	Success   bool      `json:"success"`
}

// Event types
const (
	EventLogin           = "login"
	EventLoginFailed     = "login_failed"
	EventLogout          = "logout"
	EventPasswordChanged = "password_changed"
	EventTOTPEnabled     = "totp_enabled"
	EventTOTPDisabled    = "totp_disabled"
	EventRecoveryUsed    = "recovery_used"
	EventAccountLocked   = "account_locked"
	EventUserCreated     = "user_created"
	EventUserDeleted     = "user_deleted"
	EventSessionRevoked  = "session_revoked"
)

// Roles
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// --- Request/Response types ---

// LoginRequest is the login form submission
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse on successful login (no 2FA)
type LoginResponse struct {
	User      *User  `json:"user"`
	CSRFToken string `json:"csrf_token"`
}

// TOTPRequiredResponse when 2FA is needed
type TOTPRequiredResponse struct {
	RequiresTOTP bool   `json:"requires_totp"`
	CSRFToken    string `json:"csrf_token"` // for the verify-totp call
}

// VerifyTOTPRequest is the 2FA code submission
type VerifyTOTPRequest struct {
	Code string `json:"code"`
}

// RegisterRequest for first-user registration
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
}

// ChangePasswordRequest to change own password
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// TOTPSetupResponse when starting 2FA enrollment
type TOTPSetupResponse struct {
	Secret        string   `json:"secret"`
	QRCodeDataURL string   `json:"qr_code_data_url"`
	RecoveryCodes []string `json:"recovery_codes"`
}

// TOTPVerifySetupRequest to confirm 2FA enrollment
type TOTPVerifySetupRequest struct {
	Code string `json:"code"`
}

// CreateUserRequest for admin creating users
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role,omitempty"` // defaults to "user"
}

// MeResponse for /auth/me endpoint
type MeResponse struct {
	User      *User  `json:"user"`
	CSRFToken string `json:"csrf_token"`
}
