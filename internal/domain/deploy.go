package domain

import "time"

type DeployStatus string

const (
	DeployStatusPending      DeployStatus = "pending"
	DeployStatusRunning      DeployStatus = "running"
	DeployStatusSuccess      DeployStatus = "success"
	DeployStatusFailed       DeployStatus = "failed"
	DeployStatusRollback     DeployStatus = "rollback"
	DeployStatusAutoRollback DeployStatus = "auto_rollback"
)

type Deploy struct {
	ID           int64        `json:"id"`
	AppID        int64        `json:"app_id"`
	ImageRef     string       `json:"image_ref"`
	CommitSHA    string       `json:"commit_sha,omitempty"`
	Branch       string       `json:"branch"`
	Status       DeployStatus `json:"status"`
	ContainerID  string       `json:"container_id,omitempty"`
	HealthStatus string       `json:"health_status,omitempty"`
	Logs         string       `json:"logs,omitempty"`
	EnvSnapshot  string       `json:"env_snapshot,omitempty"`
	StartedAt    *time.Time   `json:"started_at,omitempty"`
	CompletedAt  *time.Time   `json:"completed_at,omitempty"`
	TriggeredBy  string       `json:"triggered_by"`
	RollbackOfID *int64       `json:"rollback_of_id,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

func (s DeployStatus) IsTerminal() bool {
	switch s {
	case DeployStatusSuccess, DeployStatusFailed, DeployStatusAutoRollback, DeployStatusRollback:
		return true
	default:
		return false
	}
}
