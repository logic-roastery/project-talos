package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/logic-roastery/project-talos/internal/auth"
	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/deploy"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/github"
	"github.com/logic-roastery/project-talos/internal/proxy/traefik"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/server/handlers"
	"github.com/logic-roastery/project-talos/internal/services"
	"github.com/logic-roastery/project-talos/internal/settings"
	"github.com/logic-roastery/project-talos/internal/store"
	"github.com/logic-roastery/project-talos/web"
)

type Server struct {
	handler http.Handler
	logger  *slog.Logger
}

func New(
	apps store.AppStore,
	deploys store.DeployStore,
	users store.UserStore,
	svcStore store.ServiceStore,
	authSvc *auth.Service,
	engine *deploy.Engine,
	proxy *traefik.Manager,
	provisioner *services.Provisioner,
	webhook *github.WebhookHandler,
	ghClient *github.AppClient,
	ghCfg config.GitHubConfig,
	dockerClient *docker.Client,
	renderer *web.Renderer,
	backupHandler *handlers.BackupHandler,
	backupStore store.BackupStore,
	serverHost string,
	serverDomain string,
	serverProxyMode config.ProxyMode,
	serverPort int,
	logger *slog.Logger,
) *Server {
	r := chi.NewRouter()

	r.Use(RecoverMiddleware(logger))
	r.Use(LoggingMiddleware(logger))

	r.Get("/health", handlers.Health)

	authH := handlers.NewAuthHandler(authSvc)
	ghH := handlers.NewGitHubHandler(apps, ghClient, ghCfg, renderer, serverHost, serverDomain, logger)
	var pageH *handlers.PageHandler
	ghH.SetOnClientReady(func(c *github.AppClient) {
		engine.SetGHClient(c)
		if pageH != nil {
			pageH.SetGHClient(c)
		}
	})
	r.Post("/api/auth/setup", authH.Setup)
	r.Post("/api/auth/login", authH.Login)
	r.Get("/api/auth/status", authH.SetupStatus)

	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(authSvc))

		r.Get("/api/auth/me", authH.Me)
		r.Post("/api/auth/logout", authH.Logout)
		r.Get("/api/github/debug", ghH.Debug)

		appH := handlers.NewAppHandler(apps, deploys, dockerClient, proxy, serverHost, serverDomain, serverProxyMode)
		svcH := handlers.NewServiceHandler(svcStore, provisioner, logger)
		r.Route("/api/apps", func(r chi.Router) {
			r.Get("/", appH.List)
			r.Post("/", appH.Create)
			r.Get("/{appID}", appH.Get)
			r.Put("/{appID}", appH.Update)
			r.Delete("/{appID}", appH.Delete)
			r.Post("/{appID}/restart", appH.Restart)
			r.Post("/{appID}/services", svcH.LinkAppService)
			r.Delete("/{appID}/services/{serviceID}", svcH.UnlinkAppService)
			r.Get("/{appID}/services", svcH.ListAppServices)
			r.Get("/{appID}/env", svcH.ListEnvVars)
			r.Post("/{appID}/env", svcH.SetEnvVar)
			r.Delete("/{appID}/env/{key}", svcH.DeleteEnvVar)
			r.Get("/{appID}/env/{key}/history", svcH.ListEnvVarHistory)
			r.Get("/{appID}/env/{key}/reveal", svcH.RevealEnvVar)
		})

		deployH := handlers.NewDeployHandler(apps, deploys, engine)
		r.Route("/api/apps/{appID}/deploys", func(r chi.Router) {
			r.Get("/", deployH.List)
			r.Post("/", deployH.Trigger)
			r.Post("/rollback", deployH.Rollback)
		})
		r.Get("/api/deploys/{deployID}", deployH.Get)
		r.Get("/api/deploys/{deployID}/events", deployH.ListEvents)

		// Live log streaming
		logH := handlers.NewLogHandler(apps, dockerClient, logger)
		r.Get("/api/apps/{appID}/logs/stream", logH.StreamLogs)

		// Service management
		r.Route("/api/services", func(r chi.Router) {
			r.Get("/", svcH.List)
			r.Post("/", svcH.Create)
			r.Get("/{serviceID}", svcH.Get)
			r.Delete("/{serviceID}", svcH.Delete)
			r.Post("/{serviceID}/stop", svcH.Stop)
			r.Post("/{serviceID}/start", svcH.Start)
			r.Get("/{serviceID}/credentials", svcH.GetCredentials)
			r.Get("/{serviceID}/buckets", svcH.ListBuckets)
			r.Post("/{serviceID}/buckets", svcH.CreateBucket)
			r.Delete("/{serviceID}/buckets/{bucketID}", svcH.DeleteBucket)
		})

		// Backup management
		if backupHandler != nil {
			r.Route("/api/backups", func(r chi.Router) {
				r.Get("/", backupHandler.List)
				r.Post("/", backupHandler.Create)
				r.Get("/{backupID}/download", backupHandler.Download)
				r.Delete("/{backupID}", backupHandler.Delete)
				r.Post("/{backupID}/restore", backupHandler.Restore)
			})
		}

		// GitHub integration routes (always registered; handler returns 503 if not configured)
		r.Get("/api/github/install", ghH.StartInstall)
		r.Get("/api/github/callback", ghH.HandleCallback)
		r.Get("/api/github/repos", ghH.ListRepos)
		r.Get("/partials/github-repos", ghH.RepoSelectorPartial)
		r.Post("/api/github/disconnect", ghH.Disconnect)
	})

	// GitHub setup routes (always available, even without config)
	r.Get("/settings/github/setup", ghH.SetupPage)
	r.Get("/settings/github/status", ghH.StatusPage)
	r.Get("/api/github/create-manifest", ghH.CreateManifest)
	r.Get("/api/github/setup-callback", ghH.SetupCallback)

	r.Post("/api/webhooks/github", func(w http.ResponseWriter, r *http.Request) {
		result, err := webhook.VerifyAndParse(r)
		if err != nil {
			logger.Warn("webhook verification failed", "error", err)
			http.Error(w, "invalid webhook", http.StatusBadRequest)
			return
		}

		switch result.Event {
		case github.EventWorkflowRun:
			payload, err := github.ParseWorkflowRun(result.Payload)
			if err != nil {
				logger.Warn("webhook: parse workflow_run failed", "error", err)
				http.Error(w, "invalid payload", http.StatusBadRequest)
				return
			}

			if payload.Workflow.Status == "completed" && payload.Workflow.Conclusion == "success" {
				// Find app using proper fallback chain
				var app *domain.App
				if payload.Installation.ID > 0 && payload.Repository.ID > 0 {
					// Try installation+repo lookup first
					app, err = apps.GetAppByInstallationAndRepo(r.Context(), payload.Installation.ID, payload.Repository.ID)
					if err != nil {
						// Fallback to repo ID only
						app, err = apps.GetAppByGitHubRepoID(r.Context(), payload.Repository.ID)
					}
				} else if payload.Repository.ID > 0 {
					// Try repo ID only
					app, err = apps.GetAppByGitHubRepoID(r.Context(), payload.Repository.ID)
				}
				if app == nil {
					// Last resort: name-based lookup
					app, err = apps.GetAppByName(r.Context(), payload.Repository.FullName)
				}

				if err != nil {
					logger.Warn("webhook: app not found", "repo", payload.Repository.FullName)
					http.Error(w, "app not found", http.StatusNotFound)
					return
				}

				// Branch guard: only deploy if branch matches app config
				if payload.Workflow.HeadBranch != app.Branch {
					logger.Info("webhook: branch mismatch, skipping deploy",
						"repo", payload.Repository.FullName,
						"expected", app.Branch,
						"got", payload.Workflow.HeadBranch)
					w.WriteHeader(http.StatusOK)
					return
				}

				// App type validation: only deploy managed apps
				if app.AppType != domain.AppTypeManaged {
					logger.Warn("webhook: app is not managed, skipping deploy",
						"app", app.Name, "type", app.AppType)
					w.WriteHeader(http.StatusOK)
					return
				}

				sha := payload.Workflow.HeadSHA
				if len(sha) > 7 {
					sha = sha[:7]
				}

				// Construct image ref using registry URL from app config
				registry := app.RegistryURL
				if registry == "" {
					registry = "ghcr.io"
				}
				imageRef := registry + "/" + payload.Repository.FullName + ":" + sha

				_, err = engine.Deploy(r.Context(), app.ID, imageRef, payload.Workflow.HeadSHA, payload.Workflow.HeadBranch, "webhook")
				if err != nil {
					logger.Error("webhook deploy failed", "error", err)
					http.Error(w, "deploy failed", http.StatusInternalServerError)
					return
				}
			}

		case github.EventInstallation:
			payload, err := github.ParseInstallation(result.Payload)
			if err != nil {
				logger.Warn("webhook: parse installation failed", "error", err)
				http.Error(w, "invalid payload", http.StatusBadRequest)
				return
			}

			switch payload.Action {
			case "created":
				logger.Info("github app installed", "installation_id", payload.Installation.ID, "repos", len(payload.Repositories))
				// Installation tracking is handled by the callback flow
			case "deleted":
				logger.Info("github app uninstalled", "installation_id", payload.Installation.ID)
				// TODO: Clear GitHubInstallationID on affected apps
			}

		case github.EventPush:
			payload, err := github.ParsePush(result.Payload)
			if err != nil {
				logger.Warn("webhook: parse push failed", "error", err)
				http.Error(w, "invalid payload", http.StatusBadRequest)
				return
			}

			// Extract branch from ref (refs/heads/main -> main)
			branch := strings.TrimPrefix(payload.Ref, "refs/heads/")

			// Find app using proper fallback chain
			var app *domain.App
			if payload.Installation.ID > 0 && payload.Repository.ID > 0 {
				app, err = apps.GetAppByInstallationAndRepo(r.Context(), payload.Installation.ID, payload.Repository.ID)
				if err != nil {
					app, err = apps.GetAppByGitHubRepoID(r.Context(), payload.Repository.ID)
				}
			} else if payload.Repository.ID > 0 {
				app, err = apps.GetAppByGitHubRepoID(r.Context(), payload.Repository.ID)
			}
			if app == nil {
				app, err = apps.GetAppByName(r.Context(), payload.Repository.FullName)
			}

			if err != nil {
				logger.Warn("webhook: app not found for push", "repo", payload.Repository.FullName)
				w.WriteHeader(http.StatusOK)
				return
			}

			// Branch guard
			if branch != app.Branch {
				logger.Info("webhook: push branch mismatch, skipping",
					"repo", payload.Repository.FullName,
					"expected", app.Branch,
					"got", branch)
				w.WriteHeader(http.StatusOK)
				return
			}

			// App type validation
			if app.AppType != domain.AppTypeManaged {
				logger.Warn("webhook: app is not managed, skipping push deploy",
					"app", app.Name, "type", app.AppType)
				w.WriteHeader(http.StatusOK)
				return
			}

			// Only trigger deploy for talos_build mode on push events
			if app.BuildMode != domain.BuildModeTalosBuild {
				logger.Info("webhook: push event for external_ci app, skipping (wait for workflow_run)",
					"app", app.Name)
				w.WriteHeader(http.StatusOK)
				return
			}

			// Trigger deploy with empty imageRef to trigger talos_build
			_, err = engine.Deploy(r.Context(), app.ID, "", payload.After, branch, "push")
			if err != nil {
				logger.Error("webhook push deploy failed", "error", err)
				http.Error(w, "deploy failed", http.StatusInternalServerError)
				return
			}
			logger.Info("webhook: push deploy triggered", "app", app.Name, "commit", payload.After[:7])

		default:
			logger.Debug("webhook: unhandled event", "event", result.Event)
		}

		w.WriteHeader(http.StatusOK)
	})

	// Page routes (HTML)
	settingsSvc := settings.NewService(dockerClient)
	pageH = handlers.NewPageHandler(renderer, apps, deploys, users, svcStore, backupStore, authSvc, engine, dockerClient, proxy, ghClient, settingsSvc, serverHost, serverDomain, serverProxyMode, serverPort, logger)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	})
	r.Get("/setup", pageH.SetupPage)
	r.Post("/setup", pageH.SetupSubmit)
	r.Get("/login", pageH.LoginPage)
	r.Post("/login", pageH.LoginSubmit)

	r.Group(func(r chi.Router) {
		r.Use(WebAuthMiddleware(authSvc))

		r.Get("/dashboard", pageH.DashboardPage)
		r.Get("/settings", pageH.SettingsPage)
		r.Get("/settings/general", pageH.GeneralSettingsPage)
		r.Post("/settings/general", pageH.GeneralSettingsSubmit)
		r.Get("/apps/new", pageH.AppCreatePage)
		r.Post("/apps/new", pageH.AppCreateSubmit)
		r.Get("/apps/{appID}", pageH.AppDetailPage)
		r.Post("/apps/{appID}/deploy", pageH.TriggerDeploy)
		r.Post("/apps/{appID}/rollback", pageH.TriggerRollback)
		r.Delete("/apps/{appID}", pageH.DeleteApp)
		r.Get("/partials/deploy-status/{deployID}", pageH.DeployStatusPartial)
		r.Get("/partials/app-row/{appID}", pageH.AppRowPartial)
		r.Get("/services", pageH.ServicesPage)
		r.Get("/services/new", pageH.ServiceCreatePage)
		r.Get("/services/{serviceID}", pageH.ServiceDetailPage)
		r.Get("/apps/{appID}/settings", pageH.AppSettingsPage)
		r.Get("/backups", pageH.BackupPage)
		r.Post("/logout", pageH.Logout)
	})

	return &Server{handler: r, logger: logger}
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
