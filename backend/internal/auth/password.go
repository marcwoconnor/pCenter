package auth

import (
	"errors"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const (
	// MinPasswordLength is minimum password length
	MinPasswordLength = 8
)

// bcryptCost is the bcrypt work factor. Default is 12 (~300ms on modern hardware).
// Tests override this to bcrypt.MinCost (4) to avoid timeouts under the race detector.
var bcryptCost = 12

var (
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
	ErrPasswordNoUpper    = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLower    = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoDigit    = errors.New("password must contain at least one digit")
	ErrInvalidCredentials = errors.New("invalid username or password")
)

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword compares a password with its hash using constant-time comparison
func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ValidatePasswordPolicy checks if password meets requirements
func ValidatePasswordPolicy(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}

	var hasUpper, hasLower, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}

	if !hasUpper {
		return ErrPasswordNoUpper
	}
	if !hasLower {
		return ErrPasswordNoLower
	}
	if !hasDigit {
		return ErrPasswordNoDigit
	}

	return nil
}
