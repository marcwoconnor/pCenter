# pCenter Roadmap

> The original Phase 0–M5 build plan (getting pCenter to an initial functional state) is complete. This file used to track that work and is kept as historical context.

**Active roadmap** lives in two places now:

- **[`docs/vcenter-feature-parity-roadmap.md`](docs/vcenter-feature-parity-roadmap.md)** — the vCenter feature-parity matrix with phases, effort estimates, and per-feature acceptance notes.
- **[`CHANGELOG.md`](CHANGELOG.md)** — what actually shipped in each release.
- **[GitHub Issues](https://github.com/marcwoconnor/pCenter/issues?q=is%3Aissue+is%3Aopen+label%3Aroadmap)** — individual roadmap items tracked as issues, filterable by `phase-3` / `phase-4` and `effort-s` / `effort-m` / `effort-l` labels.

The original milestone plan is archived below for reference.

---

## Archived: Initial Milestone Plan (M0–M5)

> A proper Proxmox datacenter manager - what PDM should have been

### M0: Foundation ✅
- Project scaffolding (Go modules, React/Vite setup)
- Proxmox API client library in Go
- Multi-node configuration (YAML)
- Basic auth (API tokens, session mgmt)
- CI/CD pipeline (build, test, Docker image)

### M1: Unified Dashboard ✅
- Node overview: all nodes, status, resources (CPU/RAM/storage)
- Unified VM/CT list across clusters
- Real-time updates via WebSocket
- Ceph cluster status + storage overview

### M2: Basic Actions ✅
- Start/Stop/Restart VMs and CTs
- Console access (noVNC/xterm.js proxy)
- VM/CT config details + task history + notifications

### M3: Bulk Operations ✅
- Multi-select VMs/CTs + bulk start/stop/restart
- Bulk resource modification, tagging
- Operation queue with progress tracking

### M4: Migration & Placement ✅
- Manual migration wizard + progress tracking
- Node load visualization + migration suggestions
- Maintenance mode (drain node)

### M5: Polish & Initial Release ✅
- Dark/light theme, user preferences
- Error handling & recovery
- Deployment guide (Docker, systemd, LXC)
- Security audit (HTTPS, CORS, auth)

All M0–M5 items landed before the pre-1.0 versioning reset. See `docs/vcenter-feature-parity-roadmap.md` for what's next.
