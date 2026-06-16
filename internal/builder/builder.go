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

type ProgressFunc func(level, step, message string)

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
	return b.CloneAndBuildWithProgress(ctx, app, commitSHA, nil)
}

// CloneAndBuildWithProgress clones the repository and builds a Docker image while
// emitting optional progress callbacks for UI/event reporting.
func (b *Builder) CloneAndBuildWithProgress(ctx context.Context, app *domain.App, commitSHA string, progress ProgressFunc) (*CloneAndBuildResult, error) {
	if app.GitHubInstallationID == nil {
		return nil, fmt.Errorf("app %d has no GitHub installation", app.ID)
	}
	if commitSHA == "" {
		return nil, fmt.Errorf("commit SHA is required for talos_build")
	}

	// Get installation token for cloning
	emitProgress(progress, "info", "build_auth", "requesting GitHub installation token")
	token, err := b.ghClient.GetInstallationToken(ctx, *app.GitHubInstallationID)
	if err != nil {
		return nil, fmt.Errorf("get installation token: %w", err)
	}
	emitProgress(progress, "info", "build_auth", "GitHub installation token ready")

	// Create temporary directory for clone
	cloneDir := filepath.Join(b.dataDir, "builds", fmt.Sprintf("%d-%s", app.ID, shortCommitSHA(commitSHA)))
	if err := os.MkdirAll(cloneDir, 0755); err != nil {
		return nil, fmt.Errorf("create clone dir: %w", err)
	}
	defer os.RemoveAll(cloneDir)

	// Clone the repository
	repoURL := b.buildAuthURL(app.RepoURL, token)
	if err := b.cloneRepo(ctx, repoURL, app.Branch, commitSHA, cloneDir, progress); err != nil {
		return nil, fmt.Errorf("clone repo: %w", err)
	}

	// Build Docker image (auto-detects if no Dockerfile)
	imageRef := b.buildImageRef(app, commitSHA)
	result, err := b.buildImage(ctx, cloneDir, imageRef, string(app.ProjectType), app.InternalPort, progress)
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
	shortSHA := shortCommitSHA(commitSHA)

	// Extract owner/repo from RepoURL
	parts := strings.Split(strings.TrimPrefix(app.RepoURL, "https://github.com/"), "/")
	if len(parts) >= 2 {
		owner := parts[0]
		repo := strings.TrimSuffix(parts[1], ".git")
		return fmt.Sprintf("%s/%s/%s:%s", registry, owner, repo, shortSHA)
	}

	// Fallback to app name
	return fmt.Sprintf("%s/%s:%s", registry, app.Name, shortSHA)
}

// cloneRepo clones the repository to the specified directory.
func (b *Builder) cloneRepo(ctx context.Context, repoURL, branch, commitSHA, destDir string, progress ProgressFunc) error {
	// Clone with depth 1 for efficiency
	emitProgress(progress, "info", "clone", "cloning repository branch "+branch)
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
	emitProgress(progress, "info", "clone", "repository cloned successfully")

	// Checkout specific commit if not HEAD
	if commitSHA != "" {
		emitProgress(progress, "info", "clone", "checking out commit "+shortCommitSHA(commitSHA))
		checkoutCmd := exec.CommandContext(ctx, "git", "checkout", commitSHA)
		checkoutCmd.Dir = destDir
		output, err := checkoutCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git checkout failed: %s: %w", string(output), err)
		}
		emitProgress(progress, "info", "clone", "checked out commit "+shortCommitSHA(commitSHA))
	}

	return nil
}

// buildImage builds a Docker image from the specified directory.
// If no Dockerfile exists, auto-detects the project type and generates one.
func (b *Builder) buildImage(ctx context.Context, buildDir, imageRef, projectType string, configuredPort int, progress ProgressFunc) (*CloneAndBuildResult, error) {
	dockerfilePath := filepath.Join(buildDir, "Dockerfile")
	result := &CloneAndBuildResult{ImageRef: imageRef}

	// If no Dockerfile, auto-detect and generate one
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		emitProgress(progress, "info", "build_detect", "detecting project type and generating Dockerfile")
		plan, err := detect.DetectAs(buildDir, projectType)
		if err != nil {
			return nil, fmt.Errorf("no Dockerfile and auto-detection failed: %w", err)
		}
		plan.Port = effectivePlanPort(plan.Port, configuredPort)
		if projectType != "" {
			b.logger.Info("using configured project type",
				"provider", plan.Provider,
				"runtime", plan.Runtime,
				"port", plan.Port,
			)
		} else {
			b.logger.Info("auto-detected project",
				"provider", plan.Provider,
				"runtime", plan.Runtime,
				"port", plan.Port,
			)
		}
		dockerfile := detect.GenerateDockerfile(plan)
		if err := os.WriteFile(dockerfilePath, dockerfile, 0644); err != nil {
			return nil, fmt.Errorf("write generated Dockerfile: %w", err)
		}
		result.Port = plan.Port
		result.Provider = plan.Provider
		emitProgress(progress, "info", "build_detect", fmt.Sprintf("generated %s Dockerfile on port %d", plan.Provider, plan.Port))
	} else {
		emitProgress(progress, "info", "build_detect", "using repository Dockerfile")
	}

	// Build the image using docker build
	emitProgress(progress, "info", "build_image", "building Docker image "+imageRef)
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
	emitProgress(progress, "info", "build_image", "Docker image built successfully")
	return result, nil
}

func shortCommitSHA(commitSHA string) string {
	if len(commitSHA) >= 7 {
		return commitSHA[:7]
	}
	if commitSHA != "" {
		return commitSHA
	}
	return "manual"
}

func effectivePlanPort(detectedPort, configuredPort int) int {
	if configuredPort > 0 {
		return configuredPort
	}
	return detectedPort
}

func emitProgress(progress ProgressFunc, level, step, message string) {
	if progress != nil {
		progress(level, step, message)
	}
}
