package traefik

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureTalosRouteDockerMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mgr := NewManager(dir, dir, "talos", "talos.example.com", "ops@example.com", 3000, slog.Default())

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
	mgr := NewManager(dir, dir, "talos", "talos.example.com", "ops@example.com", 3000, slog.Default())

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
