package channel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/master"
)

// hungBackend 模拟 dedup 后端故障：Check 永远阻塞直到 ctx.Done。
// 用于触发 router 的 200ms 短超时 → fail-closed 路径。
type hungBackend struct {
	calls atomic.Int64
}

func (h *hungBackend) Check(ctx context.Context, _ string) (bool, error) {
	h.calls.Add(1)
	<-ctx.Done()
	return false, ctx.Err()
}
func (h *hungBackend) Stop() {}

// errorBackend 模拟后端 RPC 立即失败（连接拒绝、500 等）。
type errorBackend struct{ calls atomic.Int64 }

func (e *errorBackend) Check(_ context.Context, _ string) (bool, error) {
	e.calls.Add(1)
	return false, errors.New("simulated backend RPC error")
}
func (e *errorBackend) Stop() {}

// countingProcessor 统计 ProcessMessage 调用次数（线程安全），用于断言"消息没被处理两次"。
type countingProcessor struct {
	mu       sync.Mutex
	called   int
	response master.TaskResponse
}

func (c *countingProcessor) ProcessMessage(_ context.Context, _ string, _ string) (master.TaskResponse, error) {
	c.mu.Lock()
	c.called++
	c.mu.Unlock()
	return c.response, nil
}
func (c *countingProcessor) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.called
}

// TestDedup_HungBackend_FailClosed 是 P0-#9 红队主路径：
//   - dedup 后端 hang，超时 50ms
//   - webhook 重试 2 次同样的 messageID
//   - processor 必须 0 次被调（fail-closed），并且 retry_queue 至少 2 条 RetryReasonDedupBackend
//
// 蓝军 mutation: 把 fail-closed 改成 fail-open（即 err 时返回 dup=false 继续处理） →
// 此测试会捕获到 processor 被调用 ≥1 次。
func TestDedup_HungBackend_FailClosed(t *testing.T) {
	logger := zap.NewNop()
	proc := &countingProcessor{response: master.TaskResponse{Content: "ok"}}
	router := NewRouter(proc, logger)
	router.RegisterPlugin(&mockPlugin{platform: PlatformFeishu})
	router.Bind(Binding{Platform: PlatformFeishu, ChatID: "c", SessionID: "s"})

	hb := &hungBackend{}
	router.SetDedupBackend(hb)
	router.SetDedupTimeout(50 * time.Millisecond) // 测试用短超时
	q := NewMemoryRetryQueue(0, logger)
	router.SetRetryQueue(q)

	// SenderID 为空 → 直接走 HandleMessage 顶层 dedup（不经 debouncer）
	msg := InboundMessage{
		MessageID: "om_dup_1", Platform: PlatformFeishu, ChatID: "c",
		ChatType: ChatDirect,
	}
	for i := 0; i < 2; i++ {
		if err := router.HandleMessage(context.Background(), msg); err != nil {
			t.Fatalf("HandleMessage[%d] err: %v", i, err)
		}
	}

	// processor 不能被调用——dedup fail-closed 必须把消息卡在门口
	if got := proc.calls(); got != 0 {
		t.Fatalf("fail-closed 不变量被破坏：processor 被调用了 %d 次（期望 0）", got)
	}
	if got := q.Len(); got < 2 {
		t.Fatalf("retry_queue 应至少 2 条 dedup_backend 入队，got %d", got)
	}
	for _, it := range q.Snapshot() {
		if it.Reason != RetryReasonDedupBackend {
			t.Fatalf("retry item reason 应为 dedup_backend，got %q (%+v)", it.Reason, it)
		}
	}
	if hb.calls.Load() < 1 {
		t.Fatalf("hung backend Check 应被调用至少 1 次（说明 router 真的去查了），got %d", hb.calls.Load())
	}
}

// TestDedup_ErrorBackend_FailClosed 后端报错（不是 hang）也必须 fail-closed。
func TestDedup_ErrorBackend_FailClosed(t *testing.T) {
	logger := zap.NewNop()
	proc := &countingProcessor{response: master.TaskResponse{Content: "ok"}}
	router := NewRouter(proc, logger)
	router.RegisterPlugin(&mockPlugin{platform: PlatformFeishu})
	router.Bind(Binding{Platform: PlatformFeishu, ChatID: "c", SessionID: "s"})

	router.SetDedupBackend(&errorBackend{})
	q := NewMemoryRetryQueue(0, logger)
	router.SetRetryQueue(q)

	msg := InboundMessage{MessageID: "om_err_1", Platform: PlatformFeishu, ChatID: "c", ChatType: ChatDirect}
	if err := router.HandleMessage(context.Background(), msg); err != nil {
		t.Fatalf("HandleMessage err: %v", err)
	}
	if got := proc.calls(); got != 0 {
		t.Fatalf("error backend 必须 fail-closed，processor 被调 %d 次", got)
	}
	if got := q.Len(); got < 1 {
		t.Fatalf("retry_queue 应有 1 条 dedup_backend，got %d", got)
	}
}

// TestDedup_HealthyBackend_NormalDedup 后端正常时：第一次 fresh、第二次 dup（不入 retry_queue）。
// 这是 fail-closed 不退化"happy path"的回归保险。
func TestDedup_HealthyBackend_NormalDedup(t *testing.T) {
	logger := zap.NewNop()
	proc := &countingProcessor{response: master.TaskResponse{Content: "ok"}}
	router := NewRouter(proc, logger)
	router.RegisterPlugin(&mockPlugin{platform: PlatformFeishu})
	router.Bind(Binding{Platform: PlatformFeishu, ChatID: "c", SessionID: "s"})

	q := NewMemoryRetryQueue(0, logger)
	router.SetRetryQueue(q)

	msg := InboundMessage{MessageID: "om_ok_1", Platform: PlatformFeishu, ChatID: "c", ChatType: ChatDirect}
	for i := 0; i < 2; i++ {
		if err := router.HandleMessage(context.Background(), msg); err != nil {
			t.Fatalf("HandleMessage[%d] err: %v", i, err)
		}
	}
	// 第一条 fresh → processor 被调 1 次；第二条 dup → 跳过；retry_queue 不该有 dedup_backend
	if got := proc.calls(); got != 1 {
		t.Fatalf("健康后端：processor 应被调 1 次，got %d", got)
	}
	for _, it := range q.Snapshot() {
		if it.Reason == RetryReasonDedupBackend {
			t.Fatalf("健康后端不应触发 dedup_backend retry，got %+v", it)
		}
	}
}
