//go:build integration
// +build integration

// 集成测试运行方法:
//
// 1. 启动 WeChatPadPro 服务（docker-compose up -d）
// 2. 设置环境变量: export WECHATPADPRO_BASE_URL=http://localhost:8848
// 3. 运行测试: go test -tags=integration ./internal/channel/wechat/wechatpadpro/... -v
//
// 注意: 集成测试需要真实的 WeChatPadPro 服务运行，且可能需要扫码登录

package wechatpadpro

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel/wechat"
	"github.com/chef-guo/agents-hive/internal/config"
)

// TestIntegration_RealConnection 集成测试（需要真实的 WeChatPadPro 服务）
func TestIntegration_RealConnection(t *testing.T) {
	baseURL := os.Getenv("WECHATPADPRO_BASE_URL")
	if baseURL == "" {
		t.Skip("跳过集成测试: WECHATPADPRO_BASE_URL 未设置")
	}

	cfg := config.WeChatPadProInstanceConfig{
		BaseURL: baseURL,
		Timeout: 10,
	}

	backend := New(cfg, zap.NewExample())

	// 设置消息处理器
	messageReceived := make(chan struct{}, 1)
	backend.SetMessageHandler(func(msg wechat.IncomingMessage) {
		t.Logf("收到消息: %+v", msg)
		select {
		case messageReceived <- struct{}{}:
		default:
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动连接（需要扫码登录）
	if err := backend.Start(ctx); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer backend.Stop()

	if !backend.IsLoggedIn() {
		t.Error("登录后 IsLoggedIn() 应该返回 true")
	}

	t.Log("✓ 集成测试通过: WeChatPadPro 连接成功")
}
