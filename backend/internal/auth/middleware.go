package auth

import (
	"context"
	"net/http"
)

// Context keys for auth data
type contextKey string

const (
	contextKeySession contextKey = "auth_session"
	contextKeyUser    contextKey = "auth_user"
)

// CSRFHeader is the expected header for CSRF token
const CSRFHeader = "X-CSRF-Token"

// GetAuthContext retrieves session and user from context
func GetAuthContext(ctx context.Context) (*Session, *User) {
	session, _ := ctx.Value(contextKeySession).(*Session)
	user, _ := ctx.Value(contextKeyUser).(*User)
	return session, user
}

// SetAuthContext stores session and user in context
func SetAuthContext(ctx context.Context, session *Session, user *User) context.Context {
	ctx = context.WithValue(ctx, contextKeySession, session)
	ctx = context.WithValue(ctx, contextKeyUser, user)
	return ctx
}

// RequireAuth middleware ensures request has valid session
// Sets session and user in context for handlers
func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken := GetSessionCookie(r)
		if rawToken == "" {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		session, user, err := s.ValidateSession(r.Context(), rawToken)
		if err != nil {
			s.ClearSessionCookie(w)
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if 2FA verification is pending
		// For partial sessions, only allow /auth/verify-totp and /auth/logout
		if user.TOTPEnabled && !session.TOTPVerified {
			path := r.URL.Path
			if path != "/api/auth/verify-totp" && path != "/api/auth/logout" && path != "/api/auth/me" {
				writeError(w, "2FA verification required", http.StatusForbidden)
				return
			}
		}

		// Add to context
		ctx := SetAuthContext(r.Context(), session, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin middleware ensures user has admin role
func (s *Service) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, user := GetAuthContext(r.Context())
		if user == nil {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if user.Role != RoleAdmin {
			writeError(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireFullAuth ensures 2FA is verified (if enabled)
func (s *Service) RequireFullAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, user := GetAuthContext(r.Context())
		if session == nil || user == nil {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if user.TOTPEnabled && !session.TOTPVerified {
			writeError(w, "2FA verification required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CSRFProtection middleware validates CSRF token for state-changing requests
func (s *Service) CSRFProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip CSRF for safe methods
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		session, _ := GetAuthContext(r.Context())
		if session == nil {
			// No session = no CSRF check (will fail RequireAuth anyway)
			next.ServeHTTP(w, r)
			return
		}

		// Check CSRF token
		csrfToken := r.Header.Get(CSRFHeader)
		if csrfToken == "" {
			writeError(w, "CSRF token required", http.StatusForbidden)
			return
		}

		if csrfToken != session.CSRFToken {
			writeError(w, "invalid CSRF token", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// OptionalAuth middleware adds auth context if available, but doesn't require it
// Useful for endpoints that behave differently for logged-in vs anonymous users
func (s *Service) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken := GetSessionCookie(r)
		if rawToken != "" {
			session, user, err := s.ValidateSession(r.Context(), rawToken)
			if err == nil {
				ctx := SetAuthContext(r.Context(), session, user)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// AuthenticatedWebSocket wraps WebSocket handler with cookie auth
func (s *Service) AuthenticatedWebSocket(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawToken := GetSessionCookie(r)
		if rawToken == "" {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		session, user, err := s.ValidateSession(r.Context(), rawToken)
		if err != nil {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// For WebSocket, require full auth (2FA verified if enabled)
		if user.TOTPEnabled && !session.TOTPVerified {
			writeError(w, "2FA verification required", http.StatusForbidden)
			return
		}

		ctx := SetAuthContext(r.Context(), session, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
