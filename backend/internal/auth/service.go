package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserExists        = errors.New("username already exists")
	ErrAccountLocked     = errors.New("account is locked")
	ErrTOTPRequired      = errors.New("TOTP verification required")
	ErrInvalidTOTP       = errors.New("invalid TOTP code")
	ErrSessionNotFound   = errors.New("session not found")
	ErrSessionExpired    = errors.New("session expired")
	ErrInvalidCSRF       = errors.New("invalid CSRF token")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrRegistrationClosed = errors.New("registration is closed")
)

// Config holds all auth configuration
type Config struct {
	Enabled       bool
	DatabasePath  string
	EncryptionKey string

	Session  SessionConfig
	Lockout  LockoutConfig
	TOTP     TOTPConfig
	RateLimit RateLimitConfig
}

// LockoutConfig defines account lockout behavior
type LockoutConfig struct {
	MaxAttempts    int
	LockoutMinutes int
	Progressive    bool // double lockout time on repeated lockouts
}

// TOTPConfig defines 2FA settings
type TOTPConfig struct {
	Enabled       bool
	Required      bool   // force all users to enable 2FA
	Issuer        string // shown in authenticator apps
	RecoveryCodes int    // number of recovery codes to generate
	TrustIPHours  int    // skip 2FA for trusted IPs (0=disabled)
}

// RateLimitConfig defines login rate limiting
type RateLimitConfig struct {
	RequestsPerMinute int
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Enabled:      true,
		DatabasePath: "data/auth.db",
		Session: SessionConfig{
			DurationHours:    24,
			IdleTimeoutHours: 8,
			CookieSecure:     false, // true in production
		},
		Lockout: LockoutConfig{
			MaxAttempts:    5,
			LockoutMinutes: 15,
			Progressive:    true,
		},
		TOTP: TOTPConfig{
			Enabled:       true,
			Required:      false,
			Issuer:        "pCenter",
			RecoveryCodes: 10,
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: 10,
		},
	}
}

// Service handles authentication operations
type Service struct {
	db     *DB
	crypto *Crypto
	cfg    Config

	// Per-IP rate limiters for login attempts
	limiters   map[string]*rate.Limiter
	limitersMu sync.Mutex
}

// NewService creates a new auth service
func NewService(db *DB, crypto *Crypto, cfg Config) *Service {
	return &Service{
		db:       db,
		crypto:   crypto,
		cfg:      cfg,
		limiters: make(map[string]*rate.Limiter),
	}
}

// GetDB returns the underlying database (for handlers that need direct access)
func (s *Service) GetDB() *DB {
	return s.db
}

// GetConfig returns the auth configuration
func (s *Service) GetConfig() Config {
	return s.cfg
}

// GetCrypto returns the crypto helper
func (s *Service) GetCrypto() *Crypto {
	return s.crypto
}

// --- Rate Limiting ---

func (s *Service) getRateLimiter(ip string) *rate.Limiter {
	s.limitersMu.Lock()
	defer s.limitersMu.Unlock()

	if limiter, ok := s.limiters[ip]; ok {
		return limiter
	}

	// Token bucket: requestsPerMinute tokens, refill 1 per (60/rpm) seconds
	rpm := float64(s.cfg.RateLimit.RequestsPerMinute)
	limiter := rate.NewLimiter(rate.Limit(rpm/60), s.cfg.RateLimit.RequestsPerMinute)
	s.limiters[ip] = limiter

	return limiter
}

// CheckRateLimit returns true if request is allowed
func (s *Service) CheckRateLimit(ip string) bool {
	return s.getRateLimiter(ip).Allow()
}

// --- User Operations ---

// Register creates the first admin user (or fails if users exist)
func (s *Service) Register(ctx context.Context, req RegisterRequest, ip string) (*User, *Session, string, error) {
	// Check if users already exist
	count, err := s.db.UserCount(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	if count > 0 {
		return nil, nil, "", ErrRegistrationClosed
	}

	// Validate password
	if err := ValidatePasswordPolicy(req.Password); err != nil {
		return nil, nil, "", err
	}

	// Hash password
	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, nil, "", err
	}

	// Create admin user
	user := &User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
		Role:         RoleAdmin, // first user is always admin
	}

	if err := s.db.CreateUser(ctx, user); err != nil {
		return nil, nil, "", err
	}

	// Log event
	s.db.LogEvent(ctx, &AuthEvent{
		UserID:    user.ID,
		Username:  user.Username,
		EventType: EventUserCreated,
		IPAddress: ip,
		Details:   "first user registration",
		Success:   true,
	})

	// Create session
	session, rawToken, err := NewSession(user.ID, ip, s.cfg.Session)
	if err != nil {
		return nil, nil, "", err
	}
	session.TOTPVerified = true // no 2FA for first user

	if err := s.db.CreateSession(ctx, session); err != nil {
		return nil, nil, "", err
	}

	slog.Info("first user registered", "username", user.Username, "role", user.Role)

	return user, session, rawToken, nil
}

// Login authenticates a user and returns a session
func (s *Service) Login(ctx context.Context, req LoginRequest, ip string) (*User, *Session, string, error) {
	// Get user
	user, err := s.db.GetUserByUsername(ctx, req.Username)
	if err != nil {
		return nil, nil, "", err
	}
	if user == nil {
		// Log failed attempt (unknown user)
		s.db.LogEvent(ctx, &AuthEvent{
			Username:  req.Username,
			EventType: EventLoginFailed,
			IPAddress: ip,
			Details:   "user not found",
			Success:   false,
		})
		return nil, nil, "", ErrInvalidCredentials
	}

	// Check if account is locked
	if !user.LockedUntil.IsZero() && time.Now().Before(user.LockedUntil) {
		s.db.LogEvent(ctx, &AuthEvent{
			UserID:    user.ID,
			Username:  user.Username,
			EventType: EventLoginFailed,
			IPAddress: ip,
			Details:   "account locked",
			Success:   false,
		})
		return nil, nil, "", ErrAccountLocked
	}

	// Verify password
	if !VerifyPassword(req.Password, user.PasswordHash) {
		// Increment failed attempts
		s.db.IncrementFailedAttempts(ctx, user.ID)
		user.FailedAttempts++

		s.db.LogEvent(ctx, &AuthEvent{
			UserID:    user.ID,
			Username:  user.Username,
			EventType: EventLoginFailed,
			IPAddress: ip,
			Details:   "invalid password",
			Success:   false,
		})

		// Check if we should lock the account
		if user.FailedAttempts >= s.cfg.Lockout.MaxAttempts {
			lockoutDuration := time.Duration(s.cfg.Lockout.LockoutMinutes) * time.Minute
			if s.cfg.Lockout.Progressive && user.FailedAttempts > s.cfg.Lockout.MaxAttempts {
				// Double lockout time for repeated failures
				multiplier := (user.FailedAttempts - s.cfg.Lockout.MaxAttempts) / s.cfg.Lockout.MaxAttempts + 1
				lockoutDuration *= time.Duration(multiplier)
			}
			lockUntil := time.Now().Add(lockoutDuration)
			s.db.LockUser(ctx, user.ID, lockUntil)

			s.db.LogEvent(ctx, &AuthEvent{
				UserID:    user.ID,
				Username:  user.Username,
				EventType: EventAccountLocked,
				IPAddress: ip,
				Details:   lockUntil.Format(time.RFC3339),
				Success:   true,
			})
		}

		return nil, nil, "", ErrInvalidCredentials
	}

	// Reset failed attempts on successful password
	if user.FailedAttempts > 0 {
		s.db.ResetFailedAttempts(ctx, user.ID)
	}

	// Create session
	session, rawToken, err := NewSession(user.ID, ip, s.cfg.Session)
	if err != nil {
		return nil, nil, "", err
	}

	// If user has 2FA enabled, check if IP is trusted
	if user.TOTPEnabled {
		// Check if this IP is trusted (already verified 2FA recently)
		trusted, _ := s.db.IsTrustedIP(ctx, user.ID, ip)
		if trusted && s.cfg.TOTP.TrustIPHours > 0 {
			session.TOTPVerified = true
			slog.Debug("skipping 2FA for trusted IP", "user", user.Username, "ip", ip)
		} else {
			session.TOTPVerified = false
		}
	} else {
		session.TOTPVerified = true
	}

	if err := s.db.CreateSession(ctx, session); err != nil {
		return nil, nil, "", err
	}

	// Update last login
	s.db.UpdateLastLogin(ctx, user.ID)

	// Log successful login
	s.db.LogEvent(ctx, &AuthEvent{
		UserID:    user.ID,
		Username:  user.Username,
		EventType: EventLogin,
		IPAddress: ip,
		Success:   true,
	})

	return user, session, rawToken, nil
}

// VerifyTOTPLogin verifies a TOTP code and marks session as verified
func (s *Service) VerifyTOTPLogin(ctx context.Context, session *Session, code string) error {
	user, err := s.db.GetUserByID(ctx, session.UserID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	// Decrypt TOTP secret
	secret, err := s.crypto.Decrypt(user.TOTPSecret)
	if err != nil {
		return err
	}

	// Try TOTP code first
	if ValidateTOTP(secret, code) {
		if err := s.db.MarkTOTPVerified(ctx, session.ID); err != nil {
			return err
		}
		// Trust this IP for future logins
		s.trustIPAfterVerification(ctx, user.ID, session.IPAddress)
		return nil
	}

	// Try recovery codes
	codes, err := s.db.GetRecoveryCodes(ctx, user.ID)
	if err != nil {
		return err
	}

	for _, rc := range codes {
		if !rc.Used && VerifyPassword(code, rc.CodeHash) {
			// Mark recovery code as used
			if err := s.db.MarkRecoveryCodeUsed(ctx, rc.ID); err != nil {
				return err
			}
			if err := s.db.MarkTOTPVerified(ctx, session.ID); err != nil {
				return err
			}

			s.db.LogEvent(ctx, &AuthEvent{
				UserID:    user.ID,
				Username:  user.Username,
				EventType: EventRecoveryUsed,
				IPAddress: session.IPAddress,
				Success:   true,
			})

			// Trust this IP for future logins
			s.trustIPAfterVerification(ctx, user.ID, session.IPAddress)
			return nil
		}
	}

	return ErrInvalidTOTP
}

// trustIPAfterVerification adds the IP to trusted list if feature is enabled
func (s *Service) trustIPAfterVerification(ctx context.Context, userID, ip string) {
	if s.cfg.TOTP.TrustIPHours <= 0 || ip == "" {
		return
	}
	expiresAt := time.Now().Add(time.Duration(s.cfg.TOTP.TrustIPHours) * time.Hour)
	if err := s.db.TrustIP(ctx, userID, ip, expiresAt); err != nil {
		slog.Warn("failed to trust IP", "error", err, "userID", userID, "ip", ip)
	} else {
		slog.Debug("IP trusted for 2FA skip", "userID", userID, "ip", ip, "expiresAt", expiresAt)
	}
}

// Logout destroys a session
func (s *Service) Logout(ctx context.Context, tokenHash string, userID string, ip string) error {
	if err := s.db.DeleteSessionByTokenHash(ctx, tokenHash); err != nil {
		return err
	}

	s.db.LogEvent(ctx, &AuthEvent{
		UserID:    userID,
		EventType: EventLogout,
		IPAddress: ip,
		Success:   true,
	})

	return nil
}

// ValidateSession checks if a session token is valid and returns user
func (s *Service) ValidateSession(ctx context.Context, rawToken string) (*Session, *User, error) {
	tokenHash, err := HashSessionToken(rawToken)
	if err != nil {
		return nil, nil, ErrSessionNotFound
	}

	session, err := s.db.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, nil, err
	}
	if session == nil {
		return nil, nil, ErrSessionNotFound
	}

	// Check expiration
	if session.IsExpired() {
		s.db.DeleteSession(ctx, session.ID)
		return nil, nil, ErrSessionExpired
	}

	// Check idle timeout
	if session.IsIdle(s.cfg.Session.IdleTimeoutHours) {
		s.db.DeleteSession(ctx, session.ID)
		return nil, nil, ErrSessionExpired
	}

	// Get user
	user, err := s.db.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		s.db.DeleteSession(ctx, session.ID)
		return nil, nil, ErrUserNotFound
	}

	// Touch session (update last_seen)
	s.db.TouchSession(ctx, session.ID)

	return session, user, nil
}

// ChangePassword changes a user's password
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword, ip string) error {
	user, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	// Verify current password
	if !VerifyPassword(currentPassword, user.PasswordHash) {
		return ErrInvalidCredentials
	}

	// Validate new password
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return err
	}

	// Hash new password
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	if err := s.db.UpdateUser(ctx, user); err != nil {
		return err
	}

	s.db.LogEvent(ctx, &AuthEvent{
		UserID:    user.ID,
		Username:  user.Username,
		EventType: EventPasswordChanged,
		IPAddress: ip,
		Success:   true,
	})

	return nil
}

// CreateUser creates a new user (admin only)
func (s *Service) CreateUser(ctx context.Context, req CreateUserRequest, creatorID, ip string) (*User, error) {
	// Check if username exists
	existing, err := s.db.GetUserByUsername(ctx, req.Username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUserExists
	}

	// Validate password
	if err := ValidatePasswordPolicy(req.Password); err != nil {
		return nil, err
	}

	// Hash password
	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	role := req.Role
	if role == "" {
		role = RoleUser
	}

	user := &User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
		Role:         role,
	}

	if err := s.db.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	s.db.LogEvent(ctx, &AuthEvent{
		UserID:    creatorID,
		Username:  req.Username,
		EventType: EventUserCreated,
		IPAddress: ip,
		Details:   "created by admin",
		Success:   true,
	})

	slog.Info("user created", "username", user.Username, "role", user.Role, "creator", creatorID)

	return user, nil
}

// DeleteUser removes a user (admin only)
func (s *Service) DeleteUser(ctx context.Context, userID, deleterID, ip string) error {
	user, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	// Delete all sessions first
	s.db.DeleteUserSessions(ctx, userID, "")
	s.db.DeleteUserRecoveryCodes(ctx, userID)

	if err := s.db.DeleteUser(ctx, userID); err != nil {
		return err
	}

	s.db.LogEvent(ctx, &AuthEvent{
		UserID:    deleterID,
		Username:  user.Username,
		EventType: EventUserDeleted,
		IPAddress: ip,
		Success:   true,
	})

	slog.Info("user deleted", "username", user.Username, "deleter", deleterID)

	return nil
}

// ListUsers returns all users (admin only)
func (s *Service) ListUsers(ctx context.Context) ([]*User, error) {
	return s.db.ListUsers(ctx)
}

// UserCount returns total number of users
func (s *Service) UserCount(ctx context.Context) (int, error) {
	return s.db.UserCount(ctx)
}

// GetUserByID returns a user by ID
func (s *Service) GetUserByID(ctx context.Context, id string) (*User, error) {
	return s.db.GetUserByID(ctx, id)
}

// --- Session Management ---

// ListUserSessions returns all sessions for a user
func (s *Service) ListUserSessions(ctx context.Context, userID string) ([]*Session, error) {
	return s.db.ListUserSessions(ctx, userID)
}

// RevokeSession revokes a specific session
func (s *Service) RevokeSession(ctx context.Context, sessionID, userID, ip string) error {
	session, err := s.db.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil || session.UserID != userID {
		return ErrSessionNotFound
	}

	if err := s.db.DeleteSession(ctx, sessionID); err != nil {
		return err
	}

	s.db.LogEvent(ctx, &AuthEvent{
		UserID:    userID,
		EventType: EventSessionRevoked,
		IPAddress: ip,
		Success:   true,
	})

	return nil
}

// CleanupExpiredSessions removes expired sessions and trusted IPs (called periodically)
func (s *Service) CleanupExpiredSessions(ctx context.Context) error {
	count, err := s.db.DeleteExpiredSessions(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		slog.Debug("cleaned up expired sessions", "count", count)
	}

	// Also cleanup expired trusted IPs
	ipCount, err := s.db.CleanupExpiredTrustedIPs(ctx)
	if err != nil {
		return err
	}
	if ipCount > 0 {
		slog.Debug("cleaned up expired trusted IPs", "count", ipCount)
	}
	return nil
}

// --- Audit Log ---

// ListAuthEvents returns recent auth events (admin only)
func (s *Service) ListAuthEvents(ctx context.Context, limit int) ([]*AuthEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.db.ListEvents(ctx, limit)
}

// --- Cookie helpers ---

// SessionCookieName is the name of the session cookie
const SessionCookieName = "pcenter_session"

// SetSessionCookie sets the session cookie on the response
func (s *Service) SetSessionCookie(w http.ResponseWriter, rawToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.Session.CookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   s.cfg.Session.DurationHours * 3600,
	})
}

// ClearSessionCookie removes the session cookie
func (s *Service) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.Session.CookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// GetSessionCookie extracts session token from cookie
func GetSessionCookie(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
