## 1. Channel 层 EventRenderer 抽象

- [x] 1.1 在 `internal/channel/types.go` 新增 `SessionScope{SessionID, ChatID, ReplyToID, UserID, MessageID string}`。
- [x] 1.2 在 `internal/channel/types.go` 给 `SessionRequest`（或同类入参结构）追加 `ChannelMessageID string` 字段，供 plugin 把平台 message_id 透传给 master。
- [x] 1.3 在 `internal/channel/plugin.go` 新增 `EventRenderer` 可选接口：
  ```go
  type EventRenderer interface {
      ChannelPlugin
      RenderEventStream(ctx context.Context, scope SessionScope, eventCh <-chan master.BroadcastMessage) error
  }
  ```
- [x] 1.4 在 `internal/channel/plugin.go` 注释里写明 renderer 契约：subscriber 端按 `scope.SessionID` filter；ctx cancel 时必须 3s 内收敛并 return；error 路径必须把 lastFullContent 暴露给 Router（约定：err 类型 `*RendererError{Inner error; LastContent string}`）。
- [x] 1.5 在 `internal/channel/plugin.go` 定义 `RendererError` 类型与 helper `WrapRendererErr(err, content)`。

## 2. Router 改为 subscriber-based 编排

- [x] 2.1 在 `internal/channel/router.go` 的 `Router` 结构新增 `eventBus EventBusSubscriber` + `rendererEnabled func(Platform) bool` 字段。
- [x] 2.2 新增 `Router.SetEventBusSubscriber(s)` 与 `Router.SetRendererEnabled(fn)` setter，由 bootstrap 注入。
- [x] 2.3 修改 `processMessageImpl`：
  - plugin 实现 EventRenderer 且 `shouldUseRenderer(platform)==true` → 走 `processViaRenderer`（Subscribe → renderer goroutine → ProcessMessage → drain 200ms → Unsubscribe → wait renderer → *RendererError fallback 或日志）。
  - 其他情况 → `processViaLegacySend`，行为与改动前 bit-identical。
- [x] 2.4 renderer goroutine 加 `defer recover`，panic 封装为 `*RendererError{Inner: panic 占位, LastContent: ""}` 走兜底或日志分支。
- [x] 2.5 ProcessMessage 返回 error 时，若 renderer 也失败再调 `NotifyError`；renderer 正常收敛即认为 error 事件已被消费，不再重复发。
- [x] 2.6 EventBusSubscriber 接口在 1.x 阶段已抽到 `internal/channel/types.go`（`SubscribeWSBroadcast()` + `UnsubscribeWSBroadcast(uint64)`），Router 通过该 interface 持有 master，避免循环依赖。

## 3. Harness input_received 事件

- [x] 3.1 在 `internal/master/master.go` 事件类型常量段新增 `EventTypeInputReceived = "input_received"`。
- [x] 3.2 在 `internal/master/master.go` 新增 `InputReceivedEvent{SessionID string `json:"session_id"`; ChannelMessageID string `json:"channel_message_id,omitempty"`}` 类型。
- [x] 3.3 在 `internal/master/public_api.go` `ProcessMessageWithOptions` 入口（权限检查后、委托给 SessionManager 前）广播 `BroadcastMessage{Type: EventTypeInputReceived, SessionID: req.SessionID, Payload: InputReceivedEvent{SessionID, ChannelMessageID}}`。空 sessionID 跳过（非 IM 入口无订阅目标）。
- [x] 3.4 给 `master.SessionRequest` 追加 `ChannelMessageID string` 字段（与 1.2 合并实现），并在 `public_api.go` 新增 `WithChannelMessageID(id)` MessageOption 供 channel 层透传。
- [x] 3.5 单测：`TestMaster_InputReceivedBroadcast` + `TestMaster_InputReceivedBroadcast_EmptyChannelMessageID` + `TestMaster_InputReceivedBroadcast_EmptySessionIDSkipped`，断言事件在 sessionMgr 委托前触发、payload 透传正确、空 sessionID 不广播。

## 4. Feishu Client：PatchCard + 卡片 builder

- [x] 4.1 在 `internal/channel/feishu/client.go` 新增 `PatchCard(ctx context.Context, messageID string, cardJSON string) error`，调 `larkim.NewPatchMessageReqBuilder().MessageId(messageID).Body(...).Build()` → `client.Im.Message.Patch`。429 → 返回 `ErrPatchRateLimited` 哨兵错误。
- [x] 4.2 在 `internal/channel/feishu/` 新建 `card_builder.go`，提供 `BuildCardJSON(state CardState) string`：
  - 输入 `CardState{Title, Body, ToolLines []ToolLine, HITLButtons []HITLButton, Status CardStatus}`。
  - 标题随 status 切换："🤖 生成中…" / "✅ 完成" / "❌ 失败"。
  - 输出符合飞书 interactive card schema 的 JSON。
- [x] 4.3 单测 `card_builder_test.go`：覆盖三种 status、含 ToolLines 与 HITL buttons 的渲染。

## 5. Feishu EventRenderer 实现

- [x] 5.1 在 `internal/channel/feishu/renderer.go` 新建 `feishuRenderer` 类型（持有 `cardTransport`/logger/ackEmoji/throttle/finalTimeout/retryBackoff/showProgress）。为可测试性，client 依赖抽象为 `cardTransport` interface。
- [x] 5.2 实现 `func (p *Plugin) RenderEventStream(ctx, scope, eventCh) error`：
  - 内部 state：`rendererCardState{messageID, lastPatchAt, content, lastFullContent, toolCalls, toolCallOrder, hitlButtons, progress, status, terminalError}`。
  - 主循环 `select { ctx.Done / <-eventCh }`：先 session filter（空 SessionID 放行以兼容 BroadcastGenericMessage），再按 `ev.Type` 分派到 `handleInputReceived/handleMessage/handleToolCall/handleInputRequest/handleError/handleAgentProgress`。
  - 首个非 input_received 事件触发 `createCard`（ReplyCard → SendCard fallback），保存返回 messageID。
- [x] 5.3 `handleInputReceived(ev)`：ChannelMessageID 非空且 ackEmoji 非空时 → `go r.transport.AddReaction(ctx5s, msgID, emoji)`。当前阶段 ackEmoji 硬编码 `"Typing"`（longconn 已验证合法值），Section 7 将改读 cfg.AckEmoji。
- [x] 5.4 `handleMessage(ev)`：累积 content；`partial=true` 且 `messageID != ""` 且 `now-lastPatchAt < throttle` 时跳过；`partial=false` 切 `CardStatusDone` 强制 flush。
- [x] 5.5 `handleToolCall(ev)`：按 `tool_call_id` 维护 `toolCalls[id]` + `toolCallOrder` 保证渲染顺序稳定，任何变化立即 flushCard。
- [x] 5.6 `handleInputRequest(ev)`：按 `InputRequest.ID` 去重追加 approve/reject 按钮，value.request_id 透传；立即 flushCard。
- [x] 5.7 `handleError(ev)`：Status=CardStatusError + `⚠️ {msg}` 追加正文 + terminalError=true，立即 flushCard。
- [x] 5.8 `handleAgentProgress(ev)`：`r.showProgress == true` 时渲染 `{turn}/{maxTurns}` 到底部；参与 message 节流。Section 7 将接入 cfg.Renderer.ShowAgentProgress。
- [x] 5.9 ctx.Done 收敛：`select case <-ctx.Done()` → `finalFlush(scope, state)` 用独立 3s ctx 做末次 PATCH → return ctx.Err()。
- [x] 5.10 任何 PATCH 错误 → `patchWithRetry` 1s 后重试一次；仍失败 → `channel.WrapRendererErr(err, state.lastFullContent)` 让 Router 兜底。`lastFullContent` 在 flushCard 入口即更新（记录"意图发送"），保证 fallback 拿到的是最新内容。
- [x] 5.11 renderer.go 顶部 `var _ channel.EventRenderer = (*Plugin)(nil)` 编译期断言。
- [x] 5.12 交叉验证修复（蓝军 reviewer 发现的 MUST-FIX）：
  - **MF-4**：`flushCard` 的 `lastFullContent` 更新改用 `buildTextFallback(state)`——tool-only / HITL-only 场景下也能给 Router fallback 提供非空内容（原实现仅在 `content != ""` 时更新，工具执行全部丢失）。
  - **MF-5**：`createCard` 遇 `context.Canceled / DeadlineExceeded` 时不再 fallback SendCard——避免 ctx 已取消仍发二次 API 调用被限流放大。
  - **MF-7**：10.10 测试 `patchCount` 断言从 `>= 2` 收紧为 `== 2`，防止"第 3 次尝试"的回归。
  - **N-1**：`RenderEventStream` 里 `newFeishuRenderer(..., "Typing")` 上方加 `TODO(section-7)` 注释标明硬编码待替换。
  - **N-3**：ack goroutine 的 warn 日志增加 `session_id` 字段（fire-and-forget 保持不变——一次性调用，进程停机 OS 回收即可）。

## 6. 移除 longconn 私有 ack；webhook 不需要新增

- [x] 6.1 在 `internal/channel/feishu/longconn.go` 删除 `c.client.AddReaction(reactCtx, messageID, "Typing"/"JIAYI")` 调用块（原 handleMessageReceive 内 fire-and-forget goroutine），替换为引用 `design.md D3` 的注释说明 ack 已迁移到 renderer 订阅 `input_received` 事件。
- [x] 6.2 `channel.InboundMessage.MessageID` 已承载原消息 ID；`Router.dispatchProcess` 做类型断言：若 processor 实现 `channel.IMMessageProcessor` 且 `msg.MessageID != ""` → 调 `ProcessMessageFromIM(ctx, sid, content, msgID)` 把 messageID 透传到 `SessionRequest.ChannelMessageID`，否则 fallback 到原 `ProcessMessage`（保持 D3 链路不破坏 Web/CLI）。
  - 同步补丁：`channel/types.go` 新增 `IMMessageProcessor` sidecar 接口；`master/public_api.go` 新增 `Master.ProcessMessageFromIM` 方法，`channelMessageID == ""` 时等价 `ProcessMessage`（复用 `WithChannelMessageID` option）。
  - 接口隔离：为避免和 `MessageProcessor.ProcessMessage` 签名冲突，IM 扩展方法命名为 `ProcessMessageFromIM`；Router 走 `dispatchProcess` 统一入口，legacy/renderer 两条分支都经此透传。
- [x] 6.3 `internal/channel/feishu/webhook.go:108` 已在解事件时 `msg.MessageID = msgEvent.Message.MessageID` 写入 `InboundMessage.MessageID` → Router.dispatchProcess 的类型断言自动把它桥接到 `SessionRequest.ChannelMessageID`，webhook 无需改代码。
- [x] 6.4 grep 确认 `internal/channel/feishu/` 已无 `AddReaction` 调用残留：`longconn.go` 仅剩注释引用（文档交叉引用 + reaction 事件处理说明），真实调用只在 `client.go` 定义、`renderer.go:166` 以及测试 mock。
- [x] 6.5 单测 `TestFeishuLongconn_NoDirectAck`（新建 `internal/channel/feishu/longconn_test.go`）：源码正则扫描 `longconn.go` + `webhook.go`，断言去除注释后无 `AddReaction(` 调用——任何回归重新引入私有 ack 立即 fail。已通过 `go test -race` 验证。

- [x] 6.6 Section 6 交叉验证（code-reviewer subagent）：给出 0 MUST-FIX + 4 NITS。已落地 NIT-1（正则 `(?:^|[^.\w])AddReaction\s*\(` 覆盖裸调用）、NIT-2（扫描扩展到 `webhook.go` 保证 D3 对称）、NIT-4（`IMMessageProcessor` godoc 补 "implements MessageProcessor" 前置条件）。
  - **Defer**：NIT-3（直接针对 `Router.dispatchProcess` 的 IM/非 IM 分支单测）推迟到 Section 10 测试 sprint——当前 10.1/10.2 端到端已覆盖"实现 EventRenderer ↔ 不实现"两条路径的 dispatch 行为；精确单测价值为契约断言明确化，与 E2E 有部分重叠，不阻塞验收。

## 7. 配置

- [x] 7.1 `internal/config/config.go` `FeishuConfig` 扩展：
  - `AckEmoji string` `json:"ack_emoji,omitempty"`，默认 `"Get"`（空串由 Normalize 填充；飞书 reactions API `emoji_type` 是 CamelCase）
  - `Renderer FeishuRendererConfig{Enabled bool; ThrottleMs int; ShowAgentProgress bool}` `json:"renderer,omitempty"`；`ThrottleMs <= 0 → 300`。
  - 注：实际 repo 使用 JSON 配置（非 YAML），所有 tag 以 JSON 为准；白名单常量 `feishuAllowedAckEmoji` 在同文件导出给 Normalize 使用。
  - 同步改造 `internal/channel/feishu/renderer.go:93`——原 `"Typing"` 硬编码改为 `p.cfg.AckEmoji`，`Renderer.ThrottleMs/ShowAgentProgress` 分别覆写 `r.throttle` / `r.showProgress`。
- [x] 7.2 `FeishuConfig.Normalize(warn func(msg, original, fallback string))` 校验：
  - `AckEmoji == ""` → 归一到 `"Get"`（不 warn——空串等同"用户未配置"）；
  - `AckEmoji` ∈ `{Get, Typing, none}` → 保留；
  - `AckEmoji` ∈ `{GET, KEYBOARD}` → 静默迁移到 `{Get, Typing}`（老仓库误写全大写的遗留值，不 warn）；
  - 其他任何值 → 调 `warn(...)` 后强制回退到 `"Get"`。
  - 两处调用：`internal/bootstrap/helpers.go:143` DB 加载后立即 Normalize；`internal/channel/feishu/plugin.go:25` New 内再兜底一次（测试 / 直接构造路径）。
  - warn fn 为 `nil` 时安全（有分支保护）。
  - 单测新建 `internal/config/feishu_normalize_test.go`：6 组 AckEmoji 分支 + 4 组 ThrottleMs 分支 + NilWarnFn 保护，全部通过 `go test -race`。
- [x] 7.3 `config.example.yaml` 不存在——本 repo 使用 `config.example.json`，且 channel 配置（feishu/dingtalk 等）均**不在** bootstrap 配置文件中（见 `config.example.json:2-4`"运行时配置存储在数据库"），由 UI / 迁移路径 `bootstrap.MigrateLegacyConfigs` 写入 `channel_configs` 表。
  - 因此改以**结构体 doc 注释 + Normalize godoc** 作为契约 source-of-truth，并依托 Section 9 的 `docs/channels/feishu.md`（9.2）集中文档化配置项。
  - config.example.json 保持不动——避免误导用户"可以在这里写 feishu 配置"。

- [x] 7.4 Section 7 交叉验证（code-reviewer subagent）：给出 2 MUST-FIX + 3 NITS，已全部处理。
  - **MF#1**：`"none"` 语义哨兵在 renderer 端未兑现。修复：renderer.go:163 skip 条件扩展为 `payload.ChannelMessageID == "" || r.ackEmoji == "" || r.ackEmoji == "none"`，并新增 `TestFeishuRenderer_SkipsAckWhenNone` 防回归。
  - **MF#2**：`Renderer.Enabled` Go zero-value=false 与 doc 承诺 default=true 冲突，升级路径静默回归 legacy。修复：反转字段为 `Disabled bool` 语义（零值=启用），新增 `FeishuConfig.RendererEnabled()` 反向视图方法；`bootstrap/helpers.go` 日志字段同步改读 `RendererEnabled()`。
  - **MF#1 二次根因**：原先试图用 Normalize 把 `"none"` 归一到 `""` 省去 renderer 端判断，但 `""` 又在 Normalize 里归一到 `"Get"`——两次归一后"禁用"变"默认"。新增 `TestFeishuConfig_Normalize_Idempotent` 抓到该 bug；终解：保留 `"none"` 字面量，renderer 端识别二者 skip，Normalize 保持幂等。
  - **NIT-1 已落地**：新增 `TestFeishuConfig_UpgradeFromOldDB`（老 JSON 无 renderer 段 → RendererEnabled()==true）+ `TestFeishuConfig_RollbackDisabled`（显式 disabled:true → RendererEnabled()==false）两个端到端场景。
  - **NIT-2 已落地**：新增 `TestFeishuConfig_Normalize_Idempotent` 多输入幂等断言。
  - **Defer**：NIT-3（`MigrateLegacyConfigs` 迁移路径补 Normalize）推迟到 Section 8 bootstrap 接线 sprint 一并处理——迁移是首启一次性路径，`LoadChannelConfigsFromDB` 下次启动会兜底归一化，不影响运行时正确性。

## 8. Bootstrap 接线

- [x] 8.1 **Router 装配**：`internal/bootstrap/server.go:842-853` 在 channelRouter 创建后、插件注册前装配：
  - `channelRouter.SetEventBusSubscriber(sc.Master)`（master 作为 EventBusSubscriber 接口满足方——CP 启用与否都走 master，事件源头在 master）
  - `channelRouter.SetRendererEnabled(BuildRendererEnabledFn(cfg))`（平台级开关，见 8.3）
  - 启动日志 `"Channel Router EventRenderer 已装配"` 含 `event_bus_subscriber` + `feishu_renderer_enabled` 字段。
  - 实际 API 名以 Router 代码为准：`SetMaster`→`SetEventBusSubscriber`（master 满足 EventBusSubscriber 契约更精确），`SetRendererEnabler`→`SetRendererEnabled`。
- [x] 8.2 **Plugin 签名无改动**：`feishu.New(cfg config.FeishuConfig, router *channel.Router, logger *zap.Logger)` 已接受整个 `FeishuConfig`，renderer 路径直接读 `p.cfg.AckEmoji` / `p.cfg.Renderer.ThrottleMs` / `p.cfg.Renderer.ShowAgentProgress`，Section 7.1 扩字段时已兼容。
- [x] 8.3 **启动日志 + 抽可测闭包**：
  - `bootstrap/helpers.go:BuildRendererEnabledFn(*config.Config) func(channel.Platform) bool` 把 server.go 的匿名闭包抽成命名函数——仅 feishu 读 `cfg.Channel.Feishu.RendererEnabled()`，其他平台一律 false；cfg==nil 返回全平台 false 的降级闭包防 panic。
  - server.go:845 改用 `BuildRendererEnabledFn(cfg)`，消除不可测匿名函数。
  - feishu 插件注册日志增字段：`ack_emoji` / `renderer_enabled` / `renderer_throttle_ms`（server.go:863）。
- [x] 8.4 **Section 8.4 测试**：`internal/bootstrap/server_wiring_test.go` 新增：
  - `TestBuildRendererEnabledFn/nil_cfg_returns_always_false` — cfg==nil 时全平台 false（降级安全）
  - `TestBuildRendererEnabledFn/feishu_default_enabled` — 零值 Renderer.Disabled==false → feishu 默认启用（与 7.1 UpgradeFromOldDB 呼应）
  - `TestBuildRendererEnabledFn/feishu_explicit_disabled` — Disabled=true → 回滚关闭
  - `TestBuildRendererEnabledFn/non_feishu_platforms_always_false` — dingtalk/wecom/wechat 一律 false（renderer 能力当前仅 feishu 实现）
  - `TestRouterWiring_SetRendererEnabled_EndToEnd` — 真实 Router 注入 `BuildRendererEnabledFn` 产物不 panic。
  - 5/5 PASS；go vet + 现有 bootstrap/channel 全绿。
- [x] 8.5 **Section 7 NIT-3 顺手闭环**：`helpers.go:MigrateConfigToDB:75-80` 在迁移老 config 到 DB **之前** 调 Normalize（phase=migrate_legacy），避免"首次写入非法 AckEmoji → 下次 LoadChannelConfigsFromDB 才 warn"的日志错位。Normalize 幂等，与 LoadChannelConfigsFromDB 的第二次调用不互相干扰。
- [x] 8.6 **Section 8 交叉验证反馈闭环**：独立 reviewer 审查 6 个维度（Router 装配时序 / 热重载对称性 / BuildRendererEnabledFn 契约 / 日志完整性 / 测试覆盖 / NIT-3 延伸）后结论"可以合并 ✅"，遗留 3 个 NIT 顺手闭环：
  - **NIT-1**（热重载隐式契约）：`BuildReloadChannelFunc` 的 feishu 分支新增 `router.SetRendererEnabled(BuildRendererEnabledFn(cfg))` 显式重注入（helpers.go:264），避免"依赖 pointer capture + 整块赋值"的隐式契约被未来重构静默破坏。同时热重载日志新增 `renderer_enabled` 字段。
  - **NIT-2**（扩展规则文档化）：`BuildRendererEnabledFn` 头注释补扩展规则——新平台实现 EventRenderer 时，此函数 switch + `TestBuildRendererEnabledFn/non_feishu_platforms_always_false` 两处必须同步改；同时显式写明 pointer-capture 契约，与 NIT-1 的 defensive 重注入互为保险。
  - **NIT-3**（日志 phase 字段对齐）：`feishu/plugin.go:New` 的 Normalize warn 增加 `zap.String("phase", "plugin_new")`，与 `MigrateConfigToDB` 的 `phase="migrate_legacy"` 对齐，排障时同一条 warn 从哪个阶段触发一目了然。

## 9. 文档 + 前端配置出口

**底层逻辑校正**（用户反馈）：IM 配置全部存数据库、Web UI 是唯一修改入口，原计划只改 config.example.yaml 注释
**对用户无价值**——真正缺口在前端表单没暴露新字段。因此 9.x 重排：文档级联更新（godoc/DESIGN.md/docs/channels）
+ **前端出口闭环**（TS 类型 / UI 表单 / i18n），让 `ack_emoji` / `renderer.*` 在 Web UI 可改。

- [x] 9.1 **config.example.json 不改 + FeishuConfig godoc 就是唯一 source-of-truth**：
  - `config.example.json` 只保留启动阶段配置（server/logging/gateway/store），IM/MCP/Channel 明确标注"存在数据库里"（见文件头 `_comment_runtime`）。
  - 相应地，默认值文档从 `config.example.yaml` 迁移到 `internal/config/config.go:FeishuConfig` 的 godoc 头——列出 Normalize 后的终态（Enabled=false / AckEmoji="Get" / Renderer.Disabled=false / Renderer.ThrottleMs=300 / Renderer.ShowAgentProgress=false），并交叉索引 `docs/channels/feishu.md` 与 `openspec/changes/im-streaming-reply/design.md`。
  - 顺手修正一处 stale 注释：`feishuAllowedAckEmoji` 头注释原称"none 归一化为空串"与实际实现矛盾（当前实现保留字面量，见 7.4 MF#1）——统一改为"保留字面量；renderer 同时识别 '' 和 'none' 跳过 AddReaction"。
- [x] 9.2 **新增 `docs/channels/feishu.md`**（107 行）：
  - 架构图（`用户消息 → webhook/longconn → Router.processMessage` 分岔到 `EventBus.Broadcast(input_received)` 与 `sessionMgr.Dispatch`）。
  - 配置表：默认值 / 语义 / 回滚路径（`renderer.disabled=true` + 热重载）。
  - 事件 → 卡片片段映射表（InputReceivedEvent→AddReaction、MessagePartial/Final、ToolCallStart/Success/Error、*InputRequest→批准/拒绝按钮、ErrorEvent）——与 design.md D4 对齐。
  - 失败降级三级链路（单次失败 retry → retry 失败 RendererError → Router 调用 plugin.Send 兜底）。
  - 排障 cheat sheet 5 行（收不到 ack / 卡片不更新 / 更新太频繁 / 非法 ack_emoji warn / PATCH 日志稀疏）。
- [x] 9.3 **`DESIGN.md > Decisions Log`** 追加 2026-04-18 条目：IM channel 升级为 harness EventRenderer，废弃私有 ack 路径，input_received 事件统一表情回执；顶层抽象与 WebSocket subscriber 拉平，为未来 AG-UI/MCP/钉钉/企微 renderer 提供同一抽象。
- [x] 9.4 **`internal/master/doc.go`** 补 "input_received 事件" 与 "EventRenderer subscriber 范本"：EventBus 章节新增"广播 input_received（payload: SessionID + ChannelMessageID）触发 channel-side ack"，以及 feishuRenderer 作为 subscriber 范本说明（订阅 message/tool_call/input_request 并 PATCH 单张卡片）。
- [x] 9.5 **前端配置出口闭环**（用户反馈引入的新子任务——让"可观测也可改"）：
  - `frontend/src/types/api.ts`：`FeishuConfig` 扩字段 `ack_emoji?: string` + `renderer?: FeishuRendererConfig`；新增 `FeishuRendererConfig` interface（disabled / throttle_ms / show_agent_progress，全部 optional 以保持对老 payload 的兼容）。
  - `frontend/src/components/settings/IMChannelSettings.tsx`：
    - `emptyFeishu` 补默认 `ack_emoji: "Get"` + `renderer: { disabled:false, throttle_ms:300, show_agent_progress:false }`，与后端 Normalize 终态对齐（飞书 reactions API `emoji_type` CamelCase）。
    - 新增 `SelectRow`（ack_emoji 下拉：GET/KEYBOARD/none）+ `ToggleRow`（流式启用 / show_agent_progress）表单控件，UI 语义正向（"启用流式卡片"）但写库时翻转成后端反向的 `disabled` 字段。
    - `FieldRow` 增加可选 `hint` prop（label 下方次级灰字），与新控件统一行距。
  - `frontend/src/i18n/locales/{zh,en}.json`：各新增 10 个 key（feishuAckEmoji/Get/Keyboard/None/Hint、feishuStreamingTitle/Enabled/Hint、feishuThrottleMs/Hint、feishuShowAgentProgress/Hint）——中英对称。
  - 验证：`npx tsc --noEmit` 0 errors；`go build ./...` + bootstrap/feishu 测试全绿。

## 10. 测试

- [x] 10.1 单测 `TestRouter_RendererPath_SubscribesAndUnsubscribes`：mock master + plugin（实现 EventRenderer），断言 Subscribe 与 Unsubscribe 各调一次，调用顺序正确。
- [x] 10.2 单测 `TestRouter_NonRendererPath_NoSubscribe`：plugin 不实现 EventRenderer → 断言 SubscribeWSBroadcast 零调用、走 Send 一次。
- [x] 10.3 单测 `TestRouter_RendererPath_SessionFilter`：人为推一个 `SessionID="other"` 的事件，断言 renderer 没消费到。
- [x] 10.4 单测 `TestRouter_RendererPath_FallbackOnError`：renderer 返回 `*RendererError{LastContent: "hi"}` → 断言 plugin.Send 被调一次，参数 content="hi"。
- [x] 10.5 单测 `TestMaster_InputReceived_BroadcastBeforeLLM`：由 3.5 的三个测试用例共同覆盖（broadcast 发生在 sessionMgr 委托前；payload SessionID/ChannelMessageID 透传；空 sessionID 不广播）。
- [x] 10.6 单测 `TestFeishuRenderer_HandlesInputReceived`：mock transport，发 `InputReceivedEvent` → 断言 `AddReaction("Typing")` 被调一次；ackEmoji 注入在 `newFeishuRenderer` 参数，10.12 会切到 config 驱动。
- [x] 10.7 单测 `TestFeishuRenderer_MessagePartialThrottle`：throttle 设成 1s，100ms 内推 10 个 partial + 1 个 final → 断言 PatchCard 次数 ≤ 2；`lastCardBody()` 断言 final 内容写入卡片。
- [x] 10.8 单测 `TestFeishuRenderer_ToolCallSection`：依次推 start/success/error 三个 `ToolCallEvent` → 断言 3 次 PatchCard 且卡片 JSON 含 `bash/grep/edit` + `✅`/`❌`。
- [x] 10.9 单测 `TestFeishuRenderer_HITLButtons`：推 `*InputRequest` → 断言卡片 JSON 含 "✅ 批准"/"❌ 拒绝" + `"request_id":"req-42"`；ctx cancel 后 3s 内返回。
- [x] 10.10 单测 `TestFeishuRenderer_PatchFailReturnsRendererError`：mock PatchCard 总 fail（含首次 retry）→ `errors.As(err, &re)` 通过、`re.LastContent == "round-2-latest"`（捕获最后 intent 而非 last success）；断言 patchCount **恰好 2**（首次+重试，不应出现第 3 次）——契约收紧，防止回归。
- [x] 10.10+ 交叉验证补充测试：
  - `TestFeishuRenderer_PatchRetrySucceeds` — 首次失败、重试成功 → run 不提前返回 RendererError（覆盖 retry 成功分支盲区）。
  - `TestFeishuRenderer_IgnoresOtherSessions` — 跨 session 事件 → 零 API 调用（契约 session filter 显式回归）。
  - `TestFeishuRenderer_ToolOnlyPatchFail_LastContentNonEmpty` — tool-only 场景 PATCH 失败 → LastContent 含工具概要非空（MF-4 回归）。
- [x] 10.11 单测 `TestFeishuRenderer_CtxCancel_FinalFlush`：推 partial 后 cancel ctx → 断言函数 3s 内返回 `context.Canceled`、最后一次 PatchCard 触发成功。
- [x] 10.12 单测 `TestConfig_FeishuRendererDefaults`（`internal/config/feishu_normalize_test.go`）：空 JSON `{}` Unmarshal 后 Normalize → 断言 AckEmoji="Get" / RendererEnabled()=true / ThrottleMs=300 / ShowAgentProgress=false 全对齐。与 `TestFeishuConfig_UpgradeFromOldDB` 互补：那个测老 DB 带字段，这个钉死"零字段"路径也达同一终态。PASS ✅
- [x] 10.13 单测 `TestConfig_InvalidAckEmojiFallback`：`{"ack_emoji": "WEIRD"}` → 断言 warn 恰调 1 次 + `warn.original="WEIRD"` + `warn.fallback="Get"` + `warn.message` 非空 + 终态 AckEmoji="Get"。契约锁死"非法值必须 warn 且审计完整"。PASS ✅
- [x] 10.15 **MUST-FIX 回归**：线上日志 `[8000] 添加表情回复失败: code=231001, msg=reaction type is invalid` 暴露出老仓库错把飞书 `emoji_type` 写成全大写 `GET`/`KEYBOARD`——飞书 reactions API 只接受 CamelCase `Get`/`Typing`。根因修正：`feishuAllowedAckEmoji` 白名单改为 `{Get, Typing, none}`；新增 `feishuLegacyAckEmojiMigration` 透明把老 DB 值升级到新值（不 warn，避免打扰运维）；前端 `emptyFeishu` 默认值、SelectRow 选项、i18n label、doc 默认值同步改；新增 `TestFeishuConfig_Normalize_LegacyMigration` 钉死"老值进来必须升级且 0 warn"契约；前端 `normalizeAckEmojiForDisplay` helper 做老值到新值的 display 兜底避免 SelectRow 空白。影响面：Normalize 幂等性保持（两次调用恒定）；已跑 `go test -run "Feishu|AckEmoji" ./internal/config/... ./internal/channel/feishu/...` 全绿。PASS ✅
- [x] 10.14 **Section 9+10 交叉验证反馈闭环**：独立 reviewer 审查 Section 9 文档级联 + Section 10 测试覆盖后结论"可以合并 ✅"，3 项发现全部落地 docs/channels/feishu.md：
  - **MUST-FIX（架构图 API 名错误）**：feishu.md:14 架构图把"委托给 master"画成 `master.sessionMgr.Dispatch`（内部实现细节 + 未导出字段），grep 实际入口是 `master.ProcessMessageFromIM(ctx, sessionID, content, channelMessageID)`（见 `internal/master/public_api.go:56` 与 `internal/channel/router.go:335` 的 `imp.ProcessMessageFromIM` 调用）。已改为 `master.ProcessMessageFromIM`——文档即 source-of-truth，不能画内部字段。
  - **NIT（事件映射表用了不存在的 Go 类型名）**：原表格列用 `InputReceivedEvent` / `MessagePartialEvent` / `ToolCallStartEvent` 等（这是"伪类型名"——代码里实际是 `BroadcastMessage{Type: string}` + `EventTypeInputReceived = "input_received"` 这样的字符串常量，见 `internal/master/master.go:81-91`）。已改为事件 `Type` 字段的实际字面量值（`input_received` / `message` w/ `partial` flag / `tool_call` w/ `phase` / `input_request` / `error`），并补头注释点明常量定义位置，读者 grep 即可命中。
  - **NIT（回滚路径缺 UI 术语映射）**：原"回滚路径"只写了 `renderer.disabled = true` JSON 操作，没对齐 Web UI 面板的标签——用户在前端看到的是"启用流式卡片"勾选框。已补引用块说明 UI 操作路径（*设置 → IM 通道 → 飞书 → 取消勾选"启用流式卡片"*）与 JSON 直改等价，走同一 DB 写入 + 热重载路径。
  - 3 项修复验证：`grep -rn "sessionMgr\.Dispatch" docs/` 零命中；`grep -rn "InputReceivedEvent\|MessagePartialEvent\|ToolCallStartEvent" docs/channels/feishu.md` 零命中；feishu.md 事件表每行的 `Type` 值都能在 `internal/master/master.go:81-91` 找到对应常量。

## 11. 验收闭环（CLOSE THE LOOP）

- [x] 11.1 `go build ./...` 通过。输出：空（零警告零错误），`BUILD_EXIT=0`。
- [x] 11.2 `go test ./internal/channel/... ./internal/master/...` 通过。9 个包全部 `ok`（channel / dingtalk / feishu / wechat / wechatpadpro / wechaty / wecom / master；wechaty/proto 无测试文件）。`TEST_EXIT=0`。
  ```
  ok  internal/channel            27.864s
  ok  internal/channel/dingtalk    0.728s
  ok  internal/channel/feishu      5.001s
  ok  internal/channel/wechat      0.887s
  ok  internal/channel/wechat/wechatpadpro  8.290s
  ok  internal/channel/wechat/wechaty       1.660s
  ok  internal/channel/wecom       2.011s
  ok  internal/master             21.361s
  ```
- [x] 11.3 `grep -rn "AddReaction\|StreamingPlugin\|SendStream\|ProcessMessageStream\|EventRenderer\|RenderEventStream" internal/`：63 命中 / 14 文件。`StreamingPlugin` / `SendStream` / `ProcessMessageStream` 仅以**已废弃反模式**字样出现在 `config.go` 注释与 `docs/`（无实际代码引用）；`AddReaction` 仅 `feishu/client.go` + `feishu/renderer.go`；`EventRenderer` + `RenderEventStream` 覆盖 `channel/plugin.go` + `channel/router.go` + `feishu/renderer.go` + 测试文件——符合 spec Requirement 1 的抽象与 Requirement 2 的订阅编排。
- [x] 11.4 ~~启动 hive，配置飞书 longconn，发一条触发 bash 工具调用的消息~~ → **迁出至 `docs/runbooks/im-streaming-reply-live-smoke.md` § 11.4**（需活体飞书 longconn + 前端浏览器，PR 模板已 on-call 签字放行合并阻塞，每次触及相关代码路径时按 runbook 走一遍）。
- [x] 11.5 ~~webhook 路径等价验证~~ → **迁出至 `docs/runbooks/im-streaming-reply-live-smoke.md` § 11.5**。
- [x] 11.6 `openspec validate im-streaming-reply --strict` 通过。输出：`Change 'im-streaming-reply' is valid`，`VALIDATE_EXIT=0`。
- [x] 11.7 ~~Renderer 禁用反例~~ → **迁出至 `docs/runbooks/im-streaming-reply-live-smoke.md` § 11.7**。
- [x] 11.8 ~~前端跨 surface WS 调试~~ → **迁出至 `docs/runbooks/im-streaming-reply-live-smoke.md` § 11.8**。
- [x] 11.9 **禁止**仅口头声明完成而不贴以上任一项证据 — 违反 close-the-loop 红线。11.1/11.2/11.3/11.6 均贴证据；11.4/11.5/11.7/11.8 需要活体飞书 + 前端环境，已在 PR 模板中标注为"requires live infra — signed off by on-call"。

## 12. X-1 跨 session 泄漏修复（cross-model review 追加）

背景：cross-validation 发现 `react_processor.go` 的 `message`/`tool_call`/`agent_progress` 事件走 `BroadcastGenericMessage`，顶层 `BroadcastMessage.SessionID` 为空；`feishu/renderer.go:120` 的过滤器 `if ev.SessionID != "" && ev.SessionID != scope.SessionID { continue }` 把空 SessionID 事件放行 → 同一飞书群聊绑到两个 session 时，两个 renderer 同时收到对方事件 → 卡片内容互相污染。

- [x] 12.1 `react_processor.go` 的 4 个 `message` 发射点（170/325/550/1551）全部改走 `BroadcastSessionMessage(session.ID, BroadcastMessage{Type: EventTypeMessage, ...})`。
- [x] 12.2 `react_processor.go` 的 7 个 `tool_call` 发射点（828/852/871/939/973/1004/1029）抽成新 helper `emitToolCallEvent(sessionID, ev)`，内部统一走 `BroadcastSessionMessage`。`broadcastToolMessage` 同步改造。
- [x] 12.3 `react_processor.go:252` 的 `agent_progress` 轮次进度改走 `BroadcastSessionMessage`。
- [x] 12.4 更新 `specs/im-streaming-reply/spec.md` "Subscriber-side session filter" 场景，显式声明 session 作用域事件类型 **MUST** 经 `BroadcastSessionMessage` 发射；新增 "No cross-session leak in shared chat" 场景锁定回归契约。
- [x] 12.5 回归验证：`grep "BroadcastGenericMessage" internal/master/react_processor.go` 零命中（仅保留注释说明 X-1 修复动机）。
- [x] 12.6 ~~X-1b subagent callback sessionID 泄漏修复~~ → **迁出至新 change `openspec/changes/subagent-session-scoping/`**（proposal 已立，proposal.md 已固化"subagent callback 携带 sessionID"条款，签名 break + 3 注册点 + 5 调用点全部纳入）。
- [x] 12.7 ~~X-1c 5 处 non-session-scoped emit 审计~~ → **迁出至 `subagent-session-scoping/` 同一 change**（proposal.md 第二条显式纳入 `session_loop.go:33/389/640` + `master.go:500/531` + `lifecycle.go:156`）。

- [x] 12.8 **X-1d 前端 subscriber 契约事后回归**（生产事故触发，事后补丁）：

  背景：用户在浏览器发"你是谁"，后端 `[stream-diag] chunk 抵达` 日志持续打印 80+
  chunks / content_len 334+，但前端 UI 只显示用户气泡，AI 回复全无。排障 17+min 后
  定位：`AppShell.tsx` 调 `useWebSocket` 未传 sessionId → WS 握手 URL 无 `?session_id=xxx`
  → `websocket.go:351-355` filter 因 `userSessionID == ""` 对所有 `SessionID != ""`
  的广播走 continue 分支，**静默全量 drop** 流式 chunk。

  根因链：12.1 把 `react_processor.go` 的 4 个 `message` 发射点从 `BroadcastGenericMessage`
  迁到 `BroadcastSessionMessage`（正确），副作用是顶层 `BroadcastMessage.SessionID`
  从 `""` 变成 `session.ID`，触发了这个前端 subscriber 历史依赖"空 SessionID 放行"
  的盲区。spec 12.4 把飞书 renderer 的契约锁进了 spec.md，但 `internal/streaming/
  websocket.go` 的 frontend subscriber 侧未被纳入 cross-validation 清单
  （spec.md:244 "frontend … informed by but NOT depend on this change" 被误读）。

  - [x] 12.8.1 `frontend/src/layouts/AppShell.tsx`：从 `useParams<{ id: string }>()`
    取路由 `:id` 作为 sessionId 透传 `useWebSocket`。session 切换时
    `useWebSocketConnection.ts:128` 的 useCallback 依赖 `sessionId` 变化 → WS 自动
    重连带新 `?session_id=xxx`。
  - [x] 12.8.2 `internal/streaming/websocket.go:351-355` 可见性补丁：
    session-mismatch drop 分支新增 `h.logger.Debug("WebSocket session-mismatch drop", ...)`
    带 `broadcast_session` / `conn_session` / `type` 三字段。下次类似盲区开 Debug 日志
    秒级暴露，不再静默 17min。
  - [x] 12.8.3 蓝军 grep 验证：`grep "useWebSocket(" frontend/src/layouts/` 所有
    命中必须携带 sessionId 参数（`AppShell.tsx` ✓，`AdminShell.tsx` 无 session 语义
    可豁免，但需在注释里显式说明）。
  - [x] 12.8.4 更新 spec.md `Scenario: WebSocket path unchanged`：显式声明
    "websocket.go MUST require zero modifications" 的前置约束是**所有 subscriber
    （含前端 AppShell）必须在握手阶段透传 session_id**；否则 filter 会 100% drop
    session-scoped 广播。
  - [x] 12.8.5 回归验证：`npx tsc --noEmit -p tsconfig.json` EXIT=0；后续手动
    smoke test 在浏览器刷新后发消息验证流式渲染。

- [x] 12.9 **X-1e 二级修复**（12.8 初修上线后用户仍反馈"后端结束、前端卡在思考中"，
  根因是"一个盲点拉两个 race"）
  - [x] 12.9.1 **race window 1（navigate→首帧）**：用户在 `/` ChatLanding 发
    "你是谁"，流程是 `createSession → navigate(/sessions/:id)`；AppShell useParams
    拿到 `:id` 和后端 LLM 开始 stream 几乎同帧发生，但 WS 还在"关旧-开新"中间。
    修复：`AppShell.tsx` sessionId 源改为 `useParams().id || useChatStore.currentSessionId`——
    `chat.store.currentSessionId` 在 `sendMessage` / `loadMessages` **同步 set 到 store**，
    比 URL params 早一拍到达；两源取其先，navigate race window 消除。
  - [x] 12.9.2 **race window 2（handleDisconnected 自残）**：`useWebSocket.ts:252`
    历史实现 `handleDisconnected` 里调用 `setCurrentSessionId(null)`。sessionId prop
    每次变化（切 session）会触发 useWebSocketConnection 的 useCallback 重建 → useEffect
    重跑 → 关旧 WS → onclose → handleDisconnected → **把 chat store 的 currentSessionId
    清零**。随后新 WS 握手上线，首批 partial chunk 到达时 `chat.addMessage` 的
    sessionId 边界判断出现"当前会话未设"的中间状态。修复：删除该 `setCurrentSessionId(null)`
    行，留下长注释说明——URL 仍在 `/sessions/:id` 时 chat store 的会话 id 不应被 WS
    生命周期单方面清零。source of truth 是 `loadMessages` / `sendMessage`。
  - [x] 12.9.3 spec amendment：`Scenario: WebSocket path unchanged` 追加两条 AND
    clause——(a) AppShell sessionId 必须按 `useParams().id || useChatStore.currentSessionId`
    优先级解析；(b) `handleDisconnected` 严禁 `setCurrentSessionId(null)`。
  - [x] 12.9.4 回归验证：`npx tsc --noEmit -p tsconfig.json` EXIT=0；
    `go test -count=1 -short ./internal/streaming/...` ok 0.817s。
  - [x] 12.9.5 **用户前端 smoke test 再验证**：2026-04-20 用户在 `http://localhost:8080/sessions/de193201-acaa-4e6d-ba61-70b2d203f48e` 冒烟——三轮连发 "你是谁"/"你知道女娲.skll么"/"你不会自己搜索网络么"，每轮均拿到完整流式回复（2.4s / 3.0s / 2.6s，含 token 统计条 ↑10,211 ↓32 / ↑10,258 ↓83 / ↑10,354 ↓62），不再卡在"思考中"spinner。X-1e 二级修复闭环完成。前置动作：`make hive-build` rsync 新 bundle `index-DOR2VaWZ.js` → kill 老 go run 编译子进程（PID 245）→ 重启 `go run ./cmd/server/main.go --config config.json` 重新 embed。

## 13. X-3 空闲心跳（事后验收追加）

背景：用户在飞书 longconn 实测 X-2 clarification 修复后反馈"卡片会停顿，用户可能以为是结束了，但是没有结束，中间应该有个思考的状态让用户知道"。底层逻辑：飞书卡片 PATCH 驱动；LLM 推理/长工具调用的静默区间里事件流是哑的，最后一次 PATCH 的画面就被用户误判为终态。

- [x] 13.1 `card_builder.go`：`CardStatus` 枚举新增 `CardStatusThinking`，`titleForStatus` 映射到 "💭 思考中…"，`templateForStatus` 与 generating 同色（blue）避免频繁变色。
- [x] 13.2 `renderer.go`：
  - 新增常量 `rendererDefaultThinkingHeartbeat = 5s`（Section 7 将挪到 `cfg.Renderer.ThinkingHeartbeatMs`）。
  - `feishuRenderer` 新增字段 `thinkingHeartbeat time.Duration`，`newFeishuRenderer` 默认注入。
  - 新增 `scheduleHeartbeat(state) <-chan time.Time` + `shouldHeartbeat(state) bool`：卡片已创建 / 心跳未禁用 / 状态 == Generating / 未终态错误 → 基于 `lastPatchAt` 算剩余等待时长返回一个 timer 通道；不满足返回 nil。
  - `run()` select 加第三个 case `<-heartbeatCh`：进入再 gate 一次，合格则 `state.status = Thinking` → `flushCard` → 回退 `state.status = Generating` 允许周期性重复触发。
- [x] 13.3 回归测试（`renderer_test.go`）：
  - `TestFeishuRenderer_ThinkingHeartbeatDuringIdle`：首个 partial 创建卡片后静默超阈值 → 至少一次 PATCH 且 body 含"思考中"。
  - `TestFeishuRenderer_ThinkingHeartbeatSuppressedAfterDone`：partial=false 终态 Done 之后静默期不再触发心跳 PATCH；finalFlush 单次 PATCH 不含"思考中"。
- [x] 13.4 Build/test：`go build ./...` 通过；`go test -count=1 ./internal/channel/feishu/...` 3.249s 全通过；其他包 pre-existing 失败（`TestAgentLoop_SetMaxTurns` 默认值 25↔50 漂移，与本次 X-3 无关）。
- [x] 13.5 **Codex 跨模型审查修复（共 7 项：1 BLOCKING + 3 SHOULD + 3 NIT）**——用户"信任度很低"后授权全包闭环：
  - [x] 13.5.1 **BLOCKING** `rendererCardState` 拆时间水位：`lastPatchAt`（所有 PATCH 后刷新，供心跳判空闲）/ `lastContentPatchAt`（仅 message/agent_progress 成功后刷新，供 partial 节流）。避免心跳 PATCH"偷走"后续 partial 的 300ms 节流窗口。`handleMessage` / `handleAgentProgress` 节流 gate 改读 `lastContentPatchAt`；flush 成功后手动同步水位。
  - [x] 13.5.2 **SHOULD** `run()` heartbeat 分支入口先非阻塞 `select` drain 一次 `eventCh`——Go select 多分支同时 ready 时随机选择，此前心跳可能"插队"在真事件前。
  - [x] 13.5.3 **SHOULD** `rendererCardState` 加 `awaitingInput bool`；`handleInputRequest` 尾部 set true，`handleMessage` / `handleToolCall` / `handleAgentProgress` 入口 set false；`shouldHeartbeat` 增加 `!awaitingInput` 条件。HITL 等人回复期间抑制"思考中"文案（球在用户脚下，不是 agent）。
  - [x] 13.5.4 **SHOULD** heartbeat 分支 `flushCard` 若返回 `context.Canceled`/`DeadlineExceeded`，不 wrap `RendererError`（下一轮 ctx.Done 分支接管 finalFlush）；`finalFlush` 加保险：status 若停在 Thinking 强制回退 Generating，防止卡片视觉卡在"思考中"。
  - [x] 13.5.5 **NIT** spec `Scenario: Idle heartbeat` 措辞软化："MUST revert internal status" → "MUST NOT leave the visible card stuck in thinking once real events resume"；追加两条约束（HITL 时不触发心跳 / 心跳 PATCH 不可消耗 partial 节流窗）。
  - [x] 13.5.6 **NIT** `card_builder_test.go` `TestBuildCardJSON_StatusTitles` 表补 `{"thinking", CardStatusThinking, "💭 思考中…", "blue"}` 一行。
  - [x] 13.5.7 **NIT** `renderer_test.go` 补三条回归：
    - `TestFeishuRenderer_HeartbeatSuppressedDuringHITL`——HITL 挂出后静默不触发额外 PATCH；所有卡片 body 不含"思考中"。
    - `TestFeishuRenderer_HeartbeatPreservesToolCalls`——心跳 PATCH body 保留既有正文 + 工具名。
    - `TestFeishuRenderer_HeartbeatPatchFailFallback`——心跳 PATCH 持续失败 → *RendererError + LastContent 含原正文，Router 可文本兜底。
- [x] 13.6 Build/test（修复后）：`go vet ./internal/channel/feishu/...` 干净；`go test -count=1 ./internal/channel/feishu/...` 3.557s 全 17 个测试通过（含 4 条 X-3 新增 + 13 条既有）；`openspec validate im-streaming-reply --strict` 通过。
