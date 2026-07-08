package repository

import (
	"icarus-workflow-ms/internal/models"
	"testing"
)

func TestValidateWorkflowGraph(t *testing.T) {
	t.Run("Valid DAG - Sequential", func(t *testing.T) {
		nodes := []models.UpsertNodeRequest{
			{
				ID:   "node1",
				Name: "Node 1",
				Type: "APPROVAL",
				Config: map[string]interface{}{
					"policy":     "ANY",
					"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "alice"}},
				},
			},
			{
				ID:   "node2",
				Name: "Node 2",
				Type: "APPROVAL",
				Config: map[string]interface{}{
					"policy":     "ANY",
					"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "bob"}},
				},
				DependsOn: []string{"node1"},
			},
		}
		if err := ValidateWorkflowGraph(nodes); err != nil {
			t.Errorf("expected valid DAG, got error: %v", err)
		}
	})

	t.Run("Valid DAG - Parallel with Quorum", func(t *testing.T) {
		nodes := []models.UpsertNodeRequest{
			{
				ID:   "node1",
				Name: "Node 1",
				Type: "APPROVAL",
				Config: map[string]interface{}{
					"policy":        "QUORUM",
					"min_approvals": 2,
					"candidates": []models.WorkflowCandidate{
						{Type: "USER", Value: "alice"},
						{Type: "USER", Value: "bob"},
						{Type: "USER", Value: "charlie"},
					},
				},
			},
		}
		if err := ValidateWorkflowGraph(nodes); err != nil {
			t.Errorf("expected valid DAG, got error: %v", err)
		}
	})

	t.Run("Invalid - Cycle Detected", func(t *testing.T) {
		nodes := []models.UpsertNodeRequest{
			{
				ID:   "node1",
				Name: "Node 1",
				Type: "APPROVAL",
				Config: map[string]interface{}{
					"policy":     "ANY",
					"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "alice"}},
				},
				DependsOn: []string{"node3"},
			},
			{
				ID:   "node2",
				Name: "Node 2",
				Type: "APPROVAL",
				Config: map[string]interface{}{
					"policy":     "ANY",
					"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "bob"}},
				},
				DependsOn: []string{"node1"},
			},
			{
				ID:   "node3",
				Name: "Node 3",
				Type: "APPROVAL",
				Config: map[string]interface{}{
					"policy":     "ANY",
					"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "charlie"}},
				},
				DependsOn: []string{"node2"},
			},
		}
		err := ValidateWorkflowGraph(nodes)
		if err == nil {
			t.Error("expected cycle error, got nil")
		}
	})

	t.Run("Invalid - Non-existent dependency", func(t *testing.T) {
		nodes := []models.UpsertNodeRequest{
			{
				ID:   "node1",
				Name: "Node 1",
				Type: "APPROVAL",
				Config: map[string]interface{}{
					"policy":     "ANY",
					"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "alice"}},
				},
				DependsOn: []string{"node_fake"},
			},
		}
		err := ValidateWorkflowGraph(nodes)
		if err == nil {
			t.Error("expected missing dependency error, got nil")
		}
	})

	t.Run("Invalid - Empty candidates", func(t *testing.T) {
		nodes := []models.UpsertNodeRequest{
			{
				ID:   "node1",
				Name: "Node 1",
				Type: "APPROVAL",
				Config: map[string]interface{}{
					"policy":     "ANY",
					"candidates": []models.WorkflowCandidate{},
				},
			},
		}
		err := ValidateWorkflowGraph(nodes)
		if err == nil {
			t.Error("expected empty candidates error, got nil")
		}
	})

	t.Run("Invalid - Quorum min approvals bounds", func(t *testing.T) {
		nodes := []models.UpsertNodeRequest{
			{
				ID:   "node1",
				Name: "Node 1",
				Type: "APPROVAL",
				Config: map[string]interface{}{
					"policy":        "QUORUM",
					"min_approvals": 5,
					"candidates": []models.WorkflowCandidate{
						{Type: "USER", Value: "alice"},
					},
				},
			},
		}
		err := ValidateWorkflowGraph(nodes)
		if err == nil {
			t.Error("expected quorum error, got nil")
		}
	})
}

func TestEvaluateNodeStatus(t *testing.T) {
	// ANY Policy tests
	t.Run("ANY - Approved when at least one approves", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "APPROVAL",
			Config: map[string]interface{}{
				"policy":     "ANY",
				"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "alice"}, {Type: "USER", Value: "bob"}},
			},
		}
		steps := []models.ExecutionNode{
			{Status: "PENDING"},
			{Status: "APPROVED"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusApproved {
			t.Errorf("expected APPROVED, got %v", status)
		}
	})

	t.Run("ANY - Pending when none approved but some pending", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "APPROVAL",
			Config: map[string]interface{}{
				"policy":     "ANY",
				"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "alice"}, {Type: "USER", Value: "bob"}},
			},
		}
		steps := []models.ExecutionNode{
			{Status: "PENDING"},
			{Status: "REJECTED"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusPending {
			t.Errorf("expected PENDING, got %v", status)
		}
	})

	t.Run("ANY - Rejected when all rejected", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "APPROVAL",
			Config: map[string]interface{}{
				"policy":     "ANY",
				"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "alice"}, {Type: "USER", Value: "bob"}},
			},
		}
		steps := []models.ExecutionNode{
			{Status: "REJECTED"},
			{Status: "REJECTED"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusRejected {
			t.Errorf("expected REJECTED, got %v", status)
		}
	})

	// ALL Policy tests
	t.Run("ALL - Approved when all approve", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "APPROVAL",
			Config: map[string]interface{}{
				"policy":     "ALL",
				"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "alice"}, {Type: "USER", Value: "bob"}},
			},
		}
		steps := []models.ExecutionNode{
			{Status: "APPROVED"},
			{Status: "APPROVED"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusApproved {
			t.Errorf("expected APPROVED, got %v", status)
		}
	})

	t.Run("ALL - Rejected when any reject", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "APPROVAL",
			Config: map[string]interface{}{
				"policy":     "ALL",
				"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "alice"}, {Type: "USER", Value: "bob"}},
			},
		}
		steps := []models.ExecutionNode{
			{Status: "APPROVED"},
			{Status: "REJECTED"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusRejected {
			t.Errorf("expected REJECTED, got %v", status)
		}
	})

	t.Run("ALL - Pending when one approved and one pending", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "APPROVAL",
			Config: map[string]interface{}{
				"policy":     "ALL",
				"candidates": []models.WorkflowCandidate{{Type: "USER", Value: "alice"}, {Type: "USER", Value: "bob"}},
			},
		}
		steps := []models.ExecutionNode{
			{Status: "APPROVED"},
			{Status: "PENDING"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusPending {
			t.Errorf("expected PENDING, got %v", status)
		}
	})

	// QUORUM Policy tests (min=2, total candidates=3)
	t.Run("QUORUM - Approved when approvals reach threshold", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "APPROVAL",
			Config: map[string]interface{}{
				"policy":        "QUORUM",
				"min_approvals": 2,
				"candidates":    []models.WorkflowCandidate{{Type: "USER", Value: "alice"}, {Type: "USER", Value: "bob"}, {Type: "USER", Value: "charlie"}},
			},
		}
		steps := []models.ExecutionNode{
			{Status: "APPROVED"},
			{Status: "APPROVED"},
			{Status: "PENDING"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusApproved {
			t.Errorf("expected APPROVED, got %v", status)
		}
	})

	t.Run("QUORUM - Pending when threshold not reached but possible", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "APPROVAL",
			Config: map[string]interface{}{
				"policy":        "QUORUM",
				"min_approvals": 2,
				"candidates":    []models.WorkflowCandidate{{Type: "USER", Value: "alice"}, {Type: "USER", Value: "bob"}, {Type: "USER", Value: "charlie"}},
			},
		}
		steps := []models.ExecutionNode{
			{Status: "APPROVED"},
			{Status: "PENDING"},
			{Status: "PENDING"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusPending {
			t.Errorf("expected PENDING, got %v", status)
		}
	})

	t.Run("QUORUM - Rejected when threshold impossible to reach", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "APPROVAL",
			Config: map[string]interface{}{
				"policy":        "QUORUM",
				"min_approvals": 2,
				"candidates":    []models.WorkflowCandidate{{Type: "USER", Value: "alice"}, {Type: "USER", Value: "bob"}, {Type: "USER", Value: "charlie"}},
			},
		}
		steps := []models.ExecutionNode{
			{Status: "APPROVED"},
			{Status: "REJECTED"},
			{Status: "REJECTED"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusRejected {
			t.Errorf("expected REJECTED, got %v", status)
		}
	})

	// OPERATION tests
	t.Run("OPERATION - Approved when step is approved", func(t *testing.T) {
		node := models.WorkflowNode{
			Type: "OPERATION",
		}
		steps := []models.ExecutionNode{
			{Status: "APPROVED"},
		}
		status := EvaluateNodeStatus(node, steps)
		if status != NodeStatusApproved {
			t.Errorf("expected APPROVED, got %v", status)
		}
	})
}

func TestGetNextActiveNodes(t *testing.T) {
	wf := &models.Workflow{
		Nodes: []models.WorkflowNode{
			{ID: "node1", DependsOn: []string{}},
			{ID: "node2", DependsOn: []string{"node1"}},
			{ID: "node3", DependsOn: []string{"node1"}},
			{ID: "node4", DependsOn: []string{"node2", "node3"}},
		},
	}

	t.Run("Initial state - node1 ready", func(t *testing.T) {
		stepMap := make(map[string][]models.ExecutionNode)
		nodeStatuses := make(map[string]NodeStatus)

		next := GetNextActiveNodes(wf, stepMap, nodeStatuses)
		if len(next) != 1 || next[0].ID != "node1" {
			t.Errorf("expected node1 only, got %v", next)
		}
	})

	t.Run("Node1 approved - node2 and node3 ready", func(t *testing.T) {
		stepMap := map[string][]models.ExecutionNode{
			"node1": {{Status: "APPROVED"}},
		}
		nodeStatuses := map[string]NodeStatus{
			"node1": NodeStatusApproved,
		}

		next := GetNextActiveNodes(wf, stepMap, nodeStatuses)
		if len(next) != 2 ||
			(next[0].ID != "node2" && next[1].ID != "node2") ||
			(next[0].ID != "node3" && next[1].ID != "node3") {
			t.Errorf("expected node2 and node3, got %v", next)
		}
	})

	t.Run("Node1 & Node2 approved, Node3 pending - node4 not ready", func(t *testing.T) {
		stepMap := map[string][]models.ExecutionNode{
			"node1": {{Status: "APPROVED"}},
			"node2": {{Status: "APPROVED"}},
			"node3": {{Status: "PENDING"}},
		}
		nodeStatuses := map[string]NodeStatus{
			"node1": NodeStatusApproved,
			"node2": NodeStatusApproved,
			"node3": NodeStatusPending,
		}

		next := GetNextActiveNodes(wf, stepMap, nodeStatuses)
		if len(next) != 0 {
			t.Errorf("expected no ready nodes, got %v", next)
		}
	})

	t.Run("Node1, Node2 & Node3 approved - node4 ready", func(t *testing.T) {
		stepMap := map[string][]models.ExecutionNode{
			"node1": {{Status: "APPROVED"}},
			"node2": {{Status: "APPROVED"}},
			"node3": {{Status: "APPROVED"}},
		}
		nodeStatuses := map[string]NodeStatus{
			"node1": NodeStatusApproved,
			"node2": NodeStatusApproved,
			"node3": NodeStatusApproved,
		}

		next := GetNextActiveNodes(wf, stepMap, nodeStatuses)
		if len(next) != 1 || next[0].ID != "node4" {
			t.Errorf("expected node4, got %v", next)
		}
	})
}
