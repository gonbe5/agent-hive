package tools

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/mcphost"
	"github.com/chef-guo/agents-hive/internal/skills"
)

// skillSearchInput 是 skill_search 的入参。
//   - Query        : 对 name/description/trigger_keywords/tags 做 substring case-insensitive 匹配
//   - Requirements : 仅返回满足这些 requirement name 的 skill（remote 走 ResolveByRequirements）
//   - Scope        : "public" | "personal" | "" (不限)；仅对本地结果生效
//   - Limit        : 0/负值 → 不限；>0 → 头 N 条（按 score desc）
//   - IncludeRemote: 是否也查 marketplace（默认 true；测试可关）
type skillSearchInput struct {
	Query         string   `json:"query,omitempty"`
	Requirements  []string `json:"requirements,omitempty"`
	Scope         string   `json:"scope,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	IncludeRemote *bool    `json:"include_remote,omitempty"`
}

// skillSearchHit 是单条结果。Source 取值：
//   - "local-personal" / "local-public"
//   - marketplace URL（远程）
type skillSearchHit struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	Version              string   `json:"version,omitempty"`
	Scope                string   `json:"scope,omitempty"`
	Source               string   `json:"source"`
	ProvidesRequirements []string `json:"provides_requirements,omitempty"`
	Score                float64  `json:"score"`
}

// skillSearchRegistry 是 skill_search 对 Registry 的最小接口。
type skillSearchRegistry interface {
	List(userID ...string) []skills.SkillMetadata
}

// registerSkillSearch 注册 skill_search 工具。只读，不写盘。
func registerSkillSearch(host *mcphost.Host, logger *zap.Logger, skillReg skillSearchRegistry, discovery *skills.Discovery) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "子串匹配 name/description/keywords/tags（不区分大小写）",
			},
			"requirements": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "只返回满足这些 requirement name 的 skill",
			},
			"scope": map[string]any{
				"type":        "string",
				"enum":        []string{"public", "personal"},
				"description": "仅限本地；为空则公共+个人都查",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "返回 top N（按 score desc）；0 不限",
			},
			"include_remote": map[string]any{
				"type":        "boolean",
				"description": "是否查询 marketplace（默认 true）",
			},
		},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:              "skill_search",
			Description:       "搜索本地 + marketplace 的 skill；标注 source/scope/version/score。只查不写。",
			InputSchema:       schema,
			IsConcurrencySafe: true,
		},
		func(ctx context.Context, raw json.RawMessage) (*mcphost.ToolResult, error) {
			return handleSkillSearch(ctx, skillReg, discovery, raw)
		},
	)
	if logger != nil {
		logger.Info("已注册 skill_search 工具")
	}
}

// handleSkillSearch 合并本地 + 远程结果，按 score desc 排序。
func handleSkillSearch(ctx context.Context, reg skillSearchRegistry, disc *skills.Discovery, raw json.RawMessage) (*mcphost.ToolResult, error) {
	var in skillSearchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return errorResult("skill_search 输入无效: " + err.Error()), nil
	}

	includeRemote := true
	if in.IncludeRemote != nil {
		includeRemote = *in.IncludeRemote
	}
	wantScope, _ := skills.ParseScope(in.Scope)
	userID := auth.UserIDFrom(ctx)
	qLower := strings.ToLower(strings.TrimSpace(in.Query))

	hits := make([]skillSearchHit, 0, 16)

	// --- 本地 ---
	// Registry.List(userID) 已按 userID 做 personal 隔离（OverlayRegistry 的语义）；
	// 未传 userID → 仅公共层。这里严格按 auth ctx 的 userID 过滤，跨租户隔离由
	// Registry 层保证。
	var local []skills.SkillMetadata
	if reg != nil {
		if userID != "" {
			local = reg.List(userID)
		} else {
			local = reg.List()
		}
	}
	for _, m := range local {
		effScope := m.Scope
		if effScope == "" {
			effScope = skills.ScopePublic
		}
		if in.Scope != "" && effScope != wantScope {
			continue
		}
		if !matchesQuery(qLower, m.Name, m.Description, m.TriggerKeywords, nil) {
			continue
		}
		if !coversRequirements(in.Requirements, m.ProvidesRequirements) {
			continue
		}
		source := "local-public"
		if effScope == skills.ScopePersonal {
			source = "local-personal"
		}
		hits = append(hits, skillSearchHit{
			Name:                 m.Name,
			Description:          m.Description,
			Version:              m.Version,
			Scope:                string(effScope),
			Source:               source,
			ProvidesRequirements: m.ProvidesRequirements,
			Score:                scoreHit(qLower, m.Name, m.Description, m.TriggerKeywords, len(in.Requirements) > 0, len(m.ProvidesRequirements)),
		})
	}

	// --- 远程 ---
	// Scope 过滤仅适用于本地；远程结果默认算 "public" 候选。
	if includeRemote && disc != nil && in.Scope != "personal" {
		remote := discoverRemote(ctx, disc, in)
		for _, rs := range remote {
			hits = append(hits, skillSearchHit{
				Name:                 rs.Entry.Name,
				Description:          rs.Entry.Description,
				Version:              rs.Entry.Version,
				Scope:                "public",
				Source:               rs.Source,
				ProvidesRequirements: rs.Entry.ProvidesRequirements,
				Score:                scoreHit(qLower, rs.Entry.Name, rs.Entry.Description, nil, len(in.Requirements) > 0, len(rs.Entry.ProvidesRequirements)),
			})
		}
	}

	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if in.Limit > 0 && len(hits) > in.Limit {
		hits = hits[:in.Limit]
	}

	out, _ := json.Marshal(map[string]any{
		"count":   len(hits),
		"results": hits,
	})
	return textResult(string(out)), nil
}

// discoverRemote 包一层：有 Requirements 时走 ResolveByRequirements；否则对每个
// marketplace index 做 substring 匹配。
func discoverRemote(ctx context.Context, disc *skills.Discovery, in skillSearchInput) []*skills.ResolvedSkill {
	if len(in.Requirements) > 0 {
		res, err := disc.ResolveByRequirements(ctx, in.Requirements)
		if err != nil {
			return nil
		}
		return res
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil // 无 query 无 requirements → 不扫远程（避免一次性拉爆）
	}
	// ResolveByName 只精确匹配；subtring 场景下降级为遍历各 marketplace URL 的 index。
	// Discovery 当前不暴露 ListByFuzzy；我们用 ResolveByName 做精确命中一次作为 MVP。
	// 更完整的模糊远程搜索待 Discovery 增强后补。
	if entry, err := disc.ResolveByName(ctx, strings.TrimSpace(in.Query), false); err == nil && entry != nil {
		return []*skills.ResolvedSkill{entry}
	}
	return nil
}

// matchesQuery 对 name/description/keywords 做 substring case-insensitive 匹配。
// q=="" 视为匹配所有。
func matchesQuery(q, name, desc string, keywords []string, tags []string) bool {
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(desc), q) {
		return true
	}
	for _, k := range keywords {
		if strings.Contains(strings.ToLower(k), q) {
			return true
		}
	}
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

// coversRequirements 返回 skill 是否提供了 want 中的任意一项。
// want 为空 → 视为无约束 → true。
func coversRequirements(want, provides []string) bool {
	if len(want) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(provides))
	for _, p := range provides {
		set[p] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; ok {
			return true
		}
	}
	return false
}

// scoreHit 计算粗粒度 score：name 精确匹配 > name 部分匹配 > description 匹配 > 关键词匹配。
// Requirements 命中追加 0.5；provides 数量给微小加权。
func scoreHit(qLower, name, desc string, keywords []string, reqQuery bool, providesCount int) float64 {
	score := 0.0
	nLower := strings.ToLower(name)
	if qLower != "" {
		switch {
		case nLower == qLower:
			score += 3.0
		case strings.HasPrefix(nLower, qLower):
			score += 2.0
		case strings.Contains(nLower, qLower):
			score += 1.5
		case strings.Contains(strings.ToLower(desc), qLower):
			score += 0.8
		default:
			for _, k := range keywords {
				if strings.Contains(strings.ToLower(k), qLower) {
					score += 0.5
					break
				}
			}
		}
	} else {
		score += 0.5 // 无 query 基线分
	}
	if reqQuery && providesCount > 0 {
		score += 0.5
	}
	if providesCount > 0 {
		score += 0.05 * float64(providesCount)
	}
	return score
}
