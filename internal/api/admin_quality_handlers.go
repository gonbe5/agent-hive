package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/chef-guo/agents-hive/internal/agentquality"
	"github.com/chef-guo/agents-hive/internal/errs"
)

type adminQualityPromptSmokeRequest struct {
	Key      string `json:"key"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

func (s *Server) handleAdminQualityListCases(w http.ResponseWriter, r *http.Request) {
	cases, required, err := loadValidatedQualityCases()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cases":    cases,
		"total":    len(cases),
		"required": required,
	})
}

func (s *Server) handleAdminQualityPromptSmoke(w http.ResponseWriter, r *http.Request) {
	var req adminQualityPromptSmokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "无效的请求体", Code: errs.CodeBadRequest})
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "content 不能为空", Code: errs.CodeInvalidInput})
		return
	}

	_, required, err := loadValidatedQualityCases()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}

	warnings := promptSmokeWarnings(req.Key, content)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"checked_cases": required,
		"warnings":      warnings,
	})
}

func loadValidatedQualityCases() ([]agentquality.Case, int, error) {
	loaded, err := agentquality.LoadCases(qualityCasesDir())
	if err != nil {
		return nil, 0, err
	}

	cases := make([]agentquality.Case, 0, len(loaded))
	required := 0
	for _, lc := range loaded {
		if err := agentquality.ValidateCase(lc.Case); err != nil {
			return nil, 0, fmt.Errorf("%s: %w", lc.Path, err)
		}
		if lc.Case.Required {
			required++
		}
		cases = append(cases, lc.Case)
	}
	return cases, required, nil
}

func qualityCasesDir() string {
	_, file, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "agentquality", "testdata"))
	}
	return filepath.Join("internal", "agentquality", "testdata")
}

func promptSmokeWarnings(key, content string) []string {
	lowerContent := strings.ToLower(content)
	warnings := []string{}

	if strings.HasPrefix(key, "system/") && !strings.Contains(content, "工具") && !strings.Contains(lowerContent, "tool") {
		warnings = append(warnings, "system prompt should mention 工具 or tool")
	}
	if key == "system/safety" && !strings.Contains(content, "安全") && !strings.Contains(lowerContent, "permission") {
		warnings = append(warnings, "system/safety prompt should mention 安全 or permission")
	}
	return warnings
}
