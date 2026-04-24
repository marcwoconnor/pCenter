# Changelog

All notable changes to pCenter are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/). Versioning: pre-1.0 (SemVer 0.x.y); breaking changes are allowed in the minor digit until v1.0.

## Unreleased

### Fixed
- **WebSocket rejects same-origin connections when `cors_origins` is empty** (closes #50). `CheckOrigin` in `internal/api/websocket.go` claimed to fall back to same-origin when no origins were configured, but actually rejected every request carrying an `Origin` header — which browsers always send. Result: fresh APT installs showed "Connection lost — reconnecting..." on the dashboard and logged `websocket upgrade failed error="websocket: request origin not allowed"` on every page load. Fix: when `cors_origins` is empty, parse the `Origin` header with `net/url` and allow if `u.Host == r.Host`. The explicit-allowlist path (for reverse-proxy / cross-origin setups) is unchanged. Rejections on both paths still emit `slog.Warn("websocket origin rejected", ...)` so operators can see refusals in the journal. New regression test covers the empty-`cors_origins` + matching-Origin case.
- **"Setup Host SSH" failed with `sshpass: executable file not found`** on fresh deb installs (closes #52). `copySSHKey()` in `handlers.go` shelled out to `sshpass | ssh-copy-id` for the initial key bootstrap, but neither is installed on minimal Debian/Ubuntu and neither was declared in the deb's `Depends:`. Replaced the shell-out path with native `golang.org/x/crypto/ssh`: dial with password auth, append the pCenter pubkey to the remote `~/.ssh/authorized_keys` (idempotent — skips if already present), persist the remote host key into local `~/.ssh/known_hosts` (TOFU, matching the prior `StrictHostKeyChecking=accept-new` behavior), and verify by reconnecting with public-key auth against the pinned host key. No more runtime dependency on `sshpass` / `ssh-copy-id`. New `handlers_ssh_test.go` exercises the full path against an in-process SSH server (success + wrong-password). `ensureSSHKeypair()`'s `ssh-keygen` shell-out is unchanged — other code paths in this repo (e.g. `internal/pve/client.go`, the `pvesh` over `ssh` handlers) still rely on the `openssh-client` CLI, so keygen lives in the same dependency envelope as those.
- **Poller disabled by default on fresh deb installs** (closes #45). Two overlapping causes: the shipped `/etc/pcenter/config.yaml` had no `poller:` block, and `config.Load()` never implemented the aspirational "default true" documented in `PollerConfig`'s comment — so a fresh install loaded with `poller=false` and the dashboard showed 0 clusters. Fix: `Load()` now auto-enables the poller when at least one cluster is configured AND the user hasn't explicitly set `poller.enabled` (a second YAML pass distinguishes "unset" from "false"). Explicit `poller.enabled: false` is always honored. The deb config template and `config.yaml.example` now also include an explicit `poller: { enabled: true }` block so the knob is discoverable. Existing installs whose `/etc/pcenter/config.yaml` was left as-is (dpkg conffile) are now fixed by the code-default path on next restart. Added three regression tests in `config_test.go`.
- **Silent encryption-key data loss on deb installs** (closes #47). Postinst now seeds `PCENTER_ENCRYPTION_KEY` in `/etc/pcenter/env` on first install (32-byte random hex). The runtime's auto-persist path previously failed silently under systemd `ProtectSystem=strict` because `/etc` is read-only to the service — a new key was generated every restart, making any data encrypted with the prior key (TOTP secrets, webhook secrets) unreadable. Existing installs are left alone (grep check); source-build installs still use the runtime auto-persist which works fine against `/opt/pcenter/.env`.
- **SSH/vmstat failures against `/root/.ssh` read-only errors** (closes #49). Systemd unit now sets `Environment=HOME=/opt/pcenter/data`, which redirects pCenter's SSH state (`~/.ssh/id_ed25519`, `~/.ssh/known_hosts`) into the already-writable data directory. Previously `ProtectSystem=strict` blocked writes to `/root/.ssh` and `ProtectHome=true` blocked reads of the private key — so SSH auth failed on every poll and the journal filled with "Read-only file system" errors. No Go code changes: `ensureSSHKeypair()` already honors `HOME`, and `ssh(1)` inherits it via `exec`.

### Changed
- Empty-state UX: replaced the misleading "Connection lost — reconnecting..." yellow banner with a blue "No Proxmox hosts connected yet — Add a host →" info banner when no clusters are configured. Correctly distinguishes data-state (have hosts?) from transport-state (WS connected?). Upper-right indicator now reads "Connecting..." instead of "Reconnecting..." since the latter wrongly implied a prior successful connection on first page load.

## v0.1.10 — 2026-04-18

### Added (Phase 3)
- **Outbound webhooks** (closes #27). New `internal/webhooks` package + Settings → Webhooks admin UI. Endpoint CRUD, event filtering (dotted names like `vm.create`; empty filter = all events), enable/disable, test-ping, delete. Activity entries are translated into webhook events and dispatched through a buffered queue with 1+3 retries (5s/30s/2min backoff, 10s request timeout). Each request is HMAC-SHA256 signed with the receiver's shared secret: `X-pCenter-Signature: t=<unix>,v1=<hex>` over `<unix>.<body>` — same canonical form as Stripe, so receivers can reject replays by checking timestamp skew. Secrets are server-generated and returned **once** at creation, encrypted at rest with the existing `auth.Crypto` key (same one used for TOTP secrets). Endpoints are admin-only + CSRF-protected on mutating methods.
- **OpenAPI spec + Swagger UI** (closes #26). Hand-authored `openapi.yaml` embedded in the binary, served at `/api/openapi.yaml` and `/api/openapi.json` (YAML is the source; JSON is converted once at startup). Interactive Swagger UI at `/api/docs`. All three endpoints are unauthenticated so the docs are reachable before login. Documents the session-cookie + `X-CSRF-Token` flow via OpenAPI `securitySchemes`.
- Initial OpenAPI spec coverage: auth (14 routes), clusters/nodes (5), guests/VMs/containers (10), webhooks (5). Remaining route groups tracked as follow-up issues; see `backend/internal/api/openapi.yaml` header for the coverage note.

### Docs
- Dropped stale `PROGRESS.md` (superseded by ROADMAP + CHANGELOG + GitHub issues).
- Corrected DEVELOPMENT.md stack description: stdlib `net/http` mux, not chi.
- Opened 10 follow-up issues (#31–#32 from #26; #33–#42 from session review covering alarms consolidation, graceful shutdown, OpenAPI drift-checker, systemd readiness, persistent delivery log, wildcard event filters, endpoint auto-disable, per-project CLAUDE.md, DEVELOPMENT.md audit, frontend lint debt).

### Notes
- The existing alarms webhook path (`alarms/notifier.go`) is intentionally left untouched this round. Consolidation is tracked as #33.
- Swagger UI assets are loaded from `cdn.jsdelivr.net` — works out-of-the-box but not air-gap-capable. Vendoring tracked as #31.

## v0.1.9 — 2026-04-17

### Added (ACME Phase 3)
- **Scheduled ACME auto-renewal** (closes #24). New `acme_renew` scheduler task type. Cluster-wide: one task renews every online node in the cluster in parallel. Partial failures are tolerated — as long as at least one node succeeds, the task run is recorded as successful with a per-node summary. Recommended schedule: weekly Sunday 3am (`0 3 * * 0`).
- **Decoded certificate viewer** (closes #25). Node Certificates tab now shows extra fields parsed from each cert's PEM: serial number, signature algorithm, key usage flags, extended key usage OIDs, CA flag, self-signed flag. New "View PEM" button opens a modal with the raw PEM text and a copy-to-clipboard action.

### Changed
- Backend parses PEM fields server-side using Go's `crypto/x509` — no new JS dependencies, no bundle-size growth for the parsing logic itself.

### Docs
- README now links the detailed vCenter-parity roadmap + CHANGELOG + Phase 3/4 issue filters.
- Legacy root `ROADMAP.md` is now a pointer to the active roadmap sources; original M0–M5 build plan preserved as historical archive below the pointer.
- Phase 3 roadmap items (#24–#30) opened as GitHub Issues with `roadmap` + `phase-3` + effort labels.

## v0.1.8 — 2026-04-17

### Added
- **Backup management MVP** (roadmap Phase 2 #7). "Backup Now" context menu action on VMs/CTs opens a dialog with storage picker (auto-filtered to backup-capable storage accessible from the guest's node), mode (snapshot / suspend / stop), and compression (zstd / gzip / lzo / none).
- **Scheduled backups** via new `backup_create` task type in the scheduler — pick storage + mode in the task form, runs on a cron schedule.
- Backend: `CreateVzdump` client method + `POST /api/clusters/{cluster}/nodes/{node}/backup` handler + scheduler task dispatch for `vm_backup` / `ct_backup`.

### Notes
- **Phase 2 of the vCenter parity roadmap is now complete.** Items 6–10 (template library, backup, resource pools, scheduled snapshots, power schedules) all ship.
- Backup restore UI is deferred to a future phase — restore is a separate UX concern (target VM/CT ID, overwrite semantics).

## v0.1.7 — 2026-04-17

### Added
- **Resource pools** (roadmap Phase 2 #8). Cluster-detail **Pools** tab lists all Proxmox pools with member previews and inline remove. Create/delete pool dialogs.
- Backend: full pool CRUD (`/api/clusters/{cluster}/pools`, `GET/POST/PUT/DELETE`), mapping to Proxmox `/pools` endpoints.

## v0.1.6 — 2026-04-17

### Added
- **Scheduled snapshot rotation** (roadmap Phase 2 #9). New `snapshot_rotate` task type creates a timestamped `auto-YYYYMMDD-HHMMSS` snapshot and prunes older `auto-*` snapshots beyond a user-configurable retention count.
- Scheduler UI shows a retention input when `snapshot_rotate` is selected.

### Notes
- Power schedules (roadmap #10) work today via existing `power_on`/`power_off`/`shutdown` task types with weekday cron presets — no additional code shipped.

## v0.1.5 — 2026-04-17

### Added
- **Certificate expiry alarms** (ACME Phase 2b). New `cert_days_left` alarm metric type — set warning/critical thresholds on days-until-expiry and fire notifications when a cert is about to expire.
- Poller collects per-node certificate info every 5 minutes.

### Fixed
- **Alarm notifications were dead code.** The `Dispatch()` function that sends alarms to configured webhook/Slack/email channels was never called from the evaluator. It is now wired at the state-transition point, so all existing alarm definitions actively fire notifications.

## v0.1.4 — 2026-04-17

### Added
- **Custom certificate upload** (ACME Phase 2b). Upload non-ACME PEM cert+key to a node, with `force` flag and optional key reuse.
- **Revert to Self-Signed** button on node Certificates tab deletes a custom cert and restarts pveproxy.
- **Bulk renewal** button on cluster ACME tab triggers ACME renewal across every online node in parallel, with per-node status display.

## v0.1.3 — 2026-04-17

### Fixed
- **ACME plugin `data` unmarshal.** Proxmox returns the plugin `data` field as a newline-separated `key=value` string (not a JSON object). Added custom `UnmarshalJSON` on `ACMEPlugin` that handles both the string form and the object form, plus a plugin with no data (standalone). Regression test covering all three shapes.

## v0.1.2 — 2026-04-17

### Added
- **ACME account CRUD** (ACME Phase 2a). Register/update/delete Let's Encrypt (or other directory) accounts from pCenter UI with directory picker and ToS accept flow.
- **DNS challenge plugin CRUD** with **schema-driven form**: Proxmox's `/cluster/acme/challenge-schema` returns per-provider field schemas; pCenter renders any of the ~30 supported DNS providers dynamically (no per-provider code).
- **Per-node ACME domain editor**: add/remove domains, assign challenge plugins, supports both cluster-uniform and per-domain plugin configs.
- Auto-mask sensitive plugin fields matching `token|key|secret|password`.

## v0.1.1 — 2026-04-17

### Added
- **ACME certificate management MVP** (roadmap Phase 1 items 1-5). Per-node **Certificates** tab: issuer, SAN, expiry, fingerprint, PEM filename. Renew ACME cert button (requires PVE-side account + plugin setup).
- Read-only cluster **ACME** tab listing accounts and plugins.
- Home page **expiry banner** (yellow <30 days, red <7 days).

## v0.1.0 — 2026-04-17

### Added
- **Convert VM/CT to Template** via native Proxmox endpoint (`POST /nodes/{node}/qemu/{vmid}/template`, same for LXC). Context-menu action on VMs/CTs with running-state guard.

### Changed
- **Pre-1.0 versioning reset.** Early `v1.0.0` – `v1.3.0` releases are retired. Project will remain on `0.x.y` until shipping-ready. `vite.config.ts` fallback version is now `0.1.0-dev`.
- Stripped AI/Claude attribution from all historical commit messages via `git filter-repo`. Repository history is authored solely by the project maintainer.
