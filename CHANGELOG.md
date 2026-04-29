# Changelog

## Unreleased — Skills On-Demand (hive-skill-on-demand)

### Added

- **按需 skill 安装**：新增工具 `skill_install` / `skill_search`，支持运行期从 marketplace 拉取并注册 skill
- **Public / Personal scope**：skill 按租户隔离，personal 存 `$HIVE_DATA/skills/users/<uid>/`，public 存 `$HIVE_DATA/skills/public/`
- **四层 Overlay 优先级**：`OverlayRegistry.Get` 按 `personal DB > personal FS > public DB > public FS` 返回第一命中；dbCache key 改为复合类型 `{name, userID}`，消除跨租户覆盖
- **Business-decision HITL 通道**：`skill_install` 通过 `input_request{choice_type:"skill_install_confirmation"}` 走 HITL；`choice_type_registry.go` 显式 `MustRegisterChoiceType`
- **SubAgent 身份继承契约**：新增 `subagent.InheritUserIDFromParent`，SubAgent 必须从父 ctx 继承 userID（缺失即拒绝 spawn）；工具调用路径 `agentloop.go:304-310` 额外做兜底注入；parent `*auth.User` 指针共享以支持热更新
- **Self-heal 提示**：`skill.go` 未命中时返回 `suggested_action{tool:"skill_install", args, reason}`，LLM 可自主发起安装
- **SpecSkillResolver**：聚合 `Registry.FindBySpecRequirements`（本地）→ `Discovery.ResolveByRequirements`（远程）二级 fallback
- **Feature flag 4 维矩阵**：16 组合 (10 valid + 6 invalid)，`ValidateFlagCombination` bootstrap fail-fast；`FeatureFlagCombo.String()` 提供 grep 锚点 `skills_feature_flags: specdriven=X subagent_mode=Y semantic_routing=Z on_demand=W`
- **AdminChecker 并发安全契约**：接口声明 goroutine-safe；`auth.Enabled=false` 时使用 `NewDenyAllAdminChecker` default-deny
- **goroutine 泄漏防护**：所有 `skill_install` stage worker 监听 `ctx.Done()`；单测用 `goleak.VerifyNone` 覆盖 decline / timeout / mid-download cancel 三路径

### Changed

- **bootstrap 接线**：`SetMCPHost` 后立即 `SetHITLEmitter(newMasterHITLAdapter(Master))`；条件注册 `skill_install` / `skill_search`（`cfg.Agent.Skills.OnDemandEnabled`）
- **DB schema 迁移**：`skills` 表增加 `user_id VARCHAR NOT NULL DEFAULT ''` 列 + 复合主键 `(name, user_id, version)`；pg_notify payload 扩展 `user_id` 字段（旧 payload 解析为 public skill，向后兼容）
- **SubAgent factory**：`factory.go:240,258` 从 ctx 提取 userID 传给 `AgentLoop.SetUserID`

### Docs

- `docs/架构设计/skills/Skill-按需加载总览.md` — 总览
- `docs/架构设计/Skill-市场协议.md` — index.json schema + 自建指南
- `docs/架构设计/skills/Skill-Scope与覆盖关系.md` — scope / 路径 / 权限 / 覆盖矩阵
- `docs/架构设计/skills/Skill-安装安全模型.md` — HITL + AdminChecker + checksum roadmap
- `docs/架构设计/skills/Skill-Feature-Flag矩阵.md` — D15 矩阵 cheat-sheet
- `docs/subagent-identity-inheritance.md` — D16 继承契约

### Compatibility

- 灰度默认 `on_demand_enabled=false`，零破坏向后兼容
- Rollback 热路径：config 切换 + 滚动重启，已装 personal skill 保留在 `$HIVE_DATA/skills/users/`
- Flag matrix #1（全关）与 pre-change 行为 byte-identical

### Known Limitations

- skill `checksum` 字段当前是 optional，未强制校验（follow-up change: `skill-install-checksum-enforcement`）
- 未提供 `skill_uninstall` 工具；运维侧直接删目录 / DB 行 + pg_notify 触发 cache invalidate
