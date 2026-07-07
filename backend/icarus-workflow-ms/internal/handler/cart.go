package handler

import (
	"encoding/json"
	"fmt"
	"icarus-workflow-ms/internal/models"
	"icarus-workflow-ms/internal/repository"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// CreateCartHandler POST /api/v1/access/carts
func (s *HandlerServer) CreateCartHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)

	var body struct {
		Justification string `json:"justification"`
	}
	_ = s.decodeJSON(r, &body)

	cart := &models.AccessCart{
		ID:            uuid.New().String(),
		RequesterID:   claims.Subject,
		Status:        "DRAFT",
		Justification: body.Justification,
	}
	if err := s.Repo.CreateCart(cart); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to create cart: "+err.Error())
		return
	}
	cart, _ = s.Repo.GetCart(cart.ID)
	s.respondWithJSON(w, http.StatusCreated, cart)
}

// ListCartsHandler GET /api/v1/access/carts
func (s *HandlerServer) ListCartsHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	carts, err := s.Repo.ListCarts(claims.Subject)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if carts == nil {
		carts = []models.AccessCart{}
	}
	s.respondWithJSON(w, http.StatusOK, carts)
}

// GetCartHandler GET /api/v1/access/carts/{cart_id}
func (s *HandlerServer) GetCartHandler(w http.ResponseWriter, r *http.Request) {
	cartID := r.PathValue("cart_id")
	cart, err := s.Repo.GetCart(cartID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, cart)
}

// AddCartItemHandler POST /api/v1/access/carts/{cart_id}/items
func (s *HandlerServer) AddCartItemHandler(w http.ResponseWriter, r *http.Request) {
	cartID := r.PathValue("cart_id")

	cart, err := s.Repo.GetCart(cartID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "cart not found")
		return
	}
	if cart.Status != "DRAFT" {
		s.respondWithError(w, http.StatusBadRequest, "cannot add items to a submitted cart")
		return
	}

	var body struct {
		RoleID   string `json:"role_id"`
		RoleName string `json:"role_name"`
	}
	if err := s.decodeJSON(r, &body); err != nil || body.RoleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "role_id is required")
		return
	}
	if body.RoleName == "" {
		body.RoleName = body.RoleID
	}

	item := &models.CartItem{
		ID:       uuid.New().String(),
		CartID:   cartID,
		RoleID:   body.RoleID,
		RoleName: body.RoleName,
		Status:   "PENDING",
	}
	if err := s.Repo.AddCartItem(item); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to add item: "+err.Error())
		return
	}
	item, _ = s.Repo.GetCartItem(item.ID)
	s.respondWithJSON(w, http.StatusCreated, item)
}

// RemoveCartItemHandler DELETE /api/v1/access/carts/{cart_id}/items/{item_id}
func (s *HandlerServer) RemoveCartItemHandler(w http.ResponseWriter, r *http.Request) {
	cartID := r.PathValue("cart_id")
	itemID := r.PathValue("item_id")

	cart, err := s.Repo.GetCart(cartID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "cart not found")
		return
	}
	if cart.Status != "DRAFT" {
		s.respondWithError(w, http.StatusBadRequest, "cannot remove items from a submitted cart")
		return
	}

	if err := s.Repo.RemoveCartItem(itemID, cartID); err != nil {
		s.respondWithError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SubmitCartHandler POST /api/v1/access/carts/{cart_id}/submit
// Core orchestration logic: fans out workflow instances per item, handles auto-approve.
func (s *HandlerServer) SubmitCartHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	cartID := r.PathValue("cart_id")
	correlationID := uuid.New().String()

	cart, err := s.Repo.GetCart(cartID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "cart not found")
		return
	}
	if cart.RequesterID != claims.Subject {
		s.respondWithError(w, http.StatusForbidden, "not your cart")
		return
	}
	if cart.Status != "DRAFT" {
		s.respondWithError(w, http.StatusBadRequest, "cart is already submitted")
		return
	}
	if len(cart.Items) == 0 {
		s.respondWithError(w, http.StatusBadRequest, "cart has no items")
		return
	}

	// Transition cart to SUBMITTED
	if err := s.Repo.UpdateCartStatus(cartID, "SUBMITTED"); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to update cart status")
		return
	}

	// Audit: cart submitted
	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "CART", EntityID: cartID,
		ActorUserID: claims.Subject, Action: "SUBMITTED", CorrelationID: correlationID,
	})

	anyInProgress := false

	for _, item := range cart.Items {
		// Look up workflow definition for this role
		def, err := s.Repo.GetWorkflowDefinitionByRoleID(item.RoleID)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "workflow lookup failed: "+err.Error())
			return
		}

		if def == nil {
			// No mapping found — AUTO APPROVE
			_ = s.Repo.UpdateCartItemStatus(item.ID, "APPROVED", nil)
			_ = s.Repo.WriteAuditLog(&models.AuditLog{
				ID: uuid.New().String(), EntityType: "CART_ITEM", EntityID: item.ID,
				ActorUserID: "system", Action: "AUTO_APPROVED",
				AfterState: `{"reason":"no workflow mapping for role"}`,
				CorrelationID: correlationID,
			})
			continue
		}

		// Create workflow instance pinned to this definition version
		instanceID := uuid.New().String()
		inst := &models.WorkflowInstance{
			ID:                   instanceID,
			WorkflowDefinitionID: def.ID,
			CartItemID:           item.ID,
			CurrentStageSeq:      1,
		}
		if err := s.Repo.CreateWorkflowInstance(inst); err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "failed to create workflow instance")
			return
		}

		_ = s.Repo.UpdateCartItemStatus(item.ID, "IN_PROGRESS", &instanceID)

		// Create steps for stage 1
		stage1Steps, err := s.createStepsForStage(instanceID, def, 1, correlationID)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "failed to create approval steps")
			return
		}

		// Notify approvers via SSE
		for _, step := range stage1Steps {
			notifUsers := s.resolveNotificationTargets(step)
			for _, uid := range notifUsers {
				payload := fmt.Sprintf(`{"step_id":"%s","role":"%s","cart_id":"%s"}`,
					step.ID, item.RoleName, cartID)
				_ = s.Repo.CreateSSENotification(&models.SSENotification{
					ID: uuid.New().String(), UserID: uid,
					EventType: "inbox.new", Payload: payload,
				})
				s.SSEBroker.Publish(uid, "inbox.new", payload)
			}
		}

		_ = s.Repo.WriteAuditLog(&models.AuditLog{
			ID: uuid.New().String(), EntityType: "WORKFLOW_INSTANCE", EntityID: instanceID,
			ActorUserID: "system", Action: "STARTED", CorrelationID: correlationID,
		})

		anyInProgress = true
	}

	if !anyInProgress {
		// All items auto-approved — cart is immediately completed
		_ = s.Repo.UpdateCartStatus(cartID, "COMPLETED")
	} else {
		_ = s.Repo.UpdateCartStatus(cartID, "IN_PROGRESS")
	}

	updated, _ := s.Repo.GetCart(cartID)
	s.respondWithJSON(w, http.StatusAccepted, map[string]interface{}{
		"cart_id":    cartID,
		"status":     updated.Status,
		"item_count": len(cart.Items),
		"message":    "Request submitted. Approvers have been notified.",
	})
}

// createStepsForStage instantiates WorkflowStep rows for all approvers at the given stage sequence.
func (s *HandlerServer) createStepsForStage(instanceID string, def *models.WorkflowDefinition,
	stageSeq int, correlationID string) ([]models.WorkflowStep, error) {

	var createdSteps []models.WorkflowStep
	for _, stage := range def.Stages {
		if stage.SequenceOrder != stageSeq {
			continue
		}
		
		leaves := repository.GetLeafNodes(stage.ApprovalTree)
		for _, leaf := range leaves {
			step := models.WorkflowStep{
				ID:                 uuid.New().String(),
				WorkflowInstanceID: instanceID,
				StageDefinitionID:  stage.ID,
				NodeID:             leaf.ID,
				AssignedAt:         time.Now(),
			}
			switch leaf.Type {
			case "USER":
				val := leaf.Value
				step.AssignedToUserID = &val
			case "ROLE":
				val := leaf.Value
				step.AssignedToRole = &val
			}
			if err := s.Repo.CreateWorkflowStep(&step); err != nil {
				return nil, err
			}
			_ = s.Repo.WriteAuditLog(&models.AuditLog{
				ID: uuid.New().String(), EntityType: "WORKFLOW_STEP", EntityID: step.ID,
				ActorUserID: "system", Action: "ASSIGNED",
				AfterState: fmt.Sprintf(`{"stage":"%s","seq":%d,"node_id":"%s"}`, stage.Name, stageSeq, leaf.ID),
				CorrelationID: correlationID,
			})
			createdSteps = append(createdSteps, step)
		}
	}
	return createdSteps, nil
}


// resolveNotificationTargets returns user IDs to notify for a given step.
func (s *HandlerServer) resolveNotificationTargets(step models.WorkflowStep) []string {
	if step.AssignedToUserID != nil {
		return []string{*step.AssignedToUserID}
	}
	// For ROLE_QUEUE: fetch members from admin-ms
	if step.AssignedToRole != nil {
		members := s.fetchRoleMembers(*step.AssignedToRole)
		return members
	}
	return nil
}

// fetchRoleMembers calls admin-ms internal endpoint to get user IDs for a role.
func (s *HandlerServer) fetchRoleMembers(roleName string) []string {
	resp, err := http.Get(s.AdminMSURL + "/internal/roles/" + roleName + "/members")
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()
	var members []string
	_ = json.NewDecoder(resp.Body).Decode(&members)
	return members
}
