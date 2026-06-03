package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/store"
)

type Manager struct {
	db        *sql.DB
	store     store.BackupStore
	dataDir   string
	backupDir string
	retain    int
	logger    *slog.Logger
}

func NewManager(db *sql.DB, store store.BackupStore, dataDir, backupDir string, retain int, logger *slog.Logger) *Manager {
	if retain <= 0 {
		retain = 10
	}
	return &Manager{
		db:        db,
		store:     store,
		dataDir:   dataDir,
		backupDir: backupDir,
		retain:    retain,
		logger:    logger,
	}
}

// CreateFullBackup creates a full backup of the database and service volumes.
func (m *Manager) CreateFullBackup(ctx context.Context) (*domain.Backup, error) {
	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup directory: %w", err)
	}

	ts := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("talos-backup-%s.tar.gz", ts)
	destPath := filepath.Join(m.backupDir, filename)

	// Create a temp directory for VACUUM INTO target.
	tmpDir, err := os.MkdirTemp("", "talos-backup-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dbCopyPath := filepath.Join(tmpDir, "talos.db")

	// Use VACUUM INTO to create a consistent snapshot of the database.
	if _, err := m.db.ExecContext(ctx, `VACUUM INTO ?`, dbCopyPath); err != nil {
		return nil, fmt.Errorf("vacuum into: %w", err)
	}

	// Create the tar.gz archive.
	f, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("create backup file: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add the database snapshot.
	if err := addFileToTar(tw, dbCopyPath, "talos.db"); err != nil {
		return nil, fmt.Errorf("tar database: %w", err)
	}

	// Add service volumes if the services directory exists.
	servicesDir := filepath.Join(m.dataDir, "services")
	if _, err := os.Stat(servicesDir); err == nil {
		if err := addDirToTar(tw, servicesDir, "services"); err != nil {
			return nil, fmt.Errorf("tar services: %w", err)
		}
	}

	// Close writers to flush before measuring size.
	tw.Close()
	gw.Close()
	f.Close()

	info, err := os.Stat(destPath)
	if err != nil {
		return nil, fmt.Errorf("stat backup file: %w", err)
	}

	backup := &domain.Backup{
		Filename:  filename,
		SizeBytes: info.Size(),
		Type:      "full",
		Status:    "completed",
	}
	if err := m.store.CreateBackup(ctx, backup); err != nil {
		return nil, fmt.Errorf("record backup: %w", err)
	}

	m.logger.Info("backup created", "id", backup.ID, "filename", filename, "size", info.Size())

	if err := m.enforceRetentionPolicy(ctx); err != nil {
		m.logger.Warn("retention policy enforcement failed", "error", err)
	}

	return backup, nil
}

func (m *Manager) enforceRetentionPolicy(ctx context.Context) error {
	backups, err := m.store.ListBackups(ctx)
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}

	if len(backups) <= m.retain {
		return nil
	}

	// backups is ordered by created_at DESC; delete the oldest ones.
	for _, b := range backups[m.retain:] {
		if err := m.deleteBackupFiles(b.Filename); err != nil {
			m.logger.Warn("failed to delete backup file", "filename", b.Filename, "error", err)
		}
		if err := m.store.DeleteBackup(ctx, b.ID); err != nil {
			m.logger.Warn("failed to delete backup record", "id", b.ID, "error", err)
		}
	}

	return nil
}

func (m *Manager) deleteBackupFiles(filename string) error {
	path := filepath.Join(m.backupDir, filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Restore restores the database and service volumes from a backup.
func (m *Manager) Restore(ctx context.Context, backupID int64) error {
	b, err := m.store.GetBackup(ctx, backupID)
	if err != nil {
		return fmt.Errorf("get backup: %w", err)
	}

	backupPath := filepath.Join(m.backupDir, b.Filename)
	f, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		switch header.Name {
		case "talos.db":
			dbPath := m.dbPath()
			if err := extractFile(tr, dbPath); err != nil {
				return fmt.Errorf("restore database: %w", err)
			}
			m.logger.Info("database restored", "path", dbPath)

		default:
			// Service volume files are stored under "services/<name>/..."
			if len(header.Name) > len("services/") && header.Name[:len("services/")] == "services/" {
				destPath := filepath.Join(m.dataDir, header.Name)
				if header.Typeflag == tar.TypeDir {
					if err := os.MkdirAll(destPath, 0755); err != nil {
						return fmt.Errorf("create dir %s: %w", destPath, err)
					}
				} else {
					if err := extractFile(tr, destPath); err != nil {
						return fmt.Errorf("restore %s: %w", header.Name, err)
					}
				}
			}
		}
	}

	m.logger.Info("restore complete — process must be restarted", "backup_id", backupID)
	return nil
}

// DeleteBackup removes a backup record and its file.
func (m *Manager) DeleteBackup(ctx context.Context, backupID int64) error {
	b, err := m.store.GetBackup(ctx, backupID)
	if err != nil {
		return fmt.Errorf("get backup: %w", err)
	}

	if err := m.deleteBackupFiles(b.Filename); err != nil {
		return fmt.Errorf("delete backup file: %w", err)
	}

	if err := m.store.DeleteBackup(ctx, backupID); err != nil {
		return fmt.Errorf("delete backup record: %w", err)
	}

	m.logger.Info("backup deleted", "id", backupID, "filename", b.Filename)
	return nil
}

// ListBackups returns all backup records.
func (m *Manager) ListBackups(ctx context.Context) ([]*domain.Backup, error) {
	return m.store.ListBackups(ctx)
}

// StartScheduler runs periodic backups at the given interval.
func (m *Manager) StartScheduler(ctx context.Context, interval time.Duration) {
	m.logger.Info("backup scheduler started", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("backup scheduler stopped")
			return
		case <-ticker.C:
			if _, err := m.CreateFullBackup(ctx); err != nil {
				m.logger.Error("scheduled backup failed", "error", err)
			}
		}
	}
}

func (m *Manager) dbPath() string {
	return filepath.Join(m.dataDir, "talos.db")
}

func addFileToTar(tw *tar.Writer, filePath, archivePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = archivePath

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(tw, f)
	return err
}

func addDirToTar(tw *tar.Writer, dirPath, archivePrefix string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		name := filepath.Join(archivePrefix, relPath)

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tw, f)
		return err
	})
}

func extractFile(tr *tar.Reader, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, tr)
	return err
}
