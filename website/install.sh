#!/usr/bin/env sh
# NetScope Agent — one-line installer
# Usage:
#   curl -sSL https://netscope.ie/install.sh | sudo sh
#   curl -sSL https://netscope.ie/install.sh | sudo HUB_URL=https://hub.example.com sh
#
# Supports: macOS (ARM64 / Intel), Linux (x86_64 / ARM64)
# Installs: /usr/local/bin/netscope-agent

set -e

REPO="Libinm264/netscope"
BINARY="netscope-agent"
INSTALL_DIR="/usr/local/bin"
VERSION="${VERSION:-latest}"

# ── Colours ────────────────────────────────────────────────────────────────────
if [ -t 1 ]; then
  BOLD="\033[1m"; CYAN="\033[36m"; GREEN="\033[32m"
  YELLOW="\033[33m"; RED="\033[31m"; RESET="\033[0m"
else
  BOLD=""; CYAN=""; GREEN=""; YELLOW=""; RED=""; RESET=""
fi

info()    { printf "${CYAN}  →${RESET}  %s\n" "$1"; }
success() { printf "${GREEN}  ✓${RESET}  %s\n" "$1"; }
warn()    { printf "${YELLOW}  ⚠${RESET}  %s\n" "$1"; }
fatal()   { printf "${RED}  ✗${RESET}  %s\n" "$1"; exit 1; }
banner()  { printf "\n${BOLD}${CYAN}NetScope Agent Installer${RESET}\n\n"; }

# ── Detect OS + arch ───────────────────────────────────────────────────────────
detect_platform() {
  OS="$(uname -s)"
  ARCH="$(uname -m)"

  case "$OS" in
    Darwin)
      case "$ARCH" in
        arm64)  PLATFORM="aarch64-apple-darwin";  EXT="tar.gz" ;;
        x86_64) PLATFORM="x86_64-apple-darwin";   EXT="tar.gz" ;;
        *) fatal "Unsupported macOS architecture: $ARCH" ;;
      esac
      ;;
    Linux)
      case "$ARCH" in
        x86_64)  PLATFORM="x86_64-unknown-linux-musl";  EXT="tar.gz" ;;
        aarch64) PLATFORM="aarch64-unknown-linux-musl"; EXT="tar.gz" ;;
        *) fatal "Unsupported Linux architecture: $ARCH" ;;
      esac
      ;;
    *)
      fatal "Unsupported OS: $OS. Windows users: download from https://netscope.ie/#download"
      ;;
  esac
}

# ── Resolve latest version tag ─────────────────────────────────────────────────
resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    info "Fetching latest release version..."
    VERSION="$(curl -sSfL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
    [ -n "$VERSION" ] || fatal "Could not determine latest version. Try: VERSION=v0.1.0 curl -sSL ... | sudo sh"
  fi
  success "Version: $VERSION"
}

# ── Download + install ─────────────────────────────────────────────────────────
download_and_install() {
  FILENAME="${BINARY}-${VERSION}-${PLATFORM}.${EXT}"
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

  info "Downloading ${FILENAME}..."

  TMP="$(mktemp -d)"
  trap 'rm -rf "$TMP"' EXIT

  if ! curl -sSfL "$URL" -o "${TMP}/${FILENAME}"; then
    fatal "Download failed. Check https://github.com/${REPO}/releases for available assets."
  fi

  info "Extracting..."
  cd "$TMP"
  case "$EXT" in
    tar.gz) tar -xzf "$FILENAME" ;;
    zip)    unzip -q "$FILENAME" ;;
  esac

  BINARY_PATH="$(find "$TMP" -name "$BINARY" -type f | head -1)"
  [ -n "$BINARY_PATH" ] || fatal "Binary not found in archive"

  chmod +x "$BINARY_PATH"

  info "Installing to ${INSTALL_DIR}/${BINARY}..."
  mv "$BINARY_PATH" "${INSTALL_DIR}/${BINARY}"

  success "Installed: $(${INSTALL_DIR}/${BINARY} --version 2>/dev/null || echo "netscope-agent ${VERSION}")"
}

# ── Optional: write config ──────────────────────────────────────────────────────
write_config() {
  if [ -n "$HUB_URL" ]; then
    CONFIG_DIR="${HOME}/.config/netscope"
    mkdir -p "$CONFIG_DIR"
    cat > "${CONFIG_DIR}/agent.env" <<EOF
HUB_URL=${HUB_URL}
HUB_API_KEY=${HUB_API_KEY:-}
EOF
    success "Config written: ${CONFIG_DIR}/agent.env"
  fi
}

# ── Systemd service (Linux only) ───────────────────────────────────────────────
install_service() {
  if [ "$OS" = "Linux" ] && [ -d /etc/systemd/system ] && [ -n "$HUB_URL" ]; then
    info "Installing systemd service..."
    cat > /etc/systemd/system/netscope-agent.service <<EOF
[Unit]
Description=NetScope Agent
After=network.target
Wants=network.target

[Service]
Type=simple
EnvironmentFile=-${HOME}/.config/netscope/agent.env
ExecStart=${INSTALL_DIR}/${BINARY} start
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable netscope-agent
    systemctl start netscope-agent
    success "Service installed and started (systemctl status netscope-agent)"
  fi
}

# ── Main ───────────────────────────────────────────────────────────────────────
banner
detect_platform
resolve_version
download_and_install
write_config
install_service

printf "\n${BOLD}Done!${RESET} Run: ${CYAN}netscope-agent list-interfaces${RESET}\n\n"

if [ -z "$HUB_URL" ]; then
  printf "${YELLOW}Tip:${RESET} Re-run with HUB_URL to connect to your hub:\n"
  printf "  ${CYAN}curl -sSL https://netscope.ie/install.sh | sudo HUB_URL=https://hub.example.com sh${RESET}\n\n"
fi
