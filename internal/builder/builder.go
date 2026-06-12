package builder

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/logic-roastery/project-talos/internal/builder/detect"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/github"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
)

// Builder handles git clone and Docker build operations for talos_build mode.
type Builder struct {
	ghClient *github.AppClient
	docker   *docker.Client
	logger   *slog.Logger
	dataDir  string
}

// CloneAndBuildResult holds the build output.
type CloneAndBuildResult struct {
	ImageRef string
	Port     int    // detected port (0 if not detected / Dockerfile existed)
	Provider string // detected provider name ("" if Dockerfile was used)
}

// NewBuilder creates a new Builder instance.
func NewBuilder(ghClient *github.AppClient, docker *docker.Client, logger *slog.Logger, dataDir string) *Builder {
	return &Builder{
		ghClient: ghClient,
		docker:   docker,
		logger:   logger,
		dataDir:  dataDir,
	}
}

// CloneAndBuild clones the repository and builds a Docker image.
// Returns the build result with image reference, detected port, and provider.
func (b *Builder) CloneAndBuild(ctx context.Context, app *domain.App, commitSHA string) (*CloneAndBuildResult, error) {
	if app.GitHubInstallationID == nil {
		return nil, fmt.Errorf("app %d has no GitHub installation", app.ID)
	}

	// Get installation token for cloning
	token, err := b.ghClient.GetInstallationToken(ctx, *app.GitHubInstallationID)
	if err != nil {
		return nil, fmt.Errorf("get installation token: %w", err)
	}

	// Create temporary directory for clone
	cloneDir := filepath.Join(b.dataDir, "builds", fmt.Sprintf("%d-%s", app.ID, commitSHA[:7]))
	if err := os.MkdirAll(cloneDir, 0755); err != nil {
		return nil, fmt.Errorf("create clone dir: %w", err)
	}
	defer os.RemoveAll(cloneDir)

	// Clone the repository
	repoURL := b.buildAuthURL(app.RepoURL, token)
	if err := b.cloneRepo(ctx, repoURL, app.Branch, commitSHA, cloneDir); err != nil {
		return nil, fmt.Errorf("clone repo: %w", err)
	}

	// Build Docker image (auto-detects if no Dockerfile)
	imageRef := b.buildImageRef(app, commitSHA)
	result, err := b.buildImage(ctx, cloneDir, imageRef)
	if err != nil {
		return nil, fmt.Errorf("build image: %w", err)
	}

	b.logger.Info("build completed", "app", app.Name, "image", imageRef)
	return result, nil
}

// buildAuthURL constructs an authenticated URL for cloning.
func (b *Builder) buildAuthURL(repoURL, token string) string {
	// Convert https://github.com/owner/repo to https://x-access-token:TOKEN@github.com/owner/repo
	if strings.HasPrefix(repoURL, "https://") {
		return strings.Replace(repoURL, "https://", fmt.Sprintf("https://x-access-token:%s@", token), 1)
	}
	return repoURL
}

// buildImageRef constructs the Docker image reference.
func (b *Builder) buildImageRef(app *domain.App, commitSHA string) string {
	registry := app.RegistryURL
	if registry == "" {
		registry = "ghcr.io"
	}

	// Extract owner/repo from RepoURL
	parts := strings.Split(strings.TrimPrefix(app.RepoURL, "https://github.com/"), "/")
	if len(parts) >= 2 {
		owner := parts[0]
		repo := strings.TrimSuffix(parts[1], ".git")
		return fmt.Sprintf("%s/%s/%s:%s", registry, owner, repo, commitSHA[:7])
	}

	// Fallback to app name
	return fmt.Sprintf("%s/%s:%s", registry, app.Name, commitSHA[:7])
}

// cloneRepo clones the repository to the specified directory.
func (b *Builder) cloneRepo(ctx context.Context, repoURL, branch, commitSHA, destDir string) error {
	// Clone with depth 1 for efficiency
	cmd := exec.CommandContext(ctx, "git", "clone",
		"--depth", "1",
		"--branch", branch,
		repoURL,
		destDir,
	)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(output), err)
	}

	// Checkout specific commit if not HEAD
	if commitSHA != "" {
		checkoutCmd := exec.CommandContext(ctx, "git", "checkout", commitSHA)
		checkoutCmd.Dir = destDir
		output, err := checkoutCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git checkout failed: %s: %w", string(output), err)
		}
	}

	return nil
}

// buildImage builds a Docker image from the specified directory.
// If no Dockerfile exists, auto-detects the project type and generates one.
func (b *Builder) buildImage(ctx context.Context, buildDir, imageRef string) (*CloneAndBuildResult, error) {
	dockerfilePath := filepath.Join(buildDir, "Dockerfile")
	result := &CloneAndBuildResult{ImageRef: imageRef}

	// If no Dockerfile, auto-detect and generate one
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		plan, err := detect.Detect(buildDir)
		if err != nil {
			return nil, fmt.Errorf("no Dockerfile and auto-detection failed: %w", err)
		}
		b.logger.Info("auto-detected project",
			"provider", plan.Provider,
			"runtime", plan.Runtime,
			"port", plan.Port,
		)
		dockerfile := detect.GenerateDockerfile(plan)
		if err := os.WriteFile(dockerfilePath, dockerfile, 0644); err != nil {
			return nil, fmt.Errorf("write generated Dockerfile: %w", err)
		}
		result.Port = plan.Port
		result.Provider = plan.Provider
	}

	// Build the image using docker build
	cmd := exec.CommandContext(ctx, "docker", "build",
		"-t", imageRef,
		".",
	)
	cmd.Dir = buildDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker build failed: %s: %w", string(output), err)
	}

	b.logger.Info("docker image built", "image", imageRef)
	return result, nil
}
