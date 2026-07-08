package models

import "time"

// ApprovalPolicy defines how an approval node evaluates its steps.
type ApprovalPolicy string

const (
	PolicyAny    ApprovalPolicy = "ANY"
	PolicyAll    ApprovalPolicy = "ALL"
	PolicyQuorum ApprovalPolicy = "QUORUM"
)

// Workflow is an immutable, versioned blueprint for a workflow DAG.
// Updates always produce a new version record — existing runs (executions) are unaffected.
type Workflow struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	DefinitionKey string         `json:"definition_key"`
	Version       int            `json:"version"`
	IsCurrent     bool           `json:"is_current"`
	Status        string         `json:"status"` // DRAFT | ACTIVE | SUPERSEDED | ARCHIVED
	SupersedesID  *string        `json:"supersedes_id,omitempty"`
	Nodes         []WorkflowNode `json:"nodes,omitempty"`
	CreatedBy     string         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
}

// WorkflowNode is a generic, single-purpose node within a workflow definition.
type WorkflowNode struct {
	ID            string                 `json:"id"`
	WorkflowID    string                 `json:"workflow_id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"` // e.g., "APPROVAL" | "WEBHOOK" | "OPERATION"
	DependsOn     []string               `json:"depends_on"`
	MaxRetries    int                    `json:"max_retries,omitempty"`
	RetryInterval int                    `json:"retry_interval_seconds,omitempty"`
	Config        map[string]interface{} `json:"config,omitempty"`
}

// WorkflowCandidate defines an approver candidate.
type WorkflowCandidate struct {
	Type  string `json:"type"`  // USER | ROLE
	Value string `json:"value"` // user_id or role_name
}

// RoleWorkflowMapping links a role to its active workflow definition.
type RoleWorkflowMapping struct {
	ID            string     `json:"id"`
	RoleID        string     `json:"role_id"`
	WorkflowID    string     `json:"workflow_id"`
	IsActive      bool       `json:"is_active"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
}

// UpsertWorkflowRequest is the API payload for creating/updating a workflow definition for a role.
type UpsertWorkflowRequest struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Nodes       []UpsertNodeRequest `json:"nodes"`
}

// MapWorkflowToRoleRequest binds an existing workflow template to a role.
type MapWorkflowToRoleRequest struct {
	WorkflowID string `json:"workflow_id"`
}

type UpsertNodeRequest struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"` // e.g. "APPROVAL" | "OPERATION"
	DependsOn     []string               `json:"depends_on"`
	MaxRetries    int                    `json:"max_retries,omitempty"`
	RetryInterval int                    `json:"retry_interval_seconds,omitempty"`
	Config        map[string]interface{} `json:"config,omitempty"`
}

// WorkflowWithRoleMapping is a Workflow enriched with which role (if any) maps to it.
type WorkflowWithRoleMapping struct {
	Workflow
	MappedRoleID string `json:"mapped_role_id,omitempty"`
}

