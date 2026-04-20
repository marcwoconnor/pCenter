# pCenter

A datacenter management platform for Proxmox VE. Think vCenter, but for Proxmox. Multi-cluster, real-time, single binary.

Built because PDM (Proxmox Datacenter Manager) links to individual node UIs instead of providing a unified experience. pCenter gives you one interface for everything: VMs, containers, storage, networking, HA, DRS, metrics, and maintenance — across all your clusters.

![License](https://img.shields.io/badge/license-MIT-blue)

---

## Table of Contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Installation Guide](#installation-guide)
  - [Step 1: Create the Proxmox API Token](#step-1-create-the-proxmox-api-token)
  - [Step 2: Prepare a Machine to Run pCenter](#step-2-prepare-a-machine-to-run-pcenter)
  - [Step 3: Download and Install pCenter](#step-3-download-and-install-pcenter)
  - [Step 4: Configure pCenter](#step-4-configure-pcenter)
  - [Step 5: Run pCenter](#step-5-run-pcenter)
  - [Step 6: First Login](#step-6-first-login)
- [Building from Source](#building-from-source)
- [Configuration Reference](#configuration-reference)
- [Managing Multiple Clusters](#managing-multiple-clusters)
- [Upgrading](#upgrading)
- [Troubleshooting](#troubleshooting)
- [Architecture](#architecture)
- [API](#api)
- [Development](#development)
- [Project Structure](#project-structure)
- [Roadmap & Contributing](#roadmap--contributing)

---

## Features

### Multi-Cluster Management
Manage multiple Proxmox clusters from a single dashboard. See every VM, container, and node across all clusters at once, or drill into a specific cluster.

### VM & Container Lifecycle
Full CRUD: create, start, stop, shutdown, reboot, clone, migrate, delete. Edit hardware configuration (CPU, memory, disks, network) with live config digest tracking to prevent conflicts.

### Live Migration
Online and offline migration between nodes. Target node selection with compatibility checks. Real-time progress tracking via Proxmox UPID task system.

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
Browse storage pools and content (ISOs, templates, images, backups). Upload files to storage. SMART disk health monitoring with temperature tracking. Ceph cluster health panel with whitelisted repair commands.

### Networking
Network interface listing per node. SDN zones, VNets, and subnets management. Network topology visualization showing bridges, bonds, and VLANs.

### Metrics & Monitoring
30-second collection interval with automatic rollup: raw (24h) -> hourly (7d) -> daily (30d) -> weekly (1yr). Per-node, per-VM, per-container metrics. CPU, memory, disk, network I/O. Time range selection: 1h, 6h, 24h, 7d, 30d.

### Console Access
VNC console for VMs and terminal access for LXC containers. Proxied through the backend with ticket-based authentication. noVNC integration for browser-based access.

### Content Library
Curated catalog of ISOs, templates, and OVAs across all clusters. Tag, categorize, and deploy from a central library.

### Maintenance Mode
Safe node maintenance with preflight checks (storage, HA, quorum). Automated guest evacuation with progress tracking.

### Authentication & Security
- Username/password with bcrypt hashing
- TOTP 2FA with QR code setup (optional or required)
- Recovery codes for 2FA backup
- Trusted IP addresses (skip 2FA from known networks)
- Session management with idle timeout
- Account lockout with progressive penalties
- CSRF protection on all state-changing operations
- Rate limiting on login

### Real-Time Updates
WebSocket pushes state changes to the browser instantly. No polling from the frontend — updates appear the moment VMs start, stop, migrate, or metrics change.

---

## Screenshots

*Coming soon*

---

## Installation Guide

This guide walks you through installing pCenter from scratch. You'll need:

- A Proxmox VE server (any version 7.x or 8.x+)
- A machine to run pCenter on (an LXC container on your Proxmox host works great)
- About 10 minutes

### Quick Install (Ubuntu/Debian)

If you already have an Ubuntu/Debian machine ready and a Proxmox API token (see [Step 1](#step-1-create-the-proxmox-api-token) if you need one):

```bash
# Prerequisites (stock LXC templates often ship without these)
sudo apt update
sudo apt install -y curl gpg ca-certificates

# Add the pCenter APT repository
curl -fsSL https://marcwoconnor.github.io/pCenter/pcenter.gpg.key \
  | sudo gpg --dearmor -o /usr/share/keyrings/pcenter.gpg

echo "deb [signed-by=/usr/share/keyrings/pcenter.gpg] https://marcwoconnor.github.io/pCenter stable main" \
  | sudo tee /etc/apt/sources.list.d/pcenter.list

# Install
sudo apt update
sudo apt install pcenter

# Configure
sudo nano /etc/pcenter/config.yaml
# Set your Proxmox IP and API token (see Step 1 below)

# Start
sudo systemctl start pcenter
```

Then open `http://<your-ip>:8080` in your browser and jump to [Step 6: First Login](#step-6-first-login).

> The APT package puts config in `/etc/pcenter/config.yaml` (+ `/etc/pcenter/env` for secrets) and the binary/data in `/opt/pcenter/`, with a pre-built systemd unit. **If you used Quick Install, skip Steps 3–5 below — they describe a manual flow that keeps config inside `/opt/pcenter/` instead of `/etc/pcenter/`.**

To upgrade later: `sudo apt update && sudo apt upgrade pcenter`

---

### Manual Install (Source Build)

If you'd rather build from source or install without APT, follow the steps below. This flow uses `/opt/pcenter/` as the install root — that's different from the APT layout above, and the two should not be mixed.

### Step 1: Create the Proxmox API Token

pCenter connects to your Proxmox server using an API token. You need to create one in the Proxmox web UI.

1. Open your Proxmox web UI (usually `https://your-proxmox-ip:8006`)
2. Go to **Datacenter** > **Permissions** > **API Tokens**
3. Click **Add**
4. Fill in:
   - **User**: `root@pam`
   - **Token ID**: `pcenter` (or any name you like)
   - **Privilege Separation**: **Uncheck this** (important! pCenter needs full access)
5. Click **Add**
6. **Copy the token secret** — you'll only see it once! It looks like: `a1b2c3d4-e5f6-7890-abcd-ef1234567890`

Your full token ID will be: `root@pam!pcenter`

> **Security note**: Using `root@pam` with privilege separation disabled gives pCenter full access to your Proxmox cluster. For a home lab this is fine. For production, consider creating a dedicated user with only the permissions pCenter needs.

### Step 2: Prepare a Machine to Run pCenter

pCenter is a single binary — it can run on any Linux machine that can reach your Proxmox API. The easiest option is to create a lightweight LXC container on your Proxmox host.

#### Option A: Create an LXC container (recommended)

In the Proxmox web UI:

1. Click **Create CT**
2. Set:
   - **Hostname**: `pcenter`
   - **Template**: Ubuntu 24.04 (download from the template list if needed)
   - **Disk**: 4 GB is plenty
   - **CPU**: 1 core
   - **Memory**: 512 MB
   - **Network**: DHCP or static IP on your LAN
3. Start the container and note its IP address

Then SSH into the container:
```bash
ssh root@<container-ip>
```

#### Option B: Use any existing Linux machine

Any Ubuntu/Debian machine on the same network as your Proxmox server will work. pCenter just needs to reach port 8006 on your Proxmox nodes.

### Step 3: Download and Install pCenter

SSH into your pCenter machine and run:

```bash
# Create the install directory
mkdir -p /opt/pcenter/data
mkdir -p /opt/pcenter/frontend
cd /opt/pcenter
```

If you have a pre-built release:
```bash
# Copy the binary and frontend files to /opt/pcenter/
# (see "Building from Source" if you don't have a release)
```

If building from source, see [Building from Source](#building-from-source) below, then come back here for configuration.

### Step 4: Configure pCenter

Create the configuration file:

```bash
cat > /opt/pcenter/config.yaml << 'EOF'
clusters:
  - name: my-cluster
    discovery_node: "YOUR_PROXMOX_IP:8006"
    token_id: "root@pam!pcenter"
    token_secret: "YOUR_TOKEN_SECRET_HERE"
    insecure: true

server:
  port: 8080

auth:
  enabled: true

metrics:
  enabled: true

drs:
  enabled: true
  mode: manual
EOF
```

Replace:
- `YOUR_PROXMOX_IP` with your Proxmox node's IP address (e.g., `192.168.1.100`)
- `YOUR_TOKEN_SECRET_HERE` with the token secret you copied in Step 1

> **Tip**: If you have a Proxmox cluster with multiple nodes, you only need to specify ONE node IP. pCenter auto-discovers the rest.

#### Using environment variables for secrets (optional but recommended)

Instead of putting the token secret directly in config.yaml, you can use an environment variable:

```bash
# In config.yaml, use:
#   token_secret: ${PVE_TOKEN_SECRET}

# Create the env file:
echo "PVE_TOKEN_SECRET=your-actual-token-secret" > /opt/pcenter/.env
chmod 600 /opt/pcenter/.env
```

### Step 5: Run pCenter

#### Quick test (foreground)

```bash
cd /opt/pcenter
./pcenter -config config.yaml
```

You should see output like:
```
level=INFO msg="loaded config" clusters=1 port=8080
level=INFO msg="starting server" addr=:8080
level=INFO msg="cluster state loaded" clusters=1 nodes="2/2 online" vms="5/10 running" containers="8/12 running"
```

Open your browser to `http://<pcenter-ip>:8080` — you should see the login page.

Press `Ctrl+C` to stop the foreground process.

#### Set up as a system service (recommended)

Create the systemd service file:

```bash
cat > /etc/systemd/system/pcenter.service << 'EOF'
[Unit]
Description=pCenter - Proxmox Datacenter Manager
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/pcenter
ExecStart=/opt/pcenter/pcenter -config /opt/pcenter/config.yaml
Restart=always
RestartSec=5
EnvironmentFile=-/opt/pcenter/.env

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/pcenter/data
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
```

Enable and start:
```bash
systemctl daemon-reload
systemctl enable pcenter
systemctl start pcenter
```

Check it's running:
```bash
systemctl status pcenter
```

### Step 6: First Login

1. Open `http://<pcenter-ip>:8080` in your browser
2. Since this is the first time, you'll see a **Create Account** page
3. Create your admin account (username + password, minimum 8 characters)
4. Log in with your new credentials
5. You should see the pCenter dashboard with your Proxmox cluster data

#### Optional: Enable 2FA

1. Go to **Settings** (gear icon or `/settings`)
2. Under **Two-Factor Authentication**, click **Enable**
3. Scan the QR code with your authenticator app (Google Authenticator, Authy, etc.)
4. Enter the verification code
5. **Save your recovery codes** in a safe place

---

## Building from Source

### Prerequisites

- Go 1.24+ ([install Go](https://go.dev/doc/install))
- Node.js 18+ ([install Node.js](https://nodejs.org/))
- Git

### Build

```bash
# Clone the repository
git clone https://github.com/yourusername/pCenter.git
cd pCenter

# Build the frontend
cd frontend
npm install
npm run build
cd ..

# Build the backend
cd backend
GOOS=linux go build -o pcenter ./cmd/server
cd ..
```

### Deploy to your pCenter machine

```bash
# Copy the binary
scp backend/pcenter root@<pcenter-ip>:/opt/pcenter/

# Copy the frontend
rsync -avz --delete frontend/dist/ root@<pcenter-ip>:/opt/pcenter/frontend/

# Restart the service
ssh root@<pcenter-ip> "systemctl restart pcenter"
```

---

## Configuration Reference

All configuration is in `config.yaml`. Environment variables can be used with `${VAR_NAME}` syntax.

### Clusters

```yaml
clusters:
  - name: my-cluster              # Display name for this cluster
    discovery_node: "10.0.0.1:8006"  # IP:port of ANY node in the cluster
    token_id: "root@pam!pcenter"     # PVE API token ID
    token_secret: "${PVE_TOKEN}"     # Token secret (use env var)
    insecure: true                   # Skip TLS verification (for self-signed certs)
```

Only one node IP is needed per cluster — pCenter auto-discovers the rest.

### Server

```yaml
server:
  port: 8080                # HTTP port
  cors_origins:             # Allowed CORS origins (for dev or reverse proxy)
    - http://localhost:5173
```

### Authentication

```yaml
auth:
  enabled: true             # Enable login (strongly recommended)
  database_path: "data/auth.db"
  session:
    duration_hours: 24      # Session lifetime
    idle_timeout_hours: 8   # Expire after inactivity
  lockout:
    max_attempts: 5         # Lock after N failed logins
    lockout_minutes: 15     # Lock duration
    progressive: true       # Double lockout time on repeated lockouts
  totp:
    enabled: true           # Allow 2FA setup
    required: false         # Force all users to enable 2FA
    trust_ip_hours: 24      # Skip 2FA from trusted IPs for N hours
  rate_limit:
    requests_per_minute: 10
```

### DRS (Distributed Resource Scheduler)

```yaml
drs:
  enabled: true
  mode: manual              # manual | semi-automatic | fully-automatic
  check_interval: 300       # Seconds between analysis runs
  cpu_threshold: 0.8        # Recommend migration when node CPU > 80%
  mem_threshold: 0.85       # Recommend migration when node memory > 85%
  migration_rate: 2         # Max concurrent migrations per cluster
```

### Metrics

```yaml
metrics:
  enabled: true
  database_path: "data/metrics.db"
  collection_interval: 30   # Seconds between metric samples
  retention:
    raw_hours: 24           # Keep 30-sec samples for 24 hours
    hourly_days: 7          # Keep hourly rollups for 7 days
    daily_days: 30          # Keep daily rollups for 30 days
    weekly_months: 12       # Keep weekly rollups for 1 year
```

### Activity Logging

```yaml
activity:
  database_path: "data/activity.db"
  retention_days: 30        # Keep audit logs for 30 days
```

---

## Managing Multiple Clusters

Add more clusters to the `clusters` array in `config.yaml`:

```yaml
clusters:
  - name: home-lab
    discovery_node: "192.168.1.100:8006"
    token_id: "root@pam!pcenter"
    token_secret: "${PVE_TOKEN_HOME}"
    insecure: true

  - name: office
    discovery_node: "10.0.0.50:8006"
    token_id: "root@pam!pcenter"
    token_secret: "${PVE_TOKEN_OFFICE}"
    insecure: true
```

Each cluster needs its own API token. pCenter shows all clusters in a unified view, or you can filter by cluster.

---

## Upgrading

### APT (recommended)

```bash
sudo apt update && sudo apt upgrade pcenter
```

### Manual

```bash
# Build new version (or download release)
cd pCenter
git pull
cd frontend && npm install && npm run build && cd ..
cd backend && GOOS=linux go build -o pcenter ./cmd/server && cd ..

# Deploy
ssh root@<pcenter-ip> "systemctl stop pcenter"
scp backend/pcenter root@<pcenter-ip>:/opt/pcenter/
rsync -avz --delete frontend/dist/ root@<pcenter-ip>:/opt/pcenter/frontend/
ssh root@<pcenter-ip> "systemctl start pcenter"
```

Your data (metrics, users, settings) is stored in SQLite databases under `/opt/pcenter/data/` and is preserved across upgrades.

---

## Troubleshooting

### White screen after login

Check the backend logs:
```bash
journalctl -u pcenter -n 20 --no-pager
```

If you see `API error 401`, your Proxmox API token is invalid or expired. Regenerate it:
1. Go to Proxmox UI > Datacenter > Permissions > API Tokens
2. Delete the old token and create a new one
3. Update the secret in `/opt/pcenter/.env` or `config.yaml`
4. Restart: `systemctl restart pcenter`

### "cluster state loaded" shows 0/0 nodes

The token doesn't have access, or the Proxmox node IP is wrong. Verify:
```bash
curl -sk https://YOUR_PROXMOX_IP:8006/api2/json/version \
  -H "Authorization: PVEAPIToken=root@pam!pcenter=YOUR_TOKEN_SECRET"
```

You should get a JSON response with the PVE version. If you get `401` or connection refused, check the IP and token.

### Can't reach pCenter at port 8080

- Check the service is running: `systemctl status pcenter`
- Check firewall: `ufw allow 8080` or `iptables -L`
- If running in an LXC, make sure the container's network is configured

### Connection lost / WebSocket disconnects

Usually caused by a reverse proxy that doesn't support WebSockets. If using nginx:
```nginx
location /ws {
    proxy_pass http://pcenter-ip:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 86400;
}
```

### Uninstalling

```bash
# Keep your data:
sudo apt remove pcenter

# Remove everything including data and config:
sudo apt purge pcenter
```

### Metrics not showing

Check that metrics are enabled in config:
```yaml
metrics:
  enabled: true
```

Metrics take about 30 seconds to start appearing after pCenter starts.

---

## Architecture

```
                    Browser (React + TypeScript)
                         |
                    WebSocket + REST API
                         |
                   +-----+-----+
                   |  pCenter   |  Single Go binary
                   |  Backend   |  SQLite databases
                   +--+-----+--+
                      |     |
            +---------+     +---------+
            | Proxmox API             | Agent WebSocket
            v                         v
     +----------+             +----------+
     |  Node 1  |             | pve-agent|  Optional per-node
     |  Node 2  |             | pve-agent|  daemon for real-time
     |  ...     |             | pve-agent|  metrics & commands
     +----------+             +----------+
```

**Two data collection modes:**
- **Poller** (default): pCenter pulls from Proxmox API every 5 seconds
- **Agent** (optional): pve-agent on each node pushes data via WebSocket

Both modes can run simultaneously. Agent data takes priority when available.

### Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, stdlib HTTP router, SQLite |
| Frontend | React 18, TypeScript, Vite, TailwindCSS |
| Real-time | WebSocket (gorilla/websocket) |
| Console | noVNC (VMs), VNC terminal (containers) |
| Deployment | Single binary + static assets |

### Databases

All SQLite, all local, no external DB required:

| Database | Purpose | Default Path |
|----------|---------|-------------|
| auth.db | Users, sessions, TOTP, events | data/auth.db |
| metrics.db | Time-series metrics with rollup | data/metrics.db |
| folders.db | Folder hierarchy + members | data/folders.db |
| inventory.db | Datacenters, clusters, hosts | data/inventory.db |
| activity.db | Audit trail | data/activity.db |
| library.db | Content library items | data/library.db |

---

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

# Configuration
GET  /api/clusters/{cluster}/vms/{vmid}/config
PUT  /api/clusters/{cluster}/vms/{vmid}/config

# Metrics
GET  /api/metrics/node/{node}?start=...&end=...&metrics=cpu,mem_percent

# WebSocket
GET  /ws                              # Real-time state updates
```

---

## Development

### Prerequisites
- Go 1.24+
- Node.js 18+

### Running locally

```bash
# Backend (terminal 1)
cd backend && go run ./cmd/server -config ../config.yaml

# Frontend (terminal 2)
cd frontend && npm install && npm run dev
```

Frontend dev server runs at `http://localhost:5173` and proxies API calls to the backend at `:8080`.

### Running tests

```bash
cd backend && go test ./...
cd frontend && npm run lint
```

---

## Project Structure

```
pCenter/
├── backend/
│   ├── cmd/server/          # Entry point
│   └── internal/
│       ├── api/             # HTTP handlers, WebSocket, console proxy
│       ├── agent/           # Agent hub + command tracking
│       ├── auth/            # Sessions, TOTP, lockout, CSRF
│       ├── config/          # YAML config loading
│       ├── drs/             # Load analyzer + recommendations
│       ├── folders/         # Folder hierarchy (SQLite)
│       ├── inventory/       # Datacenter/cluster/host management
│       ├── library/         # Content library
│       ├── metrics/         # Collection, rollup, retention (SQLite)
│       ├── pve/             # Proxmox API client
│       ├── poller/          # Background node polling
│       └── state/           # In-memory state cache
├── frontend/
│   └── src/
│       ├── api/             # API client
│       ├── components/      # React components
│       ├── context/         # Cluster + Auth context providers
│       ├── hooks/           # Custom hooks
│       ├── pages/           # Page components
│       └── types/           # TypeScript types
├── config.yaml              # Main configuration
└── docs/                    # Architecture docs
```

---

## Roadmap & Contributing

- **Changelog:** [`CHANGELOG.md`](CHANGELOG.md) — what shipped in each release.
- **Feature roadmap:** [`docs/vcenter-feature-parity-roadmap.md`](docs/vcenter-feature-parity-roadmap.md) — vCenter feature parity matrix and phase planning.
- **Open work:** [Phase 3 issues](https://github.com/marcwoconnor/pCenter/issues?q=is%3Aissue+is%3Aopen+label%3Aphase-3) and [Phase 4 issues](https://github.com/marcwoconnor/pCenter/issues?q=is%3Aissue+is%3Aopen+label%3Aphase-4) track the next features.
- **Issue labels:** `roadmap` (tracked feature), `phase-3`/`phase-4` (milestone), `effort-s`/`effort-m`/`effort-l` (rough size).

Pre-1.0: the project is held at `0.x.y` until it's genuinely production-ready. Breaking changes may land between minor releases until `v1.0`.

---

## License

MIT
