# 2026-04-29 飞书文档循环读取与 ACK 延迟根因记录

## Symptom

- 飞书群里引用文档后让机器人分析，系统已经成功通过 `feishu_api` 读取表格内容，但后续 ReAct 轮次持续调用 `feishu_api`。
- 日志显示第 5 轮被循环检测器终止：`loop detected: same tool combination repeated 5 times`。
- 用户还指出收到任何问题时应该先回复表情，而不是等读取父消息、根消息、文档引用等上下文后再反馈。

## Root Cause

- 文档循环读取：`detectToolChoiceWithContext` 看到 IM refs 非空会返回 `required`，这是首轮读取文档所需。但 `runReActLoop` 每轮都把同一份 `imCtx.References` 传入 detector，即使上一轮已经成功读取正文/表格，refs 仍然非空，导致下一轮继续强制工具调用。
- ACK 延迟：飞书 ack 表情依赖 renderer 消费 `input_received` 事件。原链路中 Router 在调用 renderer/Master 前先执行 `resolveInboundContext`，resolver 可能拉父消息、根消息、引用文档，导致 ACK 被这些 I/O 拖后。

## Fix

- ReAct 内增加 `imRefsRead` 状态：只有在 `feishu_api` 成功执行读取正文/表格/多维表/资源下载类 action 后，后续轮次才不再用 IM refs 强制 `tool_choice=required`。
- Router renderer 路径改为先订阅 renderer，再立即广播 `input_received`，然后再执行 `resolveInboundContext` 和 Master 处理。
- 给 IM 处理入口增加 `ackAlreadyEmitted` 标记，避免 Router 提前发 ACK 后 Master 再重复广播一次。

## Evidence

- 新增回归测试：
  - `TestRefsForToolChoice_IMRefsExpireAfterSuccessfulRead`
  - `TestIsSuccessfulIMReferenceRead`
  - `TestRouter_RendererPath_InputReceivedBeforeResolver`
- 验证命令：
  - `env GOCACHE=/tmp/go-build go test ./internal/master -run 'Test(DetectToolChoice|RefsForToolChoice|IsSuccessfulIMReferenceRead|EvaluateRequiredGuard|ShouldSuppressStreamPartial|EmitAssistantMessage|Master_InputReceivedBroadcast)' -count=1`
  - `env GOCACHE=/tmp/go-build go test ./internal/channel ./internal/channel/feishu ./internal/bootstrap -count=1`
  - `env GOCACHE=/tmp/go-build go test ./... -count=1`

## Status

DONE
