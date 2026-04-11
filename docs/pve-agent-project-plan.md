# PVE Agent Project Plan

**Created:** 2026-01-14
**Last Updated:** 2026-01-14 (Session 2)
**Status:** Phase 1 Complete - Agent-Only Mode Working

## Current State Summary

**pCenter is now running in agent-only mode** - no polling, all data comes from agents on each PVE node.

| Metric | Value |
|--------|-------|
| Nodes | 2 (pve04, pve05) |
| VMs | 3 (2 running) |
| Containers | 40 (39 running) |
| Data Source | 100% agent push |

## Deployment Status

| Component | Status | Location | Details |
|-----------|--------|----------|---------|
| pcenter2 LXC | ✅ Running | pve04, VMID 102 | IP 10.31.11.51 |
| pCenter binary | ✅ Deployed | /opt/pcenter2/pcenter | With agent hub + poller disable |
| pve-agent (pve04) | ✅ Running | /usr/local/bin/pve-agent | systemd service enabled |
| pve-agent (pve05) | ✅ Running | /usr/local/bin/pve-agent | systemd service enabled |
| PVE API Token | ✅ Created | root@pam!pve-agent | Shared across cluster |

## Session 2 Progress (Jan 14, 2026 - Continued)

### Completed
1. ✅ Created pve-agent Go project with WebSocket client
2. ✅ Implemented collector for node/VM/CT/storage/Ceph status
3. ✅ Added agent hub to pCenter backend (`internal/agent/hub.go`)
4. ✅ Deployed pve-agent to pve04 with systemd service
5. ✅ Added PVE API token authentication to agent
6. ✅ Deployed pve-agent to pve05
7. ✅ Added `poller.enabled` config option to pCenter
8. ✅ Fixed nil pointer crashes when poller disabled
9. ✅ Verified agent-only mode works end-to-end

### Files Created/Modified

**New files (pve-agent):**
```
pve-agent/
├── cmd/pve-agent/main.go           # Entry point with reconnect logic
├── internal/
│   ├── client/websocket.go         # WebSocket client to pCenter
│   ├── collector/
│   │   ├── api.go                  # Local PVE API client (with token auth)
│   │   ├── collector.go            # Collection loop (5s interval)
│   │   └── system.go               # /proc/vmstat, /proc/loadavg
│   ├── config/config.go            # Config with PVE token support
│   └── types/types.go              # Protocol message types
├── config.example.yaml
├── pve-agent.service               # systemd unit file
└── go.mod
```

**New files (pCenter backend):**
```
backend/internal/agent/hub.go       # Agent WebSocket hub
```

**Modified files (pCenter backend):**
```
backend/internal/config/config.go   # Added PollerConfig
backend/internal/api/router.go      # Added /api/agent/ws endpoint
backend/internal/api/handlers.go    # Added nil checks for poller
backend/cmd/server/main.go          # Conditional poller startup
```

## Configuration Reference

### pve-agent config (/etc/pve-agent/config.yaml)
```yaml
pcenter:
  url: "ws://pcenter2:8080/api/agent/ws"
  token: ""  # Optional pCenter auth token

pve:
  token_id: "root@pam!pve-agent"
  token_secret: "e1f4289a-6097-4d1a-97a0-eb12dec368cf"

node:
  name: ""      # Auto-detected from hostname
  cluster: "default"

collection:
  interval: 5
  include_smart: false
  include_ceph: true
```

### pCenter config (/opt/pcenter2/config.yaml)
```yaml
server:
  port: 8080

poller:
  enabled: false  # Agent-only mode

clusters:
  - name: default
    discovery_node: 10.31.10.14:8006
    token_id: root@pam!pcenter
    token_secret: ${PVE_TOKEN_SECRET}
    insecure: true

metrics:
  enabled: true
  database_path: /opt/pcenter2/data/metrics.db

folders:
  database_path: /opt/pcenter2/data/folders.db

drs:
  enabled: false
```

## What Works in Agent-Only Mode

| Feature | Status | Notes |
|---------|--------|-------|
| Node status | ✅ Working | CPU, memory, disk, uptime |
| VM list | ✅ Working | All VM details |
| Container list | ✅ Working | All CT details |
| Storage inventory | ✅ Working | All storage pools |
| Ceph status | ✅ Working | Health, capacity |
| System metrics | ✅ Working | /proc/vmstat, loadavg |
| WebSocket to UI | ✅ Working | Real-time updates |
| Metrics collection | ✅ Working | Stored in SQLite |

## What Doesn't Work in Agent-Only Mode

| Feature | Status | Reason |
|---------|--------|--------|
| VM/CT actions | ❌ Unavailable | Requires command execution |
| Migration | ❌ Unavailable | Requires command execution |
| Console access | ❌ Unavailable | Requires proxy through node |
| Maintenance mode | ❌ Unavailable | Requires Ceph commands |
| QDevice status | ❌ Unavailable | Requires node API calls |
| SMART data | ❌ Unavailable | Not yet implemented in agent |

## Commands Cheatsheet

```bash
# === Building ===
# Build agent for Linux
cd /home/moconnor/projects/pCenter/pve-agent
GOOS=linux GOARCH=amd64 go build -o pve-agent ./cmd/pve-agent

# Build pCenter backend for Linux
cd /home/moconnor/projects/pCenter/backend
GOOS=linux GOARCH=amd64 go build -o pcenter ./cmd/server

# === Deploying ===
# Deploy pCenter to pcenter2
ssh root@pcenter2 "systemctl stop pcenter"
scp backend/pcenter root@pcenter2:/opt/pcenter2/
ssh root@pcenter2 "systemctl start pcenter"

# Deploy agent to a node
scp pve-agent/pve-agent root@pve04:/usr/local/bin/
ssh root@pve04 "systemctl restart pve-agent"

# === Monitoring ===
# Watch agent logs
ssh root@pve04 "journalctl -u pve-agent -f"
ssh root@pve05 "journalctl -u pve-agent -f"

# Watch pCenter logs
ssh root@pcenter2 "journalctl -u pcenter -f"

# Check agent connection status
ssh root@pcenter2 "journalctl -u pcenter --since '1 min ago' | grep agent"

# Test API
curl -s http://pcenter2:8080/api/summary | python3 -m json.tool
curl -s http://pcenter2:8080/api/nodes | python3 -m json.tool
curl -s http://pcenter2:8080/api/guests | python3 -m json.tool

# === Service Management ===
# Restart all agents
ssh root@pve04 "systemctl restart pve-agent"
ssh root@pve05 "systemctl restart pve-agent"

# Enable/disable poller (edit config then restart)
ssh root@pcenter2 "vim /opt/pcenter2/config.yaml"  # set poller.enabled
ssh root@pcenter2 "systemctl restart pcenter"
```

## Next Steps (Phase 2)

### Priority 1: Command Execution
Add ability for agents to execute commands from pCenter:
- VM start/stop/shutdown/reset
- Container start/stop/shutdown
- Simple operations that don't need task tracking

**Files to create:**
```
pve-agent/internal/executor/executor.go
backend/internal/agent/commands.go
```

**Protocol:**
```json
// pCenter → Agent
{"type": "command", "id": "cmd-123", "action": "vm_start", "params": {"vmid": 100}}

// Agent → pCenter
{"type": "command_result", "id": "cmd-123", "success": true, "upid": "UPID:..."}
```

### Priority 2: Task Tracking
Track long-running operations:
- Migration progress
- Backup jobs
- Clone operations

### Priority 3: Event Notifications
Push events immediately instead of waiting for poll:
- VM state changes
- Container state changes
- Node issues

### Priority 4: Console Proxy
Route console connections through agents:
- VNC proxy
- Terminal proxy
- SPICE proxy

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                     pcenter2 (10.31.11.51)                       │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                Agent Hub (/api/agent/ws)                 │   │
│  │  - Accepts WebSocket connections from agents             │   │
│  │  - Receives status updates (5s interval)                 │   │
│  │  - Updates state store                                   │   │
│  │  - Triggers WebSocket broadcast to browsers              │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│  ┌─────────────┐  ┌──────┴──────┐  ┌─────────────┐            │
│  │ State Store │  │  Metrics DB │  │ Browser Hub │            │
│  │ (in-memory) │  │  (SQLite)   │  │   (/ws)     │            │
│  └─────────────┘  └─────────────┘  └─────────────┘            │
└─────────────────────────────────────────────────────────────────┘
              ▲                              ▲
              │ WebSocket (persistent)       │ WebSocket
              │                              │
┌─────────────┴───────┐          ┌──────────┴──────────┐
│       pve04         │          │       pve05         │
│   (10.31.10.14)     │          │   (10.31.10.15)     │
│  ┌───────────────┐  │          │  ┌───────────────┐  │
│  │   pve-agent   │  │          │  │   pve-agent   │  │
│  │               │  │          │  │               │  │
│  │ Collects:     │  │          │  │ Collects:     │  │
│  │ - Node status │  │          │  │ - Node status │  │
│  │ - 0 VMs       │  │          │  │ - 3 VMs       │  │
│  │ - 20 CTs      │  │          │  │ - 20 CTs      │  │
│  │ - 6 storage   │  │          │  │ - 6 storage   │  │
│  │ - Ceph status │  │          │  │ - Ceph status │  │
│  │ - /proc stats │  │          │  │ - /proc stats │  │
│  └───────────────┘  │          │  └───────────────┘  │
│         │           │          │         │           │
│  ┌──────┴────────┐  │          │  ┌──────┴────────┐  │
│  │ PVE API       │  │          │  │ PVE API       │  │
│  │ (localhost)   │  │          │  │ (localhost)   │  │
│  │ Token: root@  │  │          │  │ Token: root@  │  │
│  │ pam!pve-agent │  │          │  │ pam!pve-agent │  │
│  └───────────────┘  │          │  └───────────────┘  │
└─────────────────────┘          └─────────────────────┘
```

## Troubleshooting

### Agent won't connect
1. Check agent logs: `journalctl -u pve-agent -f`
2. Verify pCenter URL in config
3. Check DNS resolution: `ping pcenter2`
4. Check firewall: `curl -v ws://pcenter2:8080/api/agent/ws`

### Agent connects but no data
1. Check PVE API token: Test with curl on the node
2. Verify token_id and token_secret in agent config
3. Check agent collection errors in logs

### pCenter crashes
1. Check for nil pointer errors in logs
2. Ensure `poller.enabled: false` if running agent-only
3. All handlers using poller should have nil checks

## PVE API Token

A single API token is used across the cluster:
- **Token ID:** `root@pam!pve-agent`
- **Token Secret:** `e1f4289a-6097-4d1a-97a0-eb12dec368cf`
- **Privilege Separation:** Disabled (privsep=0)
- **Created on:** pve04 (synced across cluster)

To create on a new cluster:
```bash
pvesh create /access/users/root@pam/token/pve-agent --privsep 0
```

## Session History

### Session 1 (Jan 14, 2026)
- Discussed agent architecture
- Created project plan and API spec
- Deployed pcenter2 LXC
- Started pve-agent implementation

### Session 2 (Jan 14, 2026 - Continued)
- Completed pve-agent with WebSocket client and collector
- Added agent hub to pCenter backend
- Deployed agents to both PVE nodes
- Added PVE API token authentication
- Added `poller.enabled` config option
- Fixed nil pointer crashes in handlers
- Verified agent-only mode working end-to-end
- **Result:** pCenter running 100% from agent data
