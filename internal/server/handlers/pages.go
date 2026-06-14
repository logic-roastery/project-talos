package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/logic-roastery/project-talos/internal/auth"
	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/deploy"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/github"
	"github.com/logic-roastery/project-talos/internal/proxy/traefik"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/settings"
	"github.com/logic-roastery/project-talos/internal/store"
	"github.com/logic-roastery/project-talos/web"
)

type PageHandler struct {
	renderer    *web.Renderer
	apps        store.AppStore
	deploys     store.DeployStore
	users       store.UserStore
	services    store.ServiceStore
	backupStore store.BackupStore
	authSvc     *auth.Service
	engine      *deploy.Engine
	docker      *docker.Client
	proxy       *traefik.Manager
	ghClient    *github.AppClient
	settings    *settings.Service
	host        string
	domain      string
	proxyMode   config.ProxyMode
	port        int
	logger      *slog.Logger
}

func NewPageHandler(renderer *web.Renderer, apps store.AppStore, deploys store.DeployStore,
	users store.UserStore, services store.ServiceStore, backupStore store.BackupStore, authSvc *auth.Service, engine *deploy.Engine, dockerClient *docker.Client, proxy *traefik.Manager, ghClient *github.AppClient, settingsSvc *settings.Service, host, domain string, proxyMode config.ProxyMode, port int, logger *slog.Logger) *PageHandler {
	return &PageHandler{
		renderer:    renderer,
		apps:        apps,
		deploys:     deploys,
		users:       users,
		services:    services,
		backupStore: backupStore,
		authSvc:     authSvc,
		engine:      engine,
		docker:      dockerClient,
		proxy:       proxy,
		domain:      domain,
		ghClient:    ghClient,
		settings:    settingsSvc,
		host:        host,
		proxyMode:   proxyMode,
		port:        port,
		logger:      logger,
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

	rows := make([]dashboardAppView, 0, len(apps))
	for _, app := range apps {
		rows = append(rows, buildDashboardAppView(app, h.proxyMode))
	}

	data := struct {
		User *web.UserData
		Apps []dashboardAppView
	}{
		User: h.userData(r),
		Apps: rows,
	}
	h.renderer.Render(w, "dashboard.html", "Dashboard", h.userData(r), data)
}

// --- App CRUD ---

func (h *PageHandler) AppCreatePage(w http.ResponseWriter, r *http.Request) {
	data := appCreatePageData{
		GitHubConfigured: h.ghClient != nil && h.ghClient.IsConfigured(),
		ProxyMode:        h.proxyMode,
		TypeOptions:      appTypeOptions(),
	}

	if data.GitHubConfigured {
		repos, err := listAllRepos(r.Context(), h.ghClient, h.logger)
		if err != nil {
			data.RepoError = "Talos could not load repositories from GitHub right now. Enter the repository URL manually or refresh after checking the GitHub App installation."
		} else {
			data.Repos = repos
		}
	}
	if h.docker != nil {
		containers, err := h.docker.ListContainers(r.Context(), true)
		if err == nil {
			data.Containers = containers
		}
	}

	h.renderer.Render(w, "app_create.html", "New App", h.userData(r), data)
}

func (h *PageHandler) AppCreateSubmit(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	appType := normalizeAppType(domain.AppType(r.FormValue("app_type")))
	buildMode := normalizeBuildMode(domain.BuildMode(r.FormValue("build_mode")))
	projectType := domain.ProjectType(r.FormValue("project_type"))
	repoURL := r.FormValue("repo_url")
	branch := r.FormValue("branch")
	portStr := r.FormValue("internal_port")
	if appType == domain.AppTypeAdoptedContainer {
		if v := r.FormValue("adopted_internal_port"); v != "" {
			portStr = v
		}
	}
	domainName := r.FormValue("domain")
	imageRef := r.FormValue("image_ref")
	containerName := r.FormValue("container_name")
	externalTarget := r.FormValue("external_target")
	dockerNetwork := r.FormValue("docker_network")

	if name == "" {
		h.renderer.RenderStatus(w, http.StatusUnprocessableEntity, "flash.html",
			map[string]string{"Color": "red", "Message": "Name is required."})
		return
	}

	if branch == "" {
		branch = "main"
	}
	internalPort := 0
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			internalPort = p
		}
	}
	app := &domain.App{
		Name:           name,
		AppType:        appType,
		BuildMode:      buildMode,
		ProjectType:    projectType,
		RuntimeOwner:   runtimeOwnerForType(appType),
		EdgeProvider:   edgeProviderForMode(h.proxyMode),
		Source:         sourceForType(appType),
		RepoURL:        strings.TrimSpace(repoURL),
		Branch:         branch,
		InternalPort:   internalPort,
		ImageRef:       strings.TrimSpace(imageRef),
		Domain:         strings.TrimSpace(domainName),
		Status:         domain.AppStatusInactive,
		ContainerName:  strings.TrimSpace(containerName),
		ExternalTarget: strings.TrimSpace(externalTarget),
		DockerNetwork:  strings.TrimSpace(dockerNetwork),
	}
	if err := validateCreateRequest(createAppRequest{
		Name:           app.Name,
		AppType:        app.AppType,
		RepoURL:        app.RepoURL,
		Branch:         app.Branch,
		InternalPort:   app.InternalPort,
		Domain:         app.Domain,
		ImageRef:       app.ImageRef,
		ContainerName:  app.ContainerName,
		ExternalTarget: app.ExternalTarget,
		DockerNetwork:  app.DockerNetwork,
	}); err != nil {
		h.renderer.RenderStatus(w, http.StatusUnprocessableEntity, "flash.html",
			map[string]string{"Color": "red", "Message": err.Error()})
		return
	}
	if err := applyAccessFields(r.Context(), h.apps, h.host, h.domain, app); err != nil {
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Failed to prepare app routing."})
		return
	}

	// Set GitHub connection if provided (repo was selected from dropdown)
	if app.AppType == domain.AppTypeManaged {
		if installIDStr := r.FormValue("github_installation_id"); installIDStr != "" {
			if installID, err := strconv.ParseInt(installIDStr, 10, 64); err == nil {
				app.GitHubInstallationID = &installID
			}
		}
		if repoIDStr := r.FormValue("github_repo_id"); repoIDStr != "" {
			if repoID, err := strconv.ParseInt(repoIDStr, 10, 64); err == nil {
				app.GitHubRepoID = &repoID
				app.RegistryURL = "ghcr.io"
			}
		}
	}

	if err := h.apps.CreateApp(r.Context(), app); err != nil {
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Failed to create app."})
		return
	}
	if err := syncNonManagedRoute(r.Context(), h.proxy, app); err != nil {
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": "Failed to configure route."})
		return
	}

	// For adopted containers, inspect Docker to set correct status
	if app.AppType == domain.AppTypeAdoptedContainer && app.ContainerName != "" && h.docker != nil {
		inspected, err := h.docker.Inspect(r.Context(), app.ContainerName)
		if err == nil && inspected.State.Running {
			app.Status = domain.AppStatusActive
			if updateErr := h.apps.UpdateApp(r.Context(), app); updateErr != nil {
				h.logger.Warn("could not set adopted container status to active", "error", updateErr)
			}
		}
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

	var runtimeInfo *docker.ContainerInfo
	containerName := app.EffectiveContainerName()
	if containerName != "" && h.docker != nil {
		inspected, err := h.docker.Inspect(r.Context(), containerName)
		if err == nil {
			networks := make([]string, 0, len(inspected.NetworkSettings.Networks))
			for networkName := range inspected.NetworkSettings.Networks {
				networks = append(networks, networkName)
			}
			runtimeInfo = &docker.ContainerInfo{
				ID:       inspected.ID,
				Name:     strings.TrimPrefix(inspected.Name, "/"),
				Image:    inspected.Config.Image,
				State:    string(inspected.State.Status),
				Status:   string(inspected.State.Status),
				Networks: networks,
			}
		}
	}

	// Sync adopted container status with actual Docker state
	if app.AppType == domain.AppTypeAdoptedContainer && containerName != "" && h.docker != nil {
		if runtimeInfo != nil && runtimeInfo.State == "running" && app.Status != domain.AppStatusActive {
			app.Status = domain.AppStatusActive
			h.apps.UpdateApp(r.Context(), app)
		} else if (runtimeInfo == nil || runtimeInfo.State != "running") && app.Status == domain.AppStatusActive {
			app.Status = domain.AppStatusInactive
			h.apps.UpdateApp(r.Context(), app)
		}
	}

	manualRouteSnippet := ""
	if h.proxyMode == config.ProxyModeExternal && app.AppType != domain.AppTypeManaged && app.Domain != "" {
		target := app.ExternalTarget
		if target == "" {
			target = fmt.Sprintf("http://%s:%d", app.EffectiveContainerName(), app.InternalPort)
		}
		manualRouteSnippet = renderExternalTraefikSnippet(app.Name, app.Domain, target)
	}

	data := buildAppDetailPageData(
		h.userData(r),
		app,
		deploys,
		h.ghClient != nil && h.ghClient.IsConfigured(),
		runtimeInfo,
		h.proxyMode,
		manualRouteSnippet,
	)
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
	app, err := h.apps.GetApp(r.Context(), appID)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	if app.AppType != domain.AppTypeManaged {
		h.renderer.RenderStatus(w, http.StatusBadRequest, "flash.html",
			map[string]string{"Color": "yellow", "Message": "Only Talos-managed apps support deploys."})
		return
	}

	// For talos_build mode, image_ref is optional (Talos builds automatically)
	if app.BuildMode != domain.BuildModeTalosBuild {
		if imageRef == "" {
			h.renderer.RenderStatus(w, http.StatusUnprocessableEntity, "flash.html",
				map[string]string{"Color": "red", "Message": "Image ref is required."})
			return
		}
	}
	if branch == "" {
		branch = app.Branch
	}

	d, err := h.engine.Deploy(r.Context(), appID, imageRef, "", branch, "manual")
	if err != nil {
		if errors.Is(err, domain.ErrDeployInProgress) {
			h.renderer.RenderStatus(w, http.StatusConflict, "flash.html",
				map[string]string{"Color": "yellow", "Message": "Deploy already in progress."})
			return
		}
		h.renderer.RenderStatus(w, http.StatusInternalServerError, "flash.html",
			map[string]string{"Color": "red", "Message": fmt.Sprintf("Deploy failed: %v", err)})
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

	app, err := h.apps.GetApp(r.Context(), appID)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	if app.AppType != domain.AppTypeManaged {
		h.renderer.RenderStatus(w, http.StatusBadRequest, "flash.html",
			map[string]string{"Color": "yellow", "Message": "Only Talos-managed apps support rollback."})
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

	// Look up WebUI sidecar for Garage services
	var sidecar *domain.Service
	if svc.Type == domain.ServiceGarage {
		webUISvc, err := h.services.GetServiceByName(r.Context(), svc.Name+"-webui")
		if err == nil {
			sidecar = webUISvc
		}
	}

	data := struct {
		User       *web.UserData
		Service    *domain.Service
		LinkedApps []*domain.AppService
		Sidecar    *domain.Service
	}{
		User:       h.userData(r),
		Service:    svc,
		LinkedApps: linkedApps,
		Sidecar:    sidecar,
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

	manualRouteSnippet := ""
	if h.proxyMode == config.ProxyModeExternal && app.AppType != domain.AppTypeManaged && app.Domain != "" {
		target := app.ExternalTarget
		if target == "" {
			target = fmt.Sprintf("http://%s:%d", app.EffectiveContainerName(), app.InternalPort)
		}
		manualRouteSnippet = renderExternalTraefikSnippet(app.Name, app.Domain, target)
	}

	data := buildAppSettingsPageData(h.userData(r), app, envVars, links, allServices, h.proxyMode, manualRouteSnippet)
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

	h.renderer.RenderPartial(w, "app_row.html", buildDashboardAppView(app, h.proxyMode))
}

func (h *PageHandler) BackupPage(w http.ResponseWriter, r *http.Request) {
	backups, _ := h.backupStore.ListBackups(r.Context())
	data := struct {
		User    *web.UserData
		Backups []*domain.Backup
	}{
		User:    h.userData(r),
		Backups: backups,
	}
	h.renderer.Render(w, "backups.html", "Backups", h.userData(r), data)
}

func (h *PageHandler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		User *web.UserData
	}{
		User: h.userData(r),
	}
	h.renderer.Render(w, "settings.html", "Settings", h.userData(r), data)
}

func (h *PageHandler) GeneralSettingsPage(w http.ResponseWriter, r *http.Request) {
	h.renderGeneralSettingsPage(w, r, false, "")
}

func (h *PageHandler) GeneralSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		http.Error(w, "settings unavailable", http.StatusServiceUnavailable)
		return
	}

	snapshot, err := h.settings.Save(r.Context(), settings.UpdateInput{
		Domain:           r.FormValue("domain"),
		ACMEEmail:        r.FormValue("acme_email"),
		ProxyMode:        config.ProxyMode(r.FormValue("proxy_mode")),
		EdgeProvider:     config.EdgeProvider(r.FormValue("edge_provider")),
		EdgeNetwork:      r.FormValue("edge_network"),
		EdgeCertResolver: r.FormValue("edge_cert_resolver"),
		EdgeEntrypoint:   r.FormValue("edge_entrypoint"),
	}, h.requestHost(r), h.port)
	if err != nil {
		http.Error(w, "failed to save settings", http.StatusInternalServerError)
		return
	}

	h.renderGeneralSettings(w, r, snapshot, true, "")
}

func (h *PageHandler) renderGeneralSettingsPage(w http.ResponseWriter, r *http.Request, saved bool, errorMessage string) {
	if h.settings == nil {
		http.Error(w, "settings unavailable", http.StatusServiceUnavailable)
		return
	}

	snapshot, err := h.settings.Load(r.Context(), h.requestHost(r), h.port)
	if err != nil {
		http.Error(w, "failed to load settings", http.StatusInternalServerError)
		return
	}

	h.renderGeneralSettings(w, r, snapshot, saved, errorMessage)
}

func (h *PageHandler) renderGeneralSettings(w http.ResponseWriter, r *http.Request, snapshot settings.Snapshot, saved bool, errorMessage string) {
	routingModeLabel := "IP / port mode"
	if snapshot.Domain != "" {
		routingModeLabel = "Domain mode"
	}
	proxyModeLabel := "Internal Traefik"
	if snapshot.ProxyMode == config.ProxyModeExternal {
		proxyModeLabel = "External Traefik"
	}

	data := struct {
		User              *web.UserData
		Current           settings.Snapshot
		RoutingModeLabel  string
		ProxyModeLabel    string
		EdgeProviderLabel string
		Saved             bool
		ErrorMessage      string
	}{
		User:              h.userData(r),
		Current:           snapshot,
		RoutingModeLabel:  routingModeLabel,
		ProxyModeLabel:    proxyModeLabel,
		EdgeProviderLabel: strings.ToUpper(string(snapshot.EdgeProvider)),
		Saved:             saved,
		ErrorMessage:      errorMessage,
	}

	h.renderer.Render(w, "settings_general.html", "Proxy & Domain", h.userData(r), data)
}

func (h *PageHandler) requestHost(r *http.Request) string {
	host := r.Host
	if host == "" {
		host = h.host
	}
	if host == "0.0.0.0" || host == "0.0.0.0:0" {
		host = "localhost"
	}
	return host
}

func renderExternalTraefikSnippet(name, domainName, target string) string {
	serviceName := strings.ReplaceAll(name, "_", "-")
	return fmt.Sprintf(`http:
  routers:
    %s:
      rule: "Host(%c%s%c)"
      entryPoints:
        - websecure
      service: %s
      tls:
          certResolver: letsencrypt
  services:
    %s:
      loadBalancer:
        servers:
          - url: "%s"
`, serviceName, '`', domainName, '`', serviceName, serviceName, target)
}
