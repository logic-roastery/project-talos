package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
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

func (h *DeployHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	deployID, err := parseID(r, "deployID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid deploy id")
		return
	}

	events, err := h.deploys.ListDeployEvents(r.Context(), deployID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}

	// Return HTML partial for HTMX requests, JSON otherwise
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<td colspan="6" class="px-4 py-3 bg-surface/50"><div class="space-y-1">`)
		if len(events) == 0 {
			fmt.Fprintf(w, `<div class="text-xs text-muted">No events recorded yet.</div>`)
		}
		for _, e := range events {
			color := "text-muted"
			if e.Level == "error" {
				color = "text-red-400"
			} else if e.Level == "warn" {
				color = "text-yellow-400"
			}
			fmt.Fprintf(w, `<div class="text-xs %s flex gap-2"><span class="font-mono text-[10px] opacity-60">%s</span><span class="font-semibold">%s</span><span>%s</span></div>`,
				color, e.Timestamp.Format("15:04:05"), e.Step, e.Message)
		}
		fmt.Fprintf(w, `</div></td>`)
		return
	}

	writeJSON(w, http.StatusOK, events)
}
