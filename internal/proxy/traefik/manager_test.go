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
	mgr := NewManager(dir, dir, "talos", "traefik-public", "talos.example.com", "ops@example.com", "letsencrypt", "websecure", config.ProxyModeInternal, 3000, slog.Default())

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
	mgr := NewManager(dir, dir, "talos", "traefik-public", "talos.example.com", "ops@example.com", "letsencrypt", "websecure", config.ProxyModeInternal, 3000, slog.Default())

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

func TestUpdateRouteNoopsForDomainAppsInExternalMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, dir, "talos", "traefik-public", "talos.example.com", "ops@example.com", "letsencrypt", "websecure", config.ProxyModeExternal, 3000, slog.Default())

	err := mgr.UpdateRoute(context.Background(), &domain.App{
		Name:         "demo",
		Domain:       "app.example.com",
		AccessMode:   domain.AccessModeDomain,
		InternalPort: 3000,
	}, "talos-demo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "demo.yml")); !os.IsNotExist(err) {
		t.Fatalf("expected no route file in external mode, got err=%v", err)
	}
}

func TestExternalLabelsForDomainApps(t *testing.T) {
	t.Parallel()

	mgr := NewManager("", "", "talos", "traefik-public", "talos.example.com", "ops@example.com", "letsencrypt", "websecure", config.ProxyModeExternal, 3000, slog.Default())

	labels := mgr.ExternalLabels(&domain.App{
		Name:         "demo",
		Domain:       "app.example.com",
		AccessMode:   domain.AccessModeDomain,
		InternalPort: 8080,
	})

	if labels["traefik.enable"] != "true" {
		t.Fatalf("expected traefik enable label, got %#v", labels)
	}
	if labels["traefik.docker.network"] != "traefik-public" {
		t.Fatalf("unexpected docker network label: %#v", labels)
	}
	if labels["traefik.http.routers.demo.rule"] != "Host(`app.example.com`)" {
		t.Fatalf("unexpected router rule: %#v", labels)
	}
	if labels["traefik.http.services.demo.loadbalancer.server.port"] != "8080" {
		t.Fatalf("unexpected service port label: %#v", labels)
	}
}

func TestExternalNetworksForDomainApps(t *testing.T) {
	t.Parallel()

	mgr := NewManager("", "", "talos", "traefik-public", "talos.example.com", "ops@example.com", "letsencrypt", "websecure", config.ProxyModeExternal, 3000, slog.Default())
	networks := mgr.ExternalNetworks(&domain.App{
		AccessMode: domain.AccessModeDomain,
		Domain:     "app.example.com",
	})

	if len(networks) != 1 || networks[0] != "traefik-public" {
		t.Fatalf("unexpected external networks: %#v", networks)
	}
}
