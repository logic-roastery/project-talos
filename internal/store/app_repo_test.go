package store

import (
	"context"
	"errors"
	"testing"

	"github.com/logic-roastery/project-talos/internal/domain"
)

func TestCreateAndGetApp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "my-app")

	if app.ID == 0 {
		t.Fatal("expected app ID to be set after CreateApp")
	}

	got, err := s.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Name != app.Name {
		t.Errorf("name = %q, want %q", got.Name, app.Name)
	}
	if got.Source != app.Source {
		t.Errorf("source = %q, want %q", got.Source, app.Source)
	}
	if got.RepoURL != app.RepoURL {
		t.Errorf("repo_url = %q, want %q", got.RepoURL, app.RepoURL)
	}
	if got.Branch != app.Branch {
		t.Errorf("branch = %q, want %q", got.Branch, app.Branch)
	}
	if got.InternalPort != app.InternalPort {
		t.Errorf("internal_port = %d, want %d", got.InternalPort, app.InternalPort)
	}
	if got.AccessMode != app.AccessMode {
		t.Errorf("access_mode = %q, want %q", got.AccessMode, app.AccessMode)
	}
	if got.Status != app.Status {
		t.Errorf("status = %q, want %q", got.Status, app.Status)
	}
}

func TestGetAppByName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "lookup-by-name")

	got, err := s.GetAppByName(ctx, "lookup-by-name")
	if err != nil {
		t.Fatalf("GetAppByName: %v", err)
	}
	if got.ID != app.ID {
		t.Errorf("ID = %d, want %d", got.ID, app.ID)
	}
	if got.Name != app.Name {
		t.Errorf("name = %q, want %q", got.Name, app.Name)
	}
}

func TestGetAppByDomain(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := &domain.App{
		Name:         "domain-app",
		Source:       "github",
		RepoURL:      "https://github.com/test/repo",
		Branch:       "main",
		InternalPort: 3000,
		Domain:       "app.example.com",
		AccessMode:   domain.AccessModeDomain,
		Status:       domain.AppStatusInactive,
	}
	if err := s.CreateApp(ctx, app); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	got, err := s.GetAppByDomain(ctx, "app.example.com")
	if err != nil {
		t.Fatalf("GetAppByDomain: %v", err)
	}
	if got.ID != app.ID {
		t.Errorf("ID = %d, want %d", got.ID, app.ID)
	}
	if got.Domain != app.Domain {
		t.Errorf("domain = %q, want %q", got.Domain, app.Domain)
	}
}

func TestGetAppNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetApp(ctx, 9999)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected domain.ErrNotFound, got: %v", err)
	}
}

func TestListApps(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	createTestApp(t, s, "app-one")
	createTestApp(t, s, "app-two")

	apps, err := s.ListApps(ctx)
	if err != nil {
		t.Fatalf("ListApps: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("len(apps) = %d, want 2", len(apps))
	}

	names := map[string]bool{}
	for _, a := range apps {
		names[a.Name] = true
	}
	if !names["app-one"] {
		t.Error("expected app-one in list")
	}
	if !names["app-two"] {
		t.Error("expected app-two in list")
	}
}

func TestUpdateApp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "original-name")

	app.Name = "updated-name"
	if err := s.UpdateApp(ctx, app); err != nil {
		t.Fatalf("UpdateApp: %v", err)
	}

	got, err := s.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Name != "updated-name" {
		t.Errorf("name = %q, want %q", got.Name, "updated-name")
	}
}

func TestDeleteApp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "doomed-app")

	if err := s.DeleteApp(ctx, app.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}

	_, err := s.GetApp(ctx, app.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected domain.ErrNotFound after delete, got: %v", err)
	}
}

func TestNextFallbackPort(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	port, err := s.NextFallbackPort(ctx)
	if err != nil {
		t.Fatalf("NextFallbackPort (empty): %v", err)
	}
	if port != 40001 {
		t.Errorf("port = %d, want 40001", port)
	}

	app := &domain.App{
		Name:         "port-app",
		Source:       "github",
		RepoURL:      "https://github.com/test/repo",
		Branch:       "main",
		InternalPort: 3000,
		FallbackPort: 40005,
		AccessMode:   domain.AccessModePort,
		Status:       domain.AppStatusInactive,
	}
	if err := s.CreateApp(ctx, app); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	port, err = s.NextFallbackPort(ctx)
	if err != nil {
		t.Fatalf("NextFallbackPort (after insert): %v", err)
	}
	if port != 40006 {
		t.Errorf("port = %d, want 40006", port)
	}
}
