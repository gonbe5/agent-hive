package continuation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/continuation"
)

// Fixture：模拟一个带 3 个 change 的 session state。
func buildState(now time.Time) specdriven.SessionSpecState {
	return specdriven.SessionSpecState{
		ActiveChangeID: "harden-spec-driven-phase2",
		FocusMRU:       []string{"harden-spec-driven-phase2", "add-user-auth", "refactor-billing"},
		Changes: map[string]specdriven.ChangeRef{
			"harden-spec-driven-phase2": {
				ID:          "harden-spec-driven-phase2",
				Title:       "harden spec driven phase 2",
				Status:      "in_progress",
				LastTouched: now.Add(-10 * time.Minute),
			},
			"add-user-auth": {
				ID:          "add-user-auth",
				Title:       "add user auth with OAuth2",
				Status:      "draft",
				LastTouched: now.Add(-45 * time.Minute),
			},
			"refactor-billing": {
				ID:          "refactor-billing",
				Title:       "refactor billing pipeline",
				Status:      "in_progress",
				LastTouched: now.Add(-3 * time.Hour),
			},
		},
	}
}

// TestResolve_ExplicitID：用户显式提了 change_id → RESUME 该 id（即使与 Active 不同）。
func TestResolve_ExplicitID(t *testing.T) {
	now := time.Now()
	state := buildState(now)
	r := continuation.Resolve("continue on add-user-auth please", state, now, continuation.DefaultDecayConfig())
	assert.Equal(t, continuation.TriggerExplicitID, r.Trigger)
	assert.Equal(t, specdriven.DecisionResume, r.Decision.Kind)
	assert.Equal(t, "add-user-auth", r.Decision.ChangeID)
}

// TestResolve_StrongKeyword_Active：关键词命中 Active 且 LastTouched 在 StrongWindow 内
// → RESUME 到 Active。
func TestResolve_StrongKeyword_Active(t *testing.T) {
	now := time.Now()
	state := buildState(now)
	r := continuation.Resolve("let me harden phase 2 further", state, now, continuation.DefaultDecayConfig())
	assert.Equal(t, continuation.TriggerStrongKeyword, r.Trigger)
	assert.Equal(t, specdriven.DecisionResume, r.Decision.Kind)
	assert.Equal(t, "harden-spec-driven-phase2", r.Decision.ChangeID)
}

// TestResolve_Divergence：关键词 strong 命中 NOT-active 的 change，但 active 非空
// → 必须 ASK（FM-1 变种：防止关键词撞车劫持用户当前 focus）。
func TestResolve_Divergence(t *testing.T) {
	now := time.Now()
	state := buildState(now)
	// 把 add-user-auth 的 LastTouched 拉到 strong window 内
	ref := state.Changes["add-user-auth"]
	ref.LastTouched = now.Add(-5 * time.Minute)
	state.Changes["add-user-auth"] = ref
	// active 保持 harden-spec-driven-phase2 不动

	r := continuation.Resolve("can we work on the auth OAuth2 flow", state, now, continuation.DefaultDecayConfig())
	assert.Equal(t, continuation.TriggerDivergence, r.Trigger)
	assert.Equal(t, specdriven.DecisionAsk, r.Decision.Kind)
	assert.NotEmpty(t, r.Decision.AskReason)
	// Candidates 应包含两个候选：resolved + active
	assert.Len(t, r.Candidates, 2)
}

// TestResolve_WeakSignal：关键词命中但 age 落在 WeakWindow → ASK。
func TestResolve_WeakSignal(t *testing.T) {
	now := time.Now()
	state := buildState(now)
	// 只留 add-user-auth（age 45m，在 default weak window 2h 内，超过 strong 30m）
	state.Changes = map[string]specdriven.ChangeRef{
		"add-user-auth": state.Changes["add-user-auth"],
	}
	state.FocusMRU = []string{"add-user-auth"}
	state.ActiveChangeID = "" // 清空 active 避免走 divergence 分支

	r := continuation.Resolve("add user OAuth authentication", state, now, continuation.DefaultDecayConfig())
	assert.Equal(t, continuation.TriggerWeakSignal, r.Trigger)
	assert.Equal(t, specdriven.DecisionAsk, r.Decision.Kind)
}

// TestResolve_MultipleStrongMatches：多个都 strong → 必 ASK，不赌。
func TestResolve_MultipleStrongMatches(t *testing.T) {
	now := time.Now()
	state := specdriven.SessionSpecState{
		FocusMRU: []string{"billing-refactor", "billing-v2"},
		Changes: map[string]specdriven.ChangeRef{
			"billing-refactor": {ID: "billing-refactor", Title: "billing refactor", LastTouched: now.Add(-5 * time.Minute)},
			"billing-v2":       {ID: "billing-v2", Title: "billing pipeline v2", LastTouched: now.Add(-10 * time.Minute)},
		},
	}
	r := continuation.Resolve("continue with billing work", state, now, continuation.DefaultDecayConfig())
	assert.Equal(t, continuation.TriggerWeakSignal, r.Trigger)
	assert.Equal(t, specdriven.DecisionAsk, r.Decision.Kind)
	assert.Len(t, r.Candidates, 2)
}

// TestResolve_FM1_SubagentSilentMRU：Phase 2 spec 的 FM-1 反样本核心。
//
// 场景：subagent 在 6h 前 touch 了 FocusMRU 里的某个 change（把它设为 active），
// 用户现在发了一句没任何关键词的 "继续"。
// 期望：绝不 RESUME（silent 依赖过时 MRU 会让用户 blind）；必须 ASK。
func TestResolve_FM1_SubagentSilentMRU(t *testing.T) {
	now := time.Now()
	// subagent 6h 前写入了 active 但 user 没意识到
	touched := now.Add(-6 * time.Hour)
	state := specdriven.SessionSpecState{
		ActiveChangeID: "subagent-touched-change",
		FocusMRU:       []string{"subagent-touched-change"},
		Changes: map[string]specdriven.ChangeRef{
			"subagent-touched-change": {
				ID:          "subagent-touched-change",
				Title:       "something agent touched silently",
				LastTouched: touched,
			},
		},
	}

	// 极简 user input，无关键词
	r := continuation.Resolve("继续", state, now, continuation.DefaultDecayConfig())
	assert.NotEqual(t, specdriven.DecisionResume, r.Decision.Kind,
		"FM-1 红线：subagent 6h 前的 MRU 绝不能触发自动 RESUME")
	// 6h > default WeakWindow 2h → 直接视作无信号，走 NEW
	assert.Equal(t, specdriven.DecisionNew, r.Decision.Kind)
	assert.Equal(t, continuation.TriggerNoSignal, r.Trigger)
}

// TestResolve_FM1_ActiveWithinWeakButNoKeyword：FM-1 的另一半。
// Active touch 在 WeakWindow 内，但 user input 没任何关键词 → ASK（不 RESUME）。
func TestResolve_FM1_ActiveWithinWeakButNoKeyword(t *testing.T) {
	now := time.Now()
	state := specdriven.SessionSpecState{
		ActiveChangeID: "c1",
		FocusMRU:       []string{"c1"},
		Changes: map[string]specdriven.ChangeRef{
			"c1": {
				ID:          "c1",
				Title:       "auth flow",
				LastTouched: now.Add(-90 * time.Minute), // weak window (30m, 2h] 之内
			},
		},
	}
	// 用户输入完全没关键词
	r := continuation.Resolve("hello", state, now, continuation.DefaultDecayConfig())
	assert.Equal(t, specdriven.DecisionAsk, r.Decision.Kind,
		"active change in weak window + no keyword → 必 ASK")
	assert.Equal(t, continuation.TriggerMRUOnly, r.Trigger)
}

// TestResolve_NoSignal：空 state + 空 input → NEW。
func TestResolve_NoSignal(t *testing.T) {
	r := continuation.Resolve("let's do something new", specdriven.SessionSpecState{}, time.Now(), continuation.DefaultDecayConfig())
	assert.Equal(t, specdriven.DecisionNew, r.Decision.Kind)
	assert.Equal(t, continuation.TriggerNoSignal, r.Trigger)
}

// TestResolve_ExplicitIDWhenAmbiguous：用户显式提 id 即使 keyword 也撞其他 change
// → 仍以 explicit 为准。
func TestResolve_ExplicitIDWhenAmbiguous(t *testing.T) {
	now := time.Now()
	state := specdriven.SessionSpecState{
		ActiveChangeID: "other-one",
		FocusMRU:       []string{"other-one", "target-change"},
		Changes: map[string]specdriven.ChangeRef{
			"other-one":     {ID: "other-one", Title: "other one", LastTouched: now.Add(-5 * time.Minute)},
			"target-change": {ID: "target-change", Title: "target other", LastTouched: now.Add(-1 * time.Hour)},
		},
	}
	r := continuation.Resolve("switch to target-change", state, now, continuation.DefaultDecayConfig())
	assert.Equal(t, continuation.TriggerExplicitID, r.Trigger)
	assert.Equal(t, "target-change", r.Decision.ChangeID)
}

// TestResolve_HashPrefix：#abc123 形式的短 id 前缀匹配。
func TestResolve_HashPrefix(t *testing.T) {
	now := time.Now()
	state := specdriven.SessionSpecState{
		Changes: map[string]specdriven.ChangeRef{
			"abc123def": {ID: "abc123def", Title: "some work", LastTouched: now},
		},
	}
	r := continuation.Resolve("resume #abc123", state, now, continuation.DefaultDecayConfig())
	assert.Equal(t, continuation.TriggerExplicitID, r.Trigger)
	assert.Equal(t, "abc123def", r.Decision.ChangeID)
}
