package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/proxy/traefik"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/store"
)

type Engine struct {
	apps    store.AppStore
	deploys store.DeployStore
	docker  *docker.Client
	proxy   *traefik.Manager
	logger  *slog.Logger
}

func NewEngine(apps store.AppStore, deploys store.DeployStore, docker *docker.Client, proxy *traefik.Manager, logger *slog.Logger) *Engine {
	return &Engine{
		apps:    apps,
		deploys: deploys,
		docker:  docker,
		proxy:   proxy,
		logger:  logger,
	}
}

func (e *Engine) Deploy(ctx context.Context, appID int64, imageRef, commitSHA, branch, triggeredBy string) (*domain.Deploy, error) {
	app, err := e.apps.GetApp(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	latest, err := e.deploys.GetLatestDeploy(ctx, appID)
	if err != nil && err != domain.ErrNotFound {
		return nil, fmt.Errorf("check running deploy: %w", err)
	}
	if latest != nil && latest.Status == domain.DeployStatusRunning {
		return nil, domain.ErrDeployInProgress
	}

	d := &domain.Deploy{
		AppID:       appID,
		ImageRef:    imageRef,
		CommitSHA:   commitSHA,
		Branch:      branch,
		Status:      domain.DeployStatusPending,
		TriggeredBy: triggeredBy,
	}
	if err := e.deploys.CreateDeploy(ctx, d); err != nil {
		return nil, fmt.Errorf("create deploy: %w", err)
	}

	go e.execute(context.Background(), app, d)

	return d, nil
}

func (e *Engine) Rollback(ctx context.Context, appID int64) (*domain.Deploy, error) {
	app, err := e.apps.GetApp(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	prev, err := e.deploys.GetLatestSuccessfulDeploy(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("get last successful deploy: %w", err)
	}

	d := &domain.Deploy{
		AppID:        appID,
		ImageRef:     prev.ImageRef,
		Branch:       prev.Branch,
		Status:       domain.DeployStatusPending,
		TriggeredBy:  "rollback",
		RollbackOfID: &prev.ID,
	}
	if err := e.deploys.CreateDeploy(ctx, d); err != nil {
		return nil, fmt.Errorf("create rollback deploy: %w", err)
	}

	go e.execute(context.Background(), app, d)

	return d, nil
}

func (e *Engine) execute(ctx context.Context, app *domain.App, d *domain.Deploy) {
	now := time.Now()
	d.StartedAt = &now
	d.Status = domain.DeployStatusRunning
	e.deploys.UpdateDeploy(ctx, d)

	e.logger.Info("starting deploy", "deploy_id", d.ID, "app", app.Name, "image", d.ImageRef)

	if err := e.docker.PullImage(ctx, d.ImageRef); err != nil {
		e.failDeploy(ctx, d, fmt.Sprintf("pull image: %v", err))
		return
	}

	containerName := fmt.Sprintf("talos-%s", app.Name)
	if err := e.docker.StopAndRemove(ctx, containerName); err != nil {
		e.logger.Warn("stop old container", "error", err)
	}

	containerID, err := e.docker.StartContainer(ctx, containerName, d.ImageRef, app.InternalPort)
	if err != nil {
		e.failDeploy(ctx, d, fmt.Sprintf("start container: %v", err))
		return
	}
	d.ContainerID = containerID

	if err := e.docker.WaitForHealth(ctx, containerID, 30*time.Second); err != nil {
		d.HealthStatus = "unhealthy"
		e.failDeploy(ctx, d, fmt.Sprintf("health check: %v", err))
		return
	}
	d.HealthStatus = "healthy"

	if err := e.proxy.UpdateRoute(ctx, app, containerName); err != nil {
		e.failDeploy(ctx, d, fmt.Sprintf("update route: %v", err))
		return
	}

	completed := time.Now()
	d.CompletedAt = &completed
	d.Status = domain.DeployStatusSuccess
	if err := e.deploys.UpdateDeploy(ctx, d); err != nil {
		e.logger.Error("update deploy", "error", err)
	}

	app.Status = domain.AppStatusActive
	app.ImageRef = d.ImageRef
	app.CurrentDeployID = &d.ID
	if err := e.apps.UpdateApp(ctx, app); err != nil {
		e.logger.Error("update app", "error", err)
	}

	e.logger.Info("deploy completed", "deploy_id", d.ID, "app", app.Name)
}

func (e *Engine) failDeploy(ctx context.Context, d *domain.Deploy, reason string) {
	completed := time.Now()
	d.CompletedAt = &completed
	d.Status = domain.DeployStatusFailed
	d.Logs = reason

	if err := e.deploys.UpdateDeploy(ctx, d); err != nil {
		e.logger.Error("update failed deploy", "error", err)
	}

	e.logger.Error("deploy failed", "deploy_id", d.ID, "reason", reason)
}
