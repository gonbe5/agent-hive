package wecom

import (
	"testing"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestPluginPlatform(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.WeComConfig{}, channel.NewRouter(nil, logger), logger)
	assert.Equal(t, channel.PlatformWeCom, p.Platform())
}

func TestVerifySignature(t *testing.T) {
	// 简单验证签名函数不会 panic
	result := VerifySignature("token", "timestamp", "nonce", "encrypt", "invalid")
	assert.False(t, result)
}
