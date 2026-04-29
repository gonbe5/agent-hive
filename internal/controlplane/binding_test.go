package controlplane

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBindingStore_BasicOps(t *testing.T) {
	bs, err := NewBindingStore("", zap.NewNop())
	require.NoError(t, err)

	// 绑定
	err = bs.Bind("dingtalk", "chat1", "session-abc")
	assert.NoError(t, err)

	// 查找
	sid := bs.Lookup("dingtalk", "chat1")
	assert.Equal(t, "session-abc", sid)

	// 列表
	bindings := bs.ListBindings()
	assert.Len(t, bindings, 1)

	// 解绑
	bs.Unbind("dingtalk", "chat1")
	sid = bs.Lookup("dingtalk", "chat1")
	assert.Empty(t, sid)
}

func TestBindingStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "bindings.json")

	// 创建并写入
	bs1, err := NewBindingStore(filePath, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, bs1.Bind("feishu", "chat2", "session-xyz"))

	// 验证文件存在
	_, err = os.Stat(filePath)
	assert.NoError(t, err)

	// 重新加载
	bs2, err := NewBindingStore(filePath, zap.NewNop())
	require.NoError(t, err)
	sid := bs2.Lookup("feishu", "chat2")
	assert.Equal(t, "session-xyz", sid)
}
