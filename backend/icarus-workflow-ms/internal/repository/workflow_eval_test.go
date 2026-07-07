package repository

import (
	"icarus-workflow-ms/internal/models"
	"testing"
	"time"
)

func TestEvaluateStageTree(t *testing.T) {
	// Test case: Standard AND group
	t.Run("AND Group - Approved when all approve", func(t *testing.T) {
		tree := &models.ApprovalNode{
			ID:   "root",
			Type: "GROUP",
			GroupCondition: models.ConditionAnd,
			Children: []models.ApprovalNode{
				{ID: "node1", Type: "USER", Value: "alice"},
				{ID: "node2", Type: "USER", Value: "bob"},
			},
		}

		stepsMap := map[string][]models.WorkflowStep{
			"node1": {{NodeID: "node1", Status: "APPROVED", AssignedAt: time.Now()}},
			"node2": {{NodeID: "node2", Status: "APPROVED", AssignedAt: time.Now()}},
		}

		status := evaluateStageTree(tree, stepsMap)
		if status != NodeStatusApproved {
			t.Errorf("expected APPROVED, got %v", status)
		}
	})

	t.Run("AND Group - Pending when one is pending", func(t *testing.T) {
		tree := &models.ApprovalNode{
			ID:   "root",
			Type: "GROUP",
			GroupCondition: models.ConditionAnd,
			Children: []models.ApprovalNode{
				{ID: "node1", Type: "USER", Value: "alice"},
				{ID: "node2", Type: "USER", Value: "bob"},
			},
		}

		stepsMap := map[string][]models.WorkflowStep{
			"node1": {{NodeID: "node1", Status: "APPROVED", AssignedAt: time.Now()}},
			"node2": {{NodeID: "node2", Status: "PENDING", AssignedAt: time.Now()}},
		}

		status := evaluateStageTree(tree, stepsMap)
		if status != NodeStatusPending {
			t.Errorf("expected PENDING, got %v", status)
		}
	})

	t.Run("AND Group - Rejected when one rejects", func(t *testing.T) {
		tree := &models.ApprovalNode{
			ID:   "root",
			Type: "GROUP",
			GroupCondition: models.ConditionAnd,
			Children: []models.ApprovalNode{
				{ID: "node1", Type: "USER", Value: "alice"},
				{ID: "node2", Type: "USER", Value: "bob"},
			},
		}

		stepsMap := map[string][]models.WorkflowStep{
			"node1": {{NodeID: "node1", Status: "APPROVED", AssignedAt: time.Now()}},
			"node2": {{NodeID: "node2", Status: "REJECTED", AssignedAt: time.Now()}},
		}

		status := evaluateStageTree(tree, stepsMap)
		if status != NodeStatusRejected {
			t.Errorf("expected REJECTED, got %v", status)
		}
	})

	// Test case: Standard OR group
	t.Run("OR Group - Approved when any one approves", func(t *testing.T) {
		tree := &models.ApprovalNode{
			ID:   "root",
			Type: "GROUP",
			GroupCondition: models.ConditionOr,
			Children: []models.ApprovalNode{
				{ID: "node1", Type: "USER", Value: "alice"},
				{ID: "node2", Type: "USER", Value: "bob"},
			},
		}

		stepsMap := map[string][]models.WorkflowStep{
			"node1": {{NodeID: "node1", Status: "PENDING", AssignedAt: time.Now()}},
			"node2": {{NodeID: "node2", Status: "APPROVED", AssignedAt: time.Now()}},
		}

		status := evaluateStageTree(tree, stepsMap)
		if status != NodeStatusApproved {
			t.Errorf("expected APPROVED, got %v", status)
		}
	})

	t.Run("OR Group - Pending when all pending or rejected, but not all rejected", func(t *testing.T) {
		tree := &models.ApprovalNode{
			ID:   "root",
			Type: "GROUP",
			GroupCondition: models.ConditionOr,
			Children: []models.ApprovalNode{
				{ID: "node1", Type: "USER", Value: "alice"},
				{ID: "node2", Type: "USER", Value: "bob"},
			},
		}

		stepsMap := map[string][]models.WorkflowStep{
			"node1": {{NodeID: "node1", Status: "PENDING", AssignedAt: time.Now()}},
			"node2": {{NodeID: "node2", Status: "REJECTED", AssignedAt: time.Now()}},
		}

		status := evaluateStageTree(tree, stepsMap)
		if status != NodeStatusPending {
			t.Errorf("expected PENDING, got %v", status)
		}
	})

	t.Run("OR Group - Rejected only when all reject", func(t *testing.T) {
		tree := &models.ApprovalNode{
			ID:   "root",
			Type: "GROUP",
			GroupCondition: models.ConditionOr,
			Children: []models.ApprovalNode{
				{ID: "node1", Type: "USER", Value: "alice"},
				{ID: "node2", Type: "USER", Value: "bob"},
			},
		}

		stepsMap := map[string][]models.WorkflowStep{
			"node1": {{NodeID: "node1", Status: "REJECTED", AssignedAt: time.Now()}},
			"node2": {{NodeID: "node2", Status: "REJECTED", AssignedAt: time.Now()}},
		}

		status := evaluateStageTree(tree, stepsMap)
		if status != NodeStatusRejected {
			t.Errorf("expected REJECTED, got %v", status)
		}
	})

	// Test case: Nested groups AND(OR(Alice, Bob), Manager)
	t.Run("Nested Groups - AND of OR and USER", func(t *testing.T) {
		tree := &models.ApprovalNode{
			ID:   "root",
			Type: "GROUP",
			GroupCondition: models.ConditionAnd,
			Children: []models.ApprovalNode{
				{
					ID:   "group1",
					Type: "GROUP",
					GroupCondition: models.ConditionOr,
					Children: []models.ApprovalNode{
						{ID: "alice_node", Type: "USER", Value: "alice"},
						{ID: "bob_node", Type: "USER", Value: "bob"},
					},
				},
				{ID: "manager_node", Type: "USER", Value: "manager"},
			},
		}

		// Scenario: Bob approved, Manager approved -> APPROVED
		stepsMap := map[string][]models.WorkflowStep{
			"alice_node":   {{NodeID: "alice_node", Status: "PENDING", AssignedAt: time.Now()}},
			"bob_node":     {{NodeID: "bob_node", Status: "APPROVED", AssignedAt: time.Now()}},
			"manager_node": {{NodeID: "manager_node", Status: "APPROVED", AssignedAt: time.Now()}},
		}

		status := evaluateStageTree(tree, stepsMap)
		if status != NodeStatusApproved {
			t.Errorf("expected APPROVED, got %v", status)
		}

		// Scenario: Bob rejected, Alice pending, Manager approved -> PENDING (Alice can still approve)
		stepsMap = map[string][]models.WorkflowStep{
			"alice_node":   {{NodeID: "alice_node", Status: "PENDING", AssignedAt: time.Now()}},
			"bob_node":     {{NodeID: "bob_node", Status: "REJECTED", AssignedAt: time.Now()}},
			"manager_node": {{NodeID: "manager_node", Status: "APPROVED", AssignedAt: time.Now()}},
		}

		status = evaluateStageTree(tree, stepsMap)
		if status != NodeStatusPending {
			t.Errorf("expected PENDING, got %v", status)
		}

		// Scenario: Bob rejected, Alice rejected, Manager approved -> REJECTED
		stepsMap = map[string][]models.WorkflowStep{
			"alice_node":   {{NodeID: "alice_node", Status: "REJECTED", AssignedAt: time.Now()}},
			"bob_node":     {{NodeID: "bob_node", Status: "REJECTED", AssignedAt: time.Now()}},
			"manager_node": {{NodeID: "manager_node", Status: "APPROVED", AssignedAt: time.Now()}},
		}

		status = evaluateStageTree(tree, stepsMap)
		if status != NodeStatusRejected {
			t.Errorf("expected REJECTED, got %v", status)
		}

		// Scenario: Alice approved, Manager rejected -> REJECTED
		stepsMap = map[string][]models.WorkflowStep{
			"alice_node":   {{NodeID: "alice_node", Status: "APPROVED", AssignedAt: time.Now()}},
			"bob_node":     {{NodeID: "bob_node", Status: "PENDING", AssignedAt: time.Now()}},
			"manager_node": {{NodeID: "manager_node", Status: "REJECTED", AssignedAt: time.Now()}},
		}

		status = evaluateStageTree(tree, stepsMap)
		if status != NodeStatusRejected {
			t.Errorf("expected REJECTED, got %v", status)
		}
	})
}
