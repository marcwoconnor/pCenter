# Changelog

All notable changes to pCenter are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/). Versioning: pre-1.0 (SemVer 0.x.y); breaking changes are allowed in the minor digit until v1.0.

## Unreleased

### Fixed
- **Ceph install fails at `create_mon` phase with PVE 501** — `POST /nodes/{node}/ceph/mon` returned `Method 'POST /nodes/{node}/ceph/mon' not implemented` because PVE's create endpoints require the daemon id in the URL path (`/ceph/mon/{monid}`, `/ceph/mgr/{id}`, `/ceph/mds/{name}`). The pcenter PVE client now sends the hostname as the id — matching what `pveceph mon create` does on the CLI — for MON, MGR, and MDS create. Day-2 daemon creation from the Ceph page hit the same bug; it works now too.

## v0.1.18 — 2026-04-25

### Fixed
- **Ceph install wizard: white screen on "Run preflight"** — the per-host `blockers` field came back as `null` (Go nil slice) for healthy hosts, and `h.blockers.length` then threw and unmounted the React tree. Backend now serializes `Blockers` as `[]`; the wizard also handles a missing array defensively.

## v0.1.17 — 2026-04-25

Same payload as the would-be v0.1.16 (which failed CI on a leftover `mode` reference in `AddHostDialog.tsx` from PR #69 — caught by `tsc -b` in CI but not by `tsc --noEmit` locally) plus the one-line build fix.

### Fixed
- **`AddHostDialog.tsx`: drop dead `mode === 'datacenter'` ternary** that referenced a removed prop. The cluster-mode branch was deleted in PR #61's cleanup but this one ternary survived; CI's `tsc -b` (project references) caught it where `tsc --noEmit` did not.

## v0.1.16 — 2026-04-25 (unreleased — failed CI)

Two big themes this release: the **full Ceph lifecycle** (install → day-2 → CephFS → destroy, all from `/ceph`), and a tranche of poller/inventory **fixes** from PR #69 that landed alongside.

### Added — Ceph lifecycle (closes PRs 1-4 of `docs/ceph-lifecycle-plan.md`)
- **New top-level `/ceph` page** with 7 tabs: Status, OSDs, Pools, Monitors, CephFS, CRUSH, Flags. Cluster picker URL-driven (`/ceph?cluster=foo&tab=osds`); 30s auto-refresh + manual reload. Replaces the inline Ceph panel that lived in `/storage` (Storage tab now shows a quick health summary + "Manage Ceph →" link).
- **Cluster-wide topology snapshot endpoint** — `GET /api/clusters/{cluster}/ceph` returns OSDs (with host attribution + in/up state), MONs, MGRs, MDSs, pools, CRUSH rules, CephFS, and OSD flags as a single JSON document. Backed by a new 30s `pollCephLoop` in the poller that picks any node responding to `/ceph/status` and fans out the topology fetch in parallel; partial-fetch errors log at debug and don't blank the view. 404 when Ceph isn't installed.
- **Pool lifecycle** — `POST/PUT/DELETE /api/clusters/{cluster}/ceph/pool[/{pool}]` with autoscale, application tag, CRUSH rule, and `add_storages` / `remove_storages` knobs. Create + delete return UPIDs.
- **OSD lifecycle** — `POST /api/clusters/{cluster}/nodes/{node}/ceph/osd` (separate db_dev/wal_dev BlueStore offload, encryption, CRUSH device-class, osds_per_device), `DELETE .../osd/{osdid}?cleanup=1` (LVM cleanup so the device is reusable without a manual zap), `POST .../osd/{osdid}/in`, `.../out`, `.../scrub?deep=1`.
- **Monitor / Manager / MDS lifecycle** — `POST/DELETE` for `mon`, `mgr`, `mds` under `.../nodes/{node}/ceph/`. MON creation accepts optional `mon_address` for multi-NIC nodes; MDS supports `hotstandby`. DELETE handlers do NOT enforce quorum-survival — the UI confirmation dialog spells out the impact, and "remove the last MON to recover from a corrupt monmap" is a real recovery scenario the API must allow.
- **CephFS lifecycle** — `POST /api/clusters/{cluster}/nodes/{node}/ceph/fs/{name}` (PVE creates the underlying `{name}_data` and `{name}_metadata` pools), `DELETE .../fs/{name}?remove_storages=1&remove_pools=1`.
- **CRUSH viewer** — `GET /api/clusters/{cluster}/ceph/crush` returns the decompiled CRUSH map as plain text. Editing flows through `ceph osd setcrushmap` and is intentionally out of scope.
- **OSD flag toggles** — `POST /api/clusters/{cluster}/ceph/flags/{flag}` for noout, noin, noup, nodown, nobackfill, norebalance, norecover, noscrub, nodeep-scrub, pause. Explicit allowlist so typos can't reach obscure PVE flags. Maintenance flow continues using the legacy SSH `SetCephNoout` path until that handler migrates.
- **Install wizard** — `POST /api/clusters/{cluster}/ceph/install/preflight` + `POST /api/clusters/{cluster}/ceph/install` + `GET /api/clusters/{cluster}/ceph/jobs[/{job_id}]`. New `cephcluster` package (modeled on `pvecluster`) orchestrates 6 phases: `install_preflight → install_packages → ceph_init → create_mon (per node, serial) → create_mgr → wait_healthy`. Job state in memory (restart wipes it). UI is a three-step wizard invoked from the "Ceph not installed" empty state: configure → preflight blocker table → live job poll.
- **Destroy / purge** — `POST /api/clusters/{cluster}/ceph/destroy` requires `confirm` to equal the cluster name verbatim. Orchestrates 8 phases: `destroy_preflight → set_noout → delete_fs → delete_pools → delete_mds → delete_mgr → delete_mon → ceph_purge` (parallel `pveceph purge --crash --logs` over SSH). Continue-past-failure semantics — per-resource failures are recorded in `step.error` but don't abort; half-destroys are worse than mostly-clean ones. UI is a typed-confirmation dialog inside a "Danger zone" `<details>` at the bottom of the Status tab.
- **`pve.Client` extensions** — 17 new methods covering the read + write surface above (`ListCephOSDs/MONs/MGRs/MDSs/Pools/FS`, `GetCephRules/Flags/CrushMap`, `Create/Update/DeleteCephPool`, `Create/DeleteCephOSD`, `SetCephOSDIn/Out`, `ScrubCephOSD`, `Create/DeleteCephMON/MGR/MDS`, `Create/DeleteCephFS`, `InitCephCluster`, `SetCephFlag`).

**Install execution model (v1).** SSH-based `pveceph install -y` and `pveceph purge` (uses pcenter's existing root SSH key trust — the same path `SetCephNoout` already uses), REST for everything else. No per-job password collection; consistent with the rest of pCenter's auth model. The pve-agent route + streamed apt progress are tracked as future enhancements in `docs/ceph-lifecycle-plan.md`.

### Changed
- **Sidebar nav adds a "Ceph" entry** between Storage and Network. Existing nav order otherwise unchanged.

### Fixed
- **Poller reconciles against inventory every 60s, GC'ing orphan `standalone:<id>` goroutines** (closes #66). When inventory state was changed out-of-band — operator SQL-DELETE during recovery, or a test reset — the poller's view went stale: the inventory row was gone but the per-host ClusterPoller goroutine kept running and 401'ing every interval until pcenter restarted. New `Poller.Reconcile(expected []string)` drops poller entries whose names aren't in `expected`; main.go runs it on a 60s ticker against `inventory.GetClusterConfigs`. One-directional by design: only removes, never adds (adding remains the responsibility of explicit `AddCluster` calls from the API layer and the inventory reconciler's promote callback). Added `Poller.mu sync.Mutex` so the reconciler can safely snapshot the clusters map without racing the AddCluster/RemoveCluster paths — race-detector clean.
- **Cluster context menu no longer offers a misleading "Add Host (inventory record only)" entry** (closes #61). It superficially mirrored the new "Add Member Node…" wizard but did NOT run a real PVE join — it just stamped an inventory row associating the host with the cluster. Removed from `getInventoryClusterMenuItems` in `InventoryTree.tsx`. The drag-and-drop path in the inventory tree still covers the legitimate edge case of manually associating an already-PVE-clustered host. Dead code from the now-unreachable `mode: 'cluster'` branch in `AddHostDialog` (and the unused `addClusterHost` API client method) was deleted; the backend `POST /api/inventory/clusters/{name}/hosts` route is left in place since the issue noted the drag flow may rely on it indirectly.
- **Agent deploy no longer surfaces an error when the agent binary isn't shipped** (closes #60). The .deb does not include `pve-agent` (it's a separate package), so on a fresh install the agent-deploy step in "Add Host" was logging `agent_deploy_failed` and the dialog showed `Host added and SSH configured, but agent deployment failed: agent binary not found at /opt/pcenter/pve-agent` — even though the host record and API token were both fine. `deployAgentToHost` now detects `os.IsNotExist` on the source path, logs INFO once, and returns `Success: true, Skipped: true` with a friendly message about pull-mode polling working via the API token. The handler skips the activity-log entry when `Skipped` is set so the audit trail doesn't fill with non-events.
- **`CreateAPIToken` now probes the stored secret before delete-and-recreate** (closes #59). The previous behavior unconditionally rotated any existing `pcenter` token on a re-add — and because PVE clusters share `/etc/pve/priv/token.cfg` via pmxcfs, that silently invalidated the stored secret on every other cluster member, leaving the next poll cycle stuck on HTTP 401. New shape: `CreateAPIToken` takes an `existingSecret` parameter; if non-empty, it probes `GET /version` with that secret first and reuses it on success. `resolveHostAuth` looks up the previously-stored secret for the same address (new `DB.GetTokenSecretForAddress`) and threads it through. First-time host adds (no stored secret) skip the probe and behave exactly as before. The retry inside `CreateAPIToken` after a `delete` passes empty so it doesn't re-probe a known-stale secret.

## v0.1.15 — 2026-04-25

### Added
- **Create Proxmox Cluster wizard** (#58). New 4-step flow under a datacenter's "Create Proxmox Cluster…" context menu turns 1+ standalone hosts into a real Corosync-backed PVE cluster, calling Proxmox's own `POST /cluster/config` and `POST /cluster/config/join` endpoints — no `pvecm` shell required. The wizard pivots on the founder's `root@pam` password (PVE rejects API tokens for cluster-formation endpoints with "Permission check failed (user != root@pam)"), runs preflight checks per host (reachable, no guests on joiners, not already clustered, PVE major-version homogeneity across all targets), and shows live per-step progress with a "Copy error" affordance on failure. Failure mode is fail-fast with manual-recovery commands embedded in the error (`pvecm delnode`, `systemctl stop pve-cluster && rm /etc/pve/corosync.conf …`). When a previous failed attempt left a half-formed `corosync.conf` on the founder, the orchestrator detects the 500 "already exists" response and SSHes in to run the canonical revert sequence before retrying — visible to the user as a "cleanup_corosync" step.
- **Add Member Node wizard** (#58). Companion flow under a cluster's context menu for promoting standalone hosts into an already-managed cluster. Preflight pivots the major-version check on the existing cluster's online members; orchestration uses the existing source host's stored API token to fetch join info via `GET /cluster/config/join` (token auth is allowed there — only the create/join POSTs demand password). After a successful `pvecm add`, the joiner inherits `/etc/pve/priv/token.cfg` from the cluster, so the orchestrator copies the source's existing token into the joiner's record rather than calling `CreateAPIToken` (which would delete-and-recreate the cluster-shared token, invalidating every other member's stored secret). Re-discovery is also forced post-join — `ClusterPoller.run()` only calls `discoverNodes()` once at startup, so without this step the new member is invisible to vmstat/Ceph polling.
- **`docs/PVE_CLUSTER_FORMATION.md`** capturing the API surface, orchestration sequence, failure modes, and explicit non-goals (separate ring1 link, QDevice, `pvecm delnode` automation, auto-rollback, job persistence across pcenter restarts).

### Fixed
- **Onboarding: fresh installs no longer require editing `config.yaml`** (#58). The shipped `config.yaml` previously included a placeholder `clusters:` stanza pointing at `YOUR_PROXMOX_IP:8006`. On first boot the migration in `main.go` turned that into a real cluster row, and if `${PVE_TOKEN_SECRET}` was unset (the default) the service refused to start at all. Strip the stanza; hosts are added through the UI now (token auto-created from username/password). The "Add a host →" banner button is now a real action that auto-creates a datacenter when none exists and chains into Add Host.
- **Poller now defaults to enabled regardless of `cfg.Clusters` length** (#58). Hosts moved from `clusters:` config to inventory DB, so gating the poller default on `len(cfg.Clusters) > 0` meant fresh installs never started polling even after adding hosts via the UI. New default: enabled unless the user explicitly sets `poller.enabled: false`.
- **HA badge no longer claims "OK" on a single-node cluster** (#58). PVE's HA service runs by default and reports quorum=true with a single vote (1 = majority of 1), but failover is impossible without a second node. Both `HAStatusBadge.tsx` and the inline badge in `InventoryTree.tsx` now hide / show "N/A" when nodes < 2; the cluster summary Card shows "N/A — single-node cluster".
- **`RunSSHCommand` passes explicit `-i` and `UserKnownHostsFile` paths** (#58). OpenSSH's `ssh` binary uses `getpwuid(geteuid())->pw_dir` for some path lookups (notably `~/.ssh/known_hosts`), which resolves to `/root` for uid 0 even when `$HOME` is overridden — and systemd's `ProtectHome=true` (active in the .deb's unit) makes `/root` read-only. Result: every vmstat / Ceph / QDevice / SMART probe was spamming `Could not create directory '/root/.ssh' (Read-only file system)` every 5s and silently failing. Now derives `id_ed25519` and `known_hosts` from `$HOME/.ssh` explicitly, plus `BatchMode=yes` so password prompts can't hang the polling goroutine.
- **`poller.RemoveCluster` now clears `state.Store` for that cluster** (#58). Stopping the polling goroutine wasn't enough; node/storage/VM entries from a removed cluster lingered in the store and surfaced as duplicates in the UI (most visibly in the Storage view, where a host promoted from standalone to cluster appeared twice). Also drops stale state for unknown cluster names so leaked entries from prior process lifetimes get reaped.

## v0.1.14 — 2026-04-24

### Added
- **Swagger UI assets vendored for air-gap deploys** (closes #31). `/api/docs` previously loaded `swagger-ui.css` and `swagger-ui-bundle.js` from `cdn.jsdelivr.net`, which broke the docs page on deploys without outbound internet. The assets are now `go:embed`-ed into the binary and served from `/api/swagger-ui/swagger-ui.css` and `/api/swagger-ui/swagger-ui-bundle.js` (no auth — same rationale as `/api/docs`). Pinned to `swagger-ui-dist@5.32.4` (matches what the CDN was serving). Upgrade path + SHA256s are recorded in `backend/internal/api/swagger-ui/README.md`. Binary grows by ~2.6 MB stripped. The template test now asserts the HTML contains no external-CDN references — air-gap regressions fail in CI, not in production.

## v0.1.13 — 2026-04-24

### Added
- **Webhooks auto-disable after consecutive delivery failures** (closes #39). New `consecutive_failures` column on `webhook_endpoints` (added via SQLite-safe PRAGMA-then-ALTER migration — ADD COLUMN IF NOT EXISTS isn't supported, so we inspect `PRAGMA table_info` first). `RecordDelivery` now increments the counter on failure and resets it to 0 on success; when the counter reaches `AutoDisableThreshold` (10, representing ~25min of sustained breakage given the 5s/30s/2min retry schedule) the endpoint is marked `enabled=0` with `last_status='auto_disabled'`. The transition is logged once — further failures after disable don't re-fire the disable path. Old DBs without the column upgrade cleanly on Open() — covered by a migration test. Existing `success` / `failure` last_status semantics are preserved.
- **Webhook event filters now support per-component wildcards** (closes #38). `vm.*` matches every VM action, `*.migrate` matches migrations across resource types, `*.*` matches any two-component event. Bare `*` does NOT match a two-component event (component counts must agree — documented in `matches()` and in the OpenAPI `events` field description). Case-insensitive. Mixing exact + wildcard entries in one filter works as expected. 14 new sub-tests in `webhooks_test.go::TestMatches` cover the matrix.
- **OpenAPI drift-checker test** (closes #35). New `internal/api/openapi_drift_test.go` regex-extracts every route registered in `router.go` and asserts each is either documented in `openapi.yaml` or explicitly allowlisted in `testdata/openapi_drift_allowlist.txt`. Failure mode is two-sided: adding a route to `router.go` without updating the spec fails the test, and deleting a route whose allowlist entry still lingers also fails — keeping the allowlist from rotting. Seed allowlist captures the 174 routes not yet in the spec (coverage tracker #32). Chose regex over mux-instrumentation because the latter would touch 190+ `HandleFunc` call sites for no runtime benefit; Go 1.22's stdlib mux syntax is stable enough that textual extraction is the right trade. As a bonus, the test parses the same embedded `openAPIYAML` bytes the production server uses, so YAML parse errors now fail tests instead of only surfacing at runtime `init()`.

### Fixed
- **Graceful shutdown now drains WebSockets and the webhook dispatcher** (closes #34). `main.go` had `server.Shutdown()` wired to SIGINT/SIGTERM, but `hub.Stop()` and `webhooksSvc.Stop()` existed and were never called — WebSocket clients got TCP reset instead of a 1001 close frame, and the dispatcher goroutine was relying on ctx cancellation that fired *after* its DB closed (LIFO defer ordering, which was also causing latent "database is closed" races for the poller/scheduler/metrics goroutines). New shutdown sequence: `server.Shutdown(30s)` → `hub.Stop()` → `webhooksSvc.Stop()` → `cancel()` → 200ms pause before deferred DB closes. Bumped timeout from 10s → 30s (in-flight Ceph/metric queries can exceed 10s). Second SIGINT/SIGTERM during shutdown now force-exits instead of hanging. Removed duplicate `defer webhooksDB.Close()` since `webhooksSvc.Stop()` now owns the DB lifecycle.

### Added
- **Storage vMotion frontend + OpenAPI** (closes #28). Adds the "Move Storage…" context-menu action on VMs and containers, backed by the `POST /api/clusters/{c}/vms/{id}/disk/move` and `POST /api/clusters/{c}/containers/{id}/volume/move` endpoints that shipped in c916b31. New `MoveStorageDialog` parses disk/volume keys from the guest's raw config (scsi0/virtio0/ide0/sata0 for VMs, rootfs/mpN for containers), filters target storages to those that are active and accept the right content type (`images` for VMs, `rootdir` for CTs), and submits the move via the existing backend handler. Delete-source checkbox is checked by default (matches PVE webUI). VM moves happen online (no downtime); the dialog warns and blocks LXC moves while the container is running since PVE does not support online LXC volume moves. OpenAPI spec now documents the four storage-vMotion routes (`/disk/move`, `/volume/move`, `GET /disk-moves`, `DELETE /disk-moves/{upid}`) plus `MoveDiskRequest` and `DiskMoveProgress` schemas.

## v0.1.11 — 2026-04-24

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
