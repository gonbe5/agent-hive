package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/agentquality"
	"github.com/chef-guo/agents-hive/internal/config"
)

func TestAdminQualityCases_ReturnsFixtures(t *testing.T) {
	srv := newQualityAdminTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/quality/cases", nil)
	rec := httptest.NewRecorder()

	srv.handleAdminQualityListCases(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got struct {
		Cases    []agentquality.Case `json:"cases"`
		Total    int                 `json:"total"`
		Required int                 `json:"required"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.NotEmpty(t, got.Cases)
	require.Equal(t, len(got.Cases), got.Total)
	require.Greater(t, got.Required, 0)
	for _, c := range got.Cases {
		require.NoError(t, agentquality.ValidateCase(c), c.ID)
	}
}

func TestAdminQualityPromptSmoke_EmptyContentReturns400(t *testing.T) {
	srv := newQualityAdminTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/quality/prompt-smoke", strings.NewReader(`{
		"key": "system/main",
		"language": "zh",
		"content": "   "
	}`))
	rec := httptest.NewRecorder()

	srv.handleAdminQualityPromptSmoke(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminQualityPromptSmoke_NormalReturns200(t *testing.T) {
	srv := newQualityAdminTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/quality/prompt-smoke", strings.NewReader(`{
		"key": "system/main",
		"language": "zh",
		"content": "你可以使用工具完成任务。"
	}`))
	rec := httptest.NewRecorder()

	srv.handleAdminQualityPromptSmoke(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got struct {
		OK           bool     `json:"ok"`
		CheckedCases int      `json:"checked_cases"`
		Warnings     []string `json:"warnings"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.True(t, got.OK)
	require.Greater(t, got.CheckedCases, 0)
	require.Empty(t, got.Warnings)
}

func TestAdminQualityPromptSmoke_WarningReturns200(t *testing.T) {
	srv := newQualityAdminTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/quality/prompt-smoke", strings.NewReader(`{
		"key": "system/safety",
		"language": "zh",
		"content": "请在执行前确认目标。"
	}`))
	rec := httptest.NewRecorder()

	srv.handleAdminQualityPromptSmoke(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got struct {
		OK           bool     `json:"ok"`
		CheckedCases int      `json:"checked_cases"`
		Warnings     []string `json:"warnings"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.True(t, got.OK)
	require.Greater(t, got.CheckedCases, 0)
	require.NotEmpty(t, got.Warnings)
}

func newQualityAdminTestServer() *Server {
	return &Server{
		logger: zap.NewNop(),
		config: config.Default(),
	}
}
