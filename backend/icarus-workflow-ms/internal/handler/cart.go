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
	includeArchived := r.URL.Query().Get("include_archived") == "true"
	carts, err := s.Repo.ListCarts(claims.Subject, includeArchived)
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

	var body struct {
		Justification string `json:"justification"`
	}
	_ = s.decodeJSON(r, &body)

	if body.Justification != "" {
		if err := s.Repo.UpdateCartJustification(cartID, body.Justification); err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "failed to update cart justification: "+err.Error())
			return
		}
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

// WithdrawCartHandler POST /api/v1/access/carts/{cart_id}/withdraw
func (s *HandlerServer) WithdrawCartHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	cartID := r.PathValue("cart_id")

	cart, err := s.Repo.GetCart(cartID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "cart not found")
		return
	}
	if cart.RequesterID != claims.Subject {
		s.respondWithError(w, http.StatusForbidden, "not your cart")
		return
	}
	if cart.Status != "SUBMITTED" && cart.Status != "IN_PROGRESS" {
		s.respondWithError(w, http.StatusBadRequest, "only submitted or in-progress requests can be withdrawn")
		return
	}

	// Get pending steps before withdrawal to notify approvers to refresh their inbox
	pendingSteps, _ := s.Repo.GetPendingStepsForCart(cartID)

	if err := s.Repo.WithdrawCart(cartID, claims.Subject); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to withdraw request: "+err.Error())
		return
	}

	// Notify requester (cart.updated)
	payload := fmt.Sprintf(`{"cart_id":"%s","status":"CANCELLED"}`, cartID)
	_ = s.Repo.CreateSSENotification(&models.SSENotification{
		ID: uuid.New().String(), UserID: claims.Subject,
		EventType: "cart.updated", Payload: payload,
	})
	s.SSEBroker.Publish(claims.Subject, "cart.updated", payload)

	// Notify approvers so their inbox count updates
	notifiedUsers := make(map[string]bool)
	for _, step := range pendingSteps {
		var targets []string
		if step.AssignedToUserID != nil {
			targets = []string{*step.AssignedToUserID}
		} else if step.AssignedToRole != nil {
			targets = s.fetchRoleMembers(*step.AssignedToRole)
		}
		for _, uid := range targets {
			if notifiedUsers[uid] {
				continue
			}
			notifiedUsers[uid] = true
			s.SSEBroker.Publish(uid, "inbox.new", `{"action":"withdrawn"}`)
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Request withdrawn successfully.",
	})
}

// BumpCartHandler POST /api/v1/access/carts/{cart_id}/bump
func (s *HandlerServer) BumpCartHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	cartID := r.PathValue("cart_id")

	cart, err := s.Repo.GetCart(cartID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "cart not found")
		return
	}
	if cart.RequesterID != claims.Subject {
		s.respondWithError(w, http.StatusForbidden, "not your cart")
		return
	}
	if cart.Status != "SUBMITTED" && cart.Status != "IN_PROGRESS" {
		s.respondWithError(w, http.StatusBadRequest, "only submitted or in-progress requests can be bumped")
		return
	}

	steps, err := s.Repo.GetPendingStepsForCart(cartID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to query pending steps: "+err.Error())
		return
	}

	if len(steps) == 0 {
		s.respondWithError(w, http.StatusBadRequest, "no pending approval steps found to bump")
		return
	}

	// Audit: cart bumped
	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "CART", EntityID: cartID,
		ActorUserID: claims.Subject, Action: "BUMPED",
	})

	// Notify approvers via SSE with inbox.bumped
	notifiedUsers := make(map[string]bool)
	for _, step := range steps {
		var targets []string
		if step.AssignedToUserID != nil {
			targets = []string{*step.AssignedToUserID}
		} else if step.AssignedToRole != nil {
			targets = s.fetchRoleMembers(*step.AssignedToRole)
		}

		for _, uid := range targets {
			if notifiedUsers[uid] {
				continue
			}
			notifiedUsers[uid] = true

			payload := fmt.Sprintf(`{"step_id":"%s","role":"%s","cart_id":"%s"}`,
				step.StepID, step.RoleName, cartID)
			_ = s.Repo.CreateSSENotification(&models.SSENotification{
				ID: uuid.New().String(), UserID: uid,
				EventType: "inbox.bumped", Payload: payload,
			})
			s.SSEBroker.Publish(uid, "inbox.bumped", payload)
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Request bumped successfully. Approvers have been notified.",
	})
}

// AdminListCartsHandler GET /api/v1/admin/carts
func (s *HandlerServer) AdminListCartsHandler(w http.ResponseWriter, r *http.Request) {
	carts, err := s.Repo.ListAllCarts()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to fetch carts: "+err.Error())
		return
	}
	if carts == nil {
		carts = []models.AccessCart{}
	}
	s.respondWithJSON(w, http.StatusOK, carts)
}

// AdminHousekeepingArchiveHandler POST /api/v1/admin/housekeeping/archive
func (s *HandlerServer) AdminHousekeepingArchiveHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OlderThanDays int      `json:"older_than_days"`
		Status        string   `json:"status"`
		IDs           []string `json:"ids"`
	}
	_ = s.decodeJSON(r, &body) // Ignore error, body is optional

	if body.Status == "" {
		body.Status = "ALL"
	}

	rows, err := s.Repo.ArchiveSelectedCarts(body.Status, body.OlderThanDays, body.IDs)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to archive carts: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"archived_count": rows,
		"message":        fmt.Sprintf("Housekeeping complete. Archived %d requests.", rows),
	})
}

// AdminHousekeepingDeleteHandler POST /api/v1/admin/housekeeping/delete
func (s *HandlerServer) AdminHousekeepingDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OlderThanDays int      `json:"older_than_days"`
		Status        string   `json:"status"`
		IncludeDrafts bool     `json:"include_drafts"`
		IDs           []string `json:"ids"`
	}
	_ = s.decodeJSON(r, &body) // Ignore error, body is optional

	if body.Status == "" {
		body.Status = "ALL"
	}

	rows, err := s.Repo.DeleteSelectedCarts(body.Status, body.OlderThanDays, body.IncludeDrafts, body.IDs)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delete carts: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"deleted_count": rows,
		"message":       fmt.Sprintf("Housekeeping complete. Deleted %d requests.", rows),
	})
}

// DeleteCartHandler DELETE /api/v1/access/carts/{cart_id}
func (s *HandlerServer) DeleteCartHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	cartID := r.PathValue("cart_id")

	cart, err := s.Repo.GetCart(cartID)
	if err != nil {
		s.respondWithError(w, http.StatusNotFound, "cart not found")
		return
	}
	if cart.RequesterID != claims.Subject {
		s.respondWithError(w, http.StatusForbidden, "not your cart")
		return
	}

	// Get pending steps before deletion to notify approvers to refresh their inbox
	pendingSteps, _ := s.Repo.GetPendingStepsForCart(cartID)

	_, err = s.Repo.DeleteSelectedCarts("ALL", 0, true, []string{cartID})
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to delete cart: "+err.Error())
		return
	}

	// Notify approvers so their inbox count updates
	notifiedUsers := make(map[string]bool)
	for _, step := range pendingSteps {
		var targets []string
		if step.AssignedToUserID != nil {
			targets = []string{*step.AssignedToUserID}
		} else if step.AssignedToRole != nil {
			targets = s.fetchRoleMembers(*step.AssignedToRole)
		}
		for _, uid := range targets {
			if notifiedUsers[uid] {
				continue
			}
			notifiedUsers[uid] = true
			s.SSEBroker.Publish(uid, "inbox.new", `{"action":"deleted"}`)
		}
	}

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Request deleted successfully.",
	})
}
