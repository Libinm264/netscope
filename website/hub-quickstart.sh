#!/usr/bin/env sh
# NetScope Hub — one-line quickstart
# Usage:
#   curl -sSL https://netscope.ie/hub-quickstart.sh | sh
#   curl -sSL https://netscope.ie/hub-quickstart.sh | DOMAIN=hub.example.com sh
#
# What this does:
#   1. Checks Docker is installed
#   2. Downloads docker-compose.yml + .env.example
#   3. Generates a random API_KEY
#   4. Starts the full stack (ClickHouse + Kafka + API + Dashboard + Caddy)
#   5. Prints the access URL

set -e

REPO="Libinm264/netscope"
RAW="https://raw.githubusercontent.com/${REPO}/main/hub"
INSTALL_DIR="${NETSCOPE_DIR:-$HOME/.netscope-hub}"
DOMAIN="${DOMAIN:-}"

# ── Colours ───────────────────────────────────────────────────────────────────
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

banner() {
  printf "\n${BOLD}${CYAN}"
  printf "  ███╗   ██╗███████╗████████╗███████╗ ██████╗ ██████╗ ██████╗ ███████╗\n"
  printf "  ████╗  ██║██╔════╝╚══██╔══╝██╔════╝██╔════╝██╔═══██╗██╔══██╗██╔════╝\n"
  printf "  ██╔██╗ ██║█████╗     ██║   ███████╗██║     ██║   ██║██████╔╝█████╗  \n"
  printf "  ██║╚██╗██║██╔══╝     ██║   ╚════██║██║     ██║   ██║██╔═══╝ ██╔══╝  \n"
  printf "  ██║ ╚████║███████╗   ██║   ███████║╚██████╗╚██████╔╝██║     ███████╗\n"
  printf "  ╚═╝  ╚═══╝╚══════╝   ╚═╝   ╚══════╝ ╚═════╝ ╚═════╝ ╚═╝     ╚══════╝\n"
  printf "${RESET}${BOLD}  Hub Quickstart${RESET}\n\n"
}

# ── Checks ────────────────────────────────────────────────────────────────────
check_docker() {
  command -v docker >/dev/null 2>&1 || fatal "Docker is not installed. Get it from https://docs.docker.com/get-docker/"
  docker info >/dev/null 2>&1      || fatal "Docker daemon is not running. Start Docker and try again."
  command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1 || \
    fatal "Docker Compose V2 not found. Update Docker Desktop or install the compose plugin."
  success "Docker $(docker --version | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')"
}

# ── Download files ────────────────────────────────────────────────────────────
download_files() {
  mkdir -p "$INSTALL_DIR/caddy"
  info "Downloading compose files to ${INSTALL_DIR}..."

  curl -sSfL "${RAW}/docker-compose.yml"      -o "${INSTALL_DIR}/docker-compose.yml"
  curl -sSfL "${RAW}/.env.example"            -o "${INSTALL_DIR}/.env.example"
  curl -sSfL "${RAW}/caddy/Caddyfile"         -o "${INSTALL_DIR}/caddy/Caddyfile"

  success "Files downloaded"
}

# ── Generate .env ─────────────────────────────────────────────────────────────
setup_env() {
  ENV_FILE="${INSTALL_DIR}/.env"

  if [ -f "$ENV_FILE" ]; then
    warn ".env already exists — skipping generation (delete it to reset)"
    return
  fi

  # Generate a cryptographically random 32-byte API key
  if command -v openssl >/dev/null 2>&1; then
    API_KEY="$(openssl rand -hex 32)"
  else
    API_KEY="$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | head -c 64)"
  fi

  cp "${INSTALL_DIR}/.env.example" "$ENV_FILE"

  # Replace placeholders
  sed -i.bak "s|API_KEY=changeme|API_KEY=${API_KEY}|g" "$ENV_FILE"
  sed -i.bak "s|NEXT_PUBLIC_API_KEY=changeme|NEXT_PUBLIC_API_KEY=${API_KEY}|g" "$ENV_FILE"
  rm -f "${ENV_FILE}.bak"

  if [ -n "$DOMAIN" ]; then
    sed -i.bak "s|CADDY_HOSTNAME=:80|CADDY_HOSTNAME=${DOMAIN}|g" "$ENV_FILE"
    sed -i.bak "s|APP_URL=http://localhost|APP_URL=https://${DOMAIN}|g" "$ENV_FILE"
    sed -i.bak "s|AUTH0_BASE_URL=http://localhost|AUTH0_BASE_URL=https://${DOMAIN}|g" "$ENV_FILE"
    rm -f "${ENV_FILE}.bak"
    success "Domain: ${DOMAIN}"
  fi

  success "API key generated (stored in ${ENV_FILE})"
}

# ── Pull images ───────────────────────────────────────────────────────────────
pull_images() {
  info "Pulling latest Docker images (this may take a minute)..."
  cd "$INSTALL_DIR"
  docker compose pull --quiet
  success "Images ready"
}

# ── Start stack ───────────────────────────────────────────────────────────────
start_stack() {
  info "Starting NetScope Hub..."
  cd "$INSTALL_DIR"
  docker compose up -d
  success "Stack started"
}

# ── Wait for healthy ──────────────────────────────────────────────────────────
wait_healthy() {
  info "Waiting for API to be ready..."
  i=0
  while [ $i -lt 30 ]; do
    if curl -sf http://localhost:8080/health >/dev/null 2>&1; then
      success "API is healthy"
      return
    fi
    i=$((i + 1))
    sleep 2
  done
  warn "API not yet healthy — check logs: docker compose -C ${INSTALL_DIR} logs api"
}

# ── Done ──────────────────────────────────────────────────────────────────────
print_summary() {
  API_KEY_VAL="$(grep '^API_KEY=' "${INSTALL_DIR}/.env" | cut -d= -f2)"
  URL="http://localhost"
  [ -n "$DOMAIN" ] && URL="https://${DOMAIN}"

  printf "\n${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}\n"
  printf "${BOLD}  NetScope Hub is running!${RESET}\n"
  printf "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}\n\n"
  printf "  Dashboard  ${CYAN}${URL}${RESET}\n"
  printf "  API        ${CYAN}http://localhost:8080${RESET}\n"
  printf "  API Key    ${YELLOW}${API_KEY_VAL}${RESET}\n\n"
  printf "  Manage:    cd ${INSTALL_DIR} && docker compose ...\n\n"
  printf "  Connect an agent:\n"
  printf "  ${CYAN}curl -sSL https://netscope.ie/install.sh | sudo HUB_URL=${URL} HUB_API_KEY=${API_KEY_VAL} sh${RESET}\n\n"
  printf "  Stop:      ${YELLOW}docker compose -p netscope-hub down${RESET}\n"
  printf "  Logs:      ${YELLOW}docker compose -p netscope-hub logs -f${RESET}\n\n"
}

# ── Main ──────────────────────────────────────────────────────────────────────
banner
check_docker
download_files
setup_env
pull_images
start_stack
wait_healthy
print_summary
