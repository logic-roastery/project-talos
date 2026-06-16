package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/logic-roastery/project-talos/internal/deploy"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/store"
)

type DeployHandler struct {
	apps    store.AppStore
	deploys store.DeployStore
	engine  *deploy.Engine
	events  *deploy.EventBroadcaster
}

func NewDeployHandler(apps store.AppStore, deploys store.DeployStore, engine *deploy.Engine) *DeployHandler {
	var events *deploy.EventBroadcaster
	if engine != nil {
		events = engine.EventBroadcaster()
	}
	return &DeployHandler{apps: apps, deploys: deploys, engine: engine, events: events}
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
		renderDeployEventsHTML(w, events)
		return
	}

	writeJSON(w, http.StatusOK, events)
}

func (h *DeployHandler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	deployID, err := parseID(r, "deployID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid deploy id")
		return
	}

	current, err := h.deploys.GetDeploy(r.Context(), deployID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "deploy not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get deploy")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	events, err := h.deploys.ListDeployEvents(r.Context(), deployID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}

	for _, event := range events {
		if err := writeSSEEvent(w, "deploy_event", event); err != nil {
			return
		}
	}
	if err := writeSSEEvent(w, "status", map[string]string{
		"state":         "connected",
		"deploy_status": string(current.Status),
	}); err != nil {
		return
	}
	flusher.Flush()

	if current.Status.IsTerminal() {
		_ = writeSSEEvent(w, "status", map[string]string{
			"state":         "terminal",
			"deploy_status": string(current.Status),
		})
		flusher.Flush()
		return
	}

	if h.events == nil {
		return
	}

	ch, unsubscribe := h.events.Subscribe(deployID)
	defer unsubscribe()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, "deploy_event", event); err != nil {
				return
			}

			current, err = h.deploys.GetDeploy(r.Context(), deployID)
			if err != nil {
				return
			}
			if current.Status.IsTerminal() {
				if err := writeSSEEvent(w, "status", map[string]string{
					"state":         "terminal",
					"deploy_status": string(current.Status),
				}); err != nil {
					return
				}
				flusher.Flush()
				return
			}
			flusher.Flush()
		}
	}
}

func renderDeployEventsHTML(w http.ResponseWriter, events []*domain.DeployEvent) {
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
}

func writeSSEEvent(w http.ResponseWriter, eventName string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, data)
	return err
}
