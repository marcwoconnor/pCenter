# pCenter - Developer Notes

Proxmox datacenter manager - vCenter alternative

## Project Evolution

**v1 (Current - pcenter @ 10.31.11.50):** Polling architecture - pCenter pulls from Proxmox nodes
**v2 (In Development - pcenter2 @ 10.31.11.51):** Agent architecture - pve-agent pushes to pCenter

### Key Planning Documents
- `docs/pve-agent-spec.md` - Complete Proxmox API surface (200+ endpoints)
- `docs/pve-agent-project-plan.md` - Full implementation plan and architecture
- `docs/auth-system.md` - Authentication system: sessions, TOTP 2FA, trusted IPs

## Stack
- **Backend**: Go 1.22+ (stdlib `net/http` mux with method-prefix routing, SQLite)
- **Frontend**: React 18 + TypeScript + Vite + TailwindCSS + shadcn/ui
- **Real-time**: WebSocket for live updates

## Commands

```bash
# Backend
cd backend && go run ./cmd/server
cd backend && go test ./...
cd backend && go build -o pcenter ./cmd/server

# Frontend
cd frontend && npm run dev
cd frontend && npm run build
cd frontend && npm run lint
```

## Structure

```
pCenter/
├── backend/
│   ├── cmd/server/      # Main entrypoint
│   ├── internal/
│   │   ├── api/         # HTTP handlers
│   │   ├── pve/         # Proxmox API client
│   │   ├── poller/      # Background node polling
│   │   ├── state/       # In-memory state cache
│   │   └── config/      # Config loading
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── components/  # React components
│   │   ├── pages/       # Page components
│   │   ├── hooks/       # Custom hooks
│   │   ├── api/         # API client
│   │   └── types/       # TypeScript types
│   └── package.json
└── config.yaml
```

## Proxmox API Reference
- Base: `https://<node>:8006/api2/json`
- Auth: `Authorization: PVEAPIToken=<tokenid>=<secret>`
- Nodes: `/nodes`
- VMs: `/nodes/{node}/qemu`
- CTs: `/nodes/{node}/lxc`
- Storage: `/storage`
- Cluster: `/cluster/resources`

## Deployment (Production: pcenter / 10.31.11.50)

```bash
# Frontend - deploy to /opt/pcenter/frontend/ (NOT /static/)
cd frontend && rm -rf dist node_modules/.tmp && npm run build
scp -r dist/* root@pcenter:/opt/pcenter/frontend/

# Backend - rebuild and restart service
cd backend && GOOS=linux go build -o pcenter ./cmd/server
scp pcenter root@pcenter:/opt/pcenter/
ssh root@pcenter "systemctl restart pcenter"
```

- Hostname: `pcenter` (pcenter.ad.techsnet.net)
- IP: 10.31.11.50
- Static files: `/opt/pcenter/frontend/` (NOT /static/)

## Key Decisions
- SQLite for metadata (config, preferences) - no external DB needed
- In-memory cache for real-time state - rebuilt on restart from PVE API
- WebSocket pushes state diffs to frontend
- Single binary + static assets = easy deployment

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

## PVE Agent (v2 Architecture)

### Overview
Replace polling with push-based agents running on each Proxmox node.

```
pve-agent (on each PVE node)
    │
    ├── Collects: node status, VMs, CTs, storage, ceph, /proc metrics
    ├── Pushes: WebSocket to pCenter every 5 seconds
    └── Executes: Commands received from pCenter (start/stop/migrate/etc)
```

### Benefits over Polling
- Real-time updates (instant vs 5-sec delay)
- No SSH needed (agent reads /proc locally)
- Simpler firewall (agents connect outbound)
- Better scalability (distributed collection)
- Event-driven (VM started → immediate notification)

### Agent Structure
```
pve-agent/
├── cmd/pve-agent/main.go      # Entry point
├── internal/
│   ├── collector/             # Gathers data from local PVE
│   ├── executor/              # Runs commands from pCenter
│   ├── client/                # WebSocket to pCenter
│   └── types/                 # Shared types
├── pve-agent.service          # systemd unit
└── go.mod
```

### pCenter Changes for v2
```
backend/internal/
├── agent/
│   ├── hub.go                 # WebSocket hub for agents
│   ├── protocol.go            # Message types
│   └── handler.go             # Process agent data
```

### Deployment Commands
```bash
# Build agent
cd pve-agent && GOOS=linux go build -o pve-agent ./cmd/pve-agent

# Deploy to node
scp pve-agent root@pve04:/usr/local/bin/
scp pve-agent.service root@pve04:/etc/systemd/system/
ssh root@pve04 "systemctl daemon-reload && systemctl enable --now pve-agent"

# Check agent status
ssh root@pve04 "journalctl -u pve-agent -f"
```

### Development Instance
- **pcenter2**: New LXC for v2 development (separate from production v1)
- Keep v1 running at pcenter (10.31.11.50) during development

## Gotchas & Lessons Learned

### Storage API
- `GET /api/storage` returns `active: 0` and `status: ""` for all storage entries
- Do NOT filter by `s.active === 1` or `s.status === 'available'` - it filters out everything
- Just filter by content type: `s.content.includes('images')` for VM disks, `s.content.includes('rootdir')` for containers

### Agent-Only Mode (pcenter2)
- pcenter2 runs without poller (agent-only mode)
- `h.poller` is nil - don't call `h.poller.GetClusterClients()`
- Use `h.getClient(cluster, node)` helper which falls back to `createOnDemandClient()`
- Get nodes from store: `cs.GetNodes()` where `cs, _ := h.store.GetCluster(name)`

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
