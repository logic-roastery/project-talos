package domain

import "time"

type AppStatus string

const (
	AppStatusActive   AppStatus = "active"
	AppStatusInactive AppStatus = "inactive"
	AppStatusError    AppStatus = "error"
)

type AccessMode string

const (
	AccessModeDomain AccessMode = "domain"
	AccessModePort   AccessMode = "port"
)

type AppType string

const (
	AppTypeManaged          AppType = "managed"
	AppTypeAdoptedContainer AppType = "adopted_container"
	AppTypeExternalService  AppType = "external_service"
)

type RuntimeOwner string

const (
	RuntimeOwnerTalos    RuntimeOwner = "talos"
	RuntimeOwnerExternal RuntimeOwner = "external"
)

type EdgeProvider string

const (
	EdgeProviderInternalTraefik EdgeProvider = "internal_traefik"
	EdgeProviderExternalTraefik EdgeProvider = "external_traefik"
)

type App struct {
	ID                   int64        `json:"id"`
	Name                 string       `json:"name"`
	AppType              AppType      `json:"app_type"`
	RuntimeOwner         RuntimeOwner `json:"runtime_owner"`
	EdgeProvider         EdgeProvider `json:"edge_provider"`
	Source               string       `json:"source"`
	RepoURL              string       `json:"repo_url"`
	Branch               string       `json:"branch"`
	InternalPort         int          `json:"internal_port"`
	ImageRef             string       `json:"image_ref"`
	Domain               string       `json:"domain,omitempty"`
	FallbackPort         int          `json:"fallback_port,omitempty"`
	AccessMode           AccessMode   `json:"access_mode"`
	AccessURL            string       `json:"access_url"`
	Status               AppStatus    `json:"status"`
	CurrentDeployID      *int64       `json:"current_deploy_id,omitempty"`
	LiveContainerName    string       `json:"live_container_name,omitempty"`
	ContainerName        string       `json:"container_name,omitempty"`
	ExternalTarget       string       `json:"external_target,omitempty"`
	DockerNetwork        string       `json:"docker_network,omitempty"`
	GitHubInstallationID *int64       `json:"github_installation_id,omitempty"`
	GitHubRepoID         *int64       `json:"github_repo_id,omitempty"`
	RegistryURL          string       `json:"registry_url,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

func (a *App) EffectiveContainerName() string {
	if a.ContainerName != "" {
		return a.ContainerName
	}
	if a.LiveContainerName != "" {
		return a.LiveContainerName
	}
	return ""
}
