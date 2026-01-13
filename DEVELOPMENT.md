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

## Key Decisions
- SQLite for metadata (config, preferences) - no external DB needed
- In-memory cache for real-time state - rebuilt on restart from PVE API
- WebSocket pushes state diffs to frontend
- Single binary + static assets = easy deployment
