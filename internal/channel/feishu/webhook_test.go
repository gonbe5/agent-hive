package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/observability"
)

// TestWebhook_URLVerification 验证 SDK 路径下，url_verification 挑战仍然能正确回显。
// 这是飞书后台保存 webhook URL 时必须通过的握手。
func TestWebhook_URLVerification(t *testing.T) {
	h := NewWebhookHandler("", "", nil, zap.NewNop())

	body := `{"type":"url_verification","challenge":"abc-123"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("url_verification must return 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v body=%s", err, rec.Body.String())
	}
	if resp["challenge"] != "abc-123" {
		t.Fatalf("challenge mismatch: got %q", resp["challenge"])
	}
}

// TestWebhook_RejectsNonPOST 验证非 POST 请求 405。
func TestWebhook_RejectsNonPOST(t *testing.T) {
	h := NewWebhookHandler("", "", nil, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET must return 405, got %d", rec.Code)
	}
}

// TestWebhook_MessageReceiveDispatch 验证一个未加密的 im.message.receive_v1 事件
// 能走通 SDK dispatcher → handleMessageReceive → router.HandleMessage 异步路径。
//
// 不需要真 router：handleMessageReceive 已对 nil router 做兜底（仅 log，不 panic、不 5xx）。
// 这里断言：handler 返回 nil → SDK 写 200。
func TestWebhook_MessageReceiveReturns200(t *testing.T) {
	// verification token 留空 = 不校验，便于测试构造最小事件
	h := NewWebhookHandler("", "", nil, zap.NewNop())

	// 飞书 v2 事件信封：header.event_type = im.message.receive_v1
	// schema 参考 SDK service/im/v1/event.go 中的 P2MessageReceiveV1
	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{
			"event_id":    "evt-xyz",
			"token":       "",
			"create_time": "1700000000",
			"event_type":  "im.message.receive_v1",
			"tenant_key":  "tk-test",
			"app_id":      "cli_test",
		},
		"event": map[string]any{
			"sender": map[string]any{
				"sender_id": map[string]any{"open_id": "ou_test"},
			},
			"message": map[string]any{
				"message_id":   "om_msg_1",
				"chat_id":      "oc_chat_1",
				"chat_type":    "p2p",
				"message_type": "text",
				"content":      `{"text":"hello"}`,
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("message event must return 200 (handler returned nil), got %d body=%s",
			rec.Code, rec.Body.String())
	}
}

// TestWebhook_HITLCardActionDispatch 验证 webhook 路径下 card.action.trigger
// 也能路由到 HITLBridge.SubmitInput——这是 webhook / longconn 通路对称的关键。
func TestWebhook_HITLCardActionDispatch(t *testing.T) {
	sub := &fakeSubmitter{}
	bridge := NewFeishuHITLBridge(sub, zap.NewNop(), nil)

	h := NewWebhookHandler("", "", nil, zap.NewNop()).WithHITLBridge(bridge)

	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{
			"event_id":    "evt-card",
			"token":       "",
			"create_time": "1700000000",
			"event_type":  "card.action.trigger",
			"tenant_key":  "tk-test",
		},
		"event": map[string]any{
			"operator": map[string]any{
				"open_id":    "ou_clicker",
				"tenant_key": "tk-test",
			},
			"action": map[string]any{
				"tag": "button",
				"value": map[string]any{
					"request_id": "req-via-webhook",
					"action":     "approve",
					"task_id":    "t-1",
				},
			},
			"context": map[string]any{
				"open_message_id": "om_card_1",
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("card action must return 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(sub.calls) != 1 {
		t.Fatalf("expected 1 submit, got %d", len(sub.calls))
	}
	if sub.calls[0].RequestID != "req-via-webhook" || sub.calls[0].Action != "approve" {
		t.Fatalf("unexpected submit: %+v", sub.calls[0])
	}
}

// TestWebhook_SDKDispatcherShared 验证同一个 webhook 实例先收到 message 后收到 card action，
// 都能路由——确保 once.Do 只构造一次但同时支持两种 handler。
func TestWebhook_SDKDispatcherShared(t *testing.T) {
	sub := &fakeSubmitter{}
	bridge := NewFeishuHITLBridge(sub, zap.NewNop(), nil)
	h := NewWebhookHandler("", "", nil, zap.NewNop()).WithHITLBridge(bridge)

	post := func(body []byte) int {
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	msgBody := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_type": "im.message.receive_v1", "event_id": "e1"},
		"event": map[string]any{
			"message": map[string]any{"chat_id": "c", "chat_type": "p2p", "message_id": "m"},
		},
	})
	cardBody := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_type": "card.action.trigger", "event_id": "e2"},
		"event": map[string]any{
			"operator": map[string]any{"open_id": "ou", "tenant_key": "tk"},
			"action": map[string]any{
				"tag":   "button",
				"value": map[string]any{"request_id": "r", "action": "approve"},
			},
		},
	})

	if code := post(msgBody); code != http.StatusOK {
		t.Fatalf("message: want 200, got %d", code)
	}
	if code := post(cardBody); code != http.StatusOK {
		t.Fatalf("card: want 200, got %d", code)
	}
	if len(sub.calls) != 1 || sub.calls[0].RequestID != "r" {
		t.Fatalf("card not routed to bridge: %+v", sub.calls)
	}
}

// TestWebhook_SignatureMissing_Returns401 — P0-#5 不变量：
// EncryptKey 配置后，缺任一签名头的请求必须返回 401，禁止落到 SDK 的 500 回退路径
// （500 → 飞书无限重试 = 红队链 A）。
func TestWebhook_SignatureMissing_Returns401(t *testing.T) {
	writer := &webhookMetricCaptureWriter{}
	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"no headers", nil},
		{"only signature", map[string]string{headerLarkSignature: "abc"}},
		{"only timestamp", map[string]string{headerLarkTimestamp: "1700000000"}},
		{"only nonce", map[string]string{headerLarkNonce: "n"}},
		{"sig+ts but no nonce", map[string]string{
			headerLarkSignature: "abc", headerLarkTimestamp: "1700000000",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := NewWebhookHandler("tok", "encrypt-key-set", nil, zap.NewNop())
			h.SetMetricsWriter(writer)
			body := mustJSON(map[string]any{"type": "url_verification", "challenge": "x"})
			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
	metric := writer.find(MetricWebhookSecurityReject)
	if metric == nil {
		t.Fatalf("expected %s metric", MetricWebhookSecurityReject)
	}
	if got := metric.Labels["reason"]; got != "missing_signature_header" {
		t.Fatalf("metric reason = %v, want missing_signature_header", got)
	}
}

// TestWebhook_TimestampReplayWindow — P0-#6 不变量：
//
//	timestamp 解析失败、过期 >5min、未来 >5min 必须 401；
//	±5min 窗口内必须放行（即不再被本守卫拦截，可能被 SDK 后续拒绝，但不再是 401-from-our-gate）。
func TestWebhook_TimestampReplayWindow(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	writer := &webhookMetricCaptureWriter{}
	allHeaders := func(ts string) map[string]string {
		return map[string]string{
			headerLarkSignature: "stub-sig",
			headerLarkTimestamp: ts,
			headerLarkNonce:     "stub-nonce",
		}
	}
	post := func(headers map[string]string) int {
		h := NewWebhookHandler("tok", "encrypt-key-set", nil, zap.NewNop()).
			WithNowFunc(func() time.Time { return now })
		h.SetMetricsWriter(writer)
		body := mustJSON(map[string]any{"type": "url_verification", "challenge": "x"})
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	cases := []struct {
		name    string
		ts      string
		want401 bool
	}{
		{"unparsable", "not-a-number", true},
		{"negative", "-1", true},
		{"way past (1h ago)", strconv.FormatInt(now.Add(-1*time.Hour).Unix(), 10), true},
		{"just past edge (5m1s ago)", strconv.FormatInt(now.Add(-5*time.Minute-time.Second).Unix(), 10), true},
		{"way future (1h ahead)", strconv.FormatInt(now.Add(time.Hour).Unix(), 10), true},
		{"future edge (5m1s ahead)", strconv.FormatInt(now.Add(5*time.Minute+time.Second).Unix(), 10), true},
		{"in window (1m ago)", strconv.FormatInt(now.Add(-time.Minute).Unix(), 10), false},
		{"in window (now)", strconv.FormatInt(now.Unix(), 10), false},
		{"in window (3m ahead)", strconv.FormatInt(now.Add(3*time.Minute).Unix(), 10), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code := post(allHeaders(c.ts))
			if c.want401 && code != http.StatusUnauthorized {
				t.Fatalf("expected 401 for %q, got %d", c.name, code)
			}
			if !c.want401 && code == http.StatusUnauthorized {
				t.Fatalf("expected NOT 401 for %q (in-window), got 401", c.name)
			}
		})
	}
	metric := writer.findWithReason(MetricWebhookSecurityReject, "stale_or_invalid_timestamp")
	if metric == nil {
		t.Fatalf("expected %s metric with stale_or_invalid_timestamp reason", MetricWebhookSecurityReject)
	}
}

// TestWebhook_NoEncryptKey_SkipsSignatureGuard — 当 EncryptKey 留空（开发/测试场景），
// 不要求签名头。否则我们已有的 url_verification / message tests 会回归失败。
func TestWebhook_NoEncryptKey_SkipsSignatureGuard(t *testing.T) {
	h := NewWebhookHandler("", "", nil, zap.NewNop())
	body := mustJSON(map[string]any{"type": "url_verification", "challenge": "ok"})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("EncryptKey 留空时不应做签名头校验，got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWebhook_EventEncryptEnabled_RejectsPlaintextBody(t *testing.T) {
	writer := &webhookMetricCaptureWriter{}
	h := NewWebhookHandler("", "", nil, zap.NewNop()).WithEventEncryptEnabled(true)
	h.SetMetricsWriter(writer)
	body := mustJSON(map[string]any{"type": "url_verification", "challenge": "ok"})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("strict encrypt mode should reject plaintext body, got %d body=%s", rec.Code, rec.Body.String())
	}
	metric := writer.findWithReason(MetricWebhookSecurityReject, "plaintext_body_when_encrypt_required")
	if metric == nil {
		t.Fatalf("expected %s metric with plaintext_body_when_encrypt_required reason", MetricWebhookSecurityReject)
	}
}

func TestWebhook_EventEncryptEnabled_AllowsEncryptedEnvelopeToReachSDK(t *testing.T) {
	h := NewWebhookHandler("", "", nil, zap.NewNop()).WithEventEncryptEnabled(true)
	body := mustJSON(map[string]any{"encrypt": "ciphertext"})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("encrypted envelope should not be rejected by strict encrypt guard, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestWebhook_HandlerNeverReturns500_OnRouterPanic 校验未来 P0-#7 不变量的当下子集：
// 即使 router 为 nil（一种典型上线编排错误）handler 也不能返回 5xx 让飞书重试。
func TestWebhook_HandlerNeverReturns500_OnRouterPanic(t *testing.T) {
	h := NewWebhookHandler("", "", nil, zap.NewNop())
	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_type": "im.message.receive_v1", "event_id": "e"},
		"event": map[string]any{
			"message": map[string]any{"chat_id": "c", "chat_type": "p2p", "message_id": "m"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code >= 500 {
		t.Fatalf("handler MUST NOT return 5xx (would trigger Feishu retry storm), got %d body=%s",
			rec.Code, rec.Body.String())
	}
}

func TestWebhook_BotRemovedLifecycleDispatch(t *testing.T) {
	repo := &stubChatStateRepo{
		markEvictedRecord: &ChatStateRecord{
			SessionID: "sess-removed",
			State:     ChatStateEvicted,
		},
		markEvictedChanged: true,
	}
	terminator := &stubSessionTerminator{}
	h := NewWebhookHandler("", "", nil, zap.NewNop()).
		WithLifecycleHandler(NewLifecycleHandler(repo, terminator, nil, zap.NewNop()))

	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{
			"event_id":    "evt-bot-removed",
			"token":       "",
			"create_time": "1700000005",
			"event_type":  "im.chat.member.bot.deleted_v1",
			"tenant_key":  "tenant-removed",
		},
		"event": map[string]any{
			"chat_id": "chat-removed",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("bot removed must return 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(repo.markEvictedCalls) != 1 {
		t.Fatalf("expected 1 mark evicted call, got %d", len(repo.markEvictedCalls))
	}
	if len(terminator.calls) != 1 {
		t.Fatalf("expected 1 terminate call, got %d", len(terminator.calls))
	}
}

func TestWebhook_BotAddedLifecycleDispatch(t *testing.T) {
	repo := &stubChatStateRepo{
		markActiveRecord:  &ChatStateRecord{State: ChatStateActive},
		markActiveChanged: true,
	}
	welcome := &stubWelcomeSender{}
	h := NewWebhookHandler("", "", nil, zap.NewNop()).
		WithLifecycleHandler(NewLifecycleHandler(repo, &stubSessionTerminator{}, welcome, zap.NewNop()))

	body := mustJSON(map[string]any{
		"schema": "2.0",
		"header": map[string]any{
			"event_id":    "evt-bot-added",
			"token":       "",
			"create_time": "1700000006",
			"event_type":  "im.chat.member.bot.added_v1",
			"tenant_key":  "tenant-added",
		},
		"event": map[string]any{
			"chat_id": "chat-added",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("bot added must return 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(repo.markActiveCalls) != 1 {
		t.Fatalf("expected 1 mark active call, got %d", len(repo.markActiveCalls))
	}
	if len(welcome.calls) != 1 {
		t.Fatalf("expected 1 welcome call, got %d", len(welcome.calls))
	}
	if len(repo.setSessionIDCalls) != 0 {
		t.Fatalf("bot added must not eagerly create session, got %d set session calls", len(repo.setSessionIDCalls))
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// 静态断言：保证 fakeSubmitter（hitl_bridge_test.go 已定义）满足 InputSubmitter 接口
var _ InputSubmitter = (*fakeSubmitter)(nil)

// 防止未使用 import 被 lint 摘掉
var _ = master.InputResponse{}

type webhookMetricCaptureWriter struct {
	items []observability.Metric
}

func (w *webhookMetricCaptureWriter) Record(_ context.Context, metric observability.Metric) error {
	w.items = append(w.items, metric)
	return nil
}

func (w *webhookMetricCaptureWriter) find(name string) *observability.Metric {
	for i := range w.items {
		if w.items[i].Name == name {
			return &w.items[i]
		}
	}
	return nil
}

func (w *webhookMetricCaptureWriter) findWithReason(name, reason string) *observability.Metric {
	for i := range w.items {
		if w.items[i].Name == name && w.items[i].Labels["reason"] == reason {
			return &w.items[i]
		}
	}
	return nil
}
