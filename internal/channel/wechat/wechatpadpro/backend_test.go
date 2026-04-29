package wechatpadpro

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel/wechat"
	"github.com/chef-guo/agents-hive/internal/config"
)

func TestBackend_Name(t *testing.T) {
	cfg := config.WeChatPadProInstanceConfig{
		BaseURL: "http://localhost:8848",
	}

	backend := New(cfg, zap.NewNop())
	if backend.Name() != "wechatpadpro" {
		t.Errorf("期望 Name() = wechatpadpro, 实际 = %s", backend.Name())
	}
}

func TestBackend_IsLoggedIn(t *testing.T) {
	cfg := config.WeChatPadProInstanceConfig{
		BaseURL: "http://localhost:8848",
	}

	backend := New(cfg, zap.NewNop())

	// 初始状态应该是未登录
	if backend.IsLoggedIn() {
		t.Error("初始状态应该是未登录")
	}
}

func TestBackend_SetMessageHandler(t *testing.T) {
	cfg := config.WeChatPadProInstanceConfig{
		BaseURL: "http://localhost:8848",
	}

	backend := New(cfg, zap.NewNop())

	backend.SetMessageHandler(func(_ wechat.IncomingMessage) {
		// 测试回调设置
	})

	// 验证 handler 已设置（通过内部状态检查）
	if backend.handler == nil {
		t.Error("MessageHandler 应该已设置")
	}
}

func TestBackend_Start_AlreadyLoggedIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/CheckLoginStatus" {
			resp := APIResponse{
				Code: 200,
				Text: "success",
				Data: map[string]any{
					"isLogin":  true,
					"wxid":     "test_wxid",
					"nickname": "测试账号",
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	cfg := config.WeChatPadProInstanceConfig{
		BaseURL: server.URL,
		Token:   "test_key",
		Timeout: 5,
	}

	backend := New(cfg, zap.NewNop())

	// Start 会尝试连接 WebSocket（httptest 不支持 WS），预期失败
	err := backend.Start(context.Background())
	if err == nil {
		t.Error("期望 WebSocket 连接失败（httptest 不支持 WS）")
	}

	// 但登录状态检查应该成功
	if backend.wxid != "test_wxid" {
		t.Errorf("期望 wxid = test_wxid, 实际 = %s", backend.wxid)
	}
}
