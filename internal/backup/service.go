package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/logic-roastery/project-talos/internal/crypto"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/store"
)

type Service struct {
	db          *sql.DB
	dbPath      string
	dataDir     string
	backupDir   string
	docker      *docker.Client
	services    store.ServiceStore
	backupStore store.BackupStore
	encKey      []byte
	logger      *slog.Logger
}

func NewService(
	db *sql.DB,
	dbPath string,
	dataDir string,
	dockerClient *docker.Client,
	services store.ServiceStore,
	backupStore store.BackupStore,
	encKey []byte,
	logger *slog.Logger,
) *Service {
	backupDir := filepath.Join(dataDir, "backups")
	return &Service{
		db:          db,
		dbPath:      dbPath,
		dataDir:     dataDir,
		backupDir:   backupDir,
		docker:      dockerClient,
		services:    services,
		backupStore: backupStore,
		encKey:      encKey,
		logger:      logger,
	}
}

// CreateBackup creates a full system backup and returns the backup record.
func (s *Service) CreateBackup(ctx context.Context) (*domain.Backup, error) {
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "talos-backup-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. SQLite snapshot
	if err := s.backupSQLite(ctx, tmpDir); err != nil {
		return nil, fmt.Errorf("backup sqlite: %w", err)
	}

	// 2. .env file
	if err := s.copyFileToDir(".env", tmpDir); err != nil {
		s.logger.Warn("backup: .env file not found, skipping", "error", err)
	}

	// 3. Traefik directory
	if err := s.copyDirToDir(filepath.Join(s.dataDir, "traefik"), filepath.Join(tmpDir, "traefik")); err != nil {
		s.logger.Warn("backup: traefik dir not found, skipping", "error", err)
	}

	// 4. Service data
	if err := s.backupServices(ctx, tmpDir); err != nil {
		s.logger.Warn("backup: some service backups failed", "error", err)
	}

	// 5. Create tar.gz
	timestamp := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("talos-backup-%s.tar.gz", timestamp)
	backupPath := filepath.Join(s.backupDir, filename)

	if err := s.createTarGz(tmpDir, backupPath); err != nil {
		return nil, fmt.Errorf("create tar.gz: %w", err)
	}

	// 6. Record in store
	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("stat backup file: %w", err)
	}

	backup := &domain.Backup{
		Filename:  filename,
		SizeBytes: info.Size(),
		Type:      "full",
		Status:    "completed",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.backupStore.CreateBackup(ctx, backup); err != nil {
		return nil, fmt.Errorf("record backup: %w", err)
	}

	s.logger.Info("backup created", "filename", filename, "size", info.Size())
	return backup, nil
}

// GetBackupPath returns the full filesystem path for a backup file.
func (s *Service) GetBackupPath(filename string) string {
	return filepath.Join(s.backupDir, filename)
}

// DeleteBackup removes a backup file and its database record.
func (s *Service) DeleteBackup(ctx context.Context, id int64) error {
	backup, err := s.backupStore.GetBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("get backup: %w", err)
	}

	backupPath := filepath.Join(s.backupDir, backup.Filename)
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove backup file: %w", err)
	}

	return s.backupStore.DeleteBackup(ctx, id)
}

func (s *Service) backupSQLite(ctx context.Context, dstDir string) error {
	dstPath := filepath.Join(dstDir, "talos.db")
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", dstPath)
	if err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	s.logger.Info("sqlite backup created", "path", dstPath)
	return nil
}

func (s *Service) backupServices(ctx context.Context, dstDir string) error {
	svcs, err := s.services.ListServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	servicesDir := filepath.Join(dstDir, "services")
	if err := os.MkdirAll(servicesDir, 0755); err != nil {
		return fmt.Errorf("create services dir: %w", err)
	}

	var errs []error
	for _, svc := range svcs {
		if svc.Status != domain.ServiceStatusActive || svc.ContainerID == "" {
			continue
		}

		containerName := fmt.Sprintf("talos-svc-%s", svc.Name)
		svcDir := filepath.Join(servicesDir, svc.Name)
		if err := os.MkdirAll(svcDir, 0755); err != nil {
			errs = append(errs, fmt.Errorf("mkdir %s: %w", svc.Name, err))
			continue
		}

		switch svc.Type {
		case domain.ServicePostgres:
			if err := s.backupPostgres(ctx, containerName, svc, svcDir); err != nil {
				s.logger.Warn("backup postgres failed", "service", svc.Name, "error", err)
				errs = append(errs, err)
			}
		case domain.ServiceMySQL:
			if err := s.backupMySQL(ctx, containerName, svc, svcDir); err != nil {
				s.logger.Warn("backup mysql failed", "service", svc.Name, "error", err)
				errs = append(errs, err)
			}
		case domain.ServiceRedis, domain.ServiceGarage:
			if err := s.backupVolume(svc, svcDir); err != nil {
				s.logger.Warn("backup volume failed", "service", svc.Name, "error", err)
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d service backup(s) failed: %v", len(errs), errs[0])
	}
	return nil
}

func (s *Service) backupPostgres(ctx context.Context, containerName string, svc *domain.Service, dstDir string) error {
	var creds domain.PostgresCredentials
	if err := s.decryptCreds(svc, &creds); err != nil {
		return err
	}

	output, err := s.docker.Exec(ctx, containerName, []string{
		"pg_dumpall", "-U", creds.User,
	})
	if err != nil {
		return fmt.Errorf("pg_dumpall: %w", err)
	}

	dumpPath := filepath.Join(dstDir, "dump.sql")
	return os.WriteFile(dumpPath, output, 0644)
}

func (s *Service) backupMySQL(ctx context.Context, containerName string, svc *domain.Service, dstDir string) error {
	var creds domain.MySQLCredentials
	if err := s.decryptCreds(svc, &creds); err != nil {
		return err
	}

	output, err := s.docker.Exec(ctx, containerName, []string{
		"mysqldump", "-u", creds.User, "-p"+creds.Password, "--all-databases",
	})
	if err != nil {
		return fmt.Errorf("mysqldump: %w", err)
	}

	dumpPath := filepath.Join(dstDir, "dump.sql")
	return os.WriteFile(dumpPath, output, 0644)
}

func (s *Service) backupVolume(svc *domain.Service, dstDir string) error {
	volHost := filepath.Join(s.dataDir, "services", svc.Name)
	return s.copyDirToDir(volHost, filepath.Join(dstDir, "volume"))
}

func (s *Service) decryptCreds(svc *domain.Service, target interface{}) error {
	credJSON, err := crypto.Decrypt(svc.Credentials, s.encKey)
	if err != nil {
		return fmt.Errorf("decrypt credentials: %w", err)
	}
	return json.Unmarshal([]byte(credJSON), target)
}

func (s *Service) copyFileToDir(srcPath, dstDir string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	dstPath := filepath.Join(dstDir, filepath.Base(srcPath))
	return os.WriteFile(dstPath, data, 0600)
}

func (s *Service) copyDirToDir(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

func (s *Service) createTarGz(srcDir, dstPath string) error {
	outFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

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

// RestoreBackup restores the system from a backup file.
// WARNING: This replaces the SQLite database. The server should be restarted after restore.
func (s *Service) RestoreBackup(ctx context.Context, id int64) error {
	backup, err := s.backupStore.GetBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("get backup: %w", err)
	}

	backupPath := filepath.Join(s.backupDir, backup.Filename)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}

	tmpDir, err := os.MkdirTemp("", "talos-restore-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Extract tar.gz
	if err := s.extractTarGz(backupPath, tmpDir); err != nil {
		return fmt.Errorf("extract tar.gz: %w", err)
	}

	// Stop all services
	if err := s.stopAllServices(ctx); err != nil {
		s.logger.Warn("restore: failed to stop some services", "error", err)
	}

	// Restore SQLite database
	srcDB := filepath.Join(tmpDir, "talos.db")
	if _, err := os.Stat(srcDB); err == nil {
		if err := s.restoreFile(srcDB, s.dbPath); err != nil {
			return fmt.Errorf("restore sqlite: %w", err)
		}
		s.logger.Info("sqlite database restored")
	}

	// Restore .env
	srcEnv := filepath.Join(tmpDir, ".env")
	if _, err := os.Stat(srcEnv); err == nil {
		if err := s.restoreFile(srcEnv, ".env"); err != nil {
			s.logger.Warn("restore: failed to restore .env", "error", err)
		}
	}

	// Restore traefik directory
	srcTraefik := filepath.Join(tmpDir, "traefik")
	if _, err := os.Stat(srcTraefik); err == nil {
		dstTraefik := filepath.Join(s.dataDir, "traefik")
		if err := s.copyDirToDir(srcTraefik, dstTraefik); err != nil {
			s.logger.Warn("restore: failed to restore traefik", "error", err)
		}
	}

	// Restore service data
	servicesDir := filepath.Join(tmpDir, "services")
	if _, err := os.Stat(servicesDir); err == nil {
		if err := s.restoreServices(servicesDir); err != nil {
			s.logger.Warn("restore: some service restores failed", "error", err)
		}
	}

	s.logger.Info("backup restored", "filename", backup.Filename)
	return nil
}

func (s *Service) stopAllServices(ctx context.Context) error {
	svcs, err := s.services.ListServices(ctx)
	if err != nil {
		return err
	}
	for _, svc := range svcs {
		if svc.Status == domain.ServiceStatusActive && svc.ContainerID != "" {
			containerName := fmt.Sprintf("talos-svc-%s", svc.Name)
			s.docker.StopAndRemove(ctx, containerName)
		}
	}
	return nil
}

func (s *Service) restoreServices(servicesDir string) error {
	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		svcDir := filepath.Join(servicesDir, entry.Name())
		volDir := filepath.Join(s.dataDir, "services", entry.Name())

		// Check for SQL dump (Postgres/MySQL)
		dumpPath := filepath.Join(svcDir, "dump.sql")
		if _, err := os.Stat(dumpPath); err == nil {
			s.logger.Info("found SQL dump for service", "service", entry.Name())
			continue
		}

		// Volume-based restore (Redis/Garage)
		volSrc := filepath.Join(svcDir, "volume")
		if _, err := os.Stat(volSrc); err == nil {
			if err := os.RemoveAll(volDir); err != nil {
				s.logger.Warn("restore: failed to clear volume dir", "service", entry.Name(), "error", err)
				continue
			}
			if err := s.copyDirToDir(volSrc, volDir); err != nil {
				s.logger.Warn("restore: failed to restore volume", "service", entry.Name(), "error", err)
			}
		}
	}
	return nil
}

func (s *Service) restoreFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

func (s *Service) extractTarGz(srcPath, dstDir string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dstDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(dstPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}

// FormatSize formats bytes into a human-readable string.
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
