package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/logic-roastery/project-talos/internal/builder"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/github"
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
	builder     *builder.Builder
	ghClient    *github.AppClient
	dataDir     string
	logger      *slog.Logger
}

func NewEngine(apps store.AppStore, deploys store.DeployStore, services store.ServiceStore, provisioner *services.Provisioner, docker *docker.Client, proxy *traefik.Manager, builder *builder.Builder, ghClient *github.AppClient, dataDir string, logger *slog.Logger) *Engine {
	return &Engine{
		apps:        apps,
		deploys:     deploys,
		services:    services,
		provisioner: provisioner,
		docker:      docker,
		proxy:       proxy,
		builder:     builder,
		ghClient:    ghClient,
		dataDir:     dataDir,
		logger:      logger,
	}
}

func (e *Engine) getBuilder() *builder.Builder {
	if e.builder != nil {
		return e.builder
	}
	if e.ghClient != nil {
		e.logger.Info("engine: creating builder from lazy github client")
		e.builder = builder.NewBuilder(e.ghClient, e.docker, e.logger, e.dataDir)
		return e.builder
	}
	e.logger.Warn("engine: builder not available (ghClient is nil)")
	return nil
}

// SetGHClient updates the GitHub client reference, enabling lazy builder creation.
func (e *Engine) SetGHClient(c *github.AppClient) {
	e.ghClient = c
	e.logger.Info("engine: github client updated via lazy init")
}

func (e *Engine) Deploy(ctx context.Context, appID int64, imageRef, commitSHA, branch, triggeredBy string) (*domain.Deploy, error) {
	app, err := e.apps.GetApp(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	if app.AppType != domain.AppTypeManaged {
		return nil, fmt.Errorf("deploys are only supported for managed apps")
	}

	latest, err := e.deploys.GetLatestDeploy(ctx, appID)
	if err != nil && err != domain.ErrNotFound {
		return nil, fmt.Errorf("check running deploy: %w", err)
	}
	if latest != nil && (latest.Status == domain.DeployStatusRunning || latest.Status == domain.DeployStatusPending) {
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

	backgroundDeploy := *d
	go e.executeQueued(context.Background(), app, &backgroundDeploy)

	return d, nil
}

func (e *Engine) resolveBuildCommitSHA(ctx context.Context, app *domain.App, branch string) (string, error) {
	if e.ghClient == nil {
		return "", fmt.Errorf("GitHub App is not connected — go to Settings → GitHub to set it up, then try again")
	}
	if app.GitHubInstallationID == nil {
		return "", fmt.Errorf("this app is not connected to a GitHub installation")
	}
	if branch == "" {
		branch = app.Branch
	}
	if branch == "" {
		return "", fmt.Errorf("no branch configured for this app")
	}

	owner, repo, err := github.ParseRepoFullName(app.RepoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}

	sha, err := e.ghClient.ResolveBranchHeadSHA(ctx, *app.GitHubInstallationID, owner, repo, branch)
	if err != nil {
		return "", fmt.Errorf("resolve branch HEAD for %s: %w", branch, err)
	}

	return sha, nil
}

func (e *Engine) Rollback(ctx context.Context, appID int64) (*domain.Deploy, error) {
	app, err := e.apps.GetApp(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	if app.AppType != domain.AppTypeManaged {
		return nil, fmt.Errorf("rollbacks are only supported for managed apps")
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

	backgroundDeploy := *d
	go e.execute(context.Background(), app, &backgroundDeploy)

	return d, nil
}

func (e *Engine) executeQueued(ctx context.Context, app *domain.App, d *domain.Deploy) {
	e.emitEvent(ctx, d.ID, "info", "queue", "deploy queued")

	if app.BuildMode == domain.BuildModeTalosBuild && d.ImageRef == "" {
		if err := e.prepareTalosBuild(ctx, app, d); err != nil {
			e.failDeploy(ctx, d, err.Error())
			return
		}
	}

	e.execute(ctx, app, d)
}

func (e *Engine) prepareTalosBuild(ctx context.Context, app *domain.App, d *domain.Deploy) error {
	b := e.getBuilder()
	if b == nil {
		e.emitEvent(ctx, d.ID, "error", "build_setup", "GitHub App is not connected")
		return fmt.Errorf("GitHub App is not connected — go to Settings → GitHub to set it up, then try again")
	}
	if app.RepoURL == "" {
		e.emitEvent(ctx, d.ID, "error", "build_setup", "repository URL is not configured")
		return fmt.Errorf("no repository URL set for this app — edit the app settings to add one")
	}

	if d.CommitSHA == "" {
		e.emitEvent(ctx, d.ID, "info", "resolve_commit", "resolving branch HEAD for "+d.Branch)
		resolvedSHA, err := e.resolveBuildCommitSHA(ctx, app, d.Branch)
		if err != nil {
			e.emitEvent(ctx, d.ID, "error", "resolve_commit", err.Error())
			return err
		}
		d.CommitSHA = resolvedSHA
		if err := e.deploys.UpdateDeploy(ctx, d); err != nil {
			e.logger.Error("update deploy commit sha", "deploy_id", d.ID, "error", err)
		}
		e.emitEvent(ctx, d.ID, "info", "resolve_commit", "resolved commit "+shortSHA(d.CommitSHA))
	}

	e.emitEvent(ctx, d.ID, "info", "build", "starting repository clone and image build")
	result, err := b.CloneAndBuildWithProgress(ctx, app, d.CommitSHA, func(level, step, message string) {
		e.emitEvent(ctx, d.ID, level, step, message)
	})
	if err != nil {
		e.emitEvent(ctx, d.ID, "error", "build", fmt.Sprintf("build failed: %v", err))
		return fmt.Errorf("build failed: %w", err)
	}

	d.ImageRef = result.ImageRef
	if err := e.deploys.UpdateDeploy(ctx, d); err != nil {
		e.logger.Error("update deploy image ref", "deploy_id", d.ID, "error", err)
	}
	e.emitEvent(ctx, d.ID, "info", "build", "image built successfully: "+d.ImageRef)
	return nil
}

func (e *Engine) execute(ctx context.Context, app *domain.App, d *domain.Deploy) {
	now := time.Now()
	d.StartedAt = &now
	d.Status = domain.DeployStatusRunning
	e.deploys.UpdateDeploy(ctx, d)

	e.logger.Info("starting deploy", "deploy_id", d.ID, "app", app.Name, "image", d.ImageRef)
	e.emitEvent(ctx, d.ID, "info", "start", fmt.Sprintf("deploy started for %s with image %s", app.Name, d.ImageRef))

	// Validate required env vars
	if err := e.validateEnvVars(ctx, app); err != nil {
		e.emitEvent(ctx, d.ID, "error", "start", fmt.Sprintf("validation failed: %v", err))
		e.failDeploy(ctx, d, err.Error())
		return
	}

	// Capture env snapshot
	snapshot, _ := e.services.GetAppEnvVarsSnapshot(ctx, app.ID)
	if envJSON, err := json.Marshal(snapshot); err == nil {
		d.EnvSnapshot = string(envJSON)
	}

	// Pull image
	e.emitEvent(ctx, d.ID, "info", "pull", "pulling image "+d.ImageRef)
	if err := e.docker.PullImage(ctx, d.ImageRef); err != nil {
		e.emitEvent(ctx, d.ID, "error", "pull", fmt.Sprintf("pull failed: %v", err))
		e.failDeploy(ctx, d, fmt.Sprintf("pull image: %v", err))
		return
	}
	e.emitEvent(ctx, d.ID, "info", "pull", "image pulled successfully")

	// Blue/green: start staging container alongside the live one
	stagingName := fmt.Sprintf("talos-%s-%d", app.Name, d.ID)
	liveName := app.LiveContainerName
	if liveName == "" {
		liveName = fmt.Sprintf("talos-%s", app.Name)
	}

	// Collect env vars from linked services and app env vars
	envVars := e.collectEnvVars(ctx, app)

	e.emitEvent(ctx, d.ID, "info", "start", "starting staging container "+stagingName)
	cfg := docker.ContainerConfig{
		Name:         stagingName,
		ImageRef:     d.ImageRef,
		InternalPort: app.InternalPort,
		Env:          envVars,
		Labels: map[string]string{
			"managed-by": "talos",
			"talos-app":  app.Name,
		},
	}
	for k, v := range e.proxy.ExternalLabels(app) {
		cfg.Labels[k] = v
	}
	cfg.Networks = e.proxy.ExternalNetworks(app)
	if app.AccessMode == domain.AccessModePort && app.FallbackPort > 0 {
		cfg.Ports = []string{fmt.Sprintf("%d:%d", app.FallbackPort, app.InternalPort)}
	}

	// Port-mode apps and external domain-mode apps need exclusive access during the cutover.
	requiresExclusiveCutover := app.AccessMode == domain.AccessModePort || e.proxy.RequiresExclusiveSwitch(app)
	if requiresExclusiveCutover && liveName != "" {
		reason := "before binding fallback port"
		if app.AccessMode == domain.AccessModeDomain {
			reason = "before switching external proxy labels"
		}
		e.emitEvent(ctx, d.ID, "info", "stop_old", "stopping old container "+liveName+" "+reason)
		if err := e.docker.StopAndRemove(ctx, liveName); err != nil {
			e.emitEvent(ctx, d.ID, "warn", "stop_old", fmt.Sprintf("stop old container: %v", err))
			e.logger.Warn("stop old container", "error", err)
		}
	}

	containerID, err := e.docker.StartContainerWithConfig(ctx, cfg)
	if err != nil {
		e.emitEvent(ctx, d.ID, "error", "start", fmt.Sprintf("start failed: %v", err))
		e.failDeploy(ctx, d, fmt.Sprintf("start container: %v", err))
		return
	}
	d.ContainerID = containerID
	e.emitEvent(ctx, d.ID, "info", "start", "staging container started: "+containerID)

	// Health check the staging container
	e.emitEvent(ctx, d.ID, "info", "health_check", "waiting for health check (30s timeout)")
	if err := e.docker.WaitForHealth(ctx, containerID, 30*time.Second); err != nil {
		d.HealthStatus = "unhealthy"
		e.emitEvent(ctx, d.ID, "error", "health_check", fmt.Sprintf("health check failed: %v", err))
		// Auto-rollback: stop staging, leave live container running
		e.emitEvent(ctx, d.ID, "info", "auto_rollback", "stopping staging container, live container preserved")
		if cleanErr := e.docker.StopAndRemove(ctx, stagingName); cleanErr != nil {
			e.logger.Warn("cleanup staging container", "error", cleanErr)
		}
		completed := time.Now()
		d.CompletedAt = &completed
		d.Status = domain.DeployStatusAutoRollback
		d.Logs = fmt.Sprintf("health check failed, auto-rolled back: %v", err)
		if updateErr := e.deploys.UpdateDeploy(ctx, d); updateErr != nil {
			e.logger.Error("update auto-rollback deploy", "error", updateErr)
		}
		e.emitEvent(ctx, d.ID, "info", "auto_rollback", "auto-rollback complete, previous version still live")
		e.logger.Warn("deploy auto-rolled back", "deploy_id", d.ID, "app", app.Name, "reason", err)
		return
	}
	d.HealthStatus = "healthy"
	e.emitEvent(ctx, d.ID, "info", "health_check", "health check passed")

	if app.AccessMode == domain.AccessModeDomain {
		if e.proxy.RequiresExclusiveSwitch(app) {
			e.emitEvent(ctx, d.ID, "info", "route_update", "external proxy mode uses container labels; new container is now serving the custom domain")
		} else {
			// Update route to point to staging container
			e.emitEvent(ctx, d.ID, "info", "route_update", "updating traefik route to "+stagingName)
			if err := e.proxy.UpdateRoute(ctx, app, stagingName); err != nil {
				e.emitEvent(ctx, d.ID, "error", "route_update", fmt.Sprintf("route update failed: %v", err))
				// Clean up staging, leave live
				if cleanErr := e.docker.StopAndRemove(ctx, stagingName); cleanErr != nil {
					e.logger.Warn("cleanup staging container", "error", cleanErr)
				}
				e.failDeploy(ctx, d, fmt.Sprintf("update route: %v", err))
				return
			}
			e.emitEvent(ctx, d.ID, "info", "route_update", "route updated successfully")
		}
	} else {
		e.emitEvent(ctx, d.ID, "info", "route_update", "fallback port mode uses direct host port binding; no proxy route update required")
	}

	// Stop old live container (only after new one is healthy and routed)
	if liveName != stagingName && !requiresExclusiveCutover {
		e.emitEvent(ctx, d.ID, "info", "stop_old", "stopping old container "+liveName)
		if err := e.docker.StopAndRemove(ctx, liveName); err != nil {
			e.emitEvent(ctx, d.ID, "warn", "stop_old", fmt.Sprintf("stop old container: %v", err))
			e.logger.Warn("stop old container", "error", err)
		} else {
			e.emitEvent(ctx, d.ID, "info", "stop_old", "old container stopped")
		}
	}

	// Finalize
	completed := time.Now()
	d.CompletedAt = &completed
	d.Status = domain.DeployStatusSuccess
	if err := e.deploys.UpdateDeploy(ctx, d); err != nil {
		e.logger.Error("update deploy", "error", err)
	}

	app.Status = domain.AppStatusActive
	app.ImageRef = d.ImageRef
	app.CurrentDeployID = &d.ID
	app.LiveContainerName = stagingName
	if err := e.apps.UpdateApp(ctx, app); err != nil {
		e.logger.Error("update app", "error", err)
	}

	e.emitEvent(ctx, d.ID, "info", "finalize", "deploy completed successfully")
	e.logger.Info("deploy completed", "deploy_id", d.ID, "app", app.Name)
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
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
				creds = &pc
			case domain.ServiceMySQL:
				var mc domain.MySQLCredentials
				if err := e.provisioner.DecryptCredentials(svc, &mc); err != nil {
					e.logger.Warn("decrypt creds", "service", svc.Name, "error", err)
					continue
				}
				creds = &mc
			case domain.ServiceRedis:
				var rc domain.RedisCredentials
				if err := e.provisioner.DecryptCredentials(svc, &rc); err != nil {
					e.logger.Warn("decrypt creds", "service", svc.Name, "error", err)
					continue
				}
				creds = &rc
			case domain.ServiceGarage:
				var gc domain.GarageCredentials
				if err := e.provisioner.DecryptCredentials(svc, &gc); err != nil {
					e.logger.Warn("decrypt creds", "service", svc.Name, "error", err)
					continue
				}
				creds = &gc
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

	e.emitEvent(ctx, d.ID, "error", "finalize", fmt.Sprintf("deploy failed: %s", reason))
	e.logger.Error("deploy failed", "deploy_id", d.ID, "reason", reason)
}

func (e *Engine) emitEvent(ctx context.Context, deployID int64, level, step, message string) {
	event := &domain.DeployEvent{
		DeployID: deployID,
		Level:    level,
		Step:     step,
		Message:  message,
	}
	if err := e.deploys.CreateDeployEvent(ctx, event); err != nil {
		e.logger.Error("emit deploy event", "error", err)
	}
}

func (e *Engine) validateEnvVars(ctx context.Context, app *domain.App) error {
	vars, err := e.services.GetAppEnvVars(ctx, app.ID)
	if err != nil {
		return nil // non-fatal
	}
	var missing []string
	for _, v := range vars {
		if v.Required && v.Value == "" {
			missing = append(missing, v.Key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return nil
}
