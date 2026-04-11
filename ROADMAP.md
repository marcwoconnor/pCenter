# pCenter v1 Roadmap

> A proper Proxmox datacenter manager - what PDM should have been

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     React Frontend                          │
│        (TypeScript, TailwindCSS, React Query)              │
└─────────────────────────┬───────────────────────────────────┘
                          │ WebSocket + REST
┌─────────────────────────┴───────────────────────────────────┐
│                      Go Backend                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ API Server  │  │  Poller     │  │  State Cache        │ │
│  │ (Fiber/Chi) │  │  (per node) │  │  (in-memory+SQLite) │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────┬───────────────────────────────────┘
                          │ Proxmox API (HTTPS)
┌─────────────────────────┴───────────────────────────────────┐
│     PVE Node 1          PVE Node 2          PVE Node N      │
│     (pve04)             (pve05)             (...)           │
└─────────────────────────────────────────────────────────────┘
```

## Tech Stack

| Layer | Technology | Rationale |
|-------|------------|-----------|
| Backend | Go 1.22+ | Concurrency, single binary, low mem |
| HTTP Framework | Chi or Fiber | Lightweight, fast |
| Database | SQLite | Zero config, embedded, sufficient for metadata |
| Cache | In-memory | Real-time state, synced from PVE API |
| Frontend | React 18 + TypeScript | Industry standard, rich ecosystem |
| UI Framework | TailwindCSS + shadcn/ui | Modern, consistent, fast to build |
| State Mgmt | React Query (TanStack) | Server state, caching, WebSocket integration |
| Charts | Recharts or Tremor | Clean metrics visualization |

---

## Milestones

### M0: Foundation (Week 1-2)
- [ ] Project scaffolding (Go modules, React/Vite setup)
- [ ] Proxmox API client library in Go
- [ ] Multi-node configuration (YAML)
- [ ] Basic auth (API tokens, session mgmt)
- [ ] CI/CD pipeline (build, test, Docker image)

### M1: Unified Dashboard - Read Only (Week 3-4)
**The killer feature - single pane of glass**

- [ ] Node overview: all nodes, status, resources (CPU/RAM/storage)
- [ ] VM/CT list: unified view across ALL nodes
  - Sortable, filterable, searchable
  - Status indicators (running/stopped/paused)
  - Resource usage per VM/CT
- [ ] Real-time updates via WebSocket
- [ ] Ceph cluster status (if applicable)
- [ ] Storage overview (local, shared, usage)

### M2: Basic Actions (Week 5-6)
- [ ] Start/Stop/Restart VMs and CTs
- [ ] Console access (noVNC/xterm.js proxy)
- [ ] View VM/CT config details
- [ ] Task history and status
- [ ] Basic notifications (task complete, errors)

### M3: Bulk Operations (Week 7-8)
- [ ] Multi-select VMs/CTs
- [ ] Bulk start/stop/restart
- [ ] Bulk resource modification (RAM, CPU)
- [ ] Bulk tagging
- [ ] Operation queue with progress tracking

### M4: Migration & Placement (Week 9-10)
- [ ] Manual migration wizard
- [ ] Migration progress tracking
- [ ] Node load visualization
- [ ] Migration suggestions (imbalance detection)
- [ ] Maintenance mode (drain node)

### M5: Polish & v1 Release (Week 11-12)
- [ ] Dark/light theme
- [ ] User preferences persistence
- [ ] Error handling & recovery
- [ ] Documentation
- [ ] Deployment guide (Docker, systemd, LXC)
- [ ] Security audit (HTTPS, CORS, auth)

---

## v1 Feature Summary

| Feature | PDM | pCenter v1 |
|---------|-----|------------|
| Multi-node view | ✓ (links only) | ✓ (unified) |
| Combined VM/CT list | ✗ | ✓ |
| Real-time metrics | ✗ | ✓ |
| Bulk operations | ✗ | ✓ |
| Migration wizard | ✗ | ✓ |
| Console proxy | ✗ | ✓ |
| Single binary deploy | ✗ | ✓ |
| Mobile responsive | ✗ | ✓ |

---

## Post-v1 Ideas (v2+)

- HA cluster management
- Automated DRS-like load balancing
- Template management & deployment
- Backup scheduling & management
- RBAC with fine-grained permissions
- Prometheus/Grafana integration
- API for automation
- Multi-datacenter support

---

## Getting Started

```bash
# Backend
cd backend && go run ./cmd/server

# Frontend
cd frontend && npm run dev
```

## Configuration

```yaml
# config.yaml
nodes:
  - name: pve04
    host: 10.31.11.1:8006
    token_id: root@pam!pcenter
    token_secret: ${PVE_TOKEN_SECRET}
  - name: pve05
    host: 10.31.13.1:8006
    token_id: root@pam!pcenter
    token_secret: ${PVE_TOKEN_SECRET}

server:
  port: 8080
  cors_origins:
    - http://localhost:5173
```
