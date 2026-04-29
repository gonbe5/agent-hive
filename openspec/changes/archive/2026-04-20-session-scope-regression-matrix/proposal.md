> **Status banner (2026-04-20, RESCOPED)**：前置依赖 `subagent-session-scoping` 已归档——R-1/R-2/R-3 反例 fixture 可驱动 subagent 双 callback + lifecycle 三类事件。
>
> **RESCOPE 原因**（plan-ceo-review + codex 双审查 2026-04-20）：
> 1. **R-1/R-2/R-3 保护层降级为 grep-based CI check**——仓库无 `.golangci.yml` / `analysis.Analyzer` / `tools/analyzer` 基建，从零造 analyzer 成本远超红测本身价值；改用 shell lint 脚本（grep `internal/master/` 下裸 `Broadcast(BroadcastMessage{...})` 白名单外调用）即可达到同等阻断效果
> 2. **原 "Cross-tenant penetration matrix" 改名 "Cross-session penetration matrix"**——真实安全边界是 session，飞书 tenant_key 维度**本 change Out-of-Scope**（代码先长出 tenant 字段后另起 change）
> 3. **CI harness "under 8 minutes" 硬断言删除**——新 workflow 从 0 起步无 POC baseline，改为 Phase 0 本地采 baseline → runbook 定 SLO 的循环

## Why

P10 CTO + Codex 双线评审 2026-04-20 在审 `subagent-session-scoping` / `frontend-ws-handshake-regression` / runbook 补强 三个 follow-up 决策时，**共同指出**了一个被所有 change 漏掉的战略缺口：

> **CTO 视角的 root cause**：runbook 11.8 与 frontend-ws-handshake-regression 在测同一件事——前者人工后者自动。**为什么我们的 CI 跑不了 longconn + 浏览器握手 e2e？** 这是基建短板，不是某个 change 能解决的。
>
> **Codex 视角的 Q7 FAIL**：3 个决策没覆盖负向红测、跨租户渗透矩阵、系统性 WS reconnect race 自动化。原 `im-streaming-reply` 12.x 系列仍缺一条专门回归矩阵承接。

`im-streaming-reply` 的 spec 12.4 约束（session-scoped event types MUST go through BroadcastSessionMessage）目前**只有 happy-path 测试保护**——没人写过"故意泄漏"的红测。一旦未来某个 PR 把 `BroadcastSessionMessage` 优化回 `Broadcast`，CI 不会红，spec 在文档上还绿——这正是 `subagent-session-scoping` 揭出的"spec drift"问题的**普适形态**。

**底层逻辑**：spec 不被红测保护 = 文档上的契约。我们已经被这个问题咬过一次（subagent / lifecycle 盲区），不能再咬第二次。

## What Changes

### 0. Phase 0 可行性 Spike（新增，双审查员要求）

在全体 red test 开写**之前**，先做两个 POC 验证假设：

- **Spike A（enforcement 机制选型）**：写 `scripts/ci/check_session_scope.sh`，grep `internal/master/*.go` 下所有 `eventBus.Broadcast(BroadcastMessage{...})` 调用，排除带 `// no session scope by design` 注释的白名单；脚本本地跑对当前仓库必须**全绿**（现状已 clean，由 `subagent-session-scoping` 归档保证）。如果 shell 脚本方案在复杂表达式上有漏报/误报，再升级到 ast-grep / `go/ast` 轻量静态扫描。不造 golangci-lint 自定义 analyzer。
- **Spike B（CI harness baseline）**：本地启 `go run ./cmd/server/main.go --config config.test.json` + mock 飞书 longconn + headless playwright，记录一次 full stack 冷启动 + 单测 case 运行耗时；该数字写进 `docs/runbooks/ci-baseline.md` 作为新 workflow SLO 的**证据基线**，不在 spec 里硬编码分钟数。

两个 spike 都绿再开 Phase 1。

### 1. 故意泄漏红测（Adversarial Regression Tests）

构造一组**故意违反 spec 12.4 的代码模板**作为反例 fixture：

- **R-1**：subagent 进度事件用裸 `m.eventBus.Broadcast(BroadcastMessage{...})` 不带 SessionID
- **R-2**：tool_call 事件用 `BroadcastGenericMessage` 不携带 sessionID
- **R-3**：lifecycle 事件（AgentCreated / Destroyed / ToolListChanged）用 broadcast-scoped 路径但**未在注释中标注 "no session scope by design"**

每个反例对应 CI **grep-based check 必红**——Spike A 的 shell 脚本作为 CI step，命中违规即阻塞 PR。R-1/R-2 的 "go test" 层额外断言 fixture 构造的消息**未**被订阅端收到（防 grep 漏判）。

### 2. 跨 Session 渗透矩阵（Cross-session penetration matrix）

构造 N×M 矩阵：N 个独立 session（不同 SessionID，UserID 仅作样本构造用），M 类事件（message / tool_call / agent_progress / agent_created / agent_destroyed / tool_list_changed / skill_install_progress）。

**断言**：session A 的事件**绝不**到达 session B 的 WebSocket / IM EventRenderer 订阅端。

输出：`tests/regression/session_scope_matrix_test.go` 全绿 = 渗透零；任一格变红 = 立即阻塞 PR。

> **说明**：此矩阵的真实安全边界是 **SessionID**，非 tenant。飞书 `tenant_key` 多租户维度**本 change Out-of-Scope**——主 spec `im-streaming-reply` 尚未建模 tenant 字段（`grep tenant internal/master/` 当前 0 命中）。若未来代码长出 tenant 层，另起独立 change 补矩阵维度。

### 3. WebSocket reconnect race 自动化（**Go 后端范围**）

> **Scope split（codex Round 2 修订）**：本 Phase 只做 **Go 后端 WS envelope 层** 的 race 自动化——不在 Go 测试里假装跑前端 zustand store spy。前端 `useChatStore.currentSessionId` / `setCurrentSessionId(null)` 断言由 `frontend-ws-handshake-regression` Phase 2 的 playwright spec 承担，在本 change Phase 4 提供的共享 harness 里运行；跨栈合约由共享 CI harness 端到端验证。

人工 runbook 11.8 走过的"发消息 → 断网 → 重连 → 发第二条消息"路径，自动化其 **Go 后端可观测量**：

- 三方 race 场景：reconnect mid-emit / reconnect mid-loadMessages callback / reconnect mid-handleDisconnected
- 断言：**后端 envelope** SessionID 在 3×disconnect/reconnect 循环中保持 = `"sX"`，subscriber-side filter 不因 envelope 空 SessionID drop
- 断言：reconnect 后第二条 inbound 消息触发 LLM 流（fake LLM 500ms 出首 chunk）→ 首 chunk 5s 内到达 **fake WS client recv queue**，test-captured log 无 `WebSocket session-mismatch drop`

### 4. CI 基建：longconn + 浏览器 e2e harness（CTO 视角的 root cause）

> **Scope decoupling（codex Round 2 修订）**：本 change 只交付 harness + Go 侧测试集合。前端 playwright spec 由 `frontend-ws-handshake-regression` 自行落地；harness 在**零 FE spec 存在时必须 PASS**（`npx playwright test` "no tests found" 视为成功），workflow YAML **严禁硬编码任何 FE spec 文件名**，避免形成"必须先有前端 case 才能 merge"的循环依赖。

新增 GitHub Actions（或对等 CI）workflow job（本 change 交付范围）：

- 启动 `go run ./cmd/server/main.go --config config.test.json`（含 mock 飞书 longconn endpoint）
- provisioned：headless browser (playwright) runtime + browser binary（供 `frontend-ws-handshake-regression` 后续落地的 `.spec.ts` 自动拾起）
- 跑 `tests/regression/session_scope_matrix_test.go` 全量
- 跑 `tests/regression/red_subagent_progress_raw_broadcast_test.go` / `red_subagent_stream_generic_test.go` / `red_lifecycle_unjustified_test.go` 三个故意泄漏反例
- 跑 `tests/regression/ws_reconnect_race_test.go`（Go 后端范围）
- 跑 `scripts/ci/check_session_scope.sh` grep 保护层
- 跑 `npx playwright test frontend/playwright/` —— 若目录为空或无 `.spec.ts`，MUST 以 "no tests found" 视为 PASS

**SLO 定义策略**：不在 spec 里硬编码分钟数。Phase 0 Spike B 采本地 baseline → 新 workflow 首周跑真实 CI 取 p95 → 按 p95 × 1.5 定 `timeout-minutes`，写进 `docs/runbooks/ci-baseline.md`。仓库现有 `test-specdriven.yml` `timeout-minutes: 15` 作参考锚点。

**这是 runbook 11.x 系列从人工 SOP 升级为 CI 红线的关键基建**。完成后，runbook 降级为"CI 故障时的人工兜底参考"，不再是上线阻塞门。

## Capabilities

### New Capabilities

- `session-scope-regression-matrix`: 跨 **session** SessionID 渗透自动化矩阵 + 故意泄漏反例 CI 守卫（grep-based enforcement，不造 analyzer）
- `e2e-ci-harness`: longconn + 浏览器 e2e 的 CI 基建（unblock runbook → CI 升级路径；SLO 由 Phase 0 baseline 驱动定义）

### Modified Capabilities
（无）

## Out-of-Scope（显式声明）

- **tenant_key 多租户维度**：`im-streaming-reply` 主 spec 尚未建模 tenant 字段；等代码长出后另起独立 change
- **golangci-lint 自定义 analyzer 基建**：仓库当前 0 analyzer 基建，ROI 不支持为本 change 从零造；grep-based shell check 即可覆盖 R-1/R-2/R-3 需求
- **Phase 3 `spec-driven-subagents` SubAgent 语义派活**：随 `add-spec-driven-cognition` 2026-04-20 archive 整体 descope，不在本 change 范围

## Dependency Graph

```
subagent-session-scoping (修代码)
  ↓ 提供被保护的 invariant 来源
session-scope-regression-matrix (本 change，加红测保护代码不回退)
  ↓ Phase 4 提供 CI harness
frontend-ws-handshake-regression Phase 2 (case 落地)
  ↓ 全部接入 CI
runbook 11.x 降级为 CI 故障兜底
```

**协调点**：
- 本 change 的红测必须在 `subagent-session-scoping` **合并之后**写——否则 CI 立即红（代码尚未修复 → 反例触发不了对应保护）
- 本 change 的 CI harness **可以与 `subagent-session-scoping` 并行开发**，但 e2e job 在两个 change 都 merged 后才启用

## Impact

- **代码**（本 change 范围）：
  - `tests/regression/red_subagent_progress_raw_broadcast_test.go`（新增）—— R-1 反例
  - `tests/regression/red_subagent_stream_generic_test.go`（新增）—— R-2 反例
  - `tests/regression/red_lifecycle_unjustified_test.go`（新增）—— R-3 反例（parse grep 脚本输出）
  - `tests/regression/session_scope_matrix_test.go`（新增）—— N×M 渗透矩阵
  - `tests/regression/ws_reconnect_race_test.go`（新增）—— 三方 race（**Go 后端范围**，前端 store spy 由 `frontend-ws-handshake-regression` Phase 2 承担）
  - `scripts/ci/check_session_scope.sh`（新增）—— grep-based CI 保护层
  - `.github/workflows/e2e-session-scope.yml`（新增）—— longconn + browser CI harness（**不硬编码任何 FE spec 文件名**）
  - `config.test.json`（新增）—— mock 飞书 longconn 端点
  - `docs/runbooks/ci-baseline.md`（新增）—— baseline 数字 + p95 × 1.5 SLO 推导
- **不在本 change 范围**（由 `frontend-ws-handshake-regression` 交付）：
  - `frontend/playwright/*.spec.ts` —— 前端 case 由另一 change 自行落地，harness 预先 provisioned runtime 等其拾起
- **测试**：本 change **就是测试**——核心产出物即测试代码
- **CI**：新增 1 个 workflow job，`timeout-minutes` 由 Phase 0 Spike B baseline → 首 7 green run p95 × 1.5 定稿（不在 spec 硬编码）
- **兼容性**：纯新增，无破坏
- **依赖**：
  - **Depends on**：`subagent-session-scoping`（已 2026-04-20 archive，反例有对应保护可触发）
  - **Parallel-compatible**：`frontend-ws-handshake-regression`（共享 workflow，但**不阻塞本 change 归档** —— FE spec 零文件时 harness 仍 PASS）
- **回滚**：CI workflow 可单独 disable；红测 fixture 可单独 skip；grep 脚本可从 CI step 摘除保留本地

## Verification

- `openspec validate session-scope-regression-matrix --strict` 通过
- 本地：`go test ./tests/regression/... -race -count=1` 全绿 + `bash scripts/ci/check_session_scope.sh` exit 0（前提：`subagent-session-scoping` 已 merged）
- 反向：手工 revert `subagent-session-scoping` 的 `master.go:579-613` 其中一处到裸 `Broadcast` → `scripts/ci/check_session_scope.sh` exit 非 0，对应 `red_*.go` 必须红
- CI：新 workflow 在 PR 上跑通；在反向 revert PR 上必须**红**（证明保护生效）；FE spec 目录为空时 playwright step 必须 **PASS**（证明时序解耦）
