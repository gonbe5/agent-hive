## Why

`frontend/src/components/chat/MessageBubble.tsx` **919 行**（以 `wc -l` 为准，2026-04-20 实测）里，大约 35% 是 commodity 渲染代码：ReactMarkdown + remark + rehype 管线、CodeBlock 组件（copy / fullscreen / theme / wrap 状态）、tool 调用的展示框架。这部分能力 Vercel 的 AI Elements（2025-08 发布，shadcn 安装式）已有社区维护的成熟实现——`Response`（streaming markdown）、`Tool`（tool 调用框架）、`CodeBlock`（code 展示）。

**继续自研这部分 = 重复造轮子**。维护 remark/rehype 插件版本、sanitize schema、streaming 闭合补全、code 高亮主题切换，都是在消耗团队带宽。如果社区有稳定基线，不用是浪费。

但是——**本 change 只换叶子原语，不动业务容器**。前一版方案把 `Conversation` 替换 `MessageList`、`PromptInput` 替换 `ChatInput`、`useHiveAgentEvents` 包装 `useWebSocket` 一起打包上去，实测代码后这些都不成立：

- `MessageList.tsx` 337 行不是"只做滚动和容器"，它持有 HITL inline approval 渲染（L234-254，`inlineApprovals` from `useChatStore`）、tool_call / tool_result 聚合与去重、standalone `tool` 行抑制、last-user regenerate 定位。用 `Conversation` 替换会让业务裂缝散出来。
- `ChatInput.tsx` 410 行不是输入框，是 compose 面板——25MB / 10 文件上传、拖拽、粘贴图、模型选择器、deepThinking 切换、Stop 态、IME。用 `PromptInput` 替换 = compose 面板重写，不是 aesthetic 对齐。
- `useWebSocket.ts` 311 行**不是事件发射器，是 store 分发器**。它直接 `addHITLRequest` / `addChatMessage` / `setToolCallStatus` / `setStreaming`，含 RAF batching、错误限流、跨会话过滤、Sprint 12 race 修复的 20+ 行注释语义。`useHiveAgentEvents` 包装它返回 `HiveEvent[]` 这个设计**没有事件流可包**——要做就是把分发逻辑整体改成纯转换器，这是独立的大重构，不属于本 change。

**A 路线（AI Elements）vs B 路线（CopilotKit + AG-UI）**：Hive 是企业内部 agent control center，不是开放 agent 平台；Go 后端实现完整 AG-UI 协议成本高（2-3 周）且和 `im-streaming-reply` 冲突。AI Elements 是 shadcn 方式安装（组件进自己代码库），未来真要走 B 路线只需加后端 encoder + 改 hook 实现，不需要重写 UI。A 路线是眼前能落地的抓手。

## What Changes

### 前置契约（只依赖技术契约，不依赖其他 change 的生命周期）

**本 change 只依赖一个已落地的技术契约：light-blue token 体系**。

- `DESIGN.md` Decisions Log 中锁定的 `--accent-50/100/300/500/600/700` 命名与值在本 change 期间保持稳定
- shadcn 安装后在 `index.css` 注入的 alias（`--primary` / `--secondary` / `--accent` / `--muted` / `--border` / `--ring`）引用这套 token

**显式不依赖**：
- 不依赖 `chat-ui-polish` 的 archive 状态——该 change 的剩余视觉 QA 问题由另起的新 spec 单独跟踪，不阻塞本 change
- 不依赖 polish 的视觉回归收尾——本 change 的视觉 QA 独立进行
- 不依赖任何 amber 残留的清理状态——本 change 不触碰 `GradientCard.tsx` / `Dashboard.tsx` / `ChatInput.tsx` 等 polish 涉及文件

**合并协调（软约束，不阻塞启动）**：
- 如果有其他 change 同时修改 `MessageBubble.tsx` 的内部 ReactMarkdown 管线或 `CodeBlock` 函数区域，需要先协调；除此以外的修改（如 inline code 样式）与本 change 不重叠

### Phase 0 — Feasibility Spike（3 天，硬门）

**目的**：不是"先做一部分"，是"证明三个叶子原语能在业务壳内原地装下"。spike 不通过则本 change 立即 archive。

**Spike 范围**：

1. `npx ai-elements@latest` 安装到 `frontend/src/components/ai-elements/`（shadcn 方式；组件进仓库不进 package.json）
2. 在 `MessageBubble.tsx` 内部替换三个叶子原语之一：优先 `Response`（最直接、影响面最小）
3. 视觉 + 行为验证：light / dark token alias 生效，streaming 态无回退
4. 如果 `Response` PoC 通过，继续做 `Tool`（包裹现有 `ToolInvocationChip` / `ToolExecutionBlock`）和 `CodeBlock` 的 PoC；不通过则 NO-GO

**Spike 验收矩阵（5 行，全 PASS 才进 Phase 1，不设 DEFERRED）**：

| # | 能力 | 必须验证 | 判定 |
|---|------|---------|------|
| 1 | React 19.2 peer dep | `cd frontend && npm install` 通过，`npm ls react react-dom` 无 UNMET | PASS / FAIL |
| 2 | Token alias 生效 | `--primary: var(--accent-600)` 等 alias 在 light + dark 都渲染为蓝色；chat 页面截图无视觉回退 | PASS / FAIL |
| 3 | `Response` 承接 streaming markdown | 内嵌到 MessageBubble，与现有 `parseMessageContent` / `parseMessageContentWithSkeleton` 产物兼容；streaming token-by-token、code 高亮、math (KaTeX)、GFM、sanitize 全部行为对齐；10 秒录屏 | PASS / FAIL |
| 4 | `Tool` 适配现有 chip/block | 新建 `ToolAdapter.tsx`，`Tool` 作为外框、`ToolInvocationChip` / `ToolExecutionBlock` 作为 status slot；三状态 (running/success/error) 截图 | PASS / FAIL |
| 5 | 被替换的业务壳零裂缝 | spike 期间所有其他业务（HITL、artifact 联动、并行 badge、ToolResultCard、ErrorCard、MessageList 聚合）**零改动**且功能正常；跑 10 分钟端到端手测 | PASS / FAIL |

**任何一项 FAIL = NO-GO**，change 转 archived。不允许"待 Phase 1 再看"。

### Phase 1 — Formal Migration（spike PASS 后启动，3-5 天）

**仅做三件事**：

1. **`Response` 替换 MessageBubble 内部 ReactMarkdown 管线**——把 `message.content` 流经 `<Response>`；保留现有 sanitize schema、remark/rehype 插件作为 `Response` 的 config（或对比 AI Elements 默认，选宽松的那个）；内部自研 CodeBlock 可选替换为 AI Elements `CodeBlock`
2. **`Tool` 作为外框包裹 chip/block**——新建 `ToolAdapter.tsx`；`ToolInvocationChip` / `ToolExecutionBlock` 作为 status slot；对齐 shadcn 语义和 Hive 的 `toolCallStatuses` store
3. **内部 CodeBlock 函数退役**（可选，取决于 `Response` 是否自带 CodeBlock）——如果 `Response` 已替代 code 高亮 + copy，MessageBubble 里的 CodeBlock 删掉；否则保留

**保留原样（一个字节都不动）**：

- `MessageList.tsx`（容器 + HITL inline approval + tool 聚合 + 滚动 + regenerate）
- `ChatInput.tsx`（compose 面板 + 文件上传 + 拖拽 + 粘贴 + 模型选择）
- `TaskProgressPanel.tsx`（任务进度条；不在本 change 范围）
- 所有 7 个 zustand stores（`chat` / `hitl` / `taskProgress` / `agentActivity` / `session` / `canvas` / `replay`）
- 所有 hook（`useWebSocket` / `useReplayWebSocket` / `useWebSocketConnection`）
- `ArtifactCard` / `MermaidBlock` / `shared`
- `MessageBubble` 内部的 `ToolResultCard`（L787-884）和 `ErrorCard`（L887-919）——是 MessageBubble 内部函数，`Response` 只替换 markdown 主体，不触这两个
- HITL 相关逻辑全部不动
- 后端：**零改动**。不动 EventBus payload、不改 `BroadcastGenericMessage`、不动任何 Go 代码

### 被显式删除的前方案内容（需要作者理解为什么）

| 前方案内容 | 删除原因 |
|----------|---------|
| `useHiveAgentEvents` hook | `useWebSocket` 是 store 分发器不是 emitter；包装它 = 重写分发逻辑为纯转换器，属独立重构，不属本 change |
| `Conversation` 替换 `MessageList` | `MessageList.tsx:234-254` 持有 HITL inline approval + `inlineApprovals` store 订阅；聚合 tool_call / tool_result 去重；`Conversation` 原语无这些 slot |
| `PromptInput` 替换 `ChatInput` | `ChatInput.tsx` 是 compose 面板（~200 行文件/粘贴/拖拽/模型选择），不是输入框；替换 = compose 面板重写，不是 aesthetic 对齐 |
| `Task` 替换 `TaskProgressPanel` | 不在主战场；`TaskProgressPanel` 106 行自研维护成本低，社区原语映射收益小 |
| `Reasoning` 原语集成 | 后端 LLM 响应无 `reasoning_content` 字段（非 o1 系列模型）；等后端能力落地后另起 change |
| HITL 挂在 Tool footer slot | HITL 实际挂在 `MessageList.tsx` 的 `ApprovalCard`，不是 `MessageBubble` 的 tool 里；前方案 D3 行 #4 的归属就是错的 |
| ErrorCard 替换为 Message variant | ErrorCard (L887-919) 和 ToolResultCard (L787-884) 是 MessageBubble **内部函数**不是独立组件；"替换"需要先做提取重构，属于未 scope 工作 |

## Capabilities

### New Capabilities

- `chat-ui-migrate-ai-elements`：规定 AI Elements 叶子原语（`Response` / `Tool` / 可选 `CodeBlock`）在 MessageBubble 内部的集成边界，确立"只换叶子不动业务壳、不动 hooks、不动 stores、不动容器"的迁移原则。

### Modified Capabilities

无 spec delta。本 change 只动实现，不改任何 capability 接口。

## Impact

- **代码**：
  - 新增 `frontend/src/components/ai-elements/`（shadcn 方式，~20 个文件进仓库）
  - 新增 `frontend/src/components/chat/ToolAdapter.tsx`（~50 行，包裹现有 chip/block）
  - 修改 `frontend/src/components/chat/MessageBubble.tsx`（919 → 预计 700-800 行，减量来自 CodeBlock 和 ReactMarkdown 管线外迁；**行数不作为验收门**）
  - 可能新增 `frontend/components.json`（shadcn 初始化生成；review 后入库）
  - 可能修改 `frontend/tailwind.config.js` 和 `frontend/src/index.css`（shadcn alias 注入；必须和 polish 的蓝色 token 体系拉通）
- **不修改**：`MessageList.tsx` / `ChatInput.tsx` / `TaskProgressPanel.tsx` / 所有 hooks / 所有 stores / 所有其他 chat 子组件 / 所有 Go 代码
- **后端**：**零改动**。后端改动必须另起 change（如未来的 `agui-protocol-adoption`）
- **依赖**：
  - 可能新增 `@ai-sdk/react`（AI Elements runtime 伙伴；spike Day 1 确认实际需求）
  - **不引入** server SDK（`ai` / `@ai-sdk/openai` 等；Hive 后端是 Go）
  - 实际新增依赖清单和 bundle size 差在 spike 报告中给出
- **i18n**：AI Elements 原语可见文案需要 zh/en 两套 key；spike 必须给出注入路径 PoC
- **测试**：
  - spike：每个验收矩阵项有截图或录屏证据
  - Phase 1：被改造的 `MessageBubble` 有单测覆盖 streaming / code / math / GFM 四个场景；`ToolAdapter` 有 status 映射单测
  - 视觉 QA：light + dark 两套截图，确认 polish 的蓝色 token 在 AI Elements 原语上正确生效
- **风险**：
  - **AI Elements `Response` 的 markdown 管线和 Hive 的 sanitize schema / 插件集不兼容** → spike 矩阵 #3 抓出来；不兼容则 NO-GO 或在 `Response` 外再包一层
  - **shadcn 自带 token 和 Hive 蓝色 token 冲突** → spike 矩阵 #2 的 alias 方案
  - **和 `chat-ui-polish` 的时间窗冲突** → 硬前置约束明示；polish 未 archive 则本 change 不启动
  - **AI Elements 版本稳定性** → 2025-08 发布，shadcn 安装方式降低依赖风险；升级走手动 review diff
- **向后兼容**：所有 hook / store / 组件外部接口不变；只有 MessageBubble 内部 markdown / code 渲染路径改变
- **回滚**：每个 primitive 替换是独立 commit，可单独 revert；业务壳全部保留意味着 revert 成本是"改回内部实现"，不是"重写丢失业务逻辑"
