package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// newTestServerForStarTags 创建用于 star/tags 端点测试的服务器
func newTestServerForStarTags(t *testing.T) (http.Handler, *master.Master, func()) {
	t.Helper()

	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewOverlayRegistry(logger)
	agentReg := subagent.NewRegistry(logger)

	st := store.NewMemoryStore()

	m := master.NewMaster(
		master.Config{Model: "test"},
		config.HITLConfig{},
		agentReg,
		skillReg.Registry,
		st,
		logger,
	)

	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)

	sessionDone := make(chan struct{})
	go func() {
		defer close(sessionDone)
		if err := m.SessionLoop(ctx); err != nil && err != context.Canceled {
			logger.Error("session loop error", zap.Error(err))
		}
	}()

	time.Sleep(50 * time.Millisecond)

	srv := NewServer(
		config.ServerConfig{Port: 0},
		config.HITLConfig{},
		config.WebUIConfig{},
		m,
		skillReg,
		config.Default(),
		"",
		nil,
		nil,
		nil,
		logger,
	)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	return mux, m, func() {
		cancel()
		select {
		case <-sessionDone:
		case <-time.After(5 * time.Second):
		}
		m.Stop()
	}
}

// createTestSession 创建一个测试会话并返回其 ID
func createTestSession(t *testing.T, handler http.Handler) string {
	t.Helper()
	reqBody := `{"name":"test-session"}`
	req := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建会话失败: %d %s", rec.Code, rec.Body.String())
	}
	var resp CreateSessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("解析创建会话响应失败: %v", err)
	}
	return resp.SessionID
}

// withUser 将用户注入请求 context
func withUser(req *http.Request, userID string) *http.Request {
	user := &auth.User{ID: userID, Role: "user"}
	ctx := auth.WithUser(req.Context(), user)
	return req.WithContext(ctx)
}

// --- Star 测试 ---

func TestHandleStarSession_OK(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	sessionID := createTestSession(t, handler)

	body, _ := json.Marshal(map[string]bool{"starred": true})
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID+"/star", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUser(req, "user-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStarSession_Unstar(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	sessionID := createTestSession(t, handler)

	// 先收藏
	body, _ := json.Marshal(map[string]bool{"starred": true})
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID+"/star", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUser(req, "user-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("收藏失败: %d", rec.Code)
	}

	// 再取消收藏
	body, _ = json.Marshal(map[string]bool{"starred": false})
	req = httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID+"/star", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUser(req, "user-1")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("取消收藏失败: %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStarSession_NotFound(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]bool{"starred": true})
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/nonexistent-id/star", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUser(req, "user-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", rec.Code)
	}
}

func TestHandleStarSession_NoAuth_Fallback(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	sessionID := createTestSession(t, handler)

	body, _ := json.Marshal(map[string]bool{"starred": true})
	// 不注入用户：auth 关闭时应 fallback 到全局收藏，返回 200
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID+"/star", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200（auth 关闭时 fallback），得到 %d", rec.Code)
	}
}

// --- Tags 测试 ---

func TestHandleUpdateTags_OK(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	sessionID := createTestSession(t, handler)

	body, _ := json.Marshal(map[string][]string{"tags": {"go", "backend", "api"}})
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID+"/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateTags_TooMany(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	sessionID := createTestSession(t, handler)

	tags := make([]string, 11)
	for i := range tags {
		tags[i] = "tag"
	}
	body, _ := json.Marshal(map[string][]string{"tags": tags})
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID+"/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", rec.Code)
	}
}

func TestHandleUpdateTags_TooLong(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	sessionID := createTestSession(t, handler)

	// 51 个 ASCII 字符
	longTag := strings.Repeat("a", 51)
	body, _ := json.Marshal(map[string][]string{"tags": {longTag}})
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID+"/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", rec.Code)
	}
}

func TestHandleUpdateTags_ChineseTooLong(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	sessionID := createTestSession(t, handler)

	// 51 个中文字符（每个 3 bytes，但 rune 计数为 51）
	longTag := strings.Repeat("中", 51)
	body, _ := json.Marshal(map[string][]string{"tags": {longTag}})
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID+"/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 400（中文 51 字符），得到 %d", rec.Code)
	}
}

func TestHandleUpdateTags_ChineseOK(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	sessionID := createTestSession(t, handler)

	// 50 个中文字符（rune 计数 50，bytes 150）
	tag50 := strings.Repeat("中", 50)
	body, _ := json.Marshal(map[string][]string{"tags": {tag50}})
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/"+sessionID+"/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200（中文 50 字符），得到 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateTags_NotFound(t *testing.T) {
	handler, _, cleanup := newTestServerForStarTags(t)
	defer cleanup()

	body, _ := json.Marshal(map[string][]string{"tags": {"go"}})
	req := httptest.NewRequest("PATCH", "/api/v1/sessions/nonexistent-id/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", rec.Code)
	}
}
