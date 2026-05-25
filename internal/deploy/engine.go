package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/proxy/traefik"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/services"
	"github.com/logic-roastery/project-talos/internal/store"
)

type Engine struct {
	apps        store.AppStore
	deploys     store.DeployStore
	services    store.ServiceStore
	provisioner *services.Provisioner
	docker      *docker.Client
	proxy       *traefik.Manager
	logger      *slog.Logger
}

func NewEngine(apps store.AppStore, deploys store.DeployStore, services store.ServiceStore, provisioner *services.Provisioner, docker *docker.Client, proxy *traefik.Manager, logger *slog.Logger) *Engine {
	return &Engine{
		apps:        apps,
		deploys:     deploys,
		services:    services,
		provisioner: provisioner,
		docker:      docker,
		proxy:       proxy,
		logger:      logger,
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

	// Collect env vars from linked services and app env vars
	envVars := e.collectEnvVars(ctx, app)

	cfg := docker.ContainerConfig{
		Name:         containerName,
		ImageRef:     d.ImageRef,
		InternalPort: app.InternalPort,
		Env:          envVars,
		Labels: map[string]string{
			"managed-by": "talos",
			"talos-app":  app.Name,
		},
	}

	containerID, err := e.docker.StartContainerWithConfig(ctx, cfg)
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

// collectEnvVars gathers env vars from linked services and app env vars.
func (e *Engine) collectEnvVars(ctx context.Context, app *domain.App) []string {
	var envVars []string

	// Fetch linked services and inject their credentials as env vars
	links, err := e.services.ListAppServices(ctx, app.ID)
	if err != nil {
		e.logger.Warn("list app services", "error", err)
	} else {
		for _, link := range links {
			svc, err := e.services.GetService(ctx, link.ServiceID)
			if err != nil {
				e.logger.Warn("get linked service", "service_id", link.ServiceID, "error", err)
				continue
			}
			if svc.Status != domain.ServiceStatusActive {
				continue
			}
			var creds interface{}
			switch svc.Type {
			case domain.ServicePostgres:
				var pc domain.PostgresCredentials
				if err := e.provisioner.DecryptCredentials(svc, &pc); err != nil {
					e.logger.Warn("decrypt creds", "service", svc.Name, "error", err)
					continue
				}
				creds = pc
			case domain.ServiceMySQL:
				var mc domain.MySQLCredentials
				if err := e.provisioner.DecryptCredentials(svc, &mc); err != nil {
					e.logger.Warn("decrypt creds", "service", svc.Name, "error", err)
					continue
				}
				creds = mc
			case domain.ServiceRedis:
				var rc domain.RedisCredentials
				if err := e.provisioner.DecryptCredentials(svc, &rc); err != nil {
					e.logger.Warn("decrypt creds", "service", svc.Name, "error", err)
					continue
				}
				creds = rc
			case domain.ServiceGarage:
				var gc domain.GarageCredentials
				if err := e.provisioner.DecryptCredentials(svc, &gc); err != nil {
					e.logger.Warn("decrypt creds", "service", svc.Name, "error", err)
					continue
				}
				creds = gc
			default:
				continue
			}
			envVars = append(envVars, services.FormatEnvVars(svc, creds, link.Alias)...)
		}
	}

	// Fetch app-level env vars
	appVars, err := e.services.GetAppEnvVars(ctx, app.ID)
	if err != nil {
		e.logger.Warn("get app env vars", "error", err)
	} else {
		for _, v := range appVars {
			envVars = append(envVars, v.Key+"="+v.Value)
		}
	}

	return envVars
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
