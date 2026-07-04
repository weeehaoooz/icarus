package models

import "time"

type Module struct {
	ID              string    `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	BaseURL         string    `json:"base_url"`
	IsActive        bool      `json:"is_active"`
	AppCentricRoles []Role    `json:"app_centric_roles,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
