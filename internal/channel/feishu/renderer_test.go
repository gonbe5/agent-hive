package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/master"
)

// mockTransport 记录 renderer 对外的全部 API 调用，供断言。
// 任何字段支持测试端定制返回值（err/id），默认成功。
type mockTransport struct {
	mu sync.Mutex

	// 计数
	replyCount, sendCount, patchCount, reactCount atomic.Int64

	// 记录参数（按调用顺序）
	replyBodies []string
	sendBodies  []string
	patchBodies []string

	// 行为控制
	replyErr   error
	sendErr    error
	patchErr   error // 若非 nil，每次 PatchCard 都返回此错误
	patchErrN  int32 // 前 N 次 PatchCard 返回 patchErr，此后返回 nil（0 表示一直返回）
	reactErr   error
	createdMID string // 默认 "mid_created"
}

func newMockTransport() *mockTransport {
	return &mockTransport{createdMID: "mid_created"}
}

func (m *mockTransport) ReplyCard(_ context.Context, _, cardJSON string) (string, error) {
	m.replyCount.Add(1)
	m.mu.Lock()
	m.replyBodies = append(m.replyBodies, cardJSON)
	m.mu.Unlock()
	if m.replyErr != nil {
		return "", m.replyErr
	}
	return m.createdMID, nil
}

func (m *mockTransport) SendCard(_ context.Context, _, cardJSON string) (string, error) {
	m.sendCount.Add(1)
	m.mu.Lock()
	m.sendBodies = append(m.sendBodies, cardJSON)
	m.mu.Unlock()
	if m.sendErr != nil {
		return "", m.sendErr
	}
	return m.createdMID, nil
}

func (m *mockTransport) PatchCard(_ context.Context, _, cardJSON string) error {
	n := m.patchCount.Add(1)
	m.mu.Lock()
	m.patchBodies = append(m.patchBodies, cardJSON)
	m.mu.Unlock()
	if m.patchErr != nil {
		if m.patchErrN == 0 || int32(n) <= m.patchErrN {
			return m.patchErr
		}
	}
	return nil
}

func (m *mockTransport) AddReaction(_ context.Context, _, _ string) error {
	m.reactCount.Add(1)
	return m.reactErr
}

func (m *mockTransport) lastPatchBody() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.patchBodies) == 0 {
		return ""
	}
	return m.patchBodies[len(m.patchBodies)-1]
}

// lastCardBody 按时序返回最新发出的卡片：优先 patch，否则 reply，否则 send。
// HITL 用例里首轮触发 Reply、cancel 后可能再触发 Patch——这个 helper 屏蔽此差异。
func (m *mockTransport) lastCardBody() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n := len(m.patchBodies); n > 0 {
		return m.patchBodies[n-1]
	}
	if n := len(m.replyBodies); n > 0 {
		return m.replyBodies[n-1]
	}
	if n := len(m.sendBodies); n > 0 {
		return m.sendBodies[n-1]
	}
	return ""
}

// testRenderer 构造 renderer + 默认 scope + eventCh，并在 cleanup 里同步等待 run goroutine 退出。
func testRenderer(t *testing.T, transport cardTransport, ackEmoji string) (*feishuRenderer, channel.SessionScope, chan master.BroadcastMessage) {
	t.Helper()
	r := newFeishuRenderer(transport, zap.NewNop(), ackEmoji)
	// after hook：默认走真实 time.After 会让 1s 重试阻塞测试。把 backoff 调到近乎 0，
	// 保留真实 time 语义即可（现实 300ms 节流用 now() 判断，仍由测试时间驱动）。
	r.retryBackoff = 1 * time.Millisecond
	r.finalTimeout = 500 * time.Millisecond
	scope := channel.SessionScope{
		SessionID: "sess-1",
		ChatID:    "chat-1",
		ReplyToID: "user-msg-1",
		MessageID: "user-msg-1",
	}
	evCh := make(chan master.BroadcastMessage, 32)
	return r, scope, evCh
}

// runAsync 启动 renderer.run；返回一个 wait func 阻塞到 run 返回，并返回它的 error。
func runAsync(r *feishuRenderer, ctx context.Context, scope channel.SessionScope, evCh <-chan master.BroadcastMessage) func() error {
	done := make(chan error, 1)
	go func() { done <- r.run(ctx, scope, evCh) }()
	return func() error {
		select {
		case err := <-done:
			return err
		case <-time.After(4 * time.Second):
			return fmt.Errorf("run goroutine did not return within 4s")
		}
	}
}

// -----------------------------
// 10.6 ack 表情
// -----------------------------
func TestFeishuRenderer_HandlesInputReceived(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "Typing")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	evCh <- master.BroadcastMessage{
		Type: master.EventTypeInputReceived,
		Payload: master.InputReceivedEvent{
			SessionID:        "sess-1",
			ChannelMessageID: "user-msg-1",
		},
		SessionID: "sess-1",
	}
	// ack 是 fire-and-forget goroutine，给 50ms 让它跑完。
	time.Sleep(80 * time.Millisecond)
	cancel()
	_ = wait()

	if got := tr.reactCount.Load(); got != 1 {
		t.Errorf("AddReaction 调用次数 = %d，want 1", got)
	}
	if tr.replyCount.Load() != 0 || tr.sendCount.Load() != 0 || tr.patchCount.Load() != 0 {
		t.Errorf("input_received 不应触发卡片创建/PATCH，but reply=%d send=%d patch=%d",
			tr.replyCount.Load(), tr.sendCount.Load(), tr.patchCount.Load())
	}
}

// TestFeishuRenderer_SkipsAckWhenNone 回归 Section 7 MUST-FIX #1：
// ack_emoji == "none" 是用户显式禁用 ack 的 sentinel，renderer 必须跳过 AddReaction——
// 否则飞书 reactions API 会对字面量 "none" 返回 400。
func TestFeishuRenderer_SkipsAckWhenNone(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "none")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	evCh <- master.BroadcastMessage{
		Type: master.EventTypeInputReceived,
		Payload: master.InputReceivedEvent{
			SessionID:        "sess-1",
			ChannelMessageID: "user-msg-1",
		},
		SessionID: "sess-1",
	}
	time.Sleep(80 * time.Millisecond)
	cancel()
	_ = wait()

	if got := tr.reactCount.Load(); got != 0 {
		t.Errorf("AddReaction 应跳过，实际调用 = %d 次", got)
	}
}

// -----------------------------
// 10.7 节流：100ms 内 10 个 partial → create+patch ≤ 2
// -----------------------------
func TestFeishuRenderer_MessagePartialThrottle(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	// throttle 拉长到 1s 保证 100ms 内不会过窗口。
	r.throttle = 1 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 10 个 partial，每个间隔 10ms（总 ~100ms）
	for i := range 10 {
		evCh <- master.BroadcastMessage{
			Type: master.EventTypeMessage,
			Payload: map[string]any{
				"content": fmt.Sprintf("hi-%d", i),
				"partial": true,
			},
			SessionID: "sess-1",
		}
		time.Sleep(10 * time.Millisecond)
	}
	// final 强制 flush
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"content": "hi-final",
			"partial": false,
		},
		SessionID: "sess-1",
	}
	// 让事件消费完
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()

	totalWrites := tr.replyCount.Load() + tr.sendCount.Load() + tr.patchCount.Load()
	// 预期：第 1 个 partial → Reply（1）；其余 9 个被节流；final → Patch（1）；cancel 后 finalFlush 可能再 Patch 一次（2）
	if totalWrites > 3 {
		t.Fatalf("节流失效：create+patch 总数 = %d，期望 ≤ 3（含 cancel 末次 flush）", totalWrites)
	}
	// partial=false 必 flush：至少要看到 Patch ≥ 1 且最终内容含 "hi-final"
	if tr.patchCount.Load() < 1 {
		t.Fatalf("final partial=false 未触发 PATCH：patchCount=%d", tr.patchCount.Load())
	}
	final := tr.lastCardBody()
	if !strings.Contains(final, "hi-final") {
		t.Errorf("最后一次落地应包含 final 正文 hi-final，got=%s", final)
	}
	if !strings.Contains(final, "✅ 完成") {
		t.Errorf("final 态应切标题为 ✅ 完成，got=%s", final)
	}
}

// -----------------------------
// 10.8 tool_call：start/success/error 立即 PATCH
// -----------------------------
func TestFeishuRenderer_ToolCallSection(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 1 * time.Hour // 排除节流干扰：确认所有 patch 都源于 tool_call 的 immediate flush

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	evCh <- master.BroadcastMessage{
		Type:      master.EventTypeToolCall,
		Payload:   master.ToolCallEvent{ToolCallID: "tc1", ToolName: "bash", Status: "start"},
		SessionID: "sess-1",
	}
	evCh <- master.BroadcastMessage{
		Type:      master.EventTypeToolCall,
		Payload:   master.ToolCallEvent{ToolCallID: "tc1", ToolName: "bash", Status: "success", Duration: 230},
		SessionID: "sess-1",
	}
	evCh <- master.BroadcastMessage{
		Type:      master.EventTypeToolCall,
		Payload:   master.ToolCallEvent{ToolCallID: "tc2", ToolName: "edit", Status: "error", Error: "no such file"},
		SessionID: "sess-1",
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()

	// 第 1 次是 Reply（create），后 2 次是 Patch；finalFlush 可能再一次 Patch。
	if tr.replyCount.Load() != 1 {
		t.Errorf("首轮 ReplyCard 应调用 1 次，got %d", tr.replyCount.Load())
	}
	if got := tr.patchCount.Load(); got < 2 {
		t.Errorf("tool_call 每次变化应立即 PATCH：patchCount=%d, want ≥ 2", got)
	}

	// 检查最后一次卡片 JSON 里包含 bash + edit + 错误信息
	finalBody := tr.lastPatchBody()
	for _, want := range []string{"bash", "edit", "no such file"} {
		if !strings.Contains(finalBody, want) {
			t.Errorf("卡片 JSON 缺少 %q: %s", want, finalBody)
		}
	}
}

// -----------------------------
// 10.9 HITL 按钮
// -----------------------------
func TestFeishuRenderer_HITLButtons(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 1 * time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	evCh <- master.BroadcastMessage{
		Type: master.EventTypeInputRequest,
		Payload: &master.InputRequest{
			ID:     "req-xyz",
			Type:   master.InputApproval,
			Prompt: "allow bash?",
		},
		SessionID: "sess-1",
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()

	if tr.replyCount.Load() != 1 {
		t.Fatalf("HITL 应触发首轮 Reply，got reply=%d", tr.replyCount.Load())
	}

	body := tr.lastCardBody()
	if body == "" {
		t.Fatal("HITL 卡片从未落地（既无 Reply 也无 Patch）")
	}
	if !strings.Contains(body, "req-xyz") {
		t.Errorf("卡片 JSON 应包含 request_id=req-xyz，got %s", body)
	}
	if !strings.Contains(body, "approve") || !strings.Contains(body, "reject") {
		t.Errorf("卡片 JSON 应包含 approve + reject 按钮，got %s", body)
	}

	var card map[string]any
	if err := json.Unmarshal([]byte(body), &card); err != nil {
		t.Fatalf("卡片非合法 JSON：%v", err)
	}
	els, _ := card["elements"].([]any)
	var actionEl map[string]any
	for _, el := range els {
		m, _ := el.(map[string]any)
		if m["tag"] == "action" {
			actionEl = m
			break
		}
	}
	if actionEl == nil {
		t.Fatal("卡片 elements 内未找到 tag=action")
	}
	acts, _ := actionEl["actions"].([]any)
	if len(acts) != 2 {
		t.Fatalf("HITL 按钮数量 = %d，want 2", len(acts))
	}
	for _, a := range acts {
		aMap, _ := a.(map[string]any)
		val, _ := aMap["value"].(map[string]any)
		if val["request_id"] != "req-xyz" {
			t.Errorf("按钮 value.request_id=%v，want req-xyz", val["request_id"])
		}
	}
	// primary=approve / danger=reject 的类型映射由 card_builder_test.go 覆盖，这里只断 request_id。
}

// TestFeishuRenderer_ClarificationPromptAndOptions 锁定 X-2 回归：
// InputClarification / InputChoice 类型必须把 req.Prompt 和 req.Options 写进卡片正文，
// 不能只渲染 HITL 批准/拒绝按钮（老实现的 bug）。
func TestFeishuRenderer_ClarificationPromptAndOptions(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 1 * time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	evCh <- master.BroadcastMessage{
		Type: master.EventTypeInputRequest,
		Payload: &master.InputRequest{
			ID:      "input-1",
			Type:    master.InputClarification,
			Prompt:  "你想查哪里的天气？",
			Options: []string{"自动定位", "我给你具体地址", "全国大概情况"},
		},
		SessionID: "sess-1",
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()

	body := tr.lastCardBody()
	if body == "" {
		t.Fatal("clarification 卡片从未落地")
	}
	// 回归断言：prompt 文本必须在卡片里
	if !strings.Contains(body, "你想查哪里的天气") {
		t.Errorf("卡片正文必须包含 prompt 文本，got: %s", body)
	}
	// 回归断言：所有选项必须在卡片里
	for _, opt := range []string{"自动定位", "我给你具体地址", "全国大概情况"} {
		if !strings.Contains(body, opt) {
			t.Errorf("卡片正文缺失选项 %q，got: %s", opt, body)
		}
	}
	// 回归断言：clarification 类型不应该出现批准/拒绝按钮（老实现的 bug）
	if strings.Contains(body, "approve") || strings.Contains(body, "reject") {
		t.Errorf("clarification 类型不应渲染 approve/reject 按钮，got: %s", body)
	}
}

// -----------------------------
// 10.10 PATCH 失败 → RendererError
// -----------------------------
func TestFeishuRenderer_PatchFailReturnsRendererError(t *testing.T) {
	tr := newMockTransport()
	tr.patchErr = errors.New("boom")
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 0 // 所有 partial 都立即 Patch，方便逼出 err

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 第 1 条：触发 Reply（成功）
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"content": "round-1",
			"partial": true,
		},
		SessionID: "sess-1",
	}
	// 第 2 条：触发 PATCH（失败）
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"content": "round-2-latest",
			"partial": true,
		},
		SessionID: "sess-1",
	}

	err := wait()
	if err == nil {
		t.Fatal("预期 run 返回 *RendererError，got nil")
	}
	var re *channel.RendererError
	if !errors.As(err, &re) {
		t.Fatalf("预期 *RendererError，got %T: %v", err, err)
	}
	if re.LastContent != "round-2-latest" {
		t.Errorf("LastContent = %q, want %q（应为最后一次尝试的累积内容）", re.LastContent, "round-2-latest")
	}
	// PatchCard 应被重试，且不应出现第 3 次尝试：同一次 flushCard 合同=首次+1 次重试。
	// 契约收紧：MF-7 要求 == 2，不能只断言 >= 2（否则测试漏掉"不该有第 3 次"的回归）。
	if got := tr.patchCount.Load(); got != 2 {
		t.Errorf("patchCount = %d，预期恰好 2（首次 + 1 次重试；不应有第 3 次）", got)
	}
}

// -----------------------------
// 10.10+ 补：PatchCard 首次失败、重试成功 → 不应返回 error
// 覆盖 retry 成功分支（原测试只覆盖了持续失败分支）。
// -----------------------------
func TestFeishuRenderer_PatchRetrySucceeds(t *testing.T) {
	tr := newMockTransport()
	tr.patchErr = errors.New("transient")
	tr.patchErrN = 1 // 只失败第 1 次
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	evCh <- master.BroadcastMessage{
		Type:      master.EventTypeMessage,
		Payload:   map[string]any{"content": "first", "partial": true},
		SessionID: "sess-1",
	}
	evCh <- master.BroadcastMessage{
		Type:      master.EventTypeMessage,
		Payload:   map[string]any{"content": "second", "partial": true},
		SessionID: "sess-1",
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	err := wait()
	// run 因 ctx cancel 收敛；不应在半途因 RendererError 提前返回。
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("重试成功后不应返回 RendererError，got %v", err)
	}
}

// -----------------------------
// 10.10+ 补：跨 session 事件过滤
// 契约：ev.SessionID != "" 且 != scope.SessionID 必须跳过，不应触发任何 API 调用。
// -----------------------------
func TestFeishuRenderer_IgnoresOtherSessions(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 推 3 个属于其它 session 的事件 → 应全部丢弃。
	for i := range 3 {
		evCh <- master.BroadcastMessage{
			Type:      master.EventTypeMessage,
			Payload:   map[string]any{"content": fmt.Sprintf("noise-%d", i), "partial": true},
			SessionID: "sess-OTHER",
		}
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()

	if tr.replyCount.Load() != 0 || tr.sendCount.Load() != 0 || tr.patchCount.Load() != 0 {
		t.Errorf("跨 session 事件污染：reply=%d send=%d patch=%d",
			tr.replyCount.Load(), tr.sendCount.Load(), tr.patchCount.Load())
	}
}

// -----------------------------
// 10.10+ 补：tool-only 场景 PATCH 失败 → LastContent 非空（MF-4 回归）
// -----------------------------
func TestFeishuRenderer_ToolOnlyPatchFail_LastContentNonEmpty(t *testing.T) {
	tr := newMockTransport()
	tr.patchErr = errors.New("boom-tool-only")
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 只推 tool_call，不推 message。
	evCh <- master.BroadcastMessage{
		Type:      master.EventTypeToolCall,
		Payload:   master.ToolCallEvent{ToolCallID: "tc1", ToolName: "bash", Status: "start"},
		SessionID: "sess-1",
	}
	evCh <- master.BroadcastMessage{
		Type:      master.EventTypeToolCall,
		Payload:   master.ToolCallEvent{ToolCallID: "tc1", ToolName: "bash", Status: "error", Error: "permission denied"},
		SessionID: "sess-1",
	}

	err := wait()
	var re *channel.RendererError
	if !errors.As(err, &re) {
		t.Fatalf("预期 *RendererError，got %T: %v", err, err)
	}
	if re.LastContent == "" {
		t.Fatal("MF-4 回归：tool-only 场景 PATCH 失败时 LastContent 不应为空")
	}
	// 文案应包含工具名或错误，证明 fallback 快照生效。
	if !strings.Contains(re.LastContent, "bash") {
		t.Errorf("LastContent 应包含工具名 bash，got %q", re.LastContent)
	}
}

// -----------------------------
// 10.11 ctx cancel → 3s 内 finalFlush + return ctx.Err()
// -----------------------------
func TestFeishuRenderer_CtxCancel_FinalFlush(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 推 1 条 partial → renderer create + 开始节流
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"content": "interrupted halfway",
			"partial": true,
		},
		SessionID: "sess-1",
	}
	time.Sleep(30 * time.Millisecond)

	before := tr.patchCount.Load()
	cancel()

	start := time.Now()
	err := wait()
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("预期 run 返回 context.Canceled，got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("ctx cancel 后超过 3s 才 return（%v），违反 3s 收敛契约", elapsed)
	}
	// finalFlush 应尝试了一次 Patch（因为 messageID 已设、content 非空）
	if got := tr.patchCount.Load(); got <= before {
		t.Errorf("cancel 后 finalFlush 未触发 PATCH：before=%d, after=%d", before, got)
	}
}

// -----------------------------
// X-3 回归：空闲心跳
// -----------------------------
// TestFeishuRenderer_ThinkingHeartbeatDuringIdle：
// 创建卡片后如果 thinkingHeartbeat 时间内无新事件，应自动 PATCH 一次 "💭 思考中…" 指示。
// 防止用户把 LLM 推理期的静默误判为对话结束。
func TestFeishuRenderer_ThinkingHeartbeatDuringIdle(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 10 * time.Millisecond          // 允许首个 partial 立刻 PATCH
	r.thinkingHeartbeat = 40 * time.Millisecond // 心跳阈值缩短到 40ms

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 1) 推首个 partial → 触发 Reply 创建卡片 + 进入 Generating 态
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"content": "正在查询天气...",
			"partial": true,
		},
		SessionID: "sess-1",
	}

	// 给 renderer 处理事件 + 创建卡片
	time.Sleep(20 * time.Millisecond)
	if tr.replyCount.Load() < 1 {
		t.Fatalf("首轮 partial 未触发 ReplyCard 创建卡片: reply=%d", tr.replyCount.Load())
	}
	patchBefore := tr.patchCount.Load()

	// 2) 静默 120ms > 心跳阈值 40ms → 应触发至少一次心跳 PATCH
	time.Sleep(120 * time.Millisecond)

	cancel()
	_ = wait()

	patchAfter := tr.patchCount.Load()
	if patchAfter <= patchBefore {
		t.Fatalf("空闲超过心跳阈值后未触发 PATCH：before=%d after=%d（心跳机制失效，用户会误判卡片停滞）",
			patchBefore, patchAfter)
	}

	// 3) 任何一次 heartbeat 产生的 body 必须包含 "💭 思考中…" 文案
	//    注意：cancel 后 finalFlush 也会再 PATCH 一次，此时 status 已回退到 Generating，
	//    body 会是 "🤖 生成中…"；所以只要 patchBodies 里存在任意一条含 "思考中" 即可。
	var sawThinking bool
	for _, body := range tr.patchBodies {
		if strings.Contains(body, "思考中") {
			sawThinking = true
			break
		}
	}
	if !sawThinking {
		t.Errorf("空闲期 PATCH 卡片未含 💭 思考中 指示；patch 总数=%d", patchAfter)
	}
}

// TestFeishuRenderer_ThinkingHeartbeatSuppressedAfterDone：
// 一旦卡片切到终态（partial=false → Done），即使后续静默也不应再触发心跳 PATCH。
func TestFeishuRenderer_ThinkingHeartbeatSuppressedAfterDone(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 10 * time.Millisecond
	r.thinkingHeartbeat = 30 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 单个 final 直接完成
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"content": "查询完成，温度 25 度",
			"partial": false,
		},
		SessionID: "sess-1",
	}
	time.Sleep(20 * time.Millisecond)

	patchesAtDone := tr.patchCount.Load()
	replyAtDone := tr.replyCount.Load()

	// 空闲超心跳阈值 3 倍
	time.Sleep(120 * time.Millisecond)

	cancel()
	_ = wait()

	// Done 之后不应再有 PATCH（finalFlush 会再 PATCH 一次，但 content 无变化 + 状态已 Done，
	// 无心跳 Thinking 出现）
	if tr.replyCount.Load() != replyAtDone {
		t.Errorf("Done 后不应再 Reply：before=%d after=%d", replyAtDone, tr.replyCount.Load())
	}
	extraPatches := tr.patchCount.Load() - patchesAtDone
	// finalFlush 可能多 1 次 PATCH（cancel 路径），但不应有连续多次心跳 PATCH
	if extraPatches > 1 {
		t.Errorf("Done 后静默期仍触发心跳 PATCH %d 次，期望 ≤1（只允许 finalFlush）", extraPatches)
	}
	// 任何 Done 之后的 PATCH 都不应含思考中文案
	for _, body := range tr.patchBodies[patchesAtDone:] {
		if strings.Contains(body, "思考中") {
			t.Errorf("终态 Done 之后卡片被改成'思考中'：body=%s", body)
		}
	}
}

// TestFeishuRenderer_HeartbeatSuppressedDuringHITL：
// 卡片挂出 HITL 按钮等人回复时，心跳不应触发——"思考中"文案在等人决策时会误导用户
// 以为还是 agent 在忙，其实球在用户脚下。
func TestFeishuRenderer_HeartbeatSuppressedDuringHITL(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 10 * time.Millisecond
	r.thinkingHeartbeat = 30 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 推一个 approval 型 InputRequest → 创建卡片 + hitlButtons + awaitingInput=true
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeInputRequest,
		Payload: &master.InputRequest{
			ID:     "req-hitl-1",
			Type:   master.InputApproval,
			Prompt: "是否执行删库？",
		},
		SessionID: "sess-1",
	}
	time.Sleep(20 * time.Millisecond)

	// HITL 进入后的 PATCH 计数作为基线——之后若触发心跳会增长
	patchesAtHITL := tr.patchCount.Load()
	replyAtHITL := tr.replyCount.Load()
	if replyAtHITL < 1 {
		t.Fatalf("HITL 未创建卡片: replyCount=%d", replyAtHITL)
	}

	// 静默远超心跳阈值（30ms × 4 = 120ms）
	time.Sleep(120 * time.Millisecond)

	cancel()
	_ = wait()

	// HITL 期间不应有任何额外心跳 PATCH；finalFlush 最多 1 次
	extraPatches := tr.patchCount.Load() - patchesAtHITL
	if extraPatches > 1 {
		t.Errorf("HITL 等人期间触发了 %d 次额外 PATCH，期望 ≤1（仅 finalFlush）", extraPatches)
	}
	// 所有已发出的卡片都不应含"思考中"——等人时不能误导
	for _, body := range tr.patchBodies {
		if strings.Contains(body, "思考中") {
			t.Errorf("HITL 等人期间卡片被心跳改成'思考中'（误导用户）：body=%s", body)
		}
	}
	for _, body := range tr.replyBodies {
		if strings.Contains(body, "思考中") {
			t.Errorf("HITL Reply 卡片不应含'思考中'：body=%s", body)
		}
	}
}

// TestFeishuRenderer_HeartbeatPreservesToolCalls：
// 心跳 PATCH 只应改标题；body / 工具行 / HITL 按钮必须原样保留。
// 否则用户看到"思考中"时反而以为之前的工具执行记录消失了，体感比没心跳更糟。
func TestFeishuRenderer_HeartbeatPreservesToolCalls(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 10 * time.Millisecond
	r.thinkingHeartbeat = 40 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 先一个 partial 建卡 + 正文
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"content": "开始处理",
			"partial": true,
		},
		SessionID: "sess-1",
	}
	// 一个 tool_call success，卡片里应有 "bash" 工具行
	evCh <- master.BroadcastMessage{
		Type:      master.EventTypeToolCall,
		Payload:   master.ToolCallEvent{ToolCallID: "tc-preserve", ToolName: "bash", Status: "success", Duration: 120},
		SessionID: "sess-1",
	}
	time.Sleep(20 * time.Millisecond)
	patchesBeforeHeartbeat := tr.patchCount.Load()

	// 静默触发心跳
	time.Sleep(120 * time.Millisecond)

	cancel()
	_ = wait()

	// 找到第一个"思考中"PATCH——它是心跳 body
	var thinkingBody string
	for i, body := range tr.patchBodies {
		if int64(i) < patchesBeforeHeartbeat {
			continue
		}
		if strings.Contains(body, "思考中") {
			thinkingBody = body
			break
		}
	}
	if thinkingBody == "" {
		t.Fatalf("未捕获到心跳 PATCH（含'思考中'），patchBodies=%d", len(tr.patchBodies))
	}
	// 心跳 PATCH 必须保留既有正文 + 工具名
	for _, want := range []string{"开始处理", "bash"} {
		if !strings.Contains(thinkingBody, want) {
			t.Errorf("心跳 PATCH 丢失既有内容 %q：body=%s", want, thinkingBody)
		}
	}
}

// TestFeishuRenderer_HeartbeatPatchFailFallback：
// 心跳阶段 PatchCard 若持续失败，renderer 必须返回 *RendererError 且 LastContent 非空，
// 让 Router 可以用纯文本 Send 兜底——MF-4 契约。
func TestFeishuRenderer_HeartbeatPatchFailFallback(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")
	r.throttle = 10 * time.Millisecond
	r.thinkingHeartbeat = 40 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 首轮 partial 必须成功（Reply 建卡片，PatchCard 还没被调用）
	// 之后所有 PatchCard 都失败 → 心跳触发的那次 PATCH 会重试 1 次也败，返回 err
	tr.patchErr = errors.New("feishu patch 5xx")

	done := make(chan error, 1)
	go func() { done <- r.run(ctx, scope, evCh) }()

	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"content": "开始长时间推理",
			"partial": true,
		},
		SessionID: "sess-1",
	}
	// 给 renderer 时间：创建卡片 + 进入心跳等待
	// 首次 partial 的 flushCard 走 ReplyCard 成功（Reply 分支没注入错误），messageID 设好
	time.Sleep(20 * time.Millisecond)
	if tr.replyCount.Load() < 1 {
		t.Fatalf("首轮 partial 未建卡片：replyCount=%d", tr.replyCount.Load())
	}

	// 等心跳触发（40ms 阈值 + retry backoff 1ms + 重试一次失败 ≈ 45ms）
	var err error
	select {
	case err = <-done:
	case <-time.After(1 * time.Second):
		cancel()
		t.Fatal("心跳 PATCH 失败未让 run return，超 1s")
	}

	var rendererErr *channel.RendererError
	if !errors.As(err, &rendererErr) {
		t.Fatalf("期望 *channel.RendererError，got %T: %v", err, err)
	}
	if rendererErr.LastContent == "" {
		t.Errorf("RendererError.LastContent 为空——Router 无法文本兜底")
	}
	if !strings.Contains(rendererErr.LastContent, "开始长时间推理") {
		t.Errorf("LastContent 应含原始正文，got=%q", rendererErr.LastContent)
	}
}

// TestFeishuRenderer_UserEchoBuildsPlaceholderCard role=user 的回显应**立刻建一张占位卡**，
// 状态强制 Generating，**不写 user 文本**。
//
// 真实诉求：provider TTFB ~4.4s，若等 first assistant chunk 才建卡，用户在 4 秒里界面全空白。
// 占位卡给出"我看见了你的消息，正在生成"的即时反馈，TTFB 间隔再由心跳切"💭 思考中…"。
//
// 同时验证 role=tool 不建卡（tool 结果走 handleToolCall）+ 第一帧不能是终态"✅ 完成"。
func TestFeishuRenderer_UserEchoBuildsPlaceholderCard(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 1. user echo（partial 字段缺省）：应立即建占位卡
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"role":    "user",
			"content": "用户刚发的消息",
		},
		SessionID: "sess-1",
	}
	time.Sleep(30 * time.Millisecond)

	// 占位卡应该已建
	if tr.replyCount.Load() < 1 {
		t.Fatalf("user echo 应触发首次建卡，replyCount=%d", tr.replyCount.Load())
	}
	placeholder := tr.lastCardBody()
	// 关键断言：标题不能是"✅ 完成"（倒错回归保护）
	if strings.Contains(placeholder, "✅ 完成") {
		t.Errorf("user echo 不该触发终态，占位卡不该是✅完成：%s", placeholder)
	}
	if !strings.Contains(placeholder, "🤖 生成中…") {
		t.Errorf("占位卡标题应为🤖生成中…，got=%s", placeholder)
	}
	// 占位卡不该写 user 正文（飞书侧用户已看见自己消息）
	if strings.Contains(placeholder, "用户刚发的消息") {
		t.Errorf("占位卡不该回显 user 正文，got=%s", placeholder)
	}

	// 2. tool 回显：仍然 skip（tool 结果由 handleToolCall 渲染为工具行）
	beforeTool := tr.replyCount.Load() + tr.sendCount.Load() + tr.patchCount.Load()
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"role":    "tool",
			"content": "tool 返回内容",
			"partial": false,
		},
		SessionID: "sess-1",
	}
	time.Sleep(30 * time.Millisecond)
	afterTool := tr.replyCount.Load() + tr.sendCount.Load() + tr.patchCount.Load()
	if afterTool != beforeTool {
		t.Fatalf("tool result 不该触发卡片写入，writes diff=%d", afterTool-beforeTool)
	}

	// 3. assistant partial 抵达：PATCH 占位卡，写正文
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"role":    "assistant",
			"content": "assistant 真正回复",
			"partial": true,
		},
		SessionID: "sess-1",
	}
	time.Sleep(30 * time.Millisecond)
	cancel()
	_ = wait()

	body := tr.lastCardBody()
	if !strings.Contains(body, "assistant 真正回复") {
		t.Errorf("assistant partial 后正文应含 assistant 内容，got=%s", body)
	}
	if !strings.Contains(body, "🤖 生成中…") {
		t.Errorf("assistant partial 仍应为🤖生成中…，got=%s", body)
	}
}

// TestFeishuRenderer_ToolCallsPayloadKeepsGenerating 锁定 partial=false 误终态回归。
//
// 真实 bug：上游 react_processor.go:573 在 finish_reason=tool_calls 且 ToolCalls>0 时
// 算出 partial=false，handleMessage 据此切 CardStatusDone——飞书卡片标题瞬间显示"✅ 完成"，
// 而工具（如 question/HITL）才刚开始执行/等待用户回答。
//
// 已修：上游公式改为"携带 tool_calls 即 partial=true"；下游 handleMessage 防御性 guard
// 同样兜底——任何携带 tool_calls 的 payload 都视为中间态，不切 Done。
//
// 此测试只验证渲染端防御层：手工注入一条 partial=false + tool_calls 非空的 payload，
// 卡片标题必须停留在🤖生成中…，不能是✅完成。
func TestFeishuRenderer_ToolCallsPayloadKeepsGenerating(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	// 模拟 react_processor 在 finish_reason=tool_calls 时旧公式的产物：
	// partial=false 但 tool_calls 非空。renderer 必须识别为中间态。
	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			"role":    "assistant",
			"content": "我先问你几个问题",
			"partial": false, // 上游错标——guard 必须纠正
			"tool_calls": []any{
				map[string]any{"id": "tc-1", "name": "question", "arguments": "{}"},
			},
		},
		SessionID: "sess-1",
	}
	time.Sleep(30 * time.Millisecond)
	cancel()
	_ = wait()

	body := tr.lastCardBody()
	if strings.Contains(body, "✅ 完成") {
		t.Errorf("payload 携带 tool_calls 时不能切终态：%s", body)
	}
	if !strings.Contains(body, "🤖 生成中…") {
		t.Errorf("应保持🤖生成中…标题，got=%s", body)
	}
	if !strings.Contains(body, "我先问你几个问题") {
		t.Errorf("正文应含 assistant 内容，got=%s", body)
	}
}

// TestFeishuRenderer_EmptyRoleDefaultsToAssistant role 字段缺省（兼容旧 payload）时
// 应按 assistant 处理，避免破坏既有流。
func TestFeishuRenderer_EmptyRoleDefaultsToAssistant(t *testing.T) {
	tr := newMockTransport()
	r, scope, evCh := testRenderer(t, tr, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wait := runAsync(r, ctx, scope, evCh)

	evCh <- master.BroadcastMessage{
		Type: master.EventTypeMessage,
		Payload: map[string]any{
			// 无 role 字段
			"content": "legacy payload",
			"partial": true,
		},
		SessionID: "sess-1",
	}
	time.Sleep(30 * time.Millisecond)
	cancel()
	_ = wait()

	if tr.replyCount.Load() < 1 {
		t.Fatalf("role 缺省应走 assistant 路径，replyCount=%d", tr.replyCount.Load())
	}
}
