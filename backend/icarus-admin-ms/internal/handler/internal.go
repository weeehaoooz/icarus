package handler

import (
	"encoding/json"
	"net/http"
)

// InternalListRolesHandler GET /internal/roles
// Returns all roles known to the platform — used by workflow-ms for catalogue display.
func (s *HandlerServer) InternalListRolesHandler(w http.ResponseWriter, r *http.Request) {
	roles, err := s.Repo.ListRoles()
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(roles)
}

// InternalListRoleMembersHandler GET /internal/roles/{role_name}/members
// Returns user IDs of all users holding the named role — queries auth-ms directly.
func (s *HandlerServer) InternalListRoleMembersHandler(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("role_name")
	if roleName == "" {
		s.respondWithError(w, http.StatusBadRequest, "role_name is required")
		return
	}

	resp, err := http.Get(s.AuthMSURL + "/internal/roles/" + roleName + "/members")
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to call auth-ms: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.respondWithError(w, resp.StatusCode, "auth-ms returned error status")
		return
	}

	var members []string
	if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to decode members: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(members)
}
