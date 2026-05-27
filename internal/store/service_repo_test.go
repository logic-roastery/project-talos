package store

import (
	"context"
	"errors"
	"testing"

	"github.com/logic-roastery/project-talos/internal/domain"
)

// createTestService is a helper that creates a service with sensible defaults.
func createTestService(t *testing.T, s *SQLiteStore, name string, svcType domain.ServiceType) *domain.Service {
	t.Helper()

	def := domain.ServiceDefinitions[svcType]
	svc := &domain.Service{
		Name:         name,
		Type:         svcType,
		ImageRef:     def.DefaultImage,
		Status:       domain.ServiceStatusPending,
		InternalPort: def.Port,
		VolumePath:   def.VolumePath,
	}

	ctx := context.Background()
	if err := s.CreateService(ctx, svc); err != nil {
		t.Fatalf("create test service %q: %v", name, err)
	}
	return svc
}

func TestCreateAndGetService(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	svc := createTestService(t, s, "my-postgres", domain.ServicePostgres)

	if svc.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}

	got, err := s.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got.Name != svc.Name {
		t.Errorf("name = %q, want %q", got.Name, svc.Name)
	}
	if got.Type != svc.Type {
		t.Errorf("type = %q, want %q", got.Type, svc.Type)
	}
	if got.ImageRef != svc.ImageRef {
		t.Errorf("image_ref = %q, want %q", got.ImageRef, svc.ImageRef)
	}
	if got.InternalPort != svc.InternalPort {
		t.Errorf("internal_port = %d, want %d", got.InternalPort, svc.InternalPort)
	}
}

func TestGetServiceByName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created := createTestService(t, s, "my-redis", domain.ServiceRedis)

	got, err := s.GetServiceByName(ctx, "my-redis")
	if err != nil {
		t.Fatalf("GetServiceByName: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %d, want %d", got.ID, created.ID)
	}
	if got.Name != "my-redis" {
		t.Errorf("name = %q, want %q", got.Name, "my-redis")
	}
}

func TestGetServiceNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetService(ctx, 9999)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestListServices(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	createTestService(t, s, "svc-a", domain.ServicePostgres)
	createTestService(t, s, "svc-b", domain.ServiceRedis)

	services, err := s.ListServices(ctx)
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("len = %d, want 2", len(services))
	}
}

func TestUpdateService(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	svc := createTestService(t, s, "my-mysql", domain.ServiceMySQL)

	svc.Status = domain.ServiceStatusActive
	if err := s.UpdateService(ctx, svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}

	got, err := s.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got.Status != domain.ServiceStatusActive {
		t.Errorf("status = %q, want %q", got.Status, domain.ServiceStatusActive)
	}
}

func TestDeleteService(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	svc := createTestService(t, s, "to-delete", domain.ServiceGarage)

	if err := s.DeleteService(ctx, svc.ID); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}

	_, err := s.GetService(ctx, svc.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestLinkAndListAppServices(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "my-app")
	svc1 := createTestService(t, s, "db", domain.ServicePostgres)
	svc2 := createTestService(t, s, "cache", domain.ServiceRedis)

	if err := s.LinkAppService(ctx, app.ID, svc1.ID, "DATABASE"); err != nil {
		t.Fatalf("LinkAppService (svc1): %v", err)
	}
	if err := s.LinkAppService(ctx, app.ID, svc2.ID, "CACHE"); err != nil {
		t.Fatalf("LinkAppService (svc2): %v", err)
	}

	links, err := s.ListAppServices(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListAppServices: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("len = %d, want 2", len(links))
	}

	aliases := map[int64]string{}
	for _, l := range links {
		aliases[l.ServiceID] = l.Alias
	}
	if aliases[svc1.ID] != "DATABASE" {
		t.Errorf("alias for svc1 = %q, want %q", aliases[svc1.ID], "DATABASE")
	}
	if aliases[svc2.ID] != "CACHE" {
		t.Errorf("alias for svc2 = %q, want %q", aliases[svc2.ID], "CACHE")
	}
}

func TestUnlinkAppService(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "unlink-app")
	svc := createTestService(t, s, "unlink-svc", domain.ServicePostgres)

	if err := s.LinkAppService(ctx, app.ID, svc.ID, "DB"); err != nil {
		t.Fatalf("LinkAppService: %v", err)
	}

	if err := s.UnlinkAppService(ctx, app.ID, svc.ID); err != nil {
		t.Fatalf("UnlinkAppService: %v", err)
	}

	links, err := s.ListAppServices(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListAppServices: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("len = %d, want 0 after unlink", len(links))
	}
}

func TestGetLinkedApps(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app1 := createTestApp(t, s, "app-one")
	app2 := createTestApp(t, s, "app-two")
	svc := createTestService(t, s, "shared-db", domain.ServicePostgres)

	if err := s.LinkAppService(ctx, app1.ID, svc.ID, "DB"); err != nil {
		t.Fatalf("LinkAppService (app1): %v", err)
	}
	if err := s.LinkAppService(ctx, app2.ID, svc.ID, "DB"); err != nil {
		t.Fatalf("LinkAppService (app2): %v", err)
	}

	linked, err := s.GetLinkedApps(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetLinkedApps: %v", err)
	}
	if len(linked) != 2 {
		t.Errorf("len = %d, want 2", len(linked))
	}
}

func TestSetAndGetAppEnvVar(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "env-app")

	if err := s.SetAppEnvVar(ctx, &domain.AppEnvVar{AppID: app.ID, Key: "DATABASE_URL", Value: "postgres://localhost/db"}); err != nil {
		t.Fatalf("SetAppEnvVar (DATABASE_URL): %v", err)
	}
	if err := s.SetAppEnvVar(ctx, &domain.AppEnvVar{AppID: app.ID, Key: "API_KEY", Value: "secret123"}); err != nil {
		t.Fatalf("SetAppEnvVar (API_KEY): %v", err)
	}

	vars, err := s.GetAppEnvVars(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetAppEnvVars: %v", err)
	}
	if len(vars) != 2 {
		t.Fatalf("len = %d, want 2", len(vars))
	}

	// Results should be sorted by key.
	if vars[0].Key != "API_KEY" {
		t.Errorf("vars[0].Key = %q, want %q", vars[0].Key, "API_KEY")
	}
	if vars[0].Value != "secret123" {
		t.Errorf("vars[0].Value = %q, want %q", vars[0].Value, "secret123")
	}
	if vars[1].Key != "DATABASE_URL" {
		t.Errorf("vars[1].Key = %q, want %q", vars[1].Key, "DATABASE_URL")
	}
	if vars[1].Value != "postgres://localhost/db" {
		t.Errorf("vars[1].Value = %q, want %q", vars[1].Value, "postgres://localhost/db")
	}
}

func TestSetAppEnvVarUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "upsert-app")

	if err := s.SetAppEnvVar(ctx, &domain.AppEnvVar{AppID: app.ID, Key: "TOKEN", Value: "old"}); err != nil {
		t.Fatalf("SetAppEnvVar (initial): %v", err)
	}

	if err := s.SetAppEnvVar(ctx, &domain.AppEnvVar{AppID: app.ID, Key: "TOKEN", Value: "new"}); err != nil {
		t.Fatalf("SetAppEnvVar (upsert): %v", err)
	}

	vars, err := s.GetAppEnvVars(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetAppEnvVars: %v", err)
	}
	if len(vars) != 1 {
		t.Fatalf("len = %d, want 1", len(vars))
	}
	if vars[0].Value != "new" {
		t.Errorf("value = %q, want %q (upsert should update)", vars[0].Value, "new")
	}
}

func TestDeleteAppEnvVar(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "delenv-app")

	if err := s.SetAppEnvVar(ctx, &domain.AppEnvVar{AppID: app.ID, Key: "REMOVE_ME", Value: "bye"}); err != nil {
		t.Fatalf("SetAppEnvVar: %v", err)
	}

	if err := s.DeleteAppEnvVar(ctx, app.ID, "REMOVE_ME"); err != nil {
		t.Fatalf("DeleteAppEnvVar: %v", err)
	}

	vars, err := s.GetAppEnvVars(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetAppEnvVars: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("len = %d, want 0 after delete", len(vars))
	}
}
