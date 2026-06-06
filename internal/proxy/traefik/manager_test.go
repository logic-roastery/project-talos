package traefik

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/logic-roastery/project-talos/internal/domain"
)

func TestEnsureTalosRouteDockerMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, dir, "talos", "talos.example.com", "ops@example.com", config.ProxyModeInternal, 3000, slog.Default())

	if err := mgr.EnsureTalosRoute(context.Background(), "docker"); err != nil {
		t.Fatalf("ensure talos route: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "talos-ui.yml"))
	if err != nil {
		t.Fatalf("read route file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Host(`talos.example.com`)") {
		t.Fatalf("expected host rule in route file, got:\n%s", content)
	}
	if !strings.Contains(content, `url: "http://talos:3000"`) {
		t.Fatalf("expected docker target in route file, got:\n%s", content)
	}
}

func TestEnsureTalosRouteBareMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, dir, "talos", "talos.example.com", "ops@example.com", config.ProxyModeInternal, 3000, slog.Default())

	if err := mgr.EnsureTalosRoute(context.Background(), "bare"); err != nil {
		t.Fatalf("ensure talos route: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "talos-ui.yml"))
	if err != nil {
		t.Fatalf("read route file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `url: "http://host.docker.internal:3000"`) {
		t.Fatalf("expected bare target in route file, got:\n%s", content)
	}
}

func TestUpdateRouteRejectsDomainAppsInExternalMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, dir, "talos", "talos.example.com", "ops@example.com", config.ProxyModeExternal, 3000, slog.Default())

	err := mgr.UpdateRoute(context.Background(), &domain.App{
		Name:         "demo",
		Domain:       "app.example.com",
		AccessMode:   domain.AccessModeDomain,
		InternalPort: 3000,
	}, "talos-demo")
	if err != ErrExternalProxyAppDomainsUnsupported {
		t.Fatalf("unexpected error: %v", err)
	}
}
