# Visual QA Findings

## 环境

- 桌面 Chrome，对话页 `/chat`，light mode
- 时间：2026-04-20
- 首批样本：工具"文件搜索 / 内容搜索" 多轮调用

## 进度

| 页面 | Light | Dark | iOS |
|------|-------|------|-----|
| `/chat` 工具调用三态 | ⚠️ 部分 | — | — |
| Dashboard / Settings / Admin / Guide / Canvas / Replay | — | — | — |

---

## Findings (按批次)

### 批次 1：`/chat` 桌面 light（图 1/2/3）

---

#### F-001 [spec-drift / 后端缺陷] 前端永远看不到 running 态

- **页面**：`/chat`
- **主题 / 平台**：light / 桌面
- **截图**：图 1（代理思考中... 占位）、图 2（直接 success 折叠）
- **期望**：工具执行过程中 UI 应展示 running 态（chip spinner + block disabled + duration `--`）
- **实际**：用户反馈"根本没有调用过程，都是调用完才会显示到前端"——工具调用从看不见直接跳到 success 折叠态
- **调查结论**（2026-04-20）：
  - 前端渲染路径**完整**——`useWebSocket.ts:227` 会把 `tool_call.status === 'start'` 映射成 `'running'`，`chat.ts:281` 处理 running 分支，`ToolInvocationChip.tsx:22` / `ToolExecutionBlock.tsx:42` 都有 running UI 分支
  - 意味着**后端 WebSocket 没推 `tool_call { status: 'start' }` 事件**，只推了 end 事件
- **判定**：不在本 change（视觉 polish）的 scope 内，属于后端协议缺陷
- **处置**：
  - 本 change 内不修
  - 另起 change 或 issue 追踪"后端工具调用事件流推 start 消息"
  - **spec 调整**：当前 `openspec/specs/chat-ui-polish/spec.md` 里如果承诺 chip 的 running 态可观察，需要加条件说明"当后端推 start 事件时"（否则合约和实际不符）

---

#### F-002 [minor] chip 配色对比度偏低

- **页面**：`/chat`
- **截图**：图 2
- **期望**：chip 在 `#F8FAFF` 渐变背景上可读，icon 和文字有足够层次
- **实际**：chip 背景 `var(--accent-subtle)` = `rgba(59, 130, 246, 0.08)` 过淡，与页面背景融合，chip 边界几乎看不见
- **建议**：
  - (a) chip 增加 `border: 1px solid var(--accent-200)` 强化边界，或
  - (b) 背景从 `accent-subtle` 升一档到 `accent-100`（约 `#DBEAFE`）
- **权衡**：D2 规定 chip "低干扰"，升饱和会变扎眼；加边框是更温和的折中

---

#### F-003 [minor] block 标题"工具名 + 输出"文案怪

- **页面**：`/chat`
- **截图**：图 2（折叠态）、图 3（展开态）
- **实际**：block 标题渲染成 "文件搜索 输出"，读起来像缺了"的"字；折叠态下还没展开就写"输出"也不准
- **建议**：
  - (a) 折叠态只显示工具名："文件搜索"
  - (b) 展开态再显示"文件搜索 · 调用详情" 或 "文件搜索"（详情由内部 section 自带标题）
- **根因**：i18n `tools.output` key 被拼接到标题，原意大概是"这是输出区"，但 block 是容器、不是 output section 本身

---

#### F-004 [minor] chip 与 block 信息冗余

- **页面**：`/chat`
- **截图**：图 2
- **观察**：一次工具调用占两行视觉——chip"已调用工具: 文件搜索" + block"文件搜索 输出"，工具名出现两次
- **讨论**：D2 原意 chip 是"事件流标题"、block 是"详情容器"，语义上分工合理，但当前文案让用户觉得重复
- **建议**：chip 简化为"📄 文件搜索"（去掉"已调用工具:" 前缀），和 block 标题形成"事件 → 详情"梯度，而不是"标签 → 标签"

---

#### F-005 [minor] 展开态输入/输出 标题色不对等（已修正）

- **页面**：`/chat`
- **截图**：图 3（短输出）+ 图 4（长输出，展开的"网页获取"工具）
- **原判断**：输入/输出 section 结构不对称
- **修正观察**（读图 4 后）：两栏结构其实对称——都是灰底外 section + 白底内容框。图 3 显得不对称是因为输出只有一句话"未找到匹配文件"
- **剩余问题**：
  - "输入"标题是中性灰色（`text-secondary`），"输出"标题是 success 绿色
  - 对比不对等：一个是 label 语义，一个是状态语义
  - 用户视角：调用前 = 输入、调用后成功 = 输出——但"输入"明明也是调用的一部分，为什么不用状态色？
- **建议**（三选一）：
  - (a) 两者都用中性色（label 语义）
  - (b) 两者都用 accent 蓝（统一品牌色）
  - (c) 保留绿色"输出"但把"输入"改为 accent-600 蓝（形成"蓝→绿"的时序感）
- **权衡**：(c) 语义最强但视觉上会加深层次，(a) 最保守

---

#### F-006 [minor] 工具输出 markdown 未渲染

- **页面**：`/chat`
- **截图**：图 4（"网页获取"输出包含 `## Navigation Menu`、`[Skip to content](#...)` 等 markdown 标记）
- **实际**：输出白框里 `## Navigation Menu` 显示为字面量 `## Navigation Menu`，链接 `[Sign in](...)` 不可点击
- **期望（可选）**：如果输出是 markdown 文本（文档抓取、网页内容），渲染 markdown 更可读
- **权衡**：
  - **渲染 markdown 的风险**：当输出是 code/JSON/log 时，markdown 引擎可能错误解析反引号、`#`、`>` 等符号
  - **保持 raw 的优势**：格式保真，code 输出安全
  - **折中**：按工具类型/内容特征启发式判断（比如 `web_fetch` 类工具默认启用 markdown；`bash_exec` / `file_search` 默认 raw）
- **判定**：**不在本 change 修**，留给下一 sprint 讨论"工具结果渲染策略"

---

### 批次 2 待补

- **F-006（占位）** `/chat` dark mode 三态 — 等用户提供截图
- **F-007（占位）** running 态复测（F-001 的跟进）— 等用户用慢工具重拍
- **F-008（占位）** Canvas MarkdownRenderer / Guide 页面内联代码蓝 pill 验证（B3）
- **F-009（占位）** 11 页全站主题色巡查（找黄色残留）

---

## 严重度汇总

- blocker: 0
- major: 0
- minor: 5
  - **已修**：F-002 / F-003 / F-004 / F-005（4 条，2026-04-20）
  - 留延后：F-006（工具输出 markdown 渲染策略 — 下一 sprint 决策题）
- spec-drift / 后端缺陷: 1（F-001 — 后端 WebSocket 未推 `tool_call.start`，本 change 不修）

---

## 修补记录（2026-04-20）

### Patch #1：F-002 chip 加边框
- 文件：`frontend/src/components/chat/ToolInvocationChip.tsx`
- 改动：`className` 加 `border` + `border-[var(--accent-200)] dark:border-[var(--accent-700)]/40`，error 态走 `border-[var(--danger)]/30`
- 效果：chip 在浅蓝渐变背景上有清晰边界

### Patch #2：F-003 block 标题去"输出"
- 文件：`frontend/src/components/chat/ToolExecutionBlock.tsx:86`
- 改动：`{displayName} {t('tools.output')}` → `{displayName}`
- 效果：折叠态只显示工具名，不再读着别扭

### Patch #3：F-004 chip 文案纯工具名
- 文件：`frontend/src/components/chat/ToolInvocationChip.tsx`
- 改动：`{t('tools.invoked')}: {displayName}` → `{displayName}`（aria-label 仍保留完整 `Called tool: {name}` 供屏幕阅读器）
- 同步改：`__tests__/ToolInvocationChip.test.tsx` 文本断言放宽为 `toContain('Shell')`
- 效果：chip 与 block 不再字面重复

### Patch #4：F-005 输入标题改 accent 蓝
- 文件：`frontend/src/components/chat/ToolExecutionBlock.tsx:144`
- 改动：输入标题从 `text-[var(--text-secondary)]` → `text-[var(--accent-600)] dark:text-[var(--accent-300)]`，输出保留 success 绿
- 效果：形成"蓝输入 → 绿输出"的时序色彩

### 验证
- `npx tsc --noEmit` exit=0
- `npx vitest run` 94/94 通过

---

## 下一步

1. **用户侧**：
   - 拍 **dark mode** 同样 2 态（success 折叠 + success 展开，running 跳过因为后端没推）
   - 如果本 change 要修 F-002~F-005，先一起过一遍
2. **Claude 侧**：
   - F-001 关闭（转后端 issue）
   - 等用户确认 F-002~F-005 哪些要在本 change 内修
