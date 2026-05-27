package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/logic-roastery/project-talos/internal/auth"
	"github.com/logic-roastery/project-talos/internal/deploy"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/github"
	"github.com/logic-roastery/project-talos/internal/store"
	"github.com/logic-roastery/project-talos/web"
)

type PageHandler struct {
	renderer *web.Renderer
	apps     store.AppStore
	deploys  store.DeployStore
	users    store.UserStore
	services store.ServiceStore
	authSvc  *auth.Service
	engine   *deploy.Engine
	ghClient *github.AppClient
	host     string
	domain   string
}

func NewPageHandler(renderer *web.Renderer, apps store.AppStore, deploys store.DeployStore,
	users store.UserStore, services store.ServiceStore, authSvc *auth.Service, engine *deploy.Engine, ghClient *github.AppClient, host, domain string) *PageHandler {
	return &PageHandler{
		renderer: renderer,
		apps:     apps,
		deploys:  deploys,
		users:    users,
		services: services,
		authSvc:  authSvc,
		engine:   engine,
		domain:   domain,
		ghClient: ghClient,
		host:     host,
	}
}

func (h *PageHandler) userData(r *http.Request) *web.UserData {
	u := UserFromContext(r.Context())
	if u == nil {
		return nil
	}
	return &web.UserData{Username: u.Username}
}

// --- Public pages ---

func (h *PageHandler) SetupPage(w http.ResponseWriter, r *http.Request) {
	required, err := h.authSvc.SetupRequired(r.Context())
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if !required {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if u := UserFromContext(r.Context()); u != nil {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	h.renderer.Render(w, "setup.html", "Setup", nil, nil)
}

func (h *PageHandler) SetupSubmit(w http.ResponseWriter, r *http.Request) {
	required, err := h.authSvc.SetupRequired(r.Context())
	if err != nil || !required {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		h.renderer.RenderStatus(w, http.StatusUnprocessableEntity, "flash.html",
			map[string]string{"Color": "red", "Message": "Username and password are required."})
		return
	}
	if len(password) < 8 {
		h.renderer.RenderStatus(w, http.StatusUnprocessableEntity, "flash.html",
			map[string]string{"Color": "red", "Message": "Password must be at least 8 characters."})
		return
	}

	user, err := h.authSvc.CreateUser(r.Context(), username, password)
	if err != nil {
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Failed to create account."})
		return
	}

	token, err := h.authSvc.CreateSession(user)
	if err != nil {
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Failed to create session."})
		return
	}

	setSessionCookie(w, token)
	w.Header().Set("HX-Redirect", "/dashboard")
}

func (h *PageHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	required, err := h.authSvc.SetupRequired(r.Context())
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if required {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	if u := UserFromContext(r.Context()); u != nil {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	h.renderer.Render(w, "login.html", "Login", nil, nil)
}

func (h *PageHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := h.authSvc.Authenticate(r.Context(), username, password)
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			h.renderer.RenderStatus(w, http.StatusUnauthorized, "flash.html",
				map[string]string{"Color": "red", "Message": "Invalid credentials."})
			return
		}
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Authentication failed."})
		return
	}

	token, err := h.authSvc.CreateSession(user)
	if err != nil {
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Failed to create session."})
		return
	}

	setSessionCookie(w, token)
	w.Header().Set("HX-Redirect", "/dashboard")
}

func (h *PageHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "talos_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	w.Header().Set("HX-Redirect", "/login")
}

// --- Dashboard ---

func (h *PageHandler) DashboardPage(w http.ResponseWriter, r *http.Request) {
	apps, err := h.apps.ListApps(r.Context())
	if err != nil {
		http.Error(w, "failed to load apps", http.StatusInternalServerError)
		return
	}

	data := struct {
		User *web.UserData
		Apps []*domain.App
	}{
		User: h.userData(r),
		Apps: apps,
	}
	h.renderer.Render(w, "dashboard.html", "Dashboard", h.userData(r), data)
}

// --- App CRUD ---

func (h *PageHandler) AppCreatePage(w http.ResponseWriter, r *http.Request) {
	h.renderer.Render(w, "app_create.html", "New App", h.userData(r), nil)
}

func (h *PageHandler) AppCreateSubmit(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	repoURL := r.FormValue("repo_url")
	branch := r.FormValue("branch")
	portStr := r.FormValue("internal_port")
	domainName := r.FormValue("domain")

	if name == "" || repoURL == "" {
		h.renderer.RenderStatus(w, http.StatusUnprocessableEntity, "flash.html",
			map[string]string{"Color": "red", "Message": "Name and repository URL are required."})
		return
	}

	if branch == "" {
		branch = "main"
	}
	internalPort := 3000
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			internalPort = p
		}
	}

	accessMode := domain.AccessModePort
	accessURL := ""
	var fallbackPort int

	if domainName != "" {
		accessMode = domain.AccessModeDomain
		accessURL = "https://" + domainName
	} else {
		port, err := h.apps.NextFallbackPort(r.Context())
		if err != nil {
			h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
				map[string]string{"Color": "red", "Message": "Failed to assign port."})
			return
		}
		fallbackPort = port
		host := h.host
		if h.domain != "" {
			host = h.domain
		}
		accessURL = fmt.Sprintf("http://%s:%d", host, port)
	}

	app := &domain.App{
		Name:         name,
		Source:       "github",
		RepoURL:      repoURL,
		Branch:       branch,
		InternalPort: internalPort,
		Domain:       domainName,
		FallbackPort: fallbackPort,
		AccessMode:   accessMode,
		AccessURL:    accessURL,
		Status:       domain.AppStatusInactive,
	}

	if err := h.apps.CreateApp(r.Context(), app); err != nil {
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Failed to create app."})
		return
	}

	w.Header().Set("HX-Redirect", fmt.Sprintf("/apps/%d", app.ID))
}

func (h *PageHandler) AppDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "appID")
	if err != nil {
		http.Error(w, "invalid app id", http.StatusBadRequest)
		return
	}

	app, err := h.apps.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.Error(w, "app not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to get app", http.StatusInternalServerError)
		return
	}

	deploys, err := h.deploys.ListDeploys(r.Context(), id, 20)
	if err != nil {
		http.Error(w, "failed to get deploys", http.StatusInternalServerError)
		return
	}

	data := struct {
		User             *web.UserData
		App              *domain.App
		Deploys          []*domain.Deploy
		GitHubConfigured bool
	}{
		User:             h.userData(r),
		App:              app,
		Deploys:          deploys,
		GitHubConfigured: h.ghClient != nil && h.ghClient.IsConfigured(),
	}
	h.renderer.Render(w, "app.html", app.Name, h.userData(r), data)
}

func (h *PageHandler) DeleteApp(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "appID")
	if err != nil {
		http.Error(w, "invalid app id", http.StatusBadRequest)
		return
	}

	if err := h.apps.DeleteApp(r.Context(), id); err != nil {
		http.Error(w, "failed to delete app", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/dashboard")
}

// --- Deploy actions ---

func (h *PageHandler) TriggerDeploy(w http.ResponseWriter, r *http.Request) {
	appID, err := parseID(r, "appID")
	if err != nil {
		http.Error(w, "invalid app id", http.StatusBadRequest)
		return
	}

	imageRef := r.FormValue("image_ref")
	branch := r.FormValue("branch")

	if imageRef == "" || branch == "" {
		h.renderer.RenderStatus(w, http.StatusUnprocessableEntity, "flash.html",
			map[string]string{"Color": "red", "Message": "Image ref and branch are required."})
		return
	}

	d, err := h.engine.Deploy(r.Context(), appID, imageRef, "", branch, "manual")
	if err != nil {
		if errors.Is(err, domain.ErrDeployInProgress) {
			h.renderer.RenderStatus(w, http.StatusConflict, "flash.html",
				map[string]string{"Color": "yellow", "Message": "Deploy already in progress."})
			return
		}
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Failed to trigger deploy."})
		return
	}

	data := struct {
		Deploy *domain.Deploy
		AppID  int64
	}{
		Deploy: d,
		AppID:  appID,
	}
	h.renderer.RenderPartial(w, "deploy_row.html", data)
}

func (h *PageHandler) TriggerRollback(w http.ResponseWriter, r *http.Request) {
	appID, err := parseID(r, "appID")
	if err != nil {
		http.Error(w, "invalid app id", http.StatusBadRequest)
		return
	}

	d, err := h.engine.Rollback(r.Context(), appID)
	if err != nil {
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Failed to rollback."})
		return
	}

	data := struct {
		Deploy *domain.Deploy
		AppID  int64
	}{
		Deploy: d,
		AppID:  appID,
	}
	h.renderer.RenderPartial(w, "deploy_row.html", data)
}

// --- HTMX Partials ---

func (h *PageHandler) DeployStatusPartial(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "deployID")
	if err != nil {
		http.Error(w, "invalid deploy id", http.StatusBadRequest)
		return
	}

	d, err := h.deploys.GetDeploy(r.Context(), id)
	if err != nil {
		http.Error(w, "deploy not found", http.StatusNotFound)
		return
	}

	h.renderer.RenderPartial(w, "deploy_status.html", d)
}

// --- Service Pages ---

func (h *PageHandler) ServicesPage(w http.ResponseWriter, r *http.Request) {
	svcs, err := h.services.ListServices(r.Context())
	if err != nil {
		http.Error(w, "failed to list services", http.StatusInternalServerError)
		return
	}
	data := struct {
		User     *web.UserData
		Services []*domain.Service
	}{
		User:     h.userData(r),
		Services: svcs,
	}
	h.renderer.Render(w, "services.html", "Services", h.userData(r), data)
}

func (h *PageHandler) ServiceCreatePage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		User *web.UserData
	}{
		User: h.userData(r),
	}
	h.renderer.Render(w, "service_create.html", "Create Service", h.userData(r), data)
}

func (h *PageHandler) ServiceDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "serviceID")
	if err != nil {
		http.Error(w, "invalid service id", http.StatusBadRequest)
		return
	}
	svc, err := h.services.GetService(r.Context(), id)
	if err != nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	svc.Credentials = ""

	linkedApps, _ := h.services.GetLinkedApps(r.Context(), id)

	data := struct {
		User       *web.UserData
		Service    *domain.Service
		LinkedApps []*domain.AppService
	}{
		User:       h.userData(r),
		Service:    svc,
		LinkedApps: linkedApps,
	}
	h.renderer.Render(w, "service_detail.html", svc.Name, h.userData(r), data)
}

func (h *PageHandler) AppSettingsPage(w http.ResponseWriter, r *http.Request) {
	appID, err := parseID(r, "appID")
	if err != nil {
		http.Error(w, "invalid app id", http.StatusBadRequest)
		return
	}
	app, err := h.apps.GetApp(r.Context(), appID)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	envVars, _ := h.services.GetAppEnvVars(r.Context(), appID)
	links, _ := h.services.ListAppServices(r.Context(), appID)
	allServices, _ := h.services.ListServices(r.Context())

	// Mask secrets
	for _, v := range envVars {
		if v.IsSecret {
			v.Value = "****"
		}
	}

	data := struct {
		User        *web.UserData
		App         *domain.App
		EnvVars     []*domain.AppEnvVar
		Links       []*domain.AppService
		AllServices []*domain.Service
	}{
		User:        h.userData(r),
		App:         app,
		EnvVars:     envVars,
		Links:       links,
		AllServices: allServices,
	}
	h.renderer.Render(w, "app_settings.html", app.Name+" Settings", h.userData(r), data)
}

func (h *PageHandler) AppRowPartial(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "appID")
	if err != nil {
		http.Error(w, "invalid app id", http.StatusBadRequest)
		return
	}

	app, err := h.apps.GetApp(r.Context(), id)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	h.renderer.RenderPartial(w, "app_row.html", app)
}
