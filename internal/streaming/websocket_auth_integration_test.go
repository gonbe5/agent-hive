package streaming

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

// TestWSHandler_IntegrationAuth_InvalidToken 集成测试：无效 token 被拒绝
func TestWSHandler_IntegrationAuth_InvalidToken(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)
	st := store.NewMemoryStore()
	m := master.NewMaster(
		master.Config{Model: "test"},
		config.HITLConfig{},
		agentReg,
		skillReg,
		st,
		logger,
	)

	handler := NewWSHandler(m, logger)
	handler.SetAuthToken("valid-secret-token")

	// 创建带有无效 token 的请求
	req := httptest.NewRequest("GET", "/ws?token=wrong-token", nil)
	w := httptest.NewRecorder()

	handler.HandleConnection(w, req)

	// 应该返回 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Unauthorized") {
		t.Errorf("expected Unauthorized in response, got %s", body)
	}
}

// TestWSHandler_IntegrationAuth_MissingToken 集成测试：缺少 token 被拒绝
func TestWSHandler_IntegrationAuth_MissingToken(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)
	st := store.NewMemoryStore()
	m := master.NewMaster(
		master.Config{Model: "test"},
		config.HITLConfig{},
		agentReg,
		skillReg,
		st,
		logger,
	)

	handler := NewWSHandler(m, logger)
	handler.SetAuthToken("valid-secret-token")

	// 创建不带 token 的请求
	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()

	handler.HandleConnection(w, req)

	// 应该返回 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

// TestWSHandler_IntegrationAuth_NoTokenRequired 集成测试：未配置 token 时允许连接
func TestWSHandler_IntegrationAuth_NoTokenRequired(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)
	st := store.NewMemoryStore()
	m := master.NewMaster(
		master.Config{Model: "test"},
		config.HITLConfig{},
		agentReg,
		skillReg,
		st,
		logger,
	)

	handler := NewWSHandler(m, logger)
	// 不设置 token，应该允许任何连接

	// 创建不带 token 的请求
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	w := httptest.NewRecorder()

	// 使用超时上下文，防止测试挂起
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	handler.HandleConnection(w, req)

	// WebSocket 升级会返回 101，但由于测试环境限制可能失败
	// 重要的是不应该返回 401
	if w.Code == http.StatusUnauthorized {
		t.Errorf("should not return 401 when no token is required, got %d", w.Code)
	}
}

// TestWSHandler_IntegrationConnectionLimit 集成测试：连接数限制
func TestWSHandler_IntegrationConnectionLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)
	st := store.NewMemoryStore()
	m := master.NewMaster(
		master.Config{Model: "test"},
		config.HITLConfig{},
		agentReg,
		skillReg,
		st,
		logger,
	)

	handler := NewWSHandler(m, logger)
	handler.SetMaxConnectionsPerIP(2) // 设置最大连接数为 2

	// 模拟来自同一 IP 的连接（SplitHostPort 后只保留 IP 部分）
	testIP := "192.168.1.100"
	testAddr := "192.168.1.100:12345"

	// 手动增加连接计数来模拟已有连接
	handler.ipConnectionsMu.Lock()
	handler.ipConnections[testIP] = 2 // 已达到限制
	handler.ipConnectionsMu.Unlock()

	// 尝试第三个连接
	req := httptest.NewRequest("GET", "/ws", nil)
	req.RemoteAddr = testAddr
	w := httptest.NewRecorder()

	handler.HandleConnection(w, req)

	// 应该返回 429 Too Many Requests
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Too many connections") {
		t.Errorf("expected 'Too many connections' in response, got %s", body)
	}

	// 验证连接计数没有增加
	handler.ipConnectionsMu.Lock()
	count := handler.ipConnections[testIP]
	handler.ipConnectionsMu.Unlock()

	if count != 2 {
		t.Errorf("expected connection count to remain 2, got %d", count)
	}
}
