package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/logic-roastery/project-talos/internal/deploy"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/store"
)

type DeployHandler struct {
	apps    store.AppStore
	deploys store.DeployStore
	engine  *deploy.Engine
}

func NewDeployHandler(apps store.AppStore, deploys store.DeployStore, engine *deploy.Engine) *DeployHandler {
	return &DeployHandler{apps: apps, deploys: deploys, engine: engine}
}

type triggerDeployRequest struct {
	ImageRef  string `json:"image_ref"`
	CommitSHA string `json:"commit_sha,omitempty"`
	Branch    string `json:"branch"`
}

func (h *DeployHandler) List(w http.ResponseWriter, r *http.Request) {
	appID, err := parseID(r, "appID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}

	deploys, err := h.deploys.ListDeploys(r.Context(), appID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list deploys")
		return
	}

	writeJSON(w, http.StatusOK, deploys)
}

func (h *DeployHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "deployID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid deploy id")
		return
	}

	d, err := h.deploys.GetDeploy(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "deploy not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get deploy")
		return
	}

	writeJSON(w, http.StatusOK, d)
}

func (h *DeployHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	appID, err := parseID(r, "appID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}

	var req triggerDeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ImageRef == "" || req.Branch == "" {
		writeError(w, http.StatusBadRequest, "image_ref and branch are required")
		return
	}

	d, err := h.engine.Deploy(r.Context(), appID, req.ImageRef, req.CommitSHA, req.Branch, "manual")
	if err != nil {
		if errors.Is(err, domain.ErrDeployInProgress) {
			writeError(w, http.StatusConflict, "deploy already in progress")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to trigger deploy")
		return
	}

	writeJSON(w, http.StatusAccepted, d)
}

func (h *DeployHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	appID, err := parseID(r, "appID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}

	d, err := h.engine.Rollback(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rollback")
		return
	}

	writeJSON(w, http.StatusAccepted, d)
}
