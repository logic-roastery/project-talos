package store

import "fmt"

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT    NOT NULL UNIQUE,
		password_hash TEXT    NOT NULL,
		created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS apps (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		name             TEXT    NOT NULL UNIQUE,
		source           TEXT    NOT NULL DEFAULT 'github',
		repo_url         TEXT    NOT NULL,
		branch           TEXT    NOT NULL DEFAULT 'main',
		internal_port    INTEGER NOT NULL DEFAULT 3000,
		image_ref        TEXT    NOT NULL DEFAULT '',
		domain           TEXT    DEFAULT '',
		fallback_port    INTEGER DEFAULT 0,
		access_mode      TEXT    NOT NULL DEFAULT 'port',
		access_url       TEXT    NOT NULL DEFAULT '',
		status           TEXT    NOT NULL DEFAULT 'inactive',
		current_deploy_id INTEGER REFERENCES deploys(id),
		created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_apps_domain ON apps(domain) WHERE domain != ''`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_apps_fallback_port ON apps(fallback_port) WHERE fallback_port > 0`,
	`CREATE TABLE IF NOT EXISTS deploys (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		app_id         INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
		image_ref      TEXT    NOT NULL,
		commit_sha     TEXT    DEFAULT '',
		branch         TEXT    NOT NULL,
		status         TEXT    NOT NULL DEFAULT 'pending',
		container_id   TEXT    DEFAULT '',
		health_status  TEXT    DEFAULT '',
		logs           TEXT    DEFAULT '',
		started_at     DATETIME,
		completed_at   DATETIME,
		triggered_by   TEXT    NOT NULL DEFAULT 'webhook',
		rollback_of_id INTEGER REFERENCES deploys(id),
		created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_deploys_app_id ON deploys(app_id)`,
}

func (s *SQLiteStore) migrate() error {
	for i, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i, err)
		}
	}
	return nil
}
