# pCenter

A datacenter management platform for Proxmox VE. Multi-cluster, real-time, single binary.

Built because PDM (Proxmox Datacenter Manager) links to individual node UIs instead of providing a unified experience. pCenter gives you one interface for everything: VMs, containers, storage, networking, HA, DRS, metrics, and maintenance — across all your clusters.

## Features

### Multi-Cluster Management
Manage multiple Proxmox clusters from a single dashboard. Global aggregation lets you see every VM, container, and node across all clusters at once, or drill into a specific cluster.

### VM & Container Lifecycle
Full CRUD: create, start, stop, shutdown, reboot, clone, migrate, delete. Edit hardware configuration (CPU, memory, disks, network) with live config digest tracking to prevent conflicts.

### Live Migration
Online and offline migration between nodes within a cluster. Target node selection with compatibility checks. Real-time progress tracking via Proxmox UPID task system.

### Snapshots
Create, rollback, and delete snapshots for VMs and containers. Tree visualization shows snapshot parent/child relationships. VM snapshots support optional RAM state capture.

### DRS (Distributed Resource Scheduler)
Automatic load imbalance detection across cluster nodes. Generates migration recommendations based on configurable CPU and memory thresholds. Three modes:
- **Manual** — recommendations only, you approve
- **Semi-automatic** — auto-place new VMs
- **Fully automatic** — auto-execute all migrations

### High Availability
Enable/disable HA per VM or container. Configure recovery policies (max restart, max relocate). HA status badges throughout the UI. Quorum and QDevice monitoring.

### Storage & Ceph
Browse storage pools and content (ISOs, templates, images, backups). Upload files to storage. SMART disk health monitoring with temperature tracking. Ceph cluster health panel with whitelisted repair commands (`pg_repair`, `osd_tree`, `health_detail`).

### Networking
Network interface listing per node. SDN zones, VNets, and subnets management. Network topology visualization showing bridges, bonds, and VLANs.

### Metrics & Monitoring
30-second collection interval with automatic rollup: raw (24h) -> hourly (7d) -> daily (30d) -> weekly (1yr). Per-node, per-VM, per-container, per-storage metrics. CPU, memory, disk, network I/O, with min/max/avg. Time range selection: 1h, 6h, 24h, 7d, 30d.

### Console Access
VNC console for VMs and terminal access for LXC containers. Proxied through the backend with ticket-based authentication. noVNC integration for browser-based access.

### Maintenance Mode
Safe node maintenance with preflight checks (storage, HA, quorum). Automated guest evacuation. Progress tracking. QDevice status monitoring.

### Folder Organization
Hierarchical folder structure for organizing VMs and nodes. Separate tree views for hosts and VMs. Drag-and-drop folder management.

### Datacenter Inventory
Multi-tier hierarchy: Datacenters -> Clusters -> Hosts. Host onboarding workflow with connection testing, SSH key setup, and automated agent deployment.

### Authentication & Security
- Username/password with bcrypt
- TOTP 2FA (optional or required for all users)
- Recovery codes
- Trusted IP addresses (skip 2FA from known networks)
- Session management with idle timeout and per-session revocation
- Account lockout with progressive penalties
- CSRF protection
- Rate limiting on login

### Activity Audit Trail
All operations logged with timestamp, actor, resource, cluster, and status. Searchable and filterable. Configurable retention.

### Real-Time Updates
WebSocket pushes state changes to the browser. No polling from the frontend — updates appear instantly when VMs start, stop, migrate, or when metrics change.

## Architecture

```
                    Browser (React)
                         |
                    WebSocket + REST
                         |
                   ┌─────┴─────┐
                   │  pCenter   │  Go binary
                   │  Backend   │  SQLite (auth, metrics, folders, inventory)
                   └──┬─────┬──┘
                      │     │
            ┌─────────┘     └─────────┐
            │ Proxmox API             │ Agent WebSocket
            ▼                         ▼
     ┌──────────┐             ┌──────────┐
     │  pve04   │             │ pve-agent │  Per-node daemon
     │  pve05   │             │ pve-agent │  Collects metrics
     │  ...     │             │ pve-agent │  Executes commands
     └──────────┘             └──────────┘
```

**Two data collection modes:**
- **Poller** (v1): pCenter pulls from Proxmox API every 5 seconds
- **Agent** (v2): pve-agent pushes to pCenter via WebSocket — real-time, no SSH needed, better scalability

Both modes can run simultaneously. Agent data takes priority when available.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.22+, Chi router, SQLite |
| Frontend | React 18, TypeScript, Vite, TailwindCSS |
| Real-time | WebSocket (gorilla/websocket) |
| Console | noVNC |
| Agent | Go, WebSocket client, local Proxmox API |
| Deployment | Single binary + static assets |

## Quick Start

### Prerequisites
- Go 1.22+
- Node.js 18+
- Access to a Proxmox VE cluster with API token

### Configuration

Copy the example config and edit:

```bash
cp config.yaml.example config.yaml
```

```yaml
clusters:
  - name: my-cluster
    discovery_node: "10.0.0.1:8006"
    token_id: "root@pam!mytoken"
    token_secret: "your-token-secret"

server:
  port: 8080

metrics:
  enabled: true
  collection_interval: 30

auth:
  enabled: true

drs:
  enabled: true
  mode: manual
```

### Development

```bash
# Backend
cd backend && go run ./cmd/server

# Frontend (separate terminal)
cd frontend && npm install && npm run dev
```

Frontend dev server runs at `http://localhost:5173`, proxies API calls to backend at `:8080`.

### Production Build

```bash
# Build frontend
cd frontend && npm run build

# Build backend
cd backend && GOOS=linux go build -o pcenter ./cmd/server

# Deploy
scp pcenter root@server:/opt/pcenter/
rsync -avz --delete frontend/dist/ root@server:/opt/pcenter/frontend/
ssh root@server "systemctl restart pcenter"
```

## API

90+ REST endpoints. Key patterns:

```
# Global (all clusters)
GET  /api/summary
GET  /api/nodes
GET  /api/guests                     # VMs + containers
GET  /api/storage

# Cluster-specific
GET  /api/clusters/{cluster}/summary
GET  /api/clusters/{cluster}/guests
POST /api/clusters/{cluster}/vms/{vmid}/{action}

# VM operations
POST /api/clusters/{cluster}/vms/{vmid}/migrate
POST /api/clusters/{cluster}/vms/{vmid}/clone
GET  /api/clusters/{cluster}/vms/{vmid}/snapshots

# Metrics
GET  /api/metrics/node/{node}?start=...&end=...&metrics=cpu,mem_percent

# WebSocket
GET  /ws                              # Real-time state updates
GET  /api/agent/ws                    # Agent connection
```

Full endpoint list in [CLAUDE.md](CLAUDE.md).

## Agent Deployment

The pve-agent runs on each Proxmox node for real-time metrics and command execution.

```bash
# Build
cd pve-agent && GOOS=linux go build -o pve-agent ./cmd/pve-agent

# Deploy to node
scp pve-agent root@pve-node:/usr/local/bin/
scp pve-agent.service root@pve-node:/etc/systemd/system/
ssh root@pve-node "systemctl daemon-reload && systemctl enable --now pve-agent"
```

Or deploy from the UI: Hosts & Clusters -> Add Host -> Deploy Agent.

## Project Structure

```
pCenter/
├── backend/
│   ├── cmd/server/          # Entry point
│   └── internal/
│       ├── api/             # HTTP handlers + WebSocket + console proxy
│       ├── agent/           # Agent hub + command tracking
│       ├── auth/            # Sessions, TOTP, lockout, CSRF
│       ├── config/          # YAML config loading
│       ├── drs/             # Load analyzer + recommendations
│       ├── folders/         # Folder hierarchy (SQLite)
│       ├── inventory/       # Datacenter/cluster/host management
│       ├── metrics/         # Collection, rollup, retention (SQLite)
│       ├── pve/             # Proxmox API client
│       ├── poller/          # Background node polling
│       └── state/           # In-memory state cache
├── frontend/
│   └── src/
│       ├── api/             # API client (90+ methods)
│       ├── components/      # React components (22)
│       ├── context/         # Cluster + Auth context providers
│       ├── hooks/           # useMetrics, useConfigEditor, useWebSocket
│       ├── pages/           # Home, Login, Storage, Network, Settings, etc.
│       └── types/           # TypeScript types (700+ lines)
├── pve-agent/
│   ├── cmd/pve-agent/       # Entry point
│   └── internal/
│       ├── client/          # WebSocket connection to pCenter
│       ├── collector/       # Local Proxmox data collection
│       ├── config/          # Agent configuration
│       ├── executor/        # Command execution (VM/CT/Ceph)
│       └── types/           # Message types
├── config.yaml              # Main configuration
└── docs/                    # Architecture docs, API specs
```

## Databases

All SQLite, all local, no external DB required:

| Database | Purpose | Default Path |
|----------|---------|-------------|
| auth.db | Users, sessions, TOTP, events | data/auth.db |
| metrics.db | Time-series metrics with rollup | data/metrics.db |
| folders.db | Folder hierarchy + members | data/folders.db |
| inventory.db | Datacenters, clusters, hosts | data/inventory.db |
| activity.db | Audit trail | data/activity.db |

## License

Private. Not open source.
