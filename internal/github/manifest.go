package github

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Manifest represents a GitHub App manifest for creating apps.
type Manifest struct {
	Name               string              `json:"name"`
	URL                string              `json:"url"`
	HookAttributes     HookAttributes      `json:"hook_attributes"`
	RedirectURL        string              `json:"redirect_url"`
	SetupURL           string              `json:"setup_url,omitempty"`
	Description        string              `json:"description,omitempty"`
	Public             bool                `json:"public"`
	DefaultPermissions ManifestPermissions `json:"default_permissions"`
	DefaultEvents      []string            `json:"default_events"`
}

type HookAttributes struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

type ManifestPermissions struct {
	Contents string `json:"contents"`
	Actions  string `json:"actions"`
	Metadata string `json:"metadata"`
	Packages string `json:"packages"`
}

// ManifestConfig contains the configuration for generating a manifest.
type ManifestConfig struct {
	AppName     string // Base name for the app
	TalosURL    string // Base URL of the Talos instance
	Description string
}

// GenerateManifest creates a GitHub App manifest JSON.
func GenerateManifest(cfg ManifestConfig) (*Manifest, error) {
	if cfg.TalosURL == "" {
		return nil, fmt.Errorf("talos URL is required")
	}

	name := cfg.AppName
	if name == "" {
		name = "talos-deploy"
	}

	description := cfg.Description
	if description == "" {
		description = "Auto-deploy your apps with Talos"
	}

	manifest := &Manifest{
		Name: name,
		URL:  cfg.TalosURL,
		HookAttributes: HookAttributes{
			URL:    cfg.TalosURL + "/api/webhooks/github",
			Active: true,
		},
		RedirectURL: cfg.TalosURL + "/api/github/setup-callback",
		Description: description,
		Public:      false,
		DefaultPermissions: ManifestPermissions{
			Contents: "write",
			Actions:  "write",
			Metadata: "read",
			Packages: "read",
		},
		DefaultEvents: []string{
			"workflow_run",
			"installation",
		},
	}

	return manifest, nil
}

// EncodeManifest encodes a manifest to base64 for the URL parameter.
func EncodeManifest(manifest *Manifest) (string, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// ManifestCallbackData contains the data returned by GitHub after app creation.
type ManifestCallbackData struct {
	Code string `json:"code"`
}

// ManifestResponse contains the credentials returned by GitHub.
type ManifestResponse struct {
	ID            int64               `json:"id"`
	Slug          string              `json:"slug"`
	Name          string              `json:"name"`
	ClientID      string              `json:"client_id"`
	ClientSecret  string              `json:"client_secret"`
	WebhookSecret string              `json:"webhook_secret"`
	PEM           string              `json:"pem"`
	HTMLURL       string              `json:"html_url"`
	Permissions   ManifestPermissions `json:"permissions"`
	Events        []string            `json:"events"`
}
