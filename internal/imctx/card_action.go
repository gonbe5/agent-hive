package imctx

import (
	"encoding/json"
	"time"
)

// CardActionTag 是飞书互动卡片回调中 action.tag 的中立常量。
// 飞书 SDK 把 tag 直接以字符串透传；这里只覆盖 P0 HITL 必需的子集，
// 其它（select_static、picker_*、date_picker 等）按需追加。
type CardActionTag string

const (
	CardActionTagButton    CardActionTag = "button"
	CardActionTagInput     CardActionTag = "input"
	CardActionTagOverflow  CardActionTag = "overflow"
	CardActionTagCheckbox  CardActionTag = "checkbox"
)

// CardAction 是 IM 卡片回调的中立载体。Phase 0 P0-#2 HITL 桥接必须经过此结构：
//
//   feishu.dispatcher → imctx.CardAction → master.HITLBroker.SubmitInput(InputResponse)
//
// 字段语义：
//   - RequestID: 卡片渲染时写入 action.value["request_id"]，对应 master.InputRequest.ID。
//     无 RequestID 的回调（用户点击非 HITL 卡片）应在桥接层早期返回。
//   - TaskID: 卡片渲染时写入 action.value["task_id"]，回填 InputResponse.TaskID 用于审计。
//   - Action: HITL 语义动作（"approve"/"reject"/"modify"/"proceed"/"skip"/"cancel"），
//     必须在 master/hitl.go 已定义的集合内，未知值由桥接层拒绝。
//   - Value: 自由文本（input tag 的内容；button tag 通常为空）。
//   - Tag: 控件类型，桥接层用来校验 Value 是否合法（input 必须非空，button 必须空）。
//   - RawValue: 完整 action.value JSON，便于审计与未来字段扩展，桥接层不应解析它做控制流。
//   - SafeOperatorID: 触发回调用户的 sha256[:4]，与 IMMessageContext.SafeSenderID 同口径。
//   - TenantKey/Platform/ChannelMessageID: 用于 BuildSessionID 与定位原会话。
//   - ReceivedAt: 通道收到 callback 的时间，用于超时/审计。
type CardAction struct {
	RequestID        string          `json:"request_id"`
	TaskID           string          `json:"task_id,omitempty"`
	Action           string          `json:"action"`
	Value            string          `json:"value,omitempty"`
	Tag              CardActionTag   `json:"tag"`
	RawValue         json.RawMessage `json:"raw_value,omitempty"`
	SafeOperatorID   string          `json:"safe_operator_id"`
	TenantKey        string          `json:"tenant_key"`
	Platform         Platform        `json:"platform"`
	ChannelMessageID string          `json:"channel_message_id,omitempty"` // 卡片所在的消息 ID（用于更新卡片）
	ReceivedAt       time.Time       `json:"received_at"`
}
