## Why

`chat-ui-polish` 已于 2026-04-20 归档（`openspec/changes/archive/2026-04-20-chat-ui-polish/`），代码层四连验证全绿（tsc/vitest/openspec validate/amber grep）。但归档快照里 **10 条视觉验证任务未完成**（0b.4 / 1.3 / 5.5 / 6.4 / 9.4 / 9.5 / 9.6 / 9.7 / 9.10 / 10.2），全部依赖人工浏览器 + 移动端操作。用户明确预警"视觉层有多处问题"，不可假定无事。

独立 change 的理由：
1. **证据链闭合** — 视觉 findings 需与具体截图、页面、状态、dark/light、iOS/桌面对号入座；混进已 archived change 的 tasks.md 会污染原始快照。
2. **工作量不确定** — findings 数量未知，可能触发若干 spec delta（内联代码域、heading margin、ErrorCard 视觉一致性）。把修补和验证绑在同一个 change 内，才符合"一个 change 一件事"的 openspec 纪律。
3. **预警信号** — 用户 2026-04-20 反馈"有很多问题"，进入视觉 QA 前就要准备按页面归档 findings，而不是一条一条边看边随手修。

## What Changes

### A. 视觉验证全流程（不引入新契约）

- 执行 `chat-ui-polish` 归档里未勾选的 10 条：dev server 启动、chip+block 截图、dark 模式对比、10+ 页面主题色巡查、iOS Safari 渲染、reference-*.png 并排对比、i18n runtime 检查。
- 生成 `findings.md`（本 change 目录内）按页面 × 主题（light/dark）× 平台（桌面/iOS）归档每条问题，包含：截图路径、复现步骤、期望、实际、严重度（blocker/major/minor）。

### B. 已知风险点前置验证（`chat-ui-polish` 10.2）

- **B1** ErrorCard 在 danger ring + hover underline 下视觉一致（与 ToolExecutionBlock 对比）
- **B2** `.message-content h1 mt-10` 在聊天消息首尾是否过空；首子归零已就位，若整体仍显松散需要单独讨论是否回到"chat 稠密"尺度（此前 index.css 注释里的理由）
- **B3** 内联代码 pill 不再在 Canvas `MarkdownRenderer` / Guide 页面冒蓝（scope 从 `.prose :not(pre) > code` 改为 `.message-content code:not(pre code)` 的补修效果）

### C. findings 处置策略

按严重度分流：
- **blocker / major**：本 change 内补修（新增 tasks 并实现，可能产生 spec delta 修正 `chat-ui-polish` 的 scope/scenario）
- **minor**：列入 `findings.md` 的"延后栏"，不在本 change 解决；close 时转存为 issues 或下一 sprint 的 proposal 草案
- **spec 漂移**：若发现既有 `openspec/specs/chat-ui-polish/spec.md` 与实际落地不符（例如 scenario 声称"单一 DOM 结构"但视觉发现双重渲染），在本 change 的 `specs/` 里加 MODIFIED delta 修正

### D. Impact

- 代码变更：取决于 findings —— 预估 0-N 处补修，主要触碰 `index.css` 与 `MessageBubble.tsx::ErrorCard` / `ToolExecutionBlock.tsx` / `.message-content h*` 规则
- spec 变更：**可能** MODIFIED `openspec/specs/chat-ui-polish/spec.md` 的 scenarios（如发现合约偏差）；否则 `--skip-specs` 归档
- 用户动作：提供截图 + 设备访问（桌面 / iPhone / iPad）
- Non-goals：不引入新组件、不扩 brand palette、不改 i18n 文案（如 runtime 检查发现缺 key 才补）

## 退出条件

1. `findings.md` 封版，blocker 和 major 全部在本 change 内被 tasks 覆盖并完成
2. `openspec validate chat-ui-polish-visual-qa --strict` exit=0
3. `npx tsc --noEmit` + `npx vitest run` 全绿
4. 如产生 spec delta，`openspec validate openspec/specs/chat-ui-polish` 对应 scenarios 与代码一致
