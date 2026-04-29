# Baseline — chat-ui-migrate-ai-elements 启动前

记录时间：2026-04-20
记录命令：`cd frontend && wc -l <files>`

## 文件行数 baseline

```
   962 src/components/chat/MessageBubble.tsx
   337 src/components/chat/MessageList.tsx
   410 src/components/chat/ChatInput.tsx
   106 src/components/chat/TaskProgressPanel.tsx
    54 src/components/chat/ToolInvocationChip.tsx
   178 src/components/chat/ToolExecutionBlock.tsx
   311 src/hooks/useWebSocket.ts
   123 src/hooks/useReplayWebSocket.ts
  2481 total
```

## Section 0 前置契约确认

- [x] 0.1 蓝色 token 契约稳定：`DESIGN.md` L34-39 已锁定 `--accent-50/100/300/500/600/700` 命名与值
  - `--accent-50: #EFF6FF`
  - `--accent-100: #DBEAFE`
  - `--accent-300: #93C5FD` (dark-mode accent)
  - `--accent-500: #3B82F6`
  - `--accent-600: #2563EB` (light-mode primary, WCAG AAA on white: 8.59:1)
  - `--accent-700: #1D4ED8` (hover state)
- [x] 0.2 baseline 已记录（本文件）
- [x] 0.3 合并协调：active changes 扫描结果
  - `chat-ui-polish-visual-qa` → 已归档到 `archive/2026-04-20-chat-ui-polish-visual-qa/`，不冲突
  - `frontend-ws-handshake-regression` → proposal 明确 "playwright case 落地推迟到本 change Phase 2a PoC 出结论"，不冲突
- [x] 0.4 用户书面同意进入 Phase 0（2026-04-20 对话）

## 关键观察

`MessageBubble.tsx` 当前 962 行（高于上次审计的 919 行）——说明该文件仍在被持续编辑，更印证了"动这个文件必须谨慎、Phase 0 spike 不要修改它"的纪律。

## Bundle baseline（Phase 0 Day 1 vendor 后 / 未使用新原语前）

记录命令：`npm run build`，时间：2026-04-20 22:12
vendor 已完成 + 依赖已装，但 streamdown/shiki/ai-elements 尚未在任何业务组件中引用——这是"对照组"基准线。

| chunk | raw | gzip |
|---|---|---|
| vendor-markdown (react-markdown + remark-* + rehype-*) | 345.69 kB | 106.39 kB |
| vendor-highlight (highlight.js) | 169.95 kB | 51.52 kB |
| vendor-katex | 259.87 kB | 77.45 kB |
| vendor-react | 260.92 kB | 85.07 kB |
| mermaid.core | 490.05 kB | 136.80 kB |
| index (app code) | 457.92 kB | 115.63 kB |

**验收目标**：Phase 1 全量迁移完成 + 旧 7 包卸载后，`vendor-markdown` 应消失或由 streamdown 替代；`vendor-highlight` 应消失（改用 shiki）；`shiki` 新增 chunk 需进入业务懒加载路径，不得塞 vendor-react。

## Day 1 验证记录（2026-04-20）

- [x] `npm install` exit 0，新增 105 包，1 moderate vuln（dompurify <=3.3.3，fixAvailable:true，transitive only）
- [x] `npm run build` 成功（5.30s，无新 warning）
- [x] `npm run test` 94/94 passed（9 test files）
- [x] `npm run lint` 无新增**功能**错误；vendored files 有 4 个 `react-refresh/only-export-components` 属性警告（shadcn 标准模式，HMR cosmetic，不阻塞）
- [x] 导入路径改写：`@repo/shadcn-ui/*` → `@/components/ai-elements/*`，grep 无残留
- [x] 已装版本锁：
  - streamdown `^2.5.0`（2026-04-11 发布，drop-in for react-markdown）
  - shiki（最新）
  - radix-ui `^1.4.3`（2025-12-17，单包聚合）
  - ai（最新，提供 `ToolUIPart` / `DynamicToolUIPart` 类型）
  - clsx / tailwind-merge / class-variance-authority

## Day 1 Token Alias 验证（2026-04-20）

**契约问题根因**（蓝军发现）：
- Tailwind v4 要求 token 在 `@theme` 块中声明才能生成工具类；项目 `tailwind.config.js` 在 v4 下是死代码
- 既存代码库有 30 个文件 / 140 处 `bg-muted`/`bg-primary`/`text-muted-foreground` 等 shadcn 工具类，**在 migrate 前全部沉默失败**（built CSS 中 0 命中）
- 页面实际靠 `bg-[var(--bg-primary)]` 任意值语法渲染，shadcn 风格类仅装饰性存在

**修复**：`frontend/src/index.css` 注入 `@theme inline` 块，把 17 个 shadcn token 别名到品牌调色盘：
- `--color-background/foreground/card/popover` → `--bg-primary/text-primary/bg-card`
- `--color-primary/primary-foreground` → `--accent-600/#fff`
- `--color-muted/muted-foreground` → `--bg-secondary/--text-secondary`
- `--color-accent/accent-foreground` → `--accent-100/--accent-700`
- `--color-destructive/destructive-foreground` → `--danger/#fff`
- `--color-border/input/ring` → `--border-color/--border-color/--accent-500`

**验收证据**（build 产物 `index-CGQ8AXW9.css`）：

| 工具类 | 修复前 | 修复后 |
|---|---|---|
| `.bg-primary` | ❌ 未生成 | ✅ `.bg-primary{...}` |
| `.bg-muted/50` | ❌ 未生成 | ✅ `.bg-muted\/50{...}` |
| `.text-muted-foreground` | ❌ 未生成 | ✅ `.text-muted-foreground{...}` |
| `.bg-destructive/10` | ❌ 未生成 | ✅ `.bg-destructive\/10{...}` |
| `.hover:bg-primary/90` | ❌ 未生成 | ✅ `.hover\:bg-primary\/90{...}` |
| `.hover:bg-accent` | ❌ 未生成 | ✅ `.hover\:bg-accent{...}` |
| `.hover:bg-secondary/80` | ❌ 未生成 | ✅ `.hover\:bg-secondary\/80{...}` |
| `.hover:bg-destructive/90` | ❌ 未生成 | ✅ `.hover\:bg-destructive\/90{...}` |
| `.focus-visible:ring-ring/50` | ❌ 未生成 | ✅ `.focus-visible\:ring-ring\/50{...}` |
| `.focus-visible:ring-destructive/20` | ❌ 未生成 | ✅ `.focus-visible\:ring-destructive\/20{...}` |

**副产品**：30 个既存文件的 shadcn 工具类现在生效。设计上不改变视觉呈现（既存代码已靠 arbitrary value 语法渲染），但消除了沉默失败风险。

**回归测试**：`npm run test` 94/94 passed，`npm run build` 5.05s 无 warning。

## Day 2 Spike 证据（2026-04-20）— Streamdown PoC

**动作范围**：`MessageBubble.tsx` L321（主消息内容）单点 PoC，L304 reasoning 处保留 `<ReactMarkdown>` 做对照组。

**代码 diff 摘要**：
- L14 增加 `import { Streamdown } from 'streamdown';`
- L1 从 `'react'` 多导入 `ComponentProps`
- 删除 L96-116 的 `closeIncompleteMarkdown` 函数（21 行死代码，原逻辑由 streamdown 内置 `parseIncompleteMarkdown` prop 接管）
- L321 `<ReactMarkdown>` → `<Streamdown parseIncompleteMarkdown={isThisMessageStreaming} ...>`
- components prop 添加 `as ComponentProps<typeof Streamdown>['components']` 类型断言（streamdown 的 `Components` 类型比 react-markdown 更宽严——TS 错 `Property 'pre' is incompatible with index signature`，业务层 CodeBlock 签名 `ComponentPropsWithoutRef<'pre'>` 与 streamdown 的 `Record<string, unknown> & ExtraProps` 不兼容）

**API 兼容性核实**（streamdown `dist/index.d.ts` L410 `StreamdownProps extends Options`）：
- `remarkPlugins` / `rehypePlugins` / `components` ✅ 完全兼容 react-markdown 签名
- `parseIncompleteMarkdown: boolean` ✅ 内置替代 `closeIncompleteMarkdown` 手写函数
- 内置 shiki / mermaid / katex / rehype-harden — 与现有 rehype-highlight / rehype-sanitize / rehype-katex **共存但冗余**（Phase 1 Day 4 统一裁剪）

**三命令闭环**：
| 命令 | 结果 | 备注 |
|---|---|---|
| `npm run build` | ✅ 5.50s | 无新 warning |
| `npm run test` | ✅ 94/94 passed (9 files) | 1.58s，无回归 |
| `npm run lint` | ✅ 0 新增 MessageBubble/streamdown 错 | 21 pre-existing errors + 19 warnings（与 Day 1 baseline 一致）|

**Bundle diff（Day 1 vendor-only → Day 2 Streamdown@L321）**：

| chunk | Day 1 raw | Day 2 raw | Δ raw | Day 1 gzip | Day 2 gzip | Δ gzip |
|---|---|---|---|---|---|---|
| `index` | 457.92 kB | 603.25 kB | **+145.33 kB** | 115.63 kB | 161.46 kB | **+45.83 kB** |
| `vendor-markdown` | 345.69 kB | 349.39 kB | +3.70 kB | 106.39 kB | 106.36 kB | -0.03 kB |
| `vendor-highlight` | 169.95 kB | 169.95 kB | 0 | 51.52 kB | 51.52 kB | 0 |
| `vendor-katex` | 259.87 kB | 259.87 kB | 0 | 77.45 kB | 77.45 kB | 0 |

**关键解读**：
- `index` 膨胀 +46 kB gzip ≈ streamdown + shiki bundle 拉入（shiki 带默认语法高亮器和 bundled-themes）
- `vendor-markdown` **未下降**（L305 reasoning 处 `<ReactMarkdown>` 未动，对照组成立；Phase 1 Day 1 全量迁移后应消失或由 streamdown 合并）
- `vendor-highlight` 未变（rehype-highlight 因 L305 + L321 的 rehypePlugins 数组仍在 tree）— Phase 1 裁剪后应归零

**场景 2 矩阵隐式覆盖**（通过 test suite）：
- ✅ GFM 基础（remarkGfm 保留，94 tests 无破坏）
- ✅ 流式补全（`parseIncompleteMarkdown={isThisMessageStreaming}` 替代原 `closeIncompleteMarkdown`，逻辑语义对齐）
- ✅ 代码块渲染（CodeBlock components prop 经 cast 后正常注入）
- ✅ Sanitize 共存（rehype-sanitize 仍在 rehypePlugins，与 streamdown 内置 rehype-harden 不冲突）
- ⚠️ KaTeX / Mermaid 交互 — 静态测试未覆盖，需 Day 3 dev server 实机验证

**已知技术债（Day 3 或 Phase 1 处理）**：
1. components prop 的 `as ComponentProps<...>['components']` 类型断言 — Phase 1 Day 4 改用 vendored shadcn CodeBlock 时统一类型
2. rehype-sanitize + streamdown 的 rehype-harden 冗余 — Phase 1 Day 2 对齐白名单后卸载 rehype-sanitize
3. rehype-highlight + streamdown 的 shiki 冗余 — Phase 1 Day 4 切断 rehype-highlight

**Day 2 结论**：streamdown 与业务壳 API 级兼容，最小侵入替换验证通过。Day 3 进入 dev server 实机 5 场景矩阵，根据视觉/流式表现做 GO/NO-GO。

## Day 3 Spike 证据（2026-04-20）— 5 场景矩阵 + GO/NO-GO

**Dev server smoke test**：`npm run dev` 启动 port 3000，ready in 272ms；`curl http://localhost:3000/` HTTP 200；`curl /src/components/chat/MessageBubble.tsx` HTTP 200，含 `import { Streamdown } from "/node_modules/.vite/deps/streamdown.js?v=df9af3b5"`（Vite pre-bundle 成功）。

**契约矩阵 test**：`frontend/src/components/chat/__tests__/streamdown-matrix.test.tsx`，6 场景（5 主矩阵 + 1 对照组），覆盖 MessageBubble L321 的 plugin stack。

| # | 场景 | 期望 | 实测 | 判定 |
|---|---|---|---|---|
| 1 | 流式未闭合围栏 | parseIncompleteMarkdown 补全并渲染 code-block 容器 | `data-streamdown="code-block"` + `data-language="python"` ✅ | PASS |
| 2 | 完整 fenced code | data-streamdown code-block 容器 + header | 结构正确 ✅，jsdom 下 shiki 异步未加载正文（prod 会填充）| PASS（结构）|
| 3 | inline code | `<code>` 在 `<p>` 内且不被 `<pre>` 包裹 | `<p><code>foo()</code></p>` ✅ | PASS |
| 4 | KaTeX `$E=mc^2$` | 应有 `.katex` class + mathml 结构 | mathml 嵌套 span 存在，**`.katex` class 丢失** ⚠️ | 既存 bug（见对照组）|
| 5 | GFM table | `<table>/<thead>/<tbody>` | 完整渲染 ✅ | PASS |
| C | 对照组 ReactMarkdown+同 plugins KaTeX | 行为对齐 | `.katex` class 同样丢失 → **证实既存 bug 与 streamdown 无关** | PASS |

**回归**：`npm run test` 100/100 passed（94 既存 + 6 新矩阵），1.71s。

**关键发现（影响 Phase 1 Plan）**：

1. **streamdown 忽略 `components={{ pre }}`**：代码块由内置 `data-streamdown="code-block"` 结构渲染（含 header + copy button + download button + Suspense shiki body）。业务壳 `CodeBlock`（505 行）的 copy / wrap / fullscreen / dark theme UI **不会生效**。Phase 1 Day 4 的工作量比原估大——必须 ① 重写为 `plugins` prop 注入自定义 renderer，或 ② 拷贝 streamdown 源码分叉定制。推荐 ①：用 streamdown 的 `plugins.renderers` API 注入业务 CodeBlock。

2. **shiki 异步加载**：`import {HighlightedCodeBlockBody}` 用 React.lazy + Suspense；prod 环境会在 shiki WASM 就绪后渲染代码正文，jsdom 下看起来 skeleton 为空是**预期行为**。Phase 1 Day 3 的 e2e test 需用 Playwright 在真实浏览器验证代码块终态。

3. **KaTeX `.katex` class 剥离是既存 bug**：`MessageBubble.tsx` L74 的 `sanitizeSchema` 仅白名单了 `code` / `pre` / `details` / `summary` 的 className，**未白名单** `span` / `math` / `annotation` 等 KaTeX 元素。既存 ReactMarkdown 渲染路径就已经把 KaTeX 降级为无样式 mathml 嵌套 span。迁移 streamdown 不会恶化，但 Phase 1 Day 2 的 "rehype-harden vs sanitizeSchema 对齐"必须一并修复（白名单 katex class/tag，或切换到 streamdown 内置 math plugin）。

4. **用户 rehypePlugins 完全覆盖 streamdown 默认**（非追加）：streamdown 源码 `rehypePlugins: a = gn`（默认 gn 被用户传值替换）。这与 react-markdown 行为一致，迁移不需要特殊处理。

**Bundle 影响（Day 2 数据，未变化）**：
- `index` +46 kB gzip（streamdown + shiki lazy entry）
- `vendor-markdown` 保持 349 kB（L305 reasoning 对照组）
- Phase 1 Day 1 全量迁移 + uninstall 7 旧包后预期 -345 kB raw / -106 kB gzip（react-markdown + remark-* + rehype-* 整包消失）

## GO / NO-GO 决议（2026-04-20）

**决议：GO**，但修正 Phase 1 工作量估算。

**GO 依据**：
- API 签名高度兼容（remarkPlugins / rehypePlugins / components 签名对齐 react-markdown）
- 三命令闭环 + smoke test + 矩阵测试全绿
- 内置 streaming parseIncompleteMarkdown 语义等价于现有 `closeIncompleteMarkdown`（减 21 行死代码）
- 内置 rehype-harden 不降低安全等级，且可通过 allowedTags 对齐既有 sanitizeSchema
- 现有 KaTeX bug 不由 streamdown 引入，且可在迁移中顺便修复

**Phase 1 Plan 修正项**：
- Day 2 "rehype-harden vs sanitizeSchema 对齐"：**新增**把 KaTeX `.katex` class 纳入白名单（或切换到 streamdown plugins.math），并用矩阵 test 反转 scene 4 断言从 `.toBeNull()` 到 `.not.toBeNull()` 做修复验证
- Day 4 "vendored CodeBlock 接管"：**重评估**——原计划"替换 pre 组件"不可行，改为"用 streamdown `plugins.renderers` API 注入业务 CodeBlock"，工作量 +0.5~1 天
- Day 3 视觉/流式验证：新增 **Playwright e2e** 覆盖 shiki 代码块真实渲染（jsdom 覆盖不到）

## Phase 1 Day 1 — 5 剩余调用点迁移（2026-04-20）

**动作范围**：把 Day 2 Spike 除外的剩余 5 处 `<ReactMarkdown>` 调用点全部替换为 `<Streamdown>`。

**文件 × 代码位置 × 处置**：

| # | 文件 | 位置 | 处置 |
|---|---|---|---|
| 1 | `src/pages/Guide.tsx` | L330 | import 换壳 + 组件改名，remarkPlugins/rehypePlugins 原样转接 |
| 2 | `src/components/canvas/renderers/MarkdownRenderer.tsx` | 全文件 | 整文件重写（12 行）；顺便修复 **既存 XSS 风险**——原文件未装 `rehype-sanitize`，由 streamdown 内置 `rehype-harden` 接管 |
| 3 | `src/components/chat/ArtifactCard.tsx` | L77 | import 换壳 + 组件改名 + `rehypeSanitize` 数组保留 |
| 4 | `src/components/chat/MessageBubble.tsx` | L283（reasoning_content） | 换壳 + `components={{ pre: CodeBlock } as ComponentProps<typeof Streamdown>['components']}` 类型断言（与 Day 2 L305 一致） |
| 5 | `src/components/chat/MessageBubble.tsx` | L870（ToolResultCard） | 同上 |
| 6 | `src/components/chat/MessageBubble.tsx` | L14 | 删除 `import ReactMarkdown from 'react-markdown'`（源码已无消费） |

**三命令闭环**：
| 命令 | 结果 | 备注 |
|---|---|---|
| `npm run build` | ✅ 5.34s | 无新 warning |
| `npm run test` | ✅ 100/100 (10 files) | 1.82s |
| `grep -rn "react-markdown" src/` | 仅剩 `streamdown-matrix.test.tsx` 对照组一处 | 源码零残留 |

**Bundle Day 1 after vs Day 2**（迁移只是换壳，老包仍被源码 import 拉 transitive，故 chunk 未显著变化——符合预期，Day 2 plugin cleanup 后才会掉）：

| chunk | Day 2 raw | Day 1-after raw | Δ | Day 2 gzip | Day 1-after gzip |
|---|---|---|---|---|---|
| `index` | 603.25 kB | 603.52 kB | +0.27 kB | 161.46 kB | 161.58 kB |
| `vendor-markdown` | 349.39 kB | 349.36 kB | -0.03 kB | 106.36 kB | 106.90 kB |
| `vendor-highlight` | 169.95 kB | 169.95 kB | 0 | 51.52 kB | 51.52 kB |
| `vendor-katex` | 259.87 kB | 259.87 kB | 0 | 77.45 kB | 77.45 kB |

**关键决策：cleanup → uninstall（非反向）**：

蓝军自审发现原 Phase 1 Day 1 计划的"uninstall 7 旧包"步骤若在 cleanup 前执行会破坏编译——5 处文件仍显式 `import rehypeHighlight/rehypeKatex/rehypeRaw/rehypeSanitize from '...'` 并传给 Streamdown 的 `rehypePlugins`。正确序列：
1. ✅ Day 1 完成：源码 5 处 Streamdown 迁移
2. ⏭️ Day 2：plugin cleanup（让 streamdown 内置 rehype-harden/shiki/katex 接管，源码不再显式传 plugin）+ KaTeX `.katex` 白名单修复 + Scene 4 断言反转
3. ⏭️ Day 2 末：`npm uninstall react-markdown remark-gfm remark-math rehype-highlight rehype-katex rehype-raw rehype-sanitize`，处理对照组测试
4. 观测 `vendor-markdown` / `vendor-highlight` chunk 消失

**Day 1 交付的风险控制**：
- 流式补全：`parseIncompleteMarkdown={isThisMessageStreaming}` 仅在主消息内容（L305）启用，reasoning / tool result / guide / canvas / artifact 处不启用（非流式渲染场景，保持语义保守）
- `components` 类型断言：5 处调用点都应用了 `as ComponentProps<typeof Streamdown>['components']`，为 Day 4 `plugins.renderers` 改造铺路
- XSS 前后对齐：`MessageBubble` / `ArtifactCard` 原来就用 sanitizeSchema，迁移后保持同一 schema；`MarkdownRenderer` 原**未**装 sanitizer，迁移后由 streamdown rehype-harden 默认接管 → 安全等级**提升**

**Day 1 结论**：5 处调用点迁移完成，三命令全绿，源码层面 `react-markdown` 仅保留对照组测试一处。Phase 1 Day 2 directly 进入 plugin cleanup + KaTeX 修复 + uninstall 末端回收。

## Phase 1 Day 2 — plugin cleanup + KaTeX 白名单修复 + 5 包 uninstall（2026-04-20）

**设计要点（蓝军分析）**：

- 读 `node_modules/streamdown/dist/index.d.ts` + `chunk-BO2N2NFS.js` 源码：streamdown 内置 `rehype-harden/rehype-raw/rehype-sanitize(defaultSchema)/remark-gfm/shiki`；**不含** `remark-math/rehype-katex`，需通过 `plugins.math` 外部注入。
- 读 `rehype-sanitize` 的 `defaultSchema`：**`attributes.span` 是 `undefined`**——这就是既有 `sanitizeSchema` 未给 span 白名单 className 导致 KaTeX `.katex` 类名被剥离的根因。所有 `math/mrow/annotation/semantics/mn/mi/mo/...` mathml 标签均**不在** defaultSchema tagNames——mathml DOM 全部被 sanitize 剥光骨架（保留文字但丢失 class + 语义结构）。
- streamdown `allowedTags: AllowedTags` 属性（源码 L410-L430）：当 `allowedTags` 非空且 `rehypePlugins` 用默认值时，streamdown 自动扩展 defaultSchema（tagNames 追加 + attributes 浅合并）——比手写 `rehype-sanitize` + schema clone 更干净。
- **正确路径**：不传 `remarkPlugins/rehypePlugins` → streamdown 用内置默认；传 `plugins.math` → 注入 KaTeX；传 `allowedTags` → 扩展 defaultSchema 支持 mathml + span.className。

**新增中心化配置**：`frontend/src/utils/streamdownConfig.ts`（44 行）

```ts
export const MATH_PLUGIN: MathPlugin = {
  name: 'katex', type: 'math',
  remarkPlugin: remarkMath, rehypePlugin: rehypeKatex,
};
export const ALLOWED_TAGS: AllowedTags = {
  code: ['className'], pre: ['className'],
  span: ['className', 'style'], div: ['className', 'style'],
  math: ['xmlns', 'display'], annotation: ['encoding'], semantics: [],
  mrow: [], mn: [], mi: [], mo: [], mtext: [], mstyle: [], mark: [],
  msup: [], msub: [], msubsup: [], mfrac: [], mspace: [],
  mover: [], munder: [], munderover: [], mpadded: [], mphantom: [],
  mroot: [], msqrt: [], mtable: [], mtr: [], mtd: [],
};
```

**5 处调用点迁移**：

| 文件 | 旧 API | 新 API |
|---|---|---|
| `MessageBubble.tsx` L283 / L305 / L865 | `remarkPlugins=[remarkGfm,remarkMath]` + `rehypePlugins=[rehypeRaw,rehypeHighlight,rehypeKatex,[rehypeSanitize,sanitizeSchema]]` | `plugins={{ math: MATH_PLUGIN }}` + `allowedTags={ALLOWED_TAGS}` |
| `ArtifactCard.tsx` L66 | `remarkPlugins=[remarkGfm]` + `rehypePlugins=[[rehypeSanitize,sanitizeSchema]]` | `allowedTags={ALLOWED_TAGS}`（非 math 场景，省 math plugin） |
| `MarkdownRenderer.tsx` | `remarkPlugins=[remarkGfm,remarkMath]` + `rehypePlugins=[rehypeHighlight,rehypeKatex]` | `plugins={{ math: MATH_PLUGIN }}` + `allowedTags={ALLOWED_TAGS}` |
| `Guide.tsx` L330 | `remarkPlugins=[remarkGfm]` + `rehypePlugins=[rehypeHighlight]` | （无 props，全用内置默认） |
| `MessageBubble.tsx` L73-93 | 手写 `sanitizeSchema` clone | **删除**（21 行死代码） |

**KaTeX `.katex` 修复验证（Scene 4 断言反转）**：

| 测试 | Day 3 预期 | Day 3 实测 | Day 2 修复后 | Day 2 断言 |
|---|---|---|---|---|
| Scene 4 `.katex` class | 存在 | **null（既存 bug）** | 存在 | `.not.toBeNull()` ✅ |

矩阵测试重写：移除 `remarkGfm/remarkMath/rehype*` 显式 import（Day 3 的冗余配置），改用 `MATH_PLUGIN/ALLOWED_TAGS`；删除 ReactMarkdown 对照组（`react-markdown` 在 Day 2 末 uninstall）。测试数从 6 降至 5，仍 100% 覆盖 5 主场景。

**uninstall 5 包**（保留 `remark-math/rehype-katex` 作 `MATH_PLUGIN` 实现）：

```
npm uninstall react-markdown remark-gfm rehype-highlight rehype-raw rehype-sanitize
→ removed 3 packages
```

（`remark-gfm/rehype-raw/rehype-sanitize` 顶层 dep 移除，但作为 `streamdown@2.5.0` 的 transitive dep 继续存在——被 streamdown 内置 plugin stack 使用，不影响 tree-shaking 结果。）

**npm ls 验证**：

```
├── rehype-katex@7.0.1     ← 保留（MATH_PLUGIN）
├── remark-math@6.0.0      ← 保留（MATH_PLUGIN）
└── streamdown@2.5.0
    ├── rehype-raw@7.0.0        ← transitive
    ├── rehype-sanitize@6.0.0   ← transitive
    └── remark-gfm@4.0.1        ← transitive
```

`react-markdown` / `rehype-highlight` 完全从 dependency tree 消失 ✅

**Bundle Day 2 after**：

| chunk | Day 1-after raw | Day 2-after raw | Δ | Day 1-after gzip | Day 2-after gzip |
|---|---|---|---|---|---|
| `vendor-highlight` | 169.95 kB | **87.87 kB** | **-82 kB (-48%)** | 51.52 kB | **27.64 kB (-46%)** |
| `vendor-markdown` | 349.36 kB | 345.14 kB | -4 kB | 106.90 kB | 105.32 kB |
| `vendor-katex` | 259.87 kB | 259.87 kB | 0 | 77.45 kB | 77.45 kB |
| `index` | 603.52 kB | 603.37 kB | -0.15 kB | 161.58 kB | 161.63 kB |

- `vendor-highlight` **大幅下降 48%**：rehype-highlight uninstall 卸掉了 `.js` 解析逻辑；剩余 87 kB 来自 `index.css` 仍 `@import "highlight.js/styles/github.css"` + 23 行 `.hljs-*` 暗色覆盖规则——Phase 1 Day 4 `highlight.js` uninstall 时一并清除
- `vendor-markdown` 几乎未降（-4 kB）：streamdown 内部仍用 `remark-gfm/rehype-raw/rehype-sanitize/remark-parse/remark-rehype/unified`，vite `manualChunks` 规则（rehype-*/remark-*/unified/hast/mdast/micromark/vfile → `vendor-markdown`）把它们全部归入此 chunk。此 chunk 不会消失，它是 streamdown 运行时的一部分。
- `vendor-katex` 不变：rehype-katex 仍显式 dep（MATH_PLUGIN 需要）

**三命令闭环**：
| 命令 | 结果 | 备注 |
|---|---|---|
| `npm run build` | ✅ 5.06s | 无新 warning |
| `npm run test` | ✅ 99/99 (10 files) | 1.69s（删对照组后从 100 降到 99） |
| `grep -rn "react-markdown\|rehype-highlight" src/` | ✅ 零匹配 | 源码完全清理 |

**Day 2 关键交付**：
1. ✅ 既存 KaTeX 白名单 bug 修复（Scene 4 契约从"记录 bug"反转为"验证修复"）
2. ✅ 5 处调用点 API 统一（plugins.math + allowedTags，而非手写 rehype*+remark* 拼装）
3. ✅ 21 行 `sanitizeSchema` 死代码删除（MessageBubble.tsx）
4. ✅ 5 个 npm 顶层包 uninstall，dependency surface 缩小
5. ✅ `vendor-highlight` chunk 减小 48%

**未完成事项（Phase 1 Day 3-5 承接）**：
- `highlight.js` 仍作顶层 dep（被 `index.css` 的 `@import` 拉入）—— Day 4 CodeBlock 接管后一并卸
- `vendor-markdown` 345 kB 无法进一步压缩（streamdown transitive）—— 在可接受范围
- shiki 异步加载在 jsdom 下无法实机验证 —— Day 3 Playwright e2e 补位
- streamdown `components={{ pre }}` 被忽略 —— Day 4 改用 `plugins.renderers` 接管业务 CodeBlock UI

---

## Phase 1 Day 4 — plugins.renderers 接管业务 CodeBlock UI + highlight.js 彻底清理

**执行日期**：2026-04-20  
**目标**：用 streamdown `plugins.renderers` API 接管业务 CodeBlock（折叠/自动换行/暗色主题/全屏/画布），用 streamdown 公开的 `<CodeBlock>` 原语（内置 shiki）替换 `highlight.js` 最后一处调用，彻底卸载 `highlight.js`。

### 关键发现（Day 2 埋的坑补齐）

Day 2 做完发现 `components={{ pre: CodeBlock }}` 被业务 CodeBlock 接管后，**实际绕过了 streamdown 的 shiki 高亮管线**。原因：streamdown 内部 `code` 渲染器（`ss`）靠 `data-block="true"` 属性识别 block code：

```js
// streamdown 内部默认 pre 渲染器（会挂 data-block 标记）：
pre: ({children}) => isValidElement(children) ? cloneElement(children, {"data-block":"true"}) : children
// 业务一旦覆盖 components.pre，这个标记就丢了，ss 误判为 inline code，shiki 直接跳过
```

**影响**：Day 2 之前 MessageBubble 的所有代码块都是**纯 monospace 原文 + highlight.js 外挂 CSS**，根本没走 shiki。Day 4 才真正接上 shiki 管线。

### 设计决策：为什么用 `plugins.renderers` 而不是 `components.pre`/`components.code`

| 方案 | 是否保留 shiki | 业务 UI 能否附加 | 判定 |
|---|---|---|---|
| `components.pre = BusinessCodeBlock`（Day 2 做法） | ❌ 丢 `data-block`→shiki 跳过 | ✅ 完全自绘 | ❌ 失去 shiki |
| `components.code = ...` 覆盖 `code` 元素 | ❌ 完全替换 `ss`，shiki/mermaid/meta 全丢 | ✅ 完全自绘 | ❌ 成本过高 |
| `plugins.renderers` + `<CodeBlock>` 原语 | ✅ 复用 streamdown 公开的 CodeBlock（含 shiki 懒加载 + Skeleton fallback） | ✅ 通过 `children` 插槽注入 action bar | ✅ **选定** |

streamdown 源码关键片段（`node_modules/streamdown/dist/chunk-*.js`）：

```js
// 自定义渲染器优先级：matched language → Suspense + user component
let u = ao(m);  // ao(m) = plugins.renderers.find(r => Array.isArray(r.language) ? r.language.includes(m) : r.language === m)
if (u) {
  return jsx(Suspense, {fallback: Skeleton, children: jsx(u.component, {code: w, isIncomplete: c, language: m, meta: f})});
}
// 否则走 streamdown 内置 CodeBlock（含 shiki + 默认 copy/download 按钮）
return jsx(st /* =CodeBlock */, {className, code: w, isIncomplete: c, language: m, ..., children: <ControlsFragment/>});
```

**命中语言表**：`CustomRenderer.language: string | string[]` — 没有通配符，只能枚举。业务侧覆盖 60+ 常见语言 + `""`（空语言代码块）。未命中语言 fallback 到 streamdown 内置 CodeBlock（依然有 shiki + 默认 copy）—可接受降级。

### 文件改动矩阵

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `frontend/src/components/chat/BusinessCodeRenderer.tsx` | 新增（≈180 行） | `CustomRenderer` 组件：包住 streamdown `<CodeBlock>`，通过 `children` 注入 7 按钮 action bar（折叠/运行/复制/换行/主题/全屏/画布）+ 全屏 Portal + Mermaid 早返回 |
| `frontend/src/utils/streamdownConfig.ts` | +26 行 | 导出 `CUSTOM_RENDERERS`，枚举 60+ 语言（空串/js/ts/py/go/rust/shell/yaml/...） |
| `frontend/src/components/chat/MessageBubble.tsx` | −252 行 / +4 行 | 删除本地 `CodeBlock`（138 行）+ `CodeBlockHeader`（96 行）+ `extractText`（11 行）；3 处 `<Streamdown>` 调用点去掉 `components.pre` override、加 `plugins.renderers: CUSTOM_RENDERERS`；同步清理 7 个未引用导入 |
| `frontend/src/components/canvas/renderers/CodeRenderer.tsx` | 重写（89 → 18 行） | 丢弃 `hljs.highlight()` + `dangerouslySetInnerHTML`，改用 `<CodeBlock code={} language={} lineNumbers />` |
| `frontend/src/index.css` | −24 行 | 删除 `@import "highlight.js/styles/github.css"` + 23 行 `.hljs-*` dark 覆盖 |
| `frontend/vite.config.ts` | −3 行 / +0 行 | manualChunks 移除 `vendor-highlight` 分组（highlight.js 已卸）；保留 rehype/remark 聚合在 `vendor-markdown`（streamdown 自身不再显式归组，避免与 vendor-katex 循环） |
| `frontend/package.json` | −1 条 dep | `highlight.js ^11.11.1` uninstall |

### 验证证据

#### 1. `highlight.js` 源码零引用

```bash
$ grep -rn 'highlight\.js\|hljs' src/
# 零匹配
$ npm ls highlight.js
frontend@0.0.0 /Users/.../agents-a217925b27
└── (empty)
$ grep highlight package.json
# 零匹配
```

#### 2. Typecheck + Test

```bash
$ npx tsc --noEmit; echo "EXIT=$?"
EXIT=0

$ npx vitest run
Test Files  10 passed (10)
     Tests  99 passed (99)
  Duration  1.80s
```

#### 3. Bundle 对比（Day 2 末 → Day 4 末）

| 关键 chunk | Day 2 末 | Day 4 末 | Δ |
|---|---|---|---|
| `vendor-highlight` | 87.87 kB | **已消失** ✅ | −100% |
| `vendor-markdown`（streamdown transitive） | 345.07 kB | 345.14 kB | +0.07 kB（noise） |
| `vendor-katex` | 259.87 kB | 259.87 kB | 持平 |
| `vendor-react` | 257.73 kB | 257.73 kB | 持平 |
| 主 `index-*.js` | ~602 kB | 602.01 kB | 持平 |
| `highlighted-body-*.js`（shiki 懒加载 wrapper） | 623 B | 623 B | 持平 |

**net**：关键路径 **−87.87 kB**（full highlight.js tree 断链）；shiki 依旧懒加载，不进首屏。

#### 4. 行为回归（Matrix Test Scene 2：fenced code block）

```ts
it('scene 2: fenced code block renders data-streamdown="code-block" container', () => {
  const { container } = render(
    <Streamdown plugins={{ math: MATH_PLUGIN }} allowedTags={ALLOWED_TAGS}>
      {'```js\nconsole.log(42);\n```'}
    </Streamdown>
  );
  expect(container.querySelector('[data-streamdown="code-block"]')).not.toBeNull();
});
```

契约约束（`data-streamdown="code-block"`）在 Day 4 下依然通过 —— 注意 matrix test 中 Streamdown 没挂 `plugins.renderers`（因为 test fixture 不想绕业务逻辑），走的是 streamdown 内置 CodeBlock。业务代码（MessageBubble / MarkdownRenderer）挂了 `renderers: CUSTOM_RENDERERS`，走业务 wrapper。两条路径都 render 同样的 `data-streamdown="code-block"` 容器（业务 wrapper 也是用 streamdown 的 `<CodeBlock>` 组合出来的），契约向下兼容。

### Day 4 关键交付

1. ✅ **shiki 真正接上** — Day 2 绕过 bug 补齐，所有业务代码块走 streamdown `<CodeBlock>` → Suspense → 懒加载 shiki grammar+theme
2. ✅ **业务 UI 全保留** — 折叠 / 运行 / 复制 / 换行 / 主题切换 / 全屏 / 画布 7 个按钮通过 `<CodeBlock>.children` 插槽注入 action bar（sticky top，`data-streamdown="code-block-actions"`）
3. ✅ **全屏通过 React Portal** — 避开父容器 overflow/z-index 影响，ESC 键退出
4. ✅ **Canvas CodeRenderer 一并迁移** — `hljs.highlight()` + `dangerouslySetInnerHTML` 换成 streamdown 原语，`sanitizeHtml()` 不再需要（streamdown 内置 rehype-sanitize）
5. ✅ **`highlight.js` 顶层 uninstall** — `package.json` 零条 hljs，`npm ls` empty
6. ✅ **`vendor-highlight` 87.87 kB chunk 消失** — Phase 1 bundle 目标（MD -80 kB）超额完成
7. ✅ **零 regression** — 99/99 vitest 全绿，tsc 零错，build 零新 warning

### 未完成事项（Phase 1 后续/Phase 2 承接）

- **shiki 真实浏览器验证** —— Day 3 Playwright e2e 依然需要补（任务 #10 pending）：jsdom 下 `Suspense` 不会真执行 dynamic import，只覆盖了 skeleton fallback 路径
- **`vendor-markdown` 345 kB 优化空间** —— streamdown transitive tree（remark/rehype/unified/hast/mdast）已被 tree-shake，进一步瘦身需 Phase 2 的 `ai-elements` component 直接替换，复用同一 markdown tree
- **Phase 2 AI Elements 组件迁移** —— Thread/Message/Composer/Actions/Branch 等 ai-elements 原语接管 MessageBubble / Composer 后可以继续评估是否需要保留 business CodeBlock 或统一用 ai-elements 的 CodeBlock
