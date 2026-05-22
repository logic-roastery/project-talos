package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/logic-roastery/project-talos/internal/domain"
)

func (s *SQLiteStore) CreateDeploy(ctx context.Context, d *domain.Deploy) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO deploys (app_id, image_ref, commit_sha, branch, status, triggered_by, rollback_of_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.AppID, d.ImageRef, d.CommitSHA, d.Branch, d.Status, d.TriggeredBy, d.RollbackOfID,
	)
	if err != nil {
		return fmt.Errorf("insert deploy: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("get deploy id: %w", err)
	}
	d.ID = id
	return nil
}

func (s *SQLiteStore) GetDeploy(ctx context.Context, id int64) (*domain.Deploy, error) {
	d := &domain.Deploy{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, image_ref, commit_sha, branch, status, container_id, health_status, logs, started_at, completed_at, triggered_by, rollback_of_id, created_at
		 FROM deploys WHERE id = ?`, id,
	).Scan(&d.ID, &d.AppID, &d.ImageRef, &d.CommitSHA, &d.Branch, &d.Status,
		&d.ContainerID, &d.HealthStatus, &d.Logs, &d.StartedAt, &d.CompletedAt,
		&d.TriggeredBy, &d.RollbackOfID, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get deploy: %w", err)
	}
	return d, nil
}

func (s *SQLiteStore) GetLatestDeploy(ctx context.Context, appID int64) (*domain.Deploy, error) {
	d := &domain.Deploy{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, image_ref, commit_sha, branch, status, container_id, health_status, logs, started_at, completed_at, triggered_by, rollback_of_id, created_at
		 FROM deploys WHERE app_id = ? ORDER BY created_at DESC LIMIT 1`, appID,
	).Scan(&d.ID, &d.AppID, &d.ImageRef, &d.CommitSHA, &d.Branch, &d.Status,
		&d.ContainerID, &d.HealthStatus, &d.Logs, &d.StartedAt, &d.CompletedAt,
		&d.TriggeredBy, &d.RollbackOfID, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest deploy: %w", err)
	}
	return d, nil
}

func (s *SQLiteStore) GetLatestSuccessfulDeploy(ctx context.Context, appID int64) (*domain.Deploy, error) {
	d := &domain.Deploy{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, image_ref, commit_sha, branch, status, container_id, health_status, logs, started_at, completed_at, triggered_by, rollback_of_id, created_at
		 FROM deploys WHERE app_id = ? AND status = 'success' ORDER BY created_at DESC LIMIT 1`, appID,
	).Scan(&d.ID, &d.AppID, &d.ImageRef, &d.CommitSHA, &d.Branch, &d.Status,
		&d.ContainerID, &d.HealthStatus, &d.Logs, &d.StartedAt, &d.CompletedAt,
		&d.TriggeredBy, &d.RollbackOfID, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest successful deploy: %w", err)
	}
	return d, nil
}

func (s *SQLiteStore) ListDeploys(ctx context.Context, appID int64, limit int) ([]*domain.Deploy, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, app_id, image_ref, commit_sha, branch, status, container_id, health_status, logs, started_at, completed_at, triggered_by, rollback_of_id, created_at
		 FROM deploys WHERE app_id = ? ORDER BY created_at DESC LIMIT ?`, appID, limit)
	if err != nil {
		return nil, fmt.Errorf("list deploys: %w", err)
	}
	defer rows.Close()

	var deploys []*domain.Deploy
	for rows.Next() {
		d := &domain.Deploy{}
		if err := rows.Scan(&d.ID, &d.AppID, &d.ImageRef, &d.CommitSHA, &d.Branch, &d.Status,
			&d.ContainerID, &d.HealthStatus, &d.Logs, &d.StartedAt, &d.CompletedAt,
			&d.TriggeredBy, &d.RollbackOfID, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan deploy: %w", err)
		}
		deploys = append(deploys, d)
	}
	return deploys, rows.Err()
}

func (s *SQLiteStore) UpdateDeploy(ctx context.Context, d *domain.Deploy) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE deploys SET status=?, container_id=?, health_status=?, logs=?, started_at=?, completed_at=?
		 WHERE id=?`,
		d.Status, d.ContainerID, d.HealthStatus, d.Logs, d.StartedAt, d.CompletedAt, d.ID)
	if err != nil {
		return fmt.Errorf("update deploy: %w", err)
	}
	return nil
}
