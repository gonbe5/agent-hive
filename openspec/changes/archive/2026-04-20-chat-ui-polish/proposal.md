## Why

本次是**全站主题色** + **对话页面工具卡**的联合改造，用户明确反馈："黄色不好看" + "对话页面不好看"，并提供两张参考图：
- `reference-chat.png` — 对话页面工具调用的**两段式**呈现（chip + 可折叠结果块）。
- `reference-theme.png` — **全站级** 冷色 light-blue 主题（蓝色渐变 logo、白卡 active、彩色分类 badge、几乎纯白的卡片、极轻阴影）。

### 现状认知（基于代码实测，2026-04-20）

1. **对话页面工具卡糊** — `MessageBubble.tsx:759-861` 的 `ToolCallCard` 把工具名、参数摘要、展开控件、参数+结果糊在一张厚重卡里；icon 色 `#3b82f6` 硬编码，不走 token。

2. **amber 残留规模**（`rg 'amber-\d+|#F59E0B|#D97706|#B45309|#FEF3C7' frontend/src` 实测）：
   - **181 处硬编码** × **44 个文件**（Tailwind `amber-*` 类 + 直接 hex）
   - CSS 变量仅占 `index.css` `:root`/`.dark` 里 ~14 处 var 定义；组件里 `var(--accent-*)` 引用数不足以覆盖全部 amber 表达。
   - **承认先前 proposal "242 处引用全部通过 CSS 变量间接引用" 是错误认知**：真实情况是大量 Tailwind class + 局部 hex 硬编码，token swap **不足以**批量生效。
   - 分布：`MessageBubble.tsx`(7) / `ChatInput.tsx`(4) / `shared.tsx`(4) / `MessageList.tsx`(6) / `Sidebar.tsx`(8) / `admin/*`(20+) / `settings/*`(20+) / `pages/*`(30+) / `replay/*`(45+) / `hitl/*`(4) / 其他...

3. **ChatInput 场景遗漏**（先前 proposal 未提）：`ChatInput.tsx` L252 drag overlay / L338 active model / L368 deep thinking / L397 send button **全部是 amber Tailwind 类**，必须纳入本 change。

4. **硬编码 hex 散点**（`MessageBubble.tsx` 实测）：L755 `#93c5fd` / L820 `#ef4444` / L827 `#3b82f6` / L830 / L833 `#10b981` / L899 / L732 tool accent palette 全部绕过 token。

5. **参考图细节**: 内联代码 token 高亮（与品牌色同色系）、emoji 加大号章节标题、呼吸感留白。

### 抓手

一次性做三件事：(A) 全站 token 换色 + (B) amber 硬编码彻底扫除 + (C) ToolCallCard 拆分。拆开做会导致视觉漂移 + 同组件重复修改。

## What Changes

### A. 主题色板 amber → light-blue（**全站级** —— 正式确认范围）

- **BREAKING（视觉）**：品牌色从 amber/gold 切换到 light-blue 渐变。
- **替换** `index.css` 里的 CSS 变量（Phase 1a）：
  - `--accent: #D97706` → `#3B82F6`
  - `--accent-hover: #B45309` → `#2563EB`
  - `--accent-light: #FEF3C7` → `#DBEAFE`
  - `--accent-subtle: rgba(217,119,6,0.08)` → `rgba(59,130,246,0.08)`
  - `--accent-border: rgba(217,119,6,0.2)` → `rgba(59,130,246,0.2)`
  - `--gradient-start/mid/end: #F59E0B/#D97706/#B45309` → `#60A5FA/#3B82F6/#2563EB`
  - 新增 `--accent-50/100/300/500/600/700` 显式 stops
  - `--bg-primary: #F2F2F7` → `#F4F7FB`（light only）
  - `--card-tool-*`（工具卡浅色 bg）切到 blue 家族
- **页面 bg** 增加极淡渐变（仅 light mode）：`linear-gradient(180deg, #F8FAFF 0%, #F2F5FC 100%)`。
  - ⚠️ **iOS Safari 风险**：原方案 `background-attachment: fixed` 在 iOS Safari 16+ 有已知渲染抖动 / viewport 100vh 错位问题。改为 `background-attachment: scroll`（默认）或用 `position: fixed` pseudo-element 铺底。参见 design.md R8。
- **Dark mode**：保持"更亮一档"原则（light `#3B82F6` / dark `#60A5FA`），对比度已达 AA。
- **Logo/品牌标识**：Logo 渐变从 `#F59E0B→#D97706` 换为 `#60A5FA→#3B82F6`。保留六边形（DESIGN.md 要求），仅改色。
- **amber 硬编码彻底清扫**（Phase 1a 强制验收）：
  - 对 181 处 × 44 文件的 amber 引用，按目录分组（chat / layouts / settings / admin / pages / replay / hitl / common / canvas / components/session），每组在 tasks.md 有独立验收项。
  - 扫描命令：`rg -n 'amber-\d+|#F59E0B|#D97706|#B45309|#FEF3C7|rgba\(217,\s*119,\s*6' frontend/src`，输出为 0（或仅保留 `/* warning semantic */` 明确注释的位置）才算闭环。
- **tailwind.config.js**：
  - 删除 `theme.extend.colors.brand-amber`（未来如需保留做"历史色"，改名 `legacy-amber` 并加 `@deprecated` 注释，但默认直接删）。
  - `chat.user: '#EBF3FE'` 保留（已是冷蓝），显式在 comment 标注"已对齐 light-blue 品牌"。

### B. 对话页面工具调用重构（与 A 同 PR）

- **拆分** `ToolCallCard` 为两个独立组件：
  - `ToolInvocationChip`（新）—— 仅显示"已调用工具：{name}"，pill 样式，蓝色图标，不可展开。
  - `ToolExecutionBlock`（新）—— 独立块显示"{name} 执行结果" + "点击展开 / 点击收起"右侧文字按钮；展开时显示参数 + 结果。
- **旧 `ToolCallCard` 改为 shim**，内部委托新两件，保留一个发布周期。
- **chip icon 色**：从 `#3b82f6` 硬编码改为 `var(--accent-600)`。
- **error icon 色**：统一改为 spec 要求的 `#DC2626`（当前代码是 `#ef4444` ≠ spec，tasks.md 有明确验收项修正）。
- **内联代码 token**（与 A 冷色系对齐）：`code:not(pre code)` → `bg-[var(--accent-subtle)] text-[var(--accent-700)]`（light）/ `text-[var(--accent-300)]`（dark），角半径 4px，内边距 2px 6px，JetBrains Mono。**本次用蓝系替代参考图的橙色**（因全站换色决定）。
- **章节 heading**：Markdown `##` → `text-xl` / 600 / mt-8 / mb-4。
- **并行工具组 badge** 保留（`MessageBubble.tsx:361-367`），样式降级为 `text-xs` 品牌色 pill（自动变蓝）。
- **"点击展开/收起" 文字按钮** 替代 ChevronRight，保留 `aria-expanded` + `aria-controls`。
- **收起态不显示 argsSummary**。

## Capabilities

### New Capabilities
- `chat-ui-polish`: 规定**全站主题色**、对话页面工具调用的组件拆分契约、内联代码/章节标题的视觉规范、amber 硬编码残留的验收门槛。

### Modified Capabilities
<!-- openspec/specs/ 目前为空，没有已有 capability 要 delta。 -->

## Impact

### 代码

#### Phase 1a — Token + 全站 amber 清扫（强制先做）

| 目录/文件 | amber 处数 | 处理策略 |
|---|---:|---|
| `frontend/src/index.css` | 10 | `:root` + `.dark` 变量值替换；新增 `--accent-*` stops；body 页面渐变（**非 fixed**）|
| `frontend/src/App.css` | 检 `--accent-bg` 遗漏 | 如有则改 var 引用 |
| `frontend/tailwind.config.js` | — | 删 `brand-amber`；`chat.user` 保留并加注释 |
| `frontend/src/components/chat/MessageBubble.tsx` | 7 + 6 hex | Tailwind class 改蓝；L755/L820/L827/L830/L833/L899/L732 全部改 token |
| `frontend/src/components/chat/ChatInput.tsx` | 4 | drag overlay / active model / deep thinking / send button 全部改 token（**本 change 新增范围**）|
| `frontend/src/components/chat/shared.tsx` | 4 | thinking 指示器、action btn、avatar accent 改 token |
| `frontend/src/components/chat/MessageList.tsx` | 6 | 改 token |
| `frontend/src/components/chat/TaskProgressPanel.tsx` | 2 | 改 token |
| `frontend/src/components/chat/ArtifactCard.tsx` | 1 | 改 token |
| `frontend/src/layouts/Sidebar.tsx` | 8 | 改 token |
| `frontend/src/layouts/AdminSidebar.tsx` | 1 | 改 token |
| `frontend/src/pages/Dashboard.tsx` | 6 | 改 token |
| `frontend/src/pages/Skills.tsx` | 6 | 改 token |
| `frontend/src/pages/Guide.tsx` | 8 | 改 token |
| `frontend/src/pages/Sessions.tsx` | 2 | 改 token |
| `frontend/src/pages/Agents.tsx` | 2 | 改 token |
| `frontend/src/pages/ChatLanding.tsx` | 2 | 改 token |
| `frontend/src/pages/AdminSettings.tsx` | 1 | 改 token |
| `frontend/src/pages/SessionReplay.tsx` | 1 | 改 token |
| `frontend/src/pages/admin/*` | 20+ | 逐文件改 token |
| `frontend/src/components/settings/*` | 20+ | 逐文件改 token |
| `frontend/src/components/replay/*` | 45+（scene/ 占大头）| 场景动画如含品牌语义改 token；纯装饰性保留并在注释标 `/* replay scene decor */` |
| `frontend/src/components/hitl/ApprovalCard.tsx` | 4 | 审批按钮品牌色改 token |
| `frontend/src/components/common/*` | 5 | 逐文件改 |
| `frontend/src/components/canvas/renderers/HtmlRenderer.tsx` | 1 | 检查是否语义色 |
| `frontend/src/components/session/TagEditor.tsx` | 3 | 改 token |
| Logo SVG | — | 内联 SVG / assets 里 amber 渐变 → blue 渐变 |
| `DESIGN.md` | — | Aesthetic / Color / Brand Identity / Decisions Log 四处更新 |

#### Phase 1b — 对话页面重构（与 1a 同 PR 后半）

- **新增**：`frontend/src/components/chat/ToolInvocationChip.tsx`、`ToolExecutionBlock.tsx`
- **改**：`MessageBubble.tsx:371` 调用位；旧 `ToolCallCard` 改 shim
- **改**：`index.css` 新增内联代码 + heading override
- **改**：`i18n/locales/{zh,en}.json` 新增 3 key

### API
无。纯前端/设计系统变更。

### 依赖
无新增 npm 包。

### i18n
新增 key：`tools.clickToExpand` / `tools.clickToCollapse` / `tools.invoked`。

### 测试
- 更新快照测试接受新色值。
- 新增 `ToolInvocationChip` / `ToolExecutionBlock` 单测。
- 视觉 QA：light + dark 两套全量截图，覆盖 dashboard、chat、settings、admin、HITL、sessions、agents、skills、guide、replay 至少 10 个页面。

### 向后兼容
- `ToolCallCard` 保留 shim 一个发布周期。
- Token 命名不变（仍叫 `--accent-*`），只改值；**但因存在大量 Tailwind class + 硬编码 hex 非 var 引用**，token swap **不足以**自动生效，必须配合 Phase 1a 的 44 文件清扫。

### 风险（R1-R8，详见 design.md）
- **R1** 长对话视觉高度变高（chip + block 两段）
- **R2** i18n key 缺失
- **R3** 历史对话渲染兼容（ToolCallCard shim）
- **R4** 工具 chip 蓝底叠加过饱和
- **R5** Prose 样式污染其他页面
- **R6** 色觉无障碍（新 #2563EB 对白底 8.59:1，通过 WCAG AAA）
- **R7** `legacy-amber` / 已删 `brand-amber` 被他处引用
- **R8** **iOS Safari `background-attachment: fixed` 兼容性**（新增风险）
- **R9** `warning` 语义色与新品牌色视觉相近（蓝 vs 橙）但语义不同——Decisions Log 明确写出"语义色不跟品牌色改"
- **R10** 残留 amber 色斑（已通过全目录清扫方案降为低风险，但需 tasks 10.7 强制 grep=0 验收）

## Non-Goals（**与 blast radius 一致，已重写**）

- ❌ **不重做** `CodeBlockHeader` 的深色 theme（fenced code block 独立视觉，不在本 change 范围）。
- ❌ **不重做** `MermaidBlock`、Canvas / Artifact 渲染核心逻辑（仅可能改它们品牌色引用）。
- ❌ **不重做** `MessageBubble` / `ChatInput` / `shared` 的**业务逻辑**（仅改它们的**色值引用**，从 amber 到 token）。
- ❌ **不改** HITL 审批流、工具审批流、token 数据流、ChatStore 结构。
- ❌ **不新增** 动画库，不升级 Tailwind 版本，不改主题切换逻辑，不改移动端断点。

> ✅ **显式承认**：因定调"全站 rebrand"，用户消息气泡、thinking 指示器、code toolbar、ActionBtn、Logo、Sidebar active 态、admin 图表配色等**组件的品牌色引用都会动**；但**组件的交互行为和 DOM 结构不变**。
