package master

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// newTestBroker 创建用于测试的 HITLBroker，使用 nop logger 减少测试噪声
func newTestBroker(t *testing.T, inputTimeout time.Duration) (*HITLBroker, chan struct{}) {
	t.Helper()
	logger := zap.NewNop()
	eventBus := NewEventBus(logger)
	t.Cleanup(func() { eventBus.Close() })
	stopCh := make(chan struct{})
	broker := NewHITLBroker(
		config.HITLConfig{
			Enabled:      true,
			InputTimeout: inputTimeout,
		},
		eventBus,
		stopCh,
		logger,
	)
	return broker, stopCh
}

// ────────────────────────────────────────────────────────────────────────────
// Enabled / NextInputID / RequestInput
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_Enabled 验证 Enabled() 按配置返回正确值
func TestHITLBroker_Enabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"启用状态", true},
		{"禁用状态", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := zap.NewNop()
			eb := NewEventBus(logger)
			t.Cleanup(func() { eb.Close() })
			stopCh := make(chan struct{})
			broker := NewHITLBroker(
				config.HITLConfig{Enabled: tc.enabled},
				eb, stopCh, logger,
			)
			if got := broker.Enabled(); got != tc.enabled {
				t.Errorf("Enabled() = %v, want %v", got, tc.enabled)
			}
		})
	}
}

// TestHITLBroker_NextInputID 验证 NextInputID 生成唯一的递增 ID
func TestHITLBroker_NextInputID(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	id1 := broker.NextInputID("perm")
	id2 := broker.NextInputID("perm")
	id3 := broker.NextInputID("req")

	if id1 == id2 || id1 == id3 || id2 == id3 {
		t.Errorf("NextInputID 生成了重复 ID: %s %s %s", id1, id2, id3)
	}
}

// TestHITLBroker_RequestInput_Registration 验证 RequestInput 正确注册待处理项
func TestHITLBroker_RequestInput_Registration(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	req := broker.RequestInput("task-1", "step-1", InputApproval, "请批准此操作", []string{"approve", "reject"})

	if req == nil {
		t.Fatal("RequestInput 返回了 nil")
	}
	if req.ID == "" {
		t.Error("请求 ID 不应为空")
	}
	if req.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want task-1", req.TaskID)
	}
	if req.Type != InputApproval {
		t.Errorf("Type = %q, want %q", req.Type, InputApproval)
	}

	// 验证已注册到 pendingInput
	pending := broker.PendingInputs("task-1")
	if len(pending) != 1 {
		t.Fatalf("PendingInputs 返回 %d 项，期望 1 项", len(pending))
	}
	if pending[0].ID != req.ID {
		t.Errorf("pending[0].ID = %q, want %q", pending[0].ID, req.ID)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// PendingInputs
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_PendingInputs 全面覆盖 PendingInputs 的查询行为
func TestHITLBroker_PendingInputs(t *testing.T) {
	t.Run("空列表", func(t *testing.T) {
		broker, _ := newTestBroker(t, 5*time.Second)
		result := broker.PendingInputs("task-1")
		if len(result) != 0 {
			t.Errorf("期望空列表，得到 %d 项", len(result))
		}
	})

	t.Run("按 taskID 过滤", func(t *testing.T) {
		broker, _ := newTestBroker(t, 5*time.Second)
		broker.RequestInput("task-A", "step-1", InputApproval, "A1", nil)
		broker.RequestInput("task-A", "step-2", InputApproval, "A2", nil)
		broker.RequestInput("task-B", "step-1", InputApproval, "B1", nil)

		forA := broker.PendingInputs("task-A")
		if len(forA) != 2 {
			t.Errorf("task-A 期望 2 项，得到 %d 项", len(forA))
		}
		forB := broker.PendingInputs("task-B")
		if len(forB) != 1 {
			t.Errorf("task-B 期望 1 项，得到 %d 项", len(forB))
		}
	})

	t.Run("空 taskID 返回全部", func(t *testing.T) {
		broker, _ := newTestBroker(t, 5*time.Second)
		broker.RequestInput("task-X", "step-1", InputApproval, "X1", nil)
		broker.RequestInput("task-Y", "step-1", InputApproval, "Y1", nil)

		all := broker.PendingInputs("")
		if len(all) != 2 {
			t.Errorf("空 taskID 期望全部 2 项，得到 %d 项", len(all))
		}
	})

	t.Run("不存在的 taskID 返回空", func(t *testing.T) {
		broker, _ := newTestBroker(t, 5*time.Second)
		broker.RequestInput("task-1", "step-1", InputApproval, "p1", nil)
		result := broker.PendingInputs("task-nonexistent")
		if len(result) != 0 {
			t.Errorf("不存在的 taskID 期望 0 项，得到 %d 项", len(result))
		}
	})

	t.Run("并发读取安全", func(t *testing.T) {
		broker, _ := newTestBroker(t, 5*time.Second)
		// 注册若干请求
		for i := 0; i < 10; i++ {
			broker.RequestInput("task-concurrent", "step", InputApproval, "prompt", nil)
		}

		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = broker.PendingInputs("task-concurrent")
			}()
		}
		wg.Wait() // 若发生竞态，-race 会捕获
	})
}

// ────────────────────────────────────────────────────────────────────────────
// SubmitInput 验证
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_SubmitInput_Validation 覆盖 SubmitInput 的所有验证路径
func TestHITLBroker_SubmitInput_Validation(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		wantCode int
		setupReq bool // 是否需要预先注册请求
	}{
		{"非法 action", "fly_away", errs.CodeInputInvalid, false},
		{"合法 approve", "approve", 0, true},
		{"合法 reject", "reject", 0, true},
		{"合法 modify", "modify", 0, true},
		{"合法 proceed", "proceed", 0, true},
		{"合法 skip", "skip", 0, true},
		{"合法 cancel", "cancel", 0, true},
		{"合法空 action", "", 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			broker, _ := newTestBroker(t, 5*time.Second)

			var reqID, taskID string
			if tc.setupReq {
				req := broker.RequestInput("task-v", "step-v", InputApproval, "验证", nil)
				reqID = req.ID
				taskID = "task-v"
			} else {
				reqID = "fake-id"
				taskID = "task-v"
			}

			err := broker.SubmitInput(InputResponse{
				RequestID: reqID,
				TaskID:    taskID,
				Action:    tc.action,
			})

			if tc.wantCode != 0 {
				if err == nil {
					t.Fatalf("期望错误码 %d，但没有返回错误", tc.wantCode)
				}
				if !errs.IsCode(err, tc.wantCode) {
					t.Errorf("错误码 = %v，期望 %d", err, tc.wantCode)
				}
			} else {
				if err != nil {
					t.Errorf("期望成功，得到错误: %v", err)
				}
			}
		})
	}
}

// TestHITLBroker_SubmitInput_NotPending 提交不存在的请求 ID 应返回 CodeInputNotPending
func TestHITLBroker_SubmitInput_NotPending(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	err := broker.SubmitInput(InputResponse{
		RequestID: "nonexistent-id",
		TaskID:    "task-1",
		Action:    "approve",
	})
	if err == nil {
		t.Fatal("期望 CodeInputNotPending 错误，实际无错误")
	}
	if !errs.IsCode(err, errs.CodeInputNotPending) {
		t.Errorf("错误码期望 CodeInputNotPending，得到: %v", err)
	}
}

// TestHITLBroker_SubmitInput_TaskMismatch 提交时指定不匹配的 TaskID 应报错
func TestHITLBroker_SubmitInput_TaskMismatch(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	req := broker.RequestInput("task-real", "step-1", InputApproval, "提示", nil)

	err := broker.SubmitInput(InputResponse{
		RequestID: req.ID,
		TaskID:    "task-wrong", // 不匹配
		Action:    "approve",
	})
	if err == nil {
		t.Fatal("期望 CodeInputInvalid 错误（TaskID 不匹配），实际无错误")
	}
	if !errs.IsCode(err, errs.CodeInputInvalid) {
		t.Errorf("错误码期望 CodeInputInvalid，得到: %v", err)
	}
}

// TestHITLBroker_SubmitInput_EmptyTaskID 提交时 TaskID 为空应跳过匹配验证，正常成功
func TestHITLBroker_SubmitInput_EmptyTaskID(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	req := broker.RequestInput("task-real", "step-1", InputApproval, "提示", nil)

	// 空 TaskID 不触发任务 ID 匹配校验
	err := broker.SubmitInput(InputResponse{
		RequestID: req.ID,
		TaskID:    "", // 空 TaskID
		Action:    "approve",
	})
	if err != nil {
		t.Errorf("空 TaskID 期望成功，得到错误: %v", err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// WaitForInput / SubmitInput 正常流程
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_WaitForInput_NormalFlow 验证完整的等待-提交-收到响应流程
func TestHITLBroker_WaitForInput_NormalFlow(t *testing.T) {
	tests := []struct {
		name   string
		action string
		value  string
	}{
		{"approve 响应", "approve", "已批准"},
		{"reject 响应", "reject", "已拒绝"},
		{"modify 响应", "modify", "需修改"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			broker, _ := newTestBroker(t, 5*time.Second)
			req := broker.RequestInput("task-1", "step-1", InputApproval, "请批准", []string{"approve", "reject"})

			// 在后台提交响应
			go func() {
				time.Sleep(30 * time.Millisecond)
				if err := broker.SubmitInput(InputResponse{
					RequestID: req.ID,
					TaskID:    "task-1",
					Action:    tc.action,
					Value:     tc.value,
				}); err != nil {
					t.Errorf("SubmitInput 失败: %v", err)
				}
			}()

			ctx := context.Background()
			resp, err := broker.WaitForInput(ctx, "task-1", req)
			if err != nil {
				t.Fatalf("WaitForInput 返回错误: %v", err)
			}
			if resp.Action != tc.action {
				t.Errorf("Action = %q, want %q", resp.Action, tc.action)
			}
			if resp.Value != tc.value {
				t.Errorf("Value = %q, want %q", resp.Value, tc.value)
			}
		})
	}
}

// TestHITLBroker_WaitForInput_Remember 验证 Remember 字段正确传递（用于权限请求）
func TestHITLBroker_WaitForInput_Remember(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)
	req := broker.RequestInput("task-1", "step-1", InputPermission, "允许执行脚本？", nil)

	go func() {
		time.Sleep(30 * time.Millisecond)
		broker.SubmitInput(InputResponse{
			RequestID: req.ID,
			TaskID:    "task-1",
			Action:    "approve",
			Remember:  true,
		})
	}()

	resp, err := broker.WaitForInput(context.Background(), "task-1", req)
	if err != nil {
		t.Fatalf("WaitForInput 失败: %v", err)
	}
	if !resp.Remember {
		t.Error("Remember 字段应为 true")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// WaitForInput 超时与取消
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_WaitForInput_Timeout 验证请求超时返回 CodeInputTimeout
func TestHITLBroker_WaitForInput_Timeout(t *testing.T) {
	// 使用很短的超时
	broker, _ := newTestBroker(t, 50*time.Millisecond)
	req := broker.RequestInput("task-1", "step-1", InputClarification, "请说明", nil)

	// 不提交任何响应，等待超时
	ctx := context.Background()
	_, err := broker.WaitForInput(ctx, "task-1", req)
	if err == nil {
		t.Fatal("期望超时错误，实际无错误")
	}
	if !errs.IsCode(err, errs.CodeInputTimeout) {
		t.Errorf("错误码期望 CodeInputTimeout，得到: %v", err)
	}
}

// TestHITLBroker_WaitForInput_ContextCanceled 验证 context 取消时正确返回错误
func TestHITLBroker_WaitForInput_ContextCanceled(t *testing.T) {
	broker, _ := newTestBroker(t, 10*time.Second)
	req := broker.RequestInput("task-1", "step-1", InputClarification, "请说明", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	_, err := broker.WaitForInput(ctx, "task-1", req)
	if err == nil {
		t.Fatal("期望 context 超时错误，实际无错误")
	}
	// context.DeadlineExceeded 或 context.Canceled 均可接受
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("期望 context 错误，得到: %v", err)
	}
}

// TestHITLBroker_WaitForInput_StopCh 验证关闭 stopCh 时返回 CodeCanceled
func TestHITLBroker_WaitForInput_StopCh(t *testing.T) {
	broker, stopCh := newTestBroker(t, 10*time.Second)
	req := broker.RequestInput("task-1", "step-1", InputClarification, "请说明", nil)

	// 短暂延迟后关闭 stopCh
	go func() {
		time.Sleep(30 * time.Millisecond)
		close(stopCh)
	}()

	_, err := broker.WaitForInput(context.Background(), "task-1", req)
	if err == nil {
		t.Fatal("期望 CodeCanceled 错误，实际无错误")
	}
	if !errs.IsCode(err, errs.CodeCanceled) {
		t.Errorf("错误码期望 CodeCanceled，得到: %v", err)
	}
}

// TestHITLBroker_WaitForInput_Timeout_CleansUp 超时后待处理项应被清除
func TestHITLBroker_WaitForInput_Timeout_CleansUp(t *testing.T) {
	broker, _ := newTestBroker(t, 50*time.Millisecond)
	req := broker.RequestInput("task-1", "step-1", InputApproval, "提示", nil)

	// 等待超时
	broker.WaitForInput(context.Background(), "task-1", req) //nolint:errcheck

	// 超时后 pending 应已清空
	pending := broker.PendingInputs("task-1")
	if len(pending) != 0 {
		t.Errorf("超时后 pending 应为空，得到 %d 项", len(pending))
	}
}

// ────────────────────────────────────────────────────────────────────────────
// WaitForInput_NoChannel 边界情况：找不到响应通道
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_WaitForInput_NoChannel 验证在没有响应通道时返回 CodeInternal
func TestHITLBroker_WaitForInput_NoChannel(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	// 构造一个未通过 RequestInput 注册的请求（没有对应的 respCh）
	req := &InputRequest{
		ID:     "orphan-id",
		TaskID: "task-1",
		Type:   InputApproval,
	}

	_, err := broker.WaitForInput(context.Background(), "task-1", req)
	if err == nil {
		t.Fatal("期望 CodeInternal 错误（无响应通道），实际无错误")
	}
	if !errs.IsCode(err, errs.CodeInternal) {
		t.Errorf("错误码期望 CodeInternal，得到: %v", err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// SendCommand / CmdCancel
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_SendCommand_InvalidType 非法命令类型应返回 CodeInputInvalid
func TestHITLBroker_SendCommand_InvalidType(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	err := broker.SendCommand(UserCommand{
		Type:   UserCommandType("destroy_everything"),
		TaskID: "task-1",
	})
	if err == nil {
		t.Fatal("期望 CodeInputInvalid，实际无错误")
	}
	if !errs.IsCode(err, errs.CodeInputInvalid) {
		t.Errorf("错误码期望 CodeInputInvalid，得到: %v", err)
	}
}

// TestHITLBroker_SendCommand_ValidTypes 合法命令类型应成功入队
func TestHITLBroker_SendCommand_ValidTypes(t *testing.T) {
	cmds := []UserCommandType{CmdPause, CmdResume, CmdCancel}

	for _, cmdType := range cmds {
		t.Run(string(cmdType), func(t *testing.T) {
			broker, _ := newTestBroker(t, 5*time.Second)
			err := broker.SendCommand(UserCommand{Type: cmdType, TaskID: "task-1"})
			if err != nil {
				t.Errorf("SendCommand(%q) 失败: %v", cmdType, err)
			}
		})
	}
}

// TestHITLBroker_WaitForInput_CancelCommand 发送 CmdCancel 命令应使等待者收到 CodeTaskCanceled
func TestHITLBroker_WaitForInput_CancelCommand(t *testing.T) {
	broker, _ := newTestBroker(t, 10*time.Second)
	req := broker.RequestInput("task-cancel", "step-1", InputApproval, "等待取消", nil)

	// 在后台发送 cancel 命令
	go func() {
		time.Sleep(40 * time.Millisecond)
		if err := broker.SendCommand(UserCommand{
			Type:   CmdCancel,
			TaskID: "task-cancel",
		}); err != nil {
			t.Errorf("SendCommand 失败: %v", err)
		}
	}()

	_, err := broker.WaitForInput(context.Background(), "task-cancel", req)
	if err == nil {
		t.Fatal("期望 CodeTaskCanceled 错误，实际无错误")
	}
	if !errs.IsCode(err, errs.CodeTaskCanceled) {
		t.Errorf("错误码期望 CodeTaskCanceled，得到: %v", err)
	}
}

// TestHITLBroker_WaitForInput_CancelCommand_WrongTaskID
// 发送 cancel 命令到不同 TaskID 时，等待者不应受到影响（命令应回流到通道）
func TestHITLBroker_WaitForInput_CancelCommand_WrongTaskID(t *testing.T) {
	broker, _ := newTestBroker(t, 200*time.Millisecond)
	req := broker.RequestInput("task-A", "step-1", InputApproval, "等待", nil)

	// 发送针对另一个任务的 cancel 命令
	go func() {
		time.Sleep(20 * time.Millisecond)
		broker.SendCommand(UserCommand{
			Type:   CmdCancel,
			TaskID: "task-B", // 不同任务
		})
	}()

	// task-A 的等待者最终应因超时而结束，不因他人的 cancel 命令而取消
	_, err := broker.WaitForInput(context.Background(), "task-A", req)
	if err == nil {
		t.Fatal("期望超时错误，实际无错误")
	}
	if !errs.IsCode(err, errs.CodeInputTimeout) {
		t.Errorf("期望超时（CodeInputTimeout），得到: %v", err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// RegisterPendingInput / UnregisterPendingInput
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_RegisterUnregister 验证手动注册和注销的行为
func TestHITLBroker_RegisterUnregister(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	req := &InputRequest{
		ID:     "manual-req-1",
		TaskID: "task-m",
		Type:   InputPermission,
	}
	respCh := make(chan InputResponse, 1)
	broker.RegisterPendingInput(req, respCh)

	// 注册后应可见
	pending := broker.PendingInputs("task-m")
	if len(pending) != 1 {
		t.Fatalf("注册后期望 1 项，得到 %d 项", len(pending))
	}

	broker.UnregisterPendingInput(req.ID)

	// 注销后应消失
	pending = broker.PendingInputs("task-m")
	if len(pending) != 0 {
		t.Errorf("注销后期望 0 项，得到 %d 项", len(pending))
	}
}

// TestHITLBroker_UnregisterNonexistent 注销不存在的 ID 应安全（不 panic）
func TestHITLBroker_UnregisterNonexistent(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)
	// 不应 panic
	broker.UnregisterPendingInput("does-not-exist")
}

// ────────────────────────────────────────────────────────────────────────────
// 并发安全测试
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_Concurrent_MultipleRequestsAndSubmits 多个请求并发等待和提交
func TestHITLBroker_Concurrent_MultipleRequestsAndSubmits(t *testing.T) {
	const numRequests = 20

	broker, _ := newTestBroker(t, 5*time.Second)

	// 预先创建所有请求
	reqs := make([]*InputRequest, numRequests)
	for i := 0; i < numRequests; i++ {
		reqs[i] = broker.RequestInput(
			"task-concurrent",
			"step",
			InputApproval,
			"并发测试提示",
			nil,
		)
	}

	var wg sync.WaitGroup
	results := make([]string, numRequests)

	// 每个请求启动一个等待者 goroutine
	for i, req := range reqs {
		wg.Add(1)
		go func(idx int, r *InputRequest) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			resp, err := broker.WaitForInput(ctx, "task-concurrent", r)
			if err != nil {
				results[idx] = "ERROR:" + err.Error()
				return
			}
			results[idx] = resp.Value
		}(i, req)
	}

	// 短暂延迟后为所有请求提交响应
	time.Sleep(30 * time.Millisecond)
	for i, req := range reqs {
		go func(idx int, r *InputRequest) {
			broker.SubmitInput(InputResponse{ //nolint:errcheck
				RequestID: r.ID,
				TaskID:    "task-concurrent",
				Action:    "approve",
				Value:     r.ID, // 使用 ID 作为期望值
			})
		}(i, req)
	}

	wg.Wait()

	// 验证每个等待者都收到了对应的响应
	for i, req := range reqs {
		if results[i] != req.ID {
			t.Errorf("请求 %d (ID=%s) 收到的值 = %q，期望 %q", i, req.ID, results[i], req.ID)
		}
	}
}

// TestHITLBroker_Race_SubmitVsTimeoutCleanup
// 这是 #9 修复的核心场景：SubmitInput 与 WaitForInput 超时清理的竞态。
// 在超时边界附近大量并发执行，确保不发生 panic 或死锁。
func TestHITLBroker_Race_SubmitVsTimeoutCleanup(t *testing.T) {
	// 使用极短超时触发竞态窗口
	const iterations = 200

	for i := 0; i < iterations; i++ {
		// 每次迭代使用独立的 broker，避免状态干扰
		broker, _ := newTestBroker(t, 5*time.Millisecond)
		req := broker.RequestInput("task-race", "step", InputApproval, "竞态测试", nil)

		var wg sync.WaitGroup
		wg.Add(1)

		// 等待者（会在约 5ms 后超时）
		go func() {
			defer wg.Done()
			broker.WaitForInput(context.Background(), "task-race", req) //nolint:errcheck
		}()

		// 提交者：在超时附近竞争提交
		go func() {
			// 刻意不加 sleep，制造最激烈的竞态
			broker.SubmitInput(InputResponse{ //nolint:errcheck
				RequestID: req.ID,
				TaskID:    "task-race",
				Action:    "approve",
			})
		}()

		wg.Wait()
	}
	// 测试通过 go test -race 即验证无竞态，无 panic 即验证无 channel 发送到已关闭通道
}

// TestHITLBroker_Race_ConcurrentSubmitsOneRequest
// 同一个请求被多个 goroutine 同时提交，只有一个应成功，其余得到 full-channel 或 not-pending 错误
func TestHITLBroker_Race_ConcurrentSubmitsOneRequest(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)
	req := broker.RequestInput("task-1", "step-1", InputApproval, "并发提交", nil)

	// 启动等待者
	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		broker.WaitForInput(context.Background(), "task-1", req) //nolint:errcheck
	}()

	// 10 个 goroutine 同时提交同一个请求
	const numSubmitters = 10
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < numSubmitters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := broker.SubmitInput(InputResponse{
				RequestID: req.ID,
				TaskID:    "task-1",
				Action:    "approve",
			})
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	<-waitDone

	// channel 容量为 1，最多只有 1 次 SubmitInput 能成功入队
	if successCount > 1 {
		t.Errorf("期望最多 1 次 SubmitInput 成功，实际 %d 次成功", successCount)
	}
}

// TestHITLBroker_Concurrent_PendingInputsWhileAdding 并发添加和读取 pending 列表
func TestHITLBroker_Concurrent_PendingInputsWhileAdding(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	var wg sync.WaitGroup
	const writers = 10
	const readers = 10

	// 并发写入（注册新请求）
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			broker.RequestInput("task-rw", "step", InputApproval, "写入", nil)
		}(i)
	}

	// 并发读取
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = broker.PendingInputs("task-rw")
		}()
	}

	wg.Wait()
}

// ────────────────────────────────────────────────────────────────────────────
// CreatePermissionPromptFn
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_CreatePermissionPromptFn_Approve 验证权限请求批准流程
func TestHITLBroker_CreatePermissionPromptFn_Approve(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)
	fn := broker.CreatePermissionPromptFn()

	// 在后台找到并审批权限请求
	go func() {
		// 等待请求被注册
		var found *InputRequest
		for {
			all := broker.PendingInputs("")
			if len(all) > 0 {
				found = all[0]
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		broker.SubmitInput(InputResponse{ //nolint:errcheck
			RequestID: found.ID,
			Action:    "approve",
			Remember:  true,
		})
	}()

	approved, remember, err := fn(context.Background(), "shell", "执行 shell 命令", nil)
	if err != nil {
		t.Fatalf("CreatePermissionPromptFn 返回错误: %v", err)
	}
	if !approved {
		t.Error("期望 approved = true")
	}
	if !remember {
		t.Error("期望 remember = true")
	}
}

// TestHITLBroker_CreatePermissionPromptFn_Reject 验证权限请求拒绝流程
// 注意：合法的拒绝动作为 "reject"（"deny" 不在合法 action 列表中）
func TestHITLBroker_CreatePermissionPromptFn_Reject(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)
	fn := broker.CreatePermissionPromptFn()

	go func() {
		var found *InputRequest
		for {
			all := broker.PendingInputs("")
			if len(all) > 0 {
				found = all[0]
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		// 使用合法的 "reject" action，"deny" 不在允许列表中
		if err := broker.SubmitInput(InputResponse{
			RequestID: found.ID,
			Action:    "reject",
			Remember:  false,
		}); err != nil {
			t.Errorf("SubmitInput(reject) 失败: %v", err)
		}
	}()

	approved, remember, err := fn(context.Background(), "rm", "删除文件", nil)
	if err != nil {
		t.Fatalf("CreatePermissionPromptFn 返回错误: %v", err)
	}
	// "reject" != "approve"，所以 approved 应为 false
	if approved {
		t.Error("期望 approved = false")
	}
	if remember {
		t.Error("期望 remember = false")
	}
}

// TestHITLBroker_CreatePermissionPromptFn_ContextCancel 验证 context 取消时权限请求正确返回
func TestHITLBroker_CreatePermissionPromptFn_ContextCancel(t *testing.T) {
	broker, _ := newTestBroker(t, 10*time.Second)
	fn := broker.CreatePermissionPromptFn()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	_, _, err := fn(ctx, "ls", "列出目录", nil)
	if err == nil {
		t.Fatal("期望 context 超时错误，实际无错误")
	}
}

// TestHITLBroker_CreatePermissionPromptFn_StopCh 验证停止信号时权限请求返回 CodeCanceled
func TestHITLBroker_CreatePermissionPromptFn_StopCh(t *testing.T) {
	broker, stopCh := newTestBroker(t, 10*time.Second)
	fn := broker.CreatePermissionPromptFn()

	go func() {
		time.Sleep(30 * time.Millisecond)
		close(stopCh)
	}()

	_, _, err := fn(context.Background(), "tool", "操作描述", nil)
	if err == nil {
		t.Fatal("期望 CodeCanceled 错误，实际无错误")
	}
	if !errs.IsCode(err, errs.CodeCanceled) {
		t.Errorf("错误码期望 CodeCanceled，得到: %v", err)
	}
}

// TestHITLBroker_CreatePermissionPromptFn_CleansUpAfterResponse
// 权限请求完成后，pending 列表中应不再存在该条目
func TestHITLBroker_CreatePermissionPromptFn_CleansUpAfterResponse(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)
	fn := broker.CreatePermissionPromptFn()

	var reqID string
	go func() {
		var found *InputRequest
		for {
			all := broker.PendingInputs("")
			if len(all) > 0 {
				found = all[0]
				reqID = found.ID
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		broker.SubmitInput(InputResponse{ //nolint:errcheck
			RequestID: found.ID,
			Action:    "approve",
		})
	}()

	fn(context.Background(), "tool", "提示", nil) //nolint:errcheck

	// 完成后该请求应已从 pending 中清除
	all := broker.PendingInputs("")
	for _, p := range all {
		if p.ID == reqID {
			t.Errorf("权限请求完成后仍存在于 pending 列表中: ID=%s", reqID)
		}
	}
}

// ────────────────────────────────────────────────────────────────────────────
// 边界情况：超时为 0 时使用默认值
// ────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_WaitForInput_ZeroTimeoutUsesDefault 当请求和配置均无超时设置时，使用默认超时
func TestHITLBroker_WaitForInput_ZeroTimeoutUsesDefault(t *testing.T) {
	// 配置超时为 0，应回退到 config.DefaultHITLInputTimeout（30 分钟）
	// 此测试仅验证代码路径不崩溃，使用 context 取消来结束等待
	broker, _ := newTestBroker(t, 0)

	req := broker.RequestInput("task-1", "step-1", InputApproval, "零超时测试", nil)
	// 手动清除请求的 Timeout，确保走默认分支
	req.Timeout = 0

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	_, err := broker.WaitForInput(ctx, "task-1", req)
	// 应因 context 超时而返回，而不是因为 InputTimeout 报错
	if err == nil {
		t.Fatal("期望 context 超时错误，实际无错误")
	}
	// 不应是 CodeInputTimeout（内部计时器），而应是 context 错误
	if errs.IsCode(err, errs.CodeInputTimeout) {
		t.Error("不应返回 CodeInputTimeout，应返回 context 超时错误（默认超时为 30 分钟）")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// fingerprint 去重 + fan-out 测试
// ─────────────────────────────────────────────────────────────────────────────

// TestHITLBroker_FingerprintDedup 验证：相同 fingerprint 的两个请求只广播一次，
// 但两个 caller 均收到响应（fan-out）。
func TestHITLBroker_FingerprintDedup(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	fp := "deadbeef12345678" // 伪造 fingerprint

	// 第一个请求 — 首次注册，isNew=true
	req1 := &InputRequest{
		ID:          "perm-1",
		TaskID:      "task-1",
		Type:        InputPermission,
		Fingerprint: fp,
	}
	ch1 := make(chan InputResponse, 1)
	isNew1 := broker.RegisterPendingInput(req1, ch1)
	if !isNew1 {
		t.Fatal("第一个请求应返回 isNew=true")
	}

	// 第二个请求 — 相同 fingerprint，isNew=false（去重）
	req2 := &InputRequest{
		ID:          "perm-2",
		TaskID:      "task-1",
		Type:        InputPermission,
		Fingerprint: fp,
	}
	ch2 := make(chan InputResponse, 1)
	isNew2 := broker.RegisterPendingInput(req2, ch2)
	if isNew2 {
		t.Fatal("相同 fingerprint 第二个请求应返回 isNew=false")
	}

	// 第二个 request 未注册到 pendingInput，PendingInputs 应只有一个
	pending := broker.PendingInputs("")
	if len(pending) != 1 {
		t.Fatalf("去重后应只有 1 个 pending 请求，得到 %d", len(pending))
	}
	if pending[0].ID != "perm-1" {
		t.Errorf("pending 请求应是 perm-1，得到 %s", pending[0].ID)
	}

	// Submit 对第一个请求的响应
	resp := InputResponse{RequestID: "perm-1", TaskID: "task-1", Action: "approve"}
	if err := broker.SubmitInput(resp); err != nil {
		t.Fatalf("SubmitInput failed: %v", err)
	}

	// 两个 caller 都应收到响应（fan-out）
	select {
	case r := <-ch1:
		if r.Action != "approve" {
			t.Errorf("ch1: expected approve, got %s", r.Action)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("ch1 未在 500ms 内收到响应")
	}

	select {
	case r := <-ch2:
		if r.Action != "approve" {
			t.Errorf("ch2: expected approve, got %s", r.Action)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("ch2（fan-out listener）未在 500ms 内收到响应")
	}

	// Submit 后 fingerprint 应已清理
	pending = broker.PendingInputs("")
	if len(pending) != 0 {
		t.Errorf("响应后应无 pending 请求，得到 %d", len(pending))
	}
}

// TestHITLBroker_FingerprintCleanupOnUnregister 验证：UnregisterPendingInput 清理 fingerprint 索引。
func TestHITLBroker_FingerprintCleanupOnUnregister(t *testing.T) {
	broker, _ := newTestBroker(t, 5*time.Second)

	fp := "aabbccdd11223344"
	req := &InputRequest{
		ID:          "perm-cleanup",
		TaskID:      "task-x",
		Type:        InputPermission,
		Fingerprint: fp,
	}
	ch := make(chan InputResponse, 1)
	broker.RegisterPendingInput(req, ch)
	broker.UnregisterPendingInput(req.ID)

	// 注销后，相同 fingerprint 应可再次注册（视为新请求）
	req2 := &InputRequest{
		ID:          "perm-cleanup-2",
		TaskID:      "task-x",
		Type:        InputPermission,
		Fingerprint: fp,
	}
	ch2 := make(chan InputResponse, 1)
	isNew := broker.RegisterPendingInput(req2, ch2)
	if !isNew {
		t.Error("注销后相同 fingerprint 应视为新请求（isNew=true）")
	}
	broker.UnregisterPendingInput(req2.ID)
}
