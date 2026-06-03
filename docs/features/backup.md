# Backup System

Talos includes a built-in backup system that captures the full state of your installation into portable `.tar.gz` archives. This page covers the technical details of the backup architecture.

## Backup Manager Architecture

The backup system is implemented in `internal/backup` with three components:

| Component | Responsibility |
|-----------|----------------|
| **Manager** | Orchestrates backup creation, restoration, deletion, and retention enforcement |
| **Scheduler** | Runs periodic backups at the configured interval using a ticker |
| **Store** | Persists backup metadata in the `backups` SQLite table |

### Manager

The `Manager` struct holds references to the database, backup store, data directory, backup directory, retention count, and logger. It provides methods for:

- `CreateFullBackup` -- create a new backup
- `Restore` -- restore from a backup archive
- `DeleteBackup` -- remove a backup and its file
- `ListBackups` -- list all backup records
- `StartScheduler` -- begin periodic backup execution

### Scheduler

The scheduler runs in a goroutine and uses a `time.Ticker`:

```go
func (m *Manager) StartScheduler(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            m.CreateFullBackup(ctx)
        }
    }
}
```

The scheduler is started when `TALOS_BACKUP_INTERVAL_MINUTES` is greater than zero. The interval is converted from minutes to `time.Duration`.

## Backup Creation Process

1. A temporary directory is created for the SQLite snapshot.
2. `VACUUM INTO` creates an atomic copy of the database in the temp directory.
3. A `.tar.gz` archive is created in the backup directory.
4. The database snapshot is added to the archive as `talos.db`.
5. If a `services/` directory exists under the data directory, it is walked and added to the archive.
6. The archive writers are flushed and closed.
7. File size is recorded.
8. A backup record is saved to SQLite.
9. The retention policy is enforced.

### VACUUM INTO

SQLite's `VACUUM INTO` command creates a compact, consistent copy of the database without locking the live database. This means backups can be taken while Talos is running without affecting performance or consistency.

## Scheduling Engine

Configure automatic backups with environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `TALOS_BACKUP_INTERVAL_MINUTES` | `0` (disabled) | Backup interval in minutes |
| `TALOS_BACKUP_RETAIN_COUNT` | `10` | Maximum number of backups to keep |
| `TALOS_BACKUP_DIR` | `data/backups` | Directory for backup files |

### Recommended Intervals

| Use Case | Interval | Retention |
|----------|----------|-----------|
| Development | Disabled (`0`) | -- |
| Production (standard) | 60 minutes | 24 (1 day) |
| Production (conservative) | 1440 minutes (daily) | 30 (1 month) |
| High-traffic | 30 minutes | 48 (1 day) |

## Retention Policy

After each backup creation, the retention policy is enforced:

1. All backups are listed, ordered by `created_at DESC` (newest first).
2. If the count exceeds `TALOS_BACKUP_RETAIN_COUNT`, backups beyond the limit are deleted.
3. Both the file on disk and the SQLite record are removed.

```go
func (m *Manager) enforceRetentionPolicy(ctx context.Context) error {
    backups, _ := m.store.ListBackups(ctx)
    if len(backups) <= m.retain {
        return nil
    }
    for _, b := range backups[m.retain:] {
        m.deleteBackupFiles(b.Filename)
        m.store.DeleteBackup(ctx, b.ID)
    }
    return nil
}
```

## Event Logging

Backup operations are logged via `slog`:

- **Created**: `backup created id=1 filename=talos-backup-20250101-120000.tar.gz size=5242880`
- **Deleted**: `backup deleted id=1 filename=talos-backup-20250101-120000.tar.gz`
- **Restored**: `restore complete -- process must be restarted backup_id=1`
- **Scheduler started**: `backup scheduler started interval=1h0m0s`
- **Scheduler error**: `scheduled backup failed error=...`
- **Retention error**: `retention policy enforcement failed error=...`

## File Format

Backup files are standard `.tar.gz` archives:

```
talos-backup-YYYYMMDD-HHMMSS.tar.gz
  talos.db              # SQLite VACUUM INTO snapshot
  services/             # Service data volumes (if any)
    <service-name>/
      ...               # Volume contents
```

The archive is created using Go's `archive/tar` and `compress/gzip` packages.

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/backups` | List all backups |
| `POST` | `/api/backups` | Create a new backup |
| `GET` | `/api/backups/{id}/download` | Download a backup file |
| `POST` | `/api/backups/{id}/restore` | Restore from a backup |
| `DELETE` | `/api/backups/{id}` | Delete a backup |

### Create Backup

```bash
curl -X POST http://localhost:3000/api/backups \
  -H "Cookie: session=<your-session-cookie>"
```

Response:

```json
{
  "id": 1,
  "filename": "talos-backup-20250101-120000.tar.gz",
  "size_bytes": 5242880,
  "type": "full",
  "status": "completed",
  "created_at": "2025-01-01T12:00:00Z"
}
```

### List Backups

```bash
curl http://localhost:3000/api/backups \
  -H "Cookie: session=<your-session-cookie>"
```

### Download Backup

```bash
curl -O http://localhost:3000/api/backups/{id}/download \
  -H "Cookie: session=<your-session-cookie>"
```

### Restore Backup

```bash
curl -X POST http://localhost:3000/api/backups/{id}/restore \
  -H "Cookie: session=<your-session-cookie>"
```

:::warning
After restoring, the Talos process must be restarted. The restored database is not loaded until the process starts fresh.
:::

### Delete Backup

```bash
curl -X DELETE http://localhost:3000/api/backups/{id} \
  -H "Cookie: session=<your-session-cookie>"
```

## What Gets Backed Up

| Component | Method | Notes |
|-----------|--------|-------|
| SQLite database | `VACUUM INTO` | Atomic, consistent snapshot |
| Service volumes | File copy | All files under `data/services/` |
| `.env` config | Included in volume copy | Contains `TALOS_ENCRYPTION_KEY` |
| Traefik TLS certs | Included in volume copy | Under `data/traefik/data/` |

## Next Steps

- [Backup & Restore Guide](../guide/backup.md) -- hands-on backup procedures
- [Architecture Overview](../architecture/index.md) -- system overview
- [Configuration](../guide/configuration.md) -- backup environment variables
