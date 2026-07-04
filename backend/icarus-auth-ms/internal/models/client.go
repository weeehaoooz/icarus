package models

import "time"

type Client struct {
	ClientID  string    `json:"client_id"`
	PublicKey string    `json:"public_key"`
	CreatedAt time.Time `json:"created_at"`
	Roles     []string  `json:"roles,omitempty"`
}
