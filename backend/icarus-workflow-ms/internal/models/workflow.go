package models

import "time"

// WorkflowDefinition is an immutable, versioned blueprint for an approval flow.
// Updates always produce a new version record — existing instances are unaffected.
type WorkflowDefinition struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	DefinitionKey string                  `json:"definition_key"`
	Version       int                     `json:"version"`
	IsCurrent     bool                    `json:"is_current"`
	Status        string                  `json:"status"` // DRAFT | ACTIVE | SUPERSEDED | ARCHIVED
	SupersedesID  *string                 `json:"supersedes_id,omitempty"`
	Stages        []WorkflowStageDefinition `json:"stages,omitempty"`
	CreatedBy     string                  `json:"created_by"`
	CreatedAt     time.Time               `json:"created_at"`
}

// WorkflowStageDefinition is one stage within a workflow definition.
type WorkflowStageDefinition struct {
	ID                   string               `json:"id"`
	WorkflowDefinitionID string               `json:"workflow_definition_id"`
	SequenceOrder        int                  `json:"sequence_order"`
	ExecutionMode        string               `json:"execution_mode"` // SEQUENTIAL | PARALLEL
	Name                 string               `json:"name"`
	ApprovalQuorum       int                  `json:"approval_quorum"`
	Approvers            []ApproverDefinition `json:"approvers,omitempty"`
}

// ApproverDefinition specifies how to resolve the approver(s) for a stage.
type ApproverDefinition struct {
	ID               string `json:"id"`
	StageDefinitionID string `json:"stage_definition_id"`
	ResolverType     string `json:"resolver_type"`  // USER | ROLE_QUEUE | EXPRESSION
	ResolverValue    string `json:"resolver_value"` // user_id | role_name | expression
}

// RoleWorkflowMapping links a role to its active workflow definition.
type RoleWorkflowMapping struct {
	ID                   string     `json:"id"`
	RoleID               string     `json:"role_id"`
	WorkflowDefinitionID string     `json:"workflow_definition_id"`
	IsActive             bool       `json:"is_active"`
	EffectiveFrom        time.Time  `json:"effective_from"`
	EffectiveTo          *time.Time `json:"effective_to,omitempty"`
	CreatedBy            string     `json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
}

// UpsertWorkflowRequest is the API payload for creating/updating a workflow definition for a role.
type UpsertWorkflowRequest struct {
	Name   string                   `json:"name"`
	Stages []UpsertStageRequest     `json:"stages"`
}

type UpsertStageRequest struct {
	SequenceOrder  int                      `json:"sequence_order"`
	ExecutionMode  string                   `json:"execution_mode"`
	Name           string                   `json:"name"`
	ApprovalQuorum int                      `json:"approval_quorum"`
	Approvers      []UpsertApproverRequest  `json:"approvers"`
}

type UpsertApproverRequest struct {
	ResolverType  string `json:"resolver_type"`
	ResolverValue string `json:"resolver_value"`
}
