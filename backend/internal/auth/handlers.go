package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// Handlers wraps the auth service for HTTP handling
type Handlers struct {
	svc *Service
}

// NewHandlers creates HTTP handlers for auth endpoints
func NewHandlers(svc *Service) *Handlers {
	return &Handlers{svc: svc}
}

// --- Helper functions ---

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	addr := r.RemoteAddr
	// Strip port
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// --- Public Handlers ---

// HandleLogin handles POST /api/auth/login
func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ip := getClientIP(r)

	// Rate limit check
	if !h.svc.CheckRateLimit(ip) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, "username and password required", http.StatusBadRequest)
		return
	}

	user, session, rawToken, err := h.svc.Login(r.Context(), req, ip)
	if err != nil {
		switch err {
		case ErrInvalidCredentials:
			writeError(w, "invalid username or password", http.StatusUnauthorized)
		case ErrAccountLocked:
			writeError(w, "account is locked", http.StatusForbidden)
		default:
			slog.Error("login error", "error", err)
			writeError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	// Set session cookie
	h.svc.SetSessionCookie(w, rawToken)

	// If 2FA is required, return partial response
	if user.TOTPEnabled && !session.TOTPVerified {
		writeJSON(w, TOTPRequiredResponse{
			RequiresTOTP: true,
			CSRFToken:    session.CSRFToken,
		})
		return
	}

	// Full login success
	writeJSON(w, LoginResponse{
		User:      user,
		CSRFToken: session.CSRFToken,
	})
}

// HandleVerifyTOTP handles POST /api/auth/verify-totp
func (h *Handlers) HandleVerifyTOTP(w http.ResponseWriter, r *http.Request) {
	session, user := GetAuthContext(r.Context())
	if session == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Already verified?
	if session.TOTPVerified {
		writeJSON(w, LoginResponse{
			User:      user,
			CSRFToken: session.CSRFToken,
		})
		return
	}

	var req VerifyTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Normalize code (remove spaces/dashes)
	code := strings.ReplaceAll(strings.ReplaceAll(req.Code, " ", ""), "-", "")

	if err := h.svc.VerifyTOTPLogin(r.Context(), session, code); err != nil {
		if err == ErrInvalidTOTP {
			writeError(w, "invalid code", http.StatusUnauthorized)
		} else {
			slog.Error("verify totp error", "error", err)
			writeError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	// Reload session to get updated state
	session.TOTPVerified = true

	writeJSON(w, LoginResponse{
		User:      user,
		CSRFToken: session.CSRFToken,
	})
}

// HandleLogout handles POST /api/auth/logout
func (h *Handlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := GetAuthContext(r.Context())
	ip := getClientIP(r)

	if session != nil {
		h.svc.Logout(r.Context(), session.TokenHash, session.UserID, ip)
	}

	h.svc.ClearSessionCookie(w)
	writeJSON(w, map[string]bool{"success": true})
}

// HandleRegister handles POST /api/auth/register (first user only)
func (h *Handlers) HandleRegister(w http.ResponseWriter, r *http.Request) {
	ip := getClientIP(r)

	// Rate limit
	if !h.svc.CheckRateLimit(ip) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, "username and password required", http.StatusBadRequest)
		return
	}

	user, session, rawToken, err := h.svc.Register(r.Context(), req, ip)
	if err != nil {
		switch err {
		case ErrRegistrationClosed:
			writeError(w, "registration is closed", http.StatusForbidden)
		case ErrPasswordTooShort, ErrPasswordNoUpper, ErrPasswordNoLower, ErrPasswordNoDigit:
			writeError(w, err.Error(), http.StatusBadRequest)
		default:
			slog.Error("register error", "error", err)
			writeError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	h.svc.SetSessionCookie(w, rawToken)
	writeJSON(w, LoginResponse{
		User:      user,
		CSRFToken: session.CSRFToken,
	})
}

// HandleMe handles GET /api/auth/me
func (h *Handlers) HandleMe(w http.ResponseWriter, r *http.Request) {
	session, user := GetAuthContext(r.Context())
	if session == nil || user == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// If 2FA required but not verified, return partial info
	if user.TOTPEnabled && !session.TOTPVerified {
		writeJSON(w, map[string]any{
			"requires_totp": true,
			"csrf_token":    session.CSRFToken,
		})
		return
	}

	writeJSON(w, MeResponse{
		User:      user,
		CSRFToken: session.CSRFToken,
	})
}

// HandleChangePassword handles PUT /api/auth/password
func (h *Handlers) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	session, user := GetAuthContext(r.Context())
	if session == nil || user == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ip := getClientIP(r)
	if err := h.svc.ChangePassword(r.Context(), user.ID, req.CurrentPassword, req.NewPassword, ip); err != nil {
		switch err {
		case ErrInvalidCredentials:
			writeError(w, "current password is incorrect", http.StatusUnauthorized)
		case ErrPasswordTooShort, ErrPasswordNoUpper, ErrPasswordNoLower, ErrPasswordNoDigit:
			writeError(w, err.Error(), http.StatusBadRequest)
		default:
			slog.Error("change password error", "error", err)
			writeError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, map[string]bool{"success": true})
}

// --- TOTP Setup Handlers ---

// HandleTOTPSetup handles POST /api/auth/totp/setup
func (h *Handlers) HandleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	session, user := GetAuthContext(r.Context())
	if session == nil || user == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Already enabled?
	if user.TOTPEnabled {
		writeError(w, "2FA is already enabled", http.StatusBadRequest)
		return
	}

	// Generate TOTP secret
	cfg := h.svc.GetConfig()
	key, err := GenerateTOTPSecret(user.Username, cfg.TOTP.Issuer)
	if err != nil {
		slog.Error("generate totp secret error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Generate QR code
	qrDataURL, err := GenerateQRCodeDataURL(key)
	if err != nil {
		slog.Error("generate qr code error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Generate recovery codes
	codes, err := GenerateRecoveryCodes(cfg.TOTP.RecoveryCodes)
	if err != nil {
		slog.Error("generate recovery codes error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Hash recovery codes
	hashes, err := HashRecoveryCodes(codes)
	if err != nil {
		slog.Error("hash recovery codes error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Encrypt and store TOTP secret (but don't enable yet)
	crypto := h.svc.GetCrypto()
	encryptedSecret, err := crypto.Encrypt(key.Secret())
	if err != nil {
		slog.Error("encrypt totp secret error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Update user with secret (not enabled yet - verified in next step)
	user.TOTPSecret = encryptedSecret
	if err := h.svc.GetDB().UpdateUser(r.Context(), user); err != nil {
		slog.Error("save totp secret error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Save recovery codes (replace any existing)
	if err := h.svc.GetDB().CreateRecoveryCodes(r.Context(), user.ID, hashes); err != nil {
		slog.Error("save recovery codes error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, TOTPSetupResponse{
		Secret:        key.Secret(),
		QRCodeDataURL: qrDataURL,
		RecoveryCodes: codes,
	})
}

// HandleTOTPVerifySetup handles POST /api/auth/totp/verify-setup
func (h *Handlers) HandleTOTPVerifySetup(w http.ResponseWriter, r *http.Request) {
	session, user := GetAuthContext(r.Context())
	if session == nil || user == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Already enabled?
	if user.TOTPEnabled {
		writeError(w, "2FA is already enabled", http.StatusBadRequest)
		return
	}

	// Must have secret set from setup step
	if user.TOTPSecret == "" {
		writeError(w, "run setup first", http.StatusBadRequest)
		return
	}

	var req TOTPVerifySetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Decrypt secret
	crypto := h.svc.GetCrypto()
	secret, err := crypto.Decrypt(user.TOTPSecret)
	if err != nil {
		slog.Error("decrypt totp secret error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Validate code
	code := strings.ReplaceAll(strings.ReplaceAll(req.Code, " ", ""), "-", "")
	if !ValidateTOTP(secret, code) {
		writeError(w, "invalid code", http.StatusUnauthorized)
		return
	}

	// Enable 2FA
	user.TOTPEnabled = true
	if err := h.svc.GetDB().UpdateUser(r.Context(), user); err != nil {
		slog.Error("enable totp error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	ip := getClientIP(r)
	h.svc.GetDB().LogEvent(r.Context(), &AuthEvent{
		UserID:    user.ID,
		Username:  user.Username,
		EventType: EventTOTPEnabled,
		IPAddress: ip,
		Success:   true,
	})

	slog.Info("2FA enabled", "user", user.Username)

	writeJSON(w, map[string]bool{"success": true})
}

// HandleTOTPDisable handles DELETE /api/auth/totp
func (h *Handlers) HandleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	session, user := GetAuthContext(r.Context())
	if session == nil || user == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !user.TOTPEnabled {
		writeError(w, "2FA is not enabled", http.StatusBadRequest)
		return
	}

	// Disable 2FA
	user.TOTPEnabled = false
	user.TOTPSecret = ""
	if err := h.svc.GetDB().UpdateUser(r.Context(), user); err != nil {
		slog.Error("disable totp error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Delete recovery codes
	h.svc.GetDB().DeleteUserRecoveryCodes(r.Context(), user.ID)

	ip := getClientIP(r)
	h.svc.GetDB().LogEvent(r.Context(), &AuthEvent{
		UserID:    user.ID,
		Username:  user.Username,
		EventType: EventTOTPDisabled,
		IPAddress: ip,
		Success:   true,
	})

	slog.Info("2FA disabled", "user", user.Username)

	writeJSON(w, map[string]bool{"success": true})
}

// --- Session Management ---

// HandleListSessions handles GET /api/auth/sessions
func (h *Handlers) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	session, user := GetAuthContext(r.Context())
	if session == nil || user == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessions, err := h.svc.ListUserSessions(r.Context(), user.ID)
	if err != nil {
		slog.Error("list sessions error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Mark current session
	type sessionResponse struct {
		*Session
		Current bool `json:"current"`
	}
	resp := make([]sessionResponse, len(sessions))
	for i, s := range sessions {
		resp[i] = sessionResponse{
			Session: s,
			Current: s.ID == session.ID,
		}
	}

	writeJSON(w, resp)
}

// HandleRevokeSession handles DELETE /api/auth/sessions/{id}
func (h *Handlers) HandleRevokeSession(w http.ResponseWriter, r *http.Request) {
	session, user := GetAuthContext(r.Context())
	if session == nil || user == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, "session id required", http.StatusBadRequest)
		return
	}

	// Can't revoke current session via this endpoint
	if sessionID == session.ID {
		writeError(w, "use /logout to end current session", http.StatusBadRequest)
		return
	}

	ip := getClientIP(r)
	if err := h.svc.RevokeSession(r.Context(), sessionID, user.ID, ip); err != nil {
		if err == ErrSessionNotFound {
			writeError(w, "session not found", http.StatusNotFound)
		} else {
			slog.Error("revoke session error", "error", err)
			writeError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, map[string]bool{"success": true})
}

// --- Admin Handlers ---

// HandleListUsers handles GET /api/users
func (h *Handlers) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListUsers(r.Context())
	if err != nil {
		slog.Error("list users error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, users)
}

// HandleCreateUser handles POST /api/users
func (h *Handlers) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	session, _ := GetAuthContext(r.Context())
	if session == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, "username and password required", http.StatusBadRequest)
		return
	}

	ip := getClientIP(r)
	user, err := h.svc.CreateUser(r.Context(), req, session.UserID, ip)
	if err != nil {
		switch err {
		case ErrUserExists:
			writeError(w, "username already exists", http.StatusConflict)
		case ErrPasswordTooShort, ErrPasswordNoUpper, ErrPasswordNoLower, ErrPasswordNoDigit:
			writeError(w, err.Error(), http.StatusBadRequest)
		default:
			slog.Error("create user error", "error", err)
			writeError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, user)
}

// HandleDeleteUser handles DELETE /api/users/{id}
func (h *Handlers) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	session, adminUser := GetAuthContext(r.Context())
	if session == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, "user id required", http.StatusBadRequest)
		return
	}

	// Can't delete yourself
	if userID == adminUser.ID {
		writeError(w, "cannot delete yourself", http.StatusBadRequest)
		return
	}

	ip := getClientIP(r)
	if err := h.svc.DeleteUser(r.Context(), userID, session.UserID, ip); err != nil {
		if err == ErrUserNotFound {
			writeError(w, "user not found", http.StatusNotFound)
		} else {
			slog.Error("delete user error", "error", err)
			writeError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, map[string]bool{"success": true})
}

// HandleListEvents handles GET /api/auth/events
func (h *Handlers) HandleListEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	events, err := h.svc.ListAuthEvents(r.Context(), limit)
	if err != nil {
		slog.Error("list events error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, events)
}

// HandleCheckAuth handles GET /api/auth/check - lightweight endpoint for auth status
func (h *Handlers) HandleCheckAuth(w http.ResponseWriter, r *http.Request) {
	session, user := GetAuthContext(r.Context())

	if session == nil || user == nil {
		writeJSON(w, map[string]any{
			"authenticated": false,
		})
		return
	}

	// Check if 2FA is pending
	if user.TOTPEnabled && !session.TOTPVerified {
		writeJSON(w, map[string]any{
			"authenticated": true,
			"requires_totp": true,
		})
		return
	}

	writeJSON(w, map[string]any{
		"authenticated": true,
		"user":          user,
	})
}

// HandleUserCount handles GET /api/auth/user-count - for first-user check
func (h *Handlers) HandleUserCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.svc.UserCount(r.Context())
	if err != nil {
		slog.Error("user count error", "error", err)
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]int{"count": count})
}
