package settings

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/logic-roastery/project-talos/internal/config"
	"github.com/moby/moby/api/types/container"
)

type fakeDockerInspector struct {
	response container.InspectResponse
	err      error
}

func (f fakeDockerInspector) Inspect(context.Context, string) (container.InspectResponse, error) {
	return f.response, f.err
}

func TestSavePreservesUnrelatedKeysAndRemovesEmptyValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	initial := "# Talos\nTALOS_DOMAIN=old.example.com\nKEEP_ME=1\nTALOS_ACME_EMAIL=old@example.com\n"
	if err := os.WriteFile(envPath, []byte(initial), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	svc := NewService(nil).WithEnvPath(envPath)
	svc.fileExists = func(path string) bool { return path == envPath }

	snapshot, err := svc.Save(context.Background(), UpdateInput{}, "127.0.0.1", 3000)
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}

	got, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}

	want := "# Talos\nKEEP_ME=1\n\nTALOS_PROXY_MODE=internal\nTALOS_EDGE_NETWORK=traefik-public\nTALOS_EDGE_CERT_RESOLVER=letsencrypt\nTALOS_EDGE_ENTRYPOINT=websecure\nTALOS_EDGE_PROVIDER=traefik\n"
	if string(got) != want {
		t.Fatalf("env file mismatch\nwant:\n%s\ngot:\n%s", want, string(got))
	}
	if snapshot.Domain != "" || snapshot.ACMEEmail != "" {
		t.Fatalf("expected empty values after save, got %+v", snapshot)
	}
}

func TestSaveCreatesEnvFileWithConfiguredValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	svc := NewService(nil).WithEnvPath(envPath)
	svc.fileExists = func(path string) bool { return path == envPath }

	_, err := svc.Save(context.Background(), UpdateInput{
		Domain:           "talos.example.com",
		ACMEEmail:        "ops@example.com",
		ProxyMode:        config.ProxyModeExternal,
		EdgeProvider:     config.EdgeProviderTraefik,
		EdgeNetwork:      "traefik-public",
		EdgeCertResolver: "letsencrypt",
		EdgeEntrypoint:   "websecure",
	}, "0.0.0.0", 3000)
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}

	got, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}

	want := "TALOS_DOMAIN=talos.example.com\nTALOS_ACME_EMAIL=ops@example.com\nTALOS_PROXY_MODE=external\nTALOS_EDGE_NETWORK=traefik-public\nTALOS_EDGE_CERT_RESOLVER=letsencrypt\nTALOS_EDGE_ENTRYPOINT=websecure\nTALOS_EDGE_PROVIDER=traefik\n"
	if string(got) != want {
		t.Fatalf("env file mismatch\nwant:\n%s\ngot:\n%s", want, string(got))
	}
}

func TestLoadDetectsDockerModeAndBuildsApplyCommand(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("TALOS_DOMAIN=talos.example.com\nTALOS_PROXY_MODE=external\n"), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	svc := NewService(fakeDockerInspector{
		response: container.InspectResponse{
			Config: &container.Config{Image: "ghcr.io/example/talos:v1"},
		},
	}).WithEnvPath(envPath)
	svc.fileExists = func(path string) bool { return path == envPath }

	snapshot, err := svc.Load(context.Background(), "0.0.0.0", 3000)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	if snapshot.Mode != InstallModeDocker {
		t.Fatalf("expected docker mode, got %s", snapshot.Mode)
	}
	if snapshot.ConfiguredURL != "https://talos.example.com" {
		t.Fatalf("unexpected configured URL: %s", snapshot.ConfiguredURL)
	}
	if snapshot.DockerImage != "ghcr.io/example/talos:v1" {
		t.Fatalf("unexpected docker image: %s", snapshot.DockerImage)
	}
	if snapshot.ProxyMode != config.ProxyModeExternal {
		t.Fatalf("unexpected proxy mode: %s", snapshot.ProxyMode)
	}
	if !strings.Contains(snapshot.ApplyCommand, "traefik.enable=true") {
		t.Fatalf("expected external proxy labels in apply command, got:\n%s", snapshot.ApplyCommand)
	}
	if snapshot.ApplyCommand == "" || snapshot.ApplyTitle == "" {
		t.Fatalf("expected apply guidance, got %+v", snapshot)
	}
}

func TestLoadDetectsBareModeWithoutDockerContainer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(""), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	svc := NewService(fakeDockerInspector{err: os.ErrNotExist}).WithEnvPath(envPath)
	svc.fileExists = func(path string) bool {
		return path == envPath || path == systemdUnitPath || path == talosBinaryPath
	}

	snapshot, err := svc.Load(context.Background(), "127.0.0.1", 3000)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	if snapshot.Mode != InstallModeBare {
		t.Fatalf("expected bare mode, got %s", snapshot.Mode)
	}
	if snapshot.ApplyCommand != "sudo systemctl restart talos" {
		t.Fatalf("unexpected apply command: %s", snapshot.ApplyCommand)
	}
	if snapshot.ConfiguredURL != "http://127.0.0.1:3000" {
		t.Fatalf("unexpected configured URL: %s", snapshot.ConfiguredURL)
	}
}
