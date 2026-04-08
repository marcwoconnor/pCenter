package auth

import (
	"context"
	"testing"
)

// testService creates a fresh auth service with an in-memory SQLite database
// for testing. Each call returns a completely isolated instance.
func testService(t *testing.T) *Service {
	t.Helper()

	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	crypto, err := NewCrypto("")
	if err != nil {
		t.Fatalf("failed to create crypto: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Session.DurationHours = 24
	cfg.Session.IdleTimeoutHours = 8
	cfg.Lockout.MaxAttempts = 5
	cfg.Lockout.LockoutMinutes = 15
	cfg.RateLimit.RequestsPerMinute = 100 // high limit for tests

	return NewService(db, crypto, cfg)
}

// TestRegister_FirstUser_IsAdmin verifies that the first registered user
// gets the admin role.
func TestRegister_FirstUser_IsAdmin(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	user, session, _, err := svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if user.Role != RoleAdmin {
		t.Errorf("first user should be admin, got %q", user.Role)
	}
	if session == nil {
		t.Error("register should return a session")
	}
	if user.Username != "admin" {
		t.Errorf("expected username 'admin', got %q", user.Username)
	}
}

// TestRegister_SecondUser_Rejected verifies that after the first user is
// registered, additional registrations are rejected (admin creates users).
func TestRegister_SecondUser_Rejected(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	// First user succeeds
	_, _, _, err := svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	// Second user rejected
	_, _, _, err = svc.Register(ctx, RegisterRequest{
		Username: "user2",
		Password: "StrongPass1",
	}, "127.0.0.1")
	if err == nil {
		t.Error("second registration should be rejected")
	}
	if err != ErrRegistrationClosed {
		t.Errorf("expected ErrRegistrationClosed, got: %v", err)
	}
}

// TestLogin_ValidCredentials verifies successful login with correct
// username and password.
func TestLogin_ValidCredentials(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	// Register first
	_, _, _, _ = svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	// Login
	user, session, _, err := svc.Login(ctx, LoginRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("expected username 'admin', got %q", user.Username)
	}
	if session == nil {
		t.Error("login should return a session")
	}
}

// TestLogin_WrongPassword verifies that incorrect passwords are rejected.
func TestLogin_WrongPassword(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	_, _, _, _ = svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	_, _, _, err := svc.Login(ctx, LoginRequest{
		Username: "admin",
		Password: "WrongPass1",
	}, "127.0.0.1")

	if err == nil {
		t.Error("wrong password should fail login")
	}
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

// TestLogin_NonexistentUser verifies that login with unknown username fails.
func TestLogin_NonexistentUser(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	_, _, _, err := svc.Login(ctx, LoginRequest{
		Username: "nobody",
		Password: "StrongPass1",
	}, "127.0.0.1")

	if err == nil {
		t.Error("nonexistent user should fail login")
	}
}

// TestSession_ValidateAfterLogin verifies that the session token returned
// from login can be validated.
func TestSession_ValidateAfterLogin(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	_, _, _, _ = svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	_, _, rawToken, err := svc.Login(ctx, LoginRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	session, user, err := svc.ValidateSession(ctx, rawToken)
	if err != nil {
		t.Fatalf("ValidateSession failed: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("expected username 'admin', got %q", user.Username)
	}
	if session == nil {
		t.Error("ValidateSession should return a session")
	}
}

// TestSession_InvalidToken_Rejected verifies that a random/invalid token
// is rejected.
func TestSession_InvalidToken_Rejected(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	_, _, err := svc.ValidateSession(ctx, "invalid-random-token")
	if err == nil {
		t.Error("invalid token should fail validation")
	}
}

// TestLogout_InvalidatesSession verifies that after logout, the session
// token is no longer valid.
func TestLogout_InvalidatesSession(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	_, _, _, _ = svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	_, _, rawToken, _ := svc.Login(ctx, LoginRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	// Get the token hash for logout
	tokenHash, _ := HashSessionToken(rawToken)
	session, user, _ := svc.ValidateSession(ctx, rawToken)
	_ = session

	err := svc.Logout(ctx, tokenHash, user.ID, "127.0.0.1")
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	// Session should now be invalid
	_, _, err = svc.ValidateSession(ctx, rawToken)
	if err == nil {
		t.Error("session should be invalid after logout")
	}
}

// TestAccountLockout_AfterMaxAttempts verifies that the account is locked
// after too many failed login attempts.
func TestAccountLockout_AfterMaxAttempts(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	_, _, _, _ = svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	// Fail login 5 times (MaxAttempts = 5)
	for i := 0; i < 5; i++ {
		svc.Login(ctx, LoginRequest{
			Username: "admin",
			Password: "WrongPass1",
		}, "127.0.0.1")
	}

	// Next attempt should fail even with correct password
	_, _, _, err := svc.Login(ctx, LoginRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	if err == nil {
		t.Error("account should be locked after max failed attempts")
	}
	if err != ErrAccountLocked {
		t.Errorf("expected ErrAccountLocked, got: %v", err)
	}
}

// TestChangePassword verifies that changing password works and the old
// password no longer works.
func TestChangePassword(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	user, _, _, _ := svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "OldPass123",
	}, "127.0.0.1")

	err := svc.ChangePassword(ctx, user.ID, "OldPass123", "NewPass456", "127.0.0.1")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// Old password should fail
	_, _, _, err = svc.Login(ctx, LoginRequest{
		Username: "admin",
		Password: "OldPass123",
	}, "127.0.0.1")
	if err == nil {
		t.Error("old password should not work after change")
	}

	// New password should work
	_, _, _, err = svc.Login(ctx, LoginRequest{
		Username: "admin",
		Password: "NewPass456",
	}, "127.0.0.1")
	if err != nil {
		t.Errorf("new password should work: %v", err)
	}
}

// TestCreateUser_AdminOnly verifies that the admin can create additional
// users via the CreateUser method.
func TestCreateUser_AdminOnly(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	admin, _, _, _ := svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	newUser, err := svc.CreateUser(ctx, CreateUserRequest{
		Username: "operator",
		Password: "Operator1Pass",
		Role:     RoleUser,
	}, admin.ID, "127.0.0.1")

	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if newUser.Role != RoleUser {
		t.Errorf("expected role 'user', got %q", newUser.Role)
	}

	// New user can login
	_, _, _, err = svc.Login(ctx, LoginRequest{
		Username: "operator",
		Password: "Operator1Pass",
	}, "127.0.0.1")
	if err != nil {
		t.Errorf("created user should be able to login: %v", err)
	}
}

// TestUserCount verifies the user count reflects registered users.
func TestUserCount(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	count, _ := svc.UserCount(ctx)
	if count != 0 {
		t.Errorf("expected 0 users, got %d", count)
	}

	svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "StrongPass1",
	}, "127.0.0.1")

	count, _ = svc.UserCount(ctx)
	if count != 1 {
		t.Errorf("expected 1 user, got %d", count)
	}
}

// TestWeakPassword_RejectedAtRegistration verifies that passwords not meeting
// the policy are rejected during registration.
func TestWeakPassword_RejectedAtRegistration(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()

	_, _, _, err := svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Password: "weak", // too short, no upper, no digit
	}, "127.0.0.1")

	if err == nil {
		t.Error("weak password should be rejected at registration")
	}
}
