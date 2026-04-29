// Package regression — session-scope-regression-matrix Phase 4.2
//
// 飞书 longconn mock stub：httptest.Server helper，覆盖 handshake + event push
// 两个 API 的最小形态。目的是让未来从 feishu → master → eventBus 驱动的 e2e
// 测试无需真实 LLM / 真实飞书 endpoint 就能跑。
//
// 当前 Phase 1-3 的 regression 测试直接驱动 eventBus（不经 feishu 链路），所以
// 此 stub 暂不被任何 test 引用——它是为 Phase 4 CI workflow 后续扩展
// （e.g. 未来加 "feishu message → session isolation" e2e 用例）保留的接缝。
//
// 不启动 real larksuite SDK：这里只模拟 handshake JSON 应答 + event push HTTP
// callback 两个纯 HTTP 行为；真实 WS 连接层由 larksuite/oapi-sdk-go 承担，
// 测试用例自己选是否 stub 掉 `ws.NewClient`。
package regression

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// FeishuEventPush 模拟飞书推送到我们的 event callback endpoint 的 envelope。
// 字段跟随飞书 event v2 的最小子集（schema + header + event）。
type FeishuEventPush struct {
	Schema string                 `json:"schema"`
	Header map[string]interface{} `json:"header"`
	Event  map[string]interface{} `json:"event"`
}

// FeishuStub httptest.Server 包装，两个 handler：
//   - GET  /open-apis/im/v1/chats  → handshake dummy（返回 tenant token + bot info）
//   - POST /__stub/push             → 测试驱动；从 Push() 方法接收事件并暂存，供 e2e 用
type FeishuStub struct {
	server *httptest.Server
	mu     sync.Mutex
	pushed []FeishuEventPush
}

// NewFeishuStub 启动 stub httptest.Server，返回 helper。defer Close()。
func NewFeishuStub() *FeishuStub {
	s := &FeishuStub{}
	mux := http.NewServeMux()

	// handshake：飞书 tenant_access_token 接口最小 mock
	mux.HandleFunc("/open-apis/auth/v3/tenant_access_token/internal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":               0,
			"msg":                "ok",
			"tenant_access_token": "ci-fake-token",
			"expire":             7200,
		})
	})

	// event push 采集：测试用 Push() 写入，外部读 Snapshot()
	mux.HandleFunc("/__stub/push", func(w http.ResponseWriter, r *http.Request) {
		var pushed FeishuEventPush
		if err := json.NewDecoder(r.Body).Decode(&pushed); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.pushed = append(s.pushed, pushed)
		s.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})

	s.server = httptest.NewServer(mux)
	return s
}

// URL 返回 stub endpoint base（feishu SDK 可设 domain 指向这里）
func (s *FeishuStub) URL() string {
	return s.server.URL
}

// Push 由测试代码调用，模拟一次飞书事件推送到 stub 的 collector。
// 真正的 e2e 测试需把这条 event 再路由到 master（通常通过 channel.Router.Receive）。
func (s *FeishuStub) Push(schema string, header, event map[string]interface{}) {
	pushed := FeishuEventPush{Schema: schema, Header: header, Event: event}
	s.mu.Lock()
	s.pushed = append(s.pushed, pushed)
	s.mu.Unlock()
}

// Snapshot 返回截至目前已采集的所有 event push（拷贝）
func (s *FeishuStub) Snapshot() []FeishuEventPush {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]FeishuEventPush, len(s.pushed))
	copy(out, s.pushed)
	return out
}

// Close 关闭 httptest.Server
func (s *FeishuStub) Close() {
	s.server.Close()
}

// WaitPushed 轮询等待 pushed 数达到 n，超时返回 false
func (s *FeishuStub) WaitPushed(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		got := len(s.pushed)
		s.mu.Unlock()
		if got >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
