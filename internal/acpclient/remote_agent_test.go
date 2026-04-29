package acpclient

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/subagent"
)

// TestRemoteACPAgentInterface 验证 RemoteACPAgent 实现 subagent.Agent 接口
func TestRemoteACPAgentInterface(t *testing.T) {
	var _ subagent.Agent = (*RemoteACPAgent)(nil)
}

// TestRemoteACPAgentCard 验证 Card 返回正确信息
func TestRemoteACPAgentCard(t *testing.T) {
	logger := zaptest.NewLogger(t)
	agent := NewRemoteACPAgent(
		RemoteAgentConfig{
			Name:        "test-agent",
			Description: "测试 Agent",
			Skills:      []string{"code_review"},
		},
		nil, nil, "session-123", logger,
	)

	card := agent.Card()
	if card.ID != "test-agent" {
		t.Errorf("Card().ID = %q, want %q", card.ID, "test-agent")
	}
	if card.Name != "test-agent" {
		t.Errorf("Card().Name = %q, want %q", card.Name, "test-agent")
	}
	if card.Description != "测试 Agent" {
		t.Errorf("Card().Description = %q, want %q", card.Description, "测试 Agent")
	}
	if len(card.Skills) != 1 || card.Skills[0] != "code_review" {
		t.Errorf("Card().Skills = %v, want [code_review]", card.Skills)
	}
}

// TestRemoteACPAgentID 验证 ID 返回配置名称
func TestRemoteACPAgentID(t *testing.T) {
	logger := zaptest.NewLogger(t)
	agent := NewRemoteACPAgent(
		RemoteAgentConfig{Name: "my-agent"},
		nil, nil, "", logger,
	)
	if agent.ID() != "my-agent" {
		t.Errorf("ID() = %q, want %q", agent.ID(), "my-agent")
	}
}

// TestRemoteACPAgentStatus 验证状态转换
func TestRemoteACPAgentStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	agent := NewRemoteACPAgent(
		RemoteAgentConfig{Name: "test"},
		nil, nil, "", logger,
	)

	if agent.Status() != subagent.StatusStopped {
		t.Errorf("初始状态应为 StatusStopped, got %v", agent.Status())
	}

	agent.setStatus(subagent.StatusRunning)
	if agent.Status() != subagent.StatusRunning {
		t.Errorf("设置后状态应为 StatusRunning, got %v", agent.Status())
	}
}

// TestRemoteACPAgentMailbox 验证 Mailbox 不为 nil
func TestRemoteACPAgentMailbox(t *testing.T) {
	logger := zaptest.NewLogger(t)
	agent := NewRemoteACPAgent(
		RemoteAgentConfig{Name: "test"},
		nil, nil, "", logger,
	)

	if agent.Mailbox() == nil {
		t.Error("Mailbox() should not be nil")
	}
}

// TestRemoteACPAgentSendTaskNotRunning 验证未运行时 SendTask 返回错误
func TestRemoteACPAgentSendTaskNotRunning(t *testing.T) {
	logger := zaptest.NewLogger(t)
	agent := NewRemoteACPAgent(
		RemoteAgentConfig{Name: "test"},
		nil, nil, "", logger,
	)

	ctx := context.Background()
	_, err := agent.SendTask(ctx, subagent.TaskRequest{
		ID:      "task-1",
		Type:    "test",
		Payload: json.RawMessage(`{"instruction":"hello"}`),
	})
	if err == nil {
		t.Error("SendTask 应在 agent 未运行时返回错误")
	}
}

// TestRemoteACPAgentStop 验证 Stop 不 panic
func TestRemoteACPAgentStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	agent := NewRemoteACPAgent(
		RemoteAgentConfig{Name: "test"},
		nil, nil, "", logger,
	)
	agent.Stop()
}

// TestRemoteACPAgentPingTimeout 验证 Ping 超时
func TestRemoteACPAgentPingTimeout(t *testing.T) {
	logger := zaptest.NewLogger(t)
	agent := NewRemoteACPAgent(
		RemoteAgentConfig{Name: "test"},
		nil, nil, "", logger,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := agent.Ping(ctx)
	if err == nil {
		t.Error("Ping 应在 agent 未运行时超时返回错误")
	}
}

// TestClientImplNotSupported 验证 acpClientImpl 大部分方法返回 error
func TestClientImplNotSupported(t *testing.T) {
	logger := zap.NewNop()
	impl := newACPClientImpl("test", logger)
	ctx := context.Background()

	_, err := impl.ReadTextFile(ctx, acp.ReadTextFileRequest{Path: "/tmp/test"})
	if err == nil {
		t.Error("ReadTextFile 应返回 not supported 错误")
	}

	_, err = impl.WriteTextFile(ctx, acp.WriteTextFileRequest{Path: "/tmp/test", Content: "x"})
	if err == nil {
		t.Error("WriteTextFile 应返回 not supported 错误")
	}

	_, err = impl.CreateTerminal(ctx, acp.CreateTerminalRequest{Command: "echo"})
	if err == nil {
		t.Error("CreateTerminal 应返回 not supported 错误")
	}
}

// TestClientImplSessionUpdate 验证 SessionUpdate 正常工作
func TestClientImplSessionUpdate(t *testing.T) {
	logger := zap.NewNop()
	impl := newACPClientImpl("test", logger)

	var received bool
	impl.onUpdate = func(_ acp.SessionNotification) {
		received = true
	}

	err := impl.SessionUpdate(context.Background(), acp.SessionNotification{})
	if err != nil {
		t.Errorf("SessionUpdate 不应返回错误: %v", err)
	}
	if !received {
		t.Error("SessionUpdate 回调未被调用")
	}
}

// TestNewTransportInvalidType 验证不支持的传输类型
func TestNewTransportInvalidType(t *testing.T) {
	_, err := NewTransport(RemoteAgentConfig{
		Name:      "test",
		Transport: "invalid",
	})
	if err == nil {
		t.Error("NewTransport 应对不支持的传输类型返回错误")
	}
}

// TestNewTransportStdioMissingCommand 验证缺少 command
func TestNewTransportStdioMissingCommand(t *testing.T) {
	_, err := NewTransport(RemoteAgentConfig{
		Name:      "test",
		Transport: "stdio",
	})
	if err == nil {
		t.Error("NewTransport stdio 应对缺少 command 返回错误")
	}
}

// TestNewTransportHTTPMissingURL 验证缺少 URL
func TestNewTransportHTTPMissingURL(t *testing.T) {
	_, err := NewTransport(RemoteAgentConfig{
		Name:      "test",
		Transport: "http",
	})
	if err == nil {
		t.Error("NewTransport http 应对缺少 url 返回错误")
	}
}
