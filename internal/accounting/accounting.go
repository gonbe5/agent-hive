package accounting

import (
	"context"
	"time"
)

// CostTracker 会话级成本追踪接口
type CostTracker interface {
	// Record 记录一条 LLM 用量条目
	Record(ctx context.Context, entry UsageEntry) error
	// GetSessionCost 获取指定会话的成本汇总
	GetSessionCost(ctx context.Context, sessionID string) (*CostSummary, error)
	// GetTotalCost 按过滤条件获取成本汇总
	GetTotalCost(ctx context.Context, filter CostFilter) (*CostSummary, error)
	// GetCostByUser 按用户分组的成本汇总
	GetCostByUser(ctx context.Context) ([]UserCost, error)
	// Cleanup 清理超过 retentionDays 天的历史记录，返回删除行数
	Cleanup(ctx context.Context, retentionDays int) (int64, error)
}

// UsageEntry 单次 LLM 调用的用量记录
type UsageEntry struct {
	SessionID        string  `json:"session_id"`
	UserID           string  `json:"user_id"` // Phase 5: 新增
	Model            string  `json:"model"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"` // 本次调用的美元成本（由调用方根据 ModelMeta 计算）
}

// CostSummary 成本汇总
type CostSummary struct {
	TotalCostUSD          float64              `json:"total_cost_usd"`
	TotalPromptTokens     int64                `json:"total_prompt_tokens"`
	TotalCompletionTokens int64                `json:"total_completion_tokens"`
	TotalTokens           int64                `json:"total_tokens"` // = prompt + completion，前端用
	RequestCount          int64                `json:"request_count"`
	ByModel               map[string]ModelCost `json:"by_model,omitempty"`
}

// ModelCost 按模型维度的成本明细
type ModelCost struct {
	CostUSD          float64 `json:"cost_usd"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	Tokens           int64   `json:"tokens"` // = prompt + completion，前端用
	RequestCount     int64   `json:"request_count"`
}

// CostFilter 成本查询过滤条件
type CostFilter struct {
	SessionID string     `json:"session_id,omitempty"` // 按会话过滤（空=全部）
	UserID    string     `json:"user_id,omitempty"`    // Phase 5: 新增，用于按用户查询
	Model     string     `json:"model,omitempty"`      // 按模型过滤（空=全部）
	Since     *time.Time `json:"since,omitempty"`       // 起始时间（nil=不限）
	Until     *time.Time `json:"until,omitempty"`       // 截止时间（nil=不限）
}

// CalcCost 根据模型定价计算单次调用成本
func CalcCost(promptTokens, completionTokens int64, costPerInput, costPerOutput float64) float64 {
	return float64(promptTokens)*costPerInput + float64(completionTokens)*costPerOutput
}

// UserCost 按用户维度的成本汇总
type UserCost struct {
	UserID           string  `json:"user_id"`
	TotalTokens      int64   `json:"total_tokens"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	RequestCount     int64   `json:"request_count"`
}
