#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/opt/ezweb"
SERVICE_USER="ezweb"

UPGRADE=false
if [ -f "$INSTALL_DIR/ezweb" ] && systemctl is-active --quiet ezweb 2>/dev/null; then
    UPGRADE=true
fi

echo "=== EzWeb Installer ==="
if [ "$UPGRADE" = true ]; then
    echo "Existing installation detected — running upgrade."
fi
echo ""

# Check if running as root
if [ "$(id -u)" -ne 0 ]; then
    echo "Please run this script as root (sudo ./install.sh)"
    exit 1
fi

# Check dependencies
for cmd in docker curl; do
    if ! command -v "$cmd" &> /dev/null; then
        echo "Missing dependency: $cmd"
        echo "Please install $cmd and try again."
        exit 1
    fi
done

# Create service user if it doesn't exist
if ! id "$SERVICE_USER" &> /dev/null; then
    echo "Creating service user: $SERVICE_USER"
    useradd --system --home-dir "$INSTALL_DIR" --shell /usr/sbin/nologin "$SERVICE_USER"
    usermod -aG docker "$SERVICE_USER"
fi

# Create directories
echo "Setting up directories..."
mkdir -p "$INSTALL_DIR"/{backups,data}

# Stop service before upgrading binary
if [ "$UPGRADE" = true ]; then
    echo "Stopping ezweb service for upgrade..."
    systemctl stop ezweb
fi

# Copy binary if present
if [ -f "./ezweb" ]; then
    cp ./ezweb "$INSTALL_DIR/ezweb"
    chmod +x "$INSTALL_DIR/ezweb"
    echo "Binary copied to $INSTALL_DIR"
else
    echo "Warning: No ezweb binary found in current directory."
    echo "  Run 'make prod-build' first, then copy the binary to $INSTALL_DIR"
fi

# Copy static files
if [ -d "./static" ]; then
    cp -r ./static "$INSTALL_DIR/"
    echo "Static files copied"
fi

# Create .env if it doesn't exist
if [ ! -f "$INSTALL_DIR/.env" ]; then
    JWT_SECRET=$(openssl rand -hex 32)
    cat > "$INSTALL_DIR/.env" << EOF
APP_PORT=3000
JWT_SECRET=$JWT_SECRET
ADMIN_USER=admin
ADMIN_PASS=changeme
DB_PATH=$INSTALL_DIR/data/ezweb.db
BACKUP_DIR=$INSTALL_DIR/backups
CADDYFILE_PATH=/etc/caddy/Caddyfile
SECURE_COOKIES=true
METRICS_ENABLED=true
EOF
    echo ""
    echo "Created .env with generated JWT secret."
    echo "IMPORTANT: Edit $INSTALL_DIR/.env and change ADMIN_PASS!"
fi

# Set ownership
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"

# Install systemd services
if [ -d "/etc/systemd/system" ]; then
    cp ./contrib/ezweb.service /etc/systemd/system/
    cp ./contrib/ezweb-backup.service /etc/systemd/system/
    cp ./contrib/ezweb-backup.timer /etc/systemd/system/
    systemctl daemon-reload

    echo ""
    echo "Systemd services installed. To start:"
    echo "  systemctl enable --now ezweb"
    echo "  systemctl enable --now ezweb-backup.timer"
fi

# Restart service after upgrade
if [ "$UPGRADE" = true ]; then
    echo "Restarting ezweb service..."
    systemctl start ezweb
    echo ""
    echo "=== Upgrade complete ==="
    echo ""
    echo "Service restarted. Schema migrations run automatically on startup."
else
    echo ""
    echo "=== Installation complete ==="
    echo ""
    echo "Next steps:"
    echo "  1. Edit $INSTALL_DIR/.env (set a strong ADMIN_PASS)"
    echo "  2. Start the service: systemctl start ezweb"
    echo "  3. Access the dashboard at http://localhost:3000"
fi
echo ""
