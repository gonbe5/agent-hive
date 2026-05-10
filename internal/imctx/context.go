package imctx

import "time"

// Platform 是 IM 平台标识，副本于 internal/channel.Platform 以避免反向依赖。
// 增减常量必须同步 internal/channel/types.go。
type Platform string

const (
	PlatformDingTalk  Platform = "dingtalk"
	PlatformFeishu    Platform = "feishu"
	PlatformWeCom     Platform = "wecom"
	PlatformWeChatBot Platform = "wechatbot"
)

// IMMessageContext 是从 IM 通道流向 master 的中立消息元数据。
//
// 设计准则：
//   - 字段全部为值类型（string / time.Time / bool / 简单 slice），便于跨 goroutine 拷贝。
//   - 不持有任何 *T 指针、不持有 context.Context、不持有 channel/master 的具体类型。
//   - SenderID 一律使用 SafeSenderID（sha256[:4]）；原始 open_id/union_id 仅在审计表落库。
//     违反此约束的写入路径将被 P0-#12 PII grep gate 拒绝。
type IMMessageContext struct {
	Platform         Platform  `json:"platform"`
	TenantKey        string    `json:"tenant_key"`         // 飞书 tenant_key / 钉钉 corpId / 企微 corp_id
	ChannelMessageID string    `json:"channel_message_id"` // 平台原 message_id（飞书 om_*）
	ChatID           string    `json:"chat_id"`            // 平台 chat 标识（open_chat_id 等）
	SafeSenderID     string    `json:"safe_sender_id"`     // sha256(rawSenderID)[:4]，禁止填原始 ID
	ReceivedAt       time.Time `json:"received_at"`        // 通道入口收到事件的本地时间
	EventID          string    `json:"event_id,omitempty"` // 飞书 header.event_id 等去重键
	TraceID          string    `json:"trace_id,omitempty"` // 跨 channel/master 串联日志/metric

	// M1 消息摄取扩展字段（零值兼容非飞书平台）
	References         []DocRef  `json:"references,omitempty"`           // 消息中引用的文档资源
	ParentMessageID    string    `json:"parent_message_id,omitempty"`    // 回复的父消息 ID
	ParentContent      string    `json:"parent_content,omitempty"`       // 父消息正文（Resolver 填充）
	Mentions           []Mention `json:"mentions,omitempty"`             // @ 的用户列表
	BotMentioned       bool      `json:"bot_mentioned,omitempty"`        // 是否 @ 了机器人
	SystemPromptPrefix string    `json:"system_prompt_prefix,omitempty"` // Resolver 构造的 prompt 前缀
}

// SessionID 在 imctx 之外按 BuildSessionID(tenantKey, chatID) 构造，
// 这里只暴露原料字段，不在叶子包做格式拼装，避免 hardcode 模板。
