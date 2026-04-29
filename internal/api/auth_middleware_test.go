package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/config"
)

// newTestServerNoAuth 创建无 auth 的测试服务器
func newTestServerNoAuth(t *testing.T) *Server {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	return &Server{
		logger:     logger,
		config:     config.Default(),
		authEngine: nil,
	}
}

// newTestServerWithAuth 创建带 auth engine 的测试服务器
func newTestServerWithAuth(t *testing.T) *Server {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	jwt := auth.NewJWTManager("test-secret", time.Hour, 24*time.Hour)
	engine := auth.NewEngine(nil, jwt, logger)
	return &Server{
		logger:     logger,
		config:     config.Default(),
		authEngine: engine,
	}
}

// --- handleAuthStatus tests ---

func TestHandleAuthStatus_AuthDisabled(t *testing.T) {
	srv := newTestServerNoAuth(t)
	req := httptest.NewRequest("GET", "/api/v1/auth/status", nil)
	rec := httptest.NewRecorder()

	srv.handleAuthStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("expected enabled:false, got %s", rec.Body.String())
	}
}

func TestHandleAuthStatus_AuthEnabled(t *testing.T) {
	srv := newTestServerWithAuth(t)
	req := httptest.NewRequest("GET", "/api/v1/auth/status", nil)
	rec := httptest.NewRecorder()

	srv.handleAuthStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Fatalf("expected enabled:true, got %s", rec.Body.String())
	}
}

// --- securityHeadersMiddleware tests ---

func TestSecurityHeadersMiddleware(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	srv := &Server{logger: logger}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := srv.securityHeadersMiddleware(next)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	}
	for _, tt := range tests {
		got := rec.Header().Get(tt.header)
		if got != tt.want {
			t.Errorf("header %s: got %q, want %q", tt.header, got, tt.want)
		}
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header missing")
	}
	// 验证 connect-src 已收紧（不含 wss: ws: 通配符）
	if strings.Contains(csp, "wss:") || strings.Contains(csp, " ws:") {
		t.Errorf("CSP connect-src should not contain wss:/ws: wildcards, got: %s", csp)
	}
}
