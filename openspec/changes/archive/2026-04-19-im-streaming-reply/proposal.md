## Why

IM 用户（飞书、钉钉、企微、微信）目前在每次对话中必须盯着静止的聊天界面等完整 LLM 响应返回——对于超过 5 秒的推理/工具调用任务，体感就是"机器人死了"。虽然底层 LLM 和 Master 已经有流式能力（`ChatWithToolsStream` + EventBus 实时广播到 WebSocket），但 `channel.MessageProcessor.ProcessMessage` 接口只抛出最终 `TaskResponse`，Router 在 `processMessageImpl` 里一次性 `plugin.Send`，所以 IM 链路是"完全非流式"。

另外，飞书长连接路径虽然已经有 GET 表情反馈（`longconn.go:215`），但 webhook 路径没有，之前的 Claude 终端声称"实现了"但只做了一半：流式压根没做，webhook 路径表情也没做。本次要做真正的闭环。

## What Changes

- **新增** `channel.StreamingPlugin` 可选接口：`SendStream(ctx, sessionID, streamCh) error`，支持增量卡片/消息更新。
- **扩展** `channel.MessageProcessor`：新增 `ProcessMessageStream(ctx, sessionID, input, chunkCh) (TaskResponse, error)`，供 Router 消费 chunk 流。
- **修改** `channel.Router.processMessageImpl`：若当前 plugin 实现 `StreamingPlugin`，走 chunk channel 路径；否则走现有一次性 `Send` 路径（保持向后兼容）。
- **新增** Master `ProcessMessageStream` 实现：订阅 EventBus 里 `message` 事件（`partial: true`），节流转发 chunk 到 channel 层。
- **新增** 飞书流式实现：
  - 首次 chunk 到达时创建"流式卡片"（飞书 `patch_message` / 卡片更新 API）。
  - 后续 chunk 以 ≤3fps（300ms）节流 PATCH 同一张卡片。
  - 完成时发送最终态卡片（标题/状态切换为"完成"）。
  - 失败时打红叉状态卡片。
- **新增** 飞书 webhook 路径的 `Get`/`Typing` 表情反馈（飞书 reactions API `emoji_type`，CamelCase），与 longconn 路径对齐。允许配置 `feishu.ack_emoji`（默认 `Get`，可选 `Typing` / `none`）。注：老仓库曾误写全大写 `GET`/`KEYBOARD` 导致 `code=231001 reaction type is invalid`，本 change 的 `Normalize` 静默迁移到正确值。
- **新增** 钉钉/企微的流式实现占位（分阶段：本次先飞书完整实现；钉钉/企微/微信先落"接口预留"，不走 stream 分支）。
- **配置** `config.Feishu` 增加 `streaming.enabled` / `streaming.throttle_ms` / `ack_emoji` 三个字段，默认开启。

本次不改变：Webhook/长连接签名验证、Router 去重/debounce、Master 会话状态管理。

## Capabilities

### New Capabilities

- `im-streaming-reply`: 定义 IM 通道如何将 LLM 流式输出增量呈现给用户（含表情回执、卡片增量更新、节流、失败回退到非流式）。

### Modified Capabilities

<!-- 目前 openspec/specs/ 为空，没有已有 capability 需要修改 -->

## Impact

- **代码**:
  - `internal/channel/plugin.go` + `types.go`（新增 `StreamingPlugin` 接口、`StreamChunk` 类型）
  - `internal/channel/router.go`（分支路由 streaming/非 streaming）
  - `internal/master/public_api.go`（新增 `ProcessMessageStream`）
  - `internal/master/session_manager.go`（chunk channel 桥接 EventBus）
  - `internal/channel/feishu/client.go`（新增 `PatchCard` / `CreateStreamCard`）
  - `internal/channel/feishu/plugin.go`（实现 `SendStream`）
  - `internal/channel/feishu/webhook.go`（加 `AddReaction` 调用）
  - `internal/config/config.go`（`FeishuConfig.Streaming` 字段）
- **API**: 对外 HTTP API 不变；新接口为内部 Go interface。
- **依赖**: 使用现有 `larkim` SDK 的 `PatchMessage`，不引入新依赖。
- **向后兼容**: 未实现 `StreamingPlugin` 的插件（钉钉/企微/微信）保持旧路径；配置关闭时 Feishu 也回退旧路径。
- **风险**:
  - 飞书卡片更新 API 频控（默认节流 300ms 规避）
  - EventBus 已有 50ms 节流（`react_processor.go:294`）；叠加 Router 侧节流时需确保最终一次 chunk 一定发出（完成事件触发 flush）。
