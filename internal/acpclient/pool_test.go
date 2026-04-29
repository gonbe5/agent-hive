package acpclient

import (
	"testing"

	"go.uber.org/zap/zaptest"
)

// TestPoolNewAndList 验证新建连接池和空列表
func TestPoolNewAndList(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewPool(logger)

	configs := pool.List()
	if len(configs) != 0 {
		t.Errorf("新建连接池应为空, got %d", len(configs))
	}
}

// TestPoolDisconnectNotExist 验证断开不存在的连接
func TestPoolDisconnectNotExist(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewPool(logger)

	err := pool.Disconnect("not-exist")
	if err == nil {
		t.Error("Disconnect 不存在的 agent 应返回错误")
	}
}

// TestPoolGetNotExist 验证获取不存在的 agent
func TestPoolGetNotExist(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewPool(logger)

	_, ok := pool.Get("not-exist")
	if ok {
		t.Error("Get 不存在的 agent 应返回 false")
	}
}

// TestPoolCloseAllEmpty 验证关闭空连接池不 panic
func TestPoolCloseAllEmpty(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewPool(logger)
	pool.CloseAll()
}

// TestPoolConnectInvalidTransport 验证连接无效传输类型
func TestPoolConnectInvalidTransport(t *testing.T) {
	logger := zaptest.NewLogger(t)
	pool := NewPool(logger)

	_, err := pool.Connect(t.Context(), RemoteAgentConfig{
		Name:      "bad",
		Transport: "invalid",
	})
	if err == nil {
		t.Error("Connect 应对无效传输类型返回错误")
	}
}
