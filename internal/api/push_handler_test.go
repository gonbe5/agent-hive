package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/channel/push"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
	"github.com/chef-guo/agents-hive/internal/subagent"
	"go.uber.org/zap"
)

func TestHandleChannelPush_DisabledReturns404(t *testing.T) {
	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, nil, nil, &config.Config{}, "", nil, nil, nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleChannelPush_EnabledSendsMessage(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &apiStubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, nil, nil, cfg, "", router, nil, nil, zap.NewNop())
	srv.SetPushService(push.NewService(router, push.Config{Enabled: true}, zap.NewNop()))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push", bytes.NewBufferString(`{"platform":"feishu","chat_id":"oc_push","msg_type":"text","content":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if len(plugin.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(plugin.sent))
	}
}

func TestHandleChannelPush_AuthEnabledRequiresAdminWriter(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &apiStubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, nil, nil, cfg, "", router, nil, nil, zap.NewNop())
	srv.SetPushService(push.NewService(router, push.Config{Enabled: true}, zap.NewNop()))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push", bytes.NewBufferString(`{"platform":"feishu","chat_id":"oc_push","msg_type":"text","content":"hello"}`))
	req = req.WithContext(auth.WithUser(auth.WithAuthEnabled(req.Context()), &auth.User{ID: "u1", Role: "user", Status: "active"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleChannelPush_AuthEnabledAllowsAdminWriter(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &apiStubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, nil, nil, cfg, "", router, nil, nil, zap.NewNop())
	srv.SetPushService(push.NewService(router, push.Config{Enabled: true}, zap.NewNop()))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push", bytes.NewBufferString(`{"platform":"feishu","chat_id":"oc_push","msg_type":"text","content":"hello"}`))
	req = req.WithContext(auth.WithUser(auth.WithAuthEnabled(req.Context()), &auth.User{ID: "u1", Role: "admin", Status: "active"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleChannelPush_AuthEnabledAllowsScopedWriter(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &apiStubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, nil, nil, cfg, "", router, nil, nil, zap.NewNop())
	srv.SetPushService(push.NewService(router, push.Config{Enabled: true}, zap.NewNop()))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push", bytes.NewBufferString(`{"platform":"feishu","chat_id":"oc_push","msg_type":"text","content":"hello"}`))
	req = req.WithContext(auth.WithClaims(
		auth.WithUser(auth.WithAuthEnabled(req.Context()), &auth.User{ID: "u1", Role: "user", Status: "active"}),
		&auth.Claims{Scopes: []string{"read", "push:write"}},
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleChannelPush_RateLimitedReturns429(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &apiStubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	svc := push.NewService(router, push.Config{Enabled: true, PerChatPerMinute: 1}, zap.NewNop())
	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, nil, nil, cfg, "", router, nil, nil, zap.NewNop())
	srv.SetPushService(svc)

	body := `{"platform":"feishu","chat_id":"oc_push","msg_type":"text","content":"hello"}`
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push", bytes.NewBufferString(body))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429, body=%s", rec2.Code, rec2.Body.String())
	}
}

func TestHandleChannelPush_SendFailureReturns502(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &apiStubPushPlugin{
		platform: channel.PlatformFeishu,
		err:      errs.New(errs.CodeChannelSendFailed, "send failed"),
	}
	router.RegisterPlugin(plugin)

	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, nil, nil, cfg, "", router, nil, nil, zap.NewNop())
	srv.SetPushService(push.NewService(router, push.Config{Enabled: true}, zap.NewNop()))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push", bytes.NewBufferString(`{"platform":"feishu","chat_id":"oc_push","msg_type":"text","content":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleChannelPushSchedules_CRUDLifecycle(t *testing.T) {
	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	appStore := store.NewMemoryStore()
	m := master.NewMaster(master.Config{}, config.HITLConfig{}, subagent.NewRegistry(zap.NewNop()), skills.NewRegistry(zap.NewNop()), appStore, zap.NewNop())
	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, m, nil, cfg, "", nil, appStore, nil, zap.NewNop())
	srv.SetPushService(push.NewService(nil, push.Config{Enabled: true}, zap.NewNop()))

	createBody := `{"name":"daily-report","platform":"feishu","interval_sec":60,"prompt":"scheduled_push:task_done:chat_id=oc_sched_1:title=日报生成完成:summary=请查收","enabled":true}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push/schedules", bytes.NewBufferString(createBody))
	createReq = createReq.WithContext(auth.WithClaims(
		auth.WithUser(auth.WithAuthEnabled(createReq.Context()), &auth.User{ID: "u1", Role: "admin", Status: "active"}),
		&auth.Claims{Scopes: []string{"push:write", "admin"}},
	))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body=%s", createRec.Code, createRec.Body.String())
	}
	if createRec.Header().Get("Deprecation") != "true" {
		t.Fatalf("create Deprecation header = %q, want true", createRec.Header().Get("Deprecation"))
	}

	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created["name"] != "daily-report" {
		t.Fatalf("created name = %v, want daily-report", created["name"])
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/channels/push/schedules", nil)
	listReq = listReq.WithContext(auth.WithClaims(
		auth.WithUser(auth.WithAuthEnabled(listReq.Context()), &auth.User{ID: "u1", Role: "admin", Status: "active"}),
		&auth.Claims{Scopes: []string{"push:write", "admin"}},
	))
	listRec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200, body=%s", listRec.Code, listRec.Body.String())
	}
	if listRec.Header().Get("Deprecation") != "true" {
		t.Fatalf("list Deprecation header = %q, want true", listRec.Header().Get("Deprecation"))
	}

	var listed []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed count = %d, want 1, body=%s", len(listed), listRec.Body.String())
	}

	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("created id empty, body=%s", createRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/push/schedules/"+id, nil)
	deleteReq.SetPathValue("id", id)
	deleteReq = deleteReq.WithContext(auth.WithClaims(
		auth.WithUser(auth.WithAuthEnabled(deleteReq.Context()), &auth.User{ID: "u1", Role: "admin", Status: "active"}),
		&auth.Claims{Scopes: []string{"push:write", "admin"}},
	))
	deleteRec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204, body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	if deleteRec.Header().Get("Deprecation") != "true" {
		t.Fatalf("delete Deprecation header = %q, want true", deleteRec.Header().Get("Deprecation"))
	}

	listAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/api/v1/channels/push/schedules", nil)
	listAfterDeleteReq = listAfterDeleteReq.WithContext(auth.WithClaims(
		auth.WithUser(auth.WithAuthEnabled(listAfterDeleteReq.Context()), &auth.User{ID: "u1", Role: "admin", Status: "active"}),
		&auth.Claims{Scopes: []string{"push:write", "admin"}},
	))
	listAfterDeleteRec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(listAfterDeleteRec, listAfterDeleteReq)

	if listAfterDeleteRec.Code != http.StatusOK {
		t.Fatalf("list after delete status = %d, want 200, body=%s", listAfterDeleteRec.Code, listAfterDeleteRec.Body.String())
	}
	listed = nil
	if err := json.Unmarshal(listAfterDeleteRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list after delete response: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed count after delete = %d, want 0, body=%s", len(listed), listAfterDeleteRec.Body.String())
	}
}

func TestHandleChannelPushSchedules_AuthEnabledRequiresWriter(t *testing.T) {
	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	appStore := store.NewMemoryStore()
	m := master.NewMaster(master.Config{}, config.HITLConfig{}, subagent.NewRegistry(zap.NewNop()), skills.NewRegistry(zap.NewNop()), appStore, zap.NewNop())
	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, m, nil, cfg, "", nil, appStore, nil, zap.NewNop())
	srv.SetPushService(push.NewService(nil, push.Config{Enabled: true}, zap.NewNop()))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push/schedules", bytes.NewBufferString(`{"name":"daily-report","platform":"feishu","interval_sec":60,"prompt":"scheduled_push:task_done:chat_id=oc_sched_1:title=日报生成完成:summary=请查收","enabled":true}`))
	req = req.WithContext(auth.WithUser(auth.WithAuthEnabled(req.Context()), &auth.User{ID: "u1", Role: "user", Status: "active"}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleChannelPushSchedules_RequiresAdmin(t *testing.T) {
	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	appStore := store.NewMemoryStore()
	m := master.NewMaster(master.Config{}, config.HITLConfig{}, subagent.NewRegistry(zap.NewNop()), skills.NewRegistry(zap.NewNop()), appStore, zap.NewNop())
	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, m, nil, cfg, "", nil, appStore, nil, zap.NewNop())
	srv.SetPushService(push.NewService(nil, push.Config{Enabled: true}, zap.NewNop()))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push/schedules", bytes.NewBufferString(`{"name":"daily-report","platform":"feishu","interval_sec":60,"prompt":"scheduled_push:task_done:chat_id=oc_sched_1:title=日报生成完成:summary=请查收","enabled":true}`))
	req = req.WithContext(auth.WithClaims(
		auth.WithUser(auth.WithAuthEnabled(req.Context()), &auth.User{ID: "u1", Role: "user", Status: "active"}),
		&auth.Claims{Scopes: []string{"push:write"}},
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleChannelPushSchedules_RejectsPromptPlatformMismatch(t *testing.T) {
	cfg := &config.Config{}
	cfg.Channel.Feishu.Enabled = true
	cfg.Channel.Feishu.Push.Enabled = true

	appStore := store.NewMemoryStore()
	m := master.NewMaster(master.Config{}, config.HITLConfig{}, subagent.NewRegistry(zap.NewNop()), skills.NewRegistry(zap.NewNop()), appStore, zap.NewNop())
	srv := NewServer(config.ServerConfig{Port: 0}, config.HITLConfig{}, config.WebUIConfig{}, m, nil, cfg, "", nil, appStore, nil, zap.NewNop())
	srv.SetPushService(push.NewService(nil, push.Config{Enabled: true}, zap.NewNop()))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/push/schedules", bytes.NewBufferString(`{"name":"daily-report","platform":"feishu","interval_sec":60,"prompt":"scheduled_push:task_done:platform=dingtalk:chat_id=oc_sched_1:title=日报生成完成:summary=请查收","enabled":true}`))
	req = req.WithContext(auth.WithClaims(
		auth.WithUser(auth.WithAuthEnabled(req.Context()), &auth.User{ID: "u1", Role: "admin", Status: "active"}),
		&auth.Claims{Scopes: []string{"push:write", "admin"}},
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Mux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

type apiStubPushPlugin struct {
	platform channel.Platform
	sent     []channel.OutboundMessage
	err      error
}

func (s *apiStubPushPlugin) Platform() channel.Platform { return s.platform }
func (s *apiStubPushPlugin) Send(_ context.Context, msg channel.OutboundMessage) error {
	s.sent = append(s.sent, msg)
	return s.err
}
func (s *apiStubPushPlugin) WebhookHandler() http.HandlerFunc { return nil }
func (s *apiStubPushPlugin) Verify(_ *http.Request) bool      { return true }
