## Why

两个已提交但尚未实施的 OpenSpec change（`add-spec-driven-cognition/permission-minimalism` 和 `hive-skill-on-demand`）都假设 `InputRequest` 结构体存在 **`ChoiceType string`** 字段并由一个业务决策白名单管控——但 `internal/master/hitl.go:20-34` 的真实 struct 仅有 `Type InputRequestType` 枚举 5 值（`approval / clarification / confirmation / choice / permission`），**`ChoiceType` 字段和对应白名单根本不存在**。这是 P0 硬冲突，任一下游 change 实施时都会编译失败或语义漂移。同时两个下游 change 对该白名单的取值定义**互不知情**：`permission-minimalism` 声明闭集 3 值，`hive-skill-on-demand` 引入的 `skill_install_confirmation` 不在闭集内，按前者措辞会被判为非保留通道。本 change 作为**前置基建** PR，一次性在 Master 协议层拉齐字段与注册表，解锁两条下游 change 的并行实施。

## What Changes

- **新增字段** `ChoiceType string \`json:"choice_type,omitempty"\`` 到 `internal/master/hitl.go` 的 `InputRequest` struct（**与 `Type InputRequestType` 正交**：`Type` 描述交互协议形态，`ChoiceType` 描述业务决策子语义）
- **新增白名单注册表** `internal/master/choice_type_registry.go`：提供 `RegisterChoiceType(name, spec)` / `IsRegistered(name) bool` / `List() []string` API，取代 `permission-minimalism` 硬编码的 `{account_selector, ambiguity_clarification, confirmation_before_irreversible_business_action}` 闭集
- **内置注册** 3 个原 `permission-minimalism` 白名单值 + 预留 `skill_install_confirmation` 注册点（由 `hive-skill-on-demand` 实施阶段调用 `RegisterChoiceType` 挂入）
- **扩展广播协议**：`BroadcastInputRequest` 序列化时自动携带 `choice_type` 字段（若非空）；前端渲染端按 `choice_type` 分发审批 UI
- **新增 HITL 审批辅助** `Host.EmitInputRequest(ctx, InputRequest)`：封装「构造 InputRequest → broadcast → 等待 InputResponse → 返回」的闭环，供 Skill/Tool 作者直接调用而不必关心底层广播/订阅
- **文档补丁**（hint-only，不算 break）：在 `add-spec-driven-cognition/permission-minimalism/spec.md:78` 的白名单措辞改为「**registered via `choice_type_registry`**」而非硬编码闭集——作为**跨 change 协调任务**记录，由下游 change 并行改动

## Capabilities

### New Capabilities

- `hitl-choice-type-registry`: Master HITL 协议层的 `choice_type` 字段 + 白名单注册表 + `EmitInputRequest` 闭环辅助，作为所有业务决策型 `input_request` 事件的统一基建

### Modified Capabilities

- （无。本 change 为**新增基建**，不改变任何已有 capability 的既定要求。`permission-minimalism` 的白名单措辞由其自身 change 或未合入前的补丁负责修正——本 change 仅提供注册表机制。）

## Impact

- **代码改动**（仅 Go 后端，零前端必需改动）：
  - **新增** `internal/master/choice_type_registry.go`（~80 行，含线程安全注册表 + 内置初始化）
  - **修改** `internal/master/hitl.go`：`InputRequest` struct 追加 `ChoiceType string` 字段（~1 行），不动已有 5 个 `Type` 枚举值
  - **修改** `internal/master/broadcast_api.go`：`BroadcastInputRequest` 序列化路径透传新字段（~0 行实质改动——`encoding/json` 自动处理）
  - **新增** `internal/master/host_emit.go`：`Host.EmitInputRequest` 闭环辅助（~60 行，含超时/取消处理）
  - **新增** `internal/master/choice_type_registry_test.go` + `host_emit_test.go`（~120 行单测）
- **协议兼容**：新字段 `omitempty`，不序列化空值 → 对所有**未使用** `ChoiceType` 的既有 `InputRequest` 消费者（Feishu/WeChat/Web 前端）**零破坏**
- **依赖关系**：
  - **Blocks（解锁）**：`add-spec-driven-cognition/permission-minimalism`（其白名单实现依赖注册表）、`hive-skill-on-demand`（其 D7 HITL 通道依赖字段 + `skill_install_confirmation` 注册）
  - **Not blocked by**：本 change 为根节点，不依赖任何未合入 change
- **测试**：新增 20+ 单测覆盖 register/dedupe/list/emit/timeout/cancel；`make test ./internal/master/...` 全量通过为验收硬标准
- **文档**：`openspec/project.md` 新增「HITL 协议层（choice_type 注册表）」段落；前端/IM 文档增量（如果存在）不在本 change 范围
- **风险**：极低。新增字段 + 新增文件为主，现有路径只触 `BroadcastInputRequest` 一处序列化点；加单测保证老通道（`Type`=5 值、无 `ChoiceType`）行为不变
