# Configuration

Talos is configured through environment variables stored in `/opt/talos/.env`.

## Core Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TALOS_PORT` | `3000` | Port for the Talos web UI |
| `TALOS_DB_PATH` | `data/talos.db` | Path to SQLite database |
| `TALOS_DATA_DIR` | `data` | Base data directory |
| `TALOS_ENCRYPTION_KEY` | _(auto-generated)_ | Key for encrypting secrets at rest |
| `TALOS_DOMAIN` | _(empty)_ | Public hostname for the Talos UI |
| `TALOS_ACME_EMAIL` | _(empty)_ | Contact email for internal Traefik TLS |
| `TALOS_PROXY_MODE` | `internal` | `internal` for Talos-managed Traefik, `external` for a shared edge proxy |
| `TALOS_EDGE_NETWORK` | `traefik-public` | Shared Docker network for external proxy mode |
| `TALOS_EDGE_CERT_RESOLVER` | `letsencrypt` | External Traefik cert resolver label value |
| `TALOS_DEBUG_ENDPOINTS` | `false` | Enables authenticated diagnostic endpoints such as `/api/github/debug` for temporary troubleshooting |

## Traefik Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TALOS_TRAEFIK_IMAGE` | `traefik:v3.0` | Traefik Docker image |
| `TALOS_TRAEFIK_DASHBOARD` | `false` | Enable Traefik dashboard |

## Backup Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TALOS_BACKUP_DIR` | `data/backups` | Directory for backup files |
| `TALOS_BACKUP_INTERVAL_MINUTES` | `0` | Scheduled backup interval (0 = disabled) |
| `TALOS_BACKUP_RETAIN_COUNT` | `10` | Number of backups to retain |

## GitHub App (optional)

| Variable | Default | Description |
|----------|---------|-------------|
| `TALOS_GITHUB_WEBHOOK_SECRET` | | Webhook signing secret configured on the GitHub App |
| `TALOS_GITHUB_APP_ID` | | GitHub App ID |
| `TALOS_GITHUB_APP_SLUG` | | GitHub App slug |
| `TALOS_GITHUB_APP_PRIVATE_KEY` | | GitHub App private key as a PEM string or PEM file path |
| `TALOS_GITHUB_APP_CLIENT_ID` | | GitHub App client ID |
| `TALOS_GITHUB_APP_CLIENT_SECRET` | | GitHub App client secret |

## Environment File Example

```bash
# /opt/talos/.env
TALOS_PORT=3000
TALOS_DB_PATH=data/talos.db
TALOS_DATA_DIR=data
TALOS_ENCRYPTION_KEY=auto-generated-key-here
TALOS_PROXY_MODE=internal
TALOS_EDGE_NETWORK=traefik-public
TALOS_EDGE_CERT_RESOLVER=letsencrypt
TALOS_DEBUG_ENDPOINTS=false

# Traefik
TALOS_TRAEFIK_IMAGE=traefik:v3.0
TALOS_TRAEFIK_DASHBOARD=false

# Backup
TALOS_BACKUP_DIR=data/backups
TALOS_BACKUP_INTERVAL_MINUTES=60
TALOS_BACKUP_RETAIN_COUNT=10
```

::: warning
Never commit your `.env` file to version control. The `TALOS_ENCRYPTION_KEY` is auto-generated on first run and protects all encrypted secrets.
:::

## Proxy Modes

- `internal`: Talos starts `talos-traefik`, owns `80/443`, and supports Talos-managed app custom domains.
- `external`: another reverse proxy owns `80/443`. Talos only publishes its UI hostname through container labels in Docker mode. App custom domains remain unsupported in this mode in v1.

## Debug Endpoints

Talos keeps diagnostic endpoints disabled by default.

- `TALOS_DEBUG_ENDPOINTS=false`: `/api/github/debug` returns `404`
- `TALOS_DEBUG_ENDPOINTS=true`: `/api/github/debug` is available to authenticated Talos users

The GitHub debug endpoint is intended for temporary troubleshooting only. It can confirm whether the GitHub App is configured, whether Talos can read the private key, how many installations GitHub returned, and whether repository listing is succeeding per installation.

Recommended use:

1. Set `TALOS_DEBUG_ENDPOINTS=true` in `/opt/talos/.env`
2. Recreate or restart Talos
3. Call the endpoint with an authenticated Talos session cookie
4. Set `TALOS_DEBUG_ENDPOINTS=false` again after debugging

Example:

```bash
curl -s http://127.0.0.1:3000/api/github/debug \
  -H 'Cookie: talos_session=YOUR_SESSION_COOKIE'
```
