# Upgrading Talos

This guide covers how to upgrade Talos to a newer version while preserving your configuration, data, and running applications.

## Quick Upgrade

The simplest way to upgrade is using the install script with the `--upgrade` flag:

If you installed Talos with `curl | bash`, run:

```bash
curl -fsSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash -s -- --upgrade
```

If you downloaded and kept `install.sh` on the server, run:

```bash
sudo bash install.sh --upgrade
```

The installer automatically detects your Talos installation mode (bare binary or Docker). This detection does not mean `install.sh` is available locally after a piped install.

## Bare Binary Mode

### Upgrade to Latest Version

If you installed with `curl | bash`:

```bash
curl -fsSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash -s -- --upgrade
```

If you saved `install.sh` locally:

```bash
sudo bash install.sh --upgrade
```

The upgrade process:

1. Detects the current version
2. Backs up the current binary to `/usr/local/bin/talos.bak.<timestamp>`
3. Stops the Talos systemd service
4. Downloads the latest release binary (or builds from source if no pre-built binary is available)
5. Starts the Talos service
6. Verifies the service is running

### Upgrade to a Specific Version

```bash
sudo bash install.sh --upgrade --version-tag v1.2.0
```

:::tip
Check available versions at [GitHub Releases](https://github.com/logic-roastery/project-talos/releases).
:::

### Rollback on Failure

If the upgrade fails, the script automatically rolls back to the previous binary. If you need to manually rollback:

```bash
# Stop the service
sudo systemctl stop talos

# Restore the backed-up binary (check available backups)
sudo ls /usr/local/bin/talos.bak.*
sudo cp /usr/local/bin/talos.bak.<timestamp> /usr/local/bin/talos

# Start the service
sudo systemctl start talos
```

Verify the rollback:

```bash
sudo systemctl status talos
talos --version
```

## Docker Mode

### Upgrade to Latest Version

If you installed with `curl | bash`:

```bash
curl -fsSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash -s -- --upgrade --docker
```

If you saved `install.sh` locally:

```bash
sudo bash install.sh --upgrade --docker
```

The Docker upgrade process:

1. Pulls the new Docker image
2. Compares image IDs (skips if already up to date)
3. Tags the current image for rollback (`rollback-<timestamp>`)
4. Stops and removes the current container
5. Creates a new container with the updated image
6. Verifies the container is running

### Upgrade to a Specific Version

If you installed with `curl | bash`:

```bash
curl -fsSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash -s -- --upgrade --docker --version-tag 0.2.6
```

If you saved `install.sh` locally:

```bash
sudo bash install.sh --upgrade --docker --version-tag 0.2.6
```

`--version-tag` must exactly match the repository tag name shown on GitHub. If your repo uses `v0.2.6`, include the `v`. If your repo uses `0.2.6`, do not add one.

### Manual Docker Upgrade

You can also upgrade manually:

```bash
# Pull the latest image
docker pull ghcr.io/logic-roastery/project-talos:latest

# Stop and remove the current container
docker stop talos
docker rm talos

# Start a new container with the same configuration
docker run -d \
  --name talos \
  --restart unless-stopped \
  --network talos \
  -p 3000:3000 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /opt/talos/data:/data \
  --env-file /opt/talos/.env \
  ghcr.io/logic-roastery/project-talos:latest
```

:::warning
`docker restart talos` is not a Docker upgrade. It only restarts the existing container and keeps it on the old image. To apply a new Talos image, recreate the container after `docker pull`, or use `sudo bash install.sh --upgrade --docker`.
:::

### Docker Rollback

If the upgrade fails, roll back to the previous image:

```bash
# Stop the failed container
docker stop talos
docker rm talos

# List available rollback images
docker images ghcr.io/logic-roastery/project-talos | grep rollback

# Start with the rollback image
docker run -d \
  --name talos \
  --restart unless-stopped \
  --network talos \
  -p 3000:3000 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /opt/talos/data:/data \
  --env-file /opt/talos/.env \
  ghcr.io/logic-roastery/project-talos:rollback-<timestamp>
```

## What Gets Preserved vs. Replaced

Understanding what survives an upgrade helps you plan accordingly.

### Preserved (Never Overwritten)

| Item | Location | Notes |
|------|----------|-------|
| `.env` configuration | `/opt/talos/.env` | Contains secrets, encryption key |
| SQLite database | `/opt/talos/data/talos.db` | Apps, deploys, users, services |
| Service data volumes | `/opt/talos/data/services/` | Database contents |
| Backup files | `/opt/talos/data/backups/` | All backup archives |
| Traefik TLS certificates | `/opt/talos/data/traefik/data/` | Let's Encrypt certs |
| Traefik config | `/opt/talos/data/traefik/config/` | Route definitions |
| Docker network | `talos` | Shared by all containers |

### Replaced During Upgrade

| Item | Notes |
|------|-------|
| Talos binary | Replaced with new version |
| Docker image | Pulled fresh from GHCR |
| Docker container | Recreated with new image |
| Systemd unit file | Regenerated by installer |

:::warning
The `.env` file is never overwritten by the installer. If new configuration options are added in a release, you may need to add them manually. Check the [changelog](../changelog.md) for new environment variables.
:::

### Database Migrations

Talos includes automatic schema migrations. When a new version starts:

1. It reads the current schema version from the `schema_migrations` table.
2. Any pending migrations are applied in order.
3. Migrations are additive only (new columns, tables, indexes) -- they never drop or modify existing data.

This means upgrading the binary or container automatically upgrades the database schema. No manual migration steps are required.

## Pre-Upgrade Checklist

Before upgrading in production:

1. **Create a backup**:
   ```bash
   curl -X POST http://localhost:3000/api/backups \
     -H "Cookie: session=<your-session-cookie>"
   ```

2. **Download the backup** for safekeeping:
   ```bash
   curl -O http://localhost:3000/api/backups/{backupID}/download \
     -H "Cookie: session=<your-session-cookie>"
   ```

3. **Check the [changelog](../changelog.md)** for breaking changes or new required configuration.

4. **Verify disk space** -- ensure enough room for the new binary/image alongside the backup.

5. **Plan a maintenance window** -- while upgrades are fast, there is a brief period where the service is restarting.

## Next Steps

- [Configuration](./configuration.md) -- all environment variables
- [Backup & Restore](./backup.md) -- backup before upgrading
- [Uninstalling](./uninstalling.md) -- removing Talos
