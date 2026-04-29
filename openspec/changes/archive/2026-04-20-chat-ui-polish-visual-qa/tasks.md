## 0. 前置准备

- [ ] 0.1 启动 `frontend` dev server（`npm run dev`），确认无编译错误。
- [ ] 0.2 新建 `openspec/changes/chat-ui-polish-visual-qa/findings.md`，采用 page × theme × platform 的三维表格骨架（见 proposal A）。
- [ ] 0.3 准备截图工具 + 设备：桌面 Chrome / Safari，iPhone / iPad Safari（用户提供）。

## 1. 桌面 light mode 主路径（对应 `chat-ui-polish` 9.4 + 5.5 + 6.4）

- [ ] 1.1 对话页：发送一条触发 bash 工具调用的消息，截三张图并归档到 `findings.md` 附件路径：
  - (a) running `<ToolInvocationChip>` + `<ToolExecutionBlock>` 未展开态（chip 带 Loader2 spinner、block 按钮 disabled + duration 占位 `--`）
  - (b) success 折叠态（chip 带 Settings icon、block 带 Check + duration）
  - (c) 展开后 args + result 双 section
- [ ] 1.2 验证 chip 配色：icon `var(--accent-600)`，bg `var(--accent-subtle)`，error 态走 `var(--danger)`。
- [ ] 1.3 验证 block 切换按钮：`text-[var(--text-secondary)]` + hover underline（D3），**非** accent-blue。
- [ ] 1.4 内联代码蓝 pill 验证（5.5）：消息正文含反引号代码 `foo` 时背景 `var(--accent-subtle)` + 文字 `var(--accent-700)`。
- [ ] 1.5 heading 留白验证（6.4）：消息正文含 `## 主要差异` / `### 子项` 时，h2/h3 间距观感是否符合 D5 `mt-8 mb-4` / `mt-6 mb-3`；若显松散记录到 findings B2。

## 2. 桌面 dark mode（对应 9.5）

- [ ] 2.1 重复 1.1 三张截图，切 dark mode。
- [ ] 2.2 验证 chip 文字色 `var(--accent-300)`、block 的 Check 图标色、error ring 的 `var(--danger)` 在暗底对比度可读。
- [ ] 2.3 验证内联代码 dark override（`html.dark .message-content code:not(pre code)`）是否生效。
- [ ] 2.4 验证 `index.css:665+` 删除后的 `.tool-call-card` 规则无视觉回流（原 dark 规则 `html.dark .tool-call-args` 也一并删除，确认无 FOUC）。

## 3. 全站主题色巡查（对应 9.6，至少 10 页面）

每页各 light + dark 一组截图，填 findings.md。重点找黄色残留：

- [ ] 3.1 `/chat`（对话页，已在 1.x 覆盖）
- [ ] 3.2 `/` 或 Dashboard
- [ ] 3.3 Settings 各子页（permissions / mcp / llm-providers / agent-timeout）
- [ ] 3.4 Admin 子页（UserList / LLMProviders / PromptManager / UsageStats）
- [ ] 3.5 HITL Approval 卡
- [ ] 3.6 Sessions 列表
- [ ] 3.7 Agents 列表
- [ ] 3.8 Skills 列表
- [ ] 3.9 Guide 页（重点：验证 B3 — 内联代码不应出现蓝 pill）
- [ ] 3.10 Replay（scene decor 允许保留 amber，验证 ReplayControls / ReplayTimeline / EventDetailPanel 已转 token）
- [ ] 3.11 Canvas MarkdownRenderer（重点：验证 B3 — 内联代码不应出现蓝 pill）

## 4. iOS Safari（对应 9.7）

- [ ] 4.1 iPhone Safari 打开首页 + 对话页，验证 body 渐变 `#F8FAFF → #F2F5FC` 无抖动、`100vh` 视口正确。
- [ ] 4.2 iPad Safari 同上。
- [ ] 4.3 若出现 R8 所述 `background-attachment: fixed` 类问题，触发方案 B 并在本 change 内补修。

## 5. reference-*.png 并排对比（对应 9.10）

- [ ] 5.1 用 1.1 和 3.1 的截图与 `openspec/changes/archive/2026-04-20-chat-ui-polish/reference-chat.png` 并排贴图。
- [ ] 5.2 与 `reference-theme.png` 并排对比。差异 ≥ minor 的全部进 findings。

## 6. i18n runtime（对应 1.3）

- [ ] 6.1 切 zh ↔ en，打开 chat 页，触发工具调用 —— 确认 `tools.clickToExpand` / `tools.clickToCollapse` / `tools.invoked` / `tools.input` / `tools.output` / `tools.error` / `tools.truncated` / `chat.generating` 文案两种语言都正确。
- [ ] 6.2 console 无 missing key 警告。

## 7. 已知风险点验证（proposal B）

- [ ] 7.1 **B1** ErrorCard 观感：对比 `<ToolExecutionBlock status="error">` 和 `<ErrorCard>`，danger ring + 文字按钮应视觉一致；否则记录并补修。
- [ ] 7.2 **B2** heading 间距：如果首尾消息体 h1/h2 留白真的显得过空，讨论是否回退到"chat 稠密"尺度（`1rem/0.875rem/0.75rem`）并更新 design.md D5。
- [ ] 7.3 **B3** 内联代码不冒蓝：Canvas `MarkdownRenderer` / Guide 页若仍有蓝色 pill，说明 scope 收紧 `.message-content code:not(pre code)` 未生效，需补调或 root cause 分析。

## 8. findings 归类与修补

- [ ] 8.1 `findings.md` 按 blocker / major / minor / spec-drift 分流。
- [ ] 8.2 blocker + major：本 change 内开新 tasks 实现修补；修补后回到 1-7 对应项重新验证。
- [ ] 8.3 minor：留在 findings.md 延后栏，不阻塞归档；close 时转存为 issues 或列入下一 sprint proposal 草案。
- [ ] 8.4 spec-drift：在 `openspec/changes/chat-ui-polish-visual-qa/specs/chat-ui-polish/spec.md` 里写 MODIFIED delta 修正 scenarios；否则归档时用 `--skip-specs`。

## 9. 关闭闭环

- [ ] 9.1 `npx tsc --noEmit` exit=0
- [ ] 9.2 `npx vitest run` 全绿
- [ ] 9.3 `openspec validate chat-ui-polish-visual-qa --strict` exit=0
- [ ] 9.4 findings.md blocker + major 全部勾完；截图与 reference-*.png 并排图附全
- [ ] 9.5 运行 `openspec archive chat-ui-polish-visual-qa`（含 `--skip-specs` 如无 delta）

## 10. 实际执行摘要（2026-04-20 归档）

实际执行范围 vs. 原始计划：

- ✅ **执行了**：桌面 light mode `/chat` 工具调用三态人工目检（图 1-4）、F-001~F-006 findings 登记、F-002/F-003/F-004/F-005 四个 minor 修补
- ⏭️ **跳过**：dark mode 截图（用户判断观感无问题）、3.1-3.11 全站主题色巡查（用户判断不必要）、4.x iOS Safari 验证、5.x reference-*.png 并排对比、6.x i18n runtime
- 🔁 **转交**：F-001（后端 WebSocket 未推 `tool_call.start`）属后端缺陷，本 change 不修，转下一 sprint
- ⏸️ **延后**：F-006（工具输出 markdown 渲染策略）属产品决策，留给下一 sprint

详见 `findings.md`。
