package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
)

func TestFeishuIngressGateHandler_ForwardsInWebhookMode(t *testing.T) {
	called := false

	handler := NewFeishuIngressGateHandler(
		func() config.FeishuIngressMode { return config.FeishuIngressModeWebhook },
		func() http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
		},
		zap.NewNop(),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channel/feishu/webhook", nil)

	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected webhook handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestFeishuIngressGateHandler_RejectsInLongconnMode(t *testing.T) {
	handler := NewFeishuIngressGateHandler(
		func() config.FeishuIngressMode { return config.FeishuIngressModeLongconn },
		func() http.Handler {
			return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("must not forward in longconn mode")
			})
		},
		zap.NewNop(),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channel/feishu/webhook", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "feishu webhook ingress disabled") {
		t.Fatalf("expected stable disabled response body, got %q", body)
	}
}

func TestFeishuIngressGateHandler_RejectsWhenModeFnNil(t *testing.T) {
	handler := NewFeishuIngressGateHandler(
		nil,
		func() http.Handler {
			return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("must not forward when modeFn is nil")
			})
		},
		zap.NewNop(),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channel/feishu/webhook", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestFeishuIngressGateHandler_UnavailableHandlerDoesNotPanicWithNilLogger(t *testing.T) {
	handler := NewFeishuIngressGateHandler(
		func() config.FeishuIngressMode { return config.FeishuIngressModeWebhook },
		nil,
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channel/feishu/webhook", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "feishu webhook handler unavailable") {
		t.Fatalf("expected unavailable body, got %q", body)
	}
}

func TestFeishuIngressGateHandler_ZeroValueDoesNotPanic(t *testing.T) {
	var handler FeishuIngressGateHandler

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channel/feishu/webhook", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
