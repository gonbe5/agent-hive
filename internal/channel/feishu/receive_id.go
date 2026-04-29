package feishu

import "strings"

// inferReceiveIDType 按 receiveID 前缀推断飞书 IM CreateMessage / ReplyMessage
// 的 receive_id_type 参数。
//
// Phase 6 缺口 12 修复:push.Service 在只有 OpenID 时之前拼 "p2p:" 前缀传给
// SendMessage,但 SDK 写死 ReceiveIdType("chat_id") → 飞书必拒。
// 现在让 SendMessage 用本函数自动按前缀切 receive_id_type:
//
//   - oc_xxx → chat_id   (group / p2p chat)
//   - ou_xxx → open_id   (per-app user 标识,push P2P 主路径)
//   - on_xxx → user_id   (lark international tenant user_id)
//   - 含 @  → email      (兜底,不常用)
//   - 其他   → chat_id   (默认 fallback,兼容老调用)
//
// 注意:p2p:OpenID 这种历史前缀已经在 push.Service 端去掉,这里若仍收到就剥前缀。
func inferReceiveIDType(receiveID string) (idType, normalizedID string) {
	id := strings.TrimPrefix(receiveID, "p2p:")
	switch {
	case strings.HasPrefix(id, "oc_"):
		return "chat_id", id
	case strings.HasPrefix(id, "ou_"):
		return "open_id", id
	case strings.HasPrefix(id, "on_"):
		return "user_id", id
	case strings.Contains(id, "@"):
		return "email", id
	default:
		return "chat_id", id
	}
}
