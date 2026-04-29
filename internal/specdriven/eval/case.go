// Package eval 是 spec-driven 认知层的离线回归 harness。
// 每条 Case 描述一个 user ingress 请求的输入 + 预期决策 + 结构化的失败模式注入点，
// 由 Runner 接口驱动被测实现回放。所有 fixture 以 JSON 存于 testdata/。
//
// 设计意图：harness 是 dual-flag 上线的硬闸门，fm01~fm08 覆盖 CEO review
// 列出的 8 个失败模式；新增 regression_*.json 仅在出事故后追加。
//
// harness 分两层：
//
//   - schema gate：总是运行，验证 fixture 格式正确、required-set 完整、解码严格。
//     由 TestEvalFixtures 执行，不依赖具体实现。
//   - behavior gate：需要 Runner 注入（TG2+ 阶段）。由 TestHarnessBehavior 执行，
//     未注入 Runner 时 Harness.RunFixtures 会 t.Fatal（fail-closed）。
//
// # Regression fixture growth policy
//
// 完整流程见 design.md 的 "Runbook: Regression fixture growth policy" 章节。
// 摘要：每起归因于 spec-driven 代码路径的线上事故，关单 PR 必须添加
// testdata/regression_<incident-id>.json 且 "required": true，并能在 pre-fix
// commit 上复现原失败。
package eval

import "github.com/chef-guo/agents-hive/internal/specdriven"

// Case 是 harness 的单个测试用例。fixture JSON 直接反序列化到本结构。
// Required=true 的 case 失败会使 make test-specdriven 返回非零退出码，
// 从而阻断 spec_driven.mode 从 legacy 升到 dual 的 CI 晋升。
//
// 新增字段（Codex P1 review 修复）让 FM-2/3/6 这类"结构化失败模式"能被 harness
// 直接表达，而不是只写在 notes 里。
type Case struct {
	Name             string                      `json:"name"`
	UserID           string                      `json:"user_id"`
	SessionState     specdriven.SessionSpecState `json:"session_state"`
	Input            string                      `json:"input"`
	WantContinuation WantContinuation            `json:"want_continuation"`
	WantPlan         *specdriven.Plan            `json:"want_plan,omitempty"`
	WantFallback     bool                        `json:"want_fallback,omitempty"`
	// WantError 若非空，Runner 操作应返回 error 且其 Error() 含本子串。
	// 适用 FM-2 CAS 冲突、FM-5 非法 spec_ref 等期望错误的场景。
	WantError string `json:"want_error,omitempty"`
	// StoreState 模拟 FM-3 (store canonical vs FS export) 的 revision 状态。
	// Runner 实现应按本快照注入 store/FS 的 mock。
	StoreState *StoreState `json:"store_state,omitempty"`
	// ConcurrentWrite 触发 FM-2 CAS 冲突注入：
	// Runner 在 UpsertWithCAS 调用前，必须先模拟另一 actor 把 revision 推进 1。
	ConcurrentWrite bool   `json:"concurrent_write,omitempty"`
	Required        bool   `json:"required"`
	Notes           string `json:"notes,omitempty"`
}

// WantContinuation 描述对 continuation resolver 的期望。
// Decision 必须是 "resume" | "ask" | "new" 之一；
// ChangeID 仅在 "resume" 时校验；AskReason 仅在 "ask" 时校验。
type WantContinuation struct {
	Decision  string `json:"decision"`
	ChangeID  string `json:"change_id,omitempty"`
	AskReason string `json:"ask_reason,omitempty"`
}

// StoreState 是 FM-3 fixture 的注入点：
// Revision 是 hive_spec_changes 行的当前 revision，
// ExportedRevision 是 FS 导出 artifact 内记录的 revision。
// Revision > ExportedRevision → reader 必须 fail-closed 或 reload。
type StoreState struct {
	Revision         int `json:"revision"`
	ExportedRevision int `json:"exported_revision"`
}
