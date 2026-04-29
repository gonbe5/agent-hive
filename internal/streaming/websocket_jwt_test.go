package streaming

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

// mockAuthStore 实现 auth.Store 接口，用于 WS JWT 测试
type mockAuthStore struct {
	users map[string]*auth.User // key: userID
}

func newMockAuthStore() *mockAuthStore {
	return &mockAuthStore{users: make(map[string]*auth.User)}
}

func (m *mockAuthStore) ListEnabledProviders(_ context.Context) ([]auth.ProviderConfig, error) {
	return nil, nil
}
func (m *mockAuthStore) FindUserByExternalID(_ context.Context, _, _ string) (*auth.User, error) {
	return nil, nil
}
func (m *mockAuthStore) FindUserByExternalIDAndProviderType(_ context.Context, _, _ string) (*auth.User, error) {
	return nil, nil
}
func (m *mockAuthStore) GetUserByID(_ context.Context, userID string) (*auth.User, error) {
	return m.users[userID], nil
}
func (m *mockAuthStore) CreateUser(_ context.Context, user *auth.User) error {
	m.users[user.ID] = user
	return nil
}
func (m *mockAuthStore) UpdateUserProfile(_ context.Context, _ string, _ *auth.UserInfo) error {
	return nil
}
func (m *mockAuthStore) UpdateLoginInfo(_ context.Context, _, _ string) error { return nil }
func (m *mockAuthStore) RecordLogin(_ context.Context, _ *auth.LoginRecord) error { return nil }
func (m *mockAuthStore) GetUserQuota(_ context.Context, _ string) (*auth.UserQuota, error) {
	return nil, nil
}
func (m *mockAuthStore) UpsertUserQuota(_ context.Context, _ string, _ int64) error { return nil }
func (m *mockAuthStore) IncrementTokenUsage(_ context.Context, _ string, _ int64) error {
	return nil
}
func (m *mockAuthStore) ResetQuotaIfExpired(_ context.Context, _ string, _ time.Time) (*auth.UserQuota, error) {
	return nil, nil
}
func (m *mockAuthStore) ListUsers(_ context.Context, _ string, _, _ int) ([]*auth.UserWithQuota, int64, error) {
	return nil, 0, nil
}
func (m *mockAuthStore) GetUserWithQuota(_ context.Context, _ string) (*auth.UserWithQuota, error) {
	return nil, nil
}
func (m *mockAuthStore) UpdateUserRole(_ context.Context, _, _ string) error { return nil }
func (m *mockAuthStore) UpdateUserStatus(_ context.Context, _, _ string) error { return nil }
func (m *mockAuthStore) GetLoginHistory(_ context.Context, _ string, _ int) ([]*auth.LoginRecord, error) {
	return nil, nil
}
func (m *mockAuthStore) ListAllProviders(_ context.Context) ([]auth.ProviderConfig, error) {
	return nil, nil
}
func (m *mockAuthStore) CreateProvider(_ context.Context, _ auth.ProviderConfig) error { return nil }
func (m *mockAuthStore) UpsertProvider(_ context.Context, _ auth.ProviderConfig) error { return nil }
func (m *mockAuthStore) UpdateProvider(_ context.Context, _ string, _ auth.ProviderConfig) error {
	return nil
}
func (m *mockAuthStore) DeleteProvider(_ context.Context, _ string) error { return nil }
func (m *mockAuthStore) CountEnabledProviders(_ context.Context) (int, error) { return 0, nil }
func (m *mockAuthStore) CountUsers(_ context.Context) (int64, error)            { return 0, nil }
func (m *mockAuthStore) UpdateProviderFields(_ context.Context, _ string, _ auth.ProviderUpdate) error { return nil }
func (m *mockAuthStore) CountUsersByProvider(_ context.Context, _ string) (int64, error) { return 0, nil }

// newTestWSHandlerWithJWT 创建带 JWT authEngine 的 WSHandler
func newTestWSHandlerWithJWT(t *testing.T, st *mockAuthStore) (*WSHandler, *auth.JWTManager) {
	t.Helper()
	logger := zap.NewNop()
	jwt := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	engine := auth.NewEngine(st, jwt, logger)

	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)
	appStore := store.NewMemoryStore()
	m := master.NewMaster(
		master.Config{Model: "test"},
		config.HITLConfig{},
		agentReg,
		skillReg,
		appStore,
		logger,
	)

	handler := NewWSHandlerWithOptions(m, logger, true) // insecureOrigin=true for tests
	handler.SetAuthEngine(engine)
	return handler, jwt
}

// TestWSHandler_JWT_DisabledUser 验证：JWT 有效但用户已被禁用 → 拒绝连接（close 4403）
func TestWSHandler_JWT_DisabledUser(t *testing.T) {
	st := newMockAuthStore()
	st.users["user-1"] = &auth.User{
		ID:     "user-1",
		Status: "disabled",
		Role:   "user",
	}

	handler, jwt := newTestWSHandlerWithJWT(t, st)
	token, err := jwt.Issue("user-1", "user", "test")
	if err != nil {
		t.Fatalf("failed to issue JWT: %v", err)
	}

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "bearer-"+token)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.HandleConnection(w, req)

	// 应该不返回 200（连接被拒绝，通过 WebSocket close 4403 或 HTTP 401）
	if w.Code == http.StatusOK {
		t.Errorf("disabled user should not get 200, got %d", w.Code)
	}
}

// TestWSHandler_JWT_UserNotFound 验证：JWT 有效但用户不存在 → 拒绝连接（close 4401）
func TestWSHandler_JWT_UserNotFound(t *testing.T) {
	st := newMockAuthStore()
	// 不添加任何用户

	handler, jwt := newTestWSHandlerWithJWT(t, st)
	token, err := jwt.Issue("nonexistent-user", "user", "test")
	if err != nil {
		t.Fatalf("failed to issue JWT: %v", err)
	}

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "bearer-"+token)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.HandleConnection(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("nonexistent user should not get 200, got %d", w.Code)
	}
}

// TestWSHandler_JWT_InvalidToken_WithStaticTokenPassed 验证：JWT 无效但 static token 已通过 → 放行（OR 逻辑）
func TestWSHandler_JWT_InvalidToken_WithStaticTokenPassed(t *testing.T) {
	st := newMockAuthStore()
	handler, _ := newTestWSHandlerWithJWT(t, st)
	handler.SetAuthToken("valid-static-token")

	req := httptest.NewRequest("GET", "/ws", nil)
	// static token 通过 Authorization header
	req.Header.Set("Authorization", "Bearer valid-static-token")
	// 同时发送无效 JWT（模拟浏览器发送了过期 token）
	req.Header.Set("Sec-WebSocket-Protocol", "bearer-invalid-jwt-token, v1")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.HandleConnection(w, req)

	// static token 通过后，即使 JWT 无效也不应该返回 401
	if w.Code == http.StatusUnauthorized {
		t.Errorf("static token should allow connection even with invalid JWT, got 401")
	}
}

// TestWSHandler_JWT_NoCredentials 验证：authEngine 启用但无任何凭证 → 401
func TestWSHandler_JWT_NoCredentials(t *testing.T) {
	st := newMockAuthStore()
	handler, _ := newTestWSHandlerWithJWT(t, st)

	req := httptest.NewRequest("GET", "/ws", nil)
	// 不提供任何认证信息
	w := httptest.NewRecorder()

	handler.HandleConnection(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no credentials, got %d", w.Code)
	}
}
