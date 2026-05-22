package traefik

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/template"

	"github.com/logic-roastery/project-talos/internal/domain"
)

type Manager struct {
	configDir string
	dataDir   string
	network   string
	logger    *slog.Logger
}

func NewManager(configDir, dataDir, network string, logger *slog.Logger) *Manager {
	return &Manager{
		configDir: configDir,
		dataDir:   dataDir,
		network:   network,
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
      entryPoints:
        - web
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
}

func (m *Manager) UpdateRoute(ctx context.Context, app *domain.App, containerName string) error {
	data := routeData{
		Name:          app.Name,
		ContainerName: containerName,
		InternalPort:  app.InternalPort,
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
