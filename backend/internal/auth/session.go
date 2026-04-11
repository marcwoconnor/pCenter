package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const (
	// TokenLength is the raw session token size in bytes (32 bytes = 256 bits)
	TokenLength = 32

	// CSRFTokenLength is the CSRF token size in bytes
	CSRFTokenLength = 32
)

// GenerateSessionToken creates a cryptographically secure random token
// Returns the raw token (for cookie) and its hash (for storage)
func GenerateSessionToken() (rawToken string, tokenHash string, err error) {
	token := make([]byte, TokenLength)
	if _, err := rand.Read(token); err != nil {
		return "", "", err
	}

	rawToken = base64.URLEncoding.EncodeToString(token)
	tokenHash = hashToken(token)

	return rawToken, tokenHash, nil
}

// GenerateCSRFToken creates a CSRF token
func GenerateCSRFToken() (string, error) {
	token := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(token), nil
}

// HashSessionToken creates SHA256 hash of a raw token for lookup
func HashSessionToken(rawToken string) (string, error) {
	token, err := base64.URLEncoding.DecodeString(rawToken)
	if err != nil {
		return "", err
	}
	return hashToken(token), nil
}

func hashToken(token []byte) string {
	hash := sha256.Sum256(token)
	return hex.EncodeToString(hash[:])
}

// SessionConfig holds session-related settings
type SessionConfig struct {
	DurationHours    int
	IdleTimeoutHours int
	CookieSecure     bool
	CookieDomain     string
}

// NewSession creates a new session for a user
func NewSession(userID string, ip string, cfg SessionConfig) (*Session, string, error) {
	rawToken, tokenHash, err := GenerateSessionToken()
	if err != nil {
		return nil, "", err
	}

	csrfToken, err := GenerateCSRFToken()
	if err != nil {
		return nil, "", err
	}

	now := time.Now()
	session := &Session{
		UserID:       userID,
		TokenHash:    tokenHash,
		CSRFToken:    csrfToken,
		TOTPVerified: false,
		IPAddress:    ip,
		CreatedAt:    now,
		ExpiresAt:    now.Add(time.Duration(cfg.DurationHours) * time.Hour),
		LastSeenAt:   now,
	}

	return session, rawToken, nil
}

// IsExpired checks if session is past its expiration time
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsIdle checks if session has been idle too long
func (s *Session) IsIdle(idleTimeoutHours int) bool {
	idleThreshold := s.LastSeenAt.Add(time.Duration(idleTimeoutHours) * time.Hour)
	return time.Now().After(idleThreshold)
}
