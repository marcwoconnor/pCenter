#!/bin/bash
set -euo pipefail

# Build a .deb package for pCenter
# Usage: ./packaging/build-deb.sh [version]
# Example: ./packaging/build-deb.sh 1.0.0

# Preflight: Vite 7 requires Node 20+. On older Node it warns but keeps
# going, silently emitting a degraded bundle (no per-route code-splitting),
# which has shipped broken .debs before. Refuse rather than do that.
# Required version is pinned in frontend/.nvmrc.
node_major=$(node --version 2>/dev/null | sed -n 's/^v\([0-9][0-9]*\).*/\1/p')
if [ -z "$node_major" ]; then
    echo "error: 'node' not found on PATH. Install Node 20+ (see frontend/.nvmrc)." >&2
    exit 2
fi
required_major=$(cat "$(dirname "$0")/../frontend/.nvmrc" | tr -d '[:space:]' | sed 's/^v//')
if [ "$node_major" -lt "$required_major" ]; then
    echo "error: Node ${node_major} detected; build requires Node ${required_major}+ (see frontend/.nvmrc). With nvm: 'nvm use' from frontend/." >&2
    exit 2
fi

VERSION="${1:-$(git describe --tags --always 2>/dev/null || echo '0.0.1')}"
VERSION="${VERSION#v}"  # strip leading 'v'
ARCH="amd64"
PKG_NAME="pcenter"
PKG_DIR="$(mktemp -d)/pcenter_${VERSION}_${ARCH}"

echo "Building pCenter ${VERSION} for ${ARCH}..."

# --- Build frontend ---
echo "Building frontend..."
cd frontend
npm ci --silent
npm run build
cd ..

# --- Build backend ---
# -X injects the version into internal/updater so the running binary knows
# what release it is (used by the in-app version banner and update checker).
# Must match the CI workflow's ldflags so locally-built and CI-built .debs
# produce identical binaries.
echo "Building backend..."
cd backend
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X github.com/moconnor/pcenter/internal/updater.Version=${VERSION}" \
    -o pcenter ./cmd/server
cd ..

# --- Assemble .deb structure ---
mkdir -p "${PKG_DIR}/DEBIAN"
mkdir -p "${PKG_DIR}/opt/pcenter/frontend"
mkdir -p "${PKG_DIR}/opt/pcenter/data"
mkdir -p "${PKG_DIR}/etc/pcenter"
mkdir -p "${PKG_DIR}/lib/systemd/system"

# Binary
cp backend/pcenter "${PKG_DIR}/opt/pcenter/pcenter"
chmod 755 "${PKG_DIR}/opt/pcenter/pcenter"

# Frontend assets
cp -r frontend/dist/* "${PKG_DIR}/opt/pcenter/frontend/"

# Default config
# Intentionally ships with no `clusters:` stanza: a fresh Proxmox install has no
# cluster and no API token yet, so pre-filling this file would leave new users
# editing placeholder values before they can even start the service. Instead,
# the service boots clean and users add their first host through the UI, which
# authenticates with username/password and auto-creates an API token.
cat > "${PKG_DIR}/etc/pcenter/config.yaml" << 'CONF'
# pCenter Configuration
# Edit this file, then restart: systemctl restart pcenter
#
# Full reference: https://github.com/marcwoconnor/pCenter#configuration-reference
#
# Fresh installs start with an empty inventory. Add your first datacenter and
# Proxmox host through the web UI after pCenter is running — it will auto-detect
# whether the host is part of a real PVE cluster (via /cluster/status) and file
# it under a pcenter cluster or as a standalone accordingly. The "Create
# Proxmox Cluster" and "Add Member Node" wizards in the Hosts & Clusters tab
# can also form/extend Corosync clusters from inside pCenter.
#
# Legacy bootstrap: if you prefer to seed hosts from config, uncomment the
# clusters: block below. Each entry is probed on first start; real PVE clusters
# become pcenter clusters, standalone nodes become standalone hosts under
# the auto-created "Default" datacenter.
#
# clusters:
#   - name: legacy-bootstrap
#     discovery_node: "YOUR_PROXMOX_IP:8006"
#     token_id: "root@pam!pcenter"
#     token_secret: "${PVE_TOKEN_SECRET}"
#     insecure: true

poller:
  enabled: true   # Set to false to disable background polling (dashboard will be empty)

server:
  port: 8080

auth:
  enabled: true
  database_path: "/opt/pcenter/data/auth.db"
  encryption_key: "${PCENTER_ENCRYPTION_KEY}"  # Auto-generated on first run

metrics:
  enabled: true
  database_path: "/opt/pcenter/data/metrics.db"

activity:
  database_path: "/opt/pcenter/data/activity.db"

folders:
  database_path: "/opt/pcenter/data/folders.db"

inventory:
  database_path: "/opt/pcenter/data/inventory.db"

library:
  enabled: true
  database_path: "/opt/pcenter/data/library.db"

drs:
  enabled: true
  mode: manual
CONF

# Systemd service
cat > "${PKG_DIR}/lib/systemd/system/pcenter.service" << 'SVC'
[Unit]
Description=pCenter - Proxmox Datacenter Manager
After=network.target
Documentation=https://github.com/marcwoconnor/pCenter

[Service]
# Type=notify so systemd considers the unit "active" only after the Go
# binary has sent READY=1 via sd_notify — which happens AFTER the HTTP
# listener is bound. Closes the deploy-script race where
# `systemctl restart && curl /health` hit the listener during ~2s init
# (#36). NotifyAccess=main restricts which processes can notify.
Type=notify
NotifyAccess=main
# systemd's default TimeoutStartSec=90s is fine for our ~2s init, but
# if startup ever drags (e.g. slow DB migration) we want the unit to
# fail cleanly rather than hang forever.
TimeoutStartSec=60s
WorkingDirectory=/opt/pcenter
ExecStart=/opt/pcenter/pcenter -config /etc/pcenter/config.yaml
Restart=always
RestartSec=5
Environment=HOME=/opt/pcenter/data
EnvironmentFile=-/etc/pcenter/env

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/pcenter/data
PrivateTmp=true

[Install]
WantedBy=multi-user.target
SVC

# Symlink for convenience
mkdir -p "${PKG_DIR}/usr/local/bin"
# We'll use a postinst script to create the symlink

# --- DEBIAN control file ---
cat > "${PKG_DIR}/DEBIAN/control" << CTRL
Package: ${PKG_NAME}
Version: ${VERSION}
Architecture: ${ARCH}
Maintainer: Marc O'Connor <marc@techsnet.net>
Description: Proxmox Datacenter Manager
 A vCenter-like management platform for Proxmox VE.
 Multi-cluster, real-time dashboard with VM/container lifecycle,
 live migration, DRS, HA, metrics, snapshots, and more.
 Single binary + web UI. No external databases required.
Section: admin
Priority: optional
Homepage: https://github.com/marcwoconnor/pCenter
Depends: libc6
Installed-Size: $(du -sk "${PKG_DIR}" | cut -f1)
CTRL

# --- Post-install script ---
cat > "${PKG_DIR}/DEBIAN/postinst" << 'POST'
#!/bin/bash
set -e

# postinst is invoked with "configure <old-version>" on both fresh install
# and upgrade. We branch on $2 (the old version) to decide whether to start
# (fresh install — wait for the operator to configure) or restart
# (upgrade — the prerm stopped the service before the binary was swapped
# and we need to bring it back). #55.
FRESH_INSTALL=0
if [ -z "${2:-}" ]; then
    FRESH_INSTALL=1
fi

# Create symlink
ln -sf /opt/pcenter/pcenter /usr/local/bin/pcenter

# Create env file if it doesn't exist
if [ ! -f /etc/pcenter/env ]; then
    echo "# pCenter environment variables" > /etc/pcenter/env
    echo "# PVE_TOKEN_SECRET=your-token-secret-here" >> /etc/pcenter/env
    chmod 600 /etc/pcenter/env
fi

# Seed encryption key on first install if missing.
# The runtime can generate one too, but systemd ProtectSystem=strict blocks
# the persist-to-/etc/pcenter path, causing silent TOTP/webhook-secret loss
# across restarts. Generating here guarantees a stable key from day one.
if ! grep -q '^PCENTER_ENCRYPTION_KEY=' /etc/pcenter/env 2>/dev/null; then
    key=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')
    echo "PCENTER_ENCRYPTION_KEY=${key}" >> /etc/pcenter/env
    chmod 600 /etc/pcenter/env
fi

# Set permissions on data directory
chmod 750 /opt/pcenter/data

# Reload systemd
systemctl daemon-reload

# Enable service so it starts on boot
systemctl enable pcenter 2>/dev/null || true

# On upgrade: restart the service so the new binary takes over. The prerm
# stopped the old process; without an explicit restart here the service
# would stay inactive until the operator intervened — #55.
# Guarded by is-enabled so an operator who deliberately disabled the service
# before the upgrade doesn't get it surprise-started again.
# On fresh install: leave it stopped so the operator can register a user
# and add hosts via the UI before the dashboard goes live.
if [ "$FRESH_INSTALL" = "0" ]; then
    if systemctl is-enabled pcenter.service >/dev/null 2>&1; then
        systemctl restart pcenter.service 2>/dev/null || true
    fi
fi

echo ""
echo "=========================================="
if [ "$FRESH_INSTALL" = "1" ]; then
    echo "  pCenter installed successfully!"
    echo "=========================================="
    echo ""
    echo "  Next steps:"
    echo "  1. Start pCenter:"
    echo "       systemctl start pcenter"
    echo ""
    echo "  2. Open http://$(hostname -I | awk '{print $1}'):8080"
    echo ""
    echo "  3. Register the first user (becomes admin)."
    echo ""
    echo "  4. In the UI, click 'Add a host' and enter"
    echo "     your Proxmox address + root password."
    echo "     An API token is created for you."
else
    echo "  pCenter upgraded successfully!"
    echo "=========================================="
    echo ""
    echo "  Service restarted automatically."
    echo "  Check status: systemctl status pcenter"
fi
echo ""
echo "=========================================="

POST
chmod 755 "${PKG_DIR}/DEBIAN/postinst"

# --- Pre-remove script ---
# prerm is invoked with "upgrade <new-version>" on upgrade and "remove" on
# uninstall. We must stop the service in both cases (binary is about to be
# replaced or removed), but only `disable` on actual removal — disabling on
# upgrade would surprise-disable a service the operator deliberately left
# enabled, and the postinst's `enable` would then race against an operator
# who *had* deliberately disabled it.
cat > "${PKG_DIR}/DEBIAN/prerm" << 'PRERM'
#!/bin/bash
set -e

systemctl stop pcenter 2>/dev/null || true
if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
    systemctl disable pcenter 2>/dev/null || true
fi

PRERM
chmod 755 "${PKG_DIR}/DEBIAN/prerm"

# --- Post-remove script ---
cat > "${PKG_DIR}/DEBIAN/postrm" << 'POSTRM'
#!/bin/bash
set -e

# Remove symlink
rm -f /usr/local/bin/pcenter

# Reload systemd
systemctl daemon-reload

if [ "$1" = "purge" ]; then
    # Remove data and config on purge
    rm -rf /opt/pcenter/data
    rm -rf /etc/pcenter
    rm -rf /opt/pcenter
fi

POSTRM
chmod 755 "${PKG_DIR}/DEBIAN/postrm"

# --- Mark config as conffile (apt won't overwrite user edits) ---
cat > "${PKG_DIR}/DEBIAN/conffiles" << 'CONFFILES'
/etc/pcenter/config.yaml
CONFFILES

# --- Build the .deb ---
DEB_FILE="pcenter_${VERSION}_${ARCH}.deb"
dpkg-deb --build "${PKG_DIR}" "${DEB_FILE}"

echo ""
echo "Built: ${DEB_FILE}"
echo "Size: $(du -h "${DEB_FILE}" | cut -f1)"

# Cleanup
rm -rf "$(dirname "${PKG_DIR}")"
