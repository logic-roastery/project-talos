package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/logic-roastery/project-talos/internal/auth"
	"github.com/logic-roastery/project-talos/internal/deploy"
	"github.com/logic-roastery/project-talos/internal/github"
	"github.com/logic-roastery/project-talos/internal/server/handlers"
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
	authSvc *auth.Service,
	engine *deploy.Engine,
	webhook *github.WebhookHandler,
	renderer *web.Renderer,
	serverHost string,
	logger *slog.Logger,
) *Server {
	r := chi.NewRouter()

	r.Use(RecoverMiddleware(logger))
	r.Use(LoggingMiddleware(logger))

	r.Get("/health", handlers.Health)

	authH := handlers.NewAuthHandler(authSvc)
	r.Post("/api/auth/setup", authH.Setup)
	r.Post("/api/auth/login", authH.Login)
	r.Get("/api/auth/status", authH.SetupStatus)

	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(authSvc))

		r.Get("/api/auth/me", authH.Me)
		r.Post("/api/auth/logout", authH.Logout)

		appH := handlers.NewAppHandler(apps, deploys, serverHost)
		r.Route("/api/apps", func(r chi.Router) {
			r.Get("/", appH.List)
			r.Post("/", appH.Create)
			r.Get("/{appID}", appH.Get)
			r.Put("/{appID}", appH.Update)
			r.Delete("/{appID}", appH.Delete)
		})

		deployH := handlers.NewDeployHandler(apps, deploys, engine)
		r.Route("/api/apps/{appID}/deploys", func(r chi.Router) {
			r.Get("/", deployH.List)
			r.Post("/", deployH.Trigger)
			r.Post("/rollback", deployH.Rollback)
		})
		r.Get("/api/deploys/{deployID}", deployH.Get)
	})

	r.Post("/api/webhooks/github", func(w http.ResponseWriter, r *http.Request) {
		payload, err := webhook.VerifyAndParse(r)
		if err != nil {
			logger.Warn("webhook verification failed", "error", err)
			http.Error(w, "invalid webhook", http.StatusBadRequest)
			return
		}

		if payload.Workflow.Status == "completed" && payload.Workflow.Conclusion == "success" {
			a, err := apps.GetAppByName(r.Context(), payload.Repository.FullName)
			if err != nil {
				logger.Warn("webhook: app not found", "repo", payload.Repository.FullName)
				http.Error(w, "app not found", http.StatusNotFound)
				return
			}

			sha := payload.Workflow.HeadSHA
			if len(sha) > 7 {
				sha = sha[:7]
			}
			imageRef := payload.Repository.FullName + ":" + sha

			_, err = engine.Deploy(r.Context(), a.ID, imageRef, payload.Workflow.HeadSHA, payload.Workflow.HeadBranch, "webhook")
			if err != nil {
				logger.Error("webhook deploy failed", "error", err)
				http.Error(w, "deploy failed", http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
	})

	// Page routes (HTML)
	pageH := handlers.NewPageHandler(renderer, apps, deploys, users, authSvc, engine, serverHost)

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
		r.Get("/apps/new", pageH.AppCreatePage)
		r.Post("/apps/new", pageH.AppCreateSubmit)
		r.Get("/apps/{appID}", pageH.AppDetailPage)
		r.Post("/apps/{appID}/deploy", pageH.TriggerDeploy)
		r.Post("/apps/{appID}/rollback", pageH.TriggerRollback)
		r.Delete("/apps/{appID}", pageH.DeleteApp)
		r.Get("/partials/deploy-status/{deployID}", pageH.DeployStatusPartial)
		r.Get("/partials/app-row/{appID}", pageH.AppRowPartial)
		r.Post("/logout", pageH.Logout)
	})

	return &Server{handler: r, logger: logger}
}

func (s *Server) Handler() http.Handler {
	return s.handler
}
