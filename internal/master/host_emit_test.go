package master

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/config"
)

// captureBroadcast 订阅 EventBus 并返回第一个类型为 input_request 的 payload 副本。
// 用于测试中捕获 EmitInputRequest 的广播并据此响应 SubmitInput。
func captureBroadcast(t *testing.T, m *Master, done chan *InputRequest) {
	t.Helper()
	subID, ch := m.SubscribeWSBroadcast()
	t.Cleanup(func() { m.UnsubscribeWSBroadcast(subID) })
	go func() {
		for msg := range ch {
			if msg.Type != EventTypeInputRequest {
				continue
			}
			if req, ok := msg.Payload.(*InputRequest); ok {
				done <- req
				return
			}
		}
	}()
}

func TestEmitInputRequest_RejectUnregistered(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true, InputTimeout: time.Second})
	defer cancel()
	defer m.Stop()

	subID, ch := m.SubscribeWSBroadcast()
	defer m.UnsubscribeWSBroadcast(subID)

	req := InputRequest{TaskID: "t", Type: InputChoice, Prompt: "?", ChoiceType: "not_in_registry"}
	resp, err := m.EmitInputRequest(context.Background(), req)
	if !errors.Is(err, ErrUnregisteredChoiceType) {
		t.Fatalf("want ErrUnregisteredChoiceType, got err=%v resp=%v", err, resp)
	}
	// 确认未广播：100ms 内不应收到任何 input_request
	select {
	case msg := <-ch:
		if msg.Type == EventTypeInputRequest {
			t.Fatalf("MUST NOT broadcast when ChoiceType is unregistered, saw: %+v", msg)
		}
	case <-time.After(100 * time.Millisecond):
		// 预期路径
	}
}

func TestEmitInputRequest_EmptyChoiceTypeAllowed(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true, InputTimeout: time.Second})
	defer cancel()
	defer m.Stop()

	captured := make(chan *InputRequest, 1)
	captureBroadcast(t, m, captured)

	ctx, cancelEmit := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelEmit()

	errCh := make(chan error, 1)
	go func() {
		_, err := m.EmitInputRequest(ctx, InputRequest{TaskID: "t1", Type: InputChoice, Prompt: "ok?"})
		errCh <- err
	}()

	var reqID string
	select {
	case req := <-captured:
		reqID = req.ID
	case <-time.After(500 * time.Millisecond):
		t.Fatal("broadcast not received within 500ms")
	}
	if reqID == "" {
		t.Fatal("auto-fill ID MUST be non-empty")
	}

	if err := m.SubmitInput(InputResponse{RequestID: reqID, TaskID: "t1", Action: "approve"}); err != nil {
		t.Fatalf("SubmitInput: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("emit returned err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("emit did not return within 2s after response submitted")
	}
}

func TestEmitInputRequest_AutoFillID(t *testing.T) {
	m, cancel := setupHITLMaster(t, config.HITLConfig{Enabled: true, InputTimeout: time.Second})
	defer cancel()
	defer m.Stop()

	captured := make(chan *InputRequest, 1)
	captureBroadcast(t, m, captured)

	before := time.Now()
	go func() {
		_, _ = m.EmitInputRequest(context.Background(), InputRequest{TaskID: "t2", Type: InputChoice, Prompt: "?"})
	}()

	select {
	case req := <-captured:
		if req.ID == "" {
			t.Error("ID must be auto-filled")
		}
		if req.CreatedAt.Before(before) || req.CreatedAt.After(time.Now().Add(time.Second)) {
			t.Errorf("CreatedAt must be near time.Now(), got %v", req.CreatedAt)
		}
		// 让 emit 退出：posting 响应
		_ = m.SubmitInput(InputResponse{RequestID: req.ID, TaskID: "t2", Action: "approve"})
	case <-time.After(time.Second):
		t.Fatal("broadcast not captured")
	}
}

func TestEmitInputRequest_ContextCancel(t *testing.T) {
	m, stop := setupHITLMaster(t, config.HITLConfig{Enabled: true, InputTimeout: 10 * time.Second})
	defer stop()
	defer m.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := m.EmitInputRequest(ctx, InputRequest{TaskID: "tc", Type: InputChoice, Prompt: "?"})
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("EmitInputRequest did not return within 500ms after cancel")
	}
}

func TestEmitInputRequest_Timeout(t *testing.T) {
	m, stop := setupHITLMaster(t, config.HITLConfig{Enabled: true, InputTimeout: time.Hour})
	defer stop()
	defer m.Stop()

	// 显式短超时覆盖 HITL 默认值
	errCh := make(chan error, 1)
	go func() {
		_, err := m.EmitInputRequest(
			context.Background(),
			InputRequest{TaskID: "tt", Type: InputChoice, Prompt: "?"},
			EmitInputRequestOptions{Timeout: 100 * time.Millisecond},
		)
		errCh <- err
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrInputRequestTimeout) {
			t.Fatalf("want ErrInputRequestTimeout, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("emit did not timeout within 1s")
	}
}

// TestBroadcastInputRequest_WithChoiceType 验证 ChoiceType 字段经由广播通道忠实传递到订阅端。
func TestBroadcastInputRequest_WithChoiceType(t *testing.T) {
	m, stop := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer stop()
	defer m.Stop()

	subID, ch := m.SubscribeWSBroadcast()
	defer m.UnsubscribeWSBroadcast(subID)

	m.BroadcastInputRequest(&InputRequest{
		ID:         "r-with-ct",
		TaskID:     "t",
		Type:       InputChoice,
		ChoiceType: "account_selector",
	})
	select {
	case msg := <-ch:
		if msg.Type != EventTypeInputRequest {
			t.Fatalf("want input_request, got %q", msg.Type)
		}
		req, ok := msg.Payload.(*InputRequest)
		if !ok || req.ChoiceType != "account_selector" {
			t.Fatalf("ChoiceType not preserved, got payload=%+v", msg.Payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no broadcast received")
	}
}

// TestBroadcastInputRequest_RawBypassesRegistry 证明底层广播不做 ChoiceType 校验。
// 这是 Owner 边界：EmitInputRequest 走校验路径，BroadcastInputRequest 信任调用者。
func TestBroadcastInputRequest_RawBypassesRegistry(t *testing.T) {
	m, stop := setupHITLMaster(t, config.HITLConfig{Enabled: true})
	defer stop()
	defer m.Stop()

	subID, ch := m.SubscribeWSBroadcast()
	defer m.UnsubscribeWSBroadcast(subID)

	m.BroadcastInputRequest(&InputRequest{
		ID:         "r-raw",
		TaskID:     "t",
		Type:       InputChoice,
		ChoiceType: "never_registered_type",
	})
	select {
	case msg := <-ch:
		req, ok := msg.Payload.(*InputRequest)
		if !ok || req.ChoiceType != "never_registered_type" {
			t.Fatalf("raw broadcast should preserve unregistered ChoiceType, got %+v", msg.Payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("raw broadcast blocked (must not block)")
	}
}

// TestHITLClosedLoop_EmitToResponse 是 Section 9.1 指定的端到端集成测试：
// 一个 goroutine 通过 EmitInputRequest 发起，另一个 goroutine 提交合成 InputResponse，
// 断言 Emit 在有限时间内返回正确响应。与 HappyPath 同形但独立命名以被 -count=10 验收。
func TestHITLClosedLoop_EmitToResponse(t *testing.T) {
	m, stop := setupHITLMaster(t, config.HITLConfig{Enabled: true, InputTimeout: 5 * time.Second})
	defer stop()
	defer m.Stop()

	captured := make(chan *InputRequest, 1)
	captureBroadcast(t, m, captured)

	respCh := make(chan *InputResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := m.EmitInputRequest(context.Background(), InputRequest{
			TaskID:     "closed-loop",
			Type:       InputConfirmation,
			Prompt:     "continue?",
			ChoiceType: "confirmation_before_irreversible_business_action",
		})
		respCh <- resp
		errCh <- err
	}()

	req := <-captured
	go func() {
		_ = m.SubmitInput(InputResponse{RequestID: req.ID, TaskID: "closed-loop", Action: "proceed"})
	}()

	select {
	case resp := <-respCh:
		if resp == nil || resp.Action != "proceed" {
			t.Fatalf("unexpected resp=%+v", resp)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("closed-loop did not complete within 2s")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("emit err: %v", err)
	}
}

func TestEmitInputRequest_HappyPath(t *testing.T) {
	m, stop := setupHITLMaster(t, config.HITLConfig{Enabled: true, InputTimeout: time.Second})
	defer stop()
	defer m.Stop()

	captured := make(chan *InputRequest, 1)
	captureBroadcast(t, m, captured)

	respCh := make(chan *InputResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := m.EmitInputRequest(context.Background(), InputRequest{
			TaskID:     "th",
			Type:       InputChoice,
			Prompt:     "which?",
			ChoiceType: "account_selector",
		})
		respCh <- resp
		errCh <- err
	}()

	var reqID string
	select {
	case req := <-captured:
		reqID = req.ID
		if req.ChoiceType != "account_selector" {
			t.Errorf("ChoiceType lost in broadcast: %q", req.ChoiceType)
		}
	case <-time.After(time.Second):
		t.Fatal("broadcast not captured")
	}

	_ = m.SubmitInput(InputResponse{RequestID: reqID, TaskID: "th", Value: "acct-A", Action: "approve"})

	select {
	case resp := <-respCh:
		if resp == nil || resp.Value != "acct-A" || resp.Action != "approve" {
			t.Fatalf("unexpected resp: %+v", resp)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("emit did not return after SubmitInput")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("emit err: %v", err)
	}
}
