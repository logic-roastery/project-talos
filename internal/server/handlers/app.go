package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/proxy/traefik"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/store"
)

type AppHandler struct {
	apps         store.AppStore
	deploys      store.DeployStore
	docker       *docker.Client
	proxy        *traefik.Manager
	serverHost   string
	serverDomain string
	proxyMode    config.ProxyMode
}

func NewAppHandler(apps store.AppStore, deploys store.DeployStore, dockerClient *docker.Client, proxy *traefik.Manager, serverHost, serverDomain string, proxyMode config.ProxyMode) *AppHandler {
	return &AppHandler{apps: apps, deploys: deploys, docker: dockerClient, proxy: proxy, serverHost: serverHost, serverDomain: serverDomain, proxyMode: proxyMode}
}

type createAppRequest struct {
	Name           string           `json:"name"`
	AppType        domain.AppType   `json:"app_type"`
	BuildMode      domain.BuildMode `json:"build_mode,omitempty"`
	RepoURL        string           `json:"repo_url"`
	Branch         string           `json:"branch"`
	InternalPort   int              `json:"internal_port"`
	Domain         string           `json:"domain,omitempty"`
	ImageRef       string           `json:"image_ref,omitempty"`
	ContainerName  string           `json:"container_name,omitempty"`
	ExternalTarget string           `json:"external_target,omitempty"`
	DockerNetwork  string           `json:"docker_network,omitempty"`
}

type updateAppRequest struct {
	Branch         *string         `json:"branch,omitempty"`
	InternalPort   *int            `json:"internal_port,omitempty"`
	Domain         *string         `json:"domain,omitempty"`
	ImageRef       *string         `json:"image_ref,omitempty"`
	ContainerName  *string         `json:"container_name,omitempty"`
	ExternalTarget *string         `json:"external_target,omitempty"`
	DockerNetwork  *string         `json:"docker_network,omitempty"`
	AppType        *domain.AppType `json:"app_type,omitempty"`
}

func (h *AppHandler) List(w http.ResponseWriter, r *http.Request) {
	apps, err := h.apps.ListApps(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list apps")
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (h *AppHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "appID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}

	app, err := h.apps.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get app")
		return
	}

	writeJSON(w, http.StatusOK, app)
}

func (h *AppHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	req.AppType = normalizeAppType(req.AppType)

	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.AppType != domain.AppTypeExternalService && req.InternalPort == 0 {
		req.InternalPort = 3000
	}

	if err := validateCreateRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	app := &domain.App{
		Name:           req.Name,
		AppType:        req.AppType,
		BuildMode:      normalizeBuildMode(req.BuildMode),
		RuntimeOwner:   runtimeOwnerForType(req.AppType),
		EdgeProvider:   edgeProviderForMode(h.proxyMode),
		Source:         sourceForType(req.AppType),
		RepoURL:        strings.TrimSpace(req.RepoURL),
		Branch:         req.Branch,
		InternalPort:   req.InternalPort,
		ImageRef:       strings.TrimSpace(req.ImageRef),
		Domain:         strings.TrimSpace(req.Domain),
		Status:         domain.AppStatusInactive,
		ContainerName:  strings.TrimSpace(req.ContainerName),
		ExternalTarget: strings.TrimSpace(req.ExternalTarget),
		DockerNetwork:  strings.TrimSpace(req.DockerNetwork),
	}

	if err := applyAccessFields(r.Context(), h.apps, h.serverHost, h.serverDomain, app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign port")
		return
	}

	if err := h.apps.CreateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create app")
		return
	}
	if err := syncNonManagedRoute(r.Context(), h.proxy, app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to configure route")
		return
	}

	writeJSON(w, http.StatusCreated, app)
}

func (h *AppHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "appID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}

	app, err := h.apps.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get app")
		return
	}

	var req updateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Branch != nil {
		app.Branch = *req.Branch
	}
	if req.InternalPort != nil {
		app.InternalPort = *req.InternalPort
	}
	if req.ImageRef != nil {
		app.ImageRef = strings.TrimSpace(*req.ImageRef)
	}
	if req.ContainerName != nil {
		app.ContainerName = strings.TrimSpace(*req.ContainerName)
	}
	if req.ExternalTarget != nil {
		app.ExternalTarget = strings.TrimSpace(*req.ExternalTarget)
	}
	if req.DockerNetwork != nil {
		app.DockerNetwork = strings.TrimSpace(*req.DockerNetwork)
	}
	if req.AppType != nil {
		app.AppType = normalizeAppType(*req.AppType)
		app.RuntimeOwner = runtimeOwnerForType(app.AppType)
		app.Source = sourceForType(app.AppType)
	}
	if req.Domain != nil {
		app.Domain = strings.TrimSpace(*req.Domain)
	}

	if err := applyAccessFields(r.Context(), h.apps, h.serverHost, h.serverDomain, app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign port")
		return
	}

	if err := h.apps.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update app")
		return
	}
	if err := syncNonManagedRoute(r.Context(), h.proxy, app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update route")
		return
	}

	writeJSON(w, http.StatusOK, app)
}

func (h *AppHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "appID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}

	app, err := h.apps.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get app")
		return
	}

	if err := h.apps.DeleteApp(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete app")
		return
	}
	if h.proxy != nil {
		_ = h.proxy.RemoveRoute(r.Context(), app.Name)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AppHandler) Restart(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "appID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}

	app, err := h.apps.GetApp(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get app")
		return
	}

	if app.AppType == domain.AppTypeExternalService {
		writeError(w, http.StatusBadRequest, "external services cannot be restarted")
		return
	}

	containerName := app.EffectiveContainerName()
	if containerName == "" {
		writeError(w, http.StatusBadRequest, "no container is associated with this app")
		return
	}

	if err := h.docker.Restart(r.Context(), containerName); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to restart container")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func normalizeAppType(v domain.AppType) domain.AppType {
	switch v {
	case domain.AppTypeAdoptedContainer, domain.AppTypeExternalService:
		return v
	default:
		return domain.AppTypeManaged
	}
}

func normalizeBuildMode(v domain.BuildMode) domain.BuildMode {
	switch v {
	case domain.BuildModeTalosBuild:
		return v
	default:
		return domain.BuildModeExternalCI
	}
}

func runtimeOwnerForType(appType domain.AppType) domain.RuntimeOwner {
	if appType == domain.AppTypeManaged {
		return domain.RuntimeOwnerTalos
	}
	return domain.RuntimeOwnerExternal
}

func sourceForType(appType domain.AppType) string {
	switch appType {
	case domain.AppTypeAdoptedContainer:
		return "docker"
	case domain.AppTypeExternalService:
		return "external"
	default:
		return "github"
	}
}

func edgeProviderForMode(mode config.ProxyMode) domain.EdgeProvider {
	if mode == config.ProxyModeExternal {
		return domain.EdgeProviderExternalTraefik
	}
	return domain.EdgeProviderInternalTraefik
}

func validateCreateRequest(req createAppRequest) error {
	switch req.AppType {
	case domain.AppTypeManaged:
		if strings.TrimSpace(req.RepoURL) == "" {
			return errors.New("repo_url is required for managed apps")
		}
	case domain.AppTypeAdoptedContainer:
		if strings.TrimSpace(req.ContainerName) == "" {
			return errors.New("container_name is required for adopted containers")
		}
	case domain.AppTypeExternalService:
		if strings.TrimSpace(req.ExternalTarget) == "" {
			return errors.New("external_target is required for external services")
		}
	}
	return nil
}

func applyAccessFields(ctx context.Context, apps store.AppStore, serverHost, serverDomain string, app *domain.App) error {
	app.FallbackPort = 0
	app.AccessURL = ""
	app.AccessMode = domain.AccessModePort

	if app.Domain != "" {
		app.AccessMode = domain.AccessModeDomain
		app.AccessURL = "https://" + app.Domain
		return nil
	}

	switch app.AppType {
	case domain.AppTypeManaged:
		port, err := apps.NextFallbackPort(ctx)
		if err != nil {
			return err
		}
		app.FallbackPort = port
		host := serverHost
		if serverDomain != "" {
			host = serverDomain
		}
		app.AccessURL = fmt.Sprintf("http://%s:%d", host, port)
	case domain.AppTypeAdoptedContainer:
		app.AccessURL = ""
	case domain.AppTypeExternalService:
		app.AccessURL = app.ExternalTarget
	}

	return nil
}

func syncNonManagedRoute(ctx context.Context, proxy *traefik.Manager, app *domain.App) error {
	if proxy == nil {
		return nil
	}
	if app.AppType == domain.AppTypeManaged {
		return nil
	}
	if app.Domain == "" {
		return proxy.RemoveRoute(ctx, app.Name)
	}
	if app.AppType == domain.AppTypeAdoptedContainer {
		return proxy.UpdateRoute(ctx, app, app.EffectiveContainerName())
	}
	if app.AppType == domain.AppTypeExternalService {
		return proxy.UpdateRoute(ctx, app, "")
	}
	return nil
}

func parseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
}
