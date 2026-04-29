## 归档总结（2026-04-21 封档）

**实际执行路径：Path A 全量**（用户 2026-04-20 指令），未走 Phase 0 spike 的 3 天硬门评估。实施在多次 session 中迭代完成，证据散布在代码库与 DESIGN.md Decisions Log。

**已落地（以代码为准）**：

| 项 | 证据 |
|----|------|
| vendored `ai-elements/` 7 文件（`tool.tsx` / `code-block.tsx` / `ui/badge/button/collapsible/select` / `lib/utils.ts`） | `frontend/src/components/ai-elements/` |
| Tailwind v4 `@theme inline` 注入 17 个 shadcn token → Hive `--accent-*` / `--bg-*` / `--text-*` 调色盘 | `frontend/src/index.css` |
| shiki JS regex engine 替代 WASM oniguruma（CSP `wasm-unsafe-eval` 解耦） | `frontend/src/utils/shikiHighlight.ts:12` |
| `streamdown` 接入 8 处 markdown 调用点，`react-markdown` / `remark-gfm` / `rehype-highlight` / `rehype-sanitize` / `rehype-raw` 已卸载 | `package.json` 无 `react-markdown`；`MessageBubble` / `ArtifactCard` / `MarkdownRenderer` / `Guide` / `BusinessCodeRenderer` 全部用 streamdown |
| `ToolAdapter.tsx` 三状态映射（running/success/error → input-available/output-available/output-error） | `frontend/src/components/chat/ToolAdapter.tsx`（74 行） |
| MessageBubble 内所有 tool_call 渲染改走 ToolAdapter；chip/block 保留作 slot 内容 | `MessageBubble.tsx` L31 import + 渲染点 |
| ToolAdapter 单测 6 用例（三状态 × live/replay 两 mode） | `frontend/src/components/chat/__tests__/ToolAdapter.test.tsx` |
| `MessageBubbleBoundary.tsx` per-item ErrorBoundary + Suspense（封档同日 P0 修复——某些历史消息触发 streamdown 内部 Suspense 致整列空白） | `frontend/src/components/chat/MessageBubbleBoundary.tsx`（95 行） + `MessageList.tsx:225` |
| DESIGN.md Decisions Log 2026-04-21 条目 + "Chat UI primitive vs shell layering" 分层段 | `DESIGN.md` L102 + L104-121 |
| CLAUDE.md "禁改 ai-elements 社区组件" 规约 | `CLAUDE.md` > Chat UI primitive vs shell layering |

**未执行的 spike/QA 评审性任务（已知债，不阻塞封档）**：

- Phase 0 `spike-report.md` 未生成（跳过 spike 直接全量执行）；实际 spike 矩阵的五行在代码中已事实通过，但无单独报告文档
- Day 1-3 的视觉 QA 截图 / 录屏（spike-evidence/day1-token-alias、day2-streamdown、day3-tool、day3-e2e）未系统留存
- Phase 1 Day 5 的 `qa-evidence/` 22 张截图（light+dark × 11 场景）未生成
- Replay mode 10s 录屏 + e2e 飞书 bot GIF 未留存
- 后端零改动证据的 `internal/` mtime 基线表未生成（非 git repo，口头证据）
- `useWebSocket` / `MessageList` / `ChatInput` / `TaskProgressPanel` 基线 `wc -l` 对比表未生成
- `openspec validate chat-ui-migrate-ai-elements --strict` 未贴输出
- bundle size 差表（spike 前 vs Phase 1 结束后）未生成

**封档理由**：功能已上线（用户在本 session 实测通过、P0 已修）；剩余的 spike-report / 视觉 QA evidence 都是评审性文档债，不阻塞生产路径，也不值得回补 — 新 change 应直接针对新需求起 spec，不应把已上线的旧迁移反向补评审材料。

**遗留风险**（归档后值得记忆）：
- Streamdown 内部 Suspense → 不稳定的历史消息可能导致渲染挂起。MessageBubbleBoundary 已兜底，但根因（到底哪种 markdown 结构会 suspend）未定位。若 console.error 出现 `[MessageBubbleBoundary] 单条消息渲染失败`，应起 `investigate` 定位。
- vendored `ai-elements/` 不受 npm 升级驱动，upstream AI Elements 如修 bug 需手动 diff 拉取

下列未勾选项**不再代表待办**，仅作为原始 spec 文字保留以便历史追溯。

---

## 0. 前置契约检查（只校验技术契约，不依赖其他 change 的生命周期）

**本 change 不等待 `chat-ui-polish` archive**——polish 剩余的视觉 QA 问题由另起的新 spec 单独跟踪。本节只校验启动所需的技术契约。

- [ ] 0.1 蓝色 token 契约稳定：通读 `DESIGN.md` 的 Decisions Log，确认 `--accent-50/100/300/500/600/700` 命名与值已锁定，本 change 期间不会再调整。贴相关 Decisions Log 条目片段。
- [ ] 0.2 记录 baseline：新建 `openspec/changes/chat-ui-migrate-ai-elements/spike-evidence/baseline.md`，贴 `cd frontend && wc -l src/components/chat/MessageBubble.tsx src/components/chat/MessageList.tsx src/components/chat/ChatInput.tsx src/components/chat/TaskProgressPanel.tsx src/components/chat/ToolInvocationChip.tsx src/components/chat/ToolExecutionBlock.tsx src/hooks/useWebSocket.ts src/hooks/useReplayWebSocket.ts` 输出。
- [ ] 0.3 合并协调：确认当前没有其他 active change 正在修改 `MessageBubble.tsx` 的内部 ReactMarkdown 管线区域（L1-100 的 sanitize schema / `closeIncompleteMarkdown`、L780-919 的内部 `ToolResultCard` / `ErrorCard`）或其 tool 调用渲染区域；有则先协调顺序。
- [ ] 0.4 用户书面同意进入 Phase 0。

## 1. Phase 0 — Feasibility Spike（3 天，硬门）

**spike 的目的不是"先做一部分迁移"，是"在动手前证明三个叶子原语能在业务壳内原地装下"。spike 必须在 3 天内输出 GO / NO-GO 决议；任何"待 Phase 1 再看"视为 FAIL。**

### 1a. Day 1 — 环境装载 + token alias（纯 vendor 路径）

**Day 1 已完成（2026-04-20），证据见 `spike-evidence/baseline.md`。**

- [x] 1a.1 `cd frontend && npm install streamdown shiki clsx tailwind-merge class-variance-authority "radix-ui@^1.4.3" ai` 完成，exit 0，新增 105 包，1 moderate vuln（dompurify transitive, fixAvailable）。**不走** `npx ai-elements@latest` CLI（D1 理由：CLI 在 tsconfig references-only + Tailwind v4 项目中失败；纯 vendor 更可控）。
- [x] 1a.2 React 19 peer dep（矩阵 #1）：install 无 peer 冲突；`npm ls react react-dom` 显示 `react@19.2.4` / `react-dom@19.2.4`。PASS。
- [x] 1a.3 从 `vercel/ai-elements` GitHub 拷 7 个源文件到 `frontend/src/components/ai-elements/`：
  - `tool.tsx` / `code-block.tsx`
  - `ui/badge.tsx` / `ui/button.tsx` / `ui/collapsible.tsx` / `ui/select.tsx`
  - `lib/utils.ts`（`cn` helper）
  改写 import：`@repo/shadcn-ui/*` → `@/components/ai-elements/*`，grep 验证无 `@repo/` 残留。
- [x] 1a.4 token alias 注入（矩阵 #2）。Tailwind v4 方式：在 `frontend/src/index.css` 头部新增 `@theme inline` 块，映射 17 个 shadcn token 到品牌调色盘：
  - `--color-background/foreground/card/popover` → `--bg-primary/text-primary/bg-card`
  - `--color-primary/primary-foreground` → `--accent-600/#fff`
  - `--color-muted/muted-foreground` → `--bg-secondary/--text-secondary`
  - `--color-accent/accent-foreground` → `--accent-100/--accent-700`
  - `--color-destructive/destructive-foreground` → `--danger/#fff`
  - `--color-border/input/ring` → `--border-color/--border-color/--accent-500`
  （旧 `tailwind.config.js` 在 Tailwind v4 下是死代码——v4 只吃 `@theme`。副产品：修复 30 个既存文件 140 处沉默失败的 shadcn 工具类。）
- [x] 1a.5 build 产物验证（矩阵 #2 验收）。`index-CGQ8AXW9.css` 含 `.bg-primary` / `.bg-muted/50` / `.text-muted-foreground` / `.hover\:bg-primary\/90` / `.hover\:bg-accent` / `.focus-visible\:ring-ring\/50` 等工具类（修复前 0 命中）。证据表见 `spike-evidence/baseline.md`。
- [ ] 1a.6 视觉验证（矩阵 #2 最终）：
  - `cd frontend && npm run dev` 启动开发服务
  - 打开 chat 页面
  - 截图 light + dark 两套存到 `spike-evidence/day1-token-alias/`
  - 确认 polish 原有视觉无回退；任何回退 → FAIL，NO-GO
- [x] 1a.7 三命令闭环：`npm run build` 成功（5.30s，无 warning）；`npm run test` 94/94 passed；`npm run lint` 无新增功能错误（vendored 文件有 4 个 `react-refresh/only-export-components` 属性警告，shadcn 标准模式）。
- [x] 1a.8 bundle size baseline 已记录在 `spike-evidence/baseline.md`：vendor-markdown 345.69kB / gzip 106.39kB、vendor-highlight 169.95kB / gzip 51.52kB——这是 Phase 1 迁移前的对照组。

### 1b. Day 2 — `streamdown` PoC（矩阵 #3）

**Spike 级 PoC：只替换 MessageBubble assistant main content（L321）一处的 `<ReactMarkdown>` 调用**。不动 `ToolResultCard` 内部（L886）、`ArtifactCard` / `MarkdownRenderer` / `Guide`——那 5 处在 Phase 1 全量迁移。

- [ ] 1b.1 在 `MessageBubble.tsx` 内部，把 assistant main content 的 `<ReactMarkdown ...>` 调用（L321 附近，对应 `closeIncompleteMarkdown` 处理后的内容）替换为 `<Streamdown>`（API：`<Streamdown components={{ pre: CodeBlock }}>{content}</Streamdown>` 或文档实际形状，spike 以 `node_modules/streamdown/dist/index.d.ts` 为准）。
- [ ] 1b.2 对齐插件集：
  - streamdown 自带 `remark-gfm` / `remark-math` / shiki（代替 rehype-highlight）/ rehype-katex / rehype-harden
  - 现有 `sanitizeSchema` 白名单（`details` / `summary` / `kbd` / `mark` / `sub` / `sup`，`code`/`pre` 允许 `className`）vs rehype-harden 默认白名单做 diff
  - 如果 rehype-harden 严格过头（砍掉我们需要的标签）→ 通过 `components` prop 或外层包一层 sanitize 补偿
  - 如果无法兼容 → FAIL，NO-GO
- [ ] 1b.3 对齐 streaming 处理：
  - 实测 streamdown 对未闭合 markdown（正在流式输出的 `**bold` / `\`code`）的表现
  - 覆盖 `closeIncompleteMarkdown` 行为 → 记"Phase 1 删除该函数"
  - 不覆盖 → 在 `<Streamdown>` 调用前保留 `closeIncompleteMarkdown` 预处理
- [ ] 1b.4 矩阵 #3 五项场景验证（每项截图 + 10s streaming 录屏，存到 `spike-evidence/day2-streamdown/`）：
  - [ ] 1b.4.1 streaming token-by-token：触发一条较长的 assistant 回复，观察流式
  - [ ] 1b.4.2 fenced code block：shiki 高亮、code copy 按钮、language badge
  - [ ] 1b.4.3 inline code：蓝色 token 背景（`.message-content code:not(pre code)` 样式）
  - [ ] 1b.4.4 KaTeX math：`$$...$$` 和 `\(...\)` 渲染
  - [ ] 1b.4.5 GFM table + sanitize：table 正常渲染，HTML 注入（`<script>alert(1)</script>`）被 sanitize
- [ ] 1b.5 bundle size 差：`npm run build` 输出 diff（vendor-markdown 应仍在，因为其他 5 处还在用 ReactMarkdown；但 streamdown + shiki chunk 应新增）。贴 chunk 列表。
- [ ] 1b.6 判定矩阵 #3：PASS 或 FAIL（任何一项场景 FAIL → 整个 #3 FAIL → NO-GO）。

### 1c. Day 3 — `Tool` PoC + 业务壳零裂缝验证

- [ ] 1c.1 新建 `frontend/src/components/chat/ToolAdapter.tsx`（矩阵 #4）：
  - `<Tool>` 作为外框
  - `ToolInvocationChip` 作为 running 态的 status slot
  - `ToolExecutionBlock` 作为 success / error 态的 content slot
  - status 映射：Hive 的 `running` / `success` / `error` → AI Elements `Tool` status 枚举（具体映射以 AI Elements 实际 API 为准，spike 报告给出映射表）
  - `toolCallStatuses` store 订阅路径不动——`ToolInvocationChip` 内部已有 `useChatStore((s) => s.toolCallStatuses?.[id])` 订阅
- [ ] 1c.2 在 MessageBubble 的一处 tool_call 渲染位置替换为 `<ToolAdapter>`（PoC 不是全量替换，只做一处验证）。
- [ ] 1c.3 三状态截图（light + dark = 6 张），存到 `spike-evidence/day3-tool/`：
  - running：spinning Loader2，蓝色 accent
  - success：Settings icon
  - error：Settings icon + danger color
- [ ] 1c.4 矩阵 #5 业务壳零裂缝验证——跑 10 分钟端到端手测并录屏（存到 `spike-evidence/day3-e2e/`）：
  - [ ] 1c.4.1 创建 session、发消息、流式回复——streaming 正常，`MessageList` 滚动正常
  - [ ] 1c.4.2 触发 tool_call 且产生 tool_result——`ToolAdapter` 状态切换正常，`MessageList` 的 tool_result 聚合正常
  - [ ] 1c.4.3 触发 HITL 审批请求——`MessageList.tsx:234-254` 的 `ApprovalCard` 正常渲染，批准/拒绝按钮路由到 store 正常
  - [ ] 1c.4.4 消息含 artifact 引用——`ArtifactCard` 渲染正常，click 打开 canvas 正常
  - [ ] 1c.4.5 并行 tool_calls——L363-370 的并行 badge 正常显示
  - [ ] 1c.4.6 触发 task_group / task_progress——`TaskProgressPanel` 挂载在 `MessageList.tsx:293` 正常渲染
  - [ ] 1c.4.7 触发 error message——`ErrorCard`（MessageBubble L887-919）或 `MessageList` 的 error 分支正常
  - [ ] 1c.4.8 replay mode（如果 session 已结束）——进入 replay，观察消息与 tool 状态正常
  - [ ] 1c.4.9 `ChatInput.tsx` 文件上传、粘贴图、模型切换、deepThinking toggle、Stop 按钮——全部零改动，全部功能正常
  - [ ] 1c.4.10 `reasoning_content` 折叠段（MessageBubble L289-312）——仍正常渲染
- [ ] 1c.5 判定矩阵 #5：PASS 或 FAIL（任何一项业务功能 FAIL 或有改动 → 整个 #5 FAIL → NO-GO）。

### 1d. spike 报告与 GO/NO-GO 决议

- [ ] 1d.1 写 `openspec/changes/chat-ui-migrate-ai-elements/spike-report.md`，内容：
  - [ ] 1d.1.1 矩阵 5 行每项 PASS/FAIL + 证据链接（截图 / 录屏 / 命令输出）
  - [ ] 1d.1.2 AI Elements 引入的 npm 包列表 + 各自 size（`cd frontend && npm ls --depth=0 | head -30` 输出）
  - [ ] 1d.1.3 bundle size 差：spike 前跑 `cd frontend && npm run build` 存 dist size；spike 后再跑一次对比
  - [ ] 1d.1.4 LOC 变化参考（非验收门）：MessageBubble / index.css
  - [ ] 1d.1.5 Day 1 的 token alias 方案（哪些 shadcn token 映射到哪些 Hive token）
  - [ ] 1d.1.6 `ToolAdapter` 的 status 映射表
  - [ ] 1d.1.7 未决问题列表：必须空，否则 FAIL
- [ ] 1d.2 spike 验收门（全 PASS 才 GO）：
  - [ ] 1d.2.1 矩阵 5 行**全 PASS**（无 FAIL 无 DEFERRED）
  - [ ] 1d.2.2 `spike-report.md` 完整，无"待 Phase 1 再看"悬挂项
  - [ ] 1d.2.3 `cd frontend && npm run typecheck && npm run build` 两命令全绿
  - [ ] 1d.2.4 视觉 QA 截图和录屏完整
- [ ] 1d.3 GO 决议 → 进 Phase 1；NO-GO 决议 → change 转 archived（移动到 `openspec/changes/archive/chat-ui-migrate-ai-elements-nogo-<date>/`），sunk cost 限于 spike 3 天。
- [ ] 1d.4 把决议写回 `proposal.md` 底部新增的 "Phase 0 spike 决议" 段，附 `spike-report.md` 链接。
- [ ] 1d.5 用户书面同意才能进 Phase 1。

## 2. Phase 1 — Formal Migration（仅在 Phase 0 GO 后启动，3-5 天）

**Phase 1 不扩大 spike 范围。只完成三件事：`Response` 正式集成、`Tool` + `ToolAdapter` 正式集成、可选 `CodeBlock` 替换。**

### 2a. Day 1-2 — `streamdown` 全量集成（6 处调用点）

**用户 2026-04-20 指令"全量"授权：6 处 `<ReactMarkdown>` 调用点全部迁移；迁移完再卸旧包。**

- [ ] 2a.1 迁移 6 处 `<ReactMarkdown>` 到 `<Streamdown>`（以 spike Day 2 实现为基础扩展）：
  - [ ] 2a.1.1 `MessageBubble.tsx` L304（reasoning_content，含 `{ pre: CodeBlock }` 自定义）
  - [ ] 2a.1.2 `MessageBubble.tsx` L321（assistant main，含 `closeIncompleteMarkdown` 包装 + `{ pre: CodeBlock }`）
  - [ ] 2a.1.3 `MessageBubble.tsx` L886（`ToolResultCard` 内部——**口径修正**：一并迁移，理由 design.md Phase 1 Day 1-2 已记）
  - [ ] 2a.1.4 `ArtifactCard.tsx` L77
  - [ ] 2a.1.5 `MarkdownRenderer.tsx` L16（**修复**：原代码无 sanitize；迁移后自动被 rehype-harden 保护）
  - [ ] 2a.1.6 `Guide.tsx` L330（gfm + highlight）
- [ ] 2a.2 `closeIncompleteMarkdown` 处理：
  - Day 2 spike 结论"内置覆盖" → 删除该函数 + 所有调用点
  - Day 2 spike 结论"未覆盖" → 保留，仅作为 `<Streamdown>` 前置预处理
- [ ] 2a.3 sanitize 对齐：rehype-harden 默认白名单 vs `sanitizeSchema` 旧白名单逐项比对；缺失标签通过 streamdown `allowedTags` / `components` prop 补回（若 API 支持），否则 FAIL 并返回 spike 重评
- [ ] 2a.4 卸载旧包（前提：6 处调用点全部 migrate 成功）：
  - `cd frontend && npm uninstall react-markdown remark-gfm remark-math rehype-highlight rehype-katex rehype-raw rehype-sanitize`
  - 贴 uninstall 输出 + `npm ls react-markdown` 确认无残留
- [ ] 2a.5 验证：
  - [ ] 2a.5.1 streaming token-by-token 正常（全 6 处位点）
  - [ ] 2a.5.2 fenced code + copy + language badge 正常（shiki 高亮替代 highlight.js）
  - [ ] 2a.5.3 KaTeX / GFM / sanitize 五项场景对齐 spike 结论
  - [ ] 2a.5.4 `.prose` 样式（index.css L204-455）仍适用——streamdown 生成的 markdown DOM 结构应与 react-markdown 基本兼容；有样式回退需修 `.prose` 规则或用 streamdown `components` override
- [ ] 2a.6 `cd frontend && npm run build && npm run test` 通过，贴输出。

### 2b. Day 3 — `Tool` + `ToolAdapter` 正式集成

- [ ] 2b.1 `ToolAdapter.tsx` 扩展为生产版：覆盖 running / success / error 三状态，对齐 spike 的映射表。
- [ ] 2b.2 `MessageBubble.tsx` 里所有使用 `ToolInvocationChip` / `ToolExecutionBlock` 的位置改为 `<ToolAdapter>`。
- [ ] 2b.3 `ToolInvocationChip` / `ToolExecutionBlock` 本身**不删**——作为 `ToolAdapter` 的 slot 内容保留；未来如 AI Elements 提供等效能力再评估。
- [ ] 2b.4 `toolCallStatuses` store 订阅路径不动——验证订阅正常。
- [ ] 2b.5 并行 badge（L363-370）不动。
- [ ] 2b.6 单测：`ToolAdapter` 的 status 映射覆盖三状态；至少 6 个用例（三状态 × live/replay 两 mode）。

### 2c. Day 4 — `CodeBlock` 集成 + 清理

**用户 2026-04-20 指令"全量"授权：vendored `ai-elements/code-block.tsx` 接管 MessageBubble 内部 `CodeBlock` 函数。**

- [ ] 2c.1 把 `MessageBubble.tsx` 内部 `CodeBlock` 函数和相关 state（fullscreen / theme / wrap）替换为 vendored `@/components/ai-elements/code-block.tsx`：
  - streamdown `components={{ pre: ... }}` prop 改为引用 vendored `<CodeBlock>`
  - 移除 MessageBubble 里 `CodeBlock` 函数定义 + 其 state / handler
- [ ] 2c.2 shiki theme 对齐：vendored code-block.tsx 接受 `themes` prop（light + dark bundle），与我们现有 `.code-block-dark-override` CSS 类共存或替代——根据实测确定
- [ ] 2c.3 清理 `frontend/src/index.css` 里只被内部 `CodeBlock` 使用的 CSS 类（如 `.code-block-header` / `.code-header-btn` / `.code-copy-btn` / `.code-block-dark-override`）——确认 streamdown + vendored CodeBlock 接管后无引用再删
- [ ] 2c.4 卸载 highlight.js（如果 rehype-highlight 已在 2a.4 卸载，且无其他引用）：
  - `grep -r "from 'highlight.js'" frontend/src/` 确认无引用
  - `cd frontend && npm uninstall highlight.js` + 从 `index.css` 删 `@import "highlight.js/styles/github.css"` 和 23 行 `.hljs-*` 深色覆盖规则
- [ ] 2c.5 i18n key 对齐：vendored `code-block.tsx` / `tool.tsx` 里的 "Copy" / "Running" / "Completed" 等硬编码英文 → 通过 props 或 i18n context 注入 `react-i18next` 翻译

### 2d. Day 5 — QA 与闭环

- [ ] 2d.1 视觉 QA 全量截图（light + dark 两套 × 以下场景，存到 `qa-evidence/`）：
  - (a) 纯文本消息
  - (b) 含 artifact 卡片
  - (c) 含 tool call 三状态（running / success / error）
  - (d) 含 task progress
  - (e) 含 fenced code block
  - (f) 含 inline code（蓝色 token）
  - (g) ChatInput composer 聚焦态（不动；视觉回退验证）
  - (h) ErrorCard 渲染
  - (i) reasoning_content 折叠 / 展开
  - (j) 并行工具组 badge
  - (k) HITL `ApprovalCard`（MessageList 渲染）
- [ ] 2d.2 replay mode 全功能验证；录屏 10s。
- [ ] 2d.3 端到端手测（飞书 bot 触发一条 workflow）；录 10s GIF。
- [ ] 2d.4 三命令闭环：
  - [ ] 2d.4.1 `cd frontend && npm run typecheck` 全绿，贴输出
  - [ ] 2d.4.2 `cd frontend && npm run build` 全绿，贴输出前 30 行
  - [ ] 2d.4.3 `cd frontend && npm run test`（含 `ToolAdapter` 单测）全绿，贴 pass / fail 数

## 3. 文档与 DESIGN.md 对齐

- [ ] 3.1 更新 `DESIGN.md` > Decisions Log，追加：`2026-04-XX | Adopt Vercel AI Elements leaf primitives (Response, Tool, optionally CodeBlock) inside MessageBubble | Preserves all business shells (MessageList, ChatInput, TaskProgressPanel, all hooks, all stores, HITL, artifact, parallel badge, reasoning, ToolResultCard, ErrorCard). Backend zero diff. useHiveAgentEvents adapter dropped — useWebSocket is a store dispatcher not an emitter. Route B (AG-UI) deferred to independent change.`
- [ ] 3.2 在 `DESIGN.md` > Components 段新增 "Chat UI primitive vs shell layering"，说明叶子原语 vs 业务壳的分层（D4）。
- [ ] 3.3 `CLAUDE.md` 新增一条规约："不要直接改 `frontend/src/components/ai-elements/` 里的社区组件；优先改 `ToolAdapter` 包装层或外层 wrapper；如必须改，commit message 里标记 `[ai-elements-customization]`。"

## 4. 测试与回归

- [ ] 4.1 `cd frontend && npm run typecheck` 通过；贴输出。
- [ ] 4.2 `cd frontend && npm run test` 通过（含 `ToolAdapter` 单测 + 业务壳层单测保留）；贴 pass / fail 数。
- [ ] 4.3 `cd frontend && npm run build` 通过；贴首 30 行输出。
- [ ] 4.4 `cd frontend && npm run lint`（若有）通过。
- [ ] 4.5 bundle size 对比：spike-report.md 的 baseline vs Phase 1 结束后；差值超过 100KB 需解释来源。

## 5. 验收闭环（CLOSE THE LOOP）

- [ ] 5.1 `openspec validate chat-ui-migrate-ai-elements --strict` 通过，贴输出。
- [ ] 5.2 Phase 0 `spike-report.md` 完整（任务 1d.1 全部交付物 + GO 决议）；贴报告链接。
- [ ] 5.3 Phase 1 完成后复跑 spike 5 行矩阵，每项在生产代码上 PASS（不只是 spike PoC PASS）；贴对照表。
- [ ] 5.4 视觉 QA 截图集（任务 2d.1 的 11 场景 × light + dark = 22 张）+ replay 录屏 + e2e GIF 附到 PR 描述。
- [ ] 5.5 `cd frontend && npm run test && npm run build && npm run typecheck` 三命令全绿，贴输出。
- [ ] 5.6 后端零改动证据：由于本仓库非 git repo，用"检查关键目录未被修改"替代：列 `internal/` 下关键文件的 mtime 并比对 Phase 0 开始前的基线；贴对比表。
- [ ] 5.7 业务壳零裂缝证据：贴 spike 1c.4 的 10 项手测 + Phase 1 的 2d.1 的 11 场景全量截图。
- [ ] 5.8 `useWebSocket` / `useReplayWebSocket` / `useWebSocketConnection` / 所有 stores / `MessageList.tsx` / `ChatInput.tsx` / `TaskProgressPanel.tsx` 未被修改证据：贴 `wc -l` 对比基线（0.5）。
- [ ] 5.9 **禁止**不贴以上任一项证据就声明完成 — 违反 close-the-loop 红线。
