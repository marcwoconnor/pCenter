# pCenter — Developer Notes

Depth-oriented engineering notes. For a session quick-start (commands, conventions, layout), see `CLAUDE.md`. For end-user install, see `README.md`.

## Architecture at a glance

Two collection modes share the same pCenter brain:

- **Poller** — pCenter pulls from each Proxmox cluster every 5s (default). Simple to deploy; only pCenter needs credentials. See `backend/internal/poller`.
- **Agent** (`pve-agent/`) — optional push-mode: a tiny Go binary on each PVE node streams state over WebSocket to pCenter. Reduces polling latency and load, lets pCenter drop SSH for most reads. Enabled per-host via the `deploy-agent` action; see `docs/pve-agent-project-plan.md`.

Both modes write to the same in-memory `state.Store`. API handlers read from the store; WebSocket hub pushes state diffs to browsers on every change.

### Key planning documents
- `docs/vcenter-feature-parity-roadmap.md` — phased plan toward v1.0, with per-feature scope notes.
- `docs/pve-agent-spec.md` — Proxmox API surface we call (REST + SSH).
- `docs/pve-agent-project-plan.md` — agent architecture + deployment model.
- `docs/auth-system.md` — sessions, TOTP 2FA, trusted-IP windows, encryption-at-rest.
- `docs/test-harness.md` — the nested PVE test cluster used for release smoke tests.

## Stack
- **Backend**: Go 1.22+ (stdlib `net/http` mux with method-prefix routing, SQLite via `mattn/go-sqlite3`, native SSH via `golang.org/x/crypto/ssh`).
- **Frontend**: React 18 + TypeScript (strict) + Vite + TailwindCSS.
- **Real-time**: WebSocket for dashboard state, SSE-like broadcast hub (`internal/api/websocket.go`).
- **Persistence**: one SQLite DB per feature area (auth, alarms, webhooks, folders, …), all under the data dir.

## Deployment

Two supported install paths — both produce the same runtime layout:

- **Debian package** (recommended) — `packaging/build-deb.sh` produces `pcenter_*.deb`. `postinst` handles fresh-install vs upgrade (fresh waits for config, upgrade restarts the service). Seeds `/etc/pcenter/env` with an auto-generated encryption key on first install. Systemd unit shipped; service runs under `ProtectSystem=strict` + `ProtectHome=true`, so `HOME=/opt/pcenter/data` — SSH invocations pass `-o UserKnownHostsFile` / `IdentityFile` explicitly because OpenSSH's `~` uses `pw_dir`, not `$HOME`.
- **Source build** — `cd backend && go build ./cmd/server` + `cd frontend && npm run build`. Copy the binary to `/opt/pcenter/pcenter`, the `dist/` contents to `/opt/pcenter/frontend/`, and install the systemd unit from `packaging/`. Config in `/etc/pcenter/config.yaml` or `/opt/pcenter/config.yaml`.

For release validation, the memory note `release_deploy_recipe.md` documents provisioning a fresh Ubuntu 24.04 LXC via the PVE API and running the README's Quick Install against it — catches the exact workarounds the deb path has baked in for issues #43–#47.

## Key design decisions
- **SQLite per feature** (not one shared DB) — each package owns its migrations, nothing cross-schemas, no JOIN coupling. Cost: harder to back up as one unit; solved by pointing `PCENTER_DATA_DIR` at one place.
- **In-memory state, rebuilt on restart** — the PVE cluster IS the source of truth. The cache is a convenience for sub-second reads; nothing persists cluster inventory.
- **WebSocket pushes state diffs** — the frontend never polls for data it already has.
- **Hand-authored OpenAPI** — `backend/internal/api/openapi.yaml`, embedded at compile time. CI-enforced via `TestOpenAPINoDrift` (see `openapi_drift_test.go`).

## Proxmox API reference
- Base: `https://<node>:8006/api2/json`
- Auth: `Authorization: PVEAPIToken=<tokenid>=<secret>`
- Nodes: `/nodes`
- VMs: `/nodes/{node}/qemu`
- CTs: `/nodes/{node}/lxc`
- Storage: `/storage`
- Cluster: `/cluster/resources`, `/cluster/status`
- Tasks: `/nodes/{node}/tasks/{upid}/status`

## Overview Panel Details

The Home page summary cards show actionable details:

### Nodes Panel
- Shows PVE version and kernel version
- Version drift detection: if nodes have different versions, shows "2/3 on PVE 9.1.4" in yellow
- Data from `/nodes/{node}/status` API (pveversion, kversion, cpuinfo, loadavg)

### VMs/Containers Panels
- Top CPU consumer with percentage (yellow if >50%)
- List of stopped guests (up to 3)
- Helps quickly identify runaway processes or forgotten VMs

## Ceph Health System

### Data Flow
1. Poller fetches `/nodes/{node}/ceph/status` from Proxmox API
2. Health checks are in `health.checks` map, keyed by check name (e.g., `OSD_SCRUB_ERRORS`)
3. Each check has: severity, summary message, and detail array
4. WebSocket sends flattened check info to frontend

### Health Check Names (from Ceph)
Common checks and their meanings:
- `OSD_SCRUB_ERRORS` - Data inconsistencies found during scrub
- `PG_DAMAGED` - Placement group has inconsistent replicas
- `PG_DEGRADED` - Not enough replicas available
- `OSD_DOWN` - OSD daemon not running
- `MON_DOWN` - Monitor daemon not running
- `POOL_NO_REDUNDANCY` - Pool size=1 (no replication)

### Remediation Lookup
`frontend/src/pages/Storage.tsx` has `CEPH_FIXES` map with suggested commands:
```typescript
CEPH_FIXES['OSD_SCRUB_ERRORS'] = {
  description: 'Scrub errors indicate data inconsistencies...',
  command: 'ceph pg repair <pg_id>'
}
```
Add new entries as you encounter different Ceph issues.

### Storage Page Tabs
- `/storage` - Storage tree view (default)
- `/storage?tab=ceph` - Ceph health details with fix suggestions

### Ceph Command Execution
The Ceph tab allows running whitelisted commands directly from the UI:

**Whitelisted commands** (in `backend/internal/pve/client.go`):
- `pg_repair` - Repair a placement group (requires PG ID)
- `health_detail` - Show detailed health info
- `osd_tree` - Show OSD tree
- `status` - Show cluster status

**Security:**
- Commands are whitelisted in the backend - no arbitrary execution
- PG IDs are validated (format: `<pool_id>.<hex_pg_num>`, e.g., `4.3b`)
- Executed via SSH to Proxmox nodes (requires SSH key access from pcenter LXC)
- All commands are logged with `slog.Info`

**API:** `POST /api/ceph/command`
```json
{ "command": "pg_repair", "pg_id": "4.3b" }
```

**Adding new commands:** Update the switch statement in `RunCephCommand()` in `client.go`

## Metrics System

vCenter-style performance metrics with SQLite storage.

### Architecture
- **Collection**: 30-second intervals via `internal/metrics/collector.go`
- **Storage**: SQLite with WAL mode at `/opt/pcenter/data/metrics.db`
- **Rollups**: Raw → Hourly → Daily → Weekly → Monthly aggregations
- **Retention**: Raw 24h, Hourly 7d, Daily 30d, Weekly 1yr
- **API**: Auto-selects resolution based on query time range

### Database Tables
- `metric_types` - Lookup table (cpu, mem_percent, disk, etc.)
- `metrics_raw` - 30-sec samples, pruned after 24h
- `metrics_hourly/daily/weekly/monthly` - Aggregated with min/max/avg/count

### API Endpoints
- `GET /api/metrics` - Query with filters
- `GET /api/metrics/node/{node}` - Node-specific
- `GET /api/metrics/vm/{vmid}` - VM metrics
- `GET /api/clusters/{cluster}/metrics` - Cluster-scoped

Query params: `start`, `end`, `metrics` (comma-sep), `resolution` (auto/raw/hourly/daily)

### Config
```yaml
metrics:
  enabled: true
  database_path: "/opt/pcenter/data/metrics.db"
  collection_interval: 30
  retention:
    raw_hours: 24
    hourly_days: 7
    daily_days: 30
    weekly_months: 12
```

## React Performance Patterns

### Preventing Re-render Flashing

**Problem**: Charts flash when parent components re-render (e.g., WebSocket updates).

**Solutions used in this codebase**:

1. **Stable array references** - Move constant arrays outside components:
   ```typescript
   // BAD - new array every render
   useMetrics({ metrics: ['cpu', 'mem'] })

   // GOOD - stable reference
   const METRICS = ['cpu', 'mem'];
   useMetrics({ metrics: METRICS })
   ```

2. **String comparison in dependencies** - Arrays in useEffect deps use reference equality:
   ```typescript
   // BAD - triggers on every render
   useEffect(() => {}, [metrics])

   // GOOD - stable string comparison
   useEffect(() => {}, [metrics.join(',')])
   ```

3. **Memoize filtered data** - Filter operations create new arrays:
   ```typescript
   // BAD - new array every render
   <Chart series={data.filter(s => s.metric === 'cpu')} />

   // GOOD - memoized
   const cpuSeries = useMemo(() => data.filter(...), [data]);
   <Chart series={cpuSeries} />
   ```

4. **Wrap components in memo()** - Prevents re-renders when props unchanged:
   ```typescript
   export const MetricsChart = memo(function MetricsChart(props) { ... });
   ```

5. **Use refs for interval callbacks** - Avoid stale closures:
   ```typescript
   const optionsRef = useRef(options);
   optionsRef.current = options; // Update on every render
   useEffect(() => {
     const interval = setInterval(() => {
       fetch(optionsRef.current.url); // Always current
     }, 30000);
   }, []); // Empty deps - runs once
   ```

## PVE Agent (push-mode collection)

Optional per-node agent that replaces SSH + REST polling with a single outbound WebSocket. Both modes work against the same pCenter binary; operators can mix (poll some clusters, push from others).

### Collection model
```
pve-agent (on each PVE node)
    │
    ├── Collects: node status, VMs, CTs, storage, ceph, /proc metrics
    ├── Pushes: WebSocket to pCenter every ~5 seconds
    └── Executes: commands received from pCenter (start/stop/migrate/backup/etc.)
```

### Trade-offs vs pure polling
| Axis | Poller | Agent |
|------|--------|-------|
| Latency | ~5s per tick | Immediate |
| Credentials | API token on pCenter only | Pre-shared token on each agent |
| Firewall | Inbound allow to PVE API port | Outbound only to pCenter |
| SSH required | Yes (for node commands) | No (agent reads /proc locally) |
| Setup per host | None beyond token | Deploy binary + systemd unit |

### Layout
```
pve-agent/
├── cmd/pve-agent/         entry point
└── internal/
    ├── collector/          samples /proc + calls local PVE API
    ├── executor/           runs commands queued by pCenter
    ├── client/             WebSocket to pCenter
    └── types/              shared wire format
```

pCenter side: `backend/internal/agent/` — WebSocket hub (`hub.go`), command correlation, state translation. The `Hub` mirrors the browser-facing hub's API surface but is keyed by `<cluster>/<node>`.

### Deploying an agent
Preferred: use the "Deploy Agent" action on a host in the pCenter UI — it generates the pre-shared token, pushes the binary over SSH, installs the unit, and starts the service.

Manual (matching behaviour):
```bash
cd pve-agent && GOOS=linux go build -o pve-agent ./cmd/pve-agent
scp pve-agent root@<pve-node>:/usr/local/bin/
scp pve-agent.service root@<pve-node>:/etc/systemd/system/
ssh root@<pve-node> "systemctl daemon-reload && systemctl enable --now pve-agent"
```

Logs: `ssh root@<pve-node> "journalctl -u pve-agent -f"`.

## Gotchas & Lessons Learned

### Storage API
- `GET /api/storage` returns `active: 0` and `status: ""` for all storage entries
- Do NOT filter by `s.active === 1` or `s.status === 'available'` - it filters out everything
- Just filter by content type: `s.content.includes('images')` for VM disks, `s.content.includes('rootdir')` for containers

### Agent-Only Mode
- Deployments running without the poller (`poller.enabled: false` in config) have `h.poller == nil` — don't call `h.poller.GetClusterClients()` unguarded.
- Use `h.getClient(cluster, node)` helper which falls back to `createOnDemandClient()` when no poller client is available.
- Get nodes from the store: `cs.GetNodes()` where `cs, _ := h.store.GetCluster(name)`. Don't route node lookups through the poller.

### Guest NIC Data
- PVE list endpoints (`/nodes/{node}/qemu`, `/lxc`) do NOT return NIC config
- NICs come from per-guest config endpoints (`/nodes/{node}/qemu/{vmid}/config`)
- Poller fetches configs in parallel via `fetchVMNICs()`/`fetchCTNICs()` in `client.go`
- Config `net0`/`net1` values are parsed by `parseNICsFromConfig()` into `[]GuestNIC`
- `UpdateNode()` uses merge (not delete-rebuild) to preserve NICs if a caller omits them

### State Store Patterns
- `h.store.GetCluster(name)` returns `*ClusterState`
- `cs.GetVM(vmid)` / `cs.GetContainer(vmid)` - methods on ClusterState, not Store
- `h.store.GetVM(vmid)` searches all clusters (legacy/global endpoints)
- `UpdateNode()` merges incoming data — only deletes guests absent from incoming list

### Proxmox DELETE Operations
- DELETE requests return UPID for task tracking (not just success/error)
- Added `deleteWithData()` helper in client.go that returns response body
- Use for VM/container deletion to get task UPID

### Node Selection & Cluster Matching
- `ObjectDetail.tsx` finds nodes via `nodes.find()` with cluster matching
- Standalone hosts and orphan nodes don't have `cluster` in their selection - use optional matching: `(!selectedObject.cluster || n.cluster === selectedObject.cluster)`
- In `InventoryTree.tsx` `renderClusterWithNodes()`: use `node.cluster` (actual value from data), NOT `clusterName` (display name) - these differ when `agent_name` is set
- Same applies to `isSelected()` checks - must use consistent cluster values
