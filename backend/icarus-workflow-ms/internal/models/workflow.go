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

// StageType indicates the type of approval required at this stage.
type StageType string

const (
	StageTypeUser    StageType = "USER"
	StageTypeRole    StageType = "ROLE"
	StageTypeGrouped StageType = "GROUPED"
)

// GroupCondition defines how a Grouped approval node is evaluated.
type GroupCondition string

const (
	ConditionAnd GroupCondition = "AND"
	ConditionOr  GroupCondition = "OR"
)

// ApprovalNode defines a recursive tree structure for grouped/parallel approvals.
type ApprovalNode struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`                     // USER | ROLE | GROUP
	Value          string         `json:"value,omitempty"`          // user_id or role_name (for USER/ROLE)
	GroupCondition GroupCondition `json:"group_condition,omitempty"` // AND | OR (for GROUP)
	Children       []ApprovalNode `json:"children,omitempty"`       // Nested nodes (for GROUP)
}

// WorkflowStageDefinition is one stage within a workflow definition.
type WorkflowStageDefinition struct {
	ID                   string        `json:"id"`
	WorkflowDefinitionID string        `json:"workflow_definition_id"`
	SequenceOrder        int           `json:"sequence_order"`
	Name                 string        `json:"name"`
	Type                 StageType     `json:"type"`
	ApprovalTree         *ApprovalNode `json:"approval_tree,omitempty"`
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
	Name   string               `json:"name"`
	Stages []UpsertStageRequest `json:"stages"`
}

type UpsertStageRequest struct {
	SequenceOrder int                `json:"sequence_order"`
	Name          string             `json:"name"`
	Type          StageType          `json:"type"`
	ApprovalTree  *UpsertNodeRequest `json:"approval_tree,omitempty"`
}

type UpsertNodeRequest struct {
	Type           string              `json:"type"`
	Value          string              `json:"value,omitempty"`
	GroupCondition GroupCondition      `json:"group_condition,omitempty"`
	Children       []UpsertNodeRequest `json:"children,omitempty"`
}
