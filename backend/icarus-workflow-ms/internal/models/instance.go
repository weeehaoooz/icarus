package models

import "time"

// Execution is the runtime record for a single cart item's approval journey.
// It is pinned to the Workflow version active at submission time.
type Execution struct {
	ID           string          `json:"id"`
	WorkflowID   string          `json:"workflow_id"`
	CartItemID   string          `json:"cart_item_id"`
	Status       string          `json:"status"` // IN_PROGRESS | COMPLETED | REJECTED | CANCELLED | ERROR
	ErrorMessage *string         `json:"error_message,omitempty"`
	Nodes        []ExecutionNode `json:"nodes,omitempty"` // Replaces Steps
	StartedAt    time.Time       `json:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	Version      int             `json:"version"`
}

// ExecutionNode is one individual approval/operation assignment/action within an Execution.
type ExecutionNode struct {
	ID                 string     `json:"id"`
	ExecutionID        string     `json:"execution_id"`
	NodeID             string     `json:"node_id"`
	NodeName           string     `json:"node_name,omitempty"`
	AssignedToUserID   *string    `json:"assigned_to_user_id,omitempty"`
	AssignedToRole     *string    `json:"assigned_to_role,omitempty"`
	Status             string     `json:"status"` // PENDING | APPROVED | REJECTED | DELEGATED | CANCELLED
	DecisionComment    string     `json:"decision_comment,omitempty"`
	ActedByUserID      *string    `json:"acted_by_user_id,omitempty"`
	AssignedAt         time.Time  `json:"assigned_at"`
	ActedAt            *time.Time `json:"acted_at,omitempty"`
	RetryAttempt       int        `json:"retry_attempt"`
	Version            int        `json:"version"`
}

// AuditLog is an immutable record of a state transition.
type AuditLog struct {
	ID            string    `json:"id"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	ActorUserID   string    `json:"actor_user_id,omitempty"`
	Action        string    `json:"action"`
	BeforeState   string    `json:"before_state,omitempty"`
	AfterState    string    `json:"after_state,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// SSENotification is a persisted in-app notification for SSE delivery.
type SSENotification struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	EventType string    `json:"event_type"` // inbox.new | cart.updated | step.actioned
	Payload   string    `json:"payload"`    // JSON string
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// InboxItem is the approver-facing view of a pending step.
type InboxItem struct {
	StepID         string    `json:"step_id"`
	CartItemID     string    `json:"cart_item_id"`
	CartID         string    `json:"cart_id"`
	CorrelationID  string    `json:"correlation_id"`
	RequesterID    string    `json:"requester_id"`
	RoleRequested  string    `json:"role_requested"`
	Justification  string    `json:"justification"`
	StageName      string    `json:"stage_name"` // Replaces sequential stage name, populated with node name
	AssignmentType string    `json:"assignment_type"` // USER | ROLE_QUEUE
	AssignedAt     time.Time `json:"assigned_at"`
}

// StepActionRequest is the payload for approve / reject / delegate actions.
type StepActionRequest struct {
	Comment        string  `json:"comment"`
	DelegateToUser *string `json:"delegate_to_user_id,omitempty"` // only for delegate
}
