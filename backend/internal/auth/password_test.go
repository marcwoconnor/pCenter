package auth

import (
	"testing"
)

// TestHashPassword_RoundTrip verifies that hashing a password and then
// verifying it works correctly.
func TestHashPassword_RoundTrip(t *testing.T) {
	password := "MySecure1Password"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !VerifyPassword(password, hash) {
		t.Error("VerifyPassword should return true for correct password")
	}
}

// TestHashPassword_WrongPassword verifies that verification fails for
// incorrect passwords.
func TestHashPassword_WrongPassword(t *testing.T) {
	hash, _ := HashPassword("CorrectPassword1")

	if VerifyPassword("WrongPassword1", hash) {
		t.Error("VerifyPassword should return false for wrong password")
	}
}

// TestHashPassword_UniqueHashes verifies that the same password produces
// different hashes (bcrypt uses random salts).
func TestHashPassword_UniqueHashes(t *testing.T) {
	hash1, _ := HashPassword("SamePassword1")
	hash2, _ := HashPassword("SamePassword1")

	if hash1 == hash2 {
		t.Error("bcrypt should produce unique hashes for same password (random salt)")
	}
}

// TestValidatePasswordPolicy_ValidPasswords verifies that passwords meeting
// all requirements pass validation.
func TestValidatePasswordPolicy_ValidPasswords(t *testing.T) {
	valid := []string{
		"MyPass12",     // minimum length, has upper+lower+digit
		"Str0ngP@ss!",  // common strong password
		"Abcdefg1",     // exactly 8 chars
		"ABCDEFG1a",    // upper + lower + digit
	}

	for _, pw := range valid {
		if err := ValidatePasswordPolicy(pw); err != nil {
			t.Errorf("password %q should be valid, got error: %v", pw, err)
		}
	}
}

// TestValidatePasswordPolicy_TooShort verifies passwords under 8 characters
// are rejected.
func TestValidatePasswordPolicy_TooShort(t *testing.T) {
	err := ValidatePasswordPolicy("Short1A")
	if err == nil {
		t.Error("7-char password should be rejected")
	}
	if err != ErrPasswordTooShort {
		t.Errorf("expected ErrPasswordTooShort, got: %v", err)
	}
}

// TestValidatePasswordPolicy_NoUppercase verifies passwords without uppercase
// letters are rejected.
func TestValidatePasswordPolicy_NoUppercase(t *testing.T) {
	err := ValidatePasswordPolicy("lowercase1only")
	if err == nil {
		t.Error("password without uppercase should be rejected")
	}
	if err != ErrPasswordNoUpper {
		t.Errorf("expected ErrPasswordNoUpper, got: %v", err)
	}
}

// TestValidatePasswordPolicy_NoLowercase verifies passwords without lowercase
// letters are rejected.
func TestValidatePasswordPolicy_NoLowercase(t *testing.T) {
	err := ValidatePasswordPolicy("UPPERCASE1ONLY")
	if err == nil {
		t.Error("password without lowercase should be rejected")
	}
	if err != ErrPasswordNoLower {
		t.Errorf("expected ErrPasswordNoLower, got: %v", err)
	}
}

// TestValidatePasswordPolicy_NoDigit verifies passwords without digits
// are rejected.
func TestValidatePasswordPolicy_NoDigit(t *testing.T) {
	err := ValidatePasswordPolicy("NoDigitsHere")
	if err == nil {
		t.Error("password without digit should be rejected")
	}
	if err != ErrPasswordNoDigit {
		t.Errorf("expected ErrPasswordNoDigit, got: %v", err)
	}
}
