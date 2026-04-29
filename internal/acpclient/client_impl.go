package acpclient

import (
	"context"
	"fmt"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
)

// acpClientImpl 实现 acp.Client 接口，处理远程 Agent 的反向请求。
// agent-to-agent 场景下，大部分方法返回 NotSupported，
// 仅实现 SessionUpdate 接收流式进度。
type acpClientImpl struct {
	agentName string
	logger    *zap.Logger
	// onUpdate 回调，用于接收远程 Agent 的会话更新通知
	onUpdate func(acp.SessionNotification)
}

func newACPClientImpl(agentName string, logger *zap.Logger) *acpClientImpl {
	return &acpClientImpl{
		agentName: agentName,
		logger:    logger.With(zap.String("remote_agent", agentName)),
	}
}

func (c *acpClientImpl) notSupported(method string) error {
	return fmt.Errorf("远程 ACP Agent %q 反向调用 %s 不支持", c.agentName, method)
}

// ReadTextFile 远程 Agent 请求读取文件 — 不支持
func (c *acpClientImpl) ReadTextFile(_ context.Context, _ acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, c.notSupported("fs/read_text_file")
}

// WriteTextFile 远程 Agent 请求写入文件 — 不支持
func (c *acpClientImpl) WriteTextFile(_ context.Context, _ acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, c.notSupported("fs/write_text_file")
}

// RequestPermission 远程 Agent 请求权限 — 不支持
func (c *acpClientImpl) RequestPermission(_ context.Context, _ acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{}, c.notSupported("session/request_permission")
}

// SessionUpdate 接收远程 Agent 的会话更新通知（流式进度）
func (c *acpClientImpl) SessionUpdate(_ context.Context, params acp.SessionNotification) error {
	if c.onUpdate != nil {
		c.onUpdate(params)
	}
	return nil
}

// CreateTerminal 远程 Agent 请求创建终端 — 不支持
func (c *acpClientImpl) CreateTerminal(_ context.Context, _ acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, c.notSupported("terminal/create")
}

// KillTerminalCommand 远程 Agent 请求终止终端命令 — 不支持
func (c *acpClientImpl) KillTerminalCommand(_ context.Context, _ acp.KillTerminalCommandRequest) (acp.KillTerminalCommandResponse, error) {
	return acp.KillTerminalCommandResponse{}, c.notSupported("terminal/kill")
}

// TerminalOutput 远程 Agent 请求获取终端输出 — 不支持
func (c *acpClientImpl) TerminalOutput(_ context.Context, _ acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, c.notSupported("terminal/output")
}

// ReleaseTerminal 远程 Agent 请求释放终端 — 不支持
func (c *acpClientImpl) ReleaseTerminal(_ context.Context, _ acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, c.notSupported("terminal/release")
}

// WaitForTerminalExit 远程 Agent 请求等待终端退出 — 不支持
func (c *acpClientImpl) WaitForTerminalExit(_ context.Context, _ acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, c.notSupported("terminal/wait_for_exit")
}
