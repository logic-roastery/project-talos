package domain

import "time"

// ServiceType represents the type of managed service.
type ServiceType string

const (
	ServicePostgres ServiceType = "postgres"
	ServiceMySQL    ServiceType = "mysql"
	ServiceRedis    ServiceType = "redis"
	ServiceGarage   ServiceType = "garage"
)

// ServiceStatus represents the lifecycle state of a service.
type ServiceStatus string

const (
	ServiceStatusPending      ServiceStatus = "pending"
	ServiceStatusProvisioning ServiceStatus = "provisioning"
	ServiceStatusActive       ServiceStatus = "active"
	ServiceStatusError        ServiceStatus = "error"
	ServiceStatusStopped      ServiceStatus = "stopped"
)

// Service represents a managed backing service (database, cache, storage).
type Service struct {
	ID           int64         `json:"id"`
	Name         string        `json:"name"`
	Type         ServiceType   `json:"type"`
	ImageRef     string        `json:"image_ref"`
	Status       ServiceStatus `json:"status"`
	ContainerID  string        `json:"container_id,omitempty"`
	AppID        *int64        `json:"app_id,omitempty"`
	VolumePath   string        `json:"volume_path"`
	Credentials  string        `json:"-"`
	Config       string        `json:"config,omitempty"`
	InternalPort int           `json:"internal_port"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// AppService links an app to a service with an alias for env var injection.
type AppService struct {
	AppID     int64  `json:"app_id"`
	ServiceID int64  `json:"service_id"`
	Alias     string `json:"alias"`
}

// AppEnvVar represents a per-app environment variable.
type AppEnvVar struct {
	ID       int64  `json:"id"`
	AppID    int64  `json:"app_id"`
	Key      string `json:"key"`
	Value    string `json:"-"`
	IsSecret bool   `json:"is_secret"`
	Required bool   `json:"required"`
}

// AppEnvVarHistory records a previous value of an environment variable.
type AppEnvVarHistory struct {
	ID        int64     `json:"id"`
	AppID     int64     `json:"app_id"`
	Key       string    `json:"key"`
	Value     string    `json:"-"`
	IsSecret  bool      `json:"is_secret"`
	ChangedAt time.Time `json:"changed_at"`
	ChangedBy string    `json:"changed_by"`
}

// Credential structs — stored encrypted as JSON in Service.Credentials.

type PostgresCredentials struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type MySQLCredentials struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type RedisCredentials struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
}

type GarageCredentials struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Bucket    string `json:"bucket"`
}

// ServiceDefinition contains the built-in configuration for each service type.
type ServiceDefinition struct {
	Type         ServiceType
	DefaultImage string
	Port         int
	VolumePath   string
	HealthCmd    []string
}

// ServiceDefinitions maps service types to their built-in definitions.
var ServiceDefinitions = map[ServiceType]ServiceDefinition{
	ServicePostgres: {
		Type:         ServicePostgres,
		DefaultImage: "postgres:16",
		Port:         5432,
		VolumePath:   "/var/lib/postgresql/data",
		HealthCmd:    []string{"CMD-SHELL", "pg_isready -U $POSTGRES_USER"},
	},
	ServiceMySQL: {
		Type:         ServiceMySQL,
		DefaultImage: "mysql:8",
		Port:         3306,
		VolumePath:   "/var/lib/mysql",
		HealthCmd:    []string{"CMD-SHELL", "mysqladmin ping -h localhost -u root -p$MYSQL_ROOT_PASSWORD"},
	},
	ServiceRedis: {
		Type:         ServiceRedis,
		DefaultImage: "redis:7-alpine",
		Port:         6379,
		VolumePath:   "/data",
		HealthCmd:    []string{"CMD", "redis-cli", "ping"},
	},
	ServiceGarage: {
		Type:         ServiceGarage,
		DefaultImage: "dxflrs/garage:v1.0",
		Port:         3900,
		VolumePath:   "/data",
		HealthCmd:    []string{"CMD-SHELL", "wget -qO- http://localhost:3900/health || exit 1"},
	},
}
