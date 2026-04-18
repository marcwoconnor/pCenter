# Changelog

All notable changes to pCenter are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/). Versioning: pre-1.0 (SemVer 0.x.y); breaking changes are allowed in the minor digit until v1.0.

## Unreleased

### Added (Phase 3)
- **OpenAPI spec + Swagger UI** (closes #26). Hand-authored `openapi.yaml` embedded in the binary, served at `/api/openapi.yaml` and `/api/openapi.json` (YAML is the source; JSON is converted once at startup). Interactive Swagger UI at `/api/docs`. All three endpoints are unauthenticated so the docs are reachable before login. Documents the session-cookie + `X-CSRF-Token` flow via OpenAPI `securitySchemes`.
- Initial spec coverage: auth (14 routes), clusters/nodes (5), guests/VMs/containers (10). Remaining route groups tracked as follow-up issues; see `backend/internal/api/openapi.yaml` header for the coverage note.

### Notes
- Swagger UI assets are loaded from `cdn.jsdelivr.net` — works out-of-the-box but not air-gap-capable. Vendoring the two assets into the binary is a tracked follow-up.

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
