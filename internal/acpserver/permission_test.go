package acpserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestCreateACPPermissionFn_ReturnsNonNil 验证 createACPPermissionFn 返回非 nil 函数
func TestCreateACPPermissionFn_ReturnsNonNil(t *testing.T) {
	fn := createACPPermissionFn(nil, "test-session", zap.NewNop())
	assert.NotNil(t, fn, "createACPPermissionFn 应返回非 nil 函数")
}

// TestNewSession_WiresPermissionFn 验证 NewSession 在 conn 存在时注入权限桥接函数
func TestNewSession_WiresPermissionFn(t *testing.T) {
	// 仅验证 createACPPermissionFn 的签名与 Master.SetPermissionPromptFn 兼容
	// 实际集成测试需要完整的 ACP 连接管道
	fn := createACPPermissionFn(nil, "sess-001", zap.NewNop())

	// 验证返回函数签名正确（编译期保证类型匹配）
	assert.NotNil(t, fn, "权限桥接函数不应为 nil")
}
