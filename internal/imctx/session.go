package imctx

import (
	"errors"
	"strings"
)

// SessionIDPrefix 是所有 IM session_id 的稳定前缀。
// 完整格式：im-feishu-{tenantKey}-{chatID} / im-dingtalk-{corpId}-{chatID} 等。
//
// 此值是跨进程持久化键的一部分（journal、retry_queue、HITL InputRequest），
// 一旦发布禁止变更——任何修改将使历史会话失联。变更前必须有迁移脚本与 ROADMAP 决议。
const SessionIDPrefix = "im"

// ErrEmptySessionPart 当 BuildSessionID 收到空字符串时返回。
// 调用方禁止把空 tenant/chat 拼进 session_id——会污染 journal 主键。
var ErrEmptySessionPart = errors.New("imctx: empty platform/tenant/chat in BuildSessionID")

// BuildSessionID 是 IM session_id 的唯一构造入口。Phase 0 P0-#10 强制要求：
//   - channel/master/任何业务代码都必须经此函数；
//   - 禁止 fmt.Sprintf("im-feishu-%s-%s",...) 自拼；
//   - CI grep gate 在 internal/ 全树扫描 `"im-feishu-` 字面量出现，违者 fail。
//
// 任何包含 "-" 的 tenant/chat 都会被替换为 "_"，避免反解析时段数错位。
func BuildSessionID(platform Platform, tenantKey, chatID string) (string, error) {
	if platform == "" || tenantKey == "" || chatID == "" {
		return "", ErrEmptySessionPart
	}
	safe := func(s string) string {
		return strings.ReplaceAll(s, "-", "_")
	}
	var b strings.Builder
	b.Grow(len(SessionIDPrefix) + 1 + len(platform) + 1 + len(tenantKey) + 1 + len(chatID))
	b.WriteString(SessionIDPrefix)
	b.WriteByte('-')
	b.WriteString(string(platform))
	b.WriteByte('-')
	b.WriteString(safe(tenantKey))
	b.WriteByte('-')
	b.WriteString(safe(chatID))
	return b.String(), nil
}
