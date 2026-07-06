package handler

import (
	"fmt"
	"icarus-workflow-ms/internal/crypto"
	"icarus-workflow-ms/internal/models"
	"net/http"

	"github.com/google/uuid"
)

// GetInboxHandler GET /api/v1/workflow/inbox
func (s *HandlerServer) GetInboxHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	items, err := s.Repo.GetInboxItems(claims.Subject, claims.Roles)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []models.InboxItem{}
	}
	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": len(items),
	})
}

// ApproveStepHandler POST /api/v1/workflow/steps/{step_id}/approve
func (s *HandlerServer) ApproveStepHandler(w http.ResponseWriter, r *http.Request) {
	s.actionStep(w, r, "APPROVED")
}

// RejectStepHandler POST /api/v1/workflow/steps/{step_id}/reject
func (s *HandlerServer) RejectStepHandler(w http.ResponseWriter, r *http.Request) {
	s.actionStep(w, r, "REJECTED")
}

// DelegateStepHandler POST /api/v1/workflow/steps/{step_id}/delegate
func (s *HandlerServer) DelegateStepHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	stepID := r.PathValue("step_id")

	var body models.StepActionRequest
	if err := s.decodeJSON(r, &body); err != nil || body.DelegateToUser == nil {
		s.respondWithError(w, http.StatusBadRequest, "delegate_to_user_id is required")
		return
	}

	step, err := s.Repo.GetWorkflowStep(stepID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "step not found")
		return
	}
	if !s.isAuthorizedApprover(claims, step) {
		s.respondWithError(w, http.StatusForbidden, "not an assigned approver for this step")
		return
	}
	if step.Status != "PENDING" {
		s.respondWithError(w, http.StatusBadRequest, "step is no longer pending")
		return
	}

	// Mark original step as DELEGATED
	if err := s.Repo.ActionWorkflowStep(stepID, "DELEGATED", body.Comment, claims.Subject); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delegate step")
		return
	}

	// Create a new PENDING step for the delegate
	newStep := &models.WorkflowStep{
		ID:                 uuid.New().String(),
		WorkflowInstanceID: step.WorkflowInstanceID,
		StageDefinitionID:  step.StageDefinitionID,
		AssignedToUserID:   body.DelegateToUser,
	}
	if err := s.Repo.CreateWorkflowStep(newStep); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to create delegated step")
		return
	}

	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "WORKFLOW_STEP", EntityID: stepID,
		ActorUserID: claims.Subject, Action: "DELEGATED",
		AfterState: fmt.Sprintf(`{"delegated_to":"%s","new_step_id":"%s"}`, *body.DelegateToUser, newStep.ID),
	})

	// Notify delegate via SSE
	payload := fmt.Sprintf(`{"step_id":"%s"}`, newStep.ID)
	_ = s.Repo.CreateSSENotification(&models.SSENotification{
		ID: uuid.New().String(), UserID: *body.DelegateToUser,
		EventType: "inbox.new", Payload: payload,
	})
	s.SSEBroker.Publish(*body.DelegateToUser, "inbox.new", payload)

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"step_id":     stepID,
		"status":      "DELEGATED",
		"new_step_id": newStep.ID,
		"assigned_to": *body.DelegateToUser,
	})
}

// ─── Shared action logic ──────────────────────────────────────────────────────

func (s *HandlerServer) actionStep(w http.ResponseWriter, r *http.Request, newStatus string) {
	claims := s.claimsFrom(r)
	stepID := r.PathValue("step_id")

	var body models.StepActionRequest
	_ = s.decodeJSON(r, &body)

	step, err := s.Repo.GetWorkflowStep(stepID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "step not found")
		return
	}
	if !s.isAuthorizedApprover(claims, step) {
		s.respondWithError(w, http.StatusForbidden, "not an assigned approver for this step")
		return
	}
	if step.Status != "PENDING" {
		s.respondWithError(w, http.StatusBadRequest, "step is no longer pending")
		return
	}

	if err := s.Repo.ActionWorkflowStep(stepID, newStatus, body.Comment, claims.Subject); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to action step")
		return
	}

	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "WORKFLOW_STEP", EntityID: stepID,
		ActorUserID: claims.Subject, Action: newStatus,
		AfterState: fmt.Sprintf(`{"comment":"%s"}`, body.Comment),
	})

	inst, err := s.Repo.GetWorkflowInstance(step.WorkflowInstanceID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to get workflow instance")
		return
	}

	instanceStatus, nextMsg := s.advanceWorkflow(inst, step, newStatus, claims.Subject)

	// Notify requester via SSE
	cartItem, _ := s.Repo.GetCartItem(inst.CartItemID)
	if cartItem != nil {
		cart, _ := s.Repo.GetCart(cartItem.CartID)
		if cart != nil {
			payload := fmt.Sprintf(`{"cart_id":"%s","item_id":"%s","status":"%s"}`,
				cart.ID, cartItem.ID, newStatus)
			_ = s.Repo.CreateSSENotification(&models.SSENotification{
				ID: uuid.New().String(), UserID: cart.RequesterID,
				EventType: "cart.updated", Payload: payload,
			})
			s.SSEBroker.Publish(cart.RequesterID, "cart.updated", payload)
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"step_id":                 stepID,
		"status":                  newStatus,
		"workflow_instance_status": instanceStatus,
		"message":                 nextMsg,
	})
}

// advanceWorkflow checks quorum and either advances to the next stage or closes the instance.
func (s *HandlerServer) advanceWorkflow(inst *models.WorkflowInstance, step *models.WorkflowStep,
	stepStatus, actorID string) (string, string) {

	if stepStatus == "REJECTED" {
		_ = s.Repo.UpdateWorkflowInstanceStatus(inst.ID, "REJECTED", inst.CurrentStageSeq)
		_ = s.Repo.UpdateCartItemStatus(inst.CartItemID, "REJECTED", nil)
		_ = s.checkCartCompletion(inst.CartItemID)
		_ = s.Repo.WriteAuditLog(&models.AuditLog{
			ID: uuid.New().String(), EntityType: "WORKFLOW_INSTANCE", EntityID: inst.ID,
			ActorUserID: actorID, Action: "REJECTED",
		})
		return "REJECTED", "Request rejected"
	}

	// Approved — check quorum for current stage
	stageSteps, err := s.Repo.GetStepsForInstanceStage(inst.ID, inst.CurrentStageSeq)
	if err != nil {
		return inst.Status, "error checking quorum"
	}

	// Get stage definition to find quorum requirement
	def, err := s.Repo.GetWorkflowDefinitionByID(inst.WorkflowDefinitionID)
	if err != nil || def == nil {
		return inst.Status, "error fetching definition"
	}

	var currentStage *models.WorkflowStageDefinition
	for i, st := range def.Stages {
		if st.SequenceOrder == inst.CurrentStageSeq {
			currentStage = &def.Stages[i]
			break
		}
	}
	if currentStage == nil {
		return inst.Status, "stage not found"
	}

	approvedCount := 0
	for _, s2 := range stageSteps {
		if s2.Status == "APPROVED" {
			approvedCount++
		}
	}

	if approvedCount < currentStage.ApprovalQuorum {
		// Quorum not yet met — wait
		return "IN_PROGRESS", fmt.Sprintf("Approved %d/%d required", approvedCount, currentStage.ApprovalQuorum)
	}

	// Find next stage
	nextSeq := -1
	for _, st := range def.Stages {
		if st.SequenceOrder > inst.CurrentStageSeq {
			if nextSeq == -1 || st.SequenceOrder < nextSeq {
				nextSeq = st.SequenceOrder
			}
		}
	}

	if nextSeq == -1 {
		// No more stages — COMPLETED
		_ = s.Repo.UpdateWorkflowInstanceStatus(inst.ID, "COMPLETED", inst.CurrentStageSeq)
		_ = s.Repo.UpdateCartItemStatus(inst.CartItemID, "APPROVED", nil)
		_ = s.checkCartCompletion(inst.CartItemID)
		_ = s.Repo.WriteAuditLog(&models.AuditLog{
			ID: uuid.New().String(), EntityType: "WORKFLOW_INSTANCE", EntityID: inst.ID,
			ActorUserID: actorID, Action: "COMPLETED",
		})
		return "COMPLETED", "All stages approved — access granted"
	}

	// Advance to next stage
	_ = s.Repo.UpdateWorkflowInstanceStatus(inst.ID, "IN_PROGRESS", nextSeq)
	inst.CurrentStageSeq = nextSeq
	nextSteps, _ := s.createStepsForStage(inst.ID, def, nextSeq, uuid.New().String())
	for _, ns := range nextSteps {
		targets := s.resolveNotificationTargets(ns)
		for _, uid := range targets {
			payload := fmt.Sprintf(`{"step_id":"%s"}`, ns.ID)
			_ = s.Repo.CreateSSENotification(&models.SSENotification{
				ID: uuid.New().String(), UserID: uid,
				EventType: "inbox.new", Payload: payload,
			})
			s.SSEBroker.Publish(uid, "inbox.new", payload)
		}
	}

	return "IN_PROGRESS", fmt.Sprintf("Advanced to stage %d", nextSeq)
}

// checkCartCompletion checks if all items are terminal and updates cart status.
func (s *HandlerServer) checkCartCompletion(cartItemID string) error {
	item, err := s.Repo.GetCartItem(cartItemID)
	if err != nil {
		return err
	}
	done, finalStatus, err := s.Repo.CheckAllCartItemsTerminal(item.CartID)
	if err != nil {
		return err
	}
	if done {
		return s.Repo.UpdateCartStatus(item.CartID, finalStatus)
	}
	return nil
}

// isAuthorizedApprover checks if the acting user is the direct assignee or holds the assigned role.
func (s *HandlerServer) isAuthorizedApprover(claims *crypto.CustomClaims, step *models.WorkflowStep) bool {
	if step.AssignedToUserID != nil && *step.AssignedToUserID == claims.Subject {
		return true
	}
	if step.AssignedToRole != nil {
		for _, r := range claims.Roles {
			if r == *step.AssignedToRole {
				return true
			}
		}
	}
	return false
}
