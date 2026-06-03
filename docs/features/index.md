# Features

Talos provides a complete self-hosted deployment platform for Dockerized applications. This page lists all major features with links to detailed documentation.

## Application Management

Create, configure, and manage applications through the web UI or API. Each app tracks its repository, image, environment variables, deployment history, and routing configuration.

- CRUD operations for applications
- Environment variable management with secrets and required vars
- Deploy history with rollback support
- Live container log streaming

[Learn more about App Management](./app-management.md)

## Blue/Green Deployments

Zero-downtime deployments using the blue/green strategy. A staging container starts alongside the live one, passes health checks, and only then receives traffic.

- Automatic rollback on health check failure
- 30-second health check timeout
- Structured deploy events for diagnostics
- Manual rollback to any previous successful deploy

[Learn more about the Deployment Flow](../architecture/deployment-flow.md)

## Managed Services

Provision and manage backing services (databases, caches, storage) directly from Talos. Services run as Docker containers with persistent storage and encrypted credentials.

- PostgreSQL, MySQL, Redis, and Garage support
- Automatic credential generation and encryption
- Environment variable injection into linked apps
- Start, stop, and lifecycle management

[Learn more about Managed Services](./managed-services.md)

## Traefik Routing

Built-in reverse proxy with automatic TLS via Let's Encrypt. Traefik handles domain-based routing and HTTPS for all deployed applications.

- Domain mode with automatic HTTPS
- IP:port fallback mode for domain-free setups
- Dynamic route updates on deploy
- Migration between routing modes

[Learn more about Routing](./routing.md)

## Backup & Restore

Full system backup and restore with scheduled automation. Backups capture the SQLite database and service data volumes in a single archive.

- Manual and scheduled backups
- Configurable retention policy
- One-click restore with process restart
- Download archives for off-site storage

[Learn more about Backup](./backup.md)

## GitHub Integration

Automatic deployments triggered by GitHub events. Set up a GitHub App or webhook to deploy on successful CI builds.

- GitHub App setup wizard
- Webhook signature verification (HMAC-SHA256)
- Automatic image reference construction from commit SHA
- Support for GHCR, Docker Hub, and custom registries

## Authentication

Secure access to the Talos web UI and API with session-based authentication.

- Initial admin account setup wizard
- Bcrypt password hashing
- Configurable session lifetime
- HMAC-signed session tokens

## Environment Management

Per-application environment variable management with security features.

- Required variables that block deploys when missing
- Secret masking in UI and API
- Change history tracking
- Deploy-time snapshots for diff visibility
- Reveal endpoint for authenticated access to secret values

## Container Log Streaming

Real-time log streaming from running application containers via the web UI.

- Stream logs directly from Docker
- Accessible via the app detail page
- API endpoint for programmatic access

## Next Steps

- [Architecture Overview](../architecture/index.md) -- how it all fits together
- [First Deployment](../guide/first-deploy.md) -- get started deploying
- [Configuration](../guide/configuration.md) -- all environment variables
