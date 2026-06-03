# Uninstalling Talos

This guide covers how to remove Talos from your server, with options to preserve your data for future use or perform a full purge.

## Option 1: Preserve Data (Recommended)

This approach stops the Talos service and removes the binary, but keeps all your data intact. You can reinstall later and pick up where you left off.

### Bare Binary Mode

```bash
# Stop and disable the systemd service
sudo systemctl stop talos
sudo systemctl disable talos

# Remove the systemd unit file
sudo rm /etc/systemd/system/talos.service
sudo systemctl daemon-reload

# Remove the binary
sudo rm /usr/local/bin/talos

# (Optional) Stop Traefik
docker stop talos-traefik
docker rm talos-traefik
```

Your data remains at `/opt/talos/`:

| Preserved Item | Path |
|----------------|------|
| Configuration | `/opt/talos/.env` |
| Database | `/opt/talos/data/talos.db` |
| Service data | `/opt/talos/data/services/` |
| Backups | `/opt/talos/data/backups/` |
| Traefik config | `/opt/talos/data/traefik/` |

To reinstall later, run the installer again. It will detect the existing data directory and reuse it.

### Docker Mode

```bash
# Stop and remove the Talos container
docker stop talos
docker rm talos

# (Optional) Stop Traefik
docker stop talos-traefik
docker rm talos-traefik
```

Your data remains in the Docker volume at `/opt/talos/data/`. To reinstall:

```bash
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

## Option 2: Full Purge

This removes Talos and all associated data permanently.

:::danger
This permanently deletes all apps, deploys, services, backups, and configuration. This action cannot be undone.
:::

### Before You Purge

1. **Download any backups** you want to keep:
   ```bash
   # List backups
   ls /opt/talos/data/backups/
   
   # Copy to a safe location
   cp /opt/talos/data/backups/talos-backup-*.tar.gz ~/talos-backups/
   ```

2. **Export your .env** if you want to reference it later:
   ```bash
   cp /opt/talos/.env ~/talos-env-backup
   ```

### Bare Binary Full Purge

```bash
# Stop and disable the service
sudo systemctl stop talos
sudo systemctl disable talos
sudo rm /etc/systemd/system/talos.service
sudo systemctl daemon-reload

# Remove the binary
sudo rm /usr/local/bin/talos
sudo rm -f /usr/local/bin/talos.bak.*

# Remove Traefik container
docker stop talos-traefik 2>/dev/null || true
docker rm talos-traefik 2>/dev/null || true

# Remove the Docker network
docker network rm talos 2>/dev/null || true

# Remove all Talos data
sudo rm -rf /opt/talos

# Remove the system user
sudo userdel talos 2>/dev/null || true
```

### Docker Mode Full Purge

```bash
# Stop and remove containers
docker stop talos 2>/dev/null || true
docker rm talos 2>/dev/null || true
docker stop talos-traefik 2>/dev/null || true
docker rm talos-traefik 2>/dev/null || true

# Remove the Docker network
docker network rm talos 2>/dev/null || true

# Remove Docker images
docker rmi ghcr.io/logic-roastery/project-talos:latest 2>/dev/null || true
docker rmi traefik:v3.0 2>/dev/null || true

# Remove all Talos data
sudo rm -rf /opt/talos

# Remove the system user
sudo userdel talos 2>/dev/null || true
```

## Stopping Managed Services

Before uninstalling, you may want to stop managed services (PostgreSQL, MySQL, Redis, Garage) that Talos provisioned:

```bash
# List Talos-managed containers
docker ps -a --filter "label=managed-by=talos"

# Stop and remove them
docker ps -a --filter "label=managed-by=talos" -q | xargs docker stop
docker ps -a --filter "label=managed-by=talos" -q | xargs docker rm
```

:::warning
If you want to preserve service data (databases), make sure to back up the service volumes at `/opt/talos/data/services/` before removing them.
:::

## Verifying Removal

After a full purge, verify everything is cleaned up:

```bash
# Check for remaining containers
docker ps -a --filter "label=managed-by=talos"

# Check for remaining data
ls /opt/talos 2>/dev/null || echo "Data directory removed"

# Check for the binary
which talos 2>/dev/null || echo "Binary removed"

# Check systemd
systemctl status talos 2>/dev/null || echo "Service removed"
```

## Next Steps

- [Installation](./installation.md) -- reinstall Talos
- [Backup & Restore](./backup.md) -- restore from a previous backup
