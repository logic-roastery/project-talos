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

```mermaid
flowchart LR
    U[Browser]
    GH[GitHub Webhooks]
    T[Traefik<br/>Routing + TLS]
    TS[Talos Server<br/>chi + handlers + deploy engine]
    DB[(SQLite)]
    D[Docker Engine]
    APP[App Containers]
    SVC[Managed Services<br/>Postgres / MySQL / Redis / Garage]

    U --> T
    T --> TS
    GH --> TS
    TS --> DB
    TS --> D
    D --> APP
    D --> SVC
    T --> APP
```

Talos acts as the control plane for a single VPS. It provides the web UI, stores state in SQLite, talks to the Docker Engine to create and manage workloads, and configures Traefik to route public traffic to deployed applications.

### Components

| Component | Responsibility |
|----------|----------------|
| Browser | UI for managing apps, services, deployments, and settings |
| Talos Server | Authentication, app/service management, deployment orchestration, GitHub integration |
| SQLite | Persistent storage for users, apps, services, deployments, and configuration |
| Docker Engine | Runs application containers and managed backing services |
| Traefik | Public entrypoint, reverse proxy, domain routing, and automatic TLS |
| GitHub Webhooks | Trigger deployments from repository events |

## How Talos Works

Talos is a single-server deployment control plane. It is not a container orchestrator in the Kubernetes sense, and it does not manage a multi-node cluster. Its job is to keep a small amount of desired state in SQLite, translate that state into Docker operations, and keep public routing aligned through Traefik.

### Control Plane vs Runtime

- **Control plane**: Talos server, SQLite, GitHub integration, deployment records, service definitions, and routing decisions
- **Runtime plane**: Docker containers, Docker network, container images, persistent service volumes, and Traefik as the public ingress

This split is important:

- SQLite stores what Talos knows about apps, services, users, and deploy history
- Docker is the source of truth for running containers and attached volumes
- Traefik is responsible for receiving public traffic and forwarding it to the correct application container

If Talos restarts, running containers can continue serving traffic. Talos is needed to create, update, stop, or re-route workloads, but it is not the same thing as the workloads themselves.

### App Lifecycle

When you create an app in the UI, Talos stores its metadata in SQLite: image reference, internal port, environment variables, linked services, and deployment history. A deploy can then be triggered manually or by a GitHub webhook.

At deploy time, Talos does the following:

1. Creates a deploy record in SQLite
2. Pulls the target container image from the registry
3. Stops and removes the previously managed container for that app
4. Builds the runtime environment by combining app env vars with credentials from linked services
5. Starts a new Docker container on the Talos-managed network
6. Waits for the container health check or running state
7. Updates Traefik routing so traffic points to the new container
8. Marks the deploy as successful or failed in SQLite

This means Talos owns deployment coordination, while Docker does the actual container execution.

### Managed Services

Managed services such as PostgreSQL, MySQL, Redis, and Garage are also regular Docker containers. Talos provisions them, creates persistent storage directories, generates credentials, encrypts those credentials before saving them, and injects connection environment variables into linked apps during deployment.

Each service has:

- A record in SQLite
- A Talos-managed Docker container
- A persistent data directory or volume path on the host
- Generated credentials encrypted at rest

Talos does not implement these databases itself. It standardizes how they are created, stored, linked, and restarted.

### Persistence Model

Talos spans three layers of persistence and state:

- **SQLite**: users, sessions, apps, services, deploy metadata, GitHub integration settings
- **Docker**: container state, images, networks, container health, service runtime configuration
- **Host filesystem**: SQLite database file, service data directories, local runtime artifacts

This is also the practical backup boundary:

- Back up SQLite to preserve Talos metadata
- Back up service data directories or Docker volumes to preserve database contents
- Rebuild containers from image references when restoring runtime state

### Codebase Map

The main packages are organized around control-plane responsibilities:

- `cmd/talos`: application entrypoint
- `internal/server`: HTTP server, middleware, HTML handlers, and web endpoints
- `internal/store`: SQLite persistence, repositories, and schema migrations
- `internal/deploy`: deployment orchestration and rollback entrypoints
- `internal/runtime/docker`: Docker API wrapper used to pull images and manage containers
- `internal/services`: managed service provisioning, credential handling, and service lifecycle
- `internal/proxy/traefik`: routing and reverse-proxy integration
- `internal/github`: GitHub App auth, webhook verification, and workflow-related integration
- `internal/auth` and `internal/crypto`: authentication, password handling, and encryption primitives
- `web/templates`: server-rendered UI templates

### Current Constraints

Talos is intentionally small and opinionated. Current design constraints include:

- Single VPS deployment model
- Docker runtime only
- SQLite as the control-plane database
- No cluster scheduler or multi-node placement
- No blue/green deployment flow yet
- Traefik as the built-in ingress path for domain-based routing

These constraints keep the system understandable and easy to self-host, but they also define the current product boundary.

### Deployment Flow

1. A user creates an app in Talos and configures its repository, image, or deployment settings.
2. Talos stores the app state and deployment metadata in SQLite.
3. A manual deploy or GitHub webhook triggers the deployment pipeline.
4. Talos uses the Docker Engine API to create, update, or restart the target containers.
5. Talos updates Traefik routing so the app is reachable by domain or server IP.
6. Incoming traffic flows through Traefik to the running application container.

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
