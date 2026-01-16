package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps SQLite database for auth storage
type DB struct {
	db *sql.DB
}

// Open creates or opens the auth database
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open auth db: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init auth schema: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.db.Close()
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		email TEXT,
		password_hash TEXT NOT NULL,
		role TEXT DEFAULT 'user',
		totp_enabled INTEGER DEFAULT 0,
		totp_secret TEXT,
		failed_attempts INTEGER DEFAULT 0,
		locked_until INTEGER,
		last_login INTEGER,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		token_hash TEXT UNIQUE NOT NULL,
		csrf_token TEXT NOT NULL,
		totp_verified INTEGER DEFAULT 0,
		ip_address TEXT,
		created_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL,
		last_seen_at INTEGER NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS recovery_codes (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		code_hash TEXT NOT NULL,
		used INTEGER DEFAULT 0,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS auth_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		user_id TEXT,
		username TEXT,
		event_type TEXT NOT NULL,
		ip_address TEXT,
		details TEXT,
		success INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS trusted_ips (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		ip_address TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		UNIQUE(user_id, ip_address)
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_trusted_ips_user ON trusted_ips(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
	CREATE INDEX IF NOT EXISTS idx_recovery_codes_user ON recovery_codes(user_id);
	CREATE INDEX IF NOT EXISTS idx_auth_events_user ON auth_events(user_id);
	CREATE INDEX IF NOT EXISTS idx_auth_events_timestamp ON auth_events(timestamp);
	`
	_, err := db.Exec(schema)
	return err
}

// --- User CRUD ---

// CreateUser inserts a new user
func (d *DB) CreateUser(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now

	_, err := d.db.ExecContext(ctx, `
		INSERT INTO users (id, username, email, password_hash, role, totp_enabled, totp_secret, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, u.ID, u.Username, u.Email, u.PasswordHash, u.Role, u.TOTPEnabled, u.TOTPSecret, now.Unix(), now.Unix())
	return err
}

// GetUserByUsername finds user by username
func (d *DB) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	row := d.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, role, totp_enabled, totp_secret,
			   failed_attempts, locked_until, last_login, created_at, updated_at
		FROM users WHERE username = ?
	`, username)
	return scanUser(row)
}

// GetUserByID finds user by ID
func (d *DB) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := d.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, role, totp_enabled, totp_secret,
			   failed_attempts, locked_until, last_login, created_at, updated_at
		FROM users WHERE id = ?
	`, id)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	var lockedUntil, lastLogin, createdAt, updatedAt sql.NullInt64
	var email, totpSecret sql.NullString
	var totpEnabled int

	err := row.Scan(
		&u.ID, &u.Username, &email, &u.PasswordHash, &u.Role, &totpEnabled, &totpSecret,
		&u.FailedAttempts, &lockedUntil, &lastLogin, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	u.Email = email.String
	u.TOTPEnabled = totpEnabled == 1
	u.TOTPSecret = totpSecret.String
	if lockedUntil.Valid {
		u.LockedUntil = time.Unix(lockedUntil.Int64, 0)
	}
	if lastLogin.Valid {
		u.LastLogin = time.Unix(lastLogin.Int64, 0)
	}
	if createdAt.Valid {
		u.CreatedAt = time.Unix(createdAt.Int64, 0)
	}
	if updatedAt.Valid {
		u.UpdatedAt = time.Unix(updatedAt.Int64, 0)
	}

	return &u, nil
}

// UpdateUser updates user fields
func (d *DB) UpdateUser(ctx context.Context, u *User) error {
	u.UpdatedAt = time.Now()
	_, err := d.db.ExecContext(ctx, `
		UPDATE users SET
			email = ?, password_hash = ?, role = ?, totp_enabled = ?, totp_secret = ?,
			failed_attempts = ?, locked_until = ?, last_login = ?, updated_at = ?
		WHERE id = ?
	`, u.Email, u.PasswordHash, u.Role, boolToInt(u.TOTPEnabled), u.TOTPSecret,
		u.FailedAttempts, timeToInt(u.LockedUntil), timeToInt(u.LastLogin), u.UpdatedAt.Unix(), u.ID)
	return err
}

// DeleteUser removes a user
func (d *DB) DeleteUser(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	return err
}

// ListUsers returns all users
func (d *DB) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, username, email, password_hash, role, totp_enabled, totp_secret,
			   failed_attempts, locked_until, last_login, created_at, updated_at
		FROM users ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		var lockedUntil, lastLogin, createdAt, updatedAt sql.NullInt64
		var email, totpSecret sql.NullString
		var totpEnabled int

		err := rows.Scan(
			&u.ID, &u.Username, &email, &u.PasswordHash, &u.Role, &totpEnabled, &totpSecret,
			&u.FailedAttempts, &lockedUntil, &lastLogin, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		u.Email = email.String
		u.TOTPEnabled = totpEnabled == 1
		u.TOTPSecret = totpSecret.String
		if lockedUntil.Valid {
			u.LockedUntil = time.Unix(lockedUntil.Int64, 0)
		}
		if lastLogin.Valid {
			u.LastLogin = time.Unix(lastLogin.Int64, 0)
		}
		if createdAt.Valid {
			u.CreatedAt = time.Unix(createdAt.Int64, 0)
		}
		if updatedAt.Valid {
			u.UpdatedAt = time.Unix(updatedAt.Int64, 0)
		}

		users = append(users, &u)
	}
	return users, rows.Err()
}

// UserCount returns total number of users
func (d *DB) UserCount(ctx context.Context) (int, error) {
	var count int
	err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

// IncrementFailedAttempts increments failed login attempts
func (d *DB) IncrementFailedAttempts(ctx context.Context, userID string) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE users SET failed_attempts = failed_attempts + 1, updated_at = ?
		WHERE id = ?
	`, time.Now().Unix(), userID)
	return err
}

// LockUser locks the account until the given time
func (d *DB) LockUser(ctx context.Context, userID string, until time.Time) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE users SET locked_until = ?, updated_at = ?
		WHERE id = ?
	`, until.Unix(), time.Now().Unix(), userID)
	return err
}

// ResetFailedAttempts clears failed attempts
func (d *DB) ResetFailedAttempts(ctx context.Context, userID string) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE users SET failed_attempts = 0, locked_until = NULL, updated_at = ?
		WHERE id = ?
	`, time.Now().Unix(), userID)
	return err
}

// UpdateLastLogin sets last login time
func (d *DB) UpdateLastLogin(ctx context.Context, userID string) error {
	now := time.Now()
	_, err := d.db.ExecContext(ctx, `
		UPDATE users SET last_login = ?, updated_at = ?
		WHERE id = ?
	`, now.Unix(), now.Unix(), userID)
	return err
}

// --- Session CRUD ---

// CreateSession inserts a new session
func (d *DB) CreateSession(ctx context.Context, s *Session) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	s.CreatedAt = time.Now()
	s.LastSeenAt = s.CreatedAt

	_, err := d.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, csrf_token, totp_verified, ip_address, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.UserID, s.TokenHash, s.CSRFToken, boolToInt(s.TOTPVerified), s.IPAddress,
		s.CreatedAt.Unix(), s.ExpiresAt.Unix(), s.LastSeenAt.Unix())
	return err
}

// GetSessionByTokenHash finds session by hashed token
func (d *DB) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	row := d.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, csrf_token, totp_verified, ip_address, created_at, expires_at, last_seen_at
		FROM sessions WHERE token_hash = ?
	`, tokenHash)
	return scanSession(row)
}

// GetSessionByID finds session by ID
func (d *DB) GetSessionByID(ctx context.Context, id string) (*Session, error) {
	row := d.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, csrf_token, totp_verified, ip_address, created_at, expires_at, last_seen_at
		FROM sessions WHERE id = ?
	`, id)
	return scanSession(row)
}

func scanSession(row *sql.Row) (*Session, error) {
	var s Session
	var totpVerified int
	var createdAt, expiresAt, lastSeenAt int64
	var ipAddress sql.NullString

	err := row.Scan(
		&s.ID, &s.UserID, &s.TokenHash, &s.CSRFToken, &totpVerified, &ipAddress,
		&createdAt, &expiresAt, &lastSeenAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.TOTPVerified = totpVerified == 1
	s.IPAddress = ipAddress.String
	s.CreatedAt = time.Unix(createdAt, 0)
	s.ExpiresAt = time.Unix(expiresAt, 0)
	s.LastSeenAt = time.Unix(lastSeenAt, 0)

	return &s, nil
}

// TouchSession updates last_seen_at
func (d *DB) TouchSession(ctx context.Context, sessionID string) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE sessions SET last_seen_at = ? WHERE id = ?
	`, time.Now().Unix(), sessionID)
	return err
}

// MarkTOTPVerified marks session as 2FA verified
func (d *DB) MarkTOTPVerified(ctx context.Context, sessionID string) error {
	_, err := d.db.ExecContext(ctx, `
		UPDATE sessions SET totp_verified = 1 WHERE id = ?
	`, sessionID)
	return err
}

// DeleteSession removes a session
func (d *DB) DeleteSession(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id)
	return err
}

// DeleteSessionByTokenHash removes session by token hash
func (d *DB) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	return err
}

// ListUserSessions returns all sessions for a user
func (d *DB) ListUserSessions(ctx context.Context, userID string) ([]*Session, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, user_id, token_hash, csrf_token, totp_verified, ip_address, created_at, expires_at, last_seen_at
		FROM sessions WHERE user_id = ? ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var s Session
		var totpVerified int
		var createdAt, expiresAt, lastSeenAt int64
		var ipAddress sql.NullString

		err := rows.Scan(
			&s.ID, &s.UserID, &s.TokenHash, &s.CSRFToken, &totpVerified, &ipAddress,
			&createdAt, &expiresAt, &lastSeenAt,
		)
		if err != nil {
			return nil, err
		}

		s.TOTPVerified = totpVerified == 1
		s.IPAddress = ipAddress.String
		s.CreatedAt = time.Unix(createdAt, 0)
		s.ExpiresAt = time.Unix(expiresAt, 0)
		s.LastSeenAt = time.Unix(lastSeenAt, 0)

		sessions = append(sessions, &s)
	}
	return sessions, rows.Err()
}

// DeleteExpiredSessions removes all expired sessions
func (d *DB) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	result, err := d.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ?", time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteUserSessions removes all sessions for a user (except optionally one)
func (d *DB) DeleteUserSessions(ctx context.Context, userID string, exceptID string) error {
	if exceptID != "" {
		_, err := d.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND id != ?", userID, exceptID)
		return err
	}
	_, err := d.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

// --- Recovery Codes ---

// CreateRecoveryCodes inserts recovery codes for a user (replaces existing)
func (d *DB) CreateRecoveryCodes(ctx context.Context, userID string, codeHashes []string) error {
	// Delete existing codes
	_, err := d.db.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id = ?", userID)
	if err != nil {
		return err
	}

	// Insert new codes
	now := time.Now().Unix()
	for _, hash := range codeHashes {
		_, err := d.db.ExecContext(ctx, `
			INSERT INTO recovery_codes (id, user_id, code_hash, created_at)
			VALUES (?, ?, ?, ?)
		`, uuid.New().String(), userID, hash, now)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetRecoveryCodes returns all recovery codes for a user
func (d *DB) GetRecoveryCodes(ctx context.Context, userID string) ([]*RecoveryCode, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, user_id, code_hash, used, created_at
		FROM recovery_codes WHERE user_id = ?
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []*RecoveryCode
	for rows.Next() {
		var c RecoveryCode
		var used int
		var createdAt int64
		err := rows.Scan(&c.ID, &c.UserID, &c.CodeHash, &used, &createdAt)
		if err != nil {
			return nil, err
		}
		c.Used = used == 1
		c.CreatedAt = time.Unix(createdAt, 0)
		codes = append(codes, &c)
	}
	return codes, rows.Err()
}

// MarkRecoveryCodeUsed marks a recovery code as used
func (d *DB) MarkRecoveryCodeUsed(ctx context.Context, codeID string) error {
	_, err := d.db.ExecContext(ctx, "UPDATE recovery_codes SET used = 1 WHERE id = ?", codeID)
	return err
}

// DeleteUserRecoveryCodes removes all recovery codes for a user
func (d *DB) DeleteUserRecoveryCodes(ctx context.Context, userID string) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id = ?", userID)
	return err
}

// --- Auth Events ---

// LogEvent records an auth event
func (d *DB) LogEvent(ctx context.Context, e *AuthEvent) error {
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO auth_events (timestamp, user_id, username, event_type, ip_address, details, success)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, time.Now().Unix(), e.UserID, e.Username, e.EventType, e.IPAddress, e.Details, boolToInt(e.Success))
	return err
}

// ListEvents returns recent auth events
func (d *DB) ListEvents(ctx context.Context, limit int) ([]*AuthEvent, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, timestamp, user_id, username, event_type, ip_address, details, success
		FROM auth_events ORDER BY timestamp DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*AuthEvent
	for rows.Next() {
		var e AuthEvent
		var ts int64
		var success int
		var userID, username, ipAddress, details sql.NullString

		err := rows.Scan(&e.ID, &ts, &userID, &username, &e.EventType, &ipAddress, &details, &success)
		if err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(ts, 0)
		e.UserID = userID.String
		e.Username = username.String
		e.IPAddress = ipAddress.String
		e.Details = details.String
		e.Success = success == 1
		events = append(events, &e)
	}
	return events, rows.Err()
}

// --- Trusted IPs ---

// IsTrustedIP checks if an IP is trusted for a user (not expired)
func (d *DB) IsTrustedIP(ctx context.Context, userID, ip string) (bool, error) {
	var count int
	err := d.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM trusted_ips
		WHERE user_id = ? AND ip_address = ? AND expires_at > ?
	`, userID, ip, time.Now().Unix()).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// TrustIP adds or updates a trusted IP for a user
func (d *DB) TrustIP(ctx context.Context, userID, ip string, expiresAt time.Time) error {
	now := time.Now()
	// Use INSERT OR REPLACE since we have UNIQUE(user_id, ip_address)
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO trusted_ips (id, user_id, ip_address, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, ip_address) DO UPDATE SET expires_at = ?, created_at = ?
	`, uuid.New().String(), userID, ip, now.Unix(), expiresAt.Unix(), expiresAt.Unix(), now.Unix())
	return err
}

// DeleteTrustedIP removes a trusted IP for a user
func (d *DB) DeleteTrustedIP(ctx context.Context, userID, ip string) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM trusted_ips WHERE user_id = ? AND ip_address = ?", userID, ip)
	return err
}

// DeleteUserTrustedIPs removes all trusted IPs for a user
func (d *DB) DeleteUserTrustedIPs(ctx context.Context, userID string) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM trusted_ips WHERE user_id = ?", userID)
	return err
}

// CleanupExpiredTrustedIPs removes expired trusted IP entries
func (d *DB) CleanupExpiredTrustedIPs(ctx context.Context) (int64, error) {
	result, err := d.db.ExecContext(ctx, "DELETE FROM trusted_ips WHERE expires_at < ?", time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Helper functions
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func timeToInt(t time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	unix := t.Unix()
	return &unix
}
