# PVE Cluster Formation — Implementation Plan

Turn 2+ standalone PVE hosts already registered in pcenter into a real Proxmox VE cluster (Corosync-backed), orchestrated by pcenter itself.

Today pcenter only auto-discovers clusters via `/cluster/status`. This feature adds the ability to *form* one from the pcenter UI — no dropping to `pvecm` or the Proxmox web UI required.

## Architecture

- **Job-based async flow.** `POST /api/inventory/pve-cluster` returns `202 {job_id}` and kicks off a goroutine. Frontend polls `GET /api/inventory/pve-cluster-jobs/{id}` every 1.5s for progress.
- **New backend package** `backend/internal/pvecluster/` owns orchestration. `inventory/` stays pure CRUD.
- **PVE password auth required for join.** PVE's `/cluster/config/join` rejects API tokens — it wants `root@pam` + password. So we re-prompt for root password per joiner.
- **Credentials strategy:** after each successful join, re-auth with root password and mint a fresh `pcenter` API token on the joined node. Handles the `/etc/pve` config replacement reliably.
- **Fail-fast, no auto-rollback.** On step failure, the error message tells the user the exact `pvecm delnode` command to recover.

## API

### Preflight

```
POST /api/inventory/pve-cluster/preflight
Body: { founder_host_id, joiner_host_ids[], cluster_name }
200:  { cluster_name_ok, hosts[{host_id, address, node_name, role, reachable,
                               already_in_cluster, vm_count, ct_count,
                               pve_version, blockers[]}], can_proceed }
```
Read-only probing. Hits each host's `/version`, `/cluster/status`, `/nodes/{n}/qemu`, `/nodes/{n}/lxc`.

### Create

```
POST /api/inventory/pve-cluster
Body: { cluster_name, datacenter_id, founder_host_id, founder_password, founder_link0?,
        joiners[{host_id, password, link0?}] }
202:  { job_id }
```

### Poll job

```
GET /api/inventory/pve-cluster-jobs/{id}
200: { job_id, state: running|succeeded|failed,
       cluster_name, inventory_cluster_id?,
       steps[{host_id, address, role, phase, state, upid?, message, error?, started_at, ended_at}],
       error? }
```

## PVE client additions

`backend/internal/pve/client.go`:

```go
func (c *Client) ClusterCreate(ctx, opts ClusterCreateOptions) (upid string, err error)
func (c *Client) GetClusterJoinInfo(ctx) (*ClusterJoinInfo, error)
func (c *Client) WaitForTask(ctx, upid string, pollInterval time.Duration) (*Task, error)

// Package-level — must use password auth (API tokens rejected)
func ClusterJoin(ctx, address string, auth *AuthResult, req ClusterJoinRequest, insecure bool) (upid string, err error)
```

Types: `ClusterCreateOptions`, `ClusterJoinInfo`, `ClusterJoinNode`, `ClusterJoinRequest`.

## Orchestration sequence (happy path)

Given founder F, joiners J1..Jn, cluster name N:

1. **Validate pcenter state** — cluster_name not taken; all hosts in DC, standalone, online.
2. **Re-run preflight** — VM/CT counts, already-in-cluster, reachability, **PVE major-version homogeneity across all targeted hosts** (can't mix 7.x with 8.x in a new cluster).
3. **Founder: ClusterCreate** → UPID → WaitForTask (90s timeout).
4. **Founder: GetClusterJoinInfo** — retry 5× with 2s backoff (corosync may still be finalizing).
5. **For each joiner, serially** (parallel joins confuse corosync):
   1. `AuthenticateWithPassword(joiner, "root@pam", password)` → ticket.
   2. `ClusterJoin(joiner, auth, …)` → UPID (or empty on some PVE versions).
   3. Wait for join: if UPID, `WaitForTask` on joiner (180s, accept ctx-deadline since pveproxy restarts mid-task); regardless, poll **founder's** `/cluster/status` every 3s until joiner's node name appears `online=1`.
   4. Re-auth + `CreateAPIToken("pcenter")` (deletes existing first) + update `inventory_hosts.token_id/secret` in DB.
6. **Inventory state update** (transactional via `FormClusterFromHosts`):
   - Create Cluster row (`name=N`, `agent_name=N`, `datacenter_id=X`, `status=active`, `enabled=true`).
   - Move all hosts into it.
7. **Poller refresh.** `poller.AddCluster({name=N, discovery_node=founder.address, …})`, then `poller.RemoveCluster("standalone:"+hostID)` for each promoted host (new method).
8. **Job → succeeded.** Set `inventory_cluster_id`, WS broadcast re-fetch.

### Failure handling

| Step fails | Action | Error message template |
|---|---|---|
| Preflight | No side effects | lists blockers per host |
| Founder create | No side effects | includes PVE task log |
| Joiner N | Stop loop, don't touch already-joined | `joiner X failed at <phase>: <err>. Manual recovery: on <founder> run 'pvecm delnode <joiner>', then 'pvecm status' to verify quorum. Retry after cleanup.` |
| Inventory DB update | Log loudly, job fails | `PVE cluster formed but pcenter inventory update failed: <err>. Delete the standalone host rows in pcenter and rescan — auto-discovery will pick up the new cluster.` |

### Timeouts

- Overall job: 10 min.
- Preflight: 30s; create: 90s; fetch-join-info: 30s w/ retries; per-joiner: 240s.
- On shutdown: cancel in-flight jobs; state survives as "failed by shutdown".

## Inventory state changes (success)

| Before | After |
|---|---|
| No `clusters` row for N | `name=N, agent_name=N, datacenter_id=X, status=active, enabled=true` |
| `inventory_hosts[founder]`: `cluster_id='', datacenter_id=X` | `cluster_id=<new>, datacenter_id=X` |
| `inventory_hosts[joiner]`: `cluster_id='', token_id/secret=<old>` | `cluster_id=<new>, token_id/secret=<fresh>` |

`AgentName = PVE cluster name` is critical: `inventory/service.go:603-607` uses it as the poller config name + state-store key, matching `/cluster/status` output.

## Frontend wizard

`frontend/src/components/CreateProxmoxClusterDialog.tsx` — 4-step modal, invoked from the Datacenter context menu (only shown when DC has ≥2 standalone online hosts).

1. **Pick** — cluster name (validate: 1-15 chars, `[a-zA-Z0-9-]`, PVE rule); founder dropdown; joiner checkboxes.
2. **Preflight** — row per host with per-check green/red; `can_proceed=false` blocks Next.
3. **Credentials** — root@pam password per host. Toggle "Same password for all" defaulted ON. Collapsed "advanced" for per-host `link0` override.
4. **Progress** — poll job, render vertical step list with spinner/check/x. On failed: show full error with Copy button (recovery messages matter).

Frontend API client (`frontend/src/api/client.ts`): `createPveClusterPreflight`, `createPveCluster`, `getPveClusterJob`.

## File-by-file change list

1. `backend/internal/pve/client.go` — new methods + types.
2. `backend/internal/pve/client_test.go` — httptest mocks for all 4 operations.
3. `backend/internal/inventory/service.go` + `db.go` — `FormClusterFromHosts` transactional helper.
4. `backend/internal/pvecluster/{job,manager,preflight,orchestrate}.go` — new package.
5. `backend/internal/pvecluster/orchestrate_test.go` — table tests + PVE mock server.
6. `backend/internal/poller/poller.go` — add `RemoveCluster(name string)`.
7. `backend/internal/api/handlers_pvecluster.go` (new) — 3 handlers + `SetPveClusterManager` on `Handler`.
8. `backend/internal/api/router.go` — register 3 routes.
9. `backend/cmd/server/main.go` — construct `pvecluster.Manager`, inject.
10. `frontend/src/api/client.ts` + types — 3 new methods.
11. `frontend/src/components/CreateProxmoxClusterDialog.tsx` (new).
12. `frontend/src/components/InventoryTree.tsx` — new menu item in `getDatacenterMenuItems`; render dialog.
13. `CHANGELOG.md`.

## Test plan

### Unit

- `pve/client_test.go`: each operation against httptest server returning realistic PVE payloads. `WaitForTask` polls + respects ctx.
- `pvecluster/orchestrate_test.go`: happy 3-host path; founder-create fail; joiner-2 fail; preflight "has 2 VMs" blocks.

### Manual QA (3 fresh PVE 8.x VMs)

1. Add all 3 as standalone hosts in same datacenter via UI.
2. Create a VM on a joiner candidate — preflight must block.
3. Delete VM, rerun preflight — must pass.
4. Submit "Create Proxmox Cluster" with name `homelab1`. Progress view should complete in <2 min on LAN.
5. Tree re-renders: single `homelab1` cluster under DC with all 3 hosts.
6. On any node: `pvecm status` shows 3 nodes, quorate.
7. VMs from all 3 nodes visible under the single cluster in pcenter.
8. Failure case: joiner with VM → actionable error.
9. Failure case: pull network mid-join → error message mentions `pvecm delnode`.
10. Token survival: `/etc/pve/user.cfg` on joiner shows `pcenter` token matching pcenter's stored value.

## Decisions (2026-04-24)

1. **Job persistence across restart** — non-goal. 404 → "pcenter restarted, verify manually; Rescan".
2. **`poller.RemoveCluster`** — in scope; add as part of this feature.
3. **PVE version floor** — no hard floor; preflight requires major-version homogeneity across all targeted hosts (mixed-major clusters unsupported for new cluster formation).
4. **Join `hostname`** — always IP from `host.address` (avoids DNS dependency; PVE uses it to seed corosync `ring0_addr` when link0 not overridden).
5. **Token naming** — always overwrite `root@pam!pcenter` on joined nodes post-join.
6. **Activity log** — `pve_cluster_create` entries for start/success/fail.
7. **RBAC** — existing admin-only protected mux is sufficient for v1.

## Explicit non-goals

- Separate corosync ring1 network (`link1` plumbed through Go types but unused).
- QDevice (arbiter) configuration.
- Removing a node from a cluster (`pvecm delnode` — manual recovery only).
- Auto-rollback on partial failure.
- Progress persistence across pcenter restarts.
- Multi-datacenter clusters / joiners from different pcenter datacenters.
- Destroying a cluster back into standalone hosts.
