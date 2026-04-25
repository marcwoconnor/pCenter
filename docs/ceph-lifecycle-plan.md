# Ceph Lifecycle Management in pCenter

> **Status:** Design approved 2026-04-25. Ships in 4 PRs (see "Internal phasing" below).
> **Plan source:** This document is the canonical design for the Ceph lifecycle initiative. Update it as decisions evolve; do not let it drift from reality.

## Context

pCenter today has a **read-only Ceph monitor**: it polls `/nodes/{node}/ceph/status`, caches per-node `CephStatus`, and offers a tiny set of whitelisted repair commands (`pg_repair`, `health_detail`, `osd_tree`, `pg_query`, `set noout`) via the pve-agent. The roadmap parity matrix marks "Ceph integration" as ✅ on that basis (`docs/vcenter-feature-parity-roadmap.md:141`), but the agent spec puts real Ceph management in Phase 5 (`docs/pve-agent-spec.md:555`).

vCenter parity for storage means operators expect to **install, configure, grow, heal, and decommission** Ceph from pCenter — not just watch it. This plan adds full lifecycle: bootstrap install on a fresh PVE cluster, MON/MGR/MDS/OSD CRUD, pool + CephFS + RBD management, CRUSH rule editing, maintenance flags (noout, no-scrub, etc.), and clean teardown.

**Outcome:** an operator can take a 3-node PVE cluster from zero Ceph to a working RBD + CephFS deployment, manage it day-2 (add OSDs, replace failed disks, resize pools, set maintenance flags), and tear it down cleanly — all from the pCenter UI.

## Architecture decisions (locked)

1. **Hybrid execution model.**
   - **PVE REST** (`/nodes/{node}/ceph/{init,osd,mon,mgr,mds,fs,pool,rules,crush,log}`) for everything PVE already wraps. Returns UPIDs, integrates with PVE's task log, no shell-out surface to maintain. Auth: existing API token.
   - **pve-agent** for the gaps PVE has no REST for: `pveceph install`, `pveceph purge`, pre-OSD disk zap (`ceph-volume lvm zap`), post-destroy LVM cleanup, and any operation that needs streamed multi-minute progress (apt install).
2. **root@pam password at job start** for install + destroy, mirroring `pvecluster` (PVE's `/cluster/config` and several `/ceph` endpoints reject API tokens). Collected in the wizard, held for the job's lifetime, never persisted. Day-2 ops (pool/OSD/MON CRUD) use the existing API token.
3. **Reuse the `pvecluster` pattern** (`backend/internal/pvecluster/`) for any multi-node orchestration (install across N hosts, MON quorum changes, OSD bulk add). That package gives us `Manager` + `Job` + phased steps + preflight + recovery. New package: `backend/internal/cephcluster/`.
4. **Per-cluster Ceph model in state**, not per-node. The current `ClusterStore.ceph[node]` is wrong shape for cluster-wide concepts (pools, CRUSH rules, MON quorum). Migrate to `ClusterStore.ceph *CephState` with `Status`, `MONs`, `MGRs`, `MDSs`, `OSDs`, `Pools`, `Rules`, `Flags`, `LastUpdated`. Per-node OSD lookups become `state.GetCephOSDsForNode(node)`.

## Data model

**New file: `backend/internal/pve/ceph_types.go`** — extract Ceph types out of `types.go:201–212` and grow:

```go
type CephState struct {
    Status       *CephStatus
    MONs         []CephMON
    MGRs         []CephMGR
    MDSs         []CephMDS
    OSDs         []CephOSD     // includes node, device, weight, in/up, used/avail
    Pools        []CephPool    // size, min_size, pg_num, crush_rule, application
    Rules        []CephRule    // CRUSH rule definitions
    FS           []CephFS
    Flags        CephFlags     // noout, norebalance, norecover, noscrub, nodeep-scrub, ...
    Version      string
    LastUpdated  time.Time
}
```

Concrete struct shapes follow PVE's REST response field names verbatim (avoid translation drift); use `json` tags.

**State changes — `backend/internal/state/state.go`:**
- Replace `ceph map[string]*CephStatus` (line ~39) with `ceph *CephState` keyed at the cluster level.
- Add: `GetCeph()`, `SetCeph(*CephState)`, `GetCephOSDsForNode(node)` helpers.
- Update `state.go:~365` `GetCeph()` to return the cluster-wide state, not the first non-nil node entry.

## PVE client extensions

**Extend `backend/internal/pve/client.go`** — group new methods by resource for readability. Patterns to copy: `GetStorage()` for read shape, `MoveVMDisk()` for UPID-returning POSTs, `GetMaintenancePreflight()` (line 1517) for preflight responses.

| Resource | Methods |
|---|---|
| Cluster | `GetCephConfig`, `SetCephConfig` (network, public/cluster CIDR) |
| Init | `InitCephCluster(network, clusterNetwork, size, minSize)` (POST `/ceph/init`) |
| MON | `ListCephMONs(node)`, `CreateCephMON(node, monid)`, `DeleteCephMON(node, monid)` |
| MGR | `ListCephMGRs(node)`, `CreateCephMGR(node, id)`, `DeleteCephMGR(node, id)` |
| MDS | `ListCephMDSs(node)`, `CreateCephMDS(node, name)`, `DeleteCephMDS(node, name)` |
| OSD | `ListCephOSDs(node)`, `CreateCephOSD(node, dev, dbDev, walDev, encrypted)`, `DeleteCephOSD(node, osdid, cleanup)`, `SetCephOSDIn(node, osdid)`, `SetCephOSDOut(node, osdid)`, `ScrubCephOSD(node, osdid, deep)` |
| Pool | `ListCephPools(node)`, `CreateCephPool(node, opts)`, `UpdateCephPool(node, name, opts)`, `DeleteCephPool(node, name, removeStorages)`, `GetCephPoolStatus(node, name)` |
| FS | `ListCephFS(node)`, `CreateCephFS(node, opts)`, `DeleteCephFS(node, name)` |
| CRUSH | `GetCRUSHMap(node)`, `GetCRUSHRules(node)` |
| Flags | `SetCephFlag(node, flag, enable)` (replaces single-purpose `SetCephNoout` at line 1609 — keep it as a thin wrapper for backward compat or remove if no callers outside) |
| Misc | `GetCephLog(node, start, limit)`, `CmdSafetyCheck(node, cmd)` |

All POST/DELETE methods return UPID + use the existing `WaitForTask()` (`client.go:2735`) when the caller wants to block. Async-by-default; handler decides.

## Agent extensions (pve-agent)

**Extend `pve-agent/internal/executor/ceph.go`** with structured commands for the privileged shell-out surface PVE doesn't expose:

| Action | Maps to |
|---|---|
| `ceph_install` | `pveceph install --version <ver> --repository <enterprise\|no-subscription\|test>` (streamed progress) |
| `ceph_purge` | `pveceph purge --crash --logs` (with safety guards: refuse if OSDs/MONs still present unless `force=true`) |
| `disk_zap` | `ceph-volume lvm zap --destroy <device>` (PRE-OSD-CREATE; param: `device`, `confirm` token derived from device serial) |
| `lvm_cleanup` | List + remove orphaned `ceph-*` VGs/LVs after OSD destroy |
| `ceph_config_get` | `ceph config dump --format json` (for diffing what PVE wrote vs. expected) |

Existing whitelist (`ceph.go:21–48`) stays; new actions extend it. **Hard rules:** every action validates inputs (regex device path, version allowlist, etc.) — no `cmd.Params["device"].(string)` straight into `exec.CommandContext` without validation. Reference the existing `pgIDRegex` pattern (line 14).

**New file: `pve-agent/internal/executor/ceph_install.go`** for the multi-minute streaming actions, since they need progress reporting via the agent WebSocket protocol (`pCenter agent` package). Pattern: read `pveceph install` stdout line-by-line, emit progress messages with phase tags.

**Capability gate**: install/purge/zap/lvm_cleanup must be guarded behind a per-host capability flag in the agent registration handshake. Default = false; operator opts in per host.

## Orchestration: install + destroy

**New package: `backend/internal/cephcluster/`** — model on `backend/internal/pvecluster/`.

```
cephcluster/
  manager.go        Manager + Job registry (copy pvecluster/manager.go shape)
  preflight.go      Per-host checks: PVE version, kernel, disk inventory, network, agent capability
  install.go        runInstallJob — phases: install pkg → init → MON on founder → MGR → join MONs → first pool
  destroy.go        runDestroyJob — phases: stop services → delete pools → delete MDSs/MGRs/MONs → purge → LVM cleanup
  osd_bulk.go       Bulk OSD creation (parallel per-node, serial within a node)
  job.go            Job, Phase, Step (copy pvecluster/job.go)
```

**Install job phases** (each is a Step the UI shows):
1. Preflight all targets (`PhaseCephPreflight`)
2. `pveceph install` on each node in parallel (`PhaseCephInstall`) — agent action
3. `pveceph init --network ... --size ... --min_size ...` on founder (`PhaseCephInit`) — REST
4. Create MON on founder, then on each remaining node serially (`PhaseCreateMON`) — REST
5. Create MGR on founder + 1 standby (`PhaseCreateMGR`) — REST
6. Wait for HEALTH_OK or HEALTH_WARN (clock skew tolerated) (`PhaseWaitHealthy`)
7. Optionally create first pool + (optionally) first CephFS (`PhaseInitialPool`)

**Destroy job phases:**
1. Preflight: refuse if guests still use Ceph storage; require typed confirmation
2. Set noout, stop balancing
3. Delete each pool (with `removeStorages=true`)
4. Delete MDSs, then MGRs, then MONs (last-MON requires explicit "I understand" flag)
5. `pveceph purge` on each node (agent action)
6. LVM cleanup on each node (agent action)
7. Strip Ceph config keys from `/etc/pve/ceph.conf` removal

**Auth model in `cephcluster.StartJobRequest`:** mirror `pvecluster.StartJobRequest` (see `manager.go:43`) — collect `FounderPassword` + per-joiner passwords, build `pve.AuthenticateWithPassword()` clients for the install/init/purge calls; reuse the API-token client for task polling.

## Day-2 handlers (no orchestration needed)

These are direct PVE REST passthroughs in `backend/internal/api/handlers.go`. New routes (all under `POST /api/clusters/{cluster}/ceph/...`):

```
GET    /ceph                                  full CephState
GET    /ceph/log
POST   /ceph/flags/{flag}                     {enable: bool}      noout, norebalance, norecover, noscrub, nodeep-scrub, nobackfill
GET    /ceph/osd
POST   /ceph/osd                              create OSD on node
DELETE /ceph/osd/{id}                         {cleanup: bool}
POST   /ceph/osd/{id}/in
POST   /ceph/osd/{id}/out
POST   /ceph/osd/{id}/scrub                   {deep: bool}
GET    /ceph/mon, POST, DELETE /ceph/mon/{id}
GET    /ceph/mgr, POST, DELETE /ceph/mgr/{id}
GET    /ceph/mds, POST, DELETE /ceph/mds/{name}
GET    /ceph/pool, POST, PUT /ceph/pool/{name}, DELETE
GET    /ceph/pool/{name}/status
GET    /ceph/fs, POST, DELETE /ceph/fs/{name}
GET    /ceph/rules
GET    /ceph/crush

POST   /ceph/install/preflight                cephcluster preflight
POST   /ceph/install                          start install Job → returns job_id
GET    /ceph/jobs/{id}                        poll job state (mirrors pvecluster)
POST   /ceph/destroy/preflight
POST   /ceph/destroy                          start destroy Job
```

**OpenAPI**: every new route must be added to `backend/internal/api/openapi.yaml` OR `backend/internal/api/testdata/openapi_drift_allowlist.txt` — `TestOpenAPINoDrift` will fail otherwise (CLAUDE.md "CI guardrails"). Prefer adding to the spec; the allowlist is for backlog only.

## Poller changes

**`backend/internal/poller/poller.go:422`** currently polls only `GetCephStatus` per node. Replace with a single per-cluster `pollCeph()` that fetches:
- `/ceph/status` → `Status`
- `/nodes/{founder}/ceph/{mon,mgr,mds,osd,pool,rules,fs}` → cluster-wide lists
- `/cluster/ceph/flags` → `Flags`

Run from the founder (or any quorum-majority MON node); fall back to the next live node if the chosen one is down. Cache the choice. Frequency: same as current Ceph poll cadence; consider a fast (5s) status poll + slow (60s) topology poll if perf matters.

## Frontend

**`frontend/src/pages/Storage.tsx`** today has hardcoded `CEPH_FIXES` (lines 11–88). Move Ceph out of Storage into its own top-level page:

```
frontend/src/pages/
  Ceph.tsx                  Cluster overview tabs: Status | OSDs | Pools | MONs/MGRs/MDSs | CephFS | CRUSH | Flags | Log
  CephInstallWizard.tsx     Multi-step wizard: select hosts → preflight → network config → confirm → run
  CephDestroyDialog.tsx     Typed-confirmation destroy
frontend/src/components/ceph/
  OSDTree.tsx               Tree view (host → device → OSD)
  PoolList.tsx
  PoolEditDialog.tsx        Create/edit pool: size, min_size, pg_num, application, CRUSH rule
  MonitorList.tsx
  CreateOSDDialog.tsx       Disk picker (lists unused devices via agent), DB/WAL device, encryption
  CreateMONDialog.tsx
  FlagsPanel.tsx            Toggle noout, etc.
  HealthDetails.tsx         Replace CEPH_FIXES — render PVE health checks + suggested actions
  JobProgress.tsx           Shared with pvecluster job UI if extractable
```

**API client**: extend `frontend/src/api/client.ts` with the new endpoints. Types in `frontend/src/types/index.ts` mirror the backend `CephState`.

Dialog convention: existing `{guest, onClose, onSuccess}` shape (CLAUDE.md). For Ceph dialogs, swap `guest` for the relevant resource (`pool`, `osd`, etc).

## Activity + RBAC + webhooks

- Every Ceph mutation logs to `activity.Service` (existing audit log). Match the verbiage style used by `migration` and `pvecluster`.
- Webhooks fire on Ceph events: `ceph.installed`, `ceph.destroyed`, `ceph.osd.created`, `ceph.osd.destroyed`, `ceph.pool.created`, `ceph.pool.deleted`, `ceph.health.degraded`, `ceph.health.recovered` (last two driven from the poller diff).
- RBAC: add a `Ceph.Admin` permission gating install/destroy + OSD/MON/pool CRUD; `Ceph.View` gates read endpoints. Extend `backend/internal/rbac/`.

## Internal phasing (one plan, four PRs)

The plan is one design but ship it in slices to keep PRs reviewable:

- **PR 1 — Day-2 read + write on existing Ceph clusters.** State refactor, PVE client extensions, handlers for OSD/MON/MGR/pool/flags CRUD, frontend `Ceph.tsx` with tabs. **No install, no destroy.** This is the largest PR but contains zero novel orchestration.
- **PR 2 — CephFS + MDS + CRUSH.** Adds the remaining day-2 surface.
- **PR 3 — Install wizard.** New `cephcluster` package, agent install/zap actions, `CephInstallWizard.tsx`. Depends on PR 1 for state plumbing.
- **PR 4 — Destroy + purge.** Agent purge/lvm_cleanup actions, destroy job, confirmation UI. Last because it's the most dangerous and benefits from the install code being battle-tested.

Each PR includes its own CHANGELOG entry under `## Unreleased` and updates `vcenter-feature-parity-roadmap.md` to track progress (split the current single ✅ row into install / day-2 / FS / destroy rows).

### PR 1 sub-slicing

PR 1 itself is large — landed in commits, not one shot. A clean atomic order that keeps the tree compilable + tests green at every step:

1. **Foundation (additive, no behavior change).** `pve/ceph_types.go` with new types; new read-only client methods (`ListCephOSDs/MONs/MGRs/MDSs/Pools/FS`, `GetCephRules/Flags`); table tests against `httptest.Server`. **Status: landed.** No state, poller, handler, or frontend changes — pure capability addition.
2. **State + poller (coordinated change).** Add `cephTopology *CephCluster` alongside the existing `ceph map[string]*CephStatus` in `ClusterStore` (keep status path unchanged so agent push + existing call sites don't break). Add `pollCephTopology()` in poller that picks any healthy MON node and populates the new field. Existing `GetCeph()` keeps returning `*CephStatus` for now.
3. **Read endpoint.** New `GET /api/clusters/{cluster}/ceph` returning the full `CephCluster`. OpenAPI updated. Frontend `client.ts` + types extended. No UI consumer yet.
4. **Write client methods + handlers + OpenAPI.** OSD in/out/scrub, MON/MGR/MDS create/delete, pool CRUD, flags toggle. One commit per resource keeps reviews focused.
5. **Frontend Ceph page.** New `pages/Ceph.tsx` with tabs, components under `components/ceph/`. `Storage.tsx` keeps its current Ceph row but loses the inline `CEPH_FIXES` block (replaced by a "Manage Ceph" link to `/ceph`).

The original "Replace `ceph map[string]*CephStatus` with `*CephState`" wording in this doc was over-aggressive: per-node CephStatus from the agent push path is genuinely per-node, so keep the per-node map and add a separate per-cluster topology field rather than collapsing both into one. Refactor the per-node status to derived later (or never) — it costs nothing to keep.

## Critical files to modify

- `backend/internal/pve/types.go` (lines 201–212) → extract to `ceph_types.go`, expand
- `backend/internal/pve/client.go` (extend; lines 1131, 1140, 1182, 1517, 1609 are reference points)
- `backend/internal/state/state.go` (lines ~38, ~365 — model migration)
- `backend/internal/poller/poller.go` (line 422 — replace per-node Ceph poll)
- `backend/internal/api/handlers.go` (lines 462, 468, 553, 985 — patterns to copy)
- `backend/internal/api/router.go` + `openapi.yaml` + `testdata/openapi_drift_allowlist.txt` (drift CI)
- `backend/internal/cephcluster/*` (new — model on `pvecluster/`)
- `backend/internal/rbac/` (new permissions)
- `pve-agent/internal/executor/ceph.go` (extend whitelist)
- `pve-agent/internal/executor/ceph_install.go` (new — streaming install)
- `frontend/src/pages/Ceph.tsx` (new) + `CephInstallWizard.tsx` + `CephDestroyDialog.tsx`
- `frontend/src/components/ceph/*` (new directory)
- `frontend/src/pages/Storage.tsx` (lines 11–88) — strip `CEPH_FIXES`, link to `/ceph`
- `frontend/src/api/client.ts`, `frontend/src/types/index.ts` (extend)
- `CHANGELOG.md`, `docs/vcenter-feature-parity-roadmap.md` (split Ceph row)

## Patterns to reuse (don't reinvent)

| Need | Reuse |
|---|---|
| Multi-node orchestration | `backend/internal/pvecluster/manager.go`, `orchestrate.go`, `job.go`, `preflight.go` |
| root@pam password auth | `pve.AuthenticateWithPassword()`, `pve.ClusterCreateWithPassword()` (`orchestrate.go:32, 54`) |
| UPID polling | `pve.WaitForTask()` (`client.go:2735`), `GetTaskLog`, `GetTaskError` |
| Per-node state caching | `state.ClusterStore.maintenance` (`state.go:75`) |
| Preflight response shape | `pvecluster.HostPreflightResult` (`preflight.go:30`) |
| Activity logging | `activity.Service.Log()` calls in `migration/` and `pvecluster/` |
| Dialog prop shape | `MigrateDialog.tsx`, `MoveStorageDialog.tsx` |
| Streaming progress from agent | existing `agent/` WS protocol — see how `migration` reports task progress |

## Verification

**Per-PR checks (CLAUDE.md "Before committing"):**
1. `cd backend && go test ./...` — must pass; add table tests for each new client method against a recorded PVE response (use `httptest.Server` like existing client tests).
2. `cd backend && go vet ./...`
3. `cd frontend && npx tsc --noEmit` (touched files at minimum)
4. `cd frontend && npx eslint src/` on new files (ignore pre-existing debt)
5. `TestOpenAPINoDrift` must pass — every new route in `router.go` is in `openapi.yaml` (preferred) or the allowlist
6. CHANGELOG entry under `## Unreleased`

**End-to-end against the test harness** (`docs/test-harness.md`):

PR 1 (day-2 ops):
- Point pCenter at a test cluster that already has Ceph installed (use the team's existing harness or spin up via the PVE UI manually).
- Verify the new `Ceph.tsx` page renders OSD tree, pool list, MON quorum.
- Toggle noout via the UI; confirm via `ceph osd dump | grep flags` on a node.
- Create + delete a test pool; confirm via `ceph osd pool ls`.
- Mark an OSD out, then back in; confirm in `ceph osd tree`.

PR 3 (install):
- Tear down Ceph on a 3-node test harness cluster (manual `pveceph purge`).
- Run the install wizard end-to-end; verify HEALTH_OK after completion.
- Kill pve-agent on one joiner mid-install; confirm the job marks the step failed with a useful recovery message (don't auto-rollback — operator decides).

PR 4 (destroy):
- Run destroy on the cluster from PR 3; confirm `pveceph status` returns "not installed" and no `ceph-*` LVs remain on any node (`lvs | grep ceph`).
- Confirm the destroy preflight refuses when a guest still references a Ceph-backed storage.

**Manual smoke for the dangerous bits** (operator-in-the-loop, before merge):
- Install on a real 3-node cluster; let it run for 24h with synthetic load; check no goroutine leaks via `pprof`.
- Destroy on the same cluster; confirm packages remain (`pveceph purge` doesn't `apt remove`) so reinstall is fast.

## Out of scope (explicitly)

- Multi-cluster federation (RGW multisite, mirroring) — separate plan.
- RGW (S3 gateway) install/management — Proxmox doesn't wrap it; revisit when there's user demand.
- iSCSI gateway over RBD.
- Erasure-coded pools beyond what PVE's pool create dialog already supports — start with replicated.
- Ceph upgrades (Quincy → Reef etc.) — separate plan; needs careful per-daemon ordering.
- Cross-cluster Ceph (one Ceph cluster spanning multiple PVE clusters) — not a thing PVE supports natively.
