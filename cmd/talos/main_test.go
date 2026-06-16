package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEnvFilePathPrefersKnownPersistentLocation(t *testing.T) {
	t.Setenv("TALOS_ENV_FILE", "")

	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "talos.env")
	if err := os.WriteFile(envPath, []byte("TALOS_SESSION_SECRET=test\n"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("TALOS_ENV_FILE", envPath)

	if got := resolveEnvFilePath(); got != envPath {
		t.Fatalf("resolveEnvFilePath() = %q, want %q", got, envPath)
	}
}

func TestPersistEncryptionKeyWritesResolvedEnvFile(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "talos.env")
	if err := os.WriteFile(envPath, []byte("TALOS_SESSION_SECRET=test\n"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("TALOS_ENV_FILE", envPath)

	gotPath, err := persistEncryptionKey("abc123")
	if err != nil {
		t.Fatalf("persistEncryptionKey() error = %v", err)
	}
	if gotPath != envPath {
		t.Fatalf("persistEncryptionKey() path = %q, want %q", gotPath, envPath)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "TALOS_SESSION_SECRET=test\nTALOS_ENCRYPTION_KEY=abc123" {
		t.Fatalf("persisted env = %q", string(data))
	}
}
