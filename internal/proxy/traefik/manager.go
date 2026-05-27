package traefik

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/template"

	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
)

const traefikContainerName = "talos-traefik"

type Manager struct {
	configDir string
	dataDir   string
	network   string
	domain    string
	acmeEmail string
	logger    *slog.Logger
}

func NewManager(configDir, dataDir, network, domain, acmeEmail string, logger *slog.Logger) *Manager {
	return &Manager{
		configDir: configDir,
		dataDir:   dataDir,
		network:   network,
		domain:    domain,
		acmeEmail: acmeEmail,
		logger:    logger,
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

type routeData struct {
	Name          string
	Rule          string
	ContainerName string
	InternalPort  int
	EntryPoints   []string
	TLS           bool
}

func (m *Manager) UpdateRoute(ctx context.Context, app *domain.App, containerName string) error {
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

	switch app.AccessMode {
	case domain.AccessModeDomain:
		data.Rule = fmt.Sprintf("Host(`%s`)", app.Domain)
	case domain.AccessModePort:
		data.Rule = fmt.Sprintf("Host(`*`)")
	}

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

// EnsureTraefik starts the Traefik container if it's not already running.
// In IP mode (no domain), Traefik is not needed — the app listens directly.
func (m *Manager) EnsureTraefik(ctx context.Context, dc *docker.Client, image string) error {
	if m.domain == "" {
		m.logger.Info("no domain configured, skipping traefik")
		return nil
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

	// Pull the Traefik image.
	if err := dc.PullImage(ctx, image); err != nil {
		return fmt.Errorf("pull traefik image: %w", err)
	}

	// Start the Traefik container.
	staticConfigPath := filepath.Join(m.dataDir, "traefik.yml")
	_, err = dc.StartContainerWithConfig(ctx, docker.ContainerConfig{
		Name:     traefikContainerName,
		ImageRef: image,
		Volumes: []string{
			staticConfigPath + ":/etc/traefik/traefik.yml:ro",
			m.configDir + ":/etc/traefik/config:ro",
			m.dataDir + ":/data",
			"/var/run/docker.sock:/var/run/docker.sock:ro",
		},
		Labels: map[string]string{"managed-by": "talos"},
		Ports:  []string{"80:80", "443:443"},
	})
	if err != nil {
		return fmt.Errorf("start traefik: %w", err)
	}

	m.logger.Info("traefik started", "domain", m.domain)
	return nil
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
