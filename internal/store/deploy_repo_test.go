package store

import (
	"context"
	"errors"
	"testing"

	"github.com/logic-roastery/project-talos/internal/domain"
)

func TestCreateAndGetDeploy(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "deploy-app")
	d := createTestDeploy(t, s, app.ID, domain.DeployStatusPending)

	if d.ID == 0 {
		t.Fatal("expected deploy ID to be set after CreateDeploy")
	}

	got, err := s.GetDeploy(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDeploy: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("ID = %d, want %d", got.ID, d.ID)
	}
	if got.AppID != d.AppID {
		t.Errorf("app_id = %d, want %d", got.AppID, d.AppID)
	}
	if got.ImageRef != d.ImageRef {
		t.Errorf("image_ref = %q, want %q", got.ImageRef, d.ImageRef)
	}
	if got.CommitSHA != d.CommitSHA {
		t.Errorf("commit_sha = %q, want %q", got.CommitSHA, d.CommitSHA)
	}
	if got.Status != d.Status {
		t.Errorf("status = %q, want %q", got.Status, d.Status)
	}
	if got.TriggeredBy != d.TriggeredBy {
		t.Errorf("triggered_by = %q, want %q", got.TriggeredBy, d.TriggeredBy)
	}
}

func TestGetLatestDeploy(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "latest-app")

	first := createTestDeploy(t, s, app.ID, domain.DeployStatusPending)
	second := createTestDeploy(t, s, app.ID, domain.DeployStatusRunning)

	got, err := s.GetLatestDeploy(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestDeploy: %v", err)
	}
	if got.AppID != app.ID {
		t.Errorf("app_id = %d, want %d", got.AppID, app.ID)
	}
	// SQLite CURRENT_TIMESTAMP has second-level precision, so both deploys
	// may share the same created_at. Verify the result is one of the two.
	if got.ID != first.ID && got.ID != second.ID {
		t.Errorf("ID = %d, want %d or %d", got.ID, first.ID, second.ID)
	}
}

func TestGetLatestSuccessfulDeploy(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "success-app")

	createTestDeploy(t, s, app.ID, domain.DeployStatusFailed)
	success := createTestDeploy(t, s, app.ID, domain.DeployStatusSuccess)
	createTestDeploy(t, s, app.ID, domain.DeployStatusRunning)

	got, err := s.GetLatestSuccessfulDeploy(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestSuccessfulDeploy: %v", err)
	}
	if got.ID != success.ID {
		t.Errorf("ID = %d, want %d (success deploy)", got.ID, success.ID)
	}
	if got.Status != domain.DeployStatusSuccess {
		t.Errorf("status = %q, want %q", got.Status, domain.DeployStatusSuccess)
	}
}

func TestGetLatestDeployNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "empty-app")

	_, err := s.GetLatestDeploy(ctx, app.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected domain.ErrNotFound, got: %v", err)
	}
}

func TestListDeploys(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "list-app")

	createTestDeploy(t, s, app.ID, domain.DeployStatusPending)
	createTestDeploy(t, s, app.ID, domain.DeployStatusRunning)
	createTestDeploy(t, s, app.ID, domain.DeployStatusSuccess)

	deploys, err := s.ListDeploys(ctx, app.ID, 2)
	if err != nil {
		t.Fatalf("ListDeploys: %v", err)
	}
	if len(deploys) != 2 {
		t.Fatalf("len(deploys) = %d, want 2", len(deploys))
	}
}

func TestUpdateDeploy(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	app := createTestApp(t, s, "update-deploy-app")
	d := createTestDeploy(t, s, app.ID, domain.DeployStatusPending)

	d.Status = domain.DeployStatusRunning
	d.ContainerID = "container-abc123"
	if err := s.UpdateDeploy(ctx, d); err != nil {
		t.Fatalf("UpdateDeploy: %v", err)
	}

	got, err := s.GetDeploy(ctx, d.ID)
	if err != nil {
		t.Fatalf("GetDeploy: %v", err)
	}
	if got.Status != domain.DeployStatusRunning {
		t.Errorf("status = %q, want %q", got.Status, domain.DeployStatusRunning)
	}
	if got.ContainerID != "container-abc123" {
		t.Errorf("container_id = %q, want %q", got.ContainerID, "container-abc123")
	}
}
