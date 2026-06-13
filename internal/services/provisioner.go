package services

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/logic-roastery/project-talos/internal/crypto"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/runtime/docker"
	"github.com/logic-roastery/project-talos/internal/store"
	"github.com/moby/moby/api/types/container"
)

type Provisioner struct {
	services store.ServiceStore
	docker   *docker.Client
	dataDir  string
	encKey   []byte
	logger   *slog.Logger
}

func NewProvisioner(services store.ServiceStore, docker *docker.Client, dataDir string, encKey []byte, logger *slog.Logger) *Provisioner {
	return &Provisioner{
		services: services,
		docker:   docker,
		dataDir:  dataDir,
		encKey:   encKey,
		logger:   logger,
	}
}

// ProvisionService creates and starts a managed service container.
func (p *Provisioner) ProvisionService(ctx context.Context, svc *domain.Service, creds interface{}) error {
	svc.Status = domain.ServiceStatusProvisioning
	if err := p.services.UpdateService(ctx, svc); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	// Encrypt and store credentials
	credJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	encrypted, err := crypto.Encrypt(string(credJSON), p.encKey)
	if err != nil {
		return fmt.Errorf("encrypt credentials: %w", err)
	}
	svc.Credentials = encrypted

	// Create volume directory
	volHost := filepath.Join(p.dataDir, "services", svc.Name)
	if err := os.MkdirAll(volHost, 0755); err != nil {
		return fmt.Errorf("create volume dir: %w", err)
	}

	// Generate garage.toml and create meta/data subdirs for Garage
	if svc.Type == domain.ServiceGarage {
		os.MkdirAll(filepath.Join(volHost, "meta"), 0755)
		os.MkdirAll(filepath.Join(volHost, "data"), 0755)
		gc := creds.(*domain.GarageCredentials)
		configContent := generateGarageConfig(svc.Name, gc)
		if err := os.WriteFile(filepath.Join(volHost, "garage.toml"), []byte(configContent), 0644); err != nil {
			return fmt.Errorf("write garage config: %w", err)
		}
	}

	// Build container config based on service type
	containerCfg, err := p.buildContainerConfig(svc, creds, volHost)
	if err != nil {
		return fmt.Errorf("build config: %w", err)
	}

	// Pull image
	if err := p.docker.PullImage(ctx, svc.ImageRef); err != nil {
		svc.Status = domain.ServiceStatusError
		p.services.UpdateService(ctx, svc)
		return fmt.Errorf("pull image: %w", err)
	}

	// Stop existing container if any
	containerName := fmt.Sprintf("talos-svc-%s", svc.Name)
	p.docker.StopAndRemove(ctx, containerName)

	// Start container
	containerID, err := p.docker.StartContainerWithConfig(ctx, containerCfg)
	if err != nil {
		svc.Status = domain.ServiceStatusError
		p.services.UpdateService(ctx, svc)
		return fmt.Errorf("start container: %w", err)
	}

	svc.ContainerID = containerID
	svc.Status = domain.ServiceStatusActive
	if err := p.services.UpdateService(ctx, svc); err != nil {
		return fmt.Errorf("update service: %w", err)
	}

	// Auto-create a default bucket for Garage services
	if svc.Type == domain.ServiceGarage {
		gc, ok := creds.(*domain.GarageCredentials)
		if ok && gc.Bucket == "" {
			garageClient := NewGarageClient(
				fmt.Sprintf("http://%s:3903", containerName),
				gc.AdminToken,
			)
			if bucket, err := garageClient.CreateBucket(ctx, svc.Name); err == nil && len(bucket.GlobalAliases) > 0 {
				gc.Bucket = bucket.GlobalAliases[0]
				if encErr := p.EncryptCredentials(svc, gc); encErr == nil {
					p.services.UpdateService(ctx, svc)
				}
				p.logger.Info("auto-created default bucket", "service", svc.Name, "bucket", gc.Bucket)
			} else if err != nil {
				p.logger.Warn("auto-create bucket failed (non-fatal)", "service", svc.Name, "error", err)
			}
		}
	}

	p.logger.Info("service provisioned", "name", svc.Name, "type", svc.Type, "id", svc.ID)
	return nil
}

// StopService stops a running service container.
func (p *Provisioner) StopService(ctx context.Context, svc *domain.Service) error {
	if svc.ContainerID == "" {
		return nil
	}
	containerName := fmt.Sprintf("talos-svc-%s", svc.Name)
	if err := p.docker.StopAndRemove(ctx, containerName); err != nil {
		p.logger.Warn("stop service container", "error", err)
	}
	svc.ContainerID = ""
	svc.Status = domain.ServiceStatusStopped
	return p.services.UpdateService(ctx, svc)
}

// DeleteService stops and removes a service.
func (p *Provisioner) DeleteService(ctx context.Context, id int64) error {
	svc, err := p.services.GetService(ctx, id)
	if err != nil {
		return fmt.Errorf("get service: %w", err)
	}

	containerName := fmt.Sprintf("talos-svc-%s", svc.Name)
	p.docker.StopAndRemove(ctx, containerName)

	return p.services.DeleteService(ctx, id)
}

// StartService starts a stopped service.
func (p *Provisioner) StartService(ctx context.Context, svc *domain.Service) error {
	if svc.Status == domain.ServiceStatusActive {
		return nil
	}

	credJSON, err := crypto.Decrypt(svc.Credentials, p.encKey)
	if err != nil {
		return fmt.Errorf("decrypt credentials: %w", err)
	}

	volHost := filepath.Join(p.dataDir, "services", svc.Name)

	// Regenerate garage.toml if needed
	if svc.Type == domain.ServiceGarage {
		var gc domain.GarageCredentials
		json.Unmarshal([]byte(credJSON), &gc)
		configContent := generateGarageConfig(svc.Name, &gc)
		os.WriteFile(filepath.Join(volHost, "garage.toml"), []byte(configContent), 0644)
	}

	containerCfg, err := p.buildContainerConfigFromJSON(svc, credJSON, volHost)
	if err != nil {
		return fmt.Errorf("build config: %w", err)
	}

	containerName := fmt.Sprintf("talos-svc-%s", svc.Name)
	p.docker.StopAndRemove(ctx, containerName)

	containerID, err := p.docker.StartContainerWithConfig(ctx, containerCfg)
	if err != nil {
		svc.Status = domain.ServiceStatusError
		p.services.UpdateService(ctx, svc)
		return fmt.Errorf("start container: %w", err)
	}

	svc.ContainerID = containerID
	svc.Status = domain.ServiceStatusActive
	return p.services.UpdateService(ctx, svc)
}

// DecryptCredentials decrypts a service's credentials and unmarshals into the target.
func (p *Provisioner) DecryptCredentials(svc *domain.Service, target interface{}) error {
	credJSON, err := crypto.Decrypt(svc.Credentials, p.encKey)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}
	return json.Unmarshal([]byte(credJSON), target)
}

// EncryptCredentials marshals and encrypts credentials, storing them on the service.
func (p *Provisioner) EncryptCredentials(svc *domain.Service, creds interface{}) error {
	credJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("marshal creds: %w", err)
	}
	encrypted, err := crypto.Encrypt(string(credJSON), p.encKey)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}
	svc.Credentials = encrypted
	return nil
}

func (p *Provisioner) buildContainerConfig(svc *domain.Service, creds interface{}, volHost string) (docker.ContainerConfig, error) {
	credJSON, _ := json.Marshal(creds)
	return p.buildContainerConfigFromJSON(svc, string(credJSON), volHost)
}

func (p *Provisioner) buildContainerConfigFromJSON(svc *domain.Service, credJSON string, volHost string) (docker.ContainerConfig, error) {
	def := domain.ServiceDefinitions[svc.Type]
	containerName := fmt.Sprintf("talos-svc-%s", svc.Name)

	cfg := docker.ContainerConfig{
		Name:         containerName,
		ImageRef:     svc.ImageRef,
		InternalPort: svc.InternalPort,
		Labels: map[string]string{
			"managed-by": "talos",
			"talos-svc":  svc.Name,
			"talos-type": string(svc.Type),
		},
	}

	switch svc.Type {
	case domain.ServicePostgres:
		var pc domain.PostgresCredentials
		json.Unmarshal([]byte(credJSON), &pc)
		cfg.Env = []string{
			"POSTGRES_DB=" + pc.Database,
			"POSTGRES_USER=" + pc.User,
			"POSTGRES_PASSWORD=" + pc.Password,
		}
		cfg.Volumes = []string{volHost + ":" + def.VolumePath}
		cfg.HealthCheck = &container.HealthConfig{
			Test:     def.HealthCmd,
			Interval: 10 * time.Second,
			Timeout:  5 * time.Second,
			Retries:  5,
		}

	case domain.ServiceMySQL:
		var mc domain.MySQLCredentials
		json.Unmarshal([]byte(credJSON), &mc)
		cfg.Env = []string{
			"MYSQL_DATABASE=" + mc.Database,
			"MYSQL_USER=" + mc.User,
			"MYSQL_PASSWORD=" + mc.Password,
			"MYSQL_ROOT_PASSWORD=" + mc.Password,
		}
		cfg.Volumes = []string{volHost + ":" + def.VolumePath}
		cfg.HealthCheck = &container.HealthConfig{
			Test:     def.HealthCmd,
			Interval: 10 * time.Second,
			Timeout:  5 * time.Second,
			Retries:  5,
		}

	case domain.ServiceRedis:
		var rc domain.RedisCredentials
		json.Unmarshal([]byte(credJSON), &rc)
		cfg.Volumes = []string{volHost + ":" + def.VolumePath}
		cfg.HealthCheck = &container.HealthConfig{
			Test:     []string{"CMD", "redis-cli", "-a", rc.Password, "ping"},
			Interval: 10 * time.Second,
			Timeout:  5 * time.Second,
			Retries:  5,
		}

	case domain.ServiceGarage:
		var gc domain.GarageCredentials
		json.Unmarshal([]byte(credJSON), &gc)
		cfg.Env = []string{
			"GARAGE_CONFIG_FILE=/etc/garage.toml",
		}
		cfg.Volumes = []string{
			volHost + "/garage.toml:/etc/garage.toml:ro",
			volHost + "/meta:/var/lib/garage/meta",
			volHost + "/data:/var/lib/garage/data",
		}
		cfg.InternalPort = 3900 // S3 API
		cfg.HealthCheck = &container.HealthConfig{
			Test:     []string{"CMD-SHELL", "wget -qO- http://localhost:3900/health || exit 1"},
			Interval: 10 * time.Second,
			Timeout:  5 * time.Second,
			Retries:  5,
		}

	case domain.ServiceGarageWebUI:
		var wc domain.GarageWebUICredentials
		json.Unmarshal([]byte(credJSON), &wc)
		cfg.Env = []string{
			"API_BASE_URL=" + wc.AdminAPIURL,
			"API_ADMIN_KEY=" + wc.AdminKey,
			"S3_ENDPOINT_URL=" + wc.S3Endpoint,
		}
		if wc.Username != "" {
			cfg.Env = append(cfg.Env, "AUTH_USER_PASS="+wc.Username+":"+wc.Password)
		}
		cfg.HealthCheck = &container.HealthConfig{
			Test:     []string{"CMD-SHELL", "wget -qO- http://localhost:3909/ || exit 1"},
			Interval: 10 * time.Second,
			Timeout:  5 * time.Second,
			Retries:  5,
		}

	default:
		return cfg, fmt.Errorf("unsupported service type: %s", svc.Type)
	}

	return cfg, nil
}

// Credential generators

func GeneratePassword(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		result[i] = chars[n.Int64()]
	}
	return string(result)
}

func GenerateAccessKey(length int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		result[i] = chars[n.Int64()]
	}
	return string(result)
}

func generateGarageConfig(svcName string, creds *domain.GarageCredentials) string {
	return fmt.Sprintf(`metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"
replication_factor = 1

rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "%s"

[s3_api]
s3_region = "%s"
api_bind_addr = "[::]:3900"

[admin]
api_bind_addr = "[::]:3903"
admin_token = "%s"
`, creds.RPCSecret, creds.Region, creds.AdminToken)
}

func GenerateServiceName(svcType domain.ServiceType) string {
	prefix := string(svcType)
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	suffix := make([]byte, 4)
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	for i := range suffix {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		suffix[i] = chars[n.Int64()]
	}
	return prefix + "-" + string(suffix)
}

// DefaultCredentials returns default credentials for a service type with generated passwords.
func DefaultCredentials(svcType domain.ServiceType, containerName string) interface{} {
	switch svcType {
	case domain.ServicePostgres:
		return domain.PostgresCredentials{
			Host:     containerName,
			Port:     5432,
			Database: "app",
			User:     "postgres",
			Password: GeneratePassword(24),
		}
	case domain.ServiceMySQL:
		return domain.MySQLCredentials{
			Host:     containerName,
			Port:     3306,
			Database: "app",
			User:     "mysql",
			Password: GeneratePassword(24),
		}
	case domain.ServiceRedis:
		return domain.RedisCredentials{
			Host:     containerName,
			Port:     6379,
			Password: GeneratePassword(24),
		}
	case domain.ServiceGarage:
		return domain.GarageCredentials{
			Endpoint:   fmt.Sprintf("http://%s:3900", containerName),
			Region:     "garage",
			AccessKey:  GenerateAccessKey(20),
			SecretKey:  GeneratePassword(40),
			Bucket:     "",
			AdminToken: GeneratePassword(32),
			RPCSecret:  GeneratePassword(32),
		}
	case domain.ServiceGarageWebUI:
		return domain.GarageWebUICredentials{
			AdminAPIURL: fmt.Sprintf("http://talos-svc-%s:3903", containerName),
			S3Endpoint:  fmt.Sprintf("http://talos-svc-%s:3900", containerName),
			AdminKey:    "",
			Username:    "admin",
			Password:    GeneratePassword(16),
		}
	default:
		return nil
	}
}

// FormatEnvVars formats service credentials as environment variable strings
// using the alias as prefix (e.g., DATABASE_URL, REDIS_HOST).
func FormatEnvVars(svc *domain.Service, creds interface{}, alias string) []string {
	prefix := strings.ToUpper(alias)
	var vars []string

	switch svc.Type {
	case domain.ServicePostgres:
		c := creds.(domain.PostgresCredentials)
		vars = []string{
			fmt.Sprintf("%s_URL=postgres://%s:%s@%s:%d/%s", prefix, c.User, c.Password, c.Host, c.Port, c.Database),
			fmt.Sprintf("%s_HOST=%s", prefix, c.Host),
			fmt.Sprintf("%s_PORT=%d", prefix, c.Port),
			fmt.Sprintf("%s_USER=%s", prefix, c.User),
			fmt.Sprintf("%s_PASSWORD=%s", prefix, c.Password),
			fmt.Sprintf("%s_NAME=%s", prefix, c.Database),
		}

	case domain.ServiceMySQL:
		c := creds.(domain.MySQLCredentials)
		vars = []string{
			fmt.Sprintf("%s_URL=mysql://%s:%s@%s:%d/%s", prefix, c.User, c.Password, c.Host, c.Port, c.Database),
			fmt.Sprintf("%s_HOST=%s", prefix, c.Host),
			fmt.Sprintf("%s_PORT=%d", prefix, c.Port),
			fmt.Sprintf("%s_USER=%s", prefix, c.User),
			fmt.Sprintf("%s_PASSWORD=%s", prefix, c.Password),
			fmt.Sprintf("%s_NAME=%s", prefix, c.Database),
		}

	case domain.ServiceRedis:
		c := creds.(domain.RedisCredentials)
		vars = []string{
			fmt.Sprintf("%s_URL=redis://:%s@%s:%d", prefix, c.Password, c.Host, c.Port),
			fmt.Sprintf("%s_HOST=%s", prefix, c.Host),
			fmt.Sprintf("%s_PORT=%d", prefix, c.Port),
			fmt.Sprintf("%s_PASSWORD=%s", prefix, c.Password),
		}

	case domain.ServiceGarage:
		c := creds.(domain.GarageCredentials)
		vars = []string{
			fmt.Sprintf("%s_ENDPOINT=%s", prefix, c.Endpoint),
			fmt.Sprintf("%s_REGION=%s", prefix, c.Region),
			fmt.Sprintf("%s_ACCESS_KEY=%s", prefix, c.AccessKey),
			fmt.Sprintf("%s_SECRET_KEY=%s", prefix, c.SecretKey),
			fmt.Sprintf("%s_BUCKET=%s", prefix, c.Bucket),
			fmt.Sprintf("%s_ADMIN_TOKEN=%s", prefix, c.AdminToken),
		}
	case domain.ServiceGarageWebUI:
		c := creds.(domain.GarageWebUICredentials)
		vars = []string{
			fmt.Sprintf("%s_ADMIN_API_URL=%s", prefix, c.AdminAPIURL),
			fmt.Sprintf("%s_S3_ENDPOINT=%s", prefix, c.S3Endpoint),
			fmt.Sprintf("%s_USERNAME=%s", prefix, c.Username),
			fmt.Sprintf("%s_PASSWORD=%s", prefix, c.Password),
		}
	}

	return vars
}
