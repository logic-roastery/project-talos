package settings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/logic-roastery/project-talos/internal/config"
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
	Domain             string
	ACMEEmail          string
	ProxyMode          config.ProxyMode
	EdgeNetwork        string
	EdgeCertResolver   string
	Mode               InstallMode
	EnvPath            string
	DockerImage        string
	ConfiguredURL      string
	ApplyCommand       string
	ApplyTitle         string
	ApplyDescription   string
	SupportsAppDomains bool
}

type UpdateInput struct {
	Domain           string
	ACMEEmail        string
	ProxyMode        config.ProxyMode
	EdgeNetwork      string
	EdgeCertResolver string
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
	proxyMode := parseProxyMode(values["TALOS_PROXY_MODE"])
	edgeNetwork := defaultString(values["TALOS_EDGE_NETWORK"], "traefik-public")
	edgeCertResolver := defaultString(values["TALOS_EDGE_CERT_RESOLVER"], "letsencrypt")

	return Snapshot{
		Domain:             domain,
		ACMEEmail:          email,
		ProxyMode:          proxyMode,
		EdgeNetwork:        edgeNetwork,
		EdgeCertResolver:   edgeCertResolver,
		Mode:               mode,
		EnvPath:            envPath,
		DockerImage:        dockerImage,
		ConfiguredURL:      configuredURL(domain, fallbackHost, port),
		ApplyCommand:       applyCommand(mode, proxyMode, dockerImage, envPath, port, edgeNetwork, edgeCertResolver, domain),
		ApplyTitle:         applyTitle(mode, proxyMode),
		ApplyDescription:   applyDescription(mode, proxyMode),
		SupportsAppDomains: proxyMode == config.ProxyModeInternal,
	}, nil
}

func (s *Service) Save(ctx context.Context, input UpdateInput, fallbackHost string, port int) (Snapshot, error) {
	mode, dockerImage := s.detectInstallMode(ctx)
	envPath := s.resolveEnvPath(mode)

	updates := map[string]string{
		"TALOS_DOMAIN":             strings.TrimSpace(input.Domain),
		"TALOS_ACME_EMAIL":         strings.TrimSpace(input.ACMEEmail),
		"TALOS_PROXY_MODE":         string(parseProxyMode(string(input.ProxyMode))),
		"TALOS_EDGE_NETWORK":       defaultString(input.EdgeNetwork, "traefik-public"),
		"TALOS_EDGE_CERT_RESOLVER": defaultString(input.EdgeCertResolver, "letsencrypt"),
	}
	if err := s.updateEnvFile(envPath, updates); err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		Domain:             updates["TALOS_DOMAIN"],
		ACMEEmail:          updates["TALOS_ACME_EMAIL"],
		ProxyMode:          parseProxyMode(updates["TALOS_PROXY_MODE"]),
		EdgeNetwork:        updates["TALOS_EDGE_NETWORK"],
		EdgeCertResolver:   updates["TALOS_EDGE_CERT_RESOLVER"],
		Mode:               mode,
		EnvPath:            envPath,
		DockerImage:        dockerImage,
		ConfiguredURL:      configuredURL(updates["TALOS_DOMAIN"], fallbackHost, port),
		ApplyCommand:       applyCommand(mode, parseProxyMode(updates["TALOS_PROXY_MODE"]), dockerImage, envPath, port, updates["TALOS_EDGE_NETWORK"], updates["TALOS_EDGE_CERT_RESOLVER"], updates["TALOS_DOMAIN"]),
		ApplyTitle:         applyTitle(mode, parseProxyMode(updates["TALOS_PROXY_MODE"])),
		ApplyDescription:   applyDescription(mode, parseProxyMode(updates["TALOS_PROXY_MODE"])),
		SupportsAppDomains: parseProxyMode(updates["TALOS_PROXY_MODE"]) == config.ProxyModeInternal,
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

	preferredOrder := []string{
		"TALOS_DOMAIN",
		"TALOS_ACME_EMAIL",
		"TALOS_PROXY_MODE",
		"TALOS_EDGE_NETWORK",
		"TALOS_EDGE_CERT_RESOLVER",
	}
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

func applyTitle(mode InstallMode, proxyMode config.ProxyMode) string {
	switch mode {
	case InstallModeDocker:
		if proxyMode == config.ProxyModeExternal {
			return "Recreate Talos and reconnect it to the shared edge proxy network"
		}
		return "Recreate the Talos container"
	case InstallModeBare:
		return "Restart the Talos service"
	default:
		return "Apply the saved configuration"
	}
}

func applyDescription(mode InstallMode, proxyMode config.ProxyMode) string {
	switch mode {
	case InstallModeDocker:
		if proxyMode == config.ProxyModeExternal {
			return "Saving updates /opt/talos/.env, but the running Talos container keeps its old environment and external-proxy labels until you recreate it and reconnect it to the shared edge network."
		}
		return "Saving updates /opt/talos/.env, but the running Talos container keeps using the old values until you recreate it."
	case InstallModeBare:
		if proxyMode == config.ProxyModeExternal {
			return "Saving updates the env file, but the running Talos process and your external proxy keep using the old values until Talos restarts and the proxy route is refreshed."
		}
		return "Saving updates the env file, but the running Talos process keeps using the old values until the systemd service restarts."
	default:
		return "Saving updates the env file, but Talos will not use the new values until the current runtime is restarted."
	}
}

func applyCommand(mode InstallMode, proxyMode config.ProxyMode, dockerImage, envPath string, port int, edgeNetwork, edgeCertResolver, domain string) string {
	switch mode {
	case InstallModeDocker:
		image := dockerImage
		if image == "" {
			image = "ghcr.io/logic-roastery/project-talos:latest"
		}
		if proxyMode == config.ProxyModeExternal {
			lines := []string{
				"docker pull " + image,
				"docker stop talos",
				"docker rm talos",
				"docker network inspect " + edgeNetwork + " >/dev/null 2>&1 || docker network create " + edgeNetwork,
				"docker run -d \\",
				"  --name talos \\",
				"  --restart unless-stopped \\",
				"  --network talos \\",
				fmt.Sprintf("  -p %d:3000 \\", port),
				"  -v /var/run/docker.sock:/var/run/docker.sock \\",
				"  -v /opt/talos/data:/data \\",
				fmt.Sprintf("  -v %s:%s \\", envPath, envPath),
				fmt.Sprintf("  --env-file %s \\", envPath),
			}
			if domain != "" {
				lines = append(lines,
					"  --label traefik.enable=true \\",
					fmt.Sprintf("  --label traefik.docker.network=%s \\", edgeNetwork),
					fmt.Sprintf("  --label traefik.http.routers.talos.rule=Host(`%s`) \\", domain),
					"  --label traefik.http.routers.talos.entrypoints=websecure \\",
					"  --label traefik.http.routers.talos.tls=true \\",
					fmt.Sprintf("  --label traefik.http.routers.talos.tls.certresolver=%s \\", edgeCertResolver),
					"  --label traefik.http.services.talos.loadbalancer.server.port=3000 \\",
				)
			}
			lines = append(lines,
				"  "+image,
				"docker network connect "+edgeNetwork+" talos",
			)
			return strings.Join(lines, "\n")
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
			"  -v %s:%s \\\n"+
			"  --env-file %s \\\n"+
			"  %s", port, envPath, envPath, envPath, image)
	case InstallModeBare:
		if proxyMode == config.ProxyModeExternal && domain != "" {
			return "sudo systemctl restart talos\n# Then update your external proxy to route Host(`" + domain + "`) to http://<server-ip-or-host>:3000"
		}
		return "sudo systemctl restart talos"
	default:
		return "Restart the running Talos process so it reloads " + envPath
	}
}

func defaultFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func parseProxyMode(v string) config.ProxyMode {
	switch config.ProxyMode(strings.TrimSpace(v)) {
	case config.ProxyModeExternal:
		return config.ProxyModeExternal
	default:
		return config.ProxyModeInternal
	}
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}
