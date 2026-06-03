package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/logic-roastery/project-talos/internal/domain"
)

const backupColumns = `id, filename, size_bytes, type, status, created_at`

func backupScanFields(b *domain.Backup) []interface{} {
	return []interface{}{&b.ID, &b.Filename, &b.SizeBytes, &b.Type, &b.Status, &b.CreatedAt}
}

func (s *SQLiteStore) CreateBackup(ctx context.Context, backup *domain.Backup) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO backups (filename, size_bytes, type, status) VALUES (?, ?, ?, ?)`,
		backup.Filename, backup.SizeBytes, backup.Type, backup.Status,
	)
	if err != nil {
		return fmt.Errorf("insert backup: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("get backup id: %w", err)
	}
	backup.ID = id
	return nil
}

func (s *SQLiteStore) GetBackup(ctx context.Context, id int64) (*domain.Backup, error) {
	b := &domain.Backup{}
	err := s.db.QueryRowContext(ctx,
		`SELECT `+backupColumns+` FROM backups WHERE id = ?`, id,
	).Scan(backupScanFields(b)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get backup: %w", err)
	}
	return b, nil
}

func (s *SQLiteStore) ListBackups(ctx context.Context) ([]*domain.Backup, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+backupColumns+` FROM backups ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	var backups []*domain.Backup
	for rows.Next() {
		b := &domain.Backup{}
		if err := rows.Scan(backupScanFields(b)...); err != nil {
			return nil, fmt.Errorf("scan backup: %w", err)
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

func (s *SQLiteStore) DeleteBackup(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete backup rows affected: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
