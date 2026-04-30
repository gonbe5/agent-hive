package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chef-guo/agents-hive/internal/mcphost"
	"github.com/chef-guo/agents-hive/internal/toolctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// delegationTestExecutor 模拟 TaskExecutor（委托测试专用）
type delegationTestExecutor struct {
	lastAgentID     string
	lastInstruction string
	result          string
	err             error
}

func (m *delegationTestExecutor) ExecuteTask(_ context.Context, agentID string, instruction string, _ map[string]interface{}) (string, error) {
	m.lastAgentID = agentID
	m.lastInstruction = instruction
	return m.result, m.err
}

func setupTaskTool(t *testing.T, executor TaskExecutor) *mcphost.Host {
	t.Helper()
	host := mcphost.NewHost(zap.NewNop())
	registerTask(host, executor, zap.NewNop(), nil, 0)
	return host
}

func callTaskWithContext(t *testing.T, host *mcphost.Host, callerType toolctx.CallerType, callerName string, depth int, input map[string]any) (*mcphost.ToolResult, error) {
	t.Helper()
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	ctx := toolctx.WithToolContext(context.Background(), &toolctx.ToolContext{
		CallerType: callerType,
		CallerName: callerName,
		Depth:      depth,
	})
	return host.ExecuteTool(ctx, "task", inputJSON)
}

func TestTaskDelegation_FixedAgentAllowed(t *testing.T) {
	executor := &delegationTestExecutor{result: "done"}
	host := setupTaskTool(t, executor)

	result, err := callTaskWithContext(t, host, toolctx.CallerFixedAgent, "general", 0, map[string]any{
		"agent_id":    "research",
		"instruction": "查找相关资料",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, "research", executor.lastAgentID)
	assert.Equal(t, "查找相关资料", executor.lastInstruction)
}

func TestTaskDelegation_MasterStillAllowed(t *testing.T) {
	executor := &delegationTestExecutor{result: "ok"}
	host := setupTaskTool(t, executor)

	result, err := callTaskWithContext(t, host, toolctx.CallerMaster, "master", 0, map[string]any{
		"agent_id":    "explore",
		"instruction": "执行任务",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestTaskDelegation_DynamicSubAgentRejected(t *testing.T) {
	executor := &delegationTestExecutor{result: "should not reach"}
	host := setupTaskTool(t, executor)

	result, err := callTaskWithContext(t, host, toolctx.CallerSubAgent, "dynamic-123", 0, map[string]any{
		"instruction": "不应该执行",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, mcphost.DecodeToolContent(result.Content), "仅允许 Master Agent 和固定 Agent 调用")
}

func TestTaskDelegation_SelfDelegationBlocked(t *testing.T) {
	executor := &delegationTestExecutor{result: "should not reach"}
	host := setupTaskTool(t, executor)

	result, err := callTaskWithContext(t, host, toolctx.CallerFixedAgent, "general", 0, map[string]any{
		"agent_id":    "general",
		"instruction": "自己委托给自己",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, mcphost.DecodeToolContent(result.Content), "不能委托任务给自己")
}

func TestTaskDelegation_DepthLimitEnforced(t *testing.T) {
	executor := &delegationTestExecutor{result: "should not reach"}
	host := setupTaskTool(t, executor)

	result, err := callTaskWithContext(t, host, toolctx.CallerFixedAgent, "general", maxDepth, map[string]any{
		"agent_id":    "research",
		"instruction": "深度超限",
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, mcphost.DecodeToolContent(result.Content), "调用深度超过最大限制")
}

func TestTaskDelegation_DepthBelowLimitAllowed(t *testing.T) {
	executor := &delegationTestExecutor{result: "ok"}
	host := setupTaskTool(t, executor)

	result, err := callTaskWithContext(t, host, toolctx.CallerFixedAgent, "code", maxDepth-1, map[string]any{
		"agent_id":    "research",
		"instruction": "深度刚好在限制内",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
}
