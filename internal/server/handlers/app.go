package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/store"
)

type AppHandler struct {
	apps         store.AppStore
	deploys      store.DeployStore
	serverHost   string
	serverDomain string
}

func NewAppHandler(apps store.AppStore, deploys store.DeployStore, serverHost, serverDomain string) *AppHandler {
	return &AppHandler{apps: apps, deploys: deploys, serverHost: serverHost, serverDomain: serverDomain}
}

type createAppRequest struct {
	Name         string `json:"name"`
	RepoURL      string `json:"repo_url"`
	Branch       string `json:"branch"`
	InternalPort int    `json:"internal_port"`
	Domain       string `json:"domain,omitempty"`
}

type updateAppRequest struct {
	Branch       *string `json:"branch,omitempty"`
	InternalPort *int    `json:"internal_port,omitempty"`
	Domain       *string `json:"domain,omitempty"`
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

	if req.Name == "" || req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "name and repo_url are required")
		return
	}

	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.InternalPort == 0 {
		req.InternalPort = 3000
	}

	accessMode := domain.AccessModePort
	accessURL := ""
	var fallbackPort int

	if req.Domain != "" {
		accessMode = domain.AccessModeDomain
		accessURL = "https://" + req.Domain
	} else {
		port, err := h.apps.NextFallbackPort(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to assign port")
			return
		}
		fallbackPort = port
		host := h.serverHost
		if h.serverDomain != "" {
			host = h.serverDomain
		}
		accessURL = fmt.Sprintf("http://%s:%d", host, port)
	}

	app := &domain.App{
		Name:         req.Name,
		Source:       "github",
		RepoURL:      req.RepoURL,
		Branch:       req.Branch,
		InternalPort: req.InternalPort,
		Domain:       req.Domain,
		FallbackPort: fallbackPort,
		AccessMode:   accessMode,
		AccessURL:    accessURL,
		Status:       domain.AppStatusInactive,
	}

	if err := h.apps.CreateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create app")
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
	if req.Domain != nil {
		app.Domain = *req.Domain
		if *req.Domain != "" {
			app.AccessMode = domain.AccessModeDomain
			app.AccessURL = "https://" + *req.Domain
		}
	}

	if err := h.apps.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update app")
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

	if err := h.apps.DeleteApp(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete app")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
}
