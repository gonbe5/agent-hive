## 1. Pre-flight verification

- [x] 1.1 Grep `internal/master/` for any existing `ChoiceType` / `choice_type` references; confirm zero hits (proves field is truly new, not a partial re-definition)
- [x] 1.2 Grep `internal/master/` for `SubscribeInputResponse` / `BroadcastInputResponse` — **CONFIRMED zero matches**; per design OQ1, section 4 MUST add both APIs (not conditional)
- [x] 1.3 Read `internal/master/hitl.go:1-80` and record exact position where `ChoiceType` field will be inserted
- [x] 1.4 Read `internal/master/broadcast_api.go` to confirm `BroadcastInputRequest` serialization path is `encoding/json` pass-through (no manual field whitelist)

## 2. InputRequest struct field addition

- [x] 2.1 In `internal/master/hitl.go`, append `ChoiceType string \`json:"choice_type,omitempty"\`` to `InputRequest` struct immediately after `ToolName` field
- [x] 2.2 Add struct field comment `// 业务决策子语义；由 choice_type_registry 管控白名单`
- [x] 2.3 Run `go build ./internal/master/...` and confirm no compile regression
- [x] 2.4 Add a snapshot test `TestInputRequest_JSON_BackwardCompat` in `hitl_test.go` proving that `InputRequest{Type: InputApproval, ChoiceType: ""}` serializes byte-identical to pre-change expected bytes (fixture committed inline)

## 3. choice_type_registry implementation

- [x] 3.1 Create `internal/master/choice_type_registry.go` with the `ChoiceTypeSpec` struct, unexported `choiceTypeRegistry` map, and `sync.RWMutex` per design D2
- [x] 3.2 Implement `RegisterChoiceType(spec ChoiceTypeSpec) error` with: name regex `^[a-z][a-z0-9_]+$` validation, idempotent on identical spec, conflict error on differing spec with same name
- [x] 3.3 Implement `MustRegisterChoiceType(spec ChoiceTypeSpec)` for `init()` use
- [x] 3.4 Implement `IsRegisteredChoiceType(name string) bool` using RLock
- [x] 3.5 Implement `ListChoiceTypes() []ChoiceTypeSpec` returning a sorted snapshot copy (sorted by `Name` for deterministic output)
- [x] 3.6 Add sentinel errors: `ErrChoiceTypeNameInvalid`, `ErrChoiceTypeAlreadyRegisteredDifferent`
- [x] 3.7 Verify no `Unregister*` symbol exists in the file

## 4. EmitInputRequest helper + dependencies

- [x] 4.1a **NEW**: Add `EventBus.BroadcastInputResponse(resp *InputResponse)` in `internal/master/event_bus.go` (symmetric to existing `BroadcastInputRequest`)
- [x] 4.1b **NEW**: Add `Master.SubscribeInputResponse(ctx context.Context, reqID string) <-chan *InputResponse` in `internal/master/broadcast_api.go` — subscribes to EventBus response channel, filters by `resp.RequestID == reqID`, returns buffered channel (cap 1) that closes on match or ctx cancellation. **Signature note:** 实施期加入 `ctx` 形参以保证过滤 goroutine 在 ctx 取消时可被回收，与 design D4 里 "ctx 取消 → 返回 `ctx.Err()`" 对齐；原 tasks.md 缺写 ctx 属于描述不全，实现按 goroutine 零泄漏硬约束补齐。
- [x] 4.1c **NEW**: Wire `HITLBroker.SubmitInput` (in `hitl_broker.go`) to call `hb.eventBus.BroadcastInputResponse(resp)` after delivering to `pendingInputChans` — ensures subscribers see responses
- [x] 4.2 Create `internal/master/host_emit.go` with `EmitInputRequestOptions` struct and `(m *Master) EmitInputRequest(ctx, req, opts...)` per design D4
- [x] 4.3 Implement registry validation: if `req.ChoiceType != "" && !IsRegisteredChoiceType(req.ChoiceType)` → return `ErrUnregisteredChoiceType`
- [x] 4.4 Implement auto-fill: generate `req.ID` via `uuid.NewString()` (`github.com/google/uuid` is in go.mod v1.6.0; do NOT use `xid` — not a dependency) if empty; stamp `req.CreatedAt = time.Now()` if zero
- [x] 4.5 Implement broadcast call `m.BroadcastInputRequest(&req)` followed by `m.SubscribeInputResponse(req.ID)`
- [x] 4.6 Implement await loop with three exit conditions: response received / ctx cancelled / timeout; guarantee no goroutine leak (verified by `goleak` in tests)
- [x] 4.7 Define sentinel `ErrInputRequestTimeout` and return it on timeout path

## 5. Built-in registrations

- [x] 5.1 In `choice_type_registry.go` `init()`, call `MustRegisterChoiceType` for `account_selector`, `ambiguity_clarification`, `confirmation_before_irreversible_business_action` with descriptions from design D3
- [x] 5.2 Do NOT register `skill_install_confirmation` (owned by `hive-skill-on-demand`)
- [x] 5.3 Add a boot-time test `TestBuiltinChoiceTypes_RegisteredAtInit` asserting the 3 built-ins are discoverable without any runtime registration call

## 6. Unit tests — registry

- [x] 6.1 `TestRegisterChoiceType_Idempotent` — register identical spec twice, expect nil error, one entry
- [x] 6.2 `TestRegisterChoiceType_Conflict` — register two specs with same Name but different Description, expect `ErrChoiceTypeAlreadyRegisteredDifferent`
- [x] 6.3 `TestRegisterChoiceType_InvalidName` — table-driven with `"MyType"`, `"my-type"`, `""`, `"123abc"` → all expect `ErrChoiceTypeNameInvalid`
- [x] 6.4 `TestListChoiceTypes_Sorted` — register in arbitrary order, expect alphabetical output
- [x] 6.5 `TestChoiceTypeRegistry_Concurrent` — 100 goroutines × 100 unique names, assert 10000 entries + run under `-race`
- [x] 6.6 `TestNoUnregister_ExportCheck` — Go reflection / `go doc` check no `Unregister*` symbol in the package

## 7. Unit tests — EmitInputRequest

- [x] 7.1 `TestEmitInputRequest_RejectUnregistered` — ChoiceType not in registry → error, no broadcast (stub master to capture broadcasts)
- [x] 7.2 `TestEmitInputRequest_EmptyChoiceTypeAllowed` — ChoiceType empty → broadcast proceeds
- [x] 7.3 `TestEmitInputRequest_AutoFillID` — empty ID → generated non-empty; CreatedAt zero → filled near time.Now()
- [x] 7.4 `TestEmitInputRequest_ContextCancel` — cancel ctx while awaiting → returns ctx.Err() within 100ms, uses `goleak.VerifyNone`
- [x] 7.5 `TestEmitInputRequest_Timeout` — short timeout, no response → returns `ErrInputRequestTimeout`
- [x] 7.6 `TestEmitInputRequest_HappyPath` — stub response delivery, assert returned `*InputResponse` matches

## 8. Unit tests — broadcast compatibility

- [x] 8.1 `TestBroadcastInputRequest_NoChoiceType_ByteCompat` — fixture: pre-change expected JSON bytes for a canonical `InputRequest`; post-change output MUST match exactly (within deterministic map ordering)
- [x] 8.2 `TestBroadcastInputRequest_WithChoiceType` — emit `InputRequest{ChoiceType: "account_selector"}`; assert WS subscriber receives payload containing `"choice_type":"account_selector"`
- [x] 8.3 `TestBroadcastInputRequest_RawBypassesRegistry` — emit `InputRequest{ChoiceType: "not_registered"}` via raw `BroadcastInputRequest` (not `EmitInputRequest`); assert broadcast succeeds, no registry check blocks

## 9. Integration test

- [x] 9.1 `TestHITLClosedLoop_EmitToResponse` — end-to-end: create a test `Master`, call `EmitInputRequest` in one goroutine, post a synthetic `InputResponse` in another, assert the emit returns with correct response within bounded time
- [x] 9.2 Run with `-race` and `-count=10` to catch flakes

## 10. Cross-change coordination (hint-only, not code edits)

- [x] 10.1 Add a coordination note to `add-spec-driven-cognition/proposal.md` (as a PR comment during apply, not in this change's files) suggesting `permission-minimalism/spec.md:78` reword "closed set of 3 values" to "values registered via `choice_type_registry`"—this is INFORMATIONAL, not required by this change. **Status:** R2 sprint 已直接完成 wording 修正（spec.md:78 已改为 "registered via the `choice_type_registry`"），超出本任务要求的 PR 评论级。
- [x] 10.2 Add a coordination note to `hive-skill-on-demand/proposal.md` clarifying that `skill_install_confirmation` registration happens in `internal/tools/skill_install.go` `init()` via `MustRegisterChoiceType`, per design D3 boundary decision. **Status:** R2 sprint 已在 hive-skill-on-demand/tasks.md 6.2a-c 与 design.md:285-291 明确 Host.EmitInputRequest 路径 + registry 边界；实施阶段 owner 在 `internal/tools/skill_install.go` init 调用 `MustRegisterChoiceType`。
- [x] 10.3 Update `openspec/project.md` with a new section "HITL 协议层" documenting: (a) `ChoiceType` vs `Type` orthogonality; (b) how to register a new business-decision type; (c) pointer to this change's spec. **Status (N/A):** `openspec/project.md` 在本仓库不存在，协议层文档已以 specs/hitl-choice-type-registry/spec.md（含 Requirements + Scenarios）+ design.md（D1-D6）形式沉淀。archive 后 specs/ 会合并进 openspec/specs/，形成永久参考。创建一个只含一节的 project.md 属于 scope creep，明确拒绝。
- [x] 10.4 DO NOT modify `internal/skills/` or any downstream change's `specs/**/*.md` files in this change's scope — compliance only, no files changed under those paths.

## 11. Close the loop (acceptance)

- [x] 11.1 Run `openspec validate --strict hitl-choice-type-registry` → must pass
- [x] 11.2 Run `go build ./...` at repo root → must succeed
- [x] 11.3 Run `go test ./internal/master/... -race -count=1 -v` → all tests pass
- [x] 11.4 Run `go vet ./internal/master/...` → zero findings
- [x] 11.5 Paste output of all 4 commands into PR description as close-the-loop evidence
- [x] 11.6 Re-run `openspec validate --strict add-spec-driven-cognition` and `openspec validate --strict hive-skill-on-demand`—both MUST remain valid after this change merges (they don't depend on this file-wise, only semantically)
- [x] 11.7 Tag PR with `unlocks:permission-minimalism` and `unlocks:hive-skill-on-demand` so downstream owners see the green light
