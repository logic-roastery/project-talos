package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/logic-roastery/project-talos/internal/domain"
)

func (s *SQLiteStore) CreateApp(ctx context.Context, app *domain.App) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO apps (name, source, repo_url, branch, internal_port, image_ref, domain, fallback_port, access_mode, access_url, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		app.Name, app.Source, app.RepoURL, app.Branch, app.InternalPort,
		app.ImageRef, app.Domain, app.FallbackPort, app.AccessMode, app.AccessURL, app.Status,
	)
	if err != nil {
		return fmt.Errorf("insert app: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("get app id: %w", err)
	}
	app.ID = id
	return nil
}

func (s *SQLiteStore) GetApp(ctx context.Context, id int64) (*domain.App, error) {
	app := &domain.App{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, source, repo_url, branch, internal_port, image_ref, domain, fallback_port, access_mode, access_url, status, current_deploy_id, created_at, updated_at
		 FROM apps WHERE id = ?`, id,
	).Scan(&app.ID, &app.Name, &app.Source, &app.RepoURL, &app.Branch, &app.InternalPort,
		&app.ImageRef, &app.Domain, &app.FallbackPort, &app.AccessMode, &app.AccessURL,
		&app.Status, &app.CurrentDeployID, &app.CreatedAt, &app.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	return app, nil
}

func (s *SQLiteStore) GetAppByName(ctx context.Context, name string) (*domain.App, error) {
	app := &domain.App{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, source, repo_url, branch, internal_port, image_ref, domain, fallback_port, access_mode, access_url, status, current_deploy_id, created_at, updated_at
		 FROM apps WHERE name = ?`, name,
	).Scan(&app.ID, &app.Name, &app.Source, &app.RepoURL, &app.Branch, &app.InternalPort,
		&app.ImageRef, &app.Domain, &app.FallbackPort, &app.AccessMode, &app.AccessURL,
		&app.Status, &app.CurrentDeployID, &app.CreatedAt, &app.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get app by name: %w", err)
	}
	return app, nil
}

func (s *SQLiteStore) GetAppByDomain(ctx context.Context, d string) (*domain.App, error) {
	app := &domain.App{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, source, repo_url, branch, internal_port, image_ref, domain, fallback_port, access_mode, access_url, status, current_deploy_id, created_at, updated_at
		 FROM apps WHERE domain = ?`, d,
	).Scan(&app.ID, &app.Name, &app.Source, &app.RepoURL, &app.Branch, &app.InternalPort,
		&app.ImageRef, &app.Domain, &app.FallbackPort, &app.AccessMode, &app.AccessURL,
		&app.Status, &app.CurrentDeployID, &app.CreatedAt, &app.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get app by domain: %w", err)
	}
	return app, nil
}

func (s *SQLiteStore) ListApps(ctx context.Context) ([]*domain.App, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, source, repo_url, branch, internal_port, image_ref, domain, fallback_port, access_mode, access_url, status, current_deploy_id, created_at, updated_at
		 FROM apps ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	defer rows.Close()

	var apps []*domain.App
	for rows.Next() {
		app := &domain.App{}
		if err := rows.Scan(&app.ID, &app.Name, &app.Source, &app.RepoURL, &app.Branch, &app.InternalPort,
			&app.ImageRef, &app.Domain, &app.FallbackPort, &app.AccessMode, &app.AccessURL,
			&app.Status, &app.CurrentDeployID, &app.CreatedAt, &app.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan app: %w", err)
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (s *SQLiteStore) UpdateApp(ctx context.Context, app *domain.App) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE apps SET name=?, source=?, repo_url=?, branch=?, internal_port=?, image_ref=?, domain=?, fallback_port=?, access_mode=?, access_url=?, status=?, current_deploy_id=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=?`,
		app.Name, app.Source, app.RepoURL, app.Branch, app.InternalPort,
		app.ImageRef, app.Domain, app.FallbackPort, app.AccessMode, app.AccessURL,
		app.Status, app.CurrentDeployID, app.ID)
	if err != nil {
		return fmt.Errorf("update app: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteApp(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	return nil
}

func (s *SQLiteStore) NextFallbackPort(ctx context.Context) (int, error) {
	var port int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(fallback_port), 40000) + 1 FROM apps WHERE fallback_port > 0`,
	).Scan(&port)
	if err != nil {
		return 0, fmt.Errorf("next fallback port: %w", err)
	}
	return port, nil
}
