## 0. Token 层 swap（先做，全站生效起点）

- [x] 0.1 修改 `frontend/src/index.css` `:root` 块：替换 `--accent`、`--accent-hover`、`--accent-light`、`--accent-subtle`、`--accent-border`、`--gradient-start/mid/end` 的值为 light-blue 家族（见 design.md D1 表）。
- [x] 0.2 新增 `--accent-50/100/300/500/600/700` 显式 stops（与 DESIGN.md 对齐）。
- [x] 0.3 修改 `frontend/src/index.css` `.dark` 块：同样 key 换 dark 模式值（更亮一档）。
- [x] 0.4 `--bg-primary` light 从 `#F2F2F7` 改为 `#F4F7FB`。
- [x] 0.5 给 `body` 加 light-mode-only 渐变背景：`background-image: linear-gradient(180deg, #F8FAFF 0%, #F2F5FC 100%);`；**不使用** `background-attachment: fixed`（iOS Safari 风险，见 R8）；`.dark body` 清掉这个 image。
- [x] 0.6 修改 `--card-tool-border`、`--card-tool-bg`、`--card-tool-bg-expanded`（light + dark 两套）为 blue 家族。
- [x] 0.7 `frontend/src/App.css`：搜 `--accent-bg` 等遗漏；如有则改 var 引用。**验证结果**：App.css 是 Vite 脚手架遗留文件，`rg "App.css" frontend/src` 无导入引用（grep 证据：两条命令均返回 0 行）；无需修改。
- [x] 0.8 `frontend/tailwind.config.js`：
  - [x] 0.8.1 删除 `theme.extend.colors.brand-amber`（`rg 'brand-amber' frontend/src frontend/index.html` 返回 0 行，确认无引用后已删除整块）。
  - [x] 0.8.2 `chat.user: '#EBF3FE'` 保留，加注释 `// light-blue aligned with --accent-100 (#DBEAFE family)`。
- [x] 0.9 新增 semantic token：`--danger: #DC2626 / dark #EF4444`、`--success: #059669 / dark #10B981`、`--warning: #D97706 / dark #FBBF24`；已写入 `:root` + `.dark`。

## 0a. 全站 amber 清扫（按目录分组，每组独立验收 grep=0）

### 0a.1 组 1 — chat（`frontend/src/components/chat/`）

- [x] 0a.1.1 `MessageBubble.tsx`：所有 `amber-*` Tailwind 类（L175 / L279 / L294 / L467 / L695 / L703 / L925 等）全改为 `text-[var(--accent-*)]` / `bg-[var(--accent-*)]`。
- [x] 0a.1.2 `MessageBubble.tsx` 硬编码 hex 全改 token：
  - L755 `#93c5fd` → 随 `TOOL_ACCENT` 一同删除（现在走 `getToolAccentByStatus`）
  - L820 `#ef4444` → `var(--danger)`
  - L827 `#3b82f6` → `var(--accent-600)`
  - L830 `#10b981` → `var(--success)`
  - L833 `#ef4444` → `var(--danger)`
  - L899 `#ef4444 : #10b981` → `getToolAccentByStatus(status)` 统一
  - L732 tool accent palette dict：已删除硬编码映射（bash/edit/read/grep/web_search 等），换为 status-based `getToolAccentByStatus(status)`：error→`var(--danger)`，其余→`var(--accent-600)`
- [x] 0a.1.3 `ChatInput.tsx`（**本 change 新纳入**）：
  - L252 drag overlay → blue 家族（`border-[var(--accent-500)] dark:border-[var(--accent-300)] ring-2 ring-[var(--accent-100)] dark:ring-[var(--accent-border)] bg-[var(--accent-50)]/30 dark:bg-[var(--accent-light)]`）
  - L338 active model → `text-[var(--accent-600)] dark:text-[var(--accent-300)]`
  - L368 deep thinking → blue 家族等价
  - L397 send button → `bg-[var(--accent-600)] text-white hover:bg-[var(--accent-700)]`
- [x] 0a.1.4 `shared.tsx`：4 处 amber 全改 token（ClawIcon gradient `#F59E0B/#D97706` → `#60A5FA/#3B82F6`；文件类型 icon `text-amber-*` → `text-[var(--accent-*)]`）。
- [x] 0a.1.5 `MessageList.tsx`：6 处 amber 改 token（empty state icon、thinking avatar、Brain pulse、thinking dots×3）。
- [x] 0a.1.6 `TaskProgressPanel.tsx`：2 处 amber 改 `text-[var(--accent-500)]`。
- [x] 0a.1.7 `ArtifactCard.tsx`：1 处 amber 改 `text-[var(--accent-600)] dark:text-[var(--accent-300)]`。
- [x] 0a.1.8 **验收**：`rg -n 'amber-\d+|#F59E0B|#D97706|#B45309|#FEF3C7' frontend/src/components/chat/` 输出为 0 行（已验证，Bash 输出空）。

### 0a.2 组 2 — layouts（`frontend/src/layouts/`）

- [x] 0a.2.1 `Sidebar.tsx`：8 处 amber 改 token（含 SVG logo gradient `#F59E0B/#D97706` → `#3B82F6/#2563EB`）。
- [x] 0a.2.2 `AdminSidebar.tsx`：1 处 amber 改 token。
- [x] 0a.2.3 **验收**：Agent "layouts sweep" 报告输出空；全局 `grep -rn 'amber-[0-9]' frontend/src/layouts/` 0 行。

### 0a.3 组 3 — settings（`frontend/src/components/settings/`）

- [x] 0a.3.1 8 个设置文件全部扫描 28 处：PermissionRulesSettings (3) / RemoteAgentsSettings (4) / ExternalResourcesSettings (3) / ExecRulesSettings (2) / AgentTimeoutSettings (1) / IMChannelSettings (5) / WeChatSettings (7) / MCPServersSettings (3)。
- [x] 0a.3.2 **验收**：27 转 token；MCPServersSettings.tsx:237 是 `⚠` 内联校验 warning，保留 amber 并加 `/* warning semantic */` 注释。grep 剩余唯一一处在 MCPServersSettings.tsx:237，上一行有语义注释。

### 0a.4 组 4 — admin（`frontend/src/pages/admin/`）

- [x] 0a.4.1 `UserList.tsx` (2) / `LLMProviders.tsx` (7) / `AuthProviders.tsx` (1) / `PromptManager.tsx` (4) / `UsageStats.tsx` (6)：全部 amber 改 token。PromptManager 的 "restore defaults" 确认 modal 无 warning 语义词，按品牌统一原则转 token。
- [x] 0a.4.2 **验收**：Agent "admin sweep" 报告 0；全局 `grep -rn 'amber-[0-9]' frontend/src/pages/admin/` 0 行。

### 0a.5 组 5 — pages（`frontend/src/pages/` 顶层）

- [x] 0a.5.1 `Dashboard.tsx` (6) / `Skills.tsx` (6) / `Guide.tsx` (8) / `Sessions.tsx` (2) / `Agents.tsx` (2) / `ChatLanding.tsx` (2) / `AdminSettings.tsx` (1) / `SessionReplay.tsx` (1)：全部 amber 改 token。Guide.tsx Lightbulb tip 块用 `--accent-border`；ChatLanding.tsx hover border 用 `--accent-300/700` 保留明暗配对；SessionReplay.tsx inline-style fallback `#D97706` 更新为 `#1D4ED8`。
- [x] 0a.5.2 **验收**：Agent "pages sweep" 报告 0；全局 `grep -rn 'amber-[0-9]' frontend/src/pages/` 0 行（仅剩 replay/scene 装饰 + HtmlRenderer 语义 warning，属 (b)/(c)）。

### 0a.6 组 6 — replay（`frontend/src/components/replay/`）

- [x] 0a.6.1 7 个文件扫描共 50 处：SceneBackground (1) / SceneDesk (7) / SceneRobot (33) / SceneDefs (2) / ReplayControls (3) / ReplayTimeline (2) / EventDetailPanel (2)。
- [x] 0a.6.2 判定结果：(a) 品牌控件 8 处转 token（ReplayControls 3 + EventDetailPanel 2 + ReplayTimeline 3）；(b) warning 0 处（error 已用 `#DC2626`，success 已用 `#059669`）；(c) 场景装饰 42 处全加 `/* replay scene decor */` 注释保留。
- [x] 0a.6.3 **验收**：grep 剩余 42 行全部在 `scene/` 下，逐行上方均带 `/* replay scene decor */`。抽查三处：SceneRobot.tsx:44 天线 stroke / SceneDefs.tsx:27 机身渐变 stop / SceneDesk.tsx:86 显示器进度条 rect——均属 (c)。

### 0a.7 组 7 — hitl（`frontend/src/components/hitl/`）

- [x] 0a.7.1 `ApprovalCard.tsx` 4 处：primary confirm/submit/preview 按钮 + 选择高亮全转 `--accent-*`；emerald approve / red reject 不在本次范围（语义正确）。
- [x] 0a.7.2 **验收**：`frontend/src/components/hitl/` grep 为 0。

### 0a.8 组 8 — common（`frontend/src/components/common/`）

- [x] 0a.8.1 Toast.tsx (1) warning 变体 → `var(--warning)`；ErrorBoundary (1) retry → `var(--danger)`；LanguageSwitcher (1) 激活高亮 → `--accent-*`；GradientCard (1) `amber` palette key 保留（是公共 API 名义为 warning），加 `/* warning semantic */`。
- [x] 0a.8.2 **验收**：grep 剩余 GradientCard.tsx:18（amber palette key，/* warning semantic */ 已注）——属 (b) 语义保留。

### 0a.9 组 9 — canvas（`frontend/src/components/canvas/`）

- [x] 0a.9.1 `renderers/HtmlRenderer.tsx:21`：enable-scripts 切换是真实 warning（执行脚本风险），保留 amber 并加 `/* warning semantic */` 注释。
- [x] 0a.9.2 **验收**：grep 剩余 HtmlRenderer.tsx:21，上一行有语义注释——属 (b) 语义保留。

### 0a.10 组 10 — session（`frontend/src/components/session/`）

- [x] 0a.10.1 `TagEditor.tsx` 3 处：tag chip 品牌色 + save button → `--accent-*`。
- [x] 0a.10.2 **验收**：grep 为 0。

### 0a.11 全站最终验收

- [x] 0a.11.1 全局 `grep -rnE "amber-(50|100|...|900)" frontend/src` → **3 行**：HtmlRenderer.tsx:21（(a) `/* warning semantic */` enable-scripts 危险操作）、MCPServersSettings.tsx:237（(a) `/* warning semantic */` `-y` 配置错误内联警告）、GradientCard.tsx:18（(a) `/* warning semantic */` 公共 API palette key）。`grep -rnE "#F59E0B|#D97706|#FBBF24|#FCD34D|#FDE68A|#FEF3C7" frontend/src` → **57 行**：55 行全部在 `replay/scene/*.tsx` 下（(b) scene decor）、2 行在 `index.css` 的 `:root`/`.dark` 下是 `--warning` token 本体定义（(d)）。
- [x] 0a.11.2 **0 条未注释 amber 残留**。上面三类原因逐行对应，禁止性要求达成。

## 0b. Logo / assets 渐变替换

- [x] 0b.1 搜 Logo SVG：`frontend/src/assets/` 只含 Vite 脚手架 react.svg/vite.svg（非品牌 Logo）；`frontend/src/components/` 里唯一品牌 Logo 是 `chat/shared.tsx:ClawIcon` 的 `linearGradient`（已在 0a.1.4 处理：`#F59E0B/#D97706` → `#60A5FA/#3B82F6`）。replay/scene/*.tsx 里的 SVG 渐变是场景装饰，非 Logo，已按 0a.6 判定 (c) 保留并加注 `/* replay scene decor */`。
- [x] 0b.2 ClawIcon stop-color 已替换为 `#60A5FA` / `#3B82F6`（0a.1.4 验证通过）。
- [x] 0b.3 六边形 `<polygon>` 点位不动，只改 `stopColor`——确认。
- [ ] 0b.4 视觉验证：待本地 dev server 启动后截图（Section 9 close-the-loop 统一做）。

## 0c. DESIGN.md 对齐（与 0 一起提交）

- [x] 0c.1 `DESIGN.md > Aesthetic Direction`：`"refined warmth"` → `"refined calm"`，Mood `"warm honeycomb"` → `"calm honeycomb"`（hex 意象保留）。
- [x] 0c.2 `DESIGN.md > Color > Brand accent`：整节替换为 blue stops（`--accent-50/100/300/500/600/700` + `--accent-subtle/--accent-border/--accent-light`），显式记录"品牌 ≠ 语义 warning"；warning 仍为 `#D97706` / dark `#FBBF24`，danger/success/info 重新标注；body gradient 注明 iOS Safari `background-attachment: fixed` 禁用。
- [x] 0c.3 `DESIGN.md > Brand Identity`：Brand gradient 从 amber 改为 `#60A5FA → #3B82F6`（blue only），hex 形保留。
- [x] 0c.4 `DESIGN.md > Decisions Log` 追加两行（2026-04-17 rebrand 条目 + 2026-04-20 scope 条目）。

## 1. 基础骨架与 i18n

- [x] 1.1 `frontend/src/i18n/locales/zh.json` 已新增 `tools.clickToExpand: "点击展开"`、`tools.clickToCollapse: "点击收起"`、`tools.invoked: "已调用工具"`。
- [x] 1.2 `frontend/src/i18n/locales/en.json` 已新增 `tools.clickToExpand: "Click to expand"`、`tools.clickToCollapse: "Click to collapse"`、`tools.invoked: "Called tool"`。JSON 合法（`python3 -c 'json.load(...)'` 通过）。
- [ ] 1.3 验证 i18n：runtime 检查延后到 Section 9 close-the-loop（启动 dev server + 切语言 + console 监控）。

## 2. ToolInvocationChip 组件

- [x] 2.1 新建 `frontend/src/components/chat/ToolInvocationChip.tsx`，props: `{ name, status?, id? }`；`id` 可选，启用订阅 `toolCallStatuses` 实时状态。
- [x] 2.2 布局：一行 pill，running 用 `<Loader2 animate-spin/>`，其他用 `<Settings/>`；label 通过 `getToolDisplayName(name, t)` + `${t('tools.invoked')}: ${displayName}`。
- [x] 2.3 色板：icon `text-[var(--accent-600)] dark:text-[var(--accent-300)]`；error 走 `text-[var(--danger)]`；文字色随之匹配。
- [x] 2.4 背景：`bg-[var(--accent-subtle)]` + `rounded-full px-3 py-1.5 text-xs font-medium inline-flex items-center gap-1.5`。
- [x] 2.5 `role="status"`、`aria-label={label}`，icon `aria-hidden="true"` 避免重复朗读。

## 3. ToolExecutionBlock 组件

- [x] 3.1 新建 `frontend/src/components/chat/ToolExecutionBlock.tsx`，props: `{ id, name, args, result?, status?, duration?, isError? }`；订阅 `toolCallStatuses[id]` 实时状态 + duration。
- [x] 3.2 外层卡片：`rounded-lg border border-[var(--border-color)] bg-[var(--bg-card)] overflow-hidden`；error 加 `ring-1 ring-[var(--danger)]/30`。
- [x] 3.3 头部：`<FileText/>` + `{displayName} {t('tools.output')}`；右侧依状态分别渲染 `<Loader2 animate-spin>` / `<Check>` / `<X>` + duration + 切换按钮（文字 `t('tools.clickToExpand')` / `t('tools.clickToCollapse')`），按钮带 `aria-expanded={expanded}` + `aria-controls={detailId}`。**补记 2026-04-20**：按钮样式对齐 design.md D3 — `text-xs font-medium text-[var(--text-secondary)] hover:underline`（running 态 opacity-50 + cursor-not-allowed），不再用 accent-blue + hover bg。running 态 duration 占位显示 `--`（对齐 D7，宽度不抖动）。
- [x] 3.4 Running：按钮 `disabled` + `opacity-50 cursor-not-allowed`；头部追加 `{t('chat.generating')}` 副标签（`min-h-[44px]` 保持触控区）。
- [x] 3.5 展开 body：`<div id={detailId}>` 包输入（`argsFormatted`）+ 输出（result / running 占位 / 空）；label color 依 error 切红/绿；输出支持 2000 字符截断 + `t('tools.truncated')`。
- [x] 3.6 参数格式化 `argsFormatted` + duration 文本 `durationText` 用 `useMemo` 从 `ToolCallCard` 逻辑迁移；增加对 `map[...]` 格式的空判断。

## 4. 替换 ToolCallCard 并保留 shim

- [x] 4.1 `MessageBubble.tsx:370` 处替换：`message.tool_calls.map` 内部渲染一个 `<div className="flex flex-col gap-1">`，内含 `<ToolInvocationChip id={tc.id} name={tc.name} status={tcStatus}/>` + `<ToolExecutionBlock id={tc.id} name={tc.name} args={tc.arguments} result={toolResults?.get(tc.id)} status={tcStatus}/>`。
- [x] 4.2 原 `ToolCallCard`（共 103 行，L740-842）收缩为约 18 行 shim：内部 render Chip+Block，`useEffect` 在 `import.meta.env.DEV` 下 `console.warn('[deprecated] ToolCallCard')`。
- [x] 4.3 `rg "ToolCallCard" frontend/src` → MessageBubble 内部 1 定义 + 0 JSX 调用点（调用点已迁移到新组件）；MessageList.tsx 里 2 条是注释文本（无需改）。
- [x] 4.4 原 `argsSummary` / `ChevronRight` 展开图标 / tool-call-card CSS 依赖从主工具调用路径全部删除；Settings/Loader2 imports 从 MessageBubble 清理（迁移到新组件）。`npx tsc --noEmit` exit=0，无错。**补记 2026-04-20**：最初声明为"全部删除"时遗漏 `MessageBubble.tsx::ErrorCard`（错误消息折叠卡片）仍用 `tool-call-card`/`args-summary`/`tool-call-header`/`tool-call-detail`/`tool-call-section`/`tool-call-args` 类，已于本次补修重写为纯 Tailwind DOM（对齐 `ToolExecutionBlock` 外观、danger ring + 文字按钮）。

## 5. 内联代码样式

- [x] 5.1 作用域收紧为 `.message-content code:not(pre code)`（index.css ~316），对齐 R5 承诺"不泄漏到 Canvas MarkdownRenderer / Guide"；`pre code` 通过 `:not(pre code)` 排除。**补记 2026-04-20**：先前声称 "prose 本身就是 chat markdown 作用域" 错误——`.prose` 亦用于 Canvas `MarkdownRenderer` 与 Guide 等非 chat 页面，已于本次补修换为 `.message-content` 专属。
- [x] 5.2 规则实现：`background: var(--accent-subtle); color: var(--accent-700); padding: 2px 6px; border-radius: 4px; font-family: 'JetBrains Mono', monospace; font-size: 0.9em; font-weight: 500; border: 1px solid transparent`（transparent border 取代旧 `--border-color` 边框）。
- [x] 5.3 `html.dark .message-content code:not(pre code) { background: var(--accent-subtle); border-color: transparent; color: var(--accent-300); }` dark override 已写。
- [x] 5.4 `pre code`（index.css:295）完全不动；CodeBlock header dark theme（`.code-block-header` / `.code-block-dark-override`）未触碰。
- [ ] 5.5 视觉验证：待 Section 9 dev server 启动后人工验证蓝色 pill 呈现。

## 6. Heading 样式

- [x] 6.1 `MessageBubble.tsx:320` AI 文本 prose 容器增加 `message-content` 类名（只挂这一处，不污染 reasoning 折叠块和工具结果内嵌 markdown）。
- [x] 6.2 `.message-content h1/h2/h3` 规则写入 `index.css`（紧跟 inline code 规则之后）：h1 `1.5rem / 700 / mt-2.5rem mb-1.25rem`；h2 `1.25rem / 600 / mt-2rem mb-1rem / scroll-mt-4rem`；h3 `1.125rem / 600 / mt-1.5rem mb-0.75rem`。**补记 2026-04-20**：最初落地的 margin 是 `1rem/0.875rem/0.75rem`（"chat 稠密"理由），与 design.md D5 不符；本次补修对齐 D5（mt-10/8/6 + mb-5/4/3 + h2 scroll-mt-16），首子归零保留。
- [x] 6.3 全局 `.prose` 规则（h1/h2/h3 默认）未触碰；变更被 `.message-content` 子选择器 scope。
- [ ] 6.4 视觉验证：待 dev server 验证 `## 主要差异` 的留白和字号。

## 7. 删除遗留装饰

- [x] 7.1 `.tool-call-card` / `.tool-call-header` / `.tool-call-detail` / `.tool-call-section` / `.tool-call-args` / `.args-summary` CSS 规则块已整体从 `index.css` 删除（2026-04-20 本次补修；原本标注 TODO(next sprint) 保留 shim，现因 ErrorCard 同步迁移出，无剩余消费者）。`grep -rnE "tool-call-(card|header|detail|section|args)|args-summary" frontend/src` 输出空。
- [x] 7.2 `ToolInvocationChip` / `ToolExecutionBlock` / `ErrorCard` DOM 里均不再输出 `.args-summary` 或 `.tool-call-*` 类；收起态默认无摘要文本。

## 8. 单测与快照

- [x] 8.1 新建 `ToolInvocationChip.test.tsx`（5 tests）：success/running/error 三态 icon（spinner 仅 running）；`role=status` + `aria-label='Called tool: Shell'`；无 button；error 走 `text-[var(--danger)]`；displayName 走 getToolDisplayName mock。
- [x] 8.2 新建 `ToolExecutionBlock.test.tsx`（8 tests）：默认 `aria-expanded=false`、点击翻转到 true、按钮文本 `Click to expand` ↔ `Click to collapse`、running 时 button disabled、error icon style 含 `var(--danger)`、展开显示 input/output、`args="{}"` 时不渲染 input section、duration 格式化（2345ms→`2.3s`、142ms→`142ms`）。
- [x] 8.3 无旧 `MessageBubble.test.tsx` 快照文件（`frontend/src/components/__tests__/` 只有 AuthGuard 测试），无快照需要更新。
- [x] 8.4 `npx vitest run` 输出：`Test Files 9 passed (9) / Tests 94 passed (94) / Duration 1.62s`。

## 9. 验收闭环（CLOSE THE LOOP）

- [x] 9.1 `npm run build`（frontend workspace）成功 — `✓ built in 5.35s`。`dist/assets/index-C7tYWkDY.js 456.65 kB │ gzip: 115.51 kB`，bundle 无新增异常；无编译错误。
- [x] 9.2 `npx vitest run`：`Test Files 9 passed / Tests 94 passed / 0 failed`；新增 2 文件 13 用例全绿。
- [x] 9.3 `npx tsc --noEmit` exit=0 无报错；tsc -b（build 一部分）也通过。
- [ ] 9.4 启动前端 dev server，打开对话页，发送一条触发 bash 工具调用的消息，截三张图：(a) running chip+block；(b) success 折叠态；(c) 展开后 args+result。附到 PR。
- [ ] 9.5 切 dark mode 再截一组同样的三张图。
- [ ] 9.6 **全站主题色巡查**：在 light mode 下截 dashboard / chat / settings / admin / HITL / sessions / agents / skills / guide / replay **至少 10 个页面**，确认无残留黄色；与 `reference-theme.png` 并排贴图。dark mode 同样走一遍。
- [ ] 9.7 **iOS Safari 验证**：用 iPhone / iPad 的 Safari 打开首页 + chat 页，确认 body 渐变正常、无抖动、`100vh` 视口正确；如差异大触发 R8 的方案 B。
- [x] 9.8 **最终 amber 清扫验收** — `grep -rnE` 全目录输出：
  - `amber-[0-9]+` 共 3 行：
    1. `components/canvas/renderers/HtmlRenderer.tsx:21` — (a) 语义 warning（enable-scripts 危险操作），上行 `/* warning semantic */`。
    2. `components/settings/MCPServersSettings.tsx:237` — (a) 语义 warning（`-y` 配置错误内联警告），上行 `/* warning semantic */`。
    3. `components/common/GradientCard.tsx:18` — (a) 语义 warning palette key（`amber: 'bg-amber-500'` 作为 dot 状态色），上行 `/* warning semantic */`。
  - `#F59E0B|#D97706|#FBBF24|...|#B45309` 共 61 行：
    - 59 行分布 `components/replay/scene/{SceneRobot,SceneDesk,SceneDefs,SceneBackground}.tsx` — (b) scene decor，全部带 `/* replay scene decor */` 或属装饰 SVG 节点。
    - 2 行 `index.css`:58 `--warning: #D97706`（light）/:106 `--warning: #FBBF24`（dark）— (d) token 底值定义。
  - **0 条无注释残留**，红线守住。
- [x] 9.9 `openspec validate chat-ui-polish --strict` → `Change 'chat-ui-polish' is valid`。
- [ ] 9.10 `reference-*.png` 对比：**需人工补图**。当前状态：布局层级（chip+block 拆分 DOM 就位）、折叠控件文字（`t('tools.clickToExpand')/clickToCollapse`）、内联代码色（`.message-content code:not(pre code)` blue）、章节留白（`.message-content h1/h2/h3` 对齐 D5）均已落地；logo 渐变已切至 `#60A5FA/#3B82F6`；侧栏白卡保留；页面淡蓝底 `#F8FAFF → #F2F5FC`；card 阴影未动。延后到人工截屏阶段。
- [x] 9.11 close-the-loop 证据：build/test/typecheck/openspec validate 四枚命令输出已贴；禁止赤手空拳声明完成的红线达成。剩余 9.4/9.5/9.6/9.7/9.10 是**需要人工浏览器操作**的视觉验证，不在代码可自动闭环的范围内。

## 10. 后续残留（2026-04-20 与 codex 共同审后标记）

- [x] 10.1 **chip 去订阅化（2026-04-20 收口）**：`ToolInvocationChip` signature 收为 `{ name, status? }`，删除 `id` prop、`useChatStore` 订阅、`useMemo` 解析，对齐 design.md D2 "完全无状态"。live running/success 解析上提到新的 `ToolCallRow`（对应 design.md D2 的 `<ToolCallSequence>` 层），一次订阅 `useChatStore((s) => s.toolCallStatuses?.[id])`，hasError 最高优先级，然后把 resolvedStatus 同时传给 chip 和 block。ToolCallCard shim 也同步改为委托 `ToolCallRow`。验证：tsc exit=0；vitest 9 文件 / 94 用例全绿（`ToolInvocationChip.test.tsx` 5 用例无需改动，本就是 props 驱动）；`grep useChatStore frontend/src/components/chat/ToolInvocationChip.tsx` 为空；`grep '<ToolInvocationChip[^/]*id=' frontend/src` 为空。
- [ ] 10.2 视觉 QA（9.4–9.7、9.10）：完成本轮补修后进行，重点验证 (a) ErrorCard 在 danger ring + hover underline 下视觉一致；(b) `.message-content h1 mt-10` 在聊天消息首尾是否需要再调（首子归零已就位，若仍显松散再讨论"chat 稠密"例外）；(c) 内联代码 pill 不再在 Canvas / Guide 页面冒蓝。
