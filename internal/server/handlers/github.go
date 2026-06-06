package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/github"
	"github.com/logic-roastery/project-talos/internal/store"
	"github.com/logic-roastery/project-talos/web"
)

type GitHubHandler struct {
	apps     store.AppStore
	ghClient *github.AppClient
	cfg      config.GitHubConfig
	renderer *web.Renderer
	host     string
	domain   string
	logger   *slog.Logger
}

func NewGitHubHandler(apps store.AppStore, ghClient *github.AppClient, cfg config.GitHubConfig, renderer *web.Renderer, host, domain string, logger *slog.Logger) *GitHubHandler {
	return &GitHubHandler{
		apps:     apps,
		ghClient: ghClient,
		cfg:      cfg,
		renderer: renderer,
		host:     host,
		domain:   domain,
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
		AppName:  app.Name,
		ImageRef: fmt.Sprintf("ghcr.io/%s:%s", repo.FullName, "{{ github.sha }}"),
		Branch:   app.Branch,
		WebhookURL: func() string {
			if h.domain != "" {
				return "https://" + h.domain
			}
			return fmt.Sprintf("http://%s", h.host)
		}(),
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

// SetupPage shows the GitHub App setup page.
func (h *GitHubHandler) SetupPage(w http.ResponseWriter, r *http.Request) {
	// Check if already configured
	if h.ghClient != nil && h.ghClient.IsConfigured() {
		http.Redirect(w, r, "/settings/github/status", http.StatusFound)
		return
	}

	// Build a proper URL
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = h.host
	}
	if host == "0.0.0.0" || host == "0.0.0.0:0" {
		host = "localhost:4000"
	}
	talosURL := fmt.Sprintf("%s://%s", scheme, host)

	user := UserFromContext(r.Context())
	var userData *web.UserData
	if user != nil {
		userData = &web.UserData{Username: user.Username}
	}

	data := struct {
		TalosURL  string
		HasDomain bool
	}{
		TalosURL:  talosURL,
		HasDomain: h.domain != "",
	}

	h.renderer.Render(w, "github_setup.html", "GitHub App Setup", userData, data)
}

// CreateManifest generates a manifest and redirects to GitHub.
func (h *GitHubHandler) CreateManifest(w http.ResponseWriter, r *http.Request) {
	var talosURL string
	if h.domain != "" {
		talosURL = "https://" + h.domain
	} else {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := r.Host
		if host == "" {
			host = h.host
		}
		if host == "0.0.0.0" || host == "0.0.0.0:0" {
			host = "localhost:4000"
		}
		talosURL = fmt.Sprintf("%s://%s", scheme, host)
	}

	manifestCfg := github.ManifestConfig{
		AppName:  "talos-deploy",
		TalosURL: talosURL,
	}

	manifest, err := github.GenerateManifest(manifestCfg)
	if err != nil {
		h.logger.Error("failed to generate manifest", "error", err)
		http.Error(w, "failed to generate manifest", http.StatusInternalServerError)
		return
	}

	encoded, err := github.EncodeManifest(manifest)
	if err != nil {
		h.logger.Error("failed to encode manifest", "error", err)
		http.Error(w, "failed to encode manifest", http.StatusInternalServerError)
		return
	}

	redirectURL := fmt.Sprintf("https://github.com/settings/apps/new?manifest=%s", url.QueryEscape(encoded))
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// SetupCallback handles the callback from GitHub after app creation.
func (h *GitHubHandler) SetupCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		return
	}

	// Exchange code for app credentials
	exchangeURL := fmt.Sprintf("https://api.github.com/app-manifests/%s/conversions", code)

	req, err := http.NewRequest("POST", exchangeURL, nil)
	if err != nil {
		h.logger.Error("failed to create request", "error", err)
		http.Error(w, "failed to exchange code", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.logger.Error("failed to exchange code", "error", err)
		http.Error(w, "failed to exchange code", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		h.logger.Error("github returned error", "status", resp.StatusCode)
		http.Error(w, "failed to exchange code", http.StatusInternalServerError)
		return
	}

	var appData github.ManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&appData); err != nil {
		h.logger.Error("failed to decode response", "error", err)
		http.Error(w, "failed to decode response", http.StatusInternalServerError)
		return
	}

	// Save credentials to file
	if err := h.saveCredentials(&appData); err != nil {
		h.logger.Error("failed to save credentials", "error", err)
		http.Error(w, "failed to save credentials", http.StatusInternalServerError)
		return
	}

	h.logger.Info("github app created", "app_id", appData.ID, "slug", appData.Slug)

	// Show success page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>GitHub App Created - Talos</title></head>
<body style="background:#030712;color:#e5e7eb;font-family:monospace;padding:2rem;">
<h1 style="color:#4ade80;">GitHub App Created Successfully!</h1>
<br>
<p><strong>App ID:</strong> %d</p>
<p><strong>App Name:</strong> %s</p>
<p><strong>Slug:</strong> %s</p>
<br>
<p>Add these environment variables to your Talos configuration:</p>
<pre style="background:#1f2937;padding:1rem;border-radius:0.375rem;overflow-x:auto;">
TALOS_GITHUB_APP_ID=%d
TALOS_GITHUB_APP_SLUG=%s
TALOS_GITHUB_APP_CLIENT_ID=%s
TALOS_GITHUB_APP_CLIENT_SECRET=%s
TALOS_GITHUB_APP_PRIVATE_KEY="<contents of the PEM key below>"
TALOS_GITHUB_APP_WEBHOOK_SECRET=%s
</pre>
<br>
<p><strong>Private Key:</strong></p>
<pre style="background:#1f2937;padding:1rem;border-radius:0.375rem;overflow-x:auto;font-size:0.75rem;">%s</pre>
<br>
<p style="color:#fbbf24;">Important: Save the private key above. You won't be able to see it again!</p>
<br>
<a href="/dashboard" style="background:#4ade80;color:#030712;padding:0.75rem 1.5rem;text-decoration:none;border-radius:0.375rem;font-weight:bold;">
    Go to Dashboard
</a>
</body>
</html>`,
		appData.ID, appData.Name, appData.Slug,
		appData.ID, appData.Slug, appData.ClientID, appData.ClientSecret,
		appData.WebhookSecret, appData.PEM)
}

// saveCredentials saves the GitHub App credentials to a JSON file.
func (h *GitHubHandler) saveCredentials(data *github.ManifestResponse) error {
	credsDir := "data"
	credsPath := filepath.Join(credsDir, "github-app.json")

	// Ensure directory exists
	if err := os.MkdirAll(credsDir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Marshal credentials
	creds, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	// Write to file
	if err := os.WriteFile(credsPath, creds, 0600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	return nil
}

// StatusPage shows the current GitHub App configuration status.
func (h *GitHubHandler) StatusPage(w http.ResponseWriter, r *http.Request) {
	isConfigured := h.ghClient != nil && h.ghClient.IsConfigured()

	appSlug := ""
	if isConfigured {
		appSlug = h.ghClient.AppSlug()
	}

	user := UserFromContext(r.Context())
	var userData *web.UserData
	if user != nil {
		userData = &web.UserData{Username: user.Username}
	}

	data := struct {
		IsConfigured bool
		AppSlug      string
	}{
		IsConfigured: isConfigured,
		AppSlug:      appSlug,
	}

	h.renderer.Render(w, "github_status.html", "GitHub Integration", userData, data)
}

// LoadCredentials loads GitHub App credentials from the JSON file.
func LoadCredentials(dataDir string) (*github.ManifestResponse, error) {
	credsPath := filepath.Join(dataDir, "github-app.json")

	data, err := os.ReadFile(credsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No credentials file
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	var creds github.ManifestResponse
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("unmarshal credentials: %w", err)
	}

	return &creds, nil
}
