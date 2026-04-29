package skills

import (
	"context"
)

// SuggestedAction 是 resolver 在远程命中时提供给 LLM / 调用方的建议动作。
// 由 tool handler 序列化为 tool_result 的 suggested_action 字段。
type SuggestedAction struct {
	Tool   string         `json:"tool"`             // e.g. "skill_install"
	Args   map[string]any `json:"args"`             // 调用参数
	Reason string         `json:"reason,omitempty"` // 人类可读的推荐理由
}

// SpecResolveResult 聚合本地 + 远程解析结果。
// 调用方可根据需要合并/排序 Local 和 Remote；Suggested 仅在远程命中时填充。
type SpecResolveResult struct {
	Local     []*Skill
	Remote    []*ResolvedSkill
	Suggested *SuggestedAction
}

// SpecSkillResolver 统一入口：先查本地 Registry，miss 则查远程 Discovery（flag-gated）。
// 方法分工硬契约：
//   - 本地查询 ONLY 走 Registry.FindBySpecRequirements
//   - 远程查询 ONLY 走 Discovery.ResolveByRequirements
//   - spec planner 不可绕过此接口直接调底层 API
type SpecSkillResolver interface {
	Resolve(ctx context.Context, reqs []string, userID string) (*SpecResolveResult, error)
}

// LocalSkillFinder 是 defaultResolver 对 Registry 的依赖抽象（便于测试 mock 方法分工）。
type LocalSkillFinder interface {
	FindBySpecRequirements(reqs []string, userID string) []*Skill
}

// RemoteSkillFinder 是 defaultResolver 对 Discovery 的依赖抽象。
type RemoteSkillFinder interface {
	ResolveByRequirements(ctx context.Context, reqs []string) ([]*ResolvedSkill, error)
}

// defaultResolver 是 SpecSkillResolver 的标准实现。
type defaultResolver struct {
	local        LocalSkillFinder
	remote       RemoteSkillFinder
	remoteAllow  func() bool // feature-flag gate：on_demand_enabled && skills_semantic_routing
}

// NewSpecSkillResolver 构造 defaultResolver。remoteAllow 为 nil 时视为禁用远程。
func NewSpecSkillResolver(local LocalSkillFinder, remote RemoteSkillFinder, remoteAllow func() bool) SpecSkillResolver {
	return &defaultResolver{local: local, remote: remote, remoteAllow: remoteAllow}
}

// Resolve 调度顺序：
//  1. 本地 Registry.FindBySpecRequirements(reqs, userID) 命中 → 直接返回（短路远程）
//  2. 本地 miss + remoteAllow()==true + remote 非 nil → Discovery.ResolveByRequirements(reqs)
//     远程命中时构造 SuggestedAction{tool: "skill_install", ...} 附在结果里
//  3. 本地 miss + 远程关/无 → 返回空结果（不报错，交由调用方决定 fallback）
func (r *defaultResolver) Resolve(ctx context.Context, reqs []string, userID string) (*SpecResolveResult, error) {
	result := &SpecResolveResult{}
	if len(reqs) == 0 {
		return result, nil
	}

	// 第 1 步：本地
	if r.local != nil {
		if local := r.local.FindBySpecRequirements(reqs, userID); len(local) > 0 {
			result.Local = local
			return result, nil
		}
	}

	// 第 2 步：远程（flag-gated）
	if r.remote == nil || r.remoteAllow == nil || !r.remoteAllow() {
		return result, nil
	}
	remote, err := r.remote.ResolveByRequirements(ctx, reqs)
	if err != nil {
		return result, err
	}
	if len(remote) == 0 {
		return result, nil
	}
	result.Remote = remote
	result.Suggested = suggestInstallForRemote(remote[0], userID)
	return result, nil
}

// suggestInstallForRemote 为最佳远程候选构造 skill_install 建议。
func suggestInstallForRemote(rs *ResolvedSkill, userID string) *SuggestedAction {
	scope := "personal"
	if rs.Entry.ScopeHint == "public" {
		scope = "public"
	}
	if userID == "" {
		// 匿名调用默认只能装 public，但仍需 admin 审核（由 skill_install handler 执行）
		scope = "public"
	}
	return &SuggestedAction{
		Tool: "skill_install",
		Args: map[string]any{
			"name":   rs.Entry.Name,
			"scope":  scope,
			"source": rs.Source,
		},
		Reason: "remote marketplace 命中 ProvidesRequirements，本地未装",
	}
}
