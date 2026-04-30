package tools

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// mockTaskExecutor 是用于测试的 TaskExecutor 实现
type mockTaskExecutor struct {
	executeFn func(ctx context.Context, agentID string, instruction string, taskContext map[string]interface{}) (string, error)
}

func (m *mockTaskExecutor) ExecuteTask(ctx context.Context, agentID string, instruction string, taskContext map[string]interface{}) (string, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, agentID, instruction, taskContext)
	}
	return "task completed", nil
}

func TestRegisterTask(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	executor := &mockTaskExecutor{}
	registerTask(host, executor, logger, nil, 0)

	// 验证工具已注册
	tools := host.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "task" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("task 工具未注册")
	}
}

func TestTaskTool_MasterCanCall(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	called := false
	executor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, agentID string, instruction string, taskContext map[string]interface{}) (string, error) {
			called = true
			if agentID != "research" {
				t.Errorf("期望 agent_id=research, 实际=%s", agentID)
			}
			if instruction != "分析代码" {
				t.Errorf("期望 instruction=分析代码, 实际=%s", instruction)
			}
			return "分析完成", nil
		},
	}
	registerTask(host, executor, logger, nil, 0)

	// 构造 Master 调用的 context
	ctx := WithToolContext(context.Background(), &ToolContext{
		CallerType: CallerMaster,
		CallerName: "master",
		Depth:      0,
	})

	input, _ := json.Marshal(taskInput{
		AgentID:     "research",
		Instruction: "分析代码",
	})

	result, err := host.ExecuteTool(ctx, "task", input)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}

	if result.IsError {
		t.Fatalf("工具返回错误: %s", string(result.Content))
	}

	if !called {
		t.Fatal("ExecuteTask 未被调用")
	}
}

func TestTaskTool_SubAgentCannotCall(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	called := false
	executor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, agentID string, instruction string, taskContext map[string]interface{}) (string, error) {
			called = true
			return "不应该执行", nil
		},
	}
	registerTask(host, executor, logger, nil, 0)

	// 构造 SubAgent 调用的 context
	ctx := WithToolContext(context.Background(), &ToolContext{
		CallerType: CallerSubAgent,
		CallerName: "research",
		Depth:      1,
	})

	input, _ := json.Marshal(taskInput{
		AgentID:     "plan",
		Instruction: "制定计划",
	})

	result, err := host.ExecuteTool(ctx, "task", input)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}

	if !result.IsError {
		t.Fatal("期望返回错误，但成功了")
	}

	if called {
		t.Fatal("ExecuteTask 不应该被调用")
	}
}

func TestTaskTool_MaxDepthCheck(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	called := false
	executor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, agentID string, instruction string, taskContext map[string]interface{}) (string, error) {
			called = true
			return "不应该执行", nil
		},
	}
	registerTask(host, executor, logger, nil, 0)

	// 构造深度超过限制的 context
	ctx := WithToolContext(context.Background(), &ToolContext{
		CallerType: CallerMaster,
		CallerName: "master",
		Depth:      maxDepth, // 等于最大深度
	})

	input, _ := json.Marshal(taskInput{
		AgentID:     "research",
		Instruction: "深度调用",
	})

	result, err := host.ExecuteTool(ctx, "task", input)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}

	if !result.IsError {
		t.Fatal("期望返回深度错误，但成功了")
	}

	if called {
		t.Fatal("ExecuteTask 不应该被调用")
	}
}

func TestTaskTool_MissingAgentID(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	executor := &mockTaskExecutor{}
	registerTask(host, executor, logger, nil, 0)

	ctx := WithToolContext(context.Background(), &ToolContext{
		CallerType: CallerMaster,
		CallerName: "master",
		Depth:      0,
	})

	input, _ := json.Marshal(map[string]interface{}{
		"instruction": "测试任务",
	})

	result, err := host.ExecuteTool(ctx, "task", input)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}

	// agent_id 为空时应返回错误（不再默认填 "general"）
	if !result.IsError {
		t.Fatal("agent_id 为空时应返回错误，但成功了")
	}
}

func TestTaskTool_MissingInstruction(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	executor := &mockTaskExecutor{}
	registerTask(host, executor, logger, nil, 0)

	ctx := WithToolContext(context.Background(), &ToolContext{
		CallerType: CallerMaster,
		CallerName: "master",
		Depth:      0,
	})

	input, _ := json.Marshal(map[string]interface{}{
		"agent_id": "research",
	})

	result, err := host.ExecuteTool(ctx, "task", input)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}

	if !result.IsError {
		t.Fatal("期望返回错误（缺少 instruction），但成功了")
	}
}

func TestTaskTool_ExecutionError(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	executor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, agentID string, instruction string, taskContext map[string]interface{}) (string, error) {
			return "", errs.New(errs.CodeAgentNotFound, "agent not found")
		},
	}
	registerTask(host, executor, logger, nil, 0)

	ctx := WithToolContext(context.Background(), &ToolContext{
		CallerType: CallerMaster,
		CallerName: "master",
		Depth:      0,
	})

	input, _ := json.Marshal(taskInput{
		AgentID:     "nonexistent",
		Instruction: "测试",
	})

	result, err := host.ExecuteTool(ctx, "task", input)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}

	if !result.IsError {
		t.Fatal("期望返回执行错误，但成功了")
	}
}

func TestTaskTool_WithContext(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	var receivedContext map[string]interface{}
	executor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, agentID string, instruction string, taskContext map[string]interface{}) (string, error) {
			receivedContext = taskContext
			return "ok", nil
		},
	}
	registerTask(host, executor, logger, nil, 0)

	ctx := WithToolContext(context.Background(), &ToolContext{
		CallerType: CallerMaster,
		CallerName: "master",
		Depth:      0,
	})

	expectedContext := map[string]interface{}{
		"file":   "test.go",
		"line":   42,
		"commit": "abc123",
	}

	input, _ := json.Marshal(taskInput{
		AgentID:     "research",
		Instruction: "分析文件",
		Context:     expectedContext,
	})

	result, err := host.ExecuteTool(ctx, "task", input)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}

	if result.IsError {
		t.Fatalf("工具返回错误: %s", string(result.Content))
	}

	if receivedContext == nil {
		t.Fatal("context 未传递")
	}

	if receivedContext["file"] != "test.go" {
		t.Errorf("期望 context.file=test.go, 实际=%v", receivedContext["file"])
	}
}

func TestTaskTool_InvalidJSON(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	executor := &mockTaskExecutor{}
	registerTask(host, executor, logger, nil, 0)

	ctx := WithToolContext(context.Background(), &ToolContext{
		CallerType: CallerMaster,
		CallerName: "master",
		Depth:      0,
	})

	input := json.RawMessage(`{"invalid json`)

	result, err := host.ExecuteTool(ctx, "task", input)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}

	if !result.IsError {
		t.Fatal("期望返回 JSON 解析错误，但成功了")
	}
}

func TestTaskTool_SystemAgentDenyList(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	called := false
	executor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, agentID string, instruction string, taskContext map[string]interface{}) (string, error) {
			called = true
			return "不应该执行", nil
		},
	}
	registerTask(host, executor, logger, nil, 0)

	ctx := WithToolContext(context.Background(), &ToolContext{
		CallerType: CallerMaster,
		CallerName: "master",
		Depth:      0,
	})

	// 测试所有系统 Agent 都被拒绝
	for _, sysAgent := range []string{"codereview", "compaction", "title-agent", "summary"} {
		input, _ := json.Marshal(taskInput{
			AgentID:     sysAgent,
			Instruction: "测试",
		})

		result, err := host.ExecuteTool(ctx, "task", input)
		if err != nil {
			t.Fatalf("调用工具失败: %v", err)
		}
		if !result.IsError {
			t.Fatalf("系统 Agent %q 应被拒绝，但成功了", sysAgent)
		}
		if called {
			t.Fatalf("系统 Agent %q 不应触发 ExecuteTask", sysAgent)
		}
	}

	// explore 不在黑名单中，应该可以调用
	input, _ := json.Marshal(taskInput{
		AgentID:     "explore",
		Instruction: "搜索代码",
	})
	result, err := host.ExecuteTool(ctx, "task", input)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}
	if result.IsError {
		t.Fatal("explore 不应被拒绝")
	}
}
