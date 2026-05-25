package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/logic-roastery/project-talos/internal/domain"
)

const serviceColumns = `id, name, type, image_ref, status, container_id, app_id, volume_path, credentials, config, internal_port, created_at, updated_at`

func serviceScanFields(svc *domain.Service) []interface{} {
	return []interface{}{
		&svc.ID, &svc.Name, &svc.Type, &svc.ImageRef, &svc.Status, &svc.ContainerID,
		&svc.AppID, &svc.VolumePath, &svc.Credentials, &svc.Config, &svc.InternalPort,
		&svc.CreatedAt, &svc.UpdatedAt,
	}
}

func (s *SQLiteStore) CreateService(ctx context.Context, svc *domain.Service) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO services (name, type, image_ref, status, app_id, volume_path, credentials, config, internal_port)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		svc.Name, svc.Type, svc.ImageRef, svc.Status, svc.AppID,
		svc.VolumePath, svc.Credentials, svc.Config, svc.InternalPort,
	)
	if err != nil {
		return fmt.Errorf("insert service: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("get service id: %w", err)
	}
	svc.ID = id
	return nil
}

func (s *SQLiteStore) GetService(ctx context.Context, id int64) (*domain.Service, error) {
	svc := &domain.Service{}
	err := s.db.QueryRowContext(ctx,
		`SELECT `+serviceColumns+` FROM services WHERE id = ?`, id,
	).Scan(serviceScanFields(svc)...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get service: %w", err)
	}
	return svc, nil
}

func (s *SQLiteStore) GetServiceByName(ctx context.Context, name string) (*domain.Service, error) {
	svc := &domain.Service{}
	err := s.db.QueryRowContext(ctx,
		`SELECT `+serviceColumns+` FROM services WHERE name = ?`, name,
	).Scan(serviceScanFields(svc)...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get service by name: %w", err)
	}
	return svc, nil
}

func (s *SQLiteStore) ListServices(ctx context.Context) ([]*domain.Service, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+serviceColumns+` FROM services ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	var services []*domain.Service
	for rows.Next() {
		svc := &domain.Service{}
		if err := rows.Scan(serviceScanFields(svc)...); err != nil {
			return nil, fmt.Errorf("scan service: %w", err)
		}
		services = append(services, svc)
	}
	return services, rows.Err()
}

func (s *SQLiteStore) UpdateService(ctx context.Context, svc *domain.Service) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE services SET name=?, type=?, image_ref=?, status=?, container_id=?, app_id=?, volume_path=?, credentials=?, config=?, internal_port=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=?`,
		svc.Name, svc.Type, svc.ImageRef, svc.Status, svc.ContainerID,
		svc.AppID, svc.VolumePath, svc.Credentials, svc.Config, svc.InternalPort, svc.ID,
	)
	if err != nil {
		return fmt.Errorf("update service: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteService(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM services WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

// App-Service linking

func (s *SQLiteStore) LinkAppService(ctx context.Context, appID, serviceID int64, alias string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO app_services (app_id, service_id, alias) VALUES (?, ?, ?)`,
		appID, serviceID, alias,
	)
	if err != nil {
		return fmt.Errorf("link app service: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UnlinkAppService(ctx context.Context, appID, serviceID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM app_services WHERE app_id = ? AND service_id = ?`,
		appID, serviceID,
	)
	if err != nil {
		return fmt.Errorf("unlink app service: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListAppServices(ctx context.Context, appID int64) ([]*domain.AppService, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT app_id, service_id, alias FROM app_services WHERE app_id = ?`, appID,
	)
	if err != nil {
		return nil, fmt.Errorf("list app services: %w", err)
	}
	defer rows.Close()

	var links []*domain.AppService
	for rows.Next() {
		link := &domain.AppService{}
		if err := rows.Scan(&link.AppID, &link.ServiceID, &link.Alias); err != nil {
			return nil, fmt.Errorf("scan app service: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *SQLiteStore) GetLinkedApps(ctx context.Context, serviceID int64) ([]*domain.AppService, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT app_id, service_id, alias FROM app_services WHERE service_id = ?`, serviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get linked apps: %w", err)
	}
	defer rows.Close()

	var links []*domain.AppService
	for rows.Next() {
		link := &domain.AppService{}
		if err := rows.Scan(&link.AppID, &link.ServiceID, &link.Alias); err != nil {
			return nil, fmt.Errorf("scan linked app: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// App environment variables

func (s *SQLiteStore) SetAppEnvVar(ctx context.Context, envVar *domain.AppEnvVar) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO app_env_vars (app_id, key, value, is_secret) VALUES (?, ?, ?, ?)
		 ON CONFLICT(app_id, key) DO UPDATE SET value=excluded.value, is_secret=excluded.is_secret`,
		envVar.AppID, envVar.Key, envVar.Value, envVar.IsSecret,
	)
	if err != nil {
		return fmt.Errorf("set env var: %w", err)
	}
	if envVar.ID == 0 {
		id, err := res.LastInsertId()
		if err == nil {
			envVar.ID = id
		}
	}
	return nil
}

func (s *SQLiteStore) GetAppEnvVars(ctx context.Context, appID int64) ([]*domain.AppEnvVar, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, app_id, key, value, is_secret FROM app_env_vars WHERE app_id = ? ORDER BY key`, appID,
	)
	if err != nil {
		return nil, fmt.Errorf("get env vars: %w", err)
	}
	defer rows.Close()

	var vars []*domain.AppEnvVar
	for rows.Next() {
		v := &domain.AppEnvVar{}
		if err := rows.Scan(&v.ID, &v.AppID, &v.Key, &v.Value, &v.IsSecret); err != nil {
			return nil, fmt.Errorf("scan env var: %w", err)
		}
		vars = append(vars, v)
	}
	return vars, rows.Err()
}

func (s *SQLiteStore) DeleteAppEnvVar(ctx context.Context, appID int64, key string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM app_env_vars WHERE app_id = ? AND key = ?`, appID, key,
	)
	if err != nil {
		return fmt.Errorf("delete env var: %w", err)
	}
	return nil
}
