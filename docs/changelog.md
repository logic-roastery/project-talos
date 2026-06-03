# Changelog

All notable changes to Talos are documented here.

## v1.0.0

Initial stable release.

### Features

- **Web UI** -- server-rendered interface for managing applications, services, deployments, and settings
- **Blue/Green Deployments** -- zero-downtime deployments with automatic rollback on health check failure
- **Managed Services** -- provision PostgreSQL, MySQL, Redis, and Garage as Docker containers
- **Backup & Restore** -- full system backups with `VACUUM INTO` snapshots and scheduled automation
- **GitHub Integration** -- automatic deployments via GitHub App webhooks and workflow_run events
- **Traefik Routing** -- domain-based routing with automatic HTTPS via Let's Encrypt
- **IP:Port Fallback** -- domain-free access mode using unique external ports
- **Environment Management** -- per-app env vars with secrets, required vars, and change history
- **Credential Encryption** -- AES-256-GCM encryption for service credentials at rest
- **Container Log Streaming** -- real-time log access from running containers
- **Session Authentication** -- secure session management with HMAC-signed tokens
- **Schema Migrations** -- automatic database schema upgrades on startup

### Supported Platforms

- Ubuntu / Debian / Fedora (bare binary mode)
- Docker (container-based mode)
- linux/amd64 and linux/arm64 architectures

### Configuration

- All configuration via environment variables
- `.env` file with `TALOS_SESSION_SECRET` as the only required variable
- Auto-generated `TALOS_ENCRYPTION_KEY` if not provided

### API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check |
| `POST /api/auth/setup` | Create admin account |
| `POST /api/auth/login` | Log in |
| `GET /api/apps` | List apps |
| `POST /api/apps` | Create app |
| `POST /api/apps/{id}/deploys` | Trigger deploy |
| `POST /api/apps/{id}/deploys/rollback` | Rollback deploy |
| `GET /api/services` | List services |
| `POST /api/services` | Create service |
| `GET /api/backups` | List backups |
| `POST /api/backups` | Create backup |
| `POST /api/backups/{id}/restore` | Restore backup |
| `POST /api/webhooks/github` | GitHub webhook endpoint |

### Default Images

| Component | Image |
|-----------|-------|
| PostgreSQL | `postgres:16` |
| MySQL | `mysql:8` |
| Redis | `redis:7-alpine` |
| Garage | `dxflrs/garage:v1.0` |
| Traefik | `traefik:v3.0` |
