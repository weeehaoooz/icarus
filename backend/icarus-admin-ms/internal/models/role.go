package models

import "time"

type Role struct {
	ID           string    `json:"id"`
	ModuleID     string    `json:"module_id"`
	TenantID     *string   `json:"tenant_id,omitempty"`
	AppCode      string    `json:"app_code,omitempty"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	IsSystemRole bool      `json:"is_system_role"`
	Permissions  []string  `json:"permissions,omitempty"`
	NestedRoles  []string  `json:"nested_roles,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
