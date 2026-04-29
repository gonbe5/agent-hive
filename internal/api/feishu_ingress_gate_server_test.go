package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/skills"
)

type feishuGateTestPlugin struct {
	calls int
}

func (p *feishuGateTestPlugin) Platform() channel.Platform {
	return channel.PlatformFeishu
}

func (p *feishuGateTestPlugin) Send(context.Context, channel.OutboundMessage) error {
	return nil
}

func (p *feishuGateTestPlugin) WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.calls++
		w.WriteHeader(http.StatusNoContent)
	}
}

func (p *feishuGateTestPlugin) Verify(*http.Request) bool {
	return true
}

type feishuGateTestProcessor struct{}

func (p *feishuGateTestProcessor) ProcessMessage(context.Context, string, string) (master.TaskResponse, error) {
	return master.TaskResponse{}, nil
}

func TestServerFeishuRouteUsesCommittedIngressModeState(t *testing.T) {
	logger := zap.NewNop()
	skillReg := skills.NewOverlayRegistry(logger)

	fullCfg := config.Default()
	fullCfg.Channel.Feishu.IngressMode = config.FeishuIngressModeWebhook

	srv := NewServer(
		config.ServerConfig{Port: 0},
		config.HITLConfig{},
		config.WebUIConfig{},
		nil,
		skillReg,
		fullCfg,
		"",
		nil,
		nil,
		nil,
		logger,
	)

	router := channel.NewRouter(&feishuGateTestProcessor{}, logger)
	plugin := &feishuGateTestPlugin{}
	router.RegisterPlugin(plugin)
	srv.SetChannelRouter(router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channel/feishu/webhook", nil)

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected initial forward 204, got %d", rec.Code)
	}
	if plugin.calls != 1 {
		t.Fatalf("expected 1 forward call, got %d", plugin.calls)
	}

	srv.configMu.Lock()
	srv.config.Channel.Feishu.IngressMode = config.FeishuIngressModeLongconn
	srv.configMu.Unlock()

	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected runtime state to ignore direct config mutation, got %d", rec.Code)
	}
	if plugin.calls != 2 {
		t.Fatalf("expected 2 forward calls after direct config mutation, got %d", plugin.calls)
	}

	srv.SetFeishuIngressMode(config.FeishuIngressModeLongconn)
	srv.SetFeishuWebhookGateMode(config.FeishuIngressModeLongconn)

	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected committed longconn mode to reject with 404, got %d", rec.Code)
	}
	if plugin.calls != 2 {
		t.Fatalf("expected no additional forward calls after committed mode switch, got %d", plugin.calls)
	}
}
