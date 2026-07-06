package handler

import (
	"icarus-workflow-ms/internal/models"
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
		list = []models.WorkflowDefinition{}
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
		history = []models.WorkflowDefinition{}
	}
	s.respondWithJSON(w, http.StatusOK, history)
}

// GetRoleWorkflowHandler GET /api/v1/workflow/definitions/roles/{role_id}
func (s *HandlerServer) GetRoleWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	roleID := r.PathValue("role_id")
	def, err := s.Repo.GetWorkflowDefinitionByRoleID(roleID)
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
	if req.Name == "" || len(req.Stages) == 0 {
		s.respondWithError(w, http.StatusBadRequest, "name and at least one stage are required")
		return
	}
	for _, stage := range req.Stages {
		if len(stage.Approvers) == 0 {
			s.respondWithError(w, http.StatusBadRequest, "each stage must have at least one approver")
			return
		}
		if stage.ApprovalQuorum < 1 {
			s.respondWithError(w, http.StatusBadRequest, "approval_quorum must be >= 1")
			return
		}
		for _, ap := range stage.Approvers {
			if ap.ResolverType != "USER" && ap.ResolverType != "ROLE_QUEUE" && ap.ResolverType != "EXPRESSION" {
				s.respondWithError(w, http.StatusBadRequest,
					"invalid resolver_type: must be USER, ROLE_QUEUE, or EXPRESSION")
				return
			}
		}
	}

	// Derive a stable definition_key from the role ID and name
	definitionKey := strings.ToLower(strings.ReplaceAll(roleID+"-"+req.Name, " ", "-"))
	newID := uuid.New().String()

	def, err := s.Repo.UpsertWorkflowDefinition(roleID, definitionKey, newID, claims.Subject, req)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to upsert workflow definition: "+err.Error())
		return
	}

	_ = s.Repo.WriteAuditLog(&models.AuditLog{
		ID: uuid.New().String(), EntityType: "WORKFLOW_DEFINITION", EntityID: def.ID,
		ActorUserID: claims.Subject, Action: "UPSERTED",
		AfterState: `{"version":` + itoa(def.Version) + `}`,
	})

	s.respondWithJSON(w, http.StatusOK, def)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
