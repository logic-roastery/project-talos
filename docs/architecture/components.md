# Components

This page describes each major component of the Talos architecture in detail.

## Talos Server

The Talos server is the central control plane. It is a single Go binary built with:

- **[chi router](https://github.com/go-chi/chi)** for HTTP routing and middleware
- **Server-rendered HTML templates** for the web UI
- **JSON API endpoints** for programmatic access

### Responsibilities

- User authentication and session management
- Application CRUD operations
- Deployment orchestration (blue/green)
- Managed service provisioning
- Backup creation, scheduling, and restoration
- GitHub webhook handling and App integration
- Traefik route management

### Key Packages

| Package | Purpose |
|---------|---------|
| `internal/server` | HTTP server setup, middleware chain, route registration |
| `internal/server/handlers` | Request handlers for apps, deploys, services, backups, auth, pages |
| `internal/deploy` | Deployment engine with blue/green orchestration |
| `internal/backup` | Backup manager with scheduling and retention |
| `internal/services` | Managed service provisioning and credential handling |
| `internal/auth` | Authentication service and password hashing |
| `internal/crypto` | AES-256-GCM encryption for credentials |

### Middleware Stack

1. **RecoverMiddleware** -- catches panics and returns 500 errors
2. **LoggingMiddleware** -- structured request logging via `slog`
3. **AuthMiddleware** -- validates session tokens for API routes
4. **WebAuthMiddleware** -- validates sessions for HTML page routes

## SQLite Store

SQLite provides persistent storage for all Talos metadata. The store layer is in `internal/store/`.

### Architecture

- Single database file at the configured `TALOS_DB_PATH`
- Automatic schema migrations on startup (versioned, additive only)
- Repository pattern: separate repos for apps, deploys, services, users, backups
- Connection managed via `database/sql`

### Migration System

Migrations are defined as a map of version number to SQL statement. On startup:

1. Talos reads the highest applied version from `schema_migrations`.
2. Any pending migrations are applied in order.
3. Each migration is recorded in the tracking table.

Migrations are additive only -- new columns, tables, and indexes. They never drop or modify existing data.

## Docker Runtime

The Docker runtime (`internal/runtime/docker`) wraps the Docker API to manage containers.

### Capabilities

- Pull container images from registries
- Start containers with configuration (image, ports, env vars, labels, volumes)
- Stop and remove containers
- Health-check containers with configurable timeout
- Inspect container state

### Container Naming

Talos follows a naming convention for containers:

- App containers: `talos-<app-name>` (live) and `talos-<app-name>-<deploy-id>` (staging)
- Service containers: `talos-svc-<service-name>`
- Traefik: `talos-traefik`

All Talos-managed containers are labeled with `managed-by: talos` for identification.

### Network

All containers join the `talos` Docker network (configurable via `TALOS_DOCKER_NETWORK`). This allows:

- Traefik to reach application containers by name
- Application containers to reach service containers by name
- Isolation from other Docker workloads on the host

## Traefik Proxy

Traefik serves as the public ingress, handling routing and TLS termination.

### Configuration

Talos generates Traefik configuration files dynamically:

- **Static config** (`traefik.yml`) -- entrypoints, certificate resolvers, providers
- **Dynamic config** (per-app `.yml` files) -- routing rules, services, TLS settings

### Domain Mode

When `TALOS_DOMAIN` is set:

- Traefik listens on ports 80 and 443
- HTTP is redirected to HTTPS
- Let's Encrypt certificates are obtained automatically via the HTTP challenge
- App routes use `Host()` rules to match domains

### IP Mode

When no domain is configured:

- Traefik is not started
- Applications are accessed directly via `<server-ip>:<fallback-port>`
- Each app gets a unique port assigned

### Route Updates

When a deploy succeeds, Talos writes a new Traefik route config file. Traefik watches the config directory and picks up changes automatically (file provider with `watch: true`).

## GitHub Integration

Talos integrates with GitHub to trigger deployments automatically.

### GitHub App

The preferred integration method. Setup via the wizard at `/settings/github/setup`:

1. Talos creates a GitHub App manifest.
2. The user installs the App on their GitHub account/organization.
3. Talos receives installation credentials via callback.
4. Webhooks are configured automatically.

### Webhook Handler

Located in `internal/github/webhook.go`:

- Verifies webhook signatures using HMAC-SHA256
- Parses `workflow_run` events to detect successful builds
- Looks up the target app by repository name
- Constructs the image reference from the commit SHA
- Triggers a deployment through the deploy engine

### Supported Events

| Event | Action |
|-------|--------|
| `workflow_run` (completed, success) | Triggers a deployment |
| `installation` (created) | Logs installation |
| `installation` (deleted) | Logs uninstallation |

## Backup Manager

The backup manager (`internal/backup`) handles full system backups.

### Architecture

- **Manager** -- orchestrates backup creation, restoration, and deletion
- **Scheduler** -- runs periodic backups at the configured interval
- **Store** -- persists backup metadata in SQLite

### Backup Process

1. `VACUUM INTO` creates an atomic SQLite snapshot in a temp directory.
2. Service data directories are walked and added to the archive.
3. Everything is compressed into a `.tar.gz` file.
4. The backup record is saved to SQLite.
5. The retention policy is enforced (oldest backups deleted if count exceeds limit).

### Restore Process

1. The backup archive is opened and extracted.
2. The SQLite database is replaced with the snapshot.
3. Service data directories are restored.
4. The Talos process must be restarted to load the new database.

## Next Steps

- [Deployment Flow](./deployment-flow.md) -- blue/green deployment in detail
- [Data Model](./data-model.md) -- database schema reference
- [Architecture Overview](./index.md) -- system overview
