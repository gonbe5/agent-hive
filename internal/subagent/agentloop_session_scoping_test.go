package subagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/mcphost"
	"github.com/chef-guo/agents-hive/internal/toolctx"
)

// subagent-session-scoping spec contract test:
// 验证 emitProgress 在 ProgressEvent.SessionID 为空时自动注入 AgentLoop.sessionID。
// 这是 D4 single-source-of-truth 决策的硬抓手 —— 任何回归（漏写注入）会让此测试红。
func TestAgentLoop_EmitProgress_InjectsSessionID(t *testing.T) {
	var capturedEvents []ProgressEvent
	loop := &AgentLoop{
		logger:    zap.NewNop(),
		agentID:   "test-agent",
		sessionID: "sA",
		progressFn: func(event ProgressEvent) {
			capturedEvents = append(capturedEvents, event)
		},
	}

	loop.emitProgress(ProgressEvent{
		AgentID:  "test-agent",
		Turn:     0,
		MaxTurns: 25,
		ToolName: "fs_read",
		Status:   "tool_start",
	})

	require.Len(t, capturedEvents, 1)
	assert.Equal(t, "sA", capturedEvents[0].SessionID,
		"emitProgress 必须把 AgentLoop.sessionID 注入到 event.SessionID")
}

// subagent-session-scoping spec contract test:
// 验证显式传入的 SessionID 不被覆盖（防止破坏 caller 已知场景）
func TestAgentLoop_EmitProgress_PreservesExplicitSessionID(t *testing.T) {
	var capturedEvents []ProgressEvent
	loop := &AgentLoop{
		logger:    zap.NewNop(),
		agentID:   "test-agent",
		sessionID: "sA",
		progressFn: func(event ProgressEvent) {
			capturedEvents = append(capturedEvents, event)
		},
	}

	loop.emitProgress(ProgressEvent{
		AgentID:   "test-agent",
		SessionID: "sExplicit",
		Status:    "tool_done",
	})

	require.Len(t, capturedEvents, 1)
	assert.Equal(t, "sExplicit", capturedEvents[0].SessionID,
		"显式 SessionID 不应被 AgentLoop.sessionID 覆盖")
}

// subagent-session-scoping spec contract test:
// 验证 streamFn 调用时第 2 参 == AgentLoop.sessionID。
// 这是 task 2.2 的硬抓手 —— Run 路径修改后必须保证 sessionID 流出。
func TestAgentLoop_Run_StreamCallback_CarriesSessionID(t *testing.T) {
	type streamArgs struct {
		AgentID   string
		SessionID string
		Content   string
		Reasoning string
	}
	var captured []streamArgs

	llmClient := &mockLLMClient{
		responses: []*llm.ChatWithToolsResponse{
			{Content: "hello", FinishReason: "stop"},
		},
	}

	loop := &AgentLoop{
		llmClient: llmClient,
		toolBridge: &mockToolBridge{
			tools: []mcphost.ToolDefinition{},
		},
		logger:    zap.NewNop(),
		maxTurns:  5,
		agentID:   "stream-agent",
		sessionID: "sB",
		streamFn: func(agentID, sessionID, content, reasoning string) {
			captured = append(captured, streamArgs{agentID, sessionID, content, reasoning})
		},
	}

	_, err := loop.Run(context.Background(), "sys", []llm.MessageWithTools{
		{Role: "user", Content: llm.NewTextContent("hi")},
	}, nil)
	require.NoError(t, err)

	require.NotEmpty(t, captured, "streamFn 至少被调用一次")
	for i, args := range captured {
		assert.Equal(t, "sB", args.SessionID,
			"streamFn 第 %d 次调用：sessionID 必须等于 AgentLoop.sessionID", i)
		assert.Equal(t, "stream-agent", args.AgentID)
	}
}

// subagent-session-scoping spec contract test:
// 验证 emitProgress 在 progressFn 为 nil 时不 panic（保留原有 nil-safety）
func TestAgentLoop_EmitProgress_NilCallbackSafe(t *testing.T) {
	loop := &AgentLoop{
		logger:    zap.NewNop(),
		sessionID: "sA",
	}
	assert.NotPanics(t, func() {
		loop.emitProgress(ProgressEvent{Status: "tool_start"})
	})
}

// subagent-session-scoping spec contract test:
// 验证 ctx-injected sessionID 与 AgentLoop.sessionID 不同时，emitProgress 走的是
// 实例字段（D4 single source）—— ctx 字段是 tool 调用用的，与 emit 无关。
func TestAgentLoop_EmitProgress_UsesInstanceFieldNotCtx(t *testing.T) {
	var capturedEvents []ProgressEvent
	loop := &AgentLoop{
		logger:    zap.NewNop(),
		agentID:   "test-agent",
		sessionID: "sInstance",
		progressFn: func(event ProgressEvent) {
			capturedEvents = append(capturedEvents, event)
		},
	}

	// ctx 携带不同的 sessionID —— 不应影响 emit
	_ = toolctx.WithSessionID(context.Background(), "sFromCtx")

	loop.emitProgress(ProgressEvent{
		AgentID: "test-agent",
		Status:  "turn_done",
	})

	require.Len(t, capturedEvents, 1)
	assert.Equal(t, "sInstance", capturedEvents[0].SessionID,
		"emitProgress 应只读 AgentLoop.sessionID，不读 ctx")
}

// 兼容性辅助：mock LLM 在 streamCallback 路径上 emit chunks
var _ = json.RawMessage{}
