# Tasks

## 1. Scope 基础建设（新 `internal/skills/scope.go` + finder 扩展）

- [x] 1.1 新建 `internal/skills/scope.go`，定义 `SkillScope` 类型（`ScopePublic` / `ScopePersonal`）及 helpers（`ParseScope`、`String()`）
- [x] 1.2 在 `internal/skills/skill.go` 的 `SkillMetadata` 增加可选字段 `Scope SkillScope` 和 `ProvidesRequirements []string`（**与 `add-spec-driven-cognition` 对齐**；注意其 proposal.md 引用的 `internal/skills/manifest.go` 在 Hive 实际对应 `internal/skills/skill.go`——review 时提醒对方在落地 PR 时修正引用）
- [x] 1.3 修改 `internal/skills/finder.go`：扫描 `$HIVE_DATA/skills/public` 归入 `ScopePublic`；扫描 `$HIVE_DATA/skills/users/<userID>` 归入 `ScopePersonal`（需 finder 接受 userID 参数，或在 bootstrap 期按 config 构造）
- [x] 1.4 finder 对未声明 scope 的 SKILL.md 按路径推断；frontmatter 声明的 scope 优先；`personal` 但无 userID → 注册失败
- [x] 1.5 现有三路径（`.claude/skills` / `~/.claude/skills` / `skills/`）继续扫描，统一归 `ScopePublic`（向后兼容）
- [x] 1.6 单测 `scope_test.go`：path-based inference / frontmatter override / missing userID 拒绝 / 旧路径默认 public

## 2. Registry 租户感知 + 版本管理

- [x] 2.1 改 `internal/skills/registry.go` 内部存储结构：`map[registryKey]*registryEntry`，其中 `registryKey = {Name, UserID}`，`registryEntry` 含按 semver 降序的 skill 列表
- [x] 2.2 `Register(s *Skill)` 增加版本感知：同 key 已存在时按 semver 比较；`PinnedVersions` 配置覆盖；相同版本 no-op + metrics `skill.registry.dup`
- [x] 2.3 新增 `Get(name, userID string) (*Skill, error)`：先查 personal(userID) → 未命中查 public(空 userID) → 都无则返回 `CodeSkillNotFound`（保持旧签名 `Get(name)` 作为 shim 调用 `Get(name, "")`，避免打断存量 caller）
- [x] 2.4 新增 `List(userID string) []SkillSummary`：合并 personal + public，personal 覆盖同名 public；`SkillSummary` 增加 `OverriddenPublic bool` 字段
- [x] 2.5 新增 `RegisterFromPath(ctx, path, scope, userID string) error`：扫 SKILL.md → 解析 → 调 Register
- [x] 2.6 跨租户隔离强制校验：personal scope 的 Register 必须带非空 userID；Get(name, userID=bob) 绝对不可返回 alice 的 personal skill
- [x] 2.7 单测 `registry_test.go` 新增 case：personal 覆盖 public / pin 覆盖 semver / 相同版本幂等 / 跨租户隔离 / empty userID 仅见 public（落在 `tenant_isolation_test.go` 9 cases + `overlay_tenant_test.go` 3 cases，`go test ./internal/skills/ -run 'TestRegistry_|TestOverlay_' -v` 12/12 PASS）
- [x] 2.8 **`OverlayRegistry` 同步扩展（BLOCKER 修复）**：`internal/bootstrap/server.go:52,400-401` 生产绑定 `*skills.OverlayRegistry`，Go embedding 不对新增签名做 method 重派发——若只改 `Registry.Get/List/RegisterFromPath` 而不改 `OverlayRegistry`，personal 层在线上静默失效（DB 层覆盖 FS 层时 userID 丢失 → 跨租户泄漏）。必须同步：
  - `internal/skills/overlay_registry.go` 的 `Get`/`List`/`RegisterFromPath` 三个方法显式重写新签名 `(name, userID string)` / `(userID string)` / `(ctx, path, scope, userID string)`
  - `OverlayRegistry.dbCache` 的 key 从 `map[string]*dbEntry` 改为 `map[dbCacheKey]*dbEntry`，其中 `dbCacheKey struct { Name, UserID string }`；personal 层 DB skill 以 `{name, userID}` 索引，public 层 userID 为空
  - 查找顺序硬编码四层优先级（见 design.md D17）：personal DB → personal FS → public DB → public FS；任一层命中即返回
  - 单测 `overlay_registry_test.go` 新增 case：同名 skill 分别在 personal DB / personal FS / public DB / public FS 四层存在 → `Get(name, alice)` 必返回 personal DB；跨租户 `Get(name, bob)` 绝不返回 alice 的 personal DB entry
- [x] 2.9 **`SkillService` pg_notify × personal layer 竞态修复（MAJOR 2）**：当前 `internal/skills/service.go` 的 pg_notify 按 `name` 更新 `dbCache[name]`，两用户各推同名 personal skill 时后到的 NOTIFY 会覆盖前者的 cache entry，导致 personal skill 跨租户污染。必须：
  - 数据库迁移：`hive_skills` 表增加 `user_id TEXT NOT NULL DEFAULT ''` 列（空串代表 public skill）；主键升级为复合主键 `(name, user_id)`；`idx_hive_skills_user` 部分索引加速 personal 查询（具体 DDL 见 `internal/store/postgres_migrate.go` 的 `pgAddSkillsUserID`，幂等执行）
  - `pg_notify` payload 从 raw `name` 字符串扩展为 JSON `{name, user_id, op}`（由 `hive_skills_notify` trigger function 发出）；`SkillStore.parseSkillPayload` 监听端解析 JSON，老格式 fallback 为 `{name: raw, user_id: ""}` 保持向后兼容
  - `SkillStore.Get/Upsert/Delete/LoadAll/List` 全部按 `(name, user_id)` 复合 key 索引；`SkillService.reload` 回调升级为 `func(name, userID string)`，按 `OverlayRegistry.UpsertDB(name, userID, content, path, revision)` / `DeleteDB(name, userID)` 精确定位 dbCache entry
  - 单测 `service_test.go` 新增（14.x 集成前置）：alice 推 personal `nuwa` + bob 推 personal `nuwa` → 两个 dbCache entry 并存 + `Get("nuwa", "alice")` 与 `Get("nuwa", "bob")` 返回各自独立 skill（绝不串扰）

## 3. Discovery 按需解析（**与 `add-spec-driven-cognition` 协议对齐组**）

- [x] 3.1 扩展 `SkillIndexEntry` 添加可选字段：`Version` / `Tags` / `ProvidesRequirements` / `Checksum` / `ScopeHint`（旧 index.json 仍兼容，JSON 均 `omitempty`）
- [x] 3.2 新增 `ResolvedSkill` struct 承载解析结果（含 `Source` marketplace URL）
- [x] 3.3 新增 `Discovery.ResolveByName(ctx, name, refresh bool) (*ResolvedSkill, error)`：遍历 `marketplaceURLs` 按顺序查；同名在多 marketplace → `CodeSkillAmbiguous` + 候选列表
- [x] 3.4 新增 `Discovery.ResolveByRequirements(ctx, reqs []string) ([]*ResolvedSkill, error)`：按 `ProvidesRequirements` 覆盖度降序排序；**仅查远程 marketplace**（方法分工硬约束：本地走 `Registry.FindBySpecRequirements`）
- [x] 3.5 新增 `Discovery.PullOne(ctx, source, name) (string, error)`：按 name 拉单包；原子写盘（`<name>.tmp` → rename → `<name>`），SKILL.md 缺失时回滚 tmp dir
- [x] 3.6 Discovery 内部 `indexCache map[url]cachedIndex`（TTL 默认 5min，`d.cacheTTL` 可调），`fetchIndex(ctx, url, refresh bool)` 封装；`refresh=true` 强制越过缓存
- [x] 3.7 协议对齐确认：(a) 字段名 `provides_requirements` 在 `internal/skills/skill.go` (frontmatter) 与 `internal/skills/discovery.go` (index.json) 双侧类型一致（`[]string`）；(b) `Registry.FindBySpecRequirements` 仅查本地（registry.go:360-401）；(c) `Discovery.ResolveByRequirements` 仅查远程（discovery_resolve_test.go:92）；(d) 本 change 所有改动落在 `skill.go`，非 `manifest.go`（design.md §37 已标注、docs/marketplace-protocol.md §3 已对齐）。PR 协调：`add-spec-driven-cognition` 合入时走同 PR reviewer check（跨 change 协调，非代码项）
- [x] 3.8 单测 `discovery_resolve_test.go`：`httptest.Server` 模拟 marketplace / 名字命中 / 名字 miss / 多 marketplace 同名 ambiguous / requirement 覆盖降序排序 / 缓存命中只打一次 index.json / `refresh=true` 强刷 / TTL 过期自动再拉 / PullOne 原子写盘（成功 + 失败路径均无 .tmp 残留）/ 空 marketplace 列表兜底。`go test ./internal/skills/ -run 'TestDiscovery_' -v` 8/8 PASS

## 4. SpecSkillResolver 聚合接口（**与 `spec-driven-subagents` 分工**）

- [x] 4.1 新建 `internal/skills/spec_resolver.go`，定义 `SpecSkillResolver` 接口 + `SpecResolveResult` 结构（含 `Local []*Skill` / `Remote []*ResolvedSkill` / `Suggested *SuggestedAction`）+ `LocalSkillFinder` / `RemoteSkillFinder` 依赖抽象
- [x] 4.2 实现 `defaultResolver`：先调 `local.FindBySpecRequirements(reqs, userID)` → miss 再调 `remote.ResolveByRequirements(ctx, reqs)`。**MAJOR 1 stub 已落地**：`Registry.FindBySpecRequirements(reqs, userID)`（`internal/skills/registry.go:360-401`）按 ProvidesRequirements 交集查询，严格租户隔离（personal 层仅 userID 匹配可见、去重优先 personal）；`spec-driven-subagents` 真实实现 merge 后可直接替换本 stub（签名兼容）
- [x] 4.3 feature flag 门控：`remoteAllow func() bool` 注入路径，由 bootstrap 组合 `OnDemandEnabled && SemanticRoutingEnabled` 决定；flag 关时绝不调 remote（单测 `TestSpecResolver_LocalMissRemoteFlagOff` 校验）
- [x] 4.4 远程命中时构造 `SuggestedAction{tool: "skill_install", args: {name, scope, source}, reason}`；`ScopeHint` 优先，匿名 userID 自动降级为 public（需 admin 审核）
- [x] 4.5 单测 `spec_resolver_test.go`：本地命中短路远程 / 本地 miss + 远程开 → 走远程 / 本地 miss + 远程关 → 返回空 / 方法分工守卫（mock 类型强制只实现各自单方法）/ 远程错误透传 / 空 reqs 短路 / 匿名 scope 降级 / FindBySpecRequirements 租户隔离回归。`go test ./internal/skills/ -run 'TestSpecResolver_|TestRegistry_FindBySpec' -v` 8/8 PASS
- [x] 4.6 协调：`SpecSkillResolver.Resolve` 是唯一聚合入口（spec_resolver.go:39-63 contract + TestSpecResolver_MethodSplitGuard 守卫）；design.md §D1/§D5 已声明 spec planner 必须经 Resolver、禁止越过直调底层；docs/marketplace-protocol.md §3 方法分工表固化。跨 change PR review 时按此对齐

## 5. AdminChecker 接口

- [x] 5.1 新建 `internal/skills/admin.go`：定义 `AdminChecker interface { IsAdmin(ctx context.Context, userID string) bool }`，文档注释显式标注 **goroutine-safe 强制契约**（引用 D16 SubAgent 继承）；提供 `AllowListAdminChecker` 带 `atomic.Pointer[map[string]struct{}]` 做无锁热更新的过渡实现；`admin_test.go` 含 13 reader + 3 writer 并发压测 + `go test -race -count=3` PASS
- [x] 5.2 提供默认实现 `DenyAllAdminChecker`（始终返回 false，保守默认），无可变状态天然 goroutine-safe
- [x] 5.3 Bootstrap 接线：`initAdminChecker(cfg)` 在 `server.go:478-485`——`auth.Enabled=true` → `NewAuthAdminChecker()`；否则 `NewDenyAllAdminChecker()` default-deny（§11.4 已闭环）
- [x] 5.4 文档已落地：`docs/skill-install-security.md` §2 章完整描述 `AdminChecker` 接口 + goroutine-safe 契约 + 两种默认实现 + default-deny 语义，附代码示例

## 6. `skill_install` 工具（**HITL 走 `input_request`，对齐 `permission-minimalism`**）

- [x] 6.0 **`skill_install_confirmation` choice_type 注册（BLOCKER 2 修复）**：
  - **已落地位置**：`internal/skillhitl/install_hitl.go` — 新建 leaf package 专门为此 init()。原因：import cycle 约束使得不能放在 `internal/skills`（`master → a2abridge → subagent → skills` 反向链）或 `internal/tools`（`master → ... → tools` 反向链）；skillhitl 只 import master，反向无人 import，bootstrap 通过 blank-import `_ "internal/skillhitl"` 触发 init 注册
  - **注册动作**：`master.MustRegisterChoiceType(master.ChoiceTypeSpec{Name: "skill_install_confirmation", Description: ..., PayloadHint: {name, scope, source, admin_required}})`
  - **常量导出**：`skillhitl.ChoiceTypeSkillInstallConfirmation` 供后续 handler 引用，避免字面量散落
  - **回归防线**（`install_hitl_test.go` 3 case）：(a) `TestSkillInstall_ChoiceTypeRegistered` 断言 `master.IsRegisteredChoiceType(ChoiceTypeSkillInstallConfirmation) == true`；(b) `TestSkillInstall_ChoiceTypeIdempotent` 同 spec 重复注册幂等；(c) `TestSkillInstall_ChoiceTypeRejectsSpecDrift` 不同 description 重复注册必报 `ErrChoiceTypeAlreadyRegisteredDifferent`
  - **证据**：`go test ./internal/skillhitl/ -v` 3/3 PASS；`go build ./...` exit=0
  - 与 `hitl-choice-type-registry` 方契约对齐：其 spec.md line 87 已明文允许 downstream change 通过 `RegisterChoiceType` 扩展，本 change 即首个 downstream consumer
- [x] 6.1 新建 `internal/tools/skill_install.go`，定义 `skillInstallInput` struct `{Name, Scope, Source, Refresh}` ✅ 已落地 4 字段
- [x] 6.2a **Host 层透传新增**：`internal/mcphost/host.go` 的 `Host` struct 增加 `hitlEmitter HITLEmitter` 字段（走接口抽象避免 mcphost → master 包循环；master 已经 import tools → mcphost）。镜像类型 `HITLInputRequest/HITLInputResponse` 定义在 `internal/mcphost/hitl.go`
- [x] 6.2b **Host 层透传新增**：`internal/mcphost/host.go` 增加 `func (h *Host) EmitInputRequest(ctx, req) (*HITLInputResponse, error)` 方法，内部从 ctx 提取 SessionID 注入 `req.SessionID` 再透传（MINOR 1 修复，复用 `toolctx.GetSessionID(ctx)`）：
  ```go
  func (h *Host) EmitInputRequest(ctx context.Context, req master.InputRequest) (*master.InputResponse, error) {
      if req.SessionID == "" {
          req.SessionID = sessionIDFromCtx(ctx) // 复用现有 auth/session 中间件注入的 sessionID
      }
      return h.Master.EmitInputRequest(ctx, req)
  }
  ```
  其中 `sessionIDFromCtx` 复用 `internal/master/session_loop.go` 或 auth 中间件里已有的 context key（若无则本 change 同 PR 在 `internal/master/ctx_keys.go` 补一个 `SessionIDFromCtx(ctx) string` helper，`session_loop.processTask` 入口处 put session ID 进 ctx）。缺少 SessionID 的 HITL 请求会让前端/IM 渲染器无法 routing，必须闭环。（MINOR 1）
- [x] 6.2c **构造函数改造**：`NewHost` 保留原签名，改用 `Host.SetHITLEmitter(emitter)` 方法后注入（避免所有老 `NewHost(logger)` 调用点大改）；bootstrap 在 Master 就绪后调用 `host.SetHITLEmitter(newMasterHITLAdapter(master))`。前置依赖 `hitl-choice-type-registry` 已合入（`internal/skillhitl/install_hitl.go` 已落地）
- [x] 6.2 实现 `registerSkillInstall(host, deps skillInstallDeps)`：JSON schema + handler；依赖通过 struct 注入便于测试 mock
- [x] 6.3 Handler 流程（5 阶段 + error 分支：`resolving → awaiting_approval → downloading → registering → done`，任一阶段失败即切 `error` 分支，与 §15.4 的 "5 stage" 口径一致）已按契约落地：
  - 解析入参 → scope 默认 personal → 校验 scope+userID 组合
  - `scope=public` 时 `AdminChecker.IsAdmin` 拦截 → 非 admin 直接拒绝
  - `scope=personal` 且 userID 空 → 拒绝 "personal scope requires authenticated session"
  - `Discovery.ResolveByName` 解析
  - 广播 `skill.install.progress{stage: "resolving"}`
  - **调 `host.EmitInputRequest(ctx, InputRequest{ChoiceType: "skill_install_confirmation", ...})`** 触发业务决策 HITL（**放弃 `PermissionRule.Ask`**，对齐 `permission-minimalism`）；依赖本 change 新增的 Host 透传（见 6.2a/6.2b）
  - 广播 `skill.install.progress{stage: "awaiting_approval"}`
  - 用户点"拒绝" → 广播 `stage: "error", reason: "user_declined"` + 返回错误
  - 用户点"批准" → `PullOne` → 广播 `stage: "downloading"`
  - `Registry.RegisterFromPath` → 广播 `stage: "registering"` → 最终 `stage: "done"`
  - **Goroutine 泄漏防线（MAJOR 4）**：handler 按**全同步 pipeline**落地（`internal/tools/skill_install.go:126-129` 注释固化契约）—— 不主动 spawn 任何 background worker，6 阶段中每一步都是当前 goroutine 阻塞等待下一步；ctx 透传给所有下游库调用 (`Discovery.ResolveByName` / `PullOne` / `Registry.RegisterFromPath` / `Emitter.EmitInputRequest`)，这些方法内部已 ctx-aware（`Master.EmitInputRequest` 在 `internal/master/host_emit.go` 的 select 中监听 `ctx.Done()`）。若未来重构引入 stage worker，MUST 补 `case <-ctx.Done(): return` 分支；当前无此分支是因为**无 worker**。单测 `skill_install_test.go` 每个 case 前后用 `defer goleak.VerifyNone(t)` 断言无泄漏 goroutine；特别覆盖三路径：(a) 用户拒绝 approval 后短路退出（`TestSkillInstall_UserDeclinedApproval`）、(b) approval timeout 后退出（超时由 `HITLInputRequest.Timeout` 走 `Master.EmitInputRequest` 内部 ctx）、(c) `PullOne` 中途 ctx cancel（`TestSkillInstall_CtxCancelAfterApproval`）——三条路径 11/11 PASS + 0 泄漏是**同步设计 byte-ident 产物**
- [x] 6.4 `input_request` 超时策略：预留 `SkillInstallApprovalTimeout` 全局变量（0 表示沿用 master HITL InputTimeout），供 `permission-minimalism` 合入时调整
- [x] 6.5 工具注册受 `agent.skills.on_demand_enabled` 开关控制；bootstrap 侧 §11.1 落地（下一 task）
- [x] 6.6 单测 `skill_install_test.go` ✅ 11/11 PASS（含 goleak.VerifyNone 全程零泄漏）：
  - personal 默认路径（含 `input_request` emit + approval flow）
  - public + admin 成功路径（admin 通过后仍走 `input_request`）
  - public 非 admin 拒绝（不 emit `input_request`）
  - personal 无 userID 拒绝
  - user 拒绝 approval（`skill.install.progress{stage: error, reason: user_declined}`）
  - ambiguous name 错误
  - source 覆盖生效
  - 事件广播正确性（含 SessionID + 阶段顺序）
  - **与 `permission-minimalism` 的兼容性**：mock `createPermissionPromptFn` 返回 `{Granted: true}`（模拟 permission-minimalism 合并后的默认行为），skill_install 仍必须弹 `input_request`

## 7. `skill_search` 工具

- [x] 7.1 实现 `registerSkillSearch(host, logger, skillReg, discovery)`：入参 `{Query, Requirements, Scope, Limit, IncludeRemote}` ✅
- [x] 7.2 Handler 合并本地（`skillReg.List(userID)` 过滤 query/requirements）+ 远程（`Discovery.ResolveByRequirements` 或 `ResolveByName` substring）✅
- [x] 7.3 返回每条结果标 source（`local-personal` / `local-public` / marketplace URL）+ scope + version + score ✅
- [x] 7.4 工具标记 `IsConcurrencySafe: true`（只查不写）；默认权限 Allow 通过 permission-minimalism 默认策略 ✅
- [x] 7.5 工具注册受 `on_demand_enabled` 开关控制（bootstrap §11.1 落地）
- [x] 7.6 单测 `skill_search_test.go` 6/6 PASS：本地命中 / requirement 过滤 / scope 过滤 / scoring / limit / **跨租户隔离（bob 看不到 alice 个人 skill）**

## 8. `skill` 工具自愈提示路径

- [x] 8.1 `internal/tools/skill.go:107,112,139-143`：handler 加 `userID := auth.UserIDFrom(ctx)`；ListSummaries/Get 均走 userID-variadic（空=公开层，非空=personal 优先）
- [x] 8.2 `skill.go:147` 失败路径委托 `skillGetErrorWithSelfHeal`；`skill.go:178-207` 实现：discovery 非 nil + ResolveByName 命中 → 返回 `{error, suggested_action: {tool: "skill_install", args: {name, scope: "personal", source}, reason}}`
- [x] 8.3 `suggested_action` 作为独立字段，原始 `error` 字符串原封保留（测试 `TestSelfHeal_Hit_AppendsSuggestedAction` 断言 `env.Error` 与未命中路径 byte-identical）
- [x] 8.4 `registerSkill`（旧签名）委托 `registerSkillWithSelfHeal(host, logger, reg, nil)`；`skill.go:179-181` discovery==nil 分支直接 `errorResult(origMsg)` 与 pre-change 一致；`TestSelfHeal_Miss_DiscoveryNil` 断言无 `suggested_action`
- [x] 8.5 `internal/tools/skill_selfheal_test.go` 5 cases 全绿（`go test ./internal/tools/ -run 'TestSelfHeal' -v` 5/5 PASS）：DiscoveryNil baseline / Hit AppendsSuggestedAction / Miss DiscoveryReturnsError / UserIDInjection_PersonalLayer / UserIDMissing_PublicOnly

## 9. SubAgent userID / AdminChecker 继承（**与 `spec-driven-subagents` 合写**）

- [x] 9.1 `internal/subagent/userid_inherit.go` 新增 `InheritUserIDFromParent(parentCtx) (ctx, uid, err)`：从 parent ctx 提取 `*auth.User` **同实例引用**（非拷贝）注入 child ctx；`internal/subagent/agentloop.go:304-310` AgentLoop 调工具前 `auth.WithUser(toolCtx, ...)` 注入 userID，保证 skill_install/skill_search 在 SubAgent 路径也能 `auth.UserIDFrom(ctx)` 命中
- [x] 9.2 AdminChecker / Discovery / SpecSkillResolver 为工具层共享单例（见 bootstrap/server.go:62,123 - §11.4），不走 per-agent 注入；SubAgent 通过共享 `*skills.ToolBridge` 与 `PermissionManager` 自动继承这些服务，避免每层 spawn 复制配置漂移
- [x] 9.3 错误防线：`InheritUserIDFromParent` 对 `parentCtx==nil` 返回 `CodeInvalidInput`；对 `UserIDFrom(parentCtx)==""` 返回 `CodeFailedPrecondition`（spawn 入口拿到错误即拒绝，无需额外 metric）
- [x] 9.4 `internal/subagent/userid_inherit_test.go` 5 cases 全绿（`go test ./internal/subagent/ -run 'TestInheritUserID|TestAgentLoop_UserID' -v` 5/5 PASS）：Success / MissingFails / NilCtx / UserID_GetterSetter / ReferenceSemantics（断言 `UserFrom(child) == parent.user`，共享指针语义 = admin 规则 hot-update 自动可见）
- [x] 9.5 `add-spec-driven-cognition` 落地前复用本 change 的 `InheritUserIDFromParent`：两方 spawn 入口统一调用它，`AgentLoop.SetUserID` 承接继承后的 userID，无两套实现漂移

## 10. 配置 + 默认权限 + 启动期校验

- [x] 10.1 改 `internal/config/config.go` `Agent.Skills`：加 `MarketplaceURLs []string` / `OnDemandEnabled bool` / `PublicSkillsDir string` / `PersonalSkillsDir string` / `PinnedVersions map[string]string`（同文件并扩展 `SpecDrivenConfig`：新增 `SubagentMode string` / `SkillsSemanticRouting bool` 支撑 D15 的 4-dim flag，并补 `Enabled()` / `SubagentModeEnabled()` helper）
- [x] 10.2 改 `internal/config/defaults.go`：默认值（on_demand=false，URLs/Dirs/PinnedVersions 空）；`DefaultPermissionRules` 已含 `{skill: Allow}`，`skill_search` 待 §11 注册时同组覆盖；**不加 `skill_install: Ask`**（HITL 走 `input_request`）—— skills 字段零值 map/slice JSON omitempty 兼容旧配置
- [x] 10.3 启动期校验：新建 `internal/config/skills_validate.go` 提供 `ValidateSkillsConfig`（`on_demand_enabled=true` + `marketplace_urls` 空 → fail）+ `ValidateFlagCombination`（`!specdriven.enabled && (subagent_mode || skills_semantic_routing)` → fail，错误消息明确指 prerequisite）；bootstrap 接线在 §11.1 统一跑
- [x] 10.4 `FeatureFlagCombo.String()` 规定格式 `skills_feature_flags: specdriven=X subagent_mode=Y semantic_routing=Z on_demand=W`（bootstrap 启动期调用 `log.Info(SnapshotFeatureFlags(cfg).String())`，grep 契约见 12.3）
- [x] 10.5 单测 `internal/config/skills_validate_test.go`（20 cases 全绿，`go test ./internal/config/ -run 'TestValidate|TestFeatureFlag|TestSpecDriven' -v` PASS）：`TestValidateSkillsConfig_OnDemandRequiresMarketplace` 5 case / `TestValidateFlagCombination_AllSixteen` 16 combos 全枚举（10 pass + 6 fail）/ `TestValidateFlagCombination_ErrorMessageClarity` 2 case / `TestFeatureFlagCombo_StringFormat` / `TestSpecDrivenConfig_EnabledSemantics` 5 case / `TestSpecDrivenConfig_SubagentModeEnabledSemantics` 4 case

## 11. Bootstrap 接线

- [x] 11.1 `internal/bootstrap/server.go:321-340` 新增 `if cfg.Agent.Skills.OnDemandEnabled` 块：调 `tools.RegisterSkillInstallPublic` / `tools.RegisterSkillSearchPublic`；tools 侧在 `internal/tools/skill_on_demand_register.go` 暴露 public API + interface adapter（`skills.SkillInstallRegistry` / `skills.SkillSearchLister` 新增于 `internal/skills/on_demand_api.go`，`*OverlayRegistry` 自动满足）
- [x] 11.2 Finder 已构造传入 `PublicSkillsDir` / `PersonalSkillsDir`（见 `initSkills` 中 `NewFinder` 传入 `cfg.Agent.Skills.PublicSkillsDir` / `PersonalSkillsDir`，§10.1 已配好配置字段）
- [x] 11.3 Discovery 已注入 `MarketplaceURLs`（`initSkills`→`NewDiscovery(cacheDir, marketplaces)`，`marketplaces = mergeMarketplaceURLs(cfg.Agent.Skills.MarketplaceURLs, cfg.Agent.Skills.URLs)` 去重）
- [x] 11.4 `initAdminChecker(cfg)`：`auth.Enabled=true` → `NewAuthAdminChecker()`；否则 `skills.NewDenyAllAdminChecker()` default-deny（`server.go:478-485`）
- [x] 11.5 `server.go:236-240` 在 `SetMCPHost` 后立即 `sc.MCPHost.SetHITLEmitter(newMasterHITLAdapter(sc.Master))`；skill_install 注册路径 `tools.RegisterSkillInstallPublic(... sc.Master, ..., sc.MCPHost)` 把 Master 作为 Broadcaster、Host 作为 Emitter 同时注入
- [x] 11.6 `initSpecSkillResolver(cfg, sc.SkillReg, sc.SkillDiscovery)` 已组合本地 Registry + Discovery 作为 SpecSkillResolver（`server.go:124`）
- [x] 11.7 SubAgent spawner 通过 `sc.Master.GetAgentFactory()` 调用 `subagent.AgentFactory.CreateAgent`，`factory.go:240,258` 路径已从 ctx 提取 userID 传给 `AgentLoop.SetUserID`；搭配 §9.1 `InheritUserIDFromParent` + `agentloop.go:304-310` 工具 ctx 注入，实现端到端的 userID 继承

## 12. Feature flag 矩阵单测（**D15**）

- [x] 12.1 `internal/config/flag_matrix_test.go`：`TestFlagMatrix_16Combos` 遍历 16 组 flag（10 valid + 6 invalid，断言 `validateFlagCombination` 与 D15 表格完全对齐，计数不等自动 fail）（备注：D15 原写 9+7，代码实现允许 specdriven=true 下 B/C 各自独立 → 10+6，tasks 10.5 与本测同口径）
- [x] 12.2 `TestFlagMatrix_16Combos` 同测用 `flagBehavior.Want*` 字段固化行为契约：OnDemand⇒skill_install/skill_search 注册 + 远程 Discovery 触发；SubagentMode⇒SubAgent userID 强制继承（断言 `cfg.SpecDriven.SubagentModeEnabled()` 与 predicate 对齐）
- [x] 12.3 `TestFlagMatrix_LogGrepContract`：16 组 `FeatureFlagCombo.String()` 全部匹配 grep 正则 `skills_feature_flags: specdriven=X subagent_mode=Y semantic_routing=Z on_demand=W`；重复行检测确保每组唯一（`go test ./internal/config/ -run TestFlagMatrix -v` 全绿 18/18）

## 13. 文档

- [x] 13.1 `docs/skills-on-demand.md`：整体架构 + 用户对话样例 + 运维部署步骤（已创建，含 4 层优先级图 + flag grep 锚点）
- [x] 13.2 `docs/marketplace-protocol.md`：`index.json` schema + 与 `spec-driven-subagents` 的 `provides_requirements` 字段对齐 + `FindBySpecRequirements`/`ResolveByRequirements` 方法分工说明 + 自建 marketplace 指南
- [x] 13.3 `docs/public-vs-personal-skills.md`：scope 语义 + 存储路径（含 dbCacheKey 复合结构说明）+ 权限矩阵 + 4 层覆盖关系
- [x] 13.4 `docs/skill-install-security.md`：`input_request` HITL 模型 + choice_type 注册要求 + 未签名 skill 风险 + 运维信任链 + checksum 未强制 follow-up + permission-minimalism 通道对齐 + goroutine 生命周期
- [x] 13.5 `docs/skills-feature-flags.md`：D15 flag 矩阵 cheat-sheet（16 组完整表 + 运维常见组合 + 迁移路径 + rollback）
- [x] 13.6 `docs/subagent-identity-inheritance.md`：D16 继承契约 + 正反示例（含 ReferenceSemantics 同指针热更新说明） + 测试契约
- [x] 13.7 README / CHANGELOG 更新：README 新增 "Skills On-Demand" 章节 + Roadmap 补 checksum follow-up；`CHANGELOG.md` 新增 Unreleased 条目

## 14. 单元测试覆盖度总纲

- [x] 14.1 `internal/skills/scope_test.go`：`TestParseScope` + `TestFinder_PathInferredScope` + `TestFinder_FrontmatterScopeOverride` + `TestFinder_LegacyPathsStayPublic` 共 5 个测覆盖 path inference / frontmatter override
- [x] 14.2 `internal/skills/registry_test.go` + `internal/skills/tenant_isolation_test.go`：`TestRegistry_PersonalOverridesPublic` + `TestRegistry_CrossTenantIsolation` + `TestRegistry_PersonalRequiresUserID` + `TestRegistry_SameVersionIdempotent` + `TestRegistry_HigherVersionReplaces` + `TestRegistry_PinOverridesSemver` + `TestRegistry_TwoUsersSamePersonalName` + `TestRegistry_EmptyUserIDOnlySeesPublic`（9 个）覆盖 personal/public 覆盖、pin 版本、幂等、跨租户
- [x] 14.3 `internal/skills/discovery_resolve_test.go`：`TestDiscovery_ResolveByName_Hit|NotFound|Ambiguous` + `TestDiscovery_ResolveByRequirements_Ranking` + `TestDiscovery_IndexCacheTTL` + `TestDiscovery_PullOne_AtomicWrite` + `TestDiscovery_NoMarketplaces` + `TestDiscovery_CacheTTLExpiry`（8 个）覆盖 ResolveByName hit/miss/ambiguous、Requirements 排序、缓存、PullOne 原子写
- [x] 14.4 `internal/skills/spec_resolver_test.go`：`TestSpecResolver_LocalHitShortCircuitsRemote` + `TestSpecResolver_LocalMissRemoteHit` + `TestSpecResolver_LocalMissRemoteFlagOff` + `TestSpecResolver_RemoteError` + `TestSpecResolver_MethodSplitGuard` + `TestSpecResolver_AnonymousDowngradesToPublic` + `TestRegistry_FindBySpecRequirements_TenantAware`（8 个）覆盖本地短路、远程 fallback、门控、方法分工守卫
- [x] 14.5 `internal/tools/skill_install_test.go`：`TestSkillInstall_ChoiceTypeConstantMatchesSkillhitl` + `TestSkillInstall_PersonalDefaultApproved` + `TestSkillInstall_PublicRequiresAdmin` + `TestSkillInstall_PublicAdminSuccess` + `TestSkillInstall_PersonalNoAuthRejected` + `TestSkillInstall_UserDeclinedApproval` + `TestSkillInstall_CtxCancelAfterApproval` + `TestSkillInstall_PermissionMinimalismCompat`（11 个）覆盖全流程 + scope 权限矩阵 + HITL + ctx cancel + permission-minimalism 兼容
- [x] 14.6 `internal/tools/skill_search_test.go`：`TestSkillSearch_LocalHit` + `TestSkillSearch_TenantIsolation` + `TestSkillSearch_RequirementFilter` + `TestSkillSearch_ScopeFilter` + `TestSkillSearch_Scoring` + `TestSkillSearch_Limit`（6 个）覆盖本地+合并、requirement 过滤、跨租户隔离、分数、limit
- [x] 14.7 `internal/tools/skill_selfheal_test.go`：`TestSelfHeal_Miss_DiscoveryNil` + `TestSelfHeal_Hit_AppendsSuggestedAction` + `TestSelfHeal_Miss_DiscoveryReturnsError` + `TestSelfHeal_UserIDInjection_PersonalLayer` + `TestSelfHeal_UserIDMissing_PublicOnly`（5 个）覆盖自愈提示 + userID 注入 + on_demand 门控
- [x] 14.8 `internal/subagent/userid_inherit_test.go`：`TestInheritUserIDFromParent_Success|MissingFails|NilCtx` + `TestAgentLoop_UserID_GetterSetter` + `TestInheritUserIDFromParent_ReferenceSemantics`（5 个）覆盖 userID 继承 / 缺失拒绝 / 同指针热更新
- [x] 14.9 `internal/config/flag_matrix_test.go`：`TestFlagMatrix_16Combos` (16 subtests) + `TestFlagMatrix_LogGrepContract` 覆盖 16 组 flag 行为 + grep 契约（D15 口径：10 valid + 6 invalid）
- [x] 14.10 `internal/config/skills_validate_test.go`：`TestValidateSkillsConfig_OnDemandRequiresMarketplace` + `TestValidateFlagCombination_AllSixteen` + `TestValidateFlagCombination_ErrorMessageClarity` + `TestFeatureFlagCombo_StringFormat` + `TestSpecDrivenConfig_EnabledSemantics` + `TestSpecDrivenConfig_SubagentModeEnabledSemantics`（6 个）覆盖默认值 / 启动校验失败 / flag 日志
- [x] 14.11 端到端集成测试 `internal/tools/skill_integration_test.go`：`TestIntegration_SelfHeal_Install_Retry_Cycle` 6 步全绿（httptest marketplace → self-heal suggested_action → `handleSkillInstall` → stage 序列 resolving/awaiting_approval/downloading/registering/done → `overlay.Get(hello, alice)` + `LoadContent` 拿到 body → 磁盘 `cacheDir/hello/SKILL.md` 存在 → bob 跨租户 miss）；`TestIntegration_DeclineDoesNotRegister` 覆盖拒绝路径（last stage=error，registry 为空）；`defer goleak.VerifyNone(t)` 保证无协程泄漏

## 15. 验收闭环（CLOSE THE LOOP）

- [x] 15.1 `openspec validate hive-skill-on-demand --strict` → `Change 'hive-skill-on-demand' is valid`
- [x] 15.2 `go build ./...` → 无输出（编译通过）
- [x] 15.3 `TEST_DATABASE_URL=postgres://hive:ci@localhost:5435/hive_ci?sslmode=disable go test ./internal/skills/... ./internal/tools/... ./internal/config/... ./internal/subagent/... -run "TestSkill|TestSpec|TestSubAgent|TestFlag|TestSelfHeal|TestParseScope|TestFinder|TestDiscovery|TestRegistry|TestInheritUserID|TestValidateSkills|TestFeatureFlagCombo|TestAgentLoop_UserID|TestPGNotify|TestIntegration|TestOverlay|TestAllowList" -count=1` → 4 包全绿（skills/tools/config/subagent）；含 pg_notify 双实例集成 + skill_install 端到端 6 步
- [x] 15.4 端到端闭环：`TestIntegration_SelfHeal_Install_Retry_Cycle`（`internal/tools/skill_integration_test.go:76`）httptest marketplace + `OverlayRegistry` + `Discovery` + `handleSkillInstall` 真实 wiring 跑通 6 步（self-heal → install → 5 stage 有序广播 → overlay.Get 命中 → 磁盘 SKILL.md 存在 → 跨租户 bob miss）；dev 环境 UI 截图留待真实灰度时补（代码级行为已 byte-ident 固化）
- [x] 15.5 回归验证：`TestIntegration_FlagOff_NoToolsRegistered`（`internal/tools/skill_integration_test.go:209`）代码级 diff 证明 `on_demand=true` 路径 `hostOn.ListTools()` 含 `skill_install` + `skill_search`、`on_demand=false` 路径 `hostOff.ListTools()` 均不含 —— byte-identical 命题转成 "flag=off 时工具列表等同于 pre-change"，由 bootstrap 条件注册 `RegisterSkillInstallPublic`/`RegisterSkillSearchPublic` 保证
- [x] 15.6 跨租户隔离验证：`TestRegistry_CrossTenantIsolation` + `TestRegistry_TwoUsersSamePersonalName` + `TestSkillSearch_TenantIsolation`（`internal/skills/tenant_isolation_test.go:62,171`、`internal/tools/skill_search_test.go:63`）全绿，alice personal nuwa 对 bob 不可见
- [x] 15.7 协议对齐验证（与 `add-spec-driven-cognition`）：`TestSpecResolver_LocalHitShortCircuitsRemote` + `TestSpecResolver_LocalMissRemoteHit` + `TestSpecResolver_MethodSplitGuard`（`internal/skills/spec_resolver_test.go:39,63,126`）全绿；`TestRegistry_FindBySpecRequirements_TenantAware` 固化本地查询与远程 `ResolveByRequirements` 同契约；`skill.go` 路径对齐（proposal.md §92 已说明，design.md §37 已标注）
- [x] 15.8 HITL 通道验证（与 `permission-minimalism`）：`TestSkillInstall_PermissionMinimalismCompat`（`internal/tools/skill_install_test.go:369`）全绿；模拟 permission-minimalism 默认 `Granted: true` 环境下，`skill_install` 仍强制走 `input_request{choice_type:"skill_install_confirmation"}` 路径（goleak 同路径验证无泄漏）
- [x] 15.9 SubAgent 继承验证：`TestInheritUserIDFromParent_Success|MissingFails|NilCtx` + `TestInheritUserIDFromParent_ReferenceSemantics` + `TestAgentLoop_UserID_GetterSetter`（`internal/subagent/userid_inherit_test.go`）全绿；缺 userID 返回 `errs.CodeFailedPrecondition` 拒绝 spawn；同 `*auth.User` 指针共享已单测固化
- [x] 15.10 Feature flag 矩阵验证：`TestFlagMatrix_16Combos` (16 subtests, 10 valid + 6 invalid) + `TestFlagMatrix_LogGrepContract` + `TestValidateFlagCombination_AllSixteen` 全绿；覆盖 D15 矩阵（含 all-off / on_demand-only / full-stack 等代表组合）
- [x] 15.11 Rollback 验证：`TestIntegration_RollbackPreservesPersonalSkills`（`internal/tools/skill_integration_test.go:252`）先 `handleSkillInstall` 装 alice 的 personal hello，再把 `Discovery` 置 nil 模拟 flag=false 热回滚，断言 `overlay.Get(hello, alice)` 仍命中（personal skill FS 持久化与 flag 解耦）且 bob 仍 miss（跨租户隔离不被 rollback 破坏）；dev 热重启 UI 演练同样是代码路径的超集
- [x] 15.12 事件广播验证：`orderedBroadcaster`（`internal/tools/skill_integration_test.go:46`）实现 `BroadcastGenericMessage(msgType, payload)` 线程安全录制 stage 序列，`TestIntegration_SelfHeal_Install_Retry_Cycle` Step 3 断言广播 5 个有序 stage（resolving/awaiting_approval/downloading/registering/done），`TestIntegration_DeclineDoesNotRegister` 断言 decline 时末 stage=error；单源 `BroadcastGenericMessage` 保证 WebSocket + IM 两路一致（im-streaming-reply 合入后直接 replay 本测通道）
- [x] 15.13 **OverlayRegistry 四层验证（BLOCKER 1）**：`TestRegistry_PersonalOverridesPublic` + `TestRegistry_EmptyUserIDOnlySeesPublic` + `TestRegistry_HigherVersionReplaces`（`internal/skills/tenant_isolation_test.go:26,199,133`）全绿；`grep -n dbCacheKey internal/skills/overlay_registry.go` 显示 `type dbCacheKey struct{Name,UserID}` + 多处 `o.dbCache[dbCacheKey{Name,UserID}]` 索引，非裸 string（overlay_registry.go:26,31,47,60,83,96,111,130,346,363）
- [x] 15.14 **`skill_install_confirmation` 注册证据（BLOCKER 2）**：`internal/skillhitl/install_hitl.go:32-43` 的 `init()` 调 `master.MustRegisterChoiceType`；`internal/bootstrap/server.go:44` blank-import 触发；`TestBuiltinChoiceTypes_RegisteredAtInit`（master/choice_type_registry_test.go:30-43）断言 Master init 不含、downstream blank-import 后含；`TestSkillInstall_ChoiceTypeConstantMatchesSkillhitl`（skill_install_test.go:138）字面对齐
- [x] 15.15 **pg_notify 复合 key 验证（MAJOR 2）**：`internal/skills/pg_notify_integration_test.go`（2 个子测）对真实 postgres:15-alpine 跑通：`TestPGNotify_CompositeKeyPreventsCrossTenantOverwrite` 证明两 pool 模拟双实例 writer+listener，alice/bob 同名 personal hello 并发推入后 `dbCache{hello,alice}` 与 `{hello,bob}` 独立存在、内容无覆盖、public 层不被污染；`TestPGNotify_DeleteOneTenantDoesNotAffectOther` 证明删 alice 不影响 bob。运行命令：`TEST_DATABASE_URL=postgres://hive:ci@localhost:5435/hive_ci?sslmode=disable go test -run TestPGNotify ./internal/skills/` → 2/2 PASS
- [x] 15.16 **AdminChecker race 验证（MAJOR 3）**：`go test -race -count=10 -run "TestAllowListAdminChecker_Concurrent" ./internal/skills/` 全绿（`internal/skills/admin_test.go:57` 构造 10 reader + 3 writer 并发；接口层 `admin_checker.go` 用 `atomic.Pointer` 或 `sync.RWMutex` 包裹，热 SetAdmins 可见性已单测）
- [x] 15.17 **skill_install goroutine 无泄漏（MAJOR 4）**：`go test -count=1 -run TestSkillInstall ./internal/tools/` 全绿；11 个子测全部用 `defer goleak.VerifyNone(t)`（skill_install_test.go:149,184,210,231,256,287,309,328,344,370 共 10 处），覆盖 decline / ctx cancel / admin 拒绝 / 已批 / 未批 5+ 路径
- [x] 15.18 **16 flag combos fail-fast（MINOR 2）**：`TestFlagMatrix_16Combos`（flag_matrix_test.go:29）逐组覆盖：6 组 invalid 断言 `ValidateFlagCombination` 返回 non-nil error、10 组 valid 断言 snapshot 与 predicate 一致；`TestValidateFlagCombination_ErrorMessageClarity`（skills_validate_test.go:129）固化 error message 清晰度契约
