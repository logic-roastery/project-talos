package settings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moby/moby/api/types/container"
)

const (
	defaultDockerEnvPath = "/opt/talos/.env"
	legacyBareEnvPath    = "/etc/talos/.env"
	systemdUnitPath      = "/etc/systemd/system/talos.service"
	talosBinaryPath      = "/usr/local/bin/talos"
)

type InstallMode string

const (
	InstallModeUnknown InstallMode = "unknown"
	InstallModeDocker  InstallMode = "docker"
	InstallModeBare    InstallMode = "bare"
)

type DockerInspector interface {
	Inspect(ctx context.Context, name string) (container.InspectResponse, error)
}

type Service struct {
	docker     DockerInspector
	envPath    string
	fileExists func(string) bool
	readFile   func(string) ([]byte, error)
	writeFile  func(string, []byte, os.FileMode) error
	renameFile func(string, string) error
	mkdirAll   func(string, os.FileMode) error
	removeFile func(string) error
}

type Snapshot struct {
	Domain           string
	ACMEEmail        string
	Mode             InstallMode
	EnvPath          string
	DockerImage      string
	ConfiguredURL    string
	ApplyCommand     string
	ApplyTitle       string
	ApplyDescription string
}

type UpdateInput struct {
	Domain    string
	ACMEEmail string
}

func NewService(docker DockerInspector) *Service {
	return &Service{
		docker:     docker,
		fileExists: defaultFileExists,
		readFile:   os.ReadFile,
		writeFile:  os.WriteFile,
		renameFile: os.Rename,
		mkdirAll:   os.MkdirAll,
		removeFile: os.Remove,
	}
}

func (s *Service) WithEnvPath(path string) *Service {
	s.envPath = path
	return s
}

func (s *Service) Load(ctx context.Context, fallbackHost string, port int) (Snapshot, error) {
	mode, dockerImage := s.detectInstallMode(ctx)
	envPath := s.resolveEnvPath(mode)

	values, err := s.readEnvValues(envPath)
	if err != nil && !os.IsNotExist(err) {
		return Snapshot{}, err
	}

	domain := strings.TrimSpace(values["TALOS_DOMAIN"])
	email := strings.TrimSpace(values["TALOS_ACME_EMAIL"])

	return Snapshot{
		Domain:           domain,
		ACMEEmail:        email,
		Mode:             mode,
		EnvPath:          envPath,
		DockerImage:      dockerImage,
		ConfiguredURL:    configuredURL(domain, fallbackHost, port),
		ApplyCommand:     applyCommand(mode, dockerImage, envPath, port),
		ApplyTitle:       applyTitle(mode),
		ApplyDescription: applyDescription(mode),
	}, nil
}

func (s *Service) Save(ctx context.Context, input UpdateInput, fallbackHost string, port int) (Snapshot, error) {
	mode, dockerImage := s.detectInstallMode(ctx)
	envPath := s.resolveEnvPath(mode)

	updates := map[string]string{
		"TALOS_DOMAIN":     strings.TrimSpace(input.Domain),
		"TALOS_ACME_EMAIL": strings.TrimSpace(input.ACMEEmail),
	}
	if err := s.updateEnvFile(envPath, updates); err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		Domain:           updates["TALOS_DOMAIN"],
		ACMEEmail:        updates["TALOS_ACME_EMAIL"],
		Mode:             mode,
		EnvPath:          envPath,
		DockerImage:      dockerImage,
		ConfiguredURL:    configuredURL(updates["TALOS_DOMAIN"], fallbackHost, port),
		ApplyCommand:     applyCommand(mode, dockerImage, envPath, port),
		ApplyTitle:       applyTitle(mode),
		ApplyDescription: applyDescription(mode),
	}, nil
}

func (s *Service) detectInstallMode(ctx context.Context) (InstallMode, string) {
	if s.docker != nil {
		if inspected, err := s.docker.Inspect(ctx, "talos"); err == nil {
			image := ""
			if inspected.Config != nil {
				image = inspected.Config.Image
			}
			return InstallModeDocker, image
		}
	}

	if s.fileExists(systemdUnitPath) && s.fileExists(talosBinaryPath) {
		return InstallModeBare, ""
	}

	return InstallModeUnknown, ""
}

func (s *Service) resolveEnvPath(mode InstallMode) string {
	if s.envPath != "" {
		return s.envPath
	}

	switch {
	case s.fileExists(defaultDockerEnvPath):
		return defaultDockerEnvPath
	case mode == InstallModeDocker || mode == InstallModeBare:
		return defaultDockerEnvPath
	case s.fileExists(legacyBareEnvPath):
		return legacyBareEnvPath
	case s.fileExists(".env"):
		return ".env"
	default:
		return defaultDockerEnvPath
	}
}

func (s *Service) readEnvValues(path string) (map[string]string, error) {
	data, err := s.readFile(path)
	if err != nil {
		return nil, err
	}

	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	return values, nil
}

func (s *Service) updateEnvFile(path string, updates map[string]string) error {
	data, err := s.readFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := []string{}
	if err == nil {
		lines = strings.Split(string(data), "\n")
	}

	remaining := map[string]string{}
	for key, value := range updates {
		remaining[key] = value
	}

	updated := make([]string, 0, len(lines)+len(updates))
	for _, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			updated = append(updated, line)
			continue
		}
		value, tracked := remaining[key]
		if !tracked {
			updated = append(updated, line)
			continue
		}
		delete(remaining, key)
		if value == "" {
			continue
		}
		updated = append(updated, key+"="+value)
	}

	preferredOrder := []string{"TALOS_DOMAIN", "TALOS_ACME_EMAIL"}
	seen := make(map[string]bool, len(remaining))
	for _, key := range preferredOrder {
		value, ok := remaining[key]
		if !ok {
			continue
		}
		seen[key] = true
		if value == "" {
			continue
		}
		updated = append(updated, key+"="+value)
	}

	keys := make([]string, 0, len(remaining))
	for key := range remaining {
		if seen[key] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := remaining[key]
		if value == "" {
			continue
		}
		updated = append(updated, key+"="+value)
	}

	output := strings.Join(updated, "\n")
	if output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	if err := s.mkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := s.writeFile(tmpPath, []byte(output), 0600); err != nil {
		return err
	}
	if err := s.renameFile(tmpPath, path); err != nil {
		_ = s.removeFile(tmpPath)
		return fmt.Errorf("replace env file: %w", err)
	}
	return nil
}

func configuredURL(domain, fallbackHost string, port int) string {
	domain = strings.TrimSpace(domain)
	if domain != "" {
		return "https://" + domain
	}

	host := strings.TrimSpace(fallbackHost)
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}

	if strings.Contains(host, ":") {
		return "http://" + host
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

func applyTitle(mode InstallMode) string {
	switch mode {
	case InstallModeDocker:
		return "Recreate the Talos container"
	case InstallModeBare:
		return "Restart the Talos service"
	default:
		return "Apply the saved configuration"
	}
}

func applyDescription(mode InstallMode) string {
	switch mode {
	case InstallModeDocker:
		return "Saving updates /opt/talos/.env, but the running Talos container keeps using the old values until you recreate it."
	case InstallModeBare:
		return "Saving updates the env file, but the running Talos process keeps using the old values until the systemd service restarts."
	default:
		return "Saving updates the env file, but Talos will not use the new values until the current runtime is restarted."
	}
}

func applyCommand(mode InstallMode, dockerImage, envPath string, port int) string {
	switch mode {
	case InstallModeDocker:
		image := dockerImage
		if image == "" {
			image = "ghcr.io/logic-roastery/project-talos:latest"
		}
		return fmt.Sprintf("docker stop talos\n"+
			"docker rm talos\n"+
			"docker run -d \\\n"+
			"  --name talos \\\n"+
			"  --restart unless-stopped \\\n"+
			"  --network talos \\\n"+
			"  -p %d:3000 \\\n"+
			"  -v /var/run/docker.sock:/var/run/docker.sock \\\n"+
			"  -v /opt/talos/data:/data \\\n"+
			"  --env-file %s \\\n"+
			"  %s", port, envPath, image)
	case InstallModeBare:
		return "sudo systemctl restart talos"
	default:
		return "Restart the running Talos process so it reloads " + envPath
	}
}

func defaultFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
