// Package continuation 实现 spec-driven Phase 2 的 Guard 1：
// 是否把当前 user input 关联回某个 active change。
//
// 默认 OFF / fail-closed（design.md D1）：任何歧义都返回 ASK，
// 绝不静默 RESUME。宁可多问一次，不能让 subagent 6h 前偷偷改的
// MRU 在用户不觉察时劫持本轮决策（FM-1 反样本）。
package continuation

import (
	"regexp"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// DecayConfig 控制 MRU 时效与关键词匹配阈值。
type DecayConfig struct {
	// StrongWindow：用户没显式提 change_id，但关键词强匹配 + MRU 时间距今小于这个值
	// → 自动 RESUME。默认 30 分钟。FM-1 反例要求不得 > 2h（subagent 6h 前的 touch
	// 绝不能算强信号）。
	StrongWindow time.Duration

	// WeakWindow：关键词弱匹配 / MRU 时间距今在 (StrongWindow, WeakWindow]
	// → 返回 ASK。超过 WeakWindow 的 MRU 视同无信号，RESUME 不成立。
	WeakWindow time.Duration

	// KeywordMin：关键词重叠的最小 token 数（tokenized input vs ChangeRef.Title）。
	// <= 此值视为无匹配；> 此值才认为有信号。
	KeywordMin int
}

// DefaultDecayConfig 是 Codex Round 2 审查后的保守默认。
// FM-1 反例明确要求 StrongWindow << 6h；这里取 30min。
func DefaultDecayConfig() DecayConfig {
	return DecayConfig{
		StrongWindow: 30 * time.Minute,
		WeakWindow:   2 * time.Hour,
		KeywordMin:   1,
	}
}

// Trigger 标记 Decision 的触发原因。
// metric: specdriven.continuation_resume_total{trigger}
// / specdriven.continuation_ask_total{reason}
type Trigger string

const (
	TriggerExplicitID     Trigger = "explicit_id"     // 用户文本命中 change_id
	TriggerStrongKeyword  Trigger = "strong_keyword"  // 关键词匹配 + MRU 在 StrongWindow 内
	TriggerNoSignal       Trigger = "no_signal"       // 无任何线索
	TriggerWeakSignal     Trigger = "weak_signal"     // 关键词弱 / MRU 在 WeakWindow
	TriggerDivergence     Trigger = "divergence"      // resolved ≠ active 且非 explicit
	TriggerMRUOnly        Trigger = "mru_only"        // 只有 MRU 信号无关键词（ASK 路径）
)

// Result 是 Resolve 的输出。
type Result struct {
	Decision specdriven.Decision
	Trigger  Trigger
	// Candidates 记录解析过程识别到的候选（用于 ambiguous event payload 与 UI ask）。
	Candidates []specdriven.ChangeRef
}

// explicitIDRegex 捕获形如 add-spec-xyz / harden-spec-driven-phase2 / fix-...
// 这种 kebab-case change id。stricter: 至少两段（防误判普通 kebab 词）。
// 另外再接受 #<hex> 形式（短 id，min 6 hex）。
var (
	explicitKebabID = regexp.MustCompile(`\b[a-z][a-z0-9]*(?:-[a-z0-9]+){1,}\b`)
	explicitHashID  = regexp.MustCompile(`#([a-z0-9]{6,})\b`)
)

// tokenRegex 把 input/title 拆成小写 alnum token（用于关键词重叠统计）。
var tokenRegex = regexp.MustCompile(`[A-Za-z0-9]+`)

// Resolve 根据 (userText, state, now, cfg) 决定本轮 continuation。
//
// 决策顺序（design.md § Guard 1 决策矩阵）：
//  1. explicit_id：从文本提取 change_id，若命中 state.Changes → RESUME（即使与 ActiveChangeID 不同，
//     因为是用户显式指定）
//  2. strong_keyword：未 explicit 但关键词 >= KeywordMin 命中某 ChangeRef 且 now - LastTouched <= StrongWindow
//     → RESUME，但对 divergence 做复查（resolved ≠ Active → 降级 ASK）
//  3. weak_signal：关键词弱匹配 / MRU 在 WeakWindow 区间 / 纯 MRU 无文本信号 → ASK
//  4. no_signal：完全空载 → NEW
//
// 所有非 explicit 路径都带 fail-closed：任何模糊都走 ASK，绝不悄悄 RESUME。
func Resolve(userText string, state specdriven.SessionSpecState, now time.Time, cfg DecayConfig) Result {
	text := strings.ToLower(userText)

	// ——— 1. Explicit id ———
	if id := findExplicitID(text, state); id != "" {
		ref := state.Changes[id]
		return Result{
			Decision: specdriven.Decision{
				Kind:     specdriven.DecisionResume,
				ChangeID: id,
			},
			Trigger:    TriggerExplicitID,
			Candidates: []specdriven.ChangeRef{ref},
		}
	}

	// ——— 2/3. Keyword + MRU 分析 ———
	// 对每个 known ChangeRef 算 (overlap, age)。overlap 越高、age 越近越强。
	type scored struct {
		ref     specdriven.ChangeRef
		overlap int
		age     time.Duration
	}
	inputTokens := tokenize(text)
	var strong []scored
	var weak []scored
	for _, id := range state.FocusMRU {
		ref, ok := state.Changes[id]
		if !ok {
			continue
		}
		overlap := keywordOverlap(inputTokens, ref.Title)
		age := max(now.Sub(ref.LastTouched), 0)
		switch {
		case overlap >= cfg.KeywordMin && age <= cfg.StrongWindow:
			strong = append(strong, scored{ref, overlap, age})
		case overlap >= cfg.KeywordMin && age <= cfg.WeakWindow:
			weak = append(weak, scored{ref, overlap, age})
		}
	}

	// ——— Strong 路径 ———
	if len(strong) == 1 {
		best := strong[0]
		// divergence check：resolved ≠ ActiveChangeID 且非 explicit → 降级 ASK
		if state.ActiveChangeID != "" && best.ref.ID != state.ActiveChangeID {
			return Result{
				Decision: specdriven.Decision{
					Kind:      specdriven.DecisionAsk,
					AskReason: "resolved change diverges from active change; require user confirmation",
				},
				Trigger:    TriggerDivergence,
				Candidates: []specdriven.ChangeRef{best.ref, state.Changes[state.ActiveChangeID]},
			}
		}
		return Result{
			Decision: specdriven.Decision{
				Kind:     specdriven.DecisionResume,
				ChangeID: best.ref.ID,
			},
			Trigger:    TriggerStrongKeyword,
			Candidates: []specdriven.ChangeRef{best.ref},
		}
	}
	if len(strong) > 1 {
		// 多个都强匹配 → 必 ASK（绝不赌）
		refs := make([]specdriven.ChangeRef, 0, len(strong))
		for _, s := range strong {
			refs = append(refs, s.ref)
		}
		return Result{
			Decision: specdriven.Decision{
				Kind:      specdriven.DecisionAsk,
				AskReason: "multiple changes match user input; require user confirmation",
			},
			Trigger:    TriggerWeakSignal,
			Candidates: refs,
		}
	}

	// ——— Weak 路径 ———
	if len(weak) > 0 {
		refs := make([]specdriven.ChangeRef, 0, len(weak))
		for _, s := range weak {
			refs = append(refs, s.ref)
		}
		return Result{
			Decision: specdriven.Decision{
				Kind:      specdriven.DecisionAsk,
				AskReason: "weak keyword/MRU signal; require user confirmation",
			},
			Trigger:    TriggerWeakSignal,
			Candidates: refs,
		}
	}

	// ——— 纯 MRU，无文本信号 ———
	// FM-1 反例命脉：subagent 6h 前 touch 的 MRU 不能当信号。
	// 若有 ActiveChangeID 但 user 没带任何关键词，也必须 ASK——绝不默认 RESUME。
	if state.ActiveChangeID != "" {
		if ref, ok := state.Changes[state.ActiveChangeID]; ok {
			if now.Sub(ref.LastTouched) <= cfg.WeakWindow {
				return Result{
					Decision: specdriven.Decision{
						Kind:      specdriven.DecisionAsk,
						AskReason: "active change exists but user input has no keywords; require user confirmation",
					},
					Trigger:    TriggerMRUOnly,
					Candidates: []specdriven.ChangeRef{ref},
				}
			}
		}
	}

	// ——— 无信号 ———
	return Result{
		Decision: specdriven.Decision{Kind: specdriven.DecisionNew},
		Trigger:  TriggerNoSignal,
	}
}

// findExplicitID 在 text 中找与 state.Changes 某个 id 精确匹配的 token。
// 返回空字符串表示未找到。
func findExplicitID(text string, state specdriven.SessionSpecState) string {
	// kebab-case id 命中
	for _, m := range explicitKebabID.FindAllString(text, -1) {
		if _, ok := state.Changes[m]; ok {
			return m
		}
	}
	// #hexprefix 命中（短 id 形式）
	for _, m := range explicitHashID.FindAllStringSubmatch(text, -1) {
		if len(m) < 2 {
			continue
		}
		prefix := m[1]
		for id := range state.Changes {
			if strings.HasPrefix(id, prefix) {
				return id
			}
		}
	}
	return ""
}

// tokenize 小写拆词，过滤空 token。
func tokenize(text string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, tok := range tokenRegex.FindAllString(strings.ToLower(text), -1) {
		if len(tok) == 0 {
			continue
		}
		out[tok] = struct{}{}
	}
	return out
}

// keywordOverlap 返回 input tokens 与 title tokens 的重叠数。
func keywordOverlap(inputTokens map[string]struct{}, title string) int {
	if len(inputTokens) == 0 || title == "" {
		return 0
	}
	overlap := 0
	for _, tok := range tokenRegex.FindAllString(strings.ToLower(title), -1) {
		if _, ok := inputTokens[tok]; ok {
			overlap++
		}
	}
	return overlap
}
