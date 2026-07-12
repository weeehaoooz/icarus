package handler

import (
	"fmt"
	"icarus-workflow-ms/internal/models"
	"icarus-workflow-ms/internal/repository"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// ListWorkflowDefinitionsHandler GET /api/v1/workflow/definitions
func (s *HandlerServer) ListWorkflowDefinitionsHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Repo.ListWorkflowDefinitions()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []models.Workflow{}
	}
	s.respondWithJSON(w, http.StatusOK, list)
}

// ListWorkflowsWithRoleMappingHandler GET /api/v1/workflow/definitions/templates
// Returns all current workflow templates annotated with which role (if any) is mapped to each.
func (s *HandlerServer) ListWorkflowsWithRoleMappingHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Repo.ListWorkflowsWithRoleMapping()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []models.WorkflowWithRoleMapping{}
	}
	s.respondWithJSON(w, http.StatusOK, list)
}

// GetDefinitionHistoryHandler GET /api/v1/workflow/definitions/{definition_key}/history
func (s *HandlerServer) GetDefinitionHistoryHandler(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("definition_key")
	history, err := s.Repo.GetDefinitionHistory(key)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if history == nil {
		history = []models.Workflow{}
	}
	s.respondWithJSON(w, http.StatusOK, history)
}

// GetRoleWorkflowHandler GET /api/v1/workflow/definitions/roles/{role_id}
func (s *HandlerServer) GetRoleWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("role_id")
	def, err := s.Repo.GetWorkflowByRoleID(roleID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if def == nil {
		s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"role_id":            roleID,
			"workflow_definition": nil,
			"auto_approve":       true,
		})
		return
	}
	s.respondWithJSON(w, http.StatusOK, def)
}

// CreateWorkflowTemplateHandler POST /api/v1/workflow/definitions
// Creates a standalone, reusable workflow template not yet bound to any role.
func (s *HandlerServer) CreateWorkflowTemplateHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)

	var req models.UpsertWorkflowRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || len(req.Nodes) == 0 {
		s.respondWithError(w, http.StatusBadRequest, "name and at least one node are required")
		return
	}

	if err := repository.ValidateWorkflowGraph(req.Nodes); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid workflow graph: "+err.Error())
		return
	}

	def, err := s.Repo.CreateStandaloneWorkflow(claims.Subject, req)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to create workflow template: "+err.Error())
		return
	}

	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "WORKFLOW_DEFINITION", EntityID: def.ID,
		ActorUserID: claims.Subject, Action: "CREATED",
		AfterState: `{"version":` + fmt.Sprintf("%d", def.Version) + `}`,
	})

	s.respondWithJSON(w, http.StatusCreated, def)
}

// MapWorkflowToRoleHandler PUT /api/v1/workflow/definitions/roles/{role_id}/map
// Binds an existing workflow template (by ID) to a role without modifying the template.
func (s *HandlerServer) MapWorkflowToRoleHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	roleID := r.PathValue("role_id")

	var req models.MapWorkflowToRoleRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorkflowID == "" {
		s.respondWithError(w, http.StatusBadRequest, "workflow_id is required")
		return
	}

	// Verify the workflow exists
	wf, err := s.Repo.GetWorkflowByID(req.WorkflowID)
	if err != nil || wf == nil {
		s.respondWithError(w, http.StatusNotFound, "workflow template not found")
		return
	}

	if err := s.Repo.MapWorkflowToRole(roleID, req.WorkflowID, claims.Subject); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to map workflow to role: "+err.Error())
		return
	}

	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "ROLE_WORKFLOW_MAPPING", EntityID: roleID,
		ActorUserID: claims.Subject, Action: "MAPPED",
		AfterState: `{"workflow_id":"` + req.WorkflowID + `"}`,
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"role_id":     roleID,
		"workflow_id": req.WorkflowID,
		"status":      "mapped",
	})
}

// UpsertRoleWorkflowHandler PUT /api/v1/workflow/definitions/roles/{role_id}
// Creates a new immutable version of the workflow definition and updates the role mapping.
func (s *HandlerServer) UpsertRoleWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	roleID := r.PathValue("role_id")

	var req models.UpsertWorkflowRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || len(req.Nodes) == 0 {
		s.respondWithError(w, http.StatusBadRequest, "name and at least one node are required")
		return
	}

	// Run full DAG cycle detection and node validation
	if err := repository.ValidateWorkflowGraph(req.Nodes); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid workflow graph: "+err.Error())
		return
	}

	// Derive a stable definition_key from the role ID and name
	definitionKey := strings.ToLower(strings.ReplaceAll(roleID+"-"+req.Name, " ", "-"))
	newID := uuid.New().String()

	def, err := s.Repo.UpsertWorkflow(roleID, definitionKey, newID, claims.Subject, req)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to upsert workflow: "+err.Error())
		return
	}

	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "WORKFLOW_DEFINITION", EntityID: def.ID,
		ActorUserID: claims.Subject, Action: "UPSERTED",
		AfterState: `{"version":` + fmt.Sprintf("%d", def.Version) + `}`,
	})

	s.respondWithJSON(w, http.StatusOK, def)
}

// GetWorkflowByIDHandler GET /api/v1/workflow/definitions/{id}
func (s *HandlerServer) GetWorkflowByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	def, err := s.Repo.GetWorkflowByID(id)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if def == nil {
		s.respondWithError(w, http.StatusNotFound, "workflow definition not found")
		return
	}
	s.respondWithJSON(w, http.StatusOK, def)
}

// UnmapWorkflowFromRoleHandler DELETE /api/v1/workflow/definitions/roles/{role_id}/map
// Deactivates the active role→workflow mapping without deleting it.
func (s *HandlerServer) UnmapWorkflowFromRoleHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	roleID := r.PathValue("role_id")
	if roleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "role_id is required")
		return
	}

	if err := s.Repo.UnmapWorkflowFromRole(roleID); err != nil {
		s.respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "ROLE_WORKFLOW_MAPPING", EntityID: roleID,
		ActorUserID: claims.Subject, Action: "UNMAPPED",
		AfterState: `{"is_active":false}`,
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"role_id": roleID,
		"status":  "unmapped",
	})
}

// MapRoleToWorkflowHandler PUT /api/v1/workflow/definitions/{id}/roles
// Maps a role to a specific workflow template by workflow ID.
func (s *HandlerServer) MapRoleToWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	workflowID := r.PathValue("id")

	var req models.MapWorkflowToRoleRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RoleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "role_id is required")
		return
	}

	// Verify workflow exists
	wf, err := s.Repo.GetWorkflowByID(workflowID)
	if err != nil || wf == nil {
		s.respondWithError(w, http.StatusNotFound, "workflow template not found")
		return
	}

	if err := s.Repo.MapWorkflowToRole(req.RoleID, workflowID, claims.Subject); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to map role to workflow: "+err.Error())
		return
	}

	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "ROLE_WORKFLOW_MAPPING", EntityID: workflowID,
		ActorUserID: claims.Subject, Action: "MAPPED",
		AfterState: `{"role_id":"` + req.RoleID + `"}`,
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"workflow_id": workflowID,
		"role_id":     req.RoleID,
		"status":      "mapped",
	})
}

// UnmapRoleFromWorkflowHandler DELETE /api/v1/workflow/definitions/{id}/roles/{role_id}
// Deactivates a specific role mapping from a workflow template.
func (s *HandlerServer) UnmapRoleFromWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	workflowID := r.PathValue("id")
	roleID := r.PathValue("role_id")

	if workflowID == "" || roleID == "" {
		s.respondWithError(w, http.StatusBadRequest, "workflow id and role_id are required")
		return
	}

	if err := s.Repo.UnmapRoleFromWorkflow(workflowID, roleID); err != nil {
		s.respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "ROLE_WORKFLOW_MAPPING", EntityID: workflowID,
		ActorUserID: claims.Subject, Action: "UNMAPPED",
		AfterState: `{"role_id":"` + roleID + `","is_active":false}`,
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"workflow_id": workflowID,
		"role_id":     roleID,
		"status":      "unmapped",
	})
}

