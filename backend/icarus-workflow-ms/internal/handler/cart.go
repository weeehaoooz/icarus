package handler

import (
	"encoding/json"
	"fmt"
	"icarus-workflow-ms/internal/models"
	"net/http"
	"strings"

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
		def, err := s.Repo.GetWorkflowByRoleID(item.RoleID)
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
				AfterState:    `{"reason":"no workflow mapping for role"}`,
				CorrelationID: correlationID,
			})
			continue
		}

		// Create execution run pinned to this workflow version
		executionID := uuid.New().String()
		exec := &models.Execution{
			ID:         executionID,
			WorkflowID: def.ID,
			CartItemID: item.ID,
			Status:     "IN_PROGRESS",
		}
		newSteps, err := s.Repo.StartExecutionAndProgress(exec, claims.Subject)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "failed to start execution: "+err.Error())
			return
		}

		_ = s.Repo.UpdateCartItemStatus(item.ID, "IN_PROGRESS", &executionID)

		// Check blockers: if any new step is assigned to a role with 0 members, record it
		var blockerMsgs []string
		for _, step := range newSteps {
			if step.AssignedToRole != nil {
				members := s.fetchRoleMembers(*step.AssignedToRole)
				if len(members) == 0 {
					blockerMsg := fmt.Sprintf("Role '%s' assigned to step '%s' (node '%s') has no active members.", *step.AssignedToRole, step.ID, step.NodeName)
					blockerMsgs = append(blockerMsgs, blockerMsg)
					// Write audit blocker log
					_ = s.Repo.WriteAuditLog(&models.AuditLog{
						ID: uuid.New().String(), EntityType: "WORKFLOW_STEP", EntityID: step.ID,
						ActorUserID: "system", Action: "BLOCKED",
						AfterState:    fmt.Sprintf(`{"reason":"%s"}`, blockerMsg),
						CorrelationID: correlationID,
					})
				}
			}
		}

		if len(blockerMsgs) > 0 {
			combinedBlocker := strings.Join(blockerMsgs, "; ")
			_ = s.Repo.UpdateExecutionError(executionID, combinedBlocker)
			_ = s.Repo.WriteAuditLog(&models.AuditLog{
				ID: uuid.New().String(), EntityType: "WORKFLOW_INSTANCE", EntityID: executionID,
				ActorUserID: "system", Action: "BLOCKED",
				AfterState:    fmt.Sprintf(`{"reason":"%s"}`, combinedBlocker),
				CorrelationID: correlationID,
			})
		}

		// Notify approvers via SSE
		for _, step := range newSteps {
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
			ID: uuid.New().String(), EntityType: "WORKFLOW_INSTANCE", EntityID: executionID,
			ActorUserID: "system", Action: "STARTED", CorrelationID: correlationID,
		})

		anyInProgress = true
	}

	if !anyInProgress {
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

// resolveNotificationTargets returns user IDs to notify for a given step.
func (s *HandlerServer) resolveNotificationTargets(step models.ExecutionNode) []string {
	if step.AssignedToUserID != nil {
		return []string{*step.AssignedToUserID}
	}
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
