package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestInitializeEncryptionKeyGeneratesForFirstInstall(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "talos.env")
	dbPath := filepath.Join(tmp, "talos.db")

	if err := os.WriteFile(envPath, []byte("TALOS_SESSION_SECRET=test\n"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("TALOS_ENV_FILE", envPath)

	key, gotPath, err := initializeEncryptionKey("", dbPath)
	if err != nil {
		t.Fatalf("initializeEncryptionKey() error = %v", err)
	}
	if key == "" {
		t.Fatalf("initializeEncryptionKey() returned empty key")
	}
	if gotPath != envPath {
		t.Fatalf("initializeEncryptionKey() path = %q, want %q", gotPath, envPath)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) == 0 || !containsLine(string(data), "TALOS_ENCRYPTION_KEY=") {
		t.Fatalf("persisted env missing encryption key: %q", string(data))
	}
}

func TestInitializeEncryptionKeyFailsForExistingInstallWithoutKey(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "talos.db")
	if err := os.WriteFile(dbPath, []byte("db"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, _, err := initializeEncryptionKey("", dbPath)
	if !errors.Is(err, errMissingEncryptionKey) {
		t.Fatalf("initializeEncryptionKey() error = %v, want %v", err, errMissingEncryptionKey)
	}
}

func TestInitializeEncryptionKeyKeepsConfiguredKey(t *testing.T) {
	key, gotPath, err := initializeEncryptionKey("configured-key", filepath.Join(t.TempDir(), "talos.db"))
	if err != nil {
		t.Fatalf("initializeEncryptionKey() error = %v", err)
	}
	if key != "configured-key" {
		t.Fatalf("initializeEncryptionKey() key = %q", key)
	}
	if gotPath != "" {
		t.Fatalf("initializeEncryptionKey() path = %q, want empty", gotPath)
	}
}

func containsLine(content, prefix string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
