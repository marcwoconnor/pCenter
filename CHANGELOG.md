# Changelog

All notable changes to pCenter are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/). Versioning: pre-1.0 (SemVer 0.x.y); breaking changes are allowed in the minor digit until v1.0.

## Unreleased

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
