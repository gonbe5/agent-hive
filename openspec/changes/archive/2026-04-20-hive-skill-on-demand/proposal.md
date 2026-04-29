## Why

当前 Hive 的 skill 系统有**两个耦合的底层漏洞**，合起来卡住 agent harness 的能力扩展：

1. **启动期静态加载独木桥**：bootstrap 时 `finder.DiscoverAndRegister()` 跑一次就结束。`internal/tools/skill.go:116-119` 是 LLM 调 skill 的唯一入口，`registry.Get(name)` 失败直接报错，**没有自愈链路**。用户在对话框输入"我需要女娲 skill"只能得到 "not found"
2. **单租户扁平命名空间**：`internal/skills/` 包对 scope 毫无感知（`grep -n "scope\|tenant\|public\|personal" internal/skills/*.go` 零结果），而 `SessionState.UserID` 已经有租户概念。所有 skill 共享一个全局 namespace，**无法区分"运维统一下发的公共 skill"和"单个用户按需安装的个人 skill"**

> `internal/skills/discovery.go` 的 `Pull(ctx, url)` 底座 60% 可用；缺的是"对话 → 意图 → 解析 → 单包下载 → 租户感知热注册 → 重试"的完整闭环 + 公共/个人双轨命名空间。

## Alignment Matrix（与现有 5 个 OpenSpec change 的对齐关系）

| 现有 Change | 对齐点 | 冲突风险 | 本 change 的应对 |
|---|---|---|---|
| **`add-spec-driven-cognition` / `spec-driven-subagents`**（强耦合） | ① `provides_requirements: []string` 字段在 SKILL.md + marketplace index.json **共享语义**；② `Skills.FindBySpecRequirements(reqs)`（本地）与本 change `Discovery.ResolveByRequirements(reqs)`（远程）**分工协作**：spec planner 调度顺序 local-first → remote-fallback | `discovery.go` / `skill.go` **同 PR 修改** | 本 change 的 `provides_requirements` 字段完全复用前者语义；新增 `ResolveByRequirements` 接口承接前者 `FindBySpecRequirements` miss 场景（D14）；明确方法分工 + 调用顺序 |
| **`add-spec-driven-cognition` / `permission-minimalism`**（硬冲突） | 前者将 `createPermissionPromptFn` 重构为**默认 Granted:true**，只有 shell-family + 业务决策 `input_request{choice_type}` 触发 HITL | 若本 change 依赖 `PermissionRule{skill_install: Ask}` **会被静默 bypass**（非 shell 工具默认 allow） | **放弃 PermissionRule.Ask 路径**，改走 `input_request{choice_type: "skill_install_confirmation"}` 业务决策 HITL（D7 重写）——与 permission-minimalism 改革后的单一 HITL 通道对齐 |
| **`add-spec-driven-cognition` / `spec-driven-subagents`**（SubAgent 继承） | `SubAgent.Context["spec_ref"]` 驱动的 SubAgent 可能需要在 spec 执行期安装 skill | SubAgent 默认不继承主 session 的 userID / AdminChecker，会导致 `scope=personal` 拒绝或 `scope=public` 全拒 | 本 change 定义 SubAgent 派生规则（D16）：SubAgent spawn 时强制继承父 session 的 UserID + AdminChecker 引用 |
| **`im-streaming-reply`**（事件协议对齐） | `Master.BroadcastGenericMessage` 是统一事件通道，IM EventRenderer / WebSocket 同源消费 | 无冲突；纯复用 | `skill.install.progress` 事件走同一通道（D10），前端/IM 零新协议（复用 EventRenderer） |
| **`chat-ui-migrate-ai-elements`**（前端消费对齐） | 前端 `useHiveAgentEvents` hook 消费 Hive 事件流 | 无冲突；纯复用 | 安装进度 UI 通过 `useHiveAgentEvents` 订阅 `skill.install.*` 事件（`input_request` 的 HITL 按钮复用现有业务决策 UI） |
| **`chat-ui-polish`**（UI 基础） | Tool primitive 渲染基础 | 无冲突；纯复用 | `skill_install` / `skill_search` 作为普通工具走 `ToolInvocationChip` / `ToolExecutionBlock` 现有渲染链 |

### 合并策略

- **与 `add-spec-driven-cognition` 共享文件的改动必须同一 PR 落地**：`internal/skills/discovery.go`（resolver 扩展）+ `internal/skills/skill.go`（frontmatter `provides_requirements` 字段）。⚠️ 注意：`add-spec-driven-cognition/proposal.md` 引用的 `internal/skills/manifest.go` 在 Hive 实际不存在，对应文件是 `internal/skills/skill.go`——本 change 的 tasks 里加一条协调任务对齐命名
- **feature flag 矩阵**（D15）：`specdriven.enabled` × `specdriven.subagent_mode` × `specdriven.skills_semantic_routing` × `agent.skills.on_demand_enabled` 四维组合，本 change 定义全 8 个有效组合下的行为契约
- **独立 merge**：两个 change 对 fixture/测试数据独立；但 `discovery.go` 的 schema 扩展放本 change，`spec-driven-subagents` 方仅消费（避免并 review 冲突）

## What Changes

### 新增（按需安装闭环）

- **新增 `skill_install` 工具**：LLM 显式调用，入参 `{name, scope?, source?, refresh?}`；完成"按名解析 → 单包下载 → 租户感知热注册 → 返回结果"；**权限走 `input_request{choice_type: "skill_install_confirmation"}`**（与 permission-minimalism 对齐），非 `PermissionRule.Ask`
- **新增 `skill_search` 工具**：按描述/标签/requirement 搜索可用 skill（含 marketplace + 本地），返回候选列表；只查不写，无 HITL
- **新增 `Discovery.ResolveByName(ctx, name)`**：多 marketplace 聚合按名查找
- **新增 `Discovery.ResolveByRequirements(ctx, reqs)`**：**与 `spec-driven-subagents` 分工**——前者 `Skills.FindBySpecRequirements` 扫本地 Registry miss 后 fallback 到本方法查 marketplace
- **新增 `Discovery.PullOne(ctx, source, name)`**：按名拉单个 skill（区别于 `Pull` 整仓拉）
- **新增 Registry 热注册 `RegisterFromPath(ctx, path, scope, userID)`**：扫 SKILL.md → 解析 → 注册到指定 scope
- **新增 `skill` 工具 not-found 自愈提示**：条件开启，返回 `suggested_action: {tool: skill_install, args}` 让 LLM 自然跟进

### 新增（公共/个人 skill 双轨）

- **新增 Scope 模型**：`internal/skills/` 引入 `SkillScope` 类型，值域 `{ScopePublic, ScopePersonal}`
- **新增租户感知 Registry**：`Registry` 扩展 `Get(name, userID)` / `List(userID)` 语义——同名 skill 优先返回 `ScopePersonal`（用户 override），fallback 到 `ScopePublic`
- **新增存储分层**：
  - 公共 skill：`$HIVE_DATA/skills/public/<name>/SKILL.md`（运维/admin 管理）
  - 个人 skill：`$HIVE_DATA/skills/users/<userID>/<name>/SKILL.md`（用户运行时安装）
- **新增权限差异**（全部经 `input_request` 单通道）：
  - `skill_install scope=personal`：普通用户 → `input_request{choice_type: "skill_install_confirmation", scope: "personal"}` → 用户审批
  - `skill_install scope=public`：`AdminChecker.IsAdmin(ctx, userID)` 拦截 → 非 admin 直接拒绝；admin 用户仍需 `input_request{choice_type: "skill_install_confirmation", scope: "public"}` 二次确认
- **新增 UserID 注入**：`skill` 工具从 `SessionState.UserID` 读取租户 ID，传入 `skillReg.Get(name, userID)`
- **新增 SubAgent 继承**：SubAgent 通过 `spec_ref` Context 派生时，父 session 的 `UserID` + `AdminChecker` 引用**强制继承**

### 新增（Spec planner 分工协议，对齐 `add-spec-driven-cognition`）

- **新增 `SpecSkillResolver` 聚合接口**：封装"本地 `FindBySpecRequirements` miss → 远程 `ResolveByRequirements`"调度顺序，供 spec planner 单点调用（避免两个 change 的 planner 实现漂移）
- **新增 feature flag 矩阵定义**：`specdriven.skills_semantic_routing` × `agent.skills.on_demand_enabled` 共同决定 planner 路由行为；二者全 off 时退化到 name-based 旧路径

### 修改（非破坏性）

- `internal/tools/skill.go`：Get 传 userID；not-found 分支返回结构化 `suggested_action`
- `internal/skills/discovery.go`：拆 `PullOne` + `ResolveByName` + `ResolveByRequirements`；索引 schema 升级（加 `version`/`tags`/`provides_requirements`/`checksum`/`scope_hint` 可选字段）
- `internal/skills/skill.go`（**对齐 `add-spec-driven-cognition` 的 `manifest.go` 引用**）：`SkillMetadata` 加 `Scope` + `ProvidesRequirements`
- `internal/skills/registry.go`：加 scope 维度 + version-aware 注册
- `internal/skills/finder.go`：扫描路径扩展 `$HIVE_DATA/skills/public` + `$HIVE_DATA/skills/users/<userID>`
- `internal/config/config.go`：`Agent.Skills` 加 `MarketplaceURLs` / `OnDemandEnabled` / `PinnedVersions` / `PublicSkillsDir` / `PersonalSkillsDir` 字段
- `internal/bootstrap/server.go` + `internal/cli/app.go`：注册新工具 + 配置传参 + SubAgent spawn 注入 userID/AdminChecker
- `internal/config/defaults.go`：`skill_search` 默认 Allow（`skill_install` 不在 PermissionRule 体系内，走 `input_request` 通道）

### 不做（明确出 scope）

- **不做** marketplace 服务端实现（HTTP + index.json 协议沿用）
- **不做** skill 签名校验（保留 `checksum` 字段但不强制）
- **不做** 自动升级（仅"缺则装"）
- **不做** CLI 命令式入口（Hive 对话驱动）
- **不做** skill_uninstall 工具（follow-up change）
- **不做** 跨租户 skill 共享（个人 skill 严格隔离）
- **不做** admin role 新增（复用现有 auth 体系；本 change 定义 `AdminChecker` 接口但 admin 判定实现交给上游 auth 模块）
- **不做** 修改 `add-spec-driven-cognition/spec-driven-subagents` 现有 requirements（只扩展 marketplace 侧承载同字段）
- **不做** 修改 `permission-minimalism` 的 shell tool gatekeeper 策略（复用其 `input_request` 通道）

## Capabilities

### New Capabilities

- `hive-skill-on-demand`: 定义对话驱动的 skill 按需解析与安装协议 + 公共/个人 skill 双轨命名空间——包含 `skill_install` / `skill_search` 工具契约、Discovery 按名/按 requirement 解析与单包下载、Registry 租户感知热注册与版本管理、skill 工具的自愈提示路径、marketplace 索引 schema（与 `spec-driven-subagents` 对齐）、公共/个人 scope 存储分层、**基于 `input_request` 的 HITL 权限模型**（与 `permission-minimalism` 对齐）、**SubAgent userID/Admin 继承契约**（与 `spec-driven-subagents` 对齐）、**Spec planner 本地-远程分工调度协议**、**feature flag 矩阵定义**、配置与灰度开关。

### Modified Capabilities

<!-- 本 change 不修改任何 existing capability 的现有 requirements。与 spec-driven-subagents / permission-minimalism 的对齐是"扩展 + 分工协作"，不改变前者 requirements。依赖与协作关系在 Alignment Matrix 章节声明。 -->

## Impact

- **代码**：
  - **新增** `internal/tools/skill_install.go`（`skill_install` + `skill_search` 工具，**`skill_install` 通过 `input_request{choice_type}` 触发 HITL**；`init()` 调 `master.MustRegisterChoiceType("skill_install_confirmation")`）
  - **新增** `internal/skills/scope.go`（`SkillScope` 类型 + helpers）
  - **新增** `internal/skills/resolver.go`（`ResolveByName` + `ResolveByRequirements` + 多 marketplace 聚合）
  - **新增** `internal/skills/spec_resolver.go`（`SpecSkillResolver` 聚合接口：本地 `FindBySpecRequirements` miss → `ResolveByRequirements`）
  - **新增** `internal/skills/admin.go`（`AdminChecker` 接口 + `denyAllAdminChecker` 默认实现；接口文档标注 **goroutine-safe 契约**）
  - **新增** `internal/db/migrations/NNNN_skills_user_id.sql`（skills 表加 `user_id` 列 + 复合唯一索引 + pg_notify payload 扩展；详见 design.md D18）
  - **修改** `internal/skills/discovery.go`（`PullOne` + 索引 schema 字段扩展；**与 `spec-driven-subagents` 的 `provides_requirements` 字段共识**）
  - **修改** `internal/skills/skill.go`（frontmatter 加 `Scope` + `ProvidesRequirements`；**对齐 `add-spec-driven-cognition` 误引用的 `manifest.go`**）
  - **修改** `internal/skills/registry.go`（scope 维度 + `RegisterFromPath` + version-aware Register；含 `FindBySpecRequirements` stub fallback 供 `spec-driven-subagents` 未合入时兜底）
  - **修改** `internal/skills/overlay_registry.go`（**BLOCKER 1**：`Get/List/RegisterFromPath` 显式重写新签名对齐租户；`dbCache` key 从 `map[string]*dbEntry` 改为 `map[dbCacheKey]*dbEntry`；四层查找优先级 personal DB > personal FS > public DB > public FS，见 design.md D17）
  - **修改** `internal/skills/service.go`（**MAJOR 2**：`SkillService` 解析 pg_notify payload 的 `user_id` 字段；`dbCache` 更新按 `{name, userID}` 复合 key；旧 payload 无 user_id 按 public 兼容）
  - **修改** `internal/skills/finder.go`（扫描路径扩展）
  - **修改** `internal/tools/skill.go`（userID 注入 + not-found 结构化响应）
  - **修改** `internal/mcphost/host.go`（**Host 层新增 `EmitInputRequest` 透传**：`NewHost` 签名扩展接收 Master / HITLEmitter；方法内部 `req.SessionID = sessionIDFromCtx(ctx)` 后调 `Master.EmitInputRequest`，底层由 `hitl-choice-type-registry` 提供的 `internal/master/host_emit.go:31-87` 已完备，仅需透传）
  - **修改** `internal/master/choice_type_registry.go` 引用：**不修改**内置列表（hitl-choice-type-registry/spec.md:86 禁止），改由本 change 在 `skill_install` init 主动 `MustRegisterChoiceType`
  - **修改** `internal/config/config.go` + `internal/config/defaults.go`（含 `validateFlagCombination` 启动期校验 16 flag combos fail-fast）
  - **修改** `internal/bootstrap/server.go` + `internal/cli/app.go`（含 SubAgent spawn 继承 userID/AdminChecker；`Host` 构造期注入 Master 引用）
- **后端 API**：零破坏。现有 `skill` 工具入参/出参 schema 不变；`Get` 的 userID 参数走函数签名扩展，调用方小改
- **协议**：
  - marketplace `index.json` schema 向前兼容（新字段可选）
  - skill 工具 not-found 响应体新增 `suggested_action` 字段（向后兼容）
  - `skill_install` HITL：通过现有 `input_request` 事件（`choice_type: "skill_install_confirmation"`）触发，与 `permission-minimalism` 单通道对齐——前端/IM 零新协议
  - **与 `spec-driven-subagents` 对齐**：`provides_requirements` 字段在本地 SKILL.md 和 marketplace index 两处使用相同语义；`SpecSkillResolver` 聚合前者 `FindBySpecRequirements` + 本方 `ResolveByRequirements`
  - `skill.install.progress` EventBus 事件走 `Master.BroadcastGenericMessage`（对齐 `im-streaming-reply`）
- **配置**：
  - 新增 `agent.skills.marketplace_urls: []string`
  - 新增 `agent.skills.on_demand_enabled: bool`（默认 false，灰度开启）
  - 新增 `agent.skills.public_skills_dir: string`（默认 `$HIVE_DATA/skills/public`）
  - 新增 `agent.skills.personal_skills_dir: string`（默认 `$HIVE_DATA/skills/users`）
  - 新增 `agent.skills.pinned_versions: map[string]string`
- **权限**：
  - `skill_search`：默认 Allow（`DefaultPermissionRules`，只查不写）
  - `skill_install`：**不走 PermissionRule 体系**，而是在 handler 内部调 `host.EmitInputRequest(ctx, choice_type="skill_install_confirmation", ...)` 触发业务决策 HITL（对齐 `permission-minimalism` 改革后的单一 HITL 通道）；admin 判定额外叠加
  - `scope=public` 安装：`AdminChecker.IsAdmin` 拦截（默认实现 deny-all）
- **测试**：
  - `ResolveByName` / `ResolveByRequirements` / `PullOne` 单测（`httptest.Server` 模拟 marketplace）
  - `Registry` 多版本 + scope 双轨场景单测
  - `skill_install` 集成测试：`scope=personal` / `scope=public` 两条路径 + `input_request` HITL 路径 + `AdminChecker` 拦截
  - `skill` 工具 not-found 自愈提示单测（含 userID 注入）
  - SubAgent 继承单测：`spec_ref` 派生 SubAgent 调 `skill_install` 必须带父 session UserID
  - feature flag 矩阵单测：`specdriven.skills_semantic_routing` × `on_demand_enabled` 组合下的 planner 路由行为
  - 端到端场景测试：用户对话 → LLM 调 install → 下载 → 注册 → 调用新 skill
- **文档**：
  - marketplace 协议与 `index.json` schema 升级说明（**含与 `spec-driven-subagents` 的协议对齐章节**）
  - 公共/个人 skill 运维指南
  - 用户对话样例
  - SubAgent 继承契约说明（交叉引用 `spec-driven-subagents`）
  - feature flag 矩阵运维 cheat-sheet
- **风险**：
  - **网络副作用**：30s 超时 + 失败透传 LLM；5min 索引缓存
  - **安全**：复用 `input_request` HITL；`checksum` 字段保留不强制（follow-up change 强化）
  - **跨租户泄漏**：个人 skill 必须严格按 userID 隔离；Registry 层面强制 scope 检查
  - **多 marketplace 冲突**：按配置顺序优先级；同名冲突返回明确错误
  - **与 `add-spec-driven-cognition` 的 PR 冲突**：`discovery.go` / `skill.go` 改动必须同一 PR；两个 change 的 tasks 中"协议对齐"组明确由本 change 落地
  - **灰度期行为不一致**：`on_demand_enabled=false` 时新功能全关；开启前必须配置 `marketplace_urls`（启动期校验报错）
  - **SubAgent userID 丢失** → 无法安装 personal skill → spec 执行失败：通过 Bootstrap 层强制注入 + 单测兜底
  - **🔴 BLOCKER 1 — `OverlayRegistry` 架构冲突**：生产绑定 `*skills.OverlayRegistry`（server.go:52,400-401），它已做 DB>FS layering by name；若只改 `Registry` 不改 `OverlayRegistry`，Go embedding 不重派发新签名 → personal 层线上静默失效 + `dbCache` 跨租户泄漏。化解：design.md D17 四层优先级（personal DB > personal FS > public DB > public FS）+ tasks.md task 2.8 显式 override + `dbCache` key 改 `{name, userID}` 复合结构
  - **🔴 BLOCKER 2 — `skill_install_confirmation` 未注册**：`hitl-choice-type-registry/spec.md:86,94-97` 禁止内置；`choice_type_registry.go:91-104` 只注册 3 个；`host_emit.go:32-34` 硬抛 `ErrUnregisteredChoiceType`。若不主动注册，handler 首次 HITL 即 100% runtime failure。化解：tasks.md task 6.0 显式 `MustRegisterChoiceType` + 单测首个 case 断言 `IsRegisteredChoiceType` 返回 true
  - **🟡 MAJOR 1 — `FindBySpecRequirements` 符号未落地**：全仓 grep 0 命中；`spec-driven-subagents` 仅 spec 已 archive 但代码未落。化解：tasks.md task 4.2 stub fallback — 若 `spec-driven-subagents` 未合入，本 change 自行 stub 为 `List(userID).filter(reqs)` 走兜底
  - **🟡 MAJOR 2 — `SkillService` pg_notify × personal layer 竞态**：当前 pg_notify 按 name 更新 `dbCache[name]`，两用户推同名 personal skill 后到的 NOTIFY 覆盖前者。化解：design.md D18 DB migration（skills 表加 `user_id` 列 + 复合唯一索引 + payload 扩展）+ tasks.md task 2.9 监听端改复合 key
  - **🟡 MAJOR 3 — `AdminChecker` 并发安全契约缺失**：D16 要求"同实例引用"实现热更新，但接口未声明 goroutine-safe；并发读写 `IsAdmin` 可能 race。化解：tasks.md task 5.1 接口文档硬约束 + `atomic.Pointer` / `RWMutex` 实现 + `go test -race -count=100`
  - **🟡 MAJOR 4 — `skill_install` 6 阶段 goroutine 泄漏**：resolving → awaiting_approval → downloading → registering → done/error 多段异步；若 worker 不监听 `ctx.Done()`，用户 decline / timeout / mid-cancel 时 goroutine 堆积。化解：tasks.md task 6.3 ctx-aware worker + 单测 `goleak.VerifyNone`
  - **🟢 MINOR 1 — `req.SessionID` 注入路径**：HITL 请求缺 SessionID 前端/IM 无法 routing。化解：tasks.md task 6.2b `Host.EmitInputRequest` 内部 `req.SessionID = sessionIDFromCtx(ctx)`
  - **🟢 MINOR 2 — 16 flag combos 有效性校验**：4 bool flags = 16 combos，D15 只列 9 组有效；7 组（`specdriven.enabled=false` + `subagent_mode=true` 或 `semantic_routing=true`）属依赖违反。化解：tasks.md task 10.3 `validateFlagCombination` 启动期 fail-fast + task 12.1 单测枚举 16 组
- **向后兼容**：
  - `agent.skills.urls` 配置保留（启动期整仓拉行为不变）
  - `skill` 工具入参/出参兼容；`suggested_action` 新增字段可选
  - SKILL.md 未声明 `scope` 时默认 `ScopePublic`（对齐现有 skill 的隐式语义）
  - 存量 `.claude/skills` / `~/.claude/skills` / `skills` 路径继续扫描，归入 `ScopePublic`
  - `on_demand_enabled=false` 时 `skill` 工具 not-found 路径 **byte-identical** 于 pre-change
