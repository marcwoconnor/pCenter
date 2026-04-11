# pCenter Authentication System

## Overview

vCenter-like authentication with optional TOTP 2FA, implemented Jan 2026.

## Architecture

```
Frontend (React)                     Backend (Go)
─────────────────                    ────────────
AuthContext.tsx  ──────────────────► /api/auth/* handlers
    │                                     │
    ├── Login.tsx                         ├── service.go (business logic)
    ├── Settings.tsx                      ├── db.go (SQLite storage)
    └── TOTPSetupWizard.tsx               ├── middleware.go (RequireAuth)
                                          └── session.go (token management)
```

## Key Files

| File | Purpose |
|------|---------|
| `backend/internal/auth/db.go` | SQLite schema + CRUD for users, sessions, recovery_codes, auth_events, trusted_ips |
| `backend/internal/auth/service.go` | Login, logout, session validation, password change, 2FA verification |
| `backend/internal/auth/handlers.go` | HTTP handlers for /api/auth/* endpoints |
| `backend/internal/auth/middleware.go` | RequireAuth, RequireAdmin, CSRFProtection, RateLimitLogin |
| `backend/internal/auth/totp.go` | TOTP generation, QR codes, validation |
| `backend/internal/auth/crypto.go` | AES-256-GCM encryption for TOTP secrets |
| `frontend/src/context/AuthContext.tsx` | React auth state, login/logout functions |
| `frontend/src/pages/Login.tsx` | Login form, TOTP prompt, first-user registration |
| `frontend/src/pages/Settings.tsx` | Password change, 2FA enable/disable, session management |

## Database Tables (auth.db)

- **users** - id, username, password_hash (bcrypt), role, totp_enabled, totp_secret (encrypted)
- **sessions** - id, user_id, token_hash (SHA256), csrf_token, totp_verified, expires_at
- **recovery_codes** - id, user_id, code_hash (bcrypt), used
- **auth_events** - audit log of login/logout/password changes
- **trusted_ips** - user_id, ip_address, expires_at (for 2FA skip feature)

## Session Flow

1. **Login**: POST `/api/auth/login` with username/password
2. Server validates credentials, checks lockout status
3. Creates session with 32-byte random token, stores SHA256 hash in DB
4. Sets httpOnly cookie `pcenter_session` with raw token
5. If 2FA enabled and IP not trusted → returns `requires_totp: true`
6. **TOTP verify**: POST `/api/auth/verify-totp` with code
7. On success, marks session.totp_verified = true, adds IP to trusted list

## Trusted IP Feature

After successful 2FA verification, the user's IP is trusted for 24 hours (configurable). Subsequent logins from the same IP skip the TOTP prompt.

```yaml
# config.yaml
auth:
  totp:
    trust_ip_hours: 24  # 0 to disable
```

**Implementation:**
- `db.IsTrustedIP()` - checks if IP is in trusted list and not expired
- `db.TrustIP()` - adds/updates IP with expiration
- `service.Login()` - skips TOTP if IP trusted
- `service.VerifyTOTPLogin()` - adds IP to trusted list after success
- Cleanup runs hourly with session cleanup

## Security Features

| Feature | Implementation |
|---------|----------------|
| Password hashing | bcrypt cost 12 |
| Session tokens | 32-byte crypto/rand + SHA256 hash stored |
| CSRF protection | Per-session token in X-CSRF-Token header |
| TOTP secrets | AES-256-GCM encrypted in DB |
| Rate limiting | Token bucket per-IP (10 req/min default) |
| Account lockout | 5 failures → 15 min lockout (progressive) |
| Cookie security | httpOnly, SameSite=Strict, Secure (in prod) |

## Config Reference

```yaml
auth:
  enabled: true
  database_path: "data/auth.db"
  encryption_key: ${AUTH_ENCRYPTION_KEY}  # 32-byte hex for AES-256

  session:
    duration_hours: 24
    idle_timeout_hours: 8
    cookie_secure: false  # true in production

  lockout:
    max_attempts: 5
    lockout_minutes: 15
    progressive: true  # doubles lockout on repeated failures

  totp:
    enabled: true
    required: false  # force all users to enable 2FA
    issuer: "pCenter"
    recovery_codes: 10
    trust_ip_hours: 24  # skip 2FA for trusted IPs

  rate_limit:
    requests_per_minute: 10
```

## API Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| POST | `/api/auth/login` | Public | Login |
| POST | `/api/auth/verify-totp` | Partial | Complete 2FA |
| POST | `/api/auth/logout` | Auth | End session |
| POST | `/api/auth/register` | Public* | First user only |
| GET | `/api/auth/me` | Auth | Current user + CSRF |
| PUT | `/api/auth/password` | Auth | Change password |
| POST | `/api/auth/totp/setup` | Auth | Start 2FA enrollment |
| POST | `/api/auth/totp/verify-setup` | Auth | Confirm 2FA |
| DELETE | `/api/auth/totp` | Auth | Disable 2FA |
| GET | `/api/auth/sessions` | Auth | List sessions |
| DELETE | `/api/auth/sessions/{id}` | Auth | Revoke session |

## Troubleshooting

**Account locked:**
```bash
sqlite3 /opt/pcenter/data/auth.db "UPDATE users SET failed_attempts=0, locked_until=NULL WHERE username='admin';"
```

**Check trusted IPs:**
```bash
sqlite3 /opt/pcenter/data/auth.db "SELECT * FROM trusted_ips;"
```

**Clear all trusted IPs for a user:**
```bash
sqlite3 /opt/pcenter/data/auth.db "DELETE FROM trusted_ips WHERE user_id='<user_id>';"
```
