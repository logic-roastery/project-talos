package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/github"
	"github.com/logic-roastery/project-talos/internal/store"
)

type GitHubHandler struct {
	apps     store.AppStore
	ghClient *github.AppClient
	cfg      config.GitHubConfig
	host     string
	logger   *slog.Logger
}

func NewGitHubHandler(apps store.AppStore, ghClient *github.AppClient, cfg config.GitHubConfig, host string, logger *slog.Logger) *GitHubHandler {
	return &GitHubHandler{
		apps:     apps,
		ghClient: ghClient,
		cfg:      cfg,
		host:     host,
		logger:   logger,
	}
}

// StartInstall redirects the user to the GitHub App installation page.
func (h *GitHubHandler) StartInstall(w http.ResponseWriter, r *http.Request) {
	if h.ghClient == nil || !h.ghClient.IsConfigured() {
		http.Error(w, "GitHub App not configured", http.StatusServiceUnavailable)
		return
	}

	appID := r.URL.Query().Get("app_id")
	if appID == "" {
		http.Error(w, "app_id required", http.StatusBadRequest)
		return
	}

	// Verify the app exists
	id, err := strconv.ParseInt(appID, 10, 64)
	if err != nil {
		http.Error(w, "invalid app_id", http.StatusBadRequest)
		return
	}

	_, err = h.apps.GetApp(r.Context(), id)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	// Redirect to GitHub App installation page
	// The state parameter carries the Talos app ID
	slug := h.ghClient.AppSlug()
	redirectURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s", slug, appID)

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// HandleCallback handles the GitHub App installation callback.
func (h *GitHubHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if h.ghClient == nil || !h.ghClient.IsConfigured() {
		http.Error(w, "GitHub App not configured", http.StatusServiceUnavailable)
		return
	}

	installationIDStr := r.URL.Query().Get("installation_id")
	state := r.URL.Query().Get("state") // Contains our app_id

	if installationIDStr == "" || state == "" {
		http.Error(w, "missing installation_id or state", http.StatusBadRequest)
		return
	}

	installationID, err := strconv.ParseInt(installationIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid installation_id", http.StatusBadRequest)
		return
	}

	appID, err := strconv.ParseInt(state, 10, 64)
	if err != nil {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	// Get the app
	app, err := h.apps.GetApp(r.Context(), appID)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	// Get installation details to find the repo
	_, err = h.ghClient.GetInstallation(r.Context(), installationID)
	if err != nil {
		h.logger.Error("failed to get installation", "error", err)
		http.Error(w, "failed to get installation", http.StatusInternalServerError)
		return
	}

	// Get the first repo from the installation
	repos, err := h.ghClient.ListInstallationRepos(r.Context(), installationID)
	if err != nil {
		h.logger.Error("failed to list repos", "error", err)
		http.Error(w, "failed to list repos", http.StatusInternalServerError)
		return
	}

	if len(repos) == 0 {
		http.Error(w, "no repos found in installation", http.StatusBadRequest)
		return
	}

	// Find the repo that matches the app's repo URL
	var matchedRepo *github.RepositorySummary
	for _, repo := range repos {
		// Match by repo full name (owner/repo)
		repoFullName := repo.GetFullName()
		if strings.Contains(app.RepoURL, repoFullName) {
			matchedRepo = &github.RepositorySummary{
				ID:       repo.GetID(),
				FullName: repoFullName,
			}
			break
		}
	}

	if matchedRepo == nil {
		// If no match found, use the first repo
		matchedRepo = &github.RepositorySummary{
			ID:       repos[0].GetID(),
			FullName: repos[0].GetFullName(),
		}
	}

	// Update the app with GitHub installation info
	app.GitHubInstallationID = &installationID
	app.GitHubRepoID = &matchedRepo.ID
	app.RegistryURL = "ghcr.io"

	if err := h.apps.UpdateApp(r.Context(), app); err != nil {
		h.logger.Error("failed to update app", "error", err)
		http.Error(w, "failed to update app", http.StatusInternalServerError)
		return
	}

	// Generate and commit the workflow
	if err := h.setupWorkflow(r.Context(), app, matchedRepo, installationID); err != nil {
		h.logger.Error("failed to setup workflow", "error", err)
		// Don't fail the request - the app is connected, workflow can be set up later
	}

	// Redirect to the app detail page
	http.Redirect(w, r, fmt.Sprintf("/apps/%d", appID), http.StatusFound)
}

// setupWorkflow generates and commits the GitHub Actions workflow.
func (h *GitHubHandler) setupWorkflow(ctx context.Context, app *domain.App, repo *github.RepositorySummary, installationID int64) error {
	// Extract owner/repo from full name
	parts := strings.Split(repo.FullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo full name: %s", repo.FullName)
	}
	owner, repoName := parts[0], parts[1]

	// Generate workflow YAML
	workflowCfg := github.WorkflowConfig{
		AppName:    app.Name,
		ImageRef:   fmt.Sprintf("ghcr.io/%s:%s", repo.FullName, "{{ github.sha }}"),
		Branch:     app.Branch,
		WebhookURL: fmt.Sprintf("http://%s", h.host),
	}
	workflowYAML := github.GenerateWorkflow(workflowCfg)

	// Commit the workflow file
	if err := h.ghClient.CreateOrUpdateFile(ctx, installationID, owner, repoName,
		".github/workflows/talos-deploy.yml", []byte(workflowYAML), "Add Talos deploy workflow"); err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}

	h.logger.Info("workflow created", "repo", repo.FullName, "app", app.Name)
	return nil
}

// Disconnect removes the GitHub connection from an app.
func (h *GitHubHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	appIDStr := r.URL.Query().Get("app_id")
	if appIDStr == "" {
		appIDStr = r.PathValue("appID")
	}
	if appIDStr == "" {
		http.Error(w, "app_id required", http.StatusBadRequest)
		return
	}

	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid app_id", http.StatusBadRequest)
		return
	}

	app, err := h.apps.GetApp(r.Context(), appID)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	app.GitHubInstallationID = nil
	app.GitHubRepoID = nil
	app.RegistryURL = ""

	if err := h.apps.UpdateApp(r.Context(), app); err != nil {
		h.logger.Error("failed to disconnect app", "error", err)
		http.Error(w, "failed to disconnect", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
