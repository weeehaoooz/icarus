package models

import "time"

type Permission struct {
	ID          string    `json:"id"`
	ModuleID    string    `json:"module_id"`
	Action      string    `json:"action"`
	PathPattern string    `json:"path_pattern,omitempty"`
	Method      string    `json:"method,omitempty"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
