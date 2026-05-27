package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/logic-roastery/project-talos/internal/domain"

	_ "modernc.org/sqlite"
)

// newTestStore creates an in-memory SQLiteStore for testing.
// The store is automatically closed during test cleanup.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}

	t.Cleanup(func() { s.Close() })

	return s
}

// createTestApp creates an app with sensible defaults for testing.
func createTestApp(t *testing.T, s *SQLiteStore, name string) *domain.App {
	t.Helper()

	app := &domain.App{
		Name:         name,
		Source:       "github",
		RepoURL:      "https://github.com/test/repo",
		Branch:       "main",
		InternalPort: 3000,
		AccessMode:   domain.AccessModePort,
		Status:       domain.AppStatusInactive,
	}

	ctx := context.Background()
	if err := s.CreateApp(ctx, app); err != nil {
		t.Fatalf("createTestApp: %v", err)
	}

	return app
}

// createTestDeploy creates a deploy with sensible defaults for testing.
func createTestDeploy(t *testing.T, s *SQLiteStore, appID int64, status domain.DeployStatus) *domain.Deploy {
	t.Helper()

	d := &domain.Deploy{
		AppID:       appID,
		ImageRef:    "test/image:latest",
		CommitSHA:   "abc123",
		Branch:      "main",
		Status:      status,
		TriggeredBy: "test",
	}

	ctx := context.Background()
	if err := s.CreateDeploy(ctx, d); err != nil {
		t.Fatalf("createTestDeploy: %v", err)
	}

	return d
}
