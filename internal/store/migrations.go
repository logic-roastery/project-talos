package store

import "fmt"

// allMigrations is the full ordered list of schema migrations.
// Each entry is a SQL statement keyed by its version number (1-based).
// Never reorder or modify an existing entry — only append new ones.
var allMigrations = map[int]string{
	1: `CREATE TABLE IF NOT EXISTS users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT    NOT NULL UNIQUE,
		password_hash TEXT    NOT NULL,
		created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	2: `CREATE TABLE IF NOT EXISTS apps (
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
	3: `CREATE UNIQUE INDEX IF NOT EXISTS idx_apps_domain ON apps(domain) WHERE domain != ''`,
	4: `CREATE UNIQUE INDEX IF NOT EXISTS idx_apps_fallback_port ON apps(fallback_port) WHERE fallback_port > 0`,
	5: `CREATE TABLE IF NOT EXISTS deploys (
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
	6: `CREATE INDEX IF NOT EXISTS idx_deploys_app_id ON deploys(app_id)`,
	7: `CREATE TABLE IF NOT EXISTS services (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		name          TEXT    NOT NULL UNIQUE,
		type          TEXT    NOT NULL,
		image_ref     TEXT    NOT NULL,
		status        TEXT    NOT NULL DEFAULT 'pending',
		container_id  TEXT    DEFAULT '',
		app_id        INTEGER REFERENCES apps(id) ON DELETE SET NULL,
		volume_path   TEXT    NOT NULL DEFAULT '',
		credentials   TEXT    NOT NULL DEFAULT '',
		config        TEXT    NOT NULL DEFAULT '{}',
		internal_port INTEGER NOT NULL DEFAULT 0,
		created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	8: `CREATE TABLE IF NOT EXISTS app_services (
		app_id     INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
		service_id INTEGER NOT NULL REFERENCES services(id) ON DELETE CASCADE,
		alias      TEXT    NOT NULL DEFAULT '',
		PRIMARY KEY (app_id, service_id)
	)`,
	9: `CREATE TABLE IF NOT EXISTS app_env_vars (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		app_id    INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
		key       TEXT    NOT NULL,
		value     TEXT    NOT NULL DEFAULT '',
		is_secret INTEGER NOT NULL DEFAULT 0,
		UNIQUE(app_id, key)
	)`,
	10: `ALTER TABLE apps ADD COLUMN github_installation_id INTEGER`,
	11: `ALTER TABLE apps ADD COLUMN github_repo_id INTEGER`,
	12: `ALTER TABLE apps ADD COLUMN registry_url TEXT NOT NULL DEFAULT ''`,
	13: `CREATE TABLE IF NOT EXISTS backups (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		filename   TEXT    NOT NULL,
		size       INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
}

func (s *SQLiteStore) migrate() error {
	// Ensure the tracking table exists.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Determine the highest version already applied.
	var maxApplied int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&maxApplied); err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}

	// If the table is empty but other tables exist, this is a pre-tracking database.
	// Seed all current versions so we don't re-run existing migrations.
	if maxApplied == 0 {
		var hasUsers bool
		if err := s.db.QueryRow(`SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&hasUsers); err != nil {
			return fmt.Errorf("check existing tables: %w", err)
		}
		if hasUsers {
			for v := range allMigrations {
				if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`, v); err != nil {
					return fmt.Errorf("seed version %d: %w", v, err)
				}
			}
			maxApplied = len(allMigrations)
		}
	}

	// Apply any pending migrations in order.
	for v := 1; v <= len(allMigrations); v++ {
		if v <= maxApplied {
			continue
		}
		stmt, ok := allMigrations[v]
		if !ok {
			return fmt.Errorf("migration %d: missing statement", v)
		}
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
		if _, err := s.db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, v); err != nil {
			return fmt.Errorf("record migration %d: %w", v, err)
		}
	}

	return nil
}
