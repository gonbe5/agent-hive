package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// handleListPrompts GET /api/v1/admin/prompts
func (s *Server) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	if s.promptStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "prompt store not available", Code: errs.CodeInternal})
		return
	}
	page, size := parsePagination(r)
	records, total, err := s.promptStore.List(r.Context(), page, size)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": records,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// handleGetPrompt GET /api/v1/admin/prompts/{key}
func (s *Server) handleGetPrompt(w http.ResponseWriter, r *http.Request) {
	if s.promptStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "prompt store not available", Code: errs.CodeInternal})
		return
	}
	key := r.PathValue("key")
	language := r.URL.Query().Get("language")

	content, found, err := s.promptStore.Get(r.Context(), key, language)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "prompt not found", Code: errs.CodeNotFound})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key":      key,
		"language": language,
		"content":  content,
	})
}

// handleUpsertPrompt PUT /api/v1/admin/prompts/{key}
func (s *Server) handleUpsertPrompt(w http.ResponseWriter, r *http.Request) {
	if s.promptStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "prompt store not available", Code: errs.CodeInternal})
		return
	}
	key := r.PathValue("key")
	// key 中可能包含 / 分隔符（如 system/base），PathValue 会自动处理

	var body struct {
		Language  string `json:"language"`
		Content   string `json:"content"`
		UpdatedBy string `json:"updated_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: errs.CodeInvalidInput})
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "content is required", Code: errs.CodeInvalidInput})
		return
	}

	updatedBy := body.UpdatedBy
	if updatedBy == "" {
		updatedBy = "admin"
	}

	if err := s.promptStore.Upsert(r.Context(), key, body.Language, body.Content, updatedBy); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}

	// PromptStore.Upsert 内部已触发跨实例缓存失效回调，无需额外处理
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDeletePrompt DELETE /api/v1/admin/prompts/{key}
func (s *Server) handleDeletePrompt(w http.ResponseWriter, r *http.Request) {
	if s.promptStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "prompt store not available", Code: errs.CodeInternal})
		return
	}
	key := r.PathValue("key")
	language := r.URL.Query().Get("language")

	if err := s.promptStore.Delete(r.Context(), key, language); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}

	// PromptStore.Delete 内部已触发跨实例缓存失效回调，无需额外处理
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
