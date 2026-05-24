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

type App struct {
	ID                   int64      `json:"id"`
	Name                 string     `json:"name"`
	Source               string     `json:"source"`
	RepoURL              string     `json:"repo_url"`
	Branch               string     `json:"branch"`
	InternalPort         int        `json:"internal_port"`
	ImageRef             string     `json:"image_ref"`
	Domain               string     `json:"domain,omitempty"`
	FallbackPort         int        `json:"fallback_port,omitempty"`
	AccessMode           AccessMode `json:"access_mode"`
	AccessURL            string     `json:"access_url"`
	Status               AppStatus  `json:"status"`
	CurrentDeployID      *int64     `json:"current_deploy_id,omitempty"`
	GitHubInstallationID *int64     `json:"github_installation_id,omitempty"`
	GitHubRepoID         *int64     `json:"github_repo_id,omitempty"`
	RegistryURL          string     `json:"registry_url,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}
