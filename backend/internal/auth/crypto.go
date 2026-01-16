package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

var (
	ErrInvalidKeyLength = errors.New("encryption key must be 32 bytes (64 hex chars)")
	ErrDecryptionFailed = errors.New("failed to decrypt data")
)

// Crypto handles encryption/decryption of sensitive data (TOTP secrets)
type Crypto struct {
	key []byte
}

// NewCrypto creates a new crypto helper from a hex-encoded 32-byte key
func NewCrypto(hexKey string) (*Crypto, error) {
	if hexKey == "" {
		// No encryption - return nil crypto (secrets stored plaintext)
		return nil, nil
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, errors.New("encryption key must be valid hex")
	}
	if len(key) != 32 {
		return nil, ErrInvalidKeyLength
	}

	return &Crypto{key: key}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM
func (c *Crypto) Encrypt(plaintext string) (string, error) {
	if c == nil {
		return plaintext, nil // no encryption configured
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext encrypted with Encrypt
func (c *Crypto) Decrypt(ciphertext string) (string, error) {
	if c == nil {
		return ciphertext, nil // no encryption configured
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrDecryptionFailed
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}

// GenerateKey generates a random 32-byte key and returns it as hex
// Useful for initial setup
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}
