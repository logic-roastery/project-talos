package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/logic-roastery/project-talos/internal/domain"
)

const backupColumns = "id, filename, size, created_at"

func backupScanFields(b *domain.Backup) []interface{} {
	return []interface{}{&b.ID, &b.Filename, &b.Size, &b.CreatedAt}
}

func (s *SQLiteStore) CreateBackup(ctx context.Context, b *domain.Backup) error {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO backups (filename, size, created_at) VALUES (?, ?, ?)",
		b.Filename, b.Size, b.CreatedAt)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	b.ID = id
	return nil
}

func (s *SQLiteStore) ListBackups(ctx context.Context) ([]*domain.Backup, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+backupColumns+" FROM backups ORDER BY created_at DESC")
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return backups, nil
}

func (s *SQLiteStore) GetBackup(ctx context.Context, id int64) (*domain.Backup, error) {
	b := &domain.Backup{}
	err := s.db.QueryRowContext(ctx, "SELECT "+backupColumns+" FROM backups WHERE id = ?", id).
		Scan(backupScanFields(b)...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get backup: %w", err)
	}
	return b, nil
}

func (s *SQLiteStore) DeleteBackup(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM backups WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}
