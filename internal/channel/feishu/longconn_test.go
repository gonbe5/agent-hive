package feishu

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"go.uber.org/zap"
)

// TestFeishuLongconn_NoDirectAck 源码扫描回归：IM 入站层（longconn + webhook）不得再私自调 AddReaction 做 ack。
// Section 6 要求将 ack 能力统一到 renderer 订阅 input_received 事件；
// 若后续 PR 在 longconn.go 或 webhook.go 重新引入 AddReaction 调用，此用例会失败，强制走 harness 事件流。
//
// D3 对称性约束：两端同构——任一端引入私有 ack 都会破坏 webhook/longconn 对称。
// 注释/字符串中出现 "AddReaction" 是允许的（文档交叉引用），只禁止真正的调用语法。
func TestFeishuLongconn_NoDirectAck(t *testing.T) {
	// 覆盖所有非方法调用语法：方法调用 (`.AddReaction(`)、裸函数调用 (行首或紧跟非标识符的 `AddReaction(`)。
	// 排除 `func AddReaction(` / `type ... AddReaction` 等定义语境——因为这些出现在接口/方法定义里，
	// 不是真正的"调用"，而 longconn/webhook 作为消费方不应出现任何这类定义。
	callPattern := regexp.MustCompile(`(?:^|[^.\w])AddReaction\s*\(`)

	for _, fname := range []string{"longconn.go", "webhook.go"} {
		data, err := os.ReadFile(fname)
		if err != nil {
			t.Fatalf("read %s: %v", fname, err)
		}
		for lineNo, raw := range strings.Split(string(data), "\n") {
			line := raw
			if idx := strings.Index(line, "//"); idx >= 0 {
				line = line[:idx]
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			if callPattern.MatchString(line) {
				t.Fatalf("%s:%d 发现 AddReaction 调用，违反 Section 6 约定（ack 必须由 renderer 订阅 input_received 触发）：%s",
					fname, lineNo+1, strings.TrimSpace(raw))
			}
		}
	}
}

func TestLongConnClient_MessageReceiveRefreshesLastEventAt(t *testing.T) {
	t.Parallel()

	client := NewLongConnClient("", "", nil, nil, zap.NewNop())
	before := time.Now().Add(-2 * time.Minute)
	client.setLastEventAt(before)

	event := &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{
				SenderId: &larkim.UserId{OpenId: stringPtr("ou_test")},
			},
			Message: &larkim.EventMessage{
				MessageId:   stringPtr("om_test_1"),
				ChatId:      stringPtr("oc_test_1"),
				ChatType:    stringPtr("p2p"),
				MessageType: stringPtr("text"),
				Content:     stringPtr(`{"text":"hello"}`),
			},
		},
	}

	if err := client.handleMessageReceive(context.Background(), event); err != nil {
		t.Fatalf("handleMessageReceive returned error: %v", err)
	}

	after := client.LastEventAt()
	if !after.After(before) {
		t.Fatalf("lastEventAt not refreshed, before=%v after=%v", before, after)
	}
}

func stringPtr(s string) *string {
	return &s
}
