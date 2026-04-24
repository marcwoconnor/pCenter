# CLAUDE.md — pCenter

Guidance for Claude Code sessions in this repo. Load first, then work.

## What this project is

pCenter is a multi-cluster Proxmox VE manager — vCenter-style UX for PVE. Single Go binary + React SPA + embedded SQLite. Pre-1.0; breaking changes allowed at the minor digit until v1.0.0. See `ROADMAP.md` and `docs/vcenter-feature-parity-roadmap.md` for the parity plan.

## Commands

```bash
# Backend — always run from backend/ or with ./backend/... prefix
cd backend && go test ./...
cd backend && go build ./cmd/server
cd backend && go vet ./...
cd backend && go run ./cmd/server -config ../config.yaml

# Frontend — always run from frontend/
cd frontend && npm run dev           # Vite dev server, proxies /api to backend
cd frontend && npm run build         # production build into dist/
cd frontend && npx tsc --noEmit      # type-check only
cd frontend && npx eslint src/       # lint (has known pre-existing debt — see #42)

# Release
packaging/build-deb.sh               # produces pcenter_*.deb
```

## Before committing

1. `go test ./...` in backend.
2. `npx tsc --noEmit` in frontend (touched files at minimum).
3. `npx eslint` on files you changed. Ignore pre-existing errors unrelated to your change.
4. CHANGELOG entry under `## Unreleased` for user-visible changes (not doc-only).

## Conventions

**Commit messages** — Conventional Commits style. `feat(area): …`, `fix(area): …`, `docs(area): …`, `test(area): …`. Keep subject short; prose goes in the body. Use `closes #N` to auto-close — **one `closes` clause per issue**, not `closes #X, #Y` (GitHub only closes the first in a comma-joined list; see memory `feedback_github_autoclose.md`).

**Never add AI/Claude attribution** to commits, PRs, code, or docs. User-level global rule. Applies here.

**Go style** — stdlib `net/http` mux with method-prefix routing (`"POST /api/foo"`). No third-party router. Handlers live in `backend/internal/api/handlers.go`; package-local types in per-feature packages under `backend/internal/`.

**Frontend style** — React 18, TypeScript strict, Vite, TailwindCSS. Dialogs follow the `{guest, onClose, onSuccess}` prop shape (see `MigrateDialog.tsx`, `MoveStorageDialog.tsx`). API calls centralise in `frontend/src/api/client.ts`.

**Proxmox quirks to remember**
- `GET /api/storage` returns `active: 0` and empty `status` even for working storages — filter by `content` only.
- Guest NIC data isn't in the list endpoints; it's fetched per-guest from `/config`.
- PVE DELETE operations return a UPID for task tracking, not just success/error.

## CI guardrails

**OpenAPI drift test** — `TestOpenAPINoDrift` in `backend/internal/api/openapi_drift_test.go` fails if any route in `router.go` is missing from `openapi.yaml` *and* from `testdata/openapi_drift_allowlist.txt`. When adding, renaming, or removing routes, update the spec or the allowlist — both sides must reconcile. Issue #32 tracks shrinking the allowlist. See memory `feedback_openapi_drift_test.md` for the detailed workflow.

## Layout

```
backend/
  cmd/server/           entrypoint, lifecycle, graceful shutdown
  internal/
    api/                HTTP handlers, router, OpenAPI spec, WebSocket hub
    pve/                Proxmox API client (REST + SSH shell-outs)
    poller/             background cluster polling → state cache
    state/              in-memory cache, the source of truth for read APIs
    activity/           audit log → SQLite, feeds webhooks + dashboard
    webhooks/           outbound HMAC-signed dispatch with retry + auto-disable
    auth/               sessions + TOTP + encryption-at-rest for secrets
    rbac/               role + permission store, resolver against state
    migration/          monitors PVE task state for VM/CT/disk moves
    inventory/          datacenter → cluster → host hierarchy
    folders/ tags/ library/ scheduler/ alarms/ drs/ metrics/   feature-scoped
    agent/              WebSocket protocol for pve-agent push mode
  openapi.yaml          hand-authored; drift-checked in CI

frontend/src/
  components/           reusable UI (dialogs, tree, cards)
  pages/                route-level views
  api/client.ts         typed API surface
  types/index.ts        all shared TypeScript types
  context/              React context providers (Cluster, Folder, Auth)

packaging/              .deb builder + systemd unit + postinst
pve-agent/              optional push-mode agent (runs on each PVE node)
docs/                   roadmap, auth design, agent spec, test harness
```

## Key gotchas

- **The poller's global context** is cancelled *last* in shutdown via `defer cancel()`. Any long-running goroutine must observe `ctx.Done()` explicitly; the 200ms settle delay after `cancel()` in the shutdown path is pragmatic, not guaranteed.
- **Agent-only mode** (no poller) is real: `h.poller` may be nil. Use the `h.getClient(cluster, node)` helper, not direct poller access.
- **Cluster naming** — `cluster.AgentName` may differ from `cluster.Name` in the tree. When matching state to UI, prefer `node.cluster` (actual value) over `clusterName` (display).
- **Encryption-at-rest** — `auth.Crypto` is shared across modules (TOTP secrets, webhook secrets). A change to the key format breaks all of them. The key lives in `/etc/pcenter/env` (deb installs) or `/opt/pcenter/.env` (source builds).
- **Dates and memory** — when the user mentions relative dates ("next Thursday"), convert to absolute before writing project memories — they outlast the current conversation.

## Related memory

The user's Claude memory store (`.claude/projects/-home-moconnor-projects-pCenter/memory/`) contains durable cross-session notes: release deploy recipe, test-harness VMs, GitHub autoclose quirk, OpenAPI drift-test workflow, and more. Load at session start; update when claims rot.
