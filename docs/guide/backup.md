# Backup & Restore

Talos includes a built-in backup system that snapshots the SQLite database and service data volumes into a single `.tar.gz` archive. This guide covers creating, scheduling, restoring, and managing backups.

## What Gets Backed Up

Each backup captures the full state of your Talos installation:

| Component | Method |
|-----------|--------|
| SQLite database | Atomic snapshot via `VACUUM INTO` |
| PostgreSQL services | `pg_dumpall` via Docker exec |
| MySQL services | `mysqldump --all-databases` via Docker exec |
| Redis services | Volume directory copy |
| Garage services | Volume directory copy |
| Traefik TLS certificates | File copy |
| `.env` configuration | File copy (contains encryption key) |

Everything is bundled into a single `.tar.gz` file stored in the backup directory.

:::danger
Always keep the `.env` file safe. It contains `TALOS_ENCRYPTION_KEY`, which is required to decrypt service credentials stored in backups. Without this key, encrypted credentials are unrecoverable. If you intentionally replace the key later, old backups still require the original `.env` file.
:::

## Manual Backup

### Via the Web UI

1. Navigate to **Backups** in the Talos web UI.
2. Click **Create Backup**.
3. The backup runs and appears in the list once complete.
4. Click **Download** to save the `.tar.gz` for off-site storage.

### Via the API

```bash
# Create a backup
curl -X POST http://localhost:3000/api/backups \
  -H "Cookie: session=<your-session-cookie>"

# List all backups
curl http://localhost:3000/api/backups \
  -H "Cookie: session=<your-session-cookie>"

# Download a backup
curl -O http://localhost:3000/api/backups/{backupID}/download \
  -H "Cookie: session=<your-session-cookie>"
```

## Scheduled Backups

Configure automatic periodic backups using environment variables:

```bash
# In your .env file

# Run a backup every 60 minutes (set to 0 to disable)
TALOS_BACKUP_INTERVAL_MINUTES=60

# Keep the last 10 backups, delete older ones automatically
TALOS_BACKUP_RETAIN_COUNT=10
```

| Variable | Default | Description |
|----------|---------|-------------|
| `TALOS_BACKUP_INTERVAL_MINUTES` | `0` (disabled) | How often to run automatic backups, in minutes |
| `TALOS_BACKUP_RETAIN_COUNT` | `10` | Number of backups to keep before pruning old ones |
| `TALOS_BACKUP_DIR` | `data/backups` | Directory where backup files are stored |

:::tip
For production systems, set `TALOS_BACKUP_INTERVAL_MINUTES=60` (hourly) or `TALOS_BACKUP_INTERVAL_MINUTES=1440` (daily). Combine with `TALOS_BACKUP_RETAIN_COUNT=30` to keep a month of history.
:::

### How Retention Works

After each scheduled backup completes, Talos enforces the retention policy:

1. All backups are listed, ordered by creation date (newest first).
2. If the total count exceeds `TALOS_BACKUP_RETAIN_COUNT`, the oldest backups are deleted.
3. Both the backup file on disk and the database record are removed.

## Listing Backups

### Via the Web UI

The **Backups** page shows all available backups with:

- Filename
- Size
- Creation timestamp
- Actions (download, restore, delete)

### Via the API

```bash
curl http://localhost:3000/api/backups \
  -H "Cookie: session=<your-session-cookie>"
```

Response:

```json
[
  {
    "id": 1,
    "filename": "talos-backup-20250101-120000.tar.gz",
    "size_bytes": 5242880,
    "type": "full",
    "status": "completed",
    "created_at": "2025-01-01T12:00:00Z"
  }
]
```

## Restoring a Backup

### Via the Web UI

1. Navigate to **Backups**.
2. Find the backup you want to restore.
3. Click **Restore**.
4. Confirm the action.

### Via the API

```bash
curl -X POST http://localhost:3000/api/backups/{backupID}/restore \
  -H "Cookie: session=<your-session-cookie>"
```

:::warning
Restoring a backup replaces the current SQLite database and service volume data. After a restore, the Talos process must be restarted for the changes to take effect.
:::

### After Restoration

1. **Stop the Talos service** (bare binary mode):

   ```bash
   sudo systemctl stop talos
   ```

   Or stop the Docker container:

   ```bash
   docker stop talos
   ```

2. **Restart the Talos service**:

   ```bash
   sudo systemctl start talos
   ```

   Or restart the Docker container:

   ```bash
   docker start talos
   ```

3. Verify the restored state in the web UI.

## Deleting Backups

### Via the Web UI

Click the **Delete** button next to any backup in the backup list.

### Via the API

```bash
curl -X DELETE http://localhost:3000/api/backups/{backupID} \
  -H "Cookie: session=<your-session-cookie>"
```

## Disaster Recovery

If your server fails and you need to rebuild from a backup:

### 1. Install Talos on the New Server

```bash
curl -sSL https://raw.githubusercontent.com/logic-roastery/project-talos/master/scripts/install.sh | sudo bash
```

### 2. Copy the Backup and .env File

Transfer the backup `.tar.gz` and your `.env` file to the new server:

```bash
scp talos-backup-*.tar.gz user@new-server:/tmp/
scp .env user@new-server:/opt/talos/.env
```

### 3. Restore the Backup

Place the backup file in the backup directory and restart:

```bash
# Move the backup to the Talos backup directory
sudo mv /tmp/talos-backup-*.tar.gz /opt/talos/data/backups/

# Restart Talos to pick up the backup
sudo systemctl restart talos
```

Use the web UI or API to trigger a restore from the uploaded backup.

### 4. Rebuild Containers

After restoring, Talos has the metadata but containers need to be redeployed:

1. Navigate to each app in the web UI.
2. Trigger a deploy to pull images and recreate containers.
3. Managed services will be recreated from the restored volume data.

:::tip
Test your backup and restore process regularly. A backup you have never tested restoring is not a real backup.
:::

## Backup File Format

Backup files are standard `.tar.gz` archives with the following structure:

```
talos-backup-20250101-120000.tar.gz
  talos.db                    # SQLite database snapshot
  services/
    my-postgres/
      ...                     # PostgreSQL data directory
    my-redis/
      ...                     # Redis data directory
```

The `talos.db` file is created using SQLite's `VACUUM INTO` command, which produces a consistent snapshot without locking the live database.

## Next Steps

- [Configuration](./configuration.md) -- all environment variables
- [Upgrading](./upgrading.md) -- safe upgrade procedures
- [Backup Feature](../features/backup.md) -- technical details of the backup system
