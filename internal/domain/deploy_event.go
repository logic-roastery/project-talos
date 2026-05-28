package domain

import "time"

type DeployEvent struct {
	ID        int64     `json:"id"`
	DeployID  int64     `json:"deploy_id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Step      string    `json:"step"`
	Message   string    `json:"message"`
}
