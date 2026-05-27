# Talos

Self-hosted deployment platform for Dockerized applications on a single VPS.

## Quick Start

```bash
git clone https://github.com/logic-roastery/project-talos.git
cd project-talos
cp .env.example .env
# Edit .env — at minimum set TALOS_SESSION_SECRET
make build
./bin/talos
```

Open `http://your-vps-ip:3000` and create your admin account.

## Install

One-line install on a fresh Linux VPS (Ubuntu/Debian/Fedora):

```bash
curl -sSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo sh
```

This installs Talos as a bare binary with a systemd service. Docker is installed automatically if missing.

For Docker mode (container-based, easier upgrades):

```bash
curl -sSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo sh -s -- --docker
```

After install, open `http://<your-server-ip>:3000` and create your admin account.

## Domain Setup

The installer will ask if you have a domain. You have two options:

**IP mode** (default) — access at `http://<your-vps-ip>:3000`. No domain needed.

**Domain mode** — access at `https://talos.example.com` with automatic HTTPS via Let's Encrypt.
Point your domain's DNS A record at your VPS IP, then set:

```
TALOS_DOMAIN=talos.example.com
TALOS_ACME_EMAIL=admin@example.com
```

Talos will start Traefik with auto-TLS and redirect HTTP to HTTPS automatically.

## Configuration

All configuration is via environment variables. Copy `.env.example` to `.env` and edit.

| Variable | Default | Description |
|----------|---------|-------------|
| `TALOS_HOST` | `0.0.0.0` | Listen address |
| `TALOS_PORT` | `3000` | Listen port |
| `TALOS_DOMAIN` | *(empty)* | Domain name (enables HTTPS via Let's Encrypt) |
| `TALOS_ACME_EMAIL` | *(empty)* | Email for Let's Encrypt certificate notifications |
| `TALOS_DB_PATH` | `data/talos.db` | SQLite database path |
| `TALOS_SESSION_SECRET` | **required** | HMAC signing secret for sessions |
| `TALOS_SESSION_MAX_AGE` | `604800` | Session lifetime in seconds (7 days) |
| `TALOS_DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker daemon socket |
| `TALOS_DOCKER_NETWORK` | `talos` | Docker network name for containers |
| `TALOS_ENCRYPTION_KEY` | auto-generated | Base64 AES-256 key for credential encryption |
| `TALOS_TRAEFIK_IMAGE` | `traefik:v3.0` | Traefik reverse proxy image |
| `TALOS_TRAEFIK_DASHBOARD` | `false` | Enable Traefik dashboard |

### GitHub App (optional)

Set these to enable automatic deployments from GitHub pushes:

| Variable | Description |
|----------|-------------|
| `TALOS_GITHUB_WEBHOOK_SECRET` | Webhook verification secret |
| `TALOS_GITHUB_APP_ID` | GitHub App ID |
| `TALOS_GITHUB_APP_SLUG` | GitHub App slug |
| `TALOS_GITHUB_APP_PRIVATE_KEY` | Path or contents of private key |
| `TALOS_GITHUB_APP_CLIENT_ID` | OAuth client ID |
| `TALOS_GITHUB_APP_CLIENT_SECRET` | OAuth client secret |

Or use the setup wizard at `/settings/github/setup`.

## Architecture

```
Browser → Traefik → App Container
            ↕
Talos (chi + SQLite) → Docker Engine → Service Container
```

- **chi** — HTTP router
- **SQLite** — embedded database (pure Go, no CGO)
- **Docker Engine API** — container management
- **Traefik** — reverse proxy with automatic TLS

## Development

```bash
make dev          # Start dev server on port 4001
make dev-watch    # Auto-reload with air
make dev-fresh    # Reset DB and restart
make test         # Run tests with race detection
make test-cover   # Tests with HTML coverage report
make vet          # Static analysis
make ps           # List Talos-managed containers
```

## Managed Services

Talos can provision and manage backing services:

| Type | Image | Default Port |
|------|-------|-------------|
| PostgreSQL | `postgres:16` | 5432 |
| MySQL | `mysql:8` | 3306 |
| Redis | `redis:7-alpine` | 6379 |
| Garage | `dxflrs/garage:v1.0` | 3900 |

Services run as Docker containers managed by Talos. Credentials are encrypted at rest with AES-256-GCM.

## Docker

```bash
docker run -d \
  -p 3000:3000 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v talos-data:/data \
  --env-file .env \
  ghcr.io/logic-roastery/project-talos:latest
```

## License

See [LICENSE](LICENSE).
