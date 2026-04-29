package api

import (
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// handleStarSession 处理 PATCH /api/v1/sessions/{id}/star
func (s *Server) handleStarSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	record, err := s.master.GetSessionByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "会话未找到", Code: errs.CodeTaskNotFound})
		return
	}
	if record == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "会话未找到", Code: errs.CodeTaskNotFound})
		return
	}
	if !s.checkSessionOwnership(w, r, record) {
		return
	}

	var body struct {
		Starred bool `json:"starred"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "无效的请求体", Code: errs.CodeBadRequest})
		return
	}

	user := auth.UserFrom(r.Context())
	// H3 fix: auth 关闭时无 user，fallback 到空字符串（全局收藏，不区分用户）
	userID := ""
	if user != nil {
		userID = user.ID
	}
	if err := s.master.UpdateSessionStar(r.Context(), userID, id, body.Starred); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleUpdateTags 处理 PATCH /api/v1/sessions/{id}/tags
func (s *Server) handleUpdateTags(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	record, err := s.master.GetSessionByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "会话未找到", Code: errs.CodeTaskNotFound})
		return
	}
	if record == nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "会话未找到", Code: errs.CodeTaskNotFound})
		return
	}
	if !s.checkSessionOwnership(w, r, record) {
		return
	}

	var body struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "无效的请求体", Code: errs.CodeBadRequest})
		return
	}

	if len(body.Tags) > 10 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "标签数量不能超过 10 个", Code: errs.CodeInvalidInput})
		return
	}
	for _, tag := range body.Tags {
		if utf8.RuneCountInString(tag) > 50 {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "单个标签长度不能超过 50 字符", Code: errs.CodeInvalidInput})
			return
		}
	}

	if err := s.master.UpdateSessionTags(r.Context(), id, body.Tags); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
