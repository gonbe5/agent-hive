package master

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/subagent"
	"github.com/chef-guo/agents-hive/internal/toolctx"
)

// buildBashInput 构造 shell 家族工具期望的 Input JSON。
func buildBashInput(t *testing.T, cmd string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]string{"command": cmd})
	if err != nil {
		t.Fatalf("marshal bash input: %v", err)
	}
	return b
}

// callPermissionFn 通过 master.createPermissionPromptFn 构造的闭包调用权限路径。
func callPermissionFn(t *testing.T, m *Master, sessionID, toolName string, input json.RawMessage) (skills.PermissionResponse, error) {
	t.Helper()
	ctx := toolctx.WithSessionID(context.Background(), sessionID)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	fn := m.createPermissionPromptFn()
	return fn(ctx, skills.PermissionRequest{
		ToolName:    toolName,
		Description: "test",
		Input:       input,
	})
}

// TestPermissionFn_IM_PolicyAllow 路径 (a)：IM session + PolicyAllow 命令 → Granted:true，无 HITL。
func TestPermissionFn_IM_PolicyAllow(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer cancel()
	defer m.Stop()

	resp, err := callPermissionFn(t, m, "im-user-1", "bash", buildBashInput(t, "ls -la"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !resp.Granted {
		t.Errorf("IM + PolicyAllow 命令必须放行，got Granted=false")
	}
}

// TestPermissionFn_IM_PolicyDeny 路径 (b)：IM session + PolicyDeny 命令（rm -rf /）→ Granted:false。
// invariant: MatchPolicy 必须早于 im- 前缀短路；否则 IM 用户能打穿 rm -rf /。
func TestPermissionFn_IM_PolicyDeny(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer cancel()
	defer m.Stop()

	resp, err := callPermissionFn(t, m, "im-user-1", "bash", buildBashInput(t, "rm -rf /"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Granted {
		t.Error("PolicyDeny 命令在 IM 通道必须被拒绝（防 rm -rf / 打穿）")
	}
}

// TestPermissionFn_IM_PolicyAsk 路径 (c)：IM session + PolicyAsk 命令 → 走 HITL 审批。
func TestPermissionFn_IM_PolicyAsk(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer cancel()
	defer m.Stop()

	subID, ch := m.SubscribeWSBroadcast()
	defer m.UnsubscribeWSBroadcast(subID)

	respCh := make(chan skills.PermissionResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := callPermissionFn(t, m, "im-user-2", "bash", buildBashInput(t, "rm -rf /tmp/foo"))
		respCh <- resp
		errCh <- err
	}()

	select {
	case msg := <-ch:
		if msg.Type != EventTypeInputRequest {
			t.Fatalf("want input_request, got %q", msg.Type)
		}
		inputReq, ok := msg.Payload.(*InputRequest)
		if !ok {
			t.Fatalf("payload not *InputRequest, got %T", msg.Payload)
		}
		if inputReq.Type != InputPermission {
			t.Fatalf("want InputPermission, got %q", inputReq.Type)
		}
		if inputReq.TaskID != "im-user-2" {
			t.Fatalf("want TaskID im-user-2, got %q", inputReq.TaskID)
		}
		if err := m.SubmitInput(InputResponse{
			RequestID: inputReq.ID,
			TaskID:    inputReq.TaskID,
			Action:    "approve",
		}); err != nil {
			t.Fatalf("SubmitInput: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("IM + PolicyAsk 未在 500ms 内广播审批请求")
	}

	select {
	case resp := <-respCh:
		if !resp.Granted {
			t.Error("HITL approve 后必须 Granted=true")
		}
		if err := <-errCh; err != nil {
			t.Errorf("unexpected err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("permission fn 未在 HITL 响应后返回")
	}
}

// TestPermissionFn_NonIM_PolicyDeny 路径 (d)：非 IM session + PolicyDeny → Granted:false。
func TestPermissionFn_NonIM_PolicyDeny(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer cancel()
	defer m.Stop()

	resp, err := callPermissionFn(t, m, "web-session-1", "bash", buildBashInput(t, "mkfs.ext4 /dev/sda1"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Granted {
		t.Error("PolicyDeny 命令必须被拒绝，无论会话类型")
	}
}

// TestPermissionFn_NonIM_PolicyAsk_HITL 路径 (e)：非 IM + PolicyAsk → 走 HITL 审批流程。
// 通过后台 goroutine 提交 approve 响应验证 HITL 可达性。
func TestPermissionFn_NonIM_PolicyAsk_HITL(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer cancel()
	defer m.Stop()

	// 订阅广播，拦截 HITL input_request
	subID, ch := m.SubscribeWSBroadcast()
	defer m.UnsubscribeWSBroadcast(subID)

	respCh := make(chan skills.PermissionResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := callPermissionFn(t, m, "web-session-2", "bash", buildBashInput(t, "rm -rf /home/user/cache"))
		respCh <- resp
		errCh <- err
	}()

	// 等待 HITL 广播，然后 SubmitInput approve
	select {
	case msg := <-ch:
		if msg.Type != EventTypeInputRequest {
			t.Fatalf("want input_request, got %q", msg.Type)
		}
		inputReq, ok := msg.Payload.(*InputRequest)
		if !ok {
			t.Fatalf("payload not *InputRequest, got %T", msg.Payload)
		}
		if inputReq.Type != InputPermission {
			t.Fatalf("want InputPermission, got %q", inputReq.Type)
		}
		if err := m.SubmitInput(InputResponse{
			RequestID: inputReq.ID,
			TaskID:    inputReq.TaskID,
			Action:    "approve",
		}); err != nil {
			t.Fatalf("SubmitInput: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HITL input_request 未在 500ms 内广播——PolicyAsk 非 IM 路径应走 HITL")
	}

	select {
	case resp := <-respCh:
		if !resp.Granted {
			t.Errorf("HITL approve 后必须 Granted=true")
		}
		if err := <-errCh; err != nil {
			t.Errorf("unexpected err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("permission fn 未在 HITL 响应后返回")
	}
}

// TestPermissionFn_NonIM_PolicyAllow 路径 (f)：非 IM + PolicyAllow → Granted:true，无 HITL。
func TestPermissionFn_NonIM_PolicyAllow(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer cancel()
	defer m.Stop()

	resp, err := callPermissionFn(t, m, "web-session-3", "bash", buildBashInput(t, "git status"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !resp.Granted {
		t.Errorf("非 IM + PolicyAllow 必须放行（minimal 模式默认允许）")
	}
}

// TestPermissionFn_BusinessDecision_Orthogonal 路径 (g)：业务决策 input_request 与 Permission 正交。
// 非 shell 工具在 minimal 模式下直接放行；strict 模式走 HITL。
// EmitInputRequest（ChoiceType 驱动）有独立路径，与 createPermissionPromptFn 无交互。
func TestPermissionFn_BusinessDecision_Orthogonal(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer cancel()
	defer m.Stop()

	// 业务工具（非 shell 家族），minimal 模式下应直接放行，Input 不是 shell 文本
	bizInput, _ := json.Marshal(map[string]any{"theme_id": "abc", "title": "post"})
	resp, err := callPermissionFn(t, m, "web-session-4", "xiaohongshu_publish", bizInput)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !resp.Granted {
		t.Errorf("非 shell 工具在 minimal 模式必须放行（与业务决策路径正交）")
	}

	// 订阅广播，确认没有因此触发 HITL 权限弹窗
	subID, ch := m.SubscribeWSBroadcast()
	defer m.UnsubscribeWSBroadcast(subID)

	_, _ = callPermissionFn(t, m, "web-session-5", "mcp_tool_foo", bizInput)
	select {
	case msg := <-ch:
		if msg.Type == EventTypeInputRequest {
			t.Errorf("非 shell 工具不应触发 HITL permission 广播，got: %+v", msg)
		}
	case <-time.After(100 * time.Millisecond):
		// 预期路径：无广播
	}
}

// TestPermissionFn_StrictMode_NonShellTool 覆盖 strict 兜底：非 shell 工具也走 HITL。
func TestPermissionFn_StrictMode_NonShellTool(t *testing.T) {
	m, stop := setupHITLMasterWithStrict(t)
	defer stop()
	defer m.Stop()

	subID, ch := m.SubscribeWSBroadcast()
	defer m.UnsubscribeWSBroadcast(subID)

	errCh := make(chan error, 1)
	go func() {
		_, err := callPermissionFn(t, m, "web-session-strict", "xiaohongshu_publish", json.RawMessage(`{"theme_id":"abc"}`))
		errCh <- err
	}()

	// 等 HITL 广播，deny 掉验证路径畅通
	select {
	case msg := <-ch:
		if msg.Type != EventTypeInputRequest {
			t.Fatalf("want input_request, got %q", msg.Type)
		}
		inputReq, ok := msg.Payload.(*InputRequest)
		if !ok {
			t.Fatalf("payload not *InputRequest")
		}
		_ = m.SubmitInput(InputResponse{RequestID: inputReq.ID, TaskID: inputReq.TaskID, Action: "deny"})
	case <-time.After(500 * time.Millisecond):
		t.Fatal("strict 模式非 shell 工具必须走 HITL，但未广播")
	}
	<-errCh
}

// TestPermissionFn_ShellInputMalformed 覆盖 safe-deny：Input 解析失败走拒绝路径。
func TestPermissionFn_ShellInputMalformed(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer cancel()
	defer m.Stop()

	// 故意构造非法 JSON
	resp, err := callPermissionFn(t, m, "im-user-x", "bash", json.RawMessage(`not-a-json`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Granted {
		t.Error("Input 解析失败必须 safe-deny，got Granted=true")
	}

	// 空 command 同理
	emptyCmd, _ := json.Marshal(map[string]string{"command": ""})
	resp2, err := callPermissionFn(t, m, "im-user-x", "bash", emptyCmd)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp2.Granted {
		t.Error("空 command 必须 safe-deny")
	}
}

// setupHITLMasterWithStrict 构造 strict 模式 Master，镜像 setupHITLMaster 的初始化路径。
func setupHITLMasterWithStrict(t *testing.T) (*Master, context.CancelFunc) {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)
	m := NewMaster(Config{
		Model:                  "test",
		SecurityPermissionMode: "strict",
	}, config.HITLConfig{Enabled: true}, agentReg, skillReg, nil, logger)
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	return m, cancel
}
