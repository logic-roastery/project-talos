#!/usr/bin/env bash
#
# Talos Installer & Upgrader
# Usage: sudo bash install.sh [--docker] [--from-source] [--port PORT]
#        sudo bash install.sh --upgrade [--docker] [--version-tag vX.Y.Z]
#
# Installs Talos, Docker (if missing), and Traefik on a Linux host.
# Target: Ubuntu/Debian/Fedora
#
# Modes:
#   (default)     Bare binary + systemd service
#   --docker      Docker container (easier upgrades, isolated)
#   --upgrade     Upgrade to latest (or specific) version, preserving config and data
#
set -euo pipefail

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

TALOS_USER="talos"
TALOS_HOME="/opt/talos"
TALOS_BIN="/usr/local/bin/talos"
TALOS_ENV="${TALOS_HOME}/.env"
TALOS_DATA="${TALOS_HOME}/data"
TRAEFIK_CONTAINER="talos-traefik"
DOCKER_NETWORK="talos"
TRAEFIK_IMAGE="traefik:v3.0"
SYSTEMD_UNIT="/etc/systemd/system/talos.service"
REPO_URL="https://github.com/logic-roastery/project-talos"
GHCR_IMAGE="ghcr.io/logic-roastery/project-talos:latest"
GHCR_IMAGE_BASE="ghcr.io/logic-roastery/project-talos"

# Defaults
FROM_SOURCE=false
DOCKER_MODE=false
UPGRADE_MODE=false
TARGET_VERSION=""
TALOS_PORT=3000
DOCKER_GROUP="docker"
TALOS_PROXY_MODE="internal"
TALOS_EDGE_NETWORK="traefik-public"
TALOS_EDGE_CERT_RESOLVER="letsencrypt"
TALOS_DOMAIN=""

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[info]${NC}  $*"; }
ok()    { echo -e "${GREEN}[ok]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC}  $*"; }
die()   { echo -e "${RED}[error]${NC} $*" >&2; exit 1; }

detect_host_ip() {
    local ip=""

    if command -v curl &>/dev/null; then
        ip=$(curl -fsS --max-time 3 https://api.ipify.org 2>/dev/null || true)
    fi

    if [[ -z "${ip}" ]] && command -v ip &>/dev/null; then
        ip=$(ip route get 1.1.1.1 2>/dev/null | awk '/src/ {for (i = 1; i <= NF; i++) if ($i == "src") {print $(i+1); exit}}')
    fi

    if [[ -z "${ip}" ]] && command -v hostname &>/dev/null; then
        ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    fi

    printf '%s' "${ip:-<your-server-ip>}"
}

load_existing_proxy_settings() {
    if [[ -f "${TALOS_ENV}" ]]; then
        local existing_proxy_mode existing_edge_network existing_edge_cert_resolver existing_port existing_domain
        existing_proxy_mode=$(grep "^TALOS_PROXY_MODE=" "${TALOS_ENV}" 2>/dev/null | cut -d= -f2-)
        existing_edge_network=$(grep "^TALOS_EDGE_NETWORK=" "${TALOS_ENV}" 2>/dev/null | cut -d= -f2-)
        existing_edge_cert_resolver=$(grep "^TALOS_EDGE_CERT_RESOLVER=" "${TALOS_ENV}" 2>/dev/null | cut -d= -f2-)
        existing_port=$(grep "^TALOS_PORT=" "${TALOS_ENV}" 2>/dev/null | cut -d= -f2-)
        existing_domain=$(grep "^TALOS_DOMAIN=" "${TALOS_ENV}" 2>/dev/null | cut -d= -f2-)

        if [[ -n "${existing_proxy_mode}" ]]; then
            TALOS_PROXY_MODE="${existing_proxy_mode}"
        fi
        if [[ -n "${existing_edge_network}" ]]; then
            TALOS_EDGE_NETWORK="${existing_edge_network}"
        fi
        if [[ -n "${existing_edge_cert_resolver}" ]]; then
            TALOS_EDGE_CERT_RESOLVER="${existing_edge_cert_resolver}"
        fi
        if [[ -n "${existing_port}" ]]; then
            TALOS_PORT="${existing_port}"
        fi
        if [[ -n "${existing_domain}" ]]; then
            TALOS_DOMAIN="${existing_domain}"
        fi
    fi
}

ensure_edge_network() {
    if [[ "${TALOS_PROXY_MODE}" == "external" ]]; then
        docker network create "${TALOS_EDGE_NETWORK}" >/dev/null 2>&1 || true
    fi
}

connect_talos_edge_network() {
    if [[ "${TALOS_PROXY_MODE}" == "external" ]] && [[ "${TALOS_EDGE_NETWORK}" != "${DOCKER_NETWORK}" ]]; then
        docker network connect "${TALOS_EDGE_NETWORK}" talos >/dev/null 2>&1 || true
    fi
}

talos_external_label_args() {
    if [[ "${TALOS_PROXY_MODE}" != "external" ]] || [[ -z "${TALOS_DOMAIN}" ]]; then
        return 0
    fi

    printf '%s\n' \
        "--label" "traefik.enable=true" \
        "--label" "traefik.docker.network=${TALOS_EDGE_NETWORK}" \
        "--label" "traefik.http.routers.talos.rule=Host(\`${TALOS_DOMAIN}\`)" \
        "--label" "traefik.http.routers.talos.entrypoints=websecure" \
        "--label" "traefik.http.routers.talos.tls=true" \
        "--label" "traefik.http.routers.talos.tls.certresolver=${TALOS_EDGE_CERT_RESOLVER}" \
        "--label" "traefik.http.services.talos.loadbalancer.server.port=3000"
}

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------

while [[ $# -gt 0 ]]; do
    case "$1" in
        --docker)       DOCKER_MODE=true; shift ;;
        --from-source)  FROM_SOURCE=true; shift ;;
        --port)         TALOS_PORT="$2"; shift 2 ;;
        --upgrade)      UPGRADE_MODE=true; shift ;;
        --version-tag)  TARGET_VERSION="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: sudo bash install.sh [OPTIONS]"
            echo "       sudo bash install.sh --upgrade [--docker] [--version-tag vX.Y.Z]"
            echo ""
            echo "Install Options:"
            echo "  --docker          Install as a Docker container (easier upgrades)"
            echo "  --from-source     Build Talos from source (requires Go 1.21+)"
            echo "  --port PORT       Port for the Talos web UI (default: 3000)"
            echo ""
            echo "Upgrade Options:"
            echo "  --upgrade         Upgrade Talos to the latest version (preserves config & data)"
            echo "  --version-tag X   Upgrade to a specific version (e.g. v0.2.0)"
            exit 0
            ;;
        *) die "Unknown option: $1" ;;
    esac
done

# ---------------------------------------------------------------------------
# Upgrade mode
# ---------------------------------------------------------------------------

if [[ "${UPGRADE_MODE}" == "true" ]]; then

    [[ $(id -u) -eq 0 ]] || die "This script must be run as root (use sudo)."

    # --- Detect installation mode ---
    detect_install_mode() {
        if [[ "${DOCKER_MODE}" == "true" ]]; then
            echo "docker"
        elif [[ -f "${SYSTEMD_UNIT}" ]] && [[ -x "${TALOS_BIN}" ]]; then
            echo "bare"
        elif docker inspect talos &>/dev/null 2>&1; then
            echo "docker"
        else
            die "Talos is not installed. Run install.sh first."
        fi
    }

    INSTALL_MODE=$(detect_install_mode)
    BACKUP_TAG=$(date +%Y%m%d-%H%M%S)

    # --- Resolve target version string ---
    if [[ -z "${TARGET_VERSION}" ]]; then
        info "Checking latest release..."
        # Try /releases/latest first (works when stable releases exist)
        TARGET_VERSION=$(curl -fsSL "https://api.github.com/repos/logic-roastery/project-talos/releases/latest" 2>/dev/null \
            | grep '"tag_name"' | sed 's/.*"tag_name": *"\(.*\)".*/\1/' || true)
        # Fallback: /releases/latest returns 404 when all releases are prereleases.
        # List all releases and pick the newest one.
        if [[ -z "${TARGET_VERSION}" ]]; then
            TARGET_VERSION=$(curl -fsSL "https://api.github.com/repos/logic-roastery/project-talos/releases" \
                | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/' || true)
        fi
        if [[ -z "${TARGET_VERSION}" ]]; then
            die "Could not determine latest release. Check your internet connection or use --version-tag."
        fi
    fi

    # =========================================================================
    # Bare binary upgrade
    # =========================================================================

    rollback_bare() {
        warn "Upgrade failed. Rolling back..."
        local latest_bak
        latest_bak=$(ls -t "${TALOS_BIN}.bak."* 2>/dev/null | head -1)
        if [[ -n "$latest_bak" ]]; then
            cp "$latest_bak" "${TALOS_BIN}"
            systemctl start talos 2>/dev/null || true
            ok "Rolled back to previous binary: ${latest_bak}"
        else
            warn "No backup found. Manual intervention required."
        fi
        die "Upgrade failed."
    }

    if [[ "${INSTALL_MODE}" == "bare" ]]; then
        info "Upgrading Talos (bare binary mode)..."

        # Get current version
        CURRENT_VERSION=$("${TALOS_BIN}" --version 2>/dev/null | awk '{print $2}' || echo "unknown")
        info "Current version: ${CURRENT_VERSION}"
        info "Target version:  ${TARGET_VERSION}"

        if [[ "${CURRENT_VERSION}" == "${TARGET_VERSION}" ]]; then
            ok "Talos is already at ${TARGET_VERSION}. Nothing to do."
            exit 0
        fi

        # Ensure .env exists (never overwrite it)
        [[ -f "${TALOS_ENV}" ]] || die "No .env found at ${TALOS_ENV}. Run install.sh first."

        # Backup current binary
        cp "${TALOS_BIN}" "${TALOS_BIN}.bak.${BACKUP_TAG}"
        ok "Backed up current binary to ${TALOS_BIN}.bak.${BACKUP_TAG}"

        # Stop service
        info "Stopping Talos service..."
        systemctl stop talos || true

        # Download new binary (trap on failure)
        trap rollback_bare ERR
        resolve_arch
        DOWNLOAD_URL="${REPO_URL}/releases/download/${TARGET_VERSION}/talos-linux-${ARCH}"
        if curl -fsSL --head "${DOWNLOAD_URL}" &>/dev/null; then
            info "Downloading ${TARGET_VERSION}..."
            curl -fsSL "${DOWNLOAD_URL}" -o "${TALOS_BIN}"
            chmod 755 "${TALOS_BIN}"
            ok "Binary downloaded."
        else
            warn "No pre-built binary for ${TARGET_VERSION}. Building from source..."
            build_from_source
        fi

        # Start service
        info "Starting Talos service..."
        systemctl start talos
        trap - ERR

        # Verify
        sleep 3
        if systemctl is-active --quiet talos; then
            NEW_VERSION=$("${TALOS_BIN}" --version 2>/dev/null | awk '{print $2}' || echo "unknown")
            echo ""
            echo "============================================="
            echo -e "${GREEN}  Talos upgraded: ${CURRENT_VERSION} → ${NEW_VERSION}${NC}"
            echo "============================================="
            echo ""
            echo "  Rollback to previous version:"
            echo ""
            echo "    sudo systemctl stop talos"
            echo "    sudo cp ${TALOS_BIN}.bak.${BACKUP_TAG} ${TALOS_BIN}"
            echo "    sudo systemctl start talos"
            echo ""
            echo "============================================="
        else
            rollback_bare
        fi

        exit 0
    fi

    # =========================================================================
    # Docker mode upgrade
    # =========================================================================

    rollback_docker() {
        warn "Upgrade failed. Rolling back..."
        docker stop talos >/dev/null 2>&1 || true
        docker rm talos >/dev/null 2>&1 || true
        local latest_tag
        latest_tag=$(docker images "${GHCR_IMAGE_BASE}" --format '{{.Tag}}' | grep '^rollback-' | sort -r | head -1)
        if [[ -n "$latest_tag" ]]; then
            load_existing_proxy_settings
            mapfile -t TALOS_LABEL_ARGS < <(talos_external_label_args)
            ensure_edge_network
            docker run -d \
                --name talos \
                --restart unless-stopped \
                --network "${DOCKER_NETWORK}" \
                -p "${TALOS_PORT}:3000" \
                -v /var/run/docker.sock:/var/run/docker.sock \
                -v "${TALOS_DATA}:/data" \
                -v "${TALOS_ENV}:${TALOS_ENV}" \
                --env-file "${TALOS_ENV}" \
                "${TALOS_LABEL_ARGS[@]}" \
                "${GHCR_IMAGE_BASE}:${latest_tag}" >/dev/null
            connect_talos_edge_network
            ok "Rolled back to image tag: ${latest_tag}"
        else
            warn "No rollback image found. Manual intervention required."
        fi
        die "Upgrade failed."
    }

    if [[ "${INSTALL_MODE}" == "docker" ]]; then
        info "Upgrading Talos (Docker mode)..."

        # Get current image info
        CURRENT_IMAGE_ID=$(docker inspect --format='{{.Image}}' talos 2>/dev/null || echo "unknown")
        CURRENT_IMAGE_TAG=$(docker inspect --format='{{.Config.Image}}' talos 2>/dev/null || echo "unknown")
        info "Current image: ${CURRENT_IMAGE_TAG}"

        # Resolve image tag: use :latest when no version specified (matches fresh install),
        # or a specific versioned tag when --version-tag is provided.
        if [[ -n "${TARGET_VERSION}" ]]; then
            IMAGE_TAG="${TARGET_VERSION#v}"
        else
            IMAGE_TAG="latest"
        fi

        # Ensure .env exists (never overwrite it)
        [[ -f "${TALOS_ENV}" ]] || die "No .env found at ${TALOS_ENV}. Run install.sh first."
        load_existing_proxy_settings

        # Pull new image
        info "Pulling ${GHCR_IMAGE_BASE}:${IMAGE_TAG}..."
        docker pull "${GHCR_IMAGE_BASE}:${IMAGE_TAG}"

        # Compare image IDs
        NEW_IMAGE_ID=$(docker inspect --format='{{.Id}}' "${GHCR_IMAGE_BASE}:${IMAGE_TAG}" 2>/dev/null || echo "")
        if [[ "${CURRENT_IMAGE_ID}" == "${NEW_IMAGE_ID}" ]]; then
            ok "Talos Docker image is already up to date. Nothing to do."
            exit 0
        fi

        # Tag current image for rollback (use image ID, not :latest — container may use a versioned tag)
        if [[ -n "${CURRENT_IMAGE_ID}" && "${CURRENT_IMAGE_ID}" != "unknown" ]]; then
            docker tag "${CURRENT_IMAGE_ID}" "${GHCR_IMAGE_BASE}:rollback-${BACKUP_TAG}" 2>/dev/null || true
            ok "Tagged current image as ${GHCR_IMAGE_BASE}:rollback-${BACKUP_TAG}"
        fi

        # Recreate container (stop → rm → run)
        trap rollback_docker ERR
        info "Recreating Talos container..."
        docker stop talos >/dev/null 2>&1 || true
        docker rm talos >/dev/null 2>&1 || true

        mapfile -t TALOS_LABEL_ARGS < <(talos_external_label_args)
        ensure_edge_network
        docker run -d \
            --name talos \
            --restart unless-stopped \
            --network "${DOCKER_NETWORK}" \
            -p "${TALOS_PORT}:3000" \
            -v /var/run/docker.sock:/var/run/docker.sock \
            -v "${TALOS_DATA}:/data" \
            -v "${TALOS_ENV}:${TALOS_ENV}" \
            --env-file "${TALOS_ENV}" \
            "${TALOS_LABEL_ARGS[@]}" \
            "${GHCR_IMAGE_BASE}:${IMAGE_TAG}" \
            >/dev/null
        connect_talos_edge_network

        trap - ERR

        # Verify
        sleep 3
        if docker inspect -f '{{.State.Running}}' talos 2>/dev/null | grep -q true; then
            echo ""
            echo "============================================="
            echo -e "${GREEN}  Talos upgraded to ${IMAGE_TAG}${NC}"
            echo "============================================="
            echo ""
            echo "  Rollback to previous version:"
            echo ""
            echo "    sudo docker stop talos && sudo docker rm talos"
            echo "    sudo docker run -d \\"
            echo "      --name talos \\"
            echo "      --restart unless-stopped \\"
            echo "      --network ${DOCKER_NETWORK} \\"
            echo "      -p ${TALOS_PORT}:3000 \\"
            echo "      -v /var/run/docker.sock:/var/run/docker.sock \\"
            echo "      -v ${TALOS_DATA}:/data \\"
            echo "      -v ${TALOS_ENV}:${TALOS_ENV} \\"
            echo "      --env-file ${TALOS_ENV} \\"
            echo "      ${GHCR_IMAGE_BASE}:rollback-${BACKUP_TAG}"
            echo ""
            echo "============================================="
        else
            rollback_docker
        fi

        exit 0
    fi
fi

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------

[[ $(id -u) -eq 0 ]] || die "This script must be run as root (use sudo)."

info "Detecting host OS..."
if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    OS_ID="${ID:-unknown}"
    OS_VERSION="${VERSION_ID:-unknown}"
else
    die "Cannot detect OS. /etc/os-release not found."
fi

case "$OS_ID" in
    ubuntu|debian|fedora)
        ok "Detected: ${OS_ID} ${OS_VERSION}"
        ;;
    *)
        warn "Untested OS: ${OS_ID}. Proceeding anyway (Ubuntu/Debian/Fedora expected)."
        ;;
esac

# ---------------------------------------------------------------------------
# Step 1: Docker
# ---------------------------------------------------------------------------

install_docker() {
    info "Installing Docker..."
    case "$OS_ID" in
        ubuntu|debian)
            apt-get update -qq
            apt-get install -y -qq ca-certificates curl gnupg >/dev/null
            install -m 0755 -d /etc/apt/keyrings
            curl -fsSL "https://download.docker.com/linux/${OS_ID}/gpg" \
                | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
            chmod a+r /etc/apt/keyrings/docker.gpg
            echo \
                "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
                https://download.docker.com/linux/${OS_ID} ${VERSION_CODENAME} stable" \
                > /etc/apt/sources.list.d/docker.list
            apt-get update -qq
            apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-compose-plugin >/dev/null
            ;;
        fedora)
            dnf install -y -q dnf-plugins-core
            dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
            dnf install -y -q docker-ce docker-ce-cli containerd.io docker-compose-plugin
            ;;
        *)
            die "Automatic Docker installation not supported for ${OS_ID}. Install Docker manually and re-run."
            ;;
    esac
    systemctl enable --now docker
    ok "Docker installed and started."
}

if command -v docker &>/dev/null; then
    DOCKER_VERSION=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo "unknown")
    ok "Docker already installed: ${DOCKER_VERSION}"
else
    warn "Docker not found."
    install_docker
fi

# ---------------------------------------------------------------------------
# Step 2: Docker permissions
# ---------------------------------------------------------------------------

info "Checking Docker socket permissions..."
if [[ -S /var/run/docker.sock ]]; then
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock 2>/dev/null || stat -f '%g' /var/run/docker.sock)
    DOCKER_GROUP=$(getent group "$DOCKER_GID" 2>/dev/null | cut -d: -f1 || echo "")
    if [[ -z "$DOCKER_GROUP" ]]; then
        DOCKER_GROUP="docker"
        groupadd -f "$DOCKER_GROUP"
        chgrp "$DOCKER_GROUP" /var/run/docker.sock
    fi
    ok "Docker socket accessible (group: ${DOCKER_GROUP}, gid: ${DOCKER_GID})"
else
    die "Docker socket not found at /var/run/docker.sock. Is Docker running?"
fi

# Verify we can actually talk to Docker
if ! docker info &>/dev/null; then
    die "Cannot communicate with Docker daemon. Check permissions on /var/run/docker.sock."
fi
ok "Docker daemon is responsive."

# ---------------------------------------------------------------------------
# Step 3: Docker network
# ---------------------------------------------------------------------------

if docker network inspect "${DOCKER_NETWORK}" &>/dev/null; then
    ok "Docker network '${DOCKER_NETWORK}' already exists."
else
    docker network create "${DOCKER_NETWORK}"
    ok "Created Docker network '${DOCKER_NETWORK}'."
fi

# ---------------------------------------------------------------------------
# Step 4: Talos system user and directories
# ---------------------------------------------------------------------------

if id "${TALOS_USER}" &>/dev/null; then
    ok "System user '${TALOS_USER}' already exists."
else
    useradd --system --no-create-home --shell /usr/sbin/nologin \
        --groups "${DOCKER_GROUP:-docker}" "${TALOS_USER}"
    ok "Created system user '${TALOS_USER}' (added to docker group)."
fi

info "Creating directory structure..."
mkdir -p "${TALOS_DATA}/traefik/config"
mkdir -p "${TALOS_DATA}/traefik/data"
chown -R "${TALOS_USER}:${TALOS_USER}" "${TALOS_HOME}"
ok "Directories created at ${TALOS_HOME}."

# ---------------------------------------------------------------------------
# Step 5: Session secret
# ---------------------------------------------------------------------------

SESSION_SECRET=""
if [[ -f "${TALOS_ENV}" ]] && grep -q "TALOS_SESSION_SECRET" "${TALOS_ENV}" 2>/dev/null; then
    SESSION_SECRET=$(grep "^TALOS_SESSION_SECRET=" "${TALOS_ENV}" | cut -d= -f2-)
    ok "Session secret already configured."
else
    SESSION_SECRET=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 40)
    info "Generated new session secret."
fi

# ---------------------------------------------------------------------------
# Step 6: Domain configuration
# ---------------------------------------------------------------------------

TALOS_DOMAIN=""
TALOS_ACME_EMAIL=""

load_existing_proxy_settings

if [[ -f "${TALOS_ENV}" ]] && grep -q "TALOS_DOMAIN=" "${TALOS_ENV}" 2>/dev/null; then
    existing_domain=$(grep "^TALOS_DOMAIN=" "${TALOS_ENV}" | cut -d= -f2-)
    existing_email=$(grep "^TALOS_ACME_EMAIL=" "${TALOS_ENV}" | cut -d= -f2-)
    if [[ -n "$existing_domain" ]]; then
        TALOS_DOMAIN="$existing_domain"
        TALOS_ACME_EMAIL="$existing_email"
        ok "Domain already configured: ${TALOS_DOMAIN}"
    fi
fi

if [[ -z "$TALOS_DOMAIN" ]]; then
    echo ""
    read -rp "Do you have a domain name pointed at this server? [y/N] " has_domain
    if [[ "${has_domain,,}" == "y" ]]; then
        read -rp "Enter your domain (e.g. talos.example.com): " TALOS_DOMAIN
        read -rp "Enter your email for TLS certificate notifications: " TALOS_ACME_EMAIL
        read -rp "Will another reverse proxy on this server own ports 80/443? [y/N] " uses_external_proxy
        if [[ "${uses_external_proxy,,}" == "y" ]]; then
            TALOS_PROXY_MODE="external"
            read -rp "Enter the shared external proxy Docker network [traefik-public]: " edge_network
            read -rp "Enter the external proxy cert resolver name [letsencrypt]: " edge_cert_resolver
            TALOS_EDGE_NETWORK="${edge_network:-traefik-public}"
            TALOS_EDGE_CERT_RESOLVER="${edge_cert_resolver:-letsencrypt}"
        else
            TALOS_PROXY_MODE="internal"
        fi
        ok "Domain: ${TALOS_DOMAIN}"
    else
        ACCESS_HOST=$(detect_host_ip)
        info "No domain — Talos will be accessible at http://${ACCESS_HOST}:${TALOS_PORT}"
    fi
fi

ACCESS_HOST="${TALOS_DOMAIN:-$(detect_host_ip)}"
if [[ -n "${TALOS_DOMAIN}" ]]; then
    ACCESS_URL="https://${TALOS_DOMAIN}"
else
    ACCESS_URL="http://${ACCESS_HOST}:${TALOS_PORT}"
fi

# ---------------------------------------------------------------------------
# Step 6: Talos binary
# ---------------------------------------------------------------------------

build_from_source() {
    if ! command -v go &>/dev/null; then
        die "Go is not installed. Install Go 1.21+ or omit --from-source to download a pre-built binary."
    fi
    GO_VERSION=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+')
    info "Building Talos from source (Go ${GO_VERSION})..."

    local build_version build_commit
    build_version=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
    build_commit=$(git rev-parse --short HEAD 2>/dev/null || echo "none")

    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    if [[ -d "${SCRIPT_DIR}/../cmd" ]]; then
        # Running from the repo checkout
        REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
        info "Building from local source at ${REPO_ROOT}"
        cd "${REPO_ROOT}"
        CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${build_version} -X main.commit=${build_commit}" -o "${TALOS_BIN}" ./cmd/talos
    else
        BUILD_DIR=$(mktemp -d)
        git clone --depth 1 "${REPO_URL}" "${BUILD_DIR}/project-talos"
        cd "${BUILD_DIR}/project-talos"
        CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${build_version} -X main.commit=${build_commit}" -o "${TALOS_BIN}" ./cmd/talos
        rm -rf "${BUILD_DIR}"
    fi
    chmod 755 "${TALOS_BIN}"
    ok "Talos binary built and installed to ${TALOS_BIN}."
}

resolve_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        *)       die "Unsupported architecture: ${ARCH}" ;;
    esac
}

download_binary() {
    local ver="${1:-}"
    info "Downloading Talos binary..."
    resolve_arch

    if [[ -n "$ver" ]]; then
        DOWNLOAD_URL="${REPO_URL}/releases/download/${ver}/talos-linux-${ARCH}"
    else
        DOWNLOAD_URL="${REPO_URL}/releases/latest/download/talos-linux-${ARCH}"
    fi

    if curl -fsSL --head "${DOWNLOAD_URL}" &>/dev/null; then
        curl -fsSL "${DOWNLOAD_URL}" -o "${TALOS_BIN}"
        chmod 755 "${TALOS_BIN}"
        ok "Talos binary downloaded to ${TALOS_BIN}."
    else
        warn "No pre-built binary available. Falling back to building from source."
        build_from_source
    fi
}

if [[ "${DOCKER_MODE}" != "true" ]]; then
    if [[ -x "${TALOS_BIN}" ]]; then
        ok "Talos binary already exists at ${TALOS_BIN}."
        if [[ "${FROM_SOURCE}" == "true" ]]; then
            info "Rebuilding from source (--from-source flag)..."
            build_from_source
        fi
    else
        if [[ "${FROM_SOURCE}" == "true" ]]; then
            build_from_source
        else
            download_binary
        fi
    fi
fi

# ---------------------------------------------------------------------------
# Docker mode: pull image, write env, run container, skip systemd
# ---------------------------------------------------------------------------

if [[ "${DOCKER_MODE}" == "true" ]]; then
    info "Installing Talos in Docker mode..."

    # Pull the image
    info "Pulling ${GHCR_IMAGE}..."
    docker pull "${GHCR_IMAGE}"
    ok "Docker image pulled."

    # Write environment file (same as bare-metal)
    info "Writing environment file..."
    cat > "${TALOS_ENV}" <<EOF
# Talos Configuration
# Generated by installer on $(date -u '+%Y-%m-%d %H:%M:%S UTC')

# Server
TALOS_HOST=0.0.0.0
TALOS_PORT=${TALOS_PORT}

# Domain
TALOS_DOMAIN=${TALOS_DOMAIN}
TALOS_ACME_EMAIL=${TALOS_ACME_EMAIL}
TALOS_PROXY_MODE=${TALOS_PROXY_MODE}
TALOS_EDGE_NETWORK=${TALOS_EDGE_NETWORK}
TALOS_EDGE_CERT_RESOLVER=${TALOS_EDGE_CERT_RESOLVER}

# Database
TALOS_DB_PATH=/data/talos.db

# Auth
TALOS_SESSION_SECRET=${SESSION_SECRET}
TALOS_SESSION_MAX_AGE=604800

# Docker
TALOS_DOCKER_HOST=unix:///var/run/docker.sock
TALOS_DOCKER_NETWORK=${DOCKER_NETWORK}


# Host data root for Docker volume mount translation
# When running in Docker, the binary needs host paths for sibling container mounts
TALOS_HOST_DATA_ROOT=${TALOS_DATA}

# Traefik
TALOS_TRAEFIK_IMAGE=${TRAEFIK_IMAGE}
TALOS_TRAEFIK_CONFIG_DIR=/data/traefik/config
TALOS_TRAEFIK_DATA_DIR=/data/traefik/data
TALOS_TRAEFIK_DASHBOARD=false

# GitHub App (optional — configure after installation)
# TALOS_GITHUB_WEBHOOK_SECRET=
# TALOS_GITHUB_APP_ID=
# TALOS_GITHUB_APP_SLUG=
# TALOS_GITHUB_APP_PRIVATE_KEY=
# TALOS_GITHUB_APP_CLIENT_ID=
# TALOS_GITHUB_APP_CLIENT_SECRET=
EOF
    chmod 600 "${TALOS_ENV}"
    ok "Environment file written to ${TALOS_ENV}."

    # Stop existing container if present
    if docker inspect talos &>/dev/null 2>&1; then
        info "Stopping existing Talos container..."
        docker stop talos >/dev/null 2>&1 || true
        docker rm talos >/dev/null 2>&1 || true
    fi

    mapfile -t TALOS_LABEL_ARGS < <(talos_external_label_args)
    ensure_edge_network

    # Run Talos container
    docker run -d \
        --name talos \
        --restart unless-stopped \
        --network "${DOCKER_NETWORK}" \
        -p "${TALOS_PORT}:3000" \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v "${TALOS_DATA}:/data" \
        -v "${TALOS_ENV}:${TALOS_ENV}" \
        --env-file "${TALOS_ENV}" \
        "${TALOS_LABEL_ARGS[@]}" \
        "${GHCR_IMAGE}" \
        >/dev/null

    connect_talos_edge_network

    ok "Talos container started."

    # Verification
    info "Running verification checks..."
    CHECKS_PASSED=0
    CHECKS_TOTAL=3

    if docker info &>/dev/null; then
        ok "  [1/3] Docker daemon is running."
        CHECKS_PASSED=$((CHECKS_PASSED + 1))
    else
        warn "  [1/3] Docker daemon is not running."
    fi

    if docker inspect -f '{{.State.Running}}' talos 2>/dev/null | grep -q true; then
        ok "  [2/3] Talos container is running."
        CHECKS_PASSED=$((CHECKS_PASSED + 1))
    else
        warn "  [2/3] Talos container is not running."
    fi

    if [[ -f "${TALOS_ENV}" ]]; then
        ok "  [3/3] Environment file exists."
        CHECKS_PASSED=$((CHECKS_PASSED + 1))
    else
        warn "  [3/3] Environment file not found."
    fi

    echo ""
    echo "============================================="
    if [[ ${CHECKS_PASSED} -eq ${CHECKS_TOTAL} ]]; then
        echo -e "${GREEN}  Talos installed successfully (Docker mode)!${NC}"
    else
        echo -e "${YELLOW}  Talos installed with warnings (${CHECKS_PASSED}/${CHECKS_TOTAL} checks passed).${NC}"
    fi
    echo "============================================="
    echo ""
    echo "  Config:         ${TALOS_ENV}"
    echo "  Data:           ${TALOS_DATA}"
    echo "  Web UI port:    ${TALOS_PORT}"
    echo ""
    echo "  View logs:      docker logs -f talos"
    echo "  Stop:           docker stop talos"
    echo "  Start:          docker start talos"
    echo "  Upgrade:        sudo bash install.sh --upgrade --docker"
    echo ""
    echo "  Open in browser: ${ACCESS_URL}"
    echo "============================================="
    echo ""

    exit 0
fi

# ---------------------------------------------------------------------------
# Step 7: Environment file
# ---------------------------------------------------------------------------

info "Writing environment file..."

cat > "${TALOS_ENV}" <<EOF
# Talos Configuration
# Generated by installer on $(date -u '+%Y-%m-%d %H:%M:%S UTC')

# Server
TALOS_HOST=0.0.0.0
TALOS_PORT=${TALOS_PORT}

# Domain
TALOS_DOMAIN=${TALOS_DOMAIN}
TALOS_ACME_EMAIL=${TALOS_ACME_EMAIL}
TALOS_PROXY_MODE=${TALOS_PROXY_MODE}
TALOS_EDGE_NETWORK=${TALOS_EDGE_NETWORK}
TALOS_EDGE_CERT_RESOLVER=${TALOS_EDGE_CERT_RESOLVER}

# Database
TALOS_DB_PATH=${TALOS_DATA}/talos.db

# Auth
TALOS_SESSION_SECRET=${SESSION_SECRET}
TALOS_SESSION_MAX_AGE=604800

# Docker
TALOS_DOCKER_HOST=unix:///var/run/docker.sock
TALOS_DOCKER_NETWORK=${DOCKER_NETWORK}

# Traefik
TALOS_TRAEFIK_IMAGE=${TRAEFIK_IMAGE}
TALOS_TRAEFIK_CONFIG_DIR=${TALOS_DATA}/traefik/config
TALOS_TRAEFIK_DATA_DIR=${TALOS_DATA}/traefik/data
TALOS_TRAEFIK_DASHBOARD=false

# GitHub App (optional — configure after installation)
# TALOS_GITHUB_WEBHOOK_SECRET=
# TALOS_GITHUB_APP_ID=
# TALOS_GITHUB_APP_SLUG=
# TALOS_GITHUB_APP_PRIVATE_KEY=
# TALOS_GITHUB_APP_CLIENT_ID=
# TALOS_GITHUB_APP_CLIENT_SECRET=
EOF

chmod 600 "${TALOS_ENV}"
chown "${TALOS_USER}:${TALOS_USER}" "${TALOS_ENV}"
ok "Environment file written to ${TALOS_ENV}."

# ---------------------------------------------------------------------------
# Step 8: Traefik
# ---------------------------------------------------------------------------

info "Setting up Traefik..."

if [[ "${TALOS_PROXY_MODE}" == "external" ]]; then
    info "External proxy mode enabled — skipping Talos-managed Traefik."
else
    # Stop existing Traefik container if present
    if docker inspect "${TRAEFIK_CONTAINER}" &>/dev/null 2>&1; then
        info "Stopping existing Traefik container..."
        docker stop "${TRAEFIK_CONTAINER}" >/dev/null 2>&1 || true
        docker rm "${TRAEFIK_CONTAINER}" >/dev/null 2>&1 || true
    fi

    # Generate static Traefik configuration
    cat > "${TALOS_DATA}/traefik/traefik.yaml" <<EOF
# Traefik static configuration for Talos
api:
  dashboard: false
  insecure: false

entryPoints:
  web:
    address: ":80"

providers:
  file:
    directory: /etc/traefik/config
    watch: true

log:
  level: WARN

accessLog:
  filePath: /var/log/traefik/access.log
  bufferingSize: 100
EOF

    chown "${TALOS_USER}:${TALOS_USER}" "${TALOS_DATA}/traefik/traefik.yaml"

    docker run -d \
        --name "${TRAEFIK_CONTAINER}" \
        --restart unless-stopped \
        --network "${DOCKER_NETWORK}" \
        -p 80:80 \
        -p 443:443 \
        -v "${TALOS_DATA}/traefik/traefik.yaml:/etc/traefik/traefik.yaml:ro" \
        -v "${TALOS_DATA}/traefik/config:/etc/traefik/config:ro" \
        -v "${TALOS_DATA}/traefik/data:/var/log/traefik" \
        "${TRAEFIK_IMAGE}" \
        >/dev/null

    ok "Traefik container started on ports 80/443."
fi

# ---------------------------------------------------------------------------
# Step 9: Systemd service
# ---------------------------------------------------------------------------

info "Creating systemd service..."

cat > "${SYSTEMD_UNIT}" <<EOF
[Unit]
Description=Talos Deployment Platform
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
User=${TALOS_USER}
Group=${TALOS_USER}
WorkingDirectory=${TALOS_HOME}
EnvironmentFile=${TALOS_ENV}
ExecStart=${TALOS_BIN}
Restart=on-failure
RestartSec=5
StartLimitIntervalSec=60
StartLimitBurst=3

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${TALOS_HOME}
PrivateTmp=true

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=talos

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable talos.service
ok "Systemd service created and enabled."

# ---------------------------------------------------------------------------
# Step 10: Verify
# ---------------------------------------------------------------------------

info "Running verification checks..."

CHECKS_PASSED=0
CHECKS_TOTAL=5

# Check 1: Docker running
if docker info &>/dev/null; then
    ok "  [1/5] Docker daemon is running."
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    warn "  [1/5] Docker daemon is not running."
fi

# Check 2: Network exists
if docker network inspect "${DOCKER_NETWORK}" &>/dev/null; then
    ok "  [2/5] Docker network '${DOCKER_NETWORK}' exists."
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    warn "  [2/5] Docker network '${DOCKER_NETWORK}' not found."
fi

# Check 3: Traefik running
if docker inspect -f '{{.State.Running}}' "${TRAEFIK_CONTAINER}" 2>/dev/null | grep -q true; then
    ok "  [3/5] Traefik container is running."
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    warn "  [3/5] Traefik container is not running."
fi

# Check 4: Binary exists
if [[ -x "${TALOS_BIN}" ]]; then
    ok "  [4/5] Talos binary is installed."
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    warn "  [4/5] Talos binary not found."
fi

# Check 5: Env file exists
if [[ -f "${TALOS_ENV}" ]]; then
    ok "  [5/5] Environment file exists."
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    warn "  [5/5] Environment file not found."
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

echo ""
echo "============================================="
if [[ ${CHECKS_PASSED} -eq ${CHECKS_TOTAL} ]]; then
    echo -e "${GREEN}  Talos installed successfully!${NC}"
else
    echo -e "${YELLOW}  Talos installed with warnings (${CHECKS_PASSED}/${CHECKS_TOTAL} checks passed).${NC}"
fi
echo "============================================="
echo ""
echo "  Install dir:    ${TALOS_HOME}"
echo "  Binary:         ${TALOS_BIN}"
echo "  Config:         ${TALOS_ENV}"
echo "  Data:           ${TALOS_DATA}"
echo "  Web UI port:    ${TALOS_PORT}"
echo ""
echo "  Start Talos:    sudo systemctl start talos"
echo "  View logs:      sudo journalctl -u talos -f"
echo "  Check status:   sudo systemctl status talos"
echo ""
echo "  Traefik:        docker logs ${TRAEFIK_CONTAINER}"
echo ""
echo "  Open in browser: ${ACCESS_URL}"
echo "============================================="
echo ""
