package domain

import "time"

type Backup struct {
	ID        int64     `json:"id"`
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}
