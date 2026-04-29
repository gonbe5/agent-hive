package feishu

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/master"
)

// failingProcessor 让 router.HandleMessage → ProcessMessage 链路必定返回错误，
// 触发 webhook handler 的"失败入 retry_queue"路径。
type failingProcessor struct {
	mu      sync.Mutex
	calls   int
	errOnce error
}

func (f *failingProcessor) ProcessMessage(_ context.Context, _ string, _ string) (master.TaskResponse, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return master.TaskResponse{}, f.errOnce
}

// noopPlugin 让 router 在调用 plugin.Send / NotifyError 时不报错；只是占位。
type noopPlugin struct{ platform channel.Platform }

func (p *noopPlugin) Platform() channel.Platform { return p.platform }
func (p *noopPlugin) Send(_ context.Context, _ channel.OutboundMessage) error {
	return nil
}
func (p *noopPlugin) WebhookHandler() http.HandlerFunc {
	return func(_ http.ResponseWriter, _ *http.Request) {}
}
func (p *noopPlugin) Verify(_ *http.Request) bool { return true }

// waitForLen 等到 q.Len() >= want 或超时返回当前长度。
func waitForLen(q *channel.MemoryRetryQueue, want int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := q.Len(); got >= want {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	return q.Len()
}

// TestWebhook_HandlerError_EnqueuesRetry 验证 P0-#7 主路径：
// router.HandleMessage 失败 → handler 仍 200 → retry_queue 被写入一条 RetryItem。
//
// 红队闭环：MessageID/Reason/ErrorMsg 都必须在 RetryItem 上可读，否则下游 reclaim/告警无法区分。
func TestWebhook_HandlerError_EnqueuesRetry(t *testing.T) {
	logger := zap.NewNop()
	proc := &failingProcessor{errOnce: errors.New("upstream LLM down")}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{
		Platform: channel.PlatformFeishu, ChatID: "oc_x", SessionID: "s",
	})

	q := channel.NewMemoryRetryQueue(0, logger)
	router.SetRetryQueue(q)

	h := NewWebhookHandler("", "", router, logger)
	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_type": "im.message.receive_v1", "event_id": "e1"},
		"event": map[string]any{
			"sender": map[string]any{"sender_id": map[string]any{"open_id": "ou_test"}},
			"message": map[string]any{
				"message_id":   "om_msg_p07",
				"chat_id":      "oc_x",
				"chat_type":    "p2p",
				"message_type": "text",
				"content":      `{"text":"hi"}`,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code >= 500 {
		t.Fatalf("handler MUST never return 5xx，got %d", rec.Code)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("handler should return 200，got %d body=%s", rec.Code, rec.Body.String())
	}

	got := waitForLen(q, 1, 3*time.Second)
	if got < 1 {
		t.Fatalf("retry_queue 应至少有 1 条入队，实际 len=%d", got)
	}
	snap := q.Snapshot()
	found := false
	for _, it := range snap {
		if it.MessageID == "om_msg_p07" && it.Reason == channel.RetryReasonHandlerError {
			if it.ErrorMsg == "" {
				t.Fatalf("retry item 必须带 error_msg，实际为空")
			}
			if it.Platform != string(channel.PlatformFeishu) {
				t.Fatalf("retry item platform 应为 feishu，实际 %q", it.Platform)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("没有找到 messageID=om_msg_p07 / reason=handler_error 的 retry item: %+v", snap)
	}
}

// TestWebhook_NilRouter_EnqueuesRetry 验证 router 未注入时（编排错误）：
// handler 仍 200；但因为 router=nil 直接拿不到 retry_queue，只能走 logger 兜底，
// 此用例从一个不同侧面"router 非 nil 但 retry_queue 未注入"做断言：handler 必须 200，且不 panic。
func TestWebhook_NilRetryQueue_StillReturns200(t *testing.T) {
	logger := zap.NewNop()
	proc := &failingProcessor{errOnce: errors.New("x")}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{
		Platform: channel.PlatformFeishu, ChatID: "oc_y", SessionID: "s",
	})
	// 故意不调 SetRetryQueue → router.RetryQueue() 返回 nil

	h := NewWebhookHandler("", "", router, logger)
	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_type": "im.message.receive_v1", "event_id": "e2"},
		"event": map[string]any{
			"sender": map[string]any{"sender_id": map[string]any{"open_id": "ou_y"}},
			"message": map[string]any{
				"message_id": "om_no_q", "chat_id": "oc_y", "chat_type": "p2p",
				"message_type": "text", "content": `{"text":"hi"}`,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handler must still return 200 with no retry_queue, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// panicProcessor 在 ProcessMessage 第一次调用时 panic，用于触发 webhook 异步 goroutine 的 recover。
type panicProcessor struct{}

func (panicProcessor) ProcessMessage(_ context.Context, _ string, _ string) (master.TaskResponse, error) {
	panic("simulated business panic")
}

// TestWebhook_HandlerPanic_RecoversAndEnqueues 验证 P0-#7 panic recover 路径：
// 业务 goroutine panic → recover → 入 retry_queue (reason=handler_panic)，并且不让 panic 冒到 SDK。
func TestWebhook_HandlerPanic_RecoversAndEnqueues(t *testing.T) {
	logger := zap.NewNop()
	router := channel.NewRouter(panicProcessor{}, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{
		Platform: channel.PlatformFeishu, ChatID: "oc_p", SessionID: "s",
	})
	q := channel.NewMemoryRetryQueue(0, logger)
	router.SetRetryQueue(q)

	h := NewWebhookHandler("", "", router, logger)
	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_type": "im.message.receive_v1", "event_id": "ep"},
		"event": map[string]any{
			"sender": map[string]any{"sender_id": map[string]any{"open_id": "ou_p"}},
			"message": map[string]any{
				"message_id": "om_panic", "chat_id": "oc_p", "chat_type": "p2p",
				"message_type": "text", "content": `{"text":"x"}`,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// 一旦 goroutine 内 panic 没被 recover，进程会 abort —— 走到 ServeHTTP 之外。
	// 此用例若退出到 t.Fatalf 之前没崩，配合 retry_queue 内有 handler_panic 条目即可证明 recover 生效。
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handler must return 200 even when business panics, got %d", rec.Code)
	}
	if got := waitForLen(q, 1, 3*time.Second); got < 1 {
		t.Fatalf("expected at least 1 retry item from panic, got %d", got)
	}
	snap := q.Snapshot()
	found := false
	for _, it := range snap {
		if it.Reason == channel.RetryReasonHandlerPanic && it.MessageID == "om_panic" {
			found = true
		}
	}
	if !found {
		t.Fatalf("没有找到 reason=handler_panic 的 retry item: %+v", snap)
	}
}

// TestWebhook_RouterNil_EnqueuesRetry — router=nil 是一个典型上线编排错误。
// handler 应返回 200，并且当上层独立提供了 retry_queue 时也能记录到（这里通过 router=nil 路径触发 logger 兜底，
// 我们额外验证它不 panic 即可；retry_queue 主入队覆盖见上一个用例）。
func TestWebhook_RouterNil_NeverReturns500(t *testing.T) {
	h := NewWebhookHandler("", "", nil, zap.NewNop())
	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_type": "im.message.receive_v1", "event_id": "en"},
		"event": map[string]any{
			"sender": map[string]any{"sender_id": map[string]any{"open_id": "ou"}},
			"message": map[string]any{
				"message_id": "om_nilrouter", "chat_id": "c", "chat_type": "p2p",
				"message_type": "text", "content": `{"text":"x"}`,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code >= 500 {
		t.Fatalf("router=nil 时也不允许 5xx，got %d", rec.Code)
	}
}

// countingProc 统计 ProcessMessage 调用次数。
type countingProc struct {
	mu    sync.Mutex
	calls int
}

func (c *countingProc) ProcessMessage(_ context.Context, _ string, _ string) (master.TaskResponse, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return master.TaskResponse{Content: "ok"}, nil
}

func (c *countingProc) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// waitForCount 等到 proc.count() >= want 或超时。
func waitForCount(proc *countingProc, want int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := proc.count(); got >= want {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	return proc.count()
}

// TestWebhook_EventClaimer_DeduplicatesReplay 验证 P0-#8 wiring：
// 同一 eventID 的重复 webhook 推送，第二次被 EventClaimer 拦截，processor 只被调用 1 次。
//
// 蓝军 mutation：删掉 ClaimEvent 调用 → processor 被调 2 次 → 此测试捕获。
func TestWebhook_EventClaimer_DeduplicatesReplay(t *testing.T) {
	logger := zap.NewNop()
	proc := &countingProc{}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{
		Platform: channel.PlatformFeishu, ChatID: "oc_claim", SessionID: "s",
	})

	q := channel.NewMemoryRetryQueue(0, logger)
	router.SetRetryQueue(q)

	claimer := master.NewMemoryEventClaimer(0, logger)
	router.SetEventClaimer(claimer)

	h := NewWebhookHandler("", "", router, logger)

	makeReq := func() *http.Request {
		body := mustJSON(map[string]any{
			"schema": "2.0",
			"header": map[string]any{
				"event_type": "im.message.receive_v1",
				"event_id":   "evt_dedup_001",
			},
			"event": map[string]any{
				"sender": map[string]any{"sender_id": map[string]any{"open_id": "ou_c"}},
				"message": map[string]any{
					"message_id": "om_claim_1", "chat_id": "oc_claim", "chat_type": "p2p",
					"message_type": "text", "content": `{"text":"hello"}`,
				},
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	// 第一次推送
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, makeReq())
	if rec1.Code != http.StatusOK {
		t.Fatalf("第一次推送应返回 200，got %d", rec1.Code)
	}

	// 等第一次处理完成
	if got := waitForCount(proc, 1, 3*time.Second); got < 1 {
		t.Fatalf("第一次推送后 processor 应被调用 1 次，got %d", got)
	}

	// 第二次推送（同一 eventID）
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, makeReq())
	if rec2.Code != http.StatusOK {
		t.Fatalf("第二次推送应返回 200，got %d", rec2.Code)
	}

	// 等一小段时间确认第二次没有触发 processor
	time.Sleep(200 * time.Millisecond)
	if got := proc.count(); got != 1 {
		t.Fatalf("P0-#8 不变量被破坏：同一 eventID 重复推送后 processor 被调用 %d 次（期望 1）", got)
	}

	// claimer 状态应为 Completed
	if state := claimer.State("evt_dedup_001"); state != master.ClaimStateCompleted {
		t.Fatalf("eventID 应为 Completed 状态，got %v", state)
	}
}

// TestWebhook_EventClaimer_NilClaimer_StillProcesses 验证 claimer 未注入时不影响正常处理。
func TestWebhook_EventClaimer_NilClaimer_StillProcesses(t *testing.T) {
	logger := zap.NewNop()
	proc := &countingProc{}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{
		Platform: channel.PlatformFeishu, ChatID: "oc_nc", SessionID: "s",
	})
	// 不注入 EventClaimer

	h := NewWebhookHandler("", "", router, logger)
	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_type": "im.message.receive_v1", "event_id": "evt_nc"},
		"event": map[string]any{
			"sender": map[string]any{"sender_id": map[string]any{"open_id": "ou_nc"}},
			"message": map[string]any{
				"message_id": "om_nc_1", "chat_id": "oc_nc", "chat_type": "p2p",
				"message_type": "text", "content": `{"text":"hi"}`,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("应返回 200，got %d", rec.Code)
	}
	if got := waitForCount(proc, 1, 3*time.Second); got < 1 {
		t.Fatalf("claimer 未注入时 processor 应正常被调用，got %d", got)
	}
}
