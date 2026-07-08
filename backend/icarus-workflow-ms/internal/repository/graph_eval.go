package repository

import (
	"fmt"
	"icarus-workflow-ms/internal/models"
)

type NodeStatus string

const (
	NodeStatusPending  NodeStatus = "PENDING"
	NodeStatusApproved NodeStatus = "APPROVED"
	NodeStatusRejected NodeStatus = "REJECTED"
)

func parseCandidates(config map[string]interface{}) ([]models.WorkflowCandidate, error) {
	if config == nil {
		return nil, fmt.Errorf("missing configuration")
	}
	cListVal, ok := config["candidates"]
	if !ok {
		return nil, fmt.Errorf("missing 'candidates' in configuration")
	}

	list, ok := cListVal.([]interface{})
	if !ok {
		if typedList, ok := cListVal.([]models.WorkflowCandidate); ok {
			return typedList, nil
		}
		return nil, fmt.Errorf("'candidates' must be a JSON array")
	}

	var candidates []models.WorkflowCandidate
	for i, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("candidate at index %d is not a valid object", i)
		}
		tVal, _ := m["type"].(string)
		vVal, _ := m["value"].(string)
		if tVal == "" || vVal == "" {
			return nil, fmt.Errorf("candidate at index %d must have type and value", i)
		}
		candidates = append(candidates, models.WorkflowCandidate{Type: tVal, Value: vVal})
	}
	return candidates, nil
}

// ValidateWorkflowGraph validates the graph configuration, dependencies, policies, and cycle detection.
func ValidateWorkflowGraph(nodes []models.UpsertNodeRequest) error {
	nodeMap := make(map[string]bool)
	adj := make(map[string][]string)

	for _, n := range nodes {
		if n.ID == "" {
			return fmt.Errorf("node ID cannot be empty")
		}
		if nodeMap[n.ID] {
			return fmt.Errorf("duplicate node ID: %s", n.ID)
		}
		nodeMap[n.ID] = true

		if n.Type == "" {
			return fmt.Errorf("node type cannot be empty for node %s", n.ID)
		}

		if n.Type == "APPROVAL" {
			if n.Config == nil {
				return fmt.Errorf("configuration map is required for approval node %s", n.ID)
			}
			candidates, err := parseCandidates(n.Config)
			if err != nil {
				return fmt.Errorf("invalid candidates for node %s: %w", n.ID, err)
			}
			if len(candidates) == 0 {
				return fmt.Errorf("candidates list cannot be empty for node %s", n.ID)
			}
			policyStr, _ := n.Config["policy"].(string)
			policy := models.ApprovalPolicy(policyStr)
			if policy != models.PolicyAny && policy != models.PolicyAll && policy != models.PolicyQuorum {
				return fmt.Errorf("invalid policy '%s' for approval node %s", policy, n.ID)
			}
			if policy == models.PolicyQuorum {
				minApprovals := 0
				if minVal, ok := n.Config["min_approvals"]; ok {
					switch v := minVal.(type) {
					case float64:
						minApprovals = int(v)
					case int:
						minApprovals = v
					}
				}
				if minApprovals <= 0 || minApprovals > len(candidates) {
					return fmt.Errorf("invalid min_approvals: must be between 1 and number of candidates for node %s", n.ID)
				}
			}
		}
	}

	// Build adjacency list (from node to what it depends on)
	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			if !nodeMap[dep] {
				return fmt.Errorf("node %s depends on non-existent node %s", n.ID, dep)
			}
			adj[n.ID] = append(adj[n.ID], dep)
		}
	}

	// Cycle detection using graph coloring (0=unvisited, 1=visiting, 2=visited)
	visited := make(map[string]int)
	var dfs func(string) bool
	dfs = func(u string) bool {
		visited[u] = 1 // visiting
		for _, v := range adj[u] {
			if visited[v] == 1 {
				return true // cycle detected
			}
			if visited[v] == 0 {
				if dfs(v) {
					return true
				}
			}
		}
		visited[u] = 2 // visited
		return false
	}

	for _, n := range nodes {
		if visited[n.ID] == 0 {
			if dfs(n.ID) {
				return fmt.Errorf("workflow graph contains a cycle")
			}
		}
	}

	return nil
}

// EvaluateNodeStatus evaluates the execution status of a single node based on its actioned steps.
func EvaluateNodeStatus(node models.WorkflowNode, steps []models.ExecutionNode) NodeStatus {
	if node.Type != "APPROVAL" {
		if len(steps) == 0 {
			return NodeStatusPending
		}
		activeStep := &steps[0]
		switch activeStep.Status {
		case "APPROVED":
			return NodeStatusApproved
		case "REJECTED":
			return NodeStatusRejected
		default:
			return NodeStatusPending
		}
	}

	if len(steps) == 0 {
		return NodeStatusPending
	}

	candidates, err := parseCandidates(node.Config)
	if err != nil || len(candidates) == 0 {
		return NodeStatusRejected
	}

	policyStr, _ := node.Config["policy"].(string)
	policy := models.ApprovalPolicy(policyStr)

	minApprovals := 1
	if minVal, ok := node.Config["min_approvals"]; ok {
		switch v := minVal.(type) {
		case float64:
			minApprovals = int(v)
		case int:
			minApprovals = v
		}
	}

	var numApproved, numRejected, numPending int
	for _, s := range steps {
		switch s.Status {
		case "APPROVED":
			numApproved++
		case "REJECTED":
			numRejected++
		case "PENDING":
			numPending++
		}
	}

	requiredApprovals := minApprovals
	switch policy {
	case models.PolicyAny:
		requiredApprovals = 1
	case models.PolicyAll:
		requiredApprovals = len(candidates)
	case models.PolicyQuorum:
		if requiredApprovals <= 0 {
			requiredApprovals = 1
		}
	}

	if numApproved >= requiredApprovals {
		return NodeStatusApproved
	}
	if numApproved+numPending < requiredApprovals {
		return NodeStatusRejected
	}
	return NodeStatusPending
}

// GetEntryNodes returns definition nodes that have no dependencies.
func GetEntryNodes(wf *models.Workflow) []models.WorkflowNode {
	var entry []models.WorkflowNode
	for _, n := range wf.Nodes {
		if len(n.DependsOn) == 0 {
			entry = append(entry, n)
		}
	}
	return entry
}

// GetNextActiveNodes identifies nodes that can be activated next in the DAG.
func GetNextActiveNodes(wf *models.Workflow, stepMap map[string][]models.ExecutionNode, nodeStatuses map[string]NodeStatus) []models.WorkflowNode {
	var nextNodes []models.WorkflowNode

	for _, node := range wf.Nodes {
		if len(stepMap[node.ID]) > 0 {
			continue
		}

		allDepsApproved := true
		for _, depID := range node.DependsOn {
			if status, ok := nodeStatuses[depID]; !ok || status != NodeStatusApproved {
				allDepsApproved = false
				break
			}
		}

		if allDepsApproved {
			nextNodes = append(nextNodes, node)
		}
	}

	return nextNodes
}
