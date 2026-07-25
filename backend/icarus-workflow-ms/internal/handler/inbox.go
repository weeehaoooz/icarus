package handler

import (
	"fmt"
	"icarus-workflow-ms/internal/crypto"
	"icarus-workflow-ms/internal/models"
	"icarus-workflow-ms/internal/securitylog"
	"net/http"
	"strings"

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
		s.SecLogger.LogEvent(r.Context(), securitylog.Event{
			EventType:      securitylog.DomainAccessControl,
			Action:         "DELEGATE_STEP",
			Severity:       securitylog.SeverityWarn,
			Actor:          claims.Subject,
			ActorIP:        securitylog.GetClientIP(r),
			UserAgent:      r.UserAgent(),
			TargetResource: fmt.Sprintf("step:%s", stepID),
			Status:         securitylog.StatusFailure,
			Details:        map[string]interface{}{"reason": "not an assigned approver for this step"},
		})
		s.respondWithError(w, http.StatusForbidden, "not an assigned approver for this step")
		return
	}

	newStepID := uuid.New().String()
	newStep, err := s.Repo.DelegateStep(stepID, body.Comment, claims.Subject, *body.DelegateToUser, newStepID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delegate step: "+err.Error())
		return
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainWorkflow,
		Action:         "DELEGATE_STEP",
		Severity:       securitylog.SeverityInfo,
		Actor:          claims.Subject,
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: fmt.Sprintf("step:%s", stepID),
		Status:         securitylog.StatusSuccess,
		Details:        map[string]interface{}{"delegated_to": *body.DelegateToUser},
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
	correlationID := uuid.New().String()

	var body models.StepActionRequest
	_ = s.decodeJSON(r, &body)

	step, err := s.Repo.GetWorkflowStep(stepID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "step not found")
		return
	}
	if !s.isAuthorizedApprover(claims, step) {
		s.SecLogger.LogEvent(r.Context(), securitylog.Event{
			EventType:      securitylog.DomainAccessControl,
			Action:         newStatus + "_STEP",
			Severity:       securitylog.SeverityWarn,
			Actor:          claims.Subject,
			ActorIP:        securitylog.GetClientIP(r),
			UserAgent:      r.UserAgent(),
			TargetResource: fmt.Sprintf("step:%s", stepID),
			Status:         securitylog.StatusFailure,
			Details:        map[string]interface{}{"reason": "not an assigned approver for this step"},
		})
		s.respondWithError(w, http.StatusForbidden, "not an assigned approver for this step")
		return
	}

	inst, nextSteps, _, nextMsg, err := s.Repo.ActionStepAndProgress(stepID, newStatus, body.Comment, claims.Subject)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to action step: "+err.Error())
		return
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainWorkflow,
		Action:         newStatus + "_STEP",
		Severity:       securitylog.SeverityInfo,
		Actor:          claims.Subject,
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: fmt.Sprintf("step:%s", stepID),
		Status:         securitylog.StatusSuccess,
		Details:        map[string]interface{}{"workflow_instance_status": inst.Status},
	})

	// Check blockers: if any of the new steps are assigned to a role with 0 members, report it!
	var blockerMsgs []string
	for _, ns := range nextSteps {
		if ns.AssignedToRole != nil {
			members := s.fetchRoleMembers(*ns.AssignedToRole)
			if len(members) == 0 {
				blockerMsg := fmt.Sprintf("Role '%s' assigned to step '%s' (node '%s') has no active members.", *ns.AssignedToRole, ns.ID, ns.NodeName)
				blockerMsgs = append(blockerMsgs, blockerMsg)
				// Write audit blocker log
				_ = s.Repo.WriteAuditLog(&models.AuditLog{
					ID: uuid.New().String(), EntityType: "WORKFLOW_STEP", EntityID: ns.ID,
					ActorUserID: "system", Action: "BLOCKED",
					AfterState:    fmt.Sprintf(`{"reason":"%s"}`, blockerMsg),
					CorrelationID: correlationID,
				})
			}
		}
	}

	if len(blockerMsgs) > 0 {
		combinedBlocker := strings.Join(blockerMsgs, "; ")
		_ = s.Repo.UpdateExecutionError(inst.ID, combinedBlocker)
		_ = s.Repo.WriteAuditLog(&models.AuditLog{
			ID: uuid.New().String(), EntityType: "WORKFLOW_INSTANCE", EntityID: inst.ID,
			ActorUserID: "system", Action: "BLOCKED",
			AfterState:    fmt.Sprintf(`{"reason":"%s"}`, combinedBlocker),
			CorrelationID: correlationID,
		})
	}

	// Notify new approvers (if stage advanced)
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

	// Notify requester of cart item update
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
		"step_id":                  stepID,
		"status":                   newStatus,
		"workflow_instance_status": inst.Status,
		"message":                  nextMsg,
	})
}

// isAuthorizedApprover checks if the acting user is the direct assignee, holds the assigned role, or is an admin.
func (s *HandlerServer) isAuthorizedApprover(claims *crypto.CustomClaims, step *models.ExecutionNode) bool {
	// Admins are always authorized approvers
	for _, r := range claims.Roles {
		if r == "admin" {
			return true
		}
	}
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
