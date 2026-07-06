package models

import "time"

// AccessCart represents a single checkout transaction — a user's basket of role requests.
type AccessCart struct {
	ID            string     `json:"id"`
	RequesterID   string     `json:"requester_id"`
	Status        string     `json:"status"` // DRAFT | SUBMITTED | IN_PROGRESS | COMPLETED | CANCELLED
	Justification string     `json:"justification"`
	Items         []CartItem `json:"items,omitempty"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Version       int        `json:"version"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// CartItem represents a single role request within a cart.
type CartItem struct {
	ID                 string     `json:"id"`
	CartID             string     `json:"cart_id"`
	RoleID             string     `json:"role_id"`
	RoleName           string     `json:"role_name"`
	Status             string     `json:"status"` // PENDING | IN_PROGRESS | APPROVED | REJECTED | CANCELLED
	WorkflowInstanceID *string    `json:"workflow_instance_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
