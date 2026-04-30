package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/runtimepolicy"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

func TestAdminRuntimePolicy_NoMasterReturns503(t *testing.T) {
	srv := &Server{logger: zap.NewNop(), config: config.Default()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/runtime/policy", nil)
	rec := httptest.NewRecorder()

	srv.handleAdminRuntimePolicy(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestAdminRuntimePolicy_ReturnsSnapshot(t *testing.T) {
	policy := runtimepolicy.Policy{ToolTimeout: 75 * time.Millisecond}.WithDefaults()
	m := master.NewMaster(
		master.Config{Model: "test", RuntimePolicy: policy},
		config.HITLConfig{},
		subagent.NewRegistry(zap.NewNop()),
		skills.NewRegistry(zap.NewNop()),
		store.NewMemoryStore(),
		zap.NewNop(),
	)
	srv := &Server{master: m, logger: zap.NewNop(), config: config.Default()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/runtime/policy", nil)
	rec := httptest.NewRecorder()

	srv.handleAdminRuntimePolicy(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got runtimepolicy.Policy
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, 75*time.Millisecond, got.ToolTimeout)
	require.NotZero(t, got.GlobalWorkers)
}
