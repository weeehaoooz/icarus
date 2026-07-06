package handler

import "net/http"

// AuditTrailHandler GET /api/v1/workflow/instances/{instance_id}/audit-trail
func (s *HandlerServer) AuditTrailHandler(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("instance_id")
	logs, err := s.Repo.GetAuditTrail(instanceID)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"instance_id": instanceID,
		"events":      logs,
	})
}
