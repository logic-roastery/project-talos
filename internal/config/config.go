package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	Auth          AuthConfig
	Docker        DockerConfig
	Traefik       TraefikConfig
	GitHub        GitHubConfig
	Backup        BackupConfig
	EncryptionKey string // base64-encoded 32-byte key, auto-generated if empty
}

type ProxyMode string

const (
	ProxyModeInternal ProxyMode = "internal"
	ProxyModeExternal ProxyMode = "external"
)

type EdgeProvider string

const (
	EdgeProviderTraefik EdgeProvider = "traefik"
)

type ServerConfig struct {
	Host             string
	Port             int
	Domain           string    // TALOS_DOMAIN, empty = IP mode
	ACMEEmail        string    // TALOS_ACME_EMAIL, for Let's Encrypt
	ProxyMode        ProxyMode // TALOS_PROXY_MODE
	EdgeProvider     EdgeProvider
	EdgeNetwork      string // TALOS_EDGE_NETWORK, shared external proxy network
	EdgeCertResolver string // TALOS_EDGE_CERT_RESOLVER, e.g. letsencrypt
	EdgeEntrypoint   string // TALOS_EDGE_ENTRYPOINT, e.g. websecure
}

type DatabaseConfig struct {
	Path string
}

type AuthConfig struct {
	SessionSecret string
	SessionMaxAge int // seconds
}

type DockerConfig struct {
	Host    string
	Network string
}

type TraefikConfig struct {
	Image       string
	DashboardOn bool
	ConfigDir   string
	DataDir     string
}

type GitHubConfig struct {
	WebhookSecret string
	AppID         int64
	AppSlug       string
	PrivateKey    string // PEM string or file path
	ClientID      string
	ClientSecret  string
}

type BackupConfig struct {
	Dir             string // TALOS_BACKUP_DIR, default "data/backups"
	IntervalMinutes int
	RetainCount     int
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host:             hostDefault("0.0.0.0"),
			Port:             portDefault(3000),
			Domain:           envOrDefault("TALOS_DOMAIN", ""),
			ACMEEmail:        envOrDefault("TALOS_ACME_EMAIL", ""),
			ProxyMode:        proxyModeDefault("TALOS_PROXY_MODE", ProxyModeInternal),
			EdgeProvider:     edgeProviderDefault("TALOS_EDGE_PROVIDER", EdgeProviderTraefik),
			EdgeNetwork:      envOrDefault("TALOS_EDGE_NETWORK", "traefik-public"),
			EdgeCertResolver: envOrDefault("TALOS_EDGE_CERT_RESOLVER", "letsencrypt"),
			EdgeEntrypoint:   envOrDefault("TALOS_EDGE_ENTRYPOINT", "websecure"),
		},
		Database: DatabaseConfig{
			Path: envOrDefault("TALOS_DB_PATH", "data/talos.db"),
		},
		Auth: AuthConfig{
			SessionSecret: mustEnv("TALOS_SESSION_SECRET"),
			SessionMaxAge: intDefault("TALOS_SESSION_MAX_AGE", 86400*7), // 7 days
		},
		Docker: DockerConfig{
			Host:    envOrDefault("TALOS_DOCKER_HOST", "unix:///var/run/docker.sock"),
			Network: envOrDefault("TALOS_DOCKER_NETWORK", "talos"),
		},
		Traefik: TraefikConfig{
			Image:       envOrDefault("TALOS_TRAEFIK_IMAGE", "traefik:v3.0"),
			DashboardOn: boolDefault("TALOS_TRAEFIK_DASHBOARD", false),
			ConfigDir:   envOrDefault("TALOS_TRAEFIK_CONFIG_DIR", "data/traefik/config"),
			DataDir:     envOrDefault("TALOS_TRAEFIK_DATA_DIR", "data/traefik/data"),
		},
		GitHub: GitHubConfig{
			WebhookSecret: envOrDefault("TALOS_GITHUB_WEBHOOK_SECRET", ""),
			AppID:         int64Default("TALOS_GITHUB_APP_ID", 0),
			AppSlug:       envOrDefault("TALOS_GITHUB_APP_SLUG", ""),
			PrivateKey:    envOrDefault("TALOS_GITHUB_APP_PRIVATE_KEY", ""),
			ClientID:      envOrDefault("TALOS_GITHUB_APP_CLIENT_ID", ""),
			ClientSecret:  envOrDefault("TALOS_GITHUB_APP_CLIENT_SECRET", ""),
		},
		Backup: BackupConfig{
			Dir: envOrDefault("TALOS_BACKUP_DIR", "data/backups"),
		},
		EncryptionKey: envOrDefault("TALOS_ENCRYPTION_KEY", ""),
	}

	cfg.Backup.Dir = envOrDefault("TALOS_BACKUP_DIR", "data/backups")
	cfg.Backup.IntervalMinutes = intDefault("TALOS_BACKUP_INTERVAL_MINUTES", 0)
	cfg.Backup.RetainCount = intDefault("TALOS_BACKUP_RETAIN_COUNT", 10)

	return cfg, nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return v
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func hostDefault(fallback string) string {
	return envOrDefault("TALOS_HOST", fallback)
}

func portDefault(fallback int) int {
	return intDefault("TALOS_PORT", fallback)
}

func intDefault(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func boolDefault(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func int64Default(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func proxyModeDefault(key string, fallback ProxyMode) ProxyMode {
	v := envOrDefault(key, string(fallback))
	switch ProxyMode(v) {
	case ProxyModeInternal, ProxyModeExternal:
		return ProxyMode(v)
	default:
		return fallback
	}
}

func edgeProviderDefault(key string, fallback EdgeProvider) EdgeProvider {
	v := envOrDefault(key, string(fallback))
	switch EdgeProvider(v) {
	case EdgeProviderTraefik:
		return EdgeProvider(v)
	default:
		return fallback
	}
}
