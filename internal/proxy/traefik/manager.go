package traefik

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
)

const traefikContainerName = "talos-traefik"

var ErrExternalProxyAppDomainsUnsupported = errors.New("custom app domains require internal proxy mode")

type Manager struct {
	configDir        string
	dataDir          string
	hostDataRoot     string // host-path equivalent of the data root (e.g. /opt/talos/data for /data when running in Docker)
	network          string
	edgeNetwork      string
	domain           string
	acmeEmail        string
	edgeCertResolver string
	edgeEntrypoint   string
	proxyMode        config.ProxyMode
	serverPort       int
	logger           *slog.Logger
}

func NewManager(configDir, dataDir, hostDataRoot, network, edgeNetwork, domain, acmeEmail, edgeCertResolver, edgeEntrypoint string, proxyMode config.ProxyMode, serverPort int, logger *slog.Logger) *Manager {
	return &Manager{
		configDir:        configDir,
		dataDir:          dataDir,
		hostDataRoot:     hostDataRoot,
		network:          network,
		edgeNetwork:      edgeNetwork,
		domain:           domain,
		acmeEmail:        acmeEmail,
		edgeCertResolver: edgeCertResolver,
		edgeEntrypoint:   edgeEntrypoint,
		proxyMode:        proxyMode,
		serverPort:       serverPort,
		logger:           logger,
	}
}

func (m *Manager) Init(ctx context.Context) error {
	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return nil
}

var appRouteTemplate = template.Must(template.New("route").Parse(`http:
  routers:
    {{.Name}}:
      rule: "{{.Rule}}"
      service: "{{.Name}}"
      entryPoints:{{range .EntryPoints}}
        - {{.}}{{end}}{{if .TLS}}
      tls:
        certResolver: letsencrypt{{end}}
  services:
    {{.Name}}:
      loadBalancer:
        servers:
          - url: "http://{{.ContainerName}}:{{.InternalPort}}"
`))

var staticRouteTemplate = template.Must(template.New("static-route").Parse(`http:
  routers:
    {{.Name}}:
      rule: "{{.Rule}}"
      service: "{{.Name}}"
      entryPoints:
        - websecure
      tls:
        certResolver: letsencrypt
  services:
    {{.Name}}:
      loadBalancer:
        servers:
          - url: "{{.TargetURL}}"
`))

type routeData struct {
	Name          string
	Rule          string
	ContainerName string
	InternalPort  int
	EntryPoints   []string
	TLS           bool
}

type staticRouteData struct {
	Name      string
	Rule      string
	TargetURL string
}

func (m *Manager) UpdateRoute(ctx context.Context, app *domain.App, containerName string) error {
	if app.AccessMode == domain.AccessModePort {
		return nil
	}
	if m.proxyMode == config.ProxyModeExternal {
		return nil
	}
	if app.AppType == domain.AppTypeExternalService && app.ExternalTarget != "" {
		path := filepath.Join(m.configDir, app.Name+".yml")
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create route file: %w", err)
		}
		defer f.Close()

		data := staticRouteData{
			Name:      app.Name,
			Rule:      fmt.Sprintf("Host(`%s`)", app.Domain),
			TargetURL: app.ExternalTarget,
		}
		if err := staticRouteTemplate.Execute(f, data); err != nil {
			return fmt.Errorf("write static route: %w", err)
		}
		m.logger.Info("external service route updated", "app", app.Name, "target", app.ExternalTarget)
		return nil
	}

	data := routeData{
		Name:          app.Name,
		ContainerName: containerName,
		InternalPort:  app.InternalPort,
	}

	if m.domain != "" {
		data.EntryPoints = []string{"websecure"}
		data.TLS = true
	} else {
		data.EntryPoints = []string{"web"}
	}

	data.Rule = fmt.Sprintf("Host(`%s`)", app.Domain)

	path := filepath.Join(m.configDir, app.Name+".yml")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create route file: %w", err)
	}
	defer f.Close()

	if err := appRouteTemplate.Execute(f, data); err != nil {
		return fmt.Errorf("write route: %w", err)
	}

	m.logger.Info("route updated", "app", app.Name, "mode", app.AccessMode, "rule", data.Rule)
	return nil
}

func (m *Manager) EnsureTalosRoute(ctx context.Context, installMode string) error {
	if m.domain == "" {
		return nil
	}

	targetURL := fmt.Sprintf("http://talos:%d", m.serverPort)
	if installMode != "docker" {
		targetURL = fmt.Sprintf("http://host.docker.internal:%d", m.serverPort)
	}

	path := filepath.Join(m.configDir, "talos-ui.yml")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create talos route file: %w", err)
	}
	defer f.Close()

	data := staticRouteData{
		Name:      "talos-ui",
		Rule:      fmt.Sprintf("Host(`%s`)", m.domain),
		TargetURL: targetURL,
	}
	if err := staticRouteTemplate.Execute(f, data); err != nil {
		return fmt.Errorf("write talos route: %w", err)
	}

	m.logger.Info("talos ui route updated", "domain", m.domain, "target", targetURL, "install_mode", installMode)
	return nil
}

func (m *Manager) RemoveRoute(ctx context.Context, appName string) error {
	path := filepath.Join(m.configDir, appName+".yml")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove route: %w", err)
	}
	return nil
}

// DomainMode returns true if a domain is configured.
func (m *Manager) DomainMode() bool {
	return m.domain != ""
}

func (m *Manager) SupportsAppDomains() bool {
	return true
}

func (m *Manager) RequiresExclusiveSwitch(app *domain.App) bool {
	return m.proxyMode == config.ProxyModeExternal && app.AccessMode == domain.AccessModeDomain
}

func (m *Manager) ExternalNetworks(app *domain.App) []string {
	if m.proxyMode != config.ProxyModeExternal || app.AccessMode != domain.AccessModeDomain || m.edgeNetwork == "" {
		return nil
	}
	if m.edgeNetwork == m.network {
		return nil
	}
	return []string{m.edgeNetwork}
}

func (m *Manager) ExternalLabels(app *domain.App) map[string]string {
	if m.proxyMode != config.ProxyModeExternal || app.AccessMode != domain.AccessModeDomain || app.Domain == "" {
		return nil
	}

	serviceName := app.Name
	return map[string]string{
		"traefik.enable":         "true",
		"traefik.docker.network": m.edgeNetworkOrDefault(),
		fmt.Sprintf("traefik.http.routers.%s.rule", serviceName):                      fmt.Sprintf("Host(`%s`)", app.Domain),
		fmt.Sprintf("traefik.http.routers.%s.entrypoints", serviceName):               m.edgeEntrypointOrDefault(),
		fmt.Sprintf("traefik.http.routers.%s.tls", serviceName):                       "true",
		fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", serviceName):          m.edgeCertResolverOrDefault(),
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName): fmt.Sprintf("%d", app.InternalPort),
	}
}

func (m *Manager) edgeNetworkOrDefault() string {
	if m.edgeNetwork == "" {
		return "traefik-public"
	}
	return m.edgeNetwork
}

func (m *Manager) edgeCertResolverOrDefault() string {
	if m.edgeCertResolver == "" {
		return "letsencrypt"
	}
	return m.edgeCertResolver
}

func (m *Manager) edgeEntrypointOrDefault() string {
	if m.edgeEntrypoint == "" {
		return "websecure"
	}
	return m.edgeEntrypoint
}

// EnsureTraefik starts the Traefik container if it's not already running.
// In IP mode (no domain), Traefik is not needed — the app listens directly.
func (m *Manager) EnsureTraefik(ctx context.Context, dc *docker.Client, image string) error {
	if m.proxyMode == config.ProxyModeExternal {
		m.logger.Info("external proxy mode configured, skipping talos-managed traefik")
		return nil
	}
	if m.domain == "" {
		m.logger.Info("no domain configured, skipping traefik")
		return nil
	}

	installMode := "bare"
	if _, err := dc.Inspect(ctx, "talos"); err == nil {
		installMode = "docker"
	}

	if err := m.EnsureTalosRoute(ctx, installMode); err != nil {
		return fmt.Errorf("write talos ui route: %w", err)
	}

	// Check if container already exists and is running.
	info, err := dc.Inspect(ctx, traefikContainerName)
	if err == nil && info.State.Running {
		m.logger.Info("traefik already running")
		return nil
	}

	// Stop and remove stale container if present.
	dc.StopAndRemove(ctx, traefikContainerName)

	// Generate static config.
	if err := m.writeStaticConfig(); err != nil {
		return fmt.Errorf("write static config: %w", err)
	}

	// Ensure the config file exists and is a file (not a directory).
	// Docker creates directories for non-existent mount sources.
	cfgPath := filepath.Join(m.dataDir, "traefik.yml")
	if info, err := os.Stat(cfgPath); err == nil && info.IsDir() {
		os.RemoveAll(cfgPath)
		if err := m.writeStaticConfig(); err != nil {
			return fmt.Errorf("rewrite static config: %w", err)
		}
	}

	// Pull the Traefik image.
	if err := dc.PullImage(ctx, image); err != nil {
		return fmt.Errorf("pull traefik image: %w", err)
	}

	// Build volume mount specs using host paths when available.
	// When Talos runs inside a Docker container, the Docker daemon resolves
	// volume source paths on the host filesystem — container-relative paths
	// like /data/… would be auto-created as empty directories, breaking Traefik.
	staticConfigHost := m.hostPath(filepath.Join(m.dataDir, "traefik.yml"))
	configDirHost := m.hostPath(m.configDir)
	dataDirHost := m.hostPath(m.dataDir)

	// Start the Traefik container.
	_, err = dc.StartContainerWithConfig(ctx, docker.ContainerConfig{
		Name:     traefikContainerName,
		ImageRef: image,
		Volumes: []string{
			staticConfigHost + ":/etc/traefik/traefik.yml:ro",
			configDirHost + ":/etc/traefik/config:ro",
			dataDirHost + ":/data",
			"/var/run/docker.sock:/var/run/docker.sock:ro",
		},
		Labels: map[string]string{"managed-by": "talos"},
		Ports:  []string{"80:80", "443:443"},
		ExtraHosts: []string{
			"host.docker.internal:host-gateway",
		},
	})
	if err != nil {
		return fmt.Errorf("start traefik: %w", err)
	}

	m.logger.Info("traefik started", "domain", m.domain)
	return nil
}

// hostPath translates a container-relative path to its host-equivalent.
// When hostDataRoot is empty (bare-metal mode), returns the path unchanged.
// When set, replaces the /data prefix with the host data root directory.
func (m *Manager) hostPath(containerPath string) string {
	if m.hostDataRoot == "" {
		return containerPath
	}
	if strings.HasPrefix(containerPath, "/data/") {
		return strings.Replace(containerPath, "/data", m.hostDataRoot, 1)
	}
	if containerPath == "/data" {
		return m.hostDataRoot
	}
	return containerPath
}

func (m *Manager) writeStaticConfig() error {
	cfg := fmt.Sprintf(`api:
  dashboard: false
  insecure: false

entryPoints:
  web:
    address: ":80"
    http:
      redirections:
        entryPoint:
          to: websecure
          scheme: https
  websecure:
    address: ":443"

certificatesResolvers:
  letsencrypt:
    acme:
      email: %s
      storage: /data/acme.json
      httpChallenge:
        entryPoint: web

providers:
  file:
    directory: /etc/traefik/config
    watch: true

log:
  level: WARN
`, m.acmeEmail)

	path := filepath.Join(m.dataDir, "traefik.yml")
	return os.WriteFile(path, []byte(cfg), 0644)
}
