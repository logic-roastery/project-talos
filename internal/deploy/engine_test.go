package deploy

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/logic-roastery/project-talos/internal/domain"
)

func TestDeployQueuesTalosBuildAndFailsInBackground(t *testing.T) {
	t.Parallel()

	app := &domain.App{
		ID:           42,
		Name:         "demo",
		AppType:      domain.AppTypeManaged,
		BuildMode:    domain.BuildModeTalosBuild,
		RepoURL:      "https://github.com/acme/demo",
		Branch:       "main",
		InternalPort: 9001,
	}

	apps := &testAppStore{app: app}
	deploys := newTestDeployStore()
	engine := NewEngine(apps, deploys, testServiceStore{}, nil, nil, nil, nil, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)))

	deploy, err := engine.Deploy(context.Background(), app.ID, "", "", app.Branch, "manual")
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}

	if deploy.Status != domain.DeployStatusPending {
		t.Fatalf("initial deploy status = %q, want %q", deploy.Status, domain.DeployStatusPending)
	}
	if deploy.ID == 0 {
		t.Fatalf("expected deploy ID to be assigned")
	}

	stored, err := deploys.GetDeploy(context.Background(), deploy.ID)
	if err != nil {
		t.Fatalf("GetDeploy() error = %v", err)
	}
	if stored.Status != domain.DeployStatusPending {
		t.Fatalf("stored deploy status = %q, want %q", stored.Status, domain.DeployStatusPending)
	}

	waitFor(t, 2*time.Second, func() bool {
		got, err := deploys.GetDeploy(context.Background(), deploy.ID)
		return err == nil && got.Status == domain.DeployStatusFailed
	})

	events, err := deploys.ListDeployEvents(context.Background(), deploy.ID)
	if err != nil {
		t.Fatalf("ListDeployEvents() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected deploy events to be recorded")
	}

	var sawQueue bool
	var sawFinalizeFailure bool
	for _, event := range events {
		if event.Step == "queue" && event.Level == "info" {
			sawQueue = true
		}
		if event.Step == "finalize" && event.Level == "error" {
			sawFinalizeFailure = true
		}
	}
	if !sawQueue {
		t.Fatalf("expected queue event, got %#v", events)
	}
	if !sawFinalizeFailure {
		t.Fatalf("expected finalize failure event, got %#v", events)
	}
}

func TestDeployRejectsPendingDeployAsInProgress(t *testing.T) {
	t.Parallel()

	app := &domain.App{
		ID:        7,
		Name:      "demo",
		AppType:   domain.AppTypeManaged,
		BuildMode: domain.BuildModeTalosBuild,
		RepoURL:   "https://github.com/acme/demo",
		Branch:    "main",
	}

	apps := &testAppStore{app: app}
	deploys := newTestDeployStore()
	if err := deploys.CreateDeploy(context.Background(), &domain.Deploy{
		AppID:       app.ID,
		Branch:      app.Branch,
		Status:      domain.DeployStatusPending,
		TriggeredBy: "test",
	}); err != nil {
		t.Fatalf("CreateDeploy() error = %v", err)
	}

	engine := NewEngine(apps, deploys, testServiceStore{}, nil, nil, nil, nil, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)))

	_, err := engine.Deploy(context.Background(), app.ID, "", "", app.Branch, "manual")
	if err != domain.ErrDeployInProgress {
		t.Fatalf("Deploy() error = %v, want %v", err, domain.ErrDeployInProgress)
	}
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

type testAppStore struct {
	app *domain.App
}

func (s *testAppStore) CreateApp(context.Context, *domain.App) error { panic("unexpected call") }
func (s *testAppStore) GetApp(_ context.Context, id int64) (*domain.App, error) {
	if s.app != nil && s.app.ID == id {
		appCopy := *s.app
		return &appCopy, nil
	}
	return nil, domain.ErrNotFound
}
func (s *testAppStore) GetAppByName(context.Context, string) (*domain.App, error) {
	panic("unexpected call")
}
func (s *testAppStore) GetAppByDomain(context.Context, string) (*domain.App, error) {
	panic("unexpected call")
}
func (s *testAppStore) GetAppByInstallationAndRepo(context.Context, int64, int64) (*domain.App, error) {
	panic("unexpected call")
}
func (s *testAppStore) GetAppByGitHubRepoID(context.Context, int64) (*domain.App, error) {
	panic("unexpected call")
}
func (s *testAppStore) ListApps(context.Context) ([]*domain.App, error) { panic("unexpected call") }
func (s *testAppStore) UpdateApp(context.Context, *domain.App) error    { return nil }
func (s *testAppStore) DeleteApp(context.Context, int64) error          { panic("unexpected call") }
func (s *testAppStore) NextFallbackPort(context.Context) (int, error)   { panic("unexpected call") }

type testDeployStore struct {
	mu      sync.Mutex
	nextID  int64
	deploys map[int64]*domain.Deploy
	events  map[int64][]*domain.DeployEvent
}

func newTestDeployStore() *testDeployStore {
	return &testDeployStore{
		nextID:  1,
		deploys: make(map[int64]*domain.Deploy),
		events:  make(map[int64][]*domain.DeployEvent),
	}
}

func (s *testDeployStore) CreateDeploy(_ context.Context, deploy *domain.Deploy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copy := *deploy
	copy.ID = s.nextID
	s.nextID++
	copy.CreatedAt = time.Now()
	deploy.ID = copy.ID
	deploy.CreatedAt = copy.CreatedAt
	s.deploys[copy.ID] = &copy
	return nil
}

func (s *testDeployStore) GetDeploy(_ context.Context, id int64) (*domain.Deploy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deploy, ok := s.deploys[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *deploy
	return &copy, nil
}

func (s *testDeployStore) GetLatestDeploy(_ context.Context, appID int64) (*domain.Deploy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var latest *domain.Deploy
	for _, deploy := range s.deploys {
		if deploy.AppID != appID {
			continue
		}
		if latest == nil || deploy.ID > latest.ID {
			copy := *deploy
			latest = &copy
		}
	}
	if latest == nil {
		return nil, domain.ErrNotFound
	}
	return latest, nil
}

func (s *testDeployStore) GetLatestSuccessfulDeploy(_ context.Context, appID int64) (*domain.Deploy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var latest *domain.Deploy
	for _, deploy := range s.deploys {
		if deploy.AppID != appID || deploy.Status != domain.DeployStatusSuccess {
			continue
		}
		if latest == nil || deploy.ID > latest.ID {
			copy := *deploy
			latest = &copy
		}
	}
	if latest == nil {
		return nil, domain.ErrNotFound
	}
	return latest, nil
}

func (s *testDeployStore) ListDeploys(_ context.Context, appID int64, limit int) ([]*domain.Deploy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var items []*domain.Deploy
	for _, deploy := range s.deploys {
		if deploy.AppID != appID {
			continue
		}
		copy := *deploy
		items = append(items, &copy)
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (s *testDeployStore) UpdateDeploy(_ context.Context, deploy *domain.Deploy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copy := *deploy
	s.deploys[deploy.ID] = &copy
	return nil
}

func (s *testDeployStore) CreateDeployEvent(_ context.Context, event *domain.DeployEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copy := *event
	copy.Timestamp = time.Now()
	s.events[event.DeployID] = append(s.events[event.DeployID], &copy)
	return nil
}

func (s *testDeployStore) ListDeployEvents(_ context.Context, deployID int64) ([]*domain.DeployEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := s.events[deployID]
	items := make([]*domain.DeployEvent, 0, len(events))
	for _, event := range events {
		copy := *event
		items = append(items, &copy)
	}
	return items, nil
}

type testServiceStore struct{}

func (testServiceStore) CreateService(context.Context, *domain.Service) error {
	panic("unexpected call")
}
func (testServiceStore) GetService(context.Context, int64) (*domain.Service, error) {
	panic("unexpected call")
}
func (testServiceStore) GetServiceByName(context.Context, string) (*domain.Service, error) {
	panic("unexpected call")
}
func (testServiceStore) ListServices(context.Context) ([]*domain.Service, error) {
	panic("unexpected call")
}
func (testServiceStore) UpdateService(context.Context, *domain.Service) error {
	panic("unexpected call")
}
func (testServiceStore) DeleteService(context.Context, int64) error { panic("unexpected call") }
func (testServiceStore) LinkAppService(context.Context, int64, int64, string) error {
	panic("unexpected call")
}
func (testServiceStore) UnlinkAppService(context.Context, int64, int64) error {
	panic("unexpected call")
}
func (testServiceStore) ListAppServices(context.Context, int64) ([]*domain.AppService, error) {
	return nil, nil
}
func (testServiceStore) GetLinkedApps(context.Context, int64) ([]*domain.AppService, error) {
	panic("unexpected call")
}
func (testServiceStore) SetAppEnvVar(context.Context, *domain.AppEnvVar) error {
	panic("unexpected call")
}
func (testServiceStore) GetAppEnvVars(context.Context, int64) ([]*domain.AppEnvVar, error) {
	return nil, nil
}
func (testServiceStore) DeleteAppEnvVar(context.Context, int64, string) error {
	panic("unexpected call")
}
func (testServiceStore) GetAppEnvVarHistory(context.Context, int64, string) ([]*domain.AppEnvVarHistory, error) {
	panic("unexpected call")
}
func (testServiceStore) GetAppEnvVarsSnapshot(context.Context, int64) (map[string]string, error) {
	return map[string]string{}, nil
}
