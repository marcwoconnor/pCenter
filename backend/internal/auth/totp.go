package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"image/png"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret creates a new TOTP secret for a user
func GenerateTOTPSecret(username, issuer string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1, // most compatible
	})
}

// GenerateQRCodeDataURL creates a data URL for the TOTP QR code
func GenerateQRCodeDataURL(key *otp.Key) (string, error) {
	img, err := key.Image(200, 200)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// ValidateTOTP checks if a TOTP code is valid for the given secret
// Allows ±1 step tolerance for clock drift
func ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

// GenerateRecoveryCodes creates a set of random recovery codes
func GenerateRecoveryCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		code, err := generateRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes[i] = code
	}
	return codes, nil
}

// generateRecoveryCode creates a single recovery code (format: XXXX-XXXX-XXXX)
func generateRecoveryCode() (string, error) {
	// 9 bytes = 72 bits of entropy, encoded as 3 groups of 4 chars
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// Use base32 for readability (no 0/O, 1/I confusion)
	encoded := base32.StdEncoding.EncodeToString(b)
	// Take first 12 chars, split into 3 groups
	code := fmt.Sprintf("%s-%s-%s", encoded[0:4], encoded[4:8], encoded[8:12])
	return strings.ToUpper(code), nil
}

// HashRecoveryCodes hashes recovery codes for storage
func HashRecoveryCodes(codes []string) ([]string, error) {
	hashes := make([]string, len(codes))
	for i, code := range codes {
		// Normalize: remove dashes, uppercase
		normalized := strings.ToUpper(strings.ReplaceAll(code, "-", ""))
		hash, err := HashPassword(normalized)
		if err != nil {
			return nil, err
		}
		hashes[i] = hash
	}
	return hashes, nil
}
