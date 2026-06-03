# Configuration

Talos is configured through environment variables stored in `/etc/talos/.env`.

## Core Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TALOS_PORT` | `3000` | Port for the Talos web UI |
| `TALOS_DB_PATH` | `data/talos.db` | Path to SQLite database |
| `TALOS_DATA_DIR` | `data` | Base data directory |
| `TALOS_ENCRYPTION_KEY` | _(auto-generated)_ | Key for encrypting secrets at rest |

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
| `GITHUB_APP_ID` | | GitHub App ID |
| `GITHUB_APP_PRIVATE_KEY` | | GitHub App private key (base64) |
| `GITHUB_WEBHOOK_SECRET` | | Webhook signing secret |

## Environment File Example

```bash
# /etc/talos/.env
TALOS_PORT=3000
TALOS_DB_PATH=data/talos.db
TALOS_DATA_DIR=data
TALOS_ENCRYPTION_KEY=auto-generated-key-here

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
