package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v62/github"
	"github.com/logic-roastery/project-talos/internal/config"
	"golang.org/x/crypto/nacl/box"
)

// AppClient wraps the GitHub App API client.
type AppClient struct {
	appID      int64
	appSlug    string
	privateKey *rsa.PrivateKey
	clientID   string

	mu            sync.Mutex
	installations map[int64]*installationToken // cached by installation ID
}

type installationToken struct {
	token     string
	expiresAt time.Time
}

// NewAppClient creates a GitHub App client from config.
func NewAppClient(cfg config.GitHubConfig) (*AppClient, error) {
	if cfg.AppID == 0 {
		return nil, fmt.Errorf("github app ID not configured")
	}
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("github app private key not configured")
	}

	key, err := ParsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	return &AppClient{
		appID:         cfg.AppID,
		appSlug:       cfg.AppSlug,
		privateKey:    key,
		clientID:      cfg.ClientID,
		installations: make(map[int64]*installationToken),
	}, nil
}

// IsConfigured returns true if the GitHub App is fully configured.
func (c *AppClient) IsConfigured() bool {
	return c != nil && c.appID != 0
}

// AppSlug returns the GitHub App slug for installation URLs.
func (c *AppClient) AppSlug() string {
	return c.appSlug
}

// AppClient returns a GitHub client authenticated as the app (for listing installations, etc).
func (c *AppClient) AppGitHubClient(ctx context.Context) (*github.Client, error) {
	jwt, err := GenerateJWT(c.appID, c.privateKey)
	if err != nil {
		return nil, fmt.Errorf("generate JWT: %w", err)
	}

	client := github.NewClient(nil).WithAuthToken(jwt)
	return client, nil
}

// InstallationClient returns a GitHub client scoped to a specific installation.
func (c *AppClient) InstallationClient(ctx context.Context, installationID int64) (*github.Client, error) {
	token, err := c.getInstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}

	client := github.NewClient(nil).WithAuthToken(token)
	return client, nil
}

// GetInstallationToken returns an installation access token for the given installation ID.
// This is useful for operations that need the raw token, such as git clone.
func (c *AppClient) GetInstallationToken(ctx context.Context, installationID int64) (string, error) {
	return c.getInstallationToken(ctx, installationID)
}

// getInstallationToken returns a cached or fresh installation access token.
func (c *AppClient) getInstallationToken(ctx context.Context, installationID int64) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check cache
	if tok, ok := c.installations[installationID]; ok {
		if time.Now().Before(tok.expiresAt.Add(-5 * time.Minute)) {
			return tok.token, nil
		}
	}

	// Generate fresh token
	jwtToken, err := GenerateJWT(c.appID, c.privateKey)
	if err != nil {
		return "", fmt.Errorf("generate JWT: %w", err)
	}

	// Use the installations API to get a token
	appClient := github.NewClient(nil).WithAuthToken(jwtToken)
	token, _, err := appClient.Apps.CreateInstallationToken(ctx, installationID, nil)
	if err != nil {
		return "", fmt.Errorf("create installation token: %w", err)
	}

	tok := token.GetToken()
	expires := token.GetExpiresAt().Time

	c.installations[installationID] = &installationToken{
		token:     tok,
		expiresAt: expires,
	}

	return tok, nil
}

// GetInstallation returns the installation details for a given installation ID.
func (c *AppClient) GetInstallation(ctx context.Context, installationID int64) (*github.Installation, error) {
	appClient, err := c.AppGitHubClient(ctx)
	if err != nil {
		return nil, err
	}

	install, _, err := appClient.Apps.GetInstallation(ctx, installationID)
	if err != nil {
		return nil, fmt.Errorf("get installation: %w", err)
	}

	return install, nil
}

// ListInstallations returns all installations of the GitHub App.
func (c *AppClient) ListInstallations(ctx context.Context) ([]*github.Installation, error) {
	appClient, err := c.AppGitHubClient(ctx)
	if err != nil {
		return nil, err
	}

	var all []*github.Installation
	opts := &github.ListOptions{PerPage: 100}

	for {
		installs, resp, err := appClient.Apps.ListInstallations(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list installations: %w", err)
		}
		all = append(all, installs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return all, nil
}

// ListInstallationRepos returns the list of repositories accessible to an installation.
func (c *AppClient) ListInstallationRepos(ctx context.Context, installationID int64) ([]*github.Repository, error) {
	client, err := c.InstallationClient(ctx, installationID)
	if err != nil {
		return nil, err
	}

	var allRepos []*github.Repository
	opts := &github.ListOptions{PerPage: 100}

	for {
		repos, resp, err := client.Apps.ListRepos(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list repos: %w", err)
		}
		allRepos = append(allRepos, repos.Repositories...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}

// ResolveBranchHeadSHA returns the HEAD commit SHA for the given repository branch.
func (c *AppClient) ResolveBranchHeadSHA(ctx context.Context, installationID int64, owner, repo, branch string) (string, error) {
	client, err := c.InstallationClient(ctx, installationID)
	if err != nil {
		return "", err
	}

	ref, _, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
	if err != nil {
		return "", fmt.Errorf("get branch ref: %w", err)
	}

	sha := ref.GetObject().GetSHA()
	if sha == "" {
		return "", fmt.Errorf("branch %q has no HEAD SHA", branch)
	}

	return sha, nil
}

// ParseRepoFullName extracts the owner/repo pair from a GitHub repository URL.
func ParseRepoFullName(repoURL string) (owner, repo string, err error) {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil {
		return "", "", fmt.Errorf("parse repo URL: %w", err)
	}
	if !strings.EqualFold(u.Host, "github.com") {
		return "", "", fmt.Errorf("unsupported repository host: %s", u.Host)
	}

	repoPath := strings.Trim(path.Clean(u.Path), "/")
	parts := strings.Split(repoPath, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid GitHub repository path: %s", u.Path)
	}

	owner = parts[0]
	repo = strings.TrimSuffix(parts[1], ".git")
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("invalid GitHub repository path: %s", u.Path)
	}

	return owner, repo, nil
}

// GetRepoContent fetches the content of a file or directory at the given path.
func (c *AppClient) GetRepoContent(ctx context.Context, installationID int64, owner, repo, path string) ([]*github.RepositoryContent, error) {
	client, err := c.InstallationClient(ctx, installationID)
	if err != nil {
		return nil, err
	}

	_, directoryContent, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		// If path is a file, GetContents returns an error for directory listing
		fileContent, _, _, err2 := client.Repositories.GetContents(ctx, owner, repo, path, nil)
		if err2 != nil {
			return nil, fmt.Errorf("get contents: %w", err)
		}
		return []*github.RepositoryContent{fileContent}, nil
	}

	return directoryContent, nil
}

// CreateOrUpdateFile creates or updates a file in the repository.
func (c *AppClient) CreateOrUpdateFile(ctx context.Context, installationID int64, owner, repo, path string, content []byte, message string) error {
	client, err := c.InstallationClient(ctx, installationID)
	if err != nil {
		return err
	}

	// Check if file exists
	existing, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		// Create new file
		opts := &github.RepositoryContentFileOptions{
			Content: content,
			Message: github.String(message),
		}
		_, _, err = client.Repositories.CreateFile(ctx, owner, repo, path, opts)
		if err != nil {
			return fmt.Errorf("create file: %w", err)
		}
		return nil
	}

	// Update existing file
	opts := &github.RepositoryContentFileOptions{
		Content: content,
		Message: github.String(message),
		SHA:     existing.SHA,
	}
	_, _, err = client.Repositories.UpdateFile(ctx, owner, repo, path, opts)
	if err != nil {
		return fmt.Errorf("update file: %w", err)
	}
	return nil
}

// SetRepoSecret sets a GitHub Actions secret on a repository.
// Uses nacl/box to encrypt the value with the repo's public key.
func (c *AppClient) SetRepoSecret(ctx context.Context, installationID int64, owner, repo, name, value string) error {
	client, err := c.InstallationClient(ctx, installationID)
	if err != nil {
		return err
	}

	key, _, err := client.Actions.GetRepoPublicKey(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("get public key: %w", err)
	}

	publicKeyBytes, err := base64.StdEncoding.DecodeString(key.GetKey())
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}

	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	ephemeralPub, ephemeralPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ephemeral key: %w", err)
	}

	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	encrypted := box.Seal(nonce[:], []byte(value), &nonce, &publicKey, ephemeralPriv)

	// Prepend the ephemeral public key (GitHub expects this).
	payload := append(ephemeralPub[:], encrypted...)

	_, err = client.Actions.CreateOrUpdateRepoSecret(ctx, owner, repo, &github.EncryptedSecret{
		Name:           name,
		KeyID:          key.GetKeyID(),
		EncryptedValue: base64.StdEncoding.EncodeToString(payload),
	})
	if err != nil {
		return fmt.Errorf("set secret: %w", err)
	}

	return nil
}

// NewRequest creates a new HTTP request with the installation auth.
// This is a helper for making raw API calls if needed.
func (c *AppClient) NewRequest(ctx context.Context, installationID int64, method, url string, body interface{}) (*http.Request, error) {
	client, err := c.InstallationClient(ctx, installationID)
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	return req, nil
}
