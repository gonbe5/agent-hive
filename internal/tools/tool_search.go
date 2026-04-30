package tools

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// toolSearchInput 是 tool_search 的入参。
// Query 对 name/description 做 substring case-insensitive 匹配；空 query 列出所有工具。
type toolSearchInput struct {
	Query string `json:"query,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type toolSearchHit struct {
	Name              string  `json:"name"`
	Description       string  `json:"description,omitempty"`
	DangerLevel       string  `json:"danger_level"`
	RequiresApproval  bool    `json:"requires_approval"`
	IsConcurrencySafe bool    `json:"is_concurrency_safe"`
	Core              bool    `json:"core,omitempty"`
	Score             float64 `json:"score"`
}

func registerToolSearch(host *mcphost.Host, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "按工具 name/description 做不区分大小写的子串搜索；为空列出所有已注册工具",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "返回 top N（按 score desc）；0 不限",
			},
		},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:              "tool_search",
			Description:       "搜索/列出当前已注册工具的名称、描述和可用安全元数据。只读，不执行、不隐藏、不改变工具注册表。",
			InputSchema:       schema,
			Core:              true,
			IsConcurrencySafe: true,
		},
		func(ctx context.Context, raw json.RawMessage) (*mcphost.ToolResult, error) {
			return handleToolSearch(host, raw)
		},
	)
	if logger != nil {
		logger.Info("已注册 tool_search 工具")
	}
}

func handleToolSearch(host *mcphost.Host, raw json.RawMessage) (*mcphost.ToolResult, error) {
	var in toolSearchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return errorResult("tool_search 输入无效: " + err.Error()), nil
	}

	qLower := strings.ToLower(strings.TrimSpace(in.Query))
	defs := host.ListTools()
	hits := make([]toolSearchHit, 0, len(defs))
	for _, def := range defs {
		if !matchesQuery(qLower, def.Name, def.Description, nil, nil) {
			continue
		}
		hits = append(hits, toolSearchHit{
			Name:              def.Name,
			Description:       def.Description,
			DangerLevel:       inferToolDangerLevel(def),
			RequiresApproval:  false,
			IsConcurrencySafe: def.IsConcurrencySafe,
			Core:              def.Core,
			Score:             scoreHit(qLower, def.Name, def.Description, nil, false, 0),
		})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Name < hits[j].Name
		}
		return hits[i].Score > hits[j].Score
	})
	if in.Limit > 0 && len(hits) > in.Limit {
		hits = hits[:in.Limit]
	}

	out, _ := json.Marshal(map[string]any{
		"count":   len(hits),
		"results": hits,
	})
	return textResult(string(out)), nil
}

func inferToolDangerLevel(def mcphost.ToolDefinition) string {
	if def.IsConcurrencySafe {
		return "safe"
	}
	return "unknown"
}
