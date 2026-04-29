// Package master 实现了 agents-hive 的 Master Agent 核心。
//
// Master Agent 是系统的中央编排器，负责：
// 用户任务入口与路由、固定/动态 Agent 协调、会话生命周期管理、
// 权限与 HITL 安全审批、事件广播与上下文压缩，以及 direct-exec 兜底路径。
//
// 核心类型：Master（编排器）、SessionManager（会话管理）、EventBus（事件广播）。
//
// # EventBus 与 channel-side renderer 订阅
//
// Master 的 EventBus 是多 subscriber 事件总线，消息处理链路上发射的事件
// （message_partial / message_final / tool_call_* / input_request / error）
// 由浏览器 WebSocket 与 IM channel 的 EventRenderer 并行订阅。
//
// 特别地，Master 在把用户消息委托给 SessionManager 之前会广播 `input_received`
// 事件（payload 含 `SessionID` + `ChannelMessageID`）——这是 channel-side ack
// 的统一触发点。IM 插件（如 feishu）订阅该事件后调平台 API（AddReaction 等）
// 贴即时"已受理"表情，废弃旧版"每个 plugin 自己在 webhook handler 里 ack"
// 的私有路径。新增 renderer 只需实现 channel.EventRenderer 契约即可接入，
// 不再需要 master 侧改动。
//
// feishuRenderer（`internal/channel/feishu/renderer.go`）是首个 subscriber 范本：
// - 订阅本 session 的事件流，累积状态到单张飞书卡片增量 PATCH；
// - 接收 InputReceivedEvent 触发 AddReaction；
// - 失败时返回 *RendererError{LastContent}，由 Router 兜底调 Plugin.Send。
package master
