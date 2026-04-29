package feishu

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/chef-guo/agents-hive/internal/imctx"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

// 卡片回调解码错误统一前缀，便于上游过滤/告警匹配。
var (
	ErrCardActionNilEvent     = errors.New("feishu: nil card action event")
	ErrCardActionMissingValue = errors.New("feishu: card action missing value")
	ErrCardActionMissingReqID = errors.New("feishu: card action missing request_id")
)

// SafeSenderID 是 P0-#12 PII 防御函数的 feishu 包内别名转发，实体已迁到
// internal/imctx/safe_sender.go（与 BuildSessionID 同级，stdlib-only leaf）。
//
// 保留此别名是为了不破坏既有 channel/feishu 内部调用点（card_decode、webhook、
// hitl_bridge_test）。新增调用点请直接 import internal/imctx 并使用
// imctx.SafeSenderID；CI gate scripts/ci/check_pii_safe_sender.sh 守卫
// raw open_id 不得直接进 logger / metric / error。
func SafeSenderID(rawID string) string {
	return imctx.SafeSenderID(rawID)
}

// decodeCardAction 把飞书 SDK 的 CardActionTriggerEvent 转成中立 imctx.CardAction。
//
// 不变量：
//   - 不写日志、不调用 master、不发起 IO；纯函数便于单测。
//   - 不抛 panic；任何 nil 子字段返回明确错误。
//   - rawSenderID 在此处立即转 SafeSenderID，禁止把 OpenID 透传给上层。
func decodeCardAction(ev *callback.CardActionTriggerEvent, now time.Time) (imctx.CardAction, error) {
	if ev == nil || ev.Event == nil {
		return imctx.CardAction{}, ErrCardActionNilEvent
	}
	req := ev.Event
	if req.Action == nil || req.Action.Value == nil {
		return imctx.CardAction{}, ErrCardActionMissingValue
	}

	tenantKey, openID := "", ""
	if req.Operator != nil {
		if req.Operator.TenantKey != nil {
			tenantKey = *req.Operator.TenantKey
		}
		openID = req.Operator.OpenID
	}
	openMessageID := ""
	if req.Context != nil {
		openMessageID = req.Context.OpenMessageID
	}

	requestID, _ := req.Action.Value["request_id"].(string)
	if requestID == "" {
		return imctx.CardAction{}, ErrCardActionMissingReqID
	}
	taskID, _ := req.Action.Value["task_id"].(string)
	action, _ := req.Action.Value["action"].(string)

	// 控件值优先级：input 类用 InputValue；button 类用 value["value"] 字段
	value := req.Action.InputValue
	if value == "" {
		if v, ok := req.Action.Value["value"].(string); ok {
			value = v
		}
	}

	rawValue, err := json.Marshal(req.Action.Value)
	if err != nil {
		// map[string]interface{} 在飞书生产路径上必可序列化；若失败说明上游 SDK 出现非预期类型，
		// 直接报错让 wrapper 落 retry_queue 而非吞掉。
		return imctx.CardAction{}, err
	}

	return imctx.CardAction{
		RequestID:        requestID,
		TaskID:           taskID,
		Action:           action,
		Value:            value,
		Tag:              imctx.CardActionTag(req.Action.Tag),
		RawValue:         rawValue,
		SafeOperatorID:   SafeSenderID(openID),
		TenantKey:        tenantKey,
		Platform:         imctx.PlatformFeishu,
		ChannelMessageID: openMessageID,
		ReceivedAt:       now,
	}, nil
}
