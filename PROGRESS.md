# pCenter Progress Summary

## Completed Phases

| Phase | Status | Description |
|-------|--------|-------------|
| 1. Multi-cluster backend | ✅ | Per-cluster state, composite VMID keys, cluster-aware handlers |
| 2. Multi-cluster frontend | ✅ | Cluster hierarchy in tree, cluster context, prefixed API calls |
| 3. Migration (vMotion) | ✅ | Live migration dialog, progress tracking, WebSocket updates |
| 4. DRS | ✅ | Load analyzer, recommendations, Apply/Dismiss UI in DRSPanel |
| 5. HA Display | ✅ | HA badges on guests, quorum status on clusters |
| 6. HA Management | ✅ | Enable/Disable HA via context menu on VMs/CTs |

## Deployment

- **Location:** `10.31.11.50:8080` (pcenter-lxc)
- **Service:** systemd `pcenter.service`
- **Config:** `/opt/pcenter/config.yaml`
- **Env:** `/opt/pcenter/.env` (contains `PVE_TOKEN_SECRET`)

## Key Files (Phase 6 - HA Management)

- `backend/internal/pve/client.go` - EnableHA, DisableHA, GetHAGroups methods
- `backend/internal/api/handlers.go` - HA handlers (lines 1007-1172)
- `backend/internal/api/router.go` - HA routes (lines 117-120)
- `frontend/src/api/client.ts` - enableHA, disableHA, getHAGroups
- `frontend/src/components/InventoryTree.tsx` - Context menu HA options

## Known Issues

- Proxmox 8+ migrated HA groups → rules; `/cluster/ha/groups` returns empty (handled gracefully)
- Git repo has no remote configured

## Future Work

1. Add second cluster to config (test true multi-cluster)
2. DRS semi-automatic / fully-automatic modes
3. HA groups/rules UI (adapt to Proxmox 8+ rules system)
4. Set up git remote and push

## Deploy Commands

```bash
# Build frontend
cd frontend && npm run build

# Sync to server
rsync -avz --delete frontend/dist/ root@10.31.11.50:/opt/pcenter/frontend/
rsync -avz --delete backend/ root@10.31.11.50:/opt/pcenter/backend/

# Rebuild and restart on server
ssh root@10.31.11.50 "cd /opt/pcenter/backend && go build -o pcenter ./cmd/server && systemctl stop pcenter && cp pcenter /opt/pcenter/pcenter && systemctl start pcenter"
```
