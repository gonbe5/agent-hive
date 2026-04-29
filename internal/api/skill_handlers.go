package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
)

// skillStoreInterface Skill CRUD 存储接口（避免循环依赖）。
// Get/Upsert/Delete 第二参数 userID：空 "" 表示 public 层（admin 默认），非空表示 personal 层（按租户隔离）。
type skillStoreInterface interface {
	Get(ctx context.Context, name, userID string) (*store.SkillRecord, bool, error)
	Upsert(ctx context.Context, name, userID, content, level, path, updatedBy string, expectRevision int) error
	Delete(ctx context.Context, name, userID string) error
	List(ctx context.Context, page, size int) ([]store.SkillRecord, int, error)
}

// handleListAdminSkills GET /api/v1/admin/skills
// 返回合并视图（FS + DB），带来源标记
func (s *Server) handleListAdminSkills(w http.ResponseWriter, r *http.Request) {
	if s.skillRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "skill registry not available", Code: errs.CodeInternal})
		return
	}
	items := s.skillRegistry.ListOverlayItems()
	type itemDTO struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Path        string `json:"path"`
		Origin      string `json:"origin"`
		Revision    int    `json:"revision"`
	}
	result := make([]itemDTO, 0, len(items))
	for _, it := range items {
		result = append(result, itemDTO{
			Name:        it.Skill.Metadata.Name,
			Description: it.Skill.Metadata.Description,
			Path:        it.Skill.Path,
			Origin:      string(it.Origin),
			Revision:    it.Revision,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result, "total": len(result)})
}

// handleGetAdminSkill GET /api/v1/admin/skills/{name}
func (s *Server) handleGetAdminSkill(w http.ResponseWriter, r *http.Request) {
	if s.skillRegistry == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "skill registry not available", Code: errs.CodeInternal})
		return
	}
	name := r.PathValue("name")
	skill, origin, revision, err := s.skillRegistry.GetWithOrigin(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "skill not found", Code: errs.CodeNotFound})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":        skill.Metadata.Name,
		"description": skill.Metadata.Description,
		"content":     skill.Content,
		"path":        skill.Path,
		"origin":      string(origin),
		"revision":    revision,
	})
}

// handleUpsertAdminSkill PUT /api/v1/admin/skills/{name}
func (s *Server) handleUpsertAdminSkill(w http.ResponseWriter, r *http.Request) {
	if s.skillStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "skill store not available", Code: errs.CodeInternal})
		return
	}
	name := r.PathValue("name")

	var body struct {
		Content        string `json:"content"`
		Level          string `json:"level"`
		Path           string `json:"path"`
		UpdatedBy      string `json:"updated_by"`
		ExpectRevision int    `json:"expect_revision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: errs.CodeInvalidInput})
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "content is required", Code: errs.CodeInvalidInput})
		return
	}
	if err := skills.ValidateTemplateSkill(body.Content); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error(), Code: errs.CodeInvalidInput})
		return
	}

	level := body.Level
	if level == "" {
		level = "user"
	}
	updatedBy := body.UpdatedBy
	if updatedBy == "" {
		updatedBy = "admin"
	}

	// admin 管道默认写 public 层（user_id=""）。personal skill 由用户侧接口写。
	if err := s.skillStore.Upsert(r.Context(), name, "", body.Content, level, body.Path, updatedBy, body.ExpectRevision); err != nil {
		if err == store.ErrSkillConflict {
			writeJSON(w, http.StatusPreconditionFailed, ErrorResponse{Error: "revision conflict", Code: errs.CodeFailedPrecondition})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDeleteAdminSkill DELETE /api/v1/admin/skills/{name}
// 删除 DB 覆盖，恢复到 FS 默认值（若 FS 中有该 skill）
func (s *Server) handleDeleteAdminSkill(w http.ResponseWriter, r *http.Request) {
	if s.skillStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "skill store not available", Code: errs.CodeInternal})
		return
	}
	name := r.PathValue("name")
	// admin 管道默认删 public 层（user_id=""）。
	if err := s.skillStore.Delete(r.Context(), name, ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleListDBSkills GET /api/v1/admin/skills/db
// 仅返回 DB 层 skill（分页）
func (s *Server) handleListDBSkills(w http.ResponseWriter, r *http.Request) {
	if s.skillStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "skill store not available", Code: errs.CodeInternal})
		return
	}
	page, size := parsePagination(r)
	records, total, err := s.skillStore.List(r.Context(), page, size)
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
