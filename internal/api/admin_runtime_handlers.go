package api

import (
	"net/http"

	"github.com/chef-guo/agents-hive/internal/errs"
)

func (s *Server) handleAdminRuntimePolicy(w http.ResponseWriter, r *http.Request) {
	if s.master == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "master not available", Code: errs.CodeInternal})
		return
	}
	writeJSON(w, http.StatusOK, s.master.RuntimePolicySnapshot())
}
