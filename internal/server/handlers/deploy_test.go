package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	deploypkg "github.com/logic-roastery/project-talos/internal/deploy"
	"github.com/logic-roastery/project-talos/internal/domain"
)

func TestDeployHandlerStreamEventsBackfillsAndStreamsNewEvents(t *testing.T) {
	t.Parallel()

	store := newHandlerDeployStore()
	if err := store.CreateDeploy(context.Background(), &domain.Deploy{
		AppID:       1,
		Branch:      "main",
		Status:      domain.DeployStatusRunning,
		TriggeredBy: "test",
	}); err != nil {
		t.Fatalf("CreateDeploy() error = %v", err)
	}
	deployID := int64(1)
	if err := store.CreateDeployEvent(context.Background(), &domain.DeployEvent{
		DeployID: deployID,
		Level:    "info",
		Step:     "queue",
		Message:  "deploy queued",
	}); err != nil {
		t.Fatalf("CreateDeployEvent() error = %v", err)
	}

	broadcaster := deploypkg.NewEventBroadcaster()
	handler := &DeployHandler{
		deploys: store,
		events:  broadcaster,
		engine:  deploypkg.NewEngine(nil, store, nil, nil, nil, nil, nil, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil))),
	}

	req := deployRequest(t, http.MethodGet, "/api/deploys/1/events/stream", 1)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.StreamEvents(rec, req)
		close(done)
	}()

	waitForCondition(t, time.Second, func() bool {
		return strings.Contains(rec.Body.String(), `"deploy queued"`) && strings.Contains(rec.Body.String(), `"state":"connected"`)
	})

	store.setStatus(deployID, domain.DeployStatusSuccess)
	liveEvent := &domain.DeployEvent{
		ID:        2,
		DeployID:  deployID,
		Timestamp: time.Now(),
		Level:     "info",
		Step:      "finalize",
		Message:   "deploy completed successfully",
	}
	broadcaster.Publish(liveEvent)

	waitForCondition(t, time.Second, func() bool {
		body := rec.Body.String()
		return strings.Contains(body, `"deploy completed successfully"`) && strings.Contains(body, `"state":"terminal"`) && strings.Contains(body, `"deploy_status":"success"`)
	})

	cancel()
	<-done
}

func TestDeployHandlerStreamEventsCleansUpSubscriberOnDisconnect(t *testing.T) {
	t.Parallel()

	store := newHandlerDeployStore()
	if err := store.CreateDeploy(context.Background(), &domain.Deploy{
		AppID:       1,
		Branch:      "main",
		Status:      domain.DeployStatusRunning,
		TriggeredBy: "test",
	}); err != nil {
		t.Fatalf("CreateDeploy() error = %v", err)
	}

	broadcaster := deploypkg.NewEventBroadcaster()
	handler := &DeployHandler{deploys: store, events: broadcaster}

	req := deployRequest(t, http.MethodGet, "/api/deploys/1/events/stream", 1)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.StreamEvents(rec, req)
		close(done)
	}()

	waitForCondition(t, time.Second, func() bool {
		return broadcaster.SubscriberCount(1) == 1
	})

	cancel()
	<-done

	waitForCondition(t, time.Second, func() bool {
		return broadcaster.SubscriberCount(1) == 0
	})
}

func TestDeployHandlerStreamEventsReturnsTerminalStatusWithoutSubscription(t *testing.T) {
	t.Parallel()

	store := newHandlerDeployStore()
	if err := store.CreateDeploy(context.Background(), &domain.Deploy{
		AppID:       1,
		Branch:      "main",
		Status:      domain.DeployStatusFailed,
		TriggeredBy: "test",
	}); err != nil {
		t.Fatalf("CreateDeploy() error = %v", err)
	}

	handler := &DeployHandler{
		deploys: store,
		events:  deploypkg.NewEventBroadcaster(),
	}

	req := deployRequest(t, http.MethodGet, "/api/deploys/1/events/stream", 1)
	rec := httptest.NewRecorder()
	handler.StreamEvents(rec, req)

	events := parseSSE(t, rec.Body.String())
	if len(events["status"]) < 2 {
		t.Fatalf("expected connected and terminal status events, got %#v", events["status"])
	}

	last := map[string]string{}
	if err := json.Unmarshal([]byte(events["status"][len(events["status"])-1]), &last); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if last["state"] != "terminal" || last["deploy_status"] != "failed" {
		t.Fatalf("terminal status = %#v, want terminal failed", last)
	}
	if count := handler.events.SubscriberCount(1); count != 0 {
		t.Fatalf("SubscriberCount() = %d, want 0", count)
	}
}

type handlerDeployStore struct {
	mu      sync.Mutex
	nextID  int64
	deploys map[int64]*domain.Deploy
	events  map[int64][]*domain.DeployEvent
}

func newHandlerDeployStore() *handlerDeployStore {
	return &handlerDeployStore{
		nextID:  1,
		deploys: make(map[int64]*domain.Deploy),
		events:  make(map[int64][]*domain.DeployEvent),
	}
}

func (s *handlerDeployStore) CreateDeploy(_ context.Context, deploy *domain.Deploy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copy := *deploy
	copy.ID = s.nextID
	copy.CreatedAt = time.Now()
	deploy.ID = copy.ID
	deploy.CreatedAt = copy.CreatedAt
	s.nextID++
	s.deploys[copy.ID] = &copy
	return nil
}

func (s *handlerDeployStore) GetDeploy(_ context.Context, id int64) (*domain.Deploy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deploy, ok := s.deploys[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *deploy
	return &copy, nil
}

func (s *handlerDeployStore) GetLatestDeploy(context.Context, int64) (*domain.Deploy, error) {
	panic("unexpected call")
}

func (s *handlerDeployStore) GetLatestSuccessfulDeploy(context.Context, int64) (*domain.Deploy, error) {
	panic("unexpected call")
}

func (s *handlerDeployStore) ListDeploys(context.Context, int64, int) ([]*domain.Deploy, error) {
	panic("unexpected call")
}

func (s *handlerDeployStore) UpdateDeploy(_ context.Context, deploy *domain.Deploy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copy := *deploy
	s.deploys[deploy.ID] = &copy
	return nil
}

func (s *handlerDeployStore) CreateDeployEvent(_ context.Context, event *domain.DeployEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	copy := *event
	copy.ID = int64(len(s.events[event.DeployID]) + 1)
	if copy.Timestamp.IsZero() {
		copy.Timestamp = time.Now()
	}
	s.events[event.DeployID] = append(s.events[event.DeployID], &copy)
	return nil
}

func (s *handlerDeployStore) ListDeployEvents(_ context.Context, deployID int64) ([]*domain.DeployEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := s.events[deployID]
	out := make([]*domain.DeployEvent, 0, len(events))
	for _, event := range events {
		copy := *event
		out = append(out, &copy)
	}
	return out, nil
}

func (s *handlerDeployStore) setStatus(id int64, status domain.DeployStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deploy := *s.deploys[id]
	deploy.Status = status
	s.deploys[id] = &deploy
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
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

func parseSSE(t *testing.T, body string) map[string][]string {
	t.Helper()

	events := make(map[string][]string)
	scanner := bufio.NewScanner(strings.NewReader(body))
	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			events[currentEvent] = append(events[currentEvent], strings.TrimPrefix(line, "data: "))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner.Err() = %v", err)
	}
	return events
}

func deployRequest(t *testing.T, method, target string, deployID int64) *http.Request {
	t.Helper()

	req := httptest.NewRequest(method, target, nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("deployID", strconv.FormatInt(deployID, 10))
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}
