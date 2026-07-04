package models

import "time"

type Tenant struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Module struct {
	ID              string    `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	BaseURL         string    `json:"base_url"`
	IsActive        bool      `json:"is_active"`
	AppCentricRoles []Role    `json:"app_centric_roles,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type Permission struct {
	ID          string    `json:"id"`
	ModuleID    string    `json:"module_id"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Application struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type AppCentricRoleTemplate struct {
	ID          string   `json:"id"`
	ModuleID    string   `json:"module_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions,omitempty"`
}

type ModuleApplication struct {
	ModuleID  string    `json:"module_id"`
	AppCode   string    `json:"app_code"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Role struct {
	ID           string    `json:"id"`
	ModuleID     string    `json:"module_id"`
	TenantID     *string   `json:"tenant_id,omitempty"` // Nullable for system-wide roles
	AppCode      string    `json:"app_code,omitempty"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	IsSystemRole bool      `json:"is_system_role"`
	IsActive     bool      `json:"is_active"`
	Permissions  []string  `json:"permissions,omitempty"`
	NestedRoles  []string  `json:"nested_roles,omitempty"`
	Type         string    `json:"type,omitempty"` // Legacy support for tests (LDAP or Custom)
	CreatedAt    time.Time `json:"created_at"`
}

type UserTenantModuleRole struct {
	ID         string    `json:"id"`
	UserID     int64     `json:"user_id"`
	TenantID   string    `json:"tenant_id"`
	ModuleID   string    `json:"module_id"`
	RoleID     string    `json:"role_id"`
	AssignedAt time.Time `json:"assigned_at"`
}
