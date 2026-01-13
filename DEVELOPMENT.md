# pCenter - CLAUDE.md

Proxmox datacenter manager - vCenter alternative

## Stack
- **Backend**: Go 1.22+ (Chi router, SQLite)
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
