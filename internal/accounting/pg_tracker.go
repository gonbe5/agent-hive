package accounting

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// PgTracker 基于 PostgreSQL 的成本追踪实现
type PgTracker struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewPgTracker 创建 PG 成本追踪器
func NewPgTracker(pool *pgxpool.Pool, logger *zap.Logger) *PgTracker {
	return &PgTracker{pool: pool, logger: logger}
}

// Record 记录一条 LLM 用量
func (t *PgTracker) Record(ctx context.Context, entry UsageEntry) error {
	_, err := t.pool.Exec(ctx,
		`INSERT INTO usage_records (session_id, user_id, model, prompt_tokens, completion_tokens, cost_usd)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		entry.SessionID, entry.UserID, entry.Model, entry.PromptTokens, entry.CompletionTokens, entry.CostUSD,
	)
	if err != nil {
		t.logger.Warn("记录用量失败", zap.Error(err))
	}
	return err
}

// GetSessionCost 获取指定会话的成本汇总
func (t *PgTracker) GetSessionCost(ctx context.Context, sessionID string) (*CostSummary, error) {
	return t.GetTotalCost(ctx, CostFilter{SessionID: sessionID})
}

// GetTotalCost 按过滤条件获取成本汇总（单次查询，天然一致）
func (t *PgTracker) GetTotalCost(ctx context.Context, filter CostFilter) (*CostSummary, error) {
	where, args := buildWhere(filter)

	// 单次 GROUP BY model 查询，Go 侧聚合出 Total 字段，避免两次查询快照不一致
	query := `SELECT model, COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
	          COALESCE(SUM(cost_usd),0), COUNT(*)
	          FROM usage_records` + where + ` GROUP BY model`

	rows, err := t.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := &CostSummary{ByModel: make(map[string]ModelCost)}
	for rows.Next() {
		var model string
		var mc ModelCost
		if err := rows.Scan(&model, &mc.PromptTokens, &mc.CompletionTokens, &mc.CostUSD, &mc.RequestCount); err != nil {
			return nil, err
		}
		mc.Tokens = mc.PromptTokens + mc.CompletionTokens
		summary.ByModel[model] = mc
		summary.TotalPromptTokens += mc.PromptTokens
		summary.TotalCompletionTokens += mc.CompletionTokens
		summary.TotalCostUSD += mc.CostUSD
		summary.RequestCount += mc.RequestCount
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	summary.TotalTokens = summary.TotalPromptTokens + summary.TotalCompletionTokens

	return summary, nil
}

// GetCostByUser 按用户分组的成本汇总
func (t *PgTracker) GetCostByUser(ctx context.Context) ([]UserCost, error) {
	rows, err := t.pool.Query(ctx,
		`SELECT COALESCE(user_id,''), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		        COALESCE(SUM(cost_usd),0), COUNT(*)
		 FROM usage_records GROUP BY user_id ORDER BY SUM(cost_usd) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []UserCost
	for rows.Next() {
		var uc UserCost
		if err := rows.Scan(&uc.UserID, &uc.PromptTokens, &uc.CompletionTokens, &uc.CostUSD, &uc.RequestCount); err != nil {
			return nil, err
		}
		uc.TotalTokens = uc.PromptTokens + uc.CompletionTokens
		result = append(result, uc)
	}
	return result, rows.Err()
}

// Cleanup 清理超过 retentionDays 天的历史记录，返回删除行数
func (t *PgTracker) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays < 1 {
		return 0, fmt.Errorf("retentionDays must be >= 1, got %d", retentionDays)
	}
	tag, err := t.pool.Exec(ctx,
		`DELETE FROM usage_records WHERE created_at < NOW() - ($1 || ' days')::interval`,
		strconv.Itoa(retentionDays),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// buildWhere 根据 CostFilter 构建 WHERE 子句和参数列表
func buildWhere(filter CostFilter) (string, []any) {
	where := " WHERE 1=1"
	args := []any{}
	idx := 1

	if filter.SessionID != "" {
		where += " AND session_id = $" + strconv.Itoa(idx)
		args = append(args, filter.SessionID)
		idx++
	}
	if filter.UserID != "" {
		where += " AND user_id = $" + strconv.Itoa(idx)
		args = append(args, filter.UserID)
		idx++
	}
	if filter.Model != "" {
		where += " AND model = $" + strconv.Itoa(idx)
		args = append(args, filter.Model)
		idx++
	}
	if filter.Since != nil {
		where += " AND created_at >= $" + strconv.Itoa(idx)
		args = append(args, *filter.Since)
		idx++
	}
	if filter.Until != nil {
		where += " AND created_at <= $" + strconv.Itoa(idx)
		args = append(args, *filter.Until)
		idx++
	}

	return where, args
}
