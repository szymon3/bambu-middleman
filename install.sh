#!/usr/bin/env bash
# install.sh — install or upgrade bambu-observer on a Linux host.
#
# Usage:
#   sudo bash install.sh            # first-time install
#   sudo bash install.sh --upgrade  # swap binary, leave config untouched
#
# Requires: curl, sha256sum, useradd, systemctl (standard on most Linux distros).
set -euo pipefail

REPO="szymon3/bambu-middleman"
BINARY_NAME="bambu-observer"
INSTALL_BIN="/usr/local/bin/bambu-observer"
CONF_DIR="/etc/bambu-observer"
UNIT_PATH="/etc/systemd/system/bambu-observer.service"
SERVICE_USER="bambu-observer"
UPGRADE=false

# ── argument parsing ──────────────────────────────────────────────────────────

for arg in "$@"; do
  case "$arg" in
    --upgrade) UPGRADE=true ;;
    *) echo "Unknown argument: $arg" >&2; exit 1 ;;
  esac
done

# ── helpers ───────────────────────────────────────────────────────────────────

die()  { echo "ERROR: $*" >&2; exit 1; }
info() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }

[[ $EUID -eq 0 ]] || die "This script must be run as root (try: sudo bash install.sh $*)"

for cmd in curl sha256sum; do
  command -v "$cmd" &>/dev/null || die "Required tool not found: $cmd"
done

if ! $UPGRADE; then
  for cmd in useradd systemctl; do
    command -v "$cmd" &>/dev/null || die "Required tool not found: $cmd"
  done
fi

detect_arch() {
  case "$(uname -m)" in
    x86_64)  echo "amd64" ;;
    aarch64) echo "arm64" ;;
    *) die "Unsupported architecture: $(uname -m). Only amd64 and arm64 are supported." ;;
  esac
}

fetch_latest_tag() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | head -1 \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
}

# ── resolve release ───────────────────────────────────────────────────────────

ARCH=$(detect_arch)
ASSET="${BINARY_NAME}_linux_${ARCH}"

info "Fetching latest release tag..."
TAG=$(fetch_latest_tag)
[[ -n "$TAG" ]] || die "Could not determine latest release tag from GitHub API."
info "Latest release: $TAG"

# ── download and verify ───────────────────────────────────────────────────────

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

info "Downloading ${ASSET}..."
curl -fSL --progress-bar "${BASE_URL}/${ASSET}" -o "${WORKDIR}/${ASSET}"
curl -fsSL "${BASE_URL}/sha256sums.txt" -o "${WORKDIR}/sha256sums.txt"

info "Verifying checksum..."
(cd "$WORKDIR" && grep "  ${ASSET}$" sha256sums.txt | sha256sum -c -)
info "Checksum OK."

chmod +x "${WORKDIR}/${ASSET}"

# ── upgrade path ──────────────────────────────────────────────────────────────

if $UPGRADE; then
  info "Stopping service..."
  systemctl stop bambu-observer || true

  info "Installing new binary..."
  install -m 755 "${WORKDIR}/${ASSET}" "${INSTALL_BIN}"

  info "Starting service..."
  systemctl start bambu-observer

  info "Upgrade complete. Configuration in ${CONF_DIR}/env was not modified."
  exit 0
fi

# ── first-time install ────────────────────────────────────────────────────────

# System user
if id -u "$SERVICE_USER" &>/dev/null; then
  info "System user '${SERVICE_USER}' already exists."
else
  info "Creating system user '${SERVICE_USER}'..."
  useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
fi

# Binary
info "Installing binary to ${INSTALL_BIN}..."
install -m 755 "${WORKDIR}/${ASSET}" "${INSTALL_BIN}"

# Config directory
install -d -m 755 "$CONF_DIR"

# env.example — always write so it stays current
info "Writing ${CONF_DIR}/env.example..."
cat > "${CONF_DIR}/env.example" <<'EOF'
# /etc/bambu-observer/env
# Bambu Observer configuration.
# Restart the service after any change: systemctl restart bambu-observer

# ── Required ──────────────────────────────────────────────────────────────────

# Local IP address of the printer
PRINTER_IP=192.168.1.100

# Printer serial number (on the printer label or in the Bambu app)
PRINTER_SERIAL=01P00A000000001

# 8-digit access code shown on the printer screen (Settings → LAN Mode)
PRINTER_ACCESS_CODE=12345678

# ── Optional ──────────────────────────────────────────────────────────────────

# Log verbosity. DEBUG for verbose output; default is INFO.
#LOG_LEVEL=INFO

# Base URL of a Spoolman instance — enables automatic spool-usage updates.
#SPOOLMAN_URL=http://192.168.1.10:7912

# Ordered list of spool ID sources tried left-to-right.
# "api" = active spool set via the web UI; "notes" = spoolman#N tag in filament notes.
# Default: api,notes
#SPOOLMAN_SOURCE=api,notes

# Address to bind the built-in HTTP server — enables active spool tracking.
#WEBUI_ADDR=:8080

# Externally reachable base URL of the HTTP server — baked into generated QR codes.
#WEBUI_BASE_URL=http://192.168.1.10:8080

# Filesystem path for the audit log SQLite database.
#AUDIT_DB_PATH=/var/lib/bambu-observer/audit.db
EOF

# env — only create from template if it doesn't exist yet (treat as conffile)
if [[ -f "${CONF_DIR}/env" ]]; then
  info "${CONF_DIR}/env already exists — not overwritten."
else
  info "Creating initial ${CONF_DIR}/env from template..."
  cp "${CONF_DIR}/env.example" "${CONF_DIR}/env"
  chmod 600 "${CONF_DIR}/env"
fi

# Systemd unit
info "Installing systemd unit to ${UNIT_PATH}..."
cat > "${UNIT_PATH}" <<'EOF'
[Unit]
Description=Bambu Observer — printer monitoring and filament tracking
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=bambu-observer
EnvironmentFile=/etc/bambu-observer/env
ExecStart=/usr/local/bin/bambu-observer
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now bambu-observer
info "Service enabled and started."

echo ""
info "Installation complete."
info "Edit ${CONF_DIR}/env with your printer credentials, then run:"
info "  systemctl restart bambu-observer"
