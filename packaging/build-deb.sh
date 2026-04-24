#!/bin/bash
set -euo pipefail

# Build a .deb package for pCenter
# Usage: ./packaging/build-deb.sh [version]
# Example: ./packaging/build-deb.sh 1.0.0

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
echo "Building backend..."
cd backend
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o pcenter ./cmd/server
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
cat > "${PKG_DIR}/etc/pcenter/config.yaml" << 'CONF'
# pCenter Configuration
# Edit this file, then restart: systemctl restart pcenter
#
# Full reference: https://github.com/marcwoconnor/pCenter#configuration-reference
#
# Fresh installs start with an empty inventory. Add your first datacenter and
# Proxmox host through the web UI after pCenter is running — it will auto-detect
# whether the host is part of a real PVE cluster (via /cluster/status) and file
# it under a pcenter cluster or as a standalone accordingly.
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
Type=simple
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

# Enable service (but don't start — user needs to configure first)
systemctl enable pcenter 2>/dev/null || true

echo ""
echo "=========================================="
echo "  pCenter installed successfully!"
echo "=========================================="
echo ""
echo "  Next steps:"
echo "  1. Edit /etc/pcenter/config.yaml"
echo "     - Set your Proxmox IP and API token"
echo ""
echo "  2. (Optional) Set token secret in /etc/pcenter/env:"
echo "     PVE_TOKEN_SECRET=your-secret-here"
echo ""
echo "  3. Start pCenter:"
echo "     systemctl start pcenter"
echo ""
echo "  4. Open http://$(hostname -I | awk '{print $1}'):8080"
echo ""
echo "=========================================="

POST
chmod 755 "${PKG_DIR}/DEBIAN/postinst"

# --- Pre-remove script ---
cat > "${PKG_DIR}/DEBIAN/prerm" << 'PRERM'
#!/bin/bash
set -e

# Stop service before removal
systemctl stop pcenter 2>/dev/null || true
systemctl disable pcenter 2>/dev/null || true

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
