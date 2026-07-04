package models

import "time"

type UserGroup struct {
	Name string `json:"name"`
	Type string `json:"type"` // "LDAP" or "Custom"
}

type User struct {
	ID           int64       `json:"id"`
	Username     string      `json:"username"`
	Email        string      `json:"email"`
	FirstName    string      `json:"first_name"`
	LastName     string      `json:"last_name"`
	PasswordHash string      `json:"-"`
	CreatedAt    time.Time   `json:"created_at"`
	Roles        []string    `json:"roles,omitempty"`
	Groups       []UserGroup `json:"groups,omitempty"`
}
