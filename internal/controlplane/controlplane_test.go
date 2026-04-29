package controlplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestControlPlane_New(t *testing.T) {
	// 这里只测试创建成功，不依赖真实 Master
	cfg := Config{
		MaxSessions:  10,
		RateLimit:    5,
		RateBurst:    10,
		BindingsFile: "",
	}

	cp, err := New(nil, cfg, zap.NewNop())
	require.NoError(t, err)
	assert.NotNil(t, cp)
	assert.Equal(t, 0, cp.ActiveSessions())
}

func TestControlPlane_BindUnbind(t *testing.T) {
	cfg := Config{MaxSessions: 10}
	cp, err := New(nil, cfg, zap.NewNop())
	require.NoError(t, err)

	// 绑定
	err = cp.Bind("dingtalk", "chat1", "session-1")
	assert.NoError(t, err)

	// 查询
	sid := cp.bindings.Lookup("dingtalk", "chat1")
	assert.Equal(t, "session-1", sid)

	// 解绑
	cp.Unbind("dingtalk", "chat1")
	sid = cp.bindings.Lookup("dingtalk", "chat1")
	assert.Empty(t, sid)
}
