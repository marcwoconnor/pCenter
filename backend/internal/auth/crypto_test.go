package auth

import (
	"testing"
)

// TestCrypto_RoundTrip verifies that encrypting and decrypting produces
// the original plaintext.
func TestCrypto_RoundTrip(t *testing.T) {
	// Generate a valid 32-byte key
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	crypto, err := NewCrypto(key)
	if err != nil {
		t.Fatalf("NewCrypto failed: %v", err)
	}

	plaintext := "my-totp-secret-base32"
	ciphertext, err := crypto.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if ciphertext == plaintext {
		t.Error("ciphertext should not equal plaintext")
	}

	decrypted, err := crypto.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("expected %q, got %q", plaintext, decrypted)
	}
}

// TestCrypto_DifferentCiphertexts verifies that encrypting the same
// plaintext twice produces different ciphertexts (AES-GCM uses random nonce).
func TestCrypto_DifferentCiphertexts(t *testing.T) {
	key, _ := GenerateKey()
	crypto, _ := NewCrypto(key)

	ct1, _ := crypto.Encrypt("same-text")
	ct2, _ := crypto.Encrypt("same-text")

	if ct1 == ct2 {
		t.Error("AES-GCM should produce different ciphertexts (random nonce)")
	}
}

// TestCrypto_WrongKey_FailsDecrypt verifies that decrypting with a different
// key fails.
func TestCrypto_WrongKey_FailsDecrypt(t *testing.T) {
	key1, _ := GenerateKey()
	key2, _ := GenerateKey()

	crypto1, _ := NewCrypto(key1)
	crypto2, _ := NewCrypto(key2)

	ciphertext, _ := crypto1.Encrypt("secret-data")

	_, err := crypto2.Decrypt(ciphertext)
	if err == nil {
		t.Error("decrypting with wrong key should fail")
	}
}

// TestCrypto_InvalidKeyLength verifies that keys with wrong length are
// rejected.
func TestCrypto_InvalidKeyLength(t *testing.T) {
	badKeys := []string{
		"short",               // too short
		"aabbccdd",            // 4 bytes
		"00112233445566778899", // 10 bytes (20 hex chars)
	}

	for _, key := range badKeys {
		_, err := NewCrypto(key)
		if err == nil {
			t.Errorf("key %q (%d hex chars) should be rejected", key, len(key))
		}
	}
}

// TestCrypto_EmptyKey_NilCrypto verifies that an empty key produces a nil
// Crypto (used when encryption is optional, e.g., in tests).
func TestCrypto_EmptyKey_NilCrypto(t *testing.T) {
	crypto, err := NewCrypto("")
	if err != nil {
		t.Fatalf("empty key should not error: %v", err)
	}
	if crypto != nil {
		t.Error("empty key should return nil Crypto")
	}
}

// TestGenerateKey_ValidLength verifies that GenerateKey produces a 64-char
// hex string (32 bytes).
func TestGenerateKey_ValidLength(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	if len(key) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(key))
	}

	// Should be valid for NewCrypto
	_, err = NewCrypto(key)
	if err != nil {
		t.Errorf("generated key should be valid: %v", err)
	}
}
