package journal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// pgJournalInitSQL journal 表建表 SQL
const pgJournalInitSQL = `
-- 会话日志表
CREATE TABLE IF NOT EXISTS journal_sessions (
	session_id TEXT PRIMARY KEY,
	task       TEXT NOT NULL DEFAULT '',
	summary    TEXT NOT NULL DEFAULT '',
	started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	ended_at   TIMESTAMPTZ
);

-- 工具调用日志表
CREATE TABLE IF NOT EXISTS journal_tool_calls (
	id          BIGSERIAL PRIMARY KEY,
	session_id  TEXT NOT NULL,
	tool_name   TEXT NOT NULL,
	tool_call_id TEXT NOT NULL DEFAULT '',
	arguments   TEXT NOT NULL DEFAULT '',
	result      TEXT NOT NULL DEFAULT '',
	is_error    BOOLEAN NOT NULL DEFAULT FALSE,
	duration_ms BIGINT NOT NULL DEFAULT 0,
	created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_journal_tc_session ON journal_tool_calls(session_id, created_at);

-- 文件变更日志表
CREATE TABLE IF NOT EXISTS journal_file_changes (
	id         BIGSERIAL PRIMARY KEY,
	session_id TEXT NOT NULL,
	file_path  TEXT NOT NULL,
	action     TEXT NOT NULL DEFAULT 'edit',
	summary    TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_journal_fc_session ON journal_file_changes(session_id, created_at);

-- 决策日志表
CREATE TABLE IF NOT EXISTS journal_decisions (
	id         BIGSERIAL PRIMARY KEY,
	session_id TEXT NOT NULL,
	decision   TEXT NOT NULL,
	reason     TEXT NOT NULL DEFAULT '',
	agent_id   TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_journal_dec_session ON journal_decisions(session_id, created_at);
`

// PGJournal 基于 PostgreSQL 的日志实现
type PGJournal struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewPGJournal 创建 PGJournal 并执行建表迁移
func NewPGJournal(ctx context.Context, pool *pgxpool.Pool, logger *zap.Logger) (*PGJournal, error) {
	if _, err := pool.Exec(ctx, pgJournalInitSQL); err != nil {
		return nil, err
	}
	logger.Info("journal 表已初始化")
	return &PGJournal{pool: pool, logger: logger}, nil
}

// StartSession 开始或恢复一个会话日志。
// 恢复已结束的会话时清除 ended_at，避免数据矛盾（#3）。
func (j *PGJournal) StartSession(ctx context.Context, sessionID string, task string) error {
	_, err := j.pool.Exec(ctx,
		`INSERT INTO journal_sessions (session_id, task) VALUES ($1, $2)
		 ON CONFLICT (session_id) DO UPDATE SET task = $2, ended_at = NULL
		 WHERE journal_sessions.task != $2 OR journal_sessions.ended_at IS NOT NULL`,
		sessionID, task)
	return err
}

func (j *PGJournal) LogToolCall(ctx context.Context, entry ToolCallEntry) error {
	ts := entry.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	_, err := j.pool.Exec(ctx,
		`INSERT INTO journal_tool_calls (session_id, tool_name, tool_call_id, arguments, result, is_error, duration_ms, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		entry.SessionID, entry.ToolName, entry.ToolCallID, entry.Arguments, entry.Result,
		entry.IsError, entry.Duration.Milliseconds(), ts)
	return err
}

func (j *PGJournal) LogFileChange(ctx context.Context, entry FileChangeEntry) error {
	ts := entry.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	_, err := j.pool.Exec(ctx,
		`INSERT INTO journal_file_changes (session_id, file_path, action, summary, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		entry.SessionID, entry.FilePath, entry.Action, entry.Summary, ts)
	return err
}

func (j *PGJournal) LogDecision(ctx context.Context, entry DecisionEntry) error {
	ts := entry.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	_, err := j.pool.Exec(ctx,
		`INSERT INTO journal_decisions (session_id, decision, reason, agent_id, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		entry.SessionID, entry.Decision, entry.Reason, entry.AgentID, ts)
	return err
}

func (j *PGJournal) EndSession(ctx context.Context, sessionID string, summary string) error {
	_, err := j.pool.Exec(ctx,
		`UPDATE journal_sessions SET summary = $2, ended_at = NOW() WHERE session_id = $1`,
		sessionID, summary)
	return err
}

// queryWithLimit 执行带可选 LIMIT 的查询，统一处理 limit 分支逻辑
func (j *PGJournal) queryWithLimit(ctx context.Context, baseQuery string, sessionID string, limit int) (pgx.Rows, error) {
	if limit > 0 {
		return j.pool.Query(ctx, baseQuery+" LIMIT $2", sessionID, limit)
	}
	return j.pool.Query(ctx, baseQuery, sessionID)
}

// GetJournal 查询指定会话的完整日志。limit<=0 表示不限制条数。
func (j *PGJournal) GetJournal(ctx context.Context, sessionID string, limit int) (*SessionJournal, error) {
	sj := &SessionJournal{SessionID: sessionID}

	// 会话基本信息
	err := j.pool.QueryRow(ctx,
		`SELECT task, summary, started_at, ended_at FROM journal_sessions WHERE session_id = $1`,
		sessionID).Scan(&sj.Task, &sj.Summary, &sj.StartedAt, &sj.EndedAt)
	if err != nil {
		return nil, err
	}

	// 工具调用
	tcRows, err := j.queryWithLimit(ctx,
		`SELECT tool_name, tool_call_id, arguments, result, is_error, duration_ms, created_at
		 FROM journal_tool_calls WHERE session_id = $1 ORDER BY created_at, id`,
		sessionID, limit)
	if err != nil {
		return nil, err
	}
	for tcRows.Next() {
		var e ToolCallEntry
		var durationMs int64
		if err := tcRows.Scan(&e.ToolName, &e.ToolCallID, &e.Arguments, &e.Result, &e.IsError, &durationMs, &e.Timestamp); err != nil {
			tcRows.Close()
			return nil, err
		}
		e.Duration = time.Duration(durationMs) * time.Millisecond
		e.SessionID = sessionID
		sj.ToolCalls = append(sj.ToolCalls, e)
	}
	tcRows.Close()
	if err := tcRows.Err(); err != nil {
		return nil, err
	}

	// 文件变更
	fcRows, err := j.queryWithLimit(ctx,
		`SELECT file_path, action, summary, created_at
		 FROM journal_file_changes WHERE session_id = $1 ORDER BY created_at, id`,
		sessionID, limit)
	if err != nil {
		return nil, err
	}
	for fcRows.Next() {
		var e FileChangeEntry
		if err := fcRows.Scan(&e.FilePath, &e.Action, &e.Summary, &e.Timestamp); err != nil {
			fcRows.Close()
			return nil, err
		}
		e.SessionID = sessionID
		sj.FileChanges = append(sj.FileChanges, e)
	}
	fcRows.Close()
	if err := fcRows.Err(); err != nil {
		return nil, err
	}

	// 决策
	decRows, err := j.queryWithLimit(ctx,
		`SELECT decision, reason, agent_id, created_at
		 FROM journal_decisions WHERE session_id = $1 ORDER BY created_at, id`,
		sessionID, limit)
	if err != nil {
		return nil, err
	}
	for decRows.Next() {
		var e DecisionEntry
		if err := decRows.Scan(&e.Decision, &e.Reason, &e.AgentID, &e.Timestamp); err != nil {
			decRows.Close()
			return nil, err
		}
		e.SessionID = sessionID
		sj.Decisions = append(sj.Decisions, e)
	}
	decRows.Close()
	if err := decRows.Err(); err != nil {
		return nil, err
	}

	return sj, nil
}

// GetJournalEvents 返回统一事件流（UNION ALL 合并三表，三级排序保证稳定）。
// after 非零时只返回 timestamp > after 的事件，供增量查询使用。
func (j *PGJournal) GetJournalEvents(ctx context.Context, sessionID string, limit int, after time.Time) ([]JournalEvent, error) {
	// 基础 UNION ALL 查询，支持可选的 after 时间过滤
	afterCond := ""
	if !after.IsZero() {
		afterCond = " AND created_at > $2"
	}
	query := `
SELECT type, timestamp, tool_name, arguments, result, is_error, duration_ms,
       file_path, action, summary, decision, reason
FROM (
    SELECT 'tool_call' AS type, created_at AS timestamp,
           tool_name, arguments, result, is_error, duration_ms,
           '' AS file_path, '' AS action, '' AS summary,
           '' AS decision, '' AS reason,
           id AS row_num
    FROM journal_tool_calls WHERE session_id = $1` + afterCond + `
    UNION ALL
    SELECT 'file_change', created_at, '', '', '', false, 0,
           file_path, action, summary, '', '',
           id
    FROM journal_file_changes WHERE session_id = $1` + afterCond + `
    UNION ALL
    SELECT 'decision', created_at, '', '', '', false, 0,
           '', '', '', decision, reason,
           id
    FROM journal_decisions WHERE session_id = $1` + afterCond + `
) AS events
ORDER BY timestamp, type, row_num`

	var rows pgx.Rows
	var err error
	switch {
	case !after.IsZero() && limit > 0:
		rows, err = j.pool.Query(ctx, query+" LIMIT $3", sessionID, after, limit)
	case !after.IsZero():
		rows, err = j.pool.Query(ctx, query, sessionID, after)
	case limit > 0:
		rows, err = j.pool.Query(ctx, query+" LIMIT $2", sessionID, limit)
	default:
		rows, err = j.pool.Query(ctx, query, sessionID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []JournalEvent
	for rows.Next() {
		var e JournalEvent
		if err := rows.Scan(
			&e.Type, &e.Timestamp, &e.ToolName, &e.Arguments, &e.Result,
			&e.IsError, &e.DurationMs, &e.FilePath, &e.Action, &e.Summary,
			&e.Decision, &e.Reason,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetJournalStats 批量查询多个 session 的 journal 统计摘要
func (j *PGJournal) GetJournalStats(ctx context.Context, sessionIDs []string) (map[string]*JournalStats, error) {
	if len(sessionIDs) == 0 {
		return map[string]*JournalStats{}, nil
	}

	result := make(map[string]*JournalStats, len(sessionIDs))

	// 1. journal_sessions 时间信息
	sessRows, err := j.pool.Query(ctx,
		`SELECT session_id, started_at, ended_at FROM journal_sessions WHERE session_id = ANY($1)`,
		sessionIDs)
	if err != nil {
		return nil, err
	}
	for sessRows.Next() {
		var sid string
		s := &JournalStats{}
		if err := sessRows.Scan(&sid, &s.StartedAt, &s.EndedAt); err != nil {
			sessRows.Close()
			return nil, err
		}
		result[sid] = s
	}
	sessRows.Close()
	if err := sessRows.Err(); err != nil {
		return nil, err
	}

	// 2. tool_call 计数 + has_error
	tcRows, err := j.pool.Query(ctx,
		`SELECT session_id, COUNT(*), BOOL_OR(is_error) FROM journal_tool_calls WHERE session_id = ANY($1) GROUP BY session_id`,
		sessionIDs)
	if err != nil {
		return nil, err
	}
	for tcRows.Next() {
		var sid string
		var cnt int
		var hasErr bool
		if err := tcRows.Scan(&sid, &cnt, &hasErr); err != nil {
			tcRows.Close()
			return nil, err
		}
		if s, ok := result[sid]; ok {
			s.ToolCallCount = cnt
			s.HasError = hasErr
		}
	}
	tcRows.Close()
	if err := tcRows.Err(); err != nil {
		return nil, err
	}

	// 3. file_change 计数
	fcRows, err := j.pool.Query(ctx,
		`SELECT session_id, COUNT(*) FROM journal_file_changes WHERE session_id = ANY($1) GROUP BY session_id`,
		sessionIDs)
	if err != nil {
		return nil, err
	}
	for fcRows.Next() {
		var sid string
		var cnt int
		if err := fcRows.Scan(&sid, &cnt); err != nil {
			fcRows.Close()
			return nil, err
		}
		if s, ok := result[sid]; ok {
			s.FileChangeCount = cnt
		}
	}
	fcRows.Close()
	if err := fcRows.Err(); err != nil {
		return nil, err
	}

	// 4. decision 计数
	decRows, err := j.pool.Query(ctx,
		`SELECT session_id, reason FROM journal_decisions WHERE session_id = ANY($1)`,
		sessionIDs)
	if err != nil {
		return nil, err
	}
	for decRows.Next() {
		var sid string
		var reason string
		if err := decRows.Scan(&sid, &reason); err != nil {
			decRows.Close()
			return nil, err
		}
		if s, ok := result[sid]; ok {
			s.DecisionCount++
			qStats := qualityDecisionStatsFromReason(reason)
			if qStats.QualityError {
				s.QualityErrorCount++
				s.HasError = true
			}
			if qStats.Dangerous {
				s.DangerousCount++
			}
			if qStats.Delegation {
				s.DelegationCount++
			}
			if qStats.ACP {
				s.ACPCount++
			}
			if qStats.ContextPollution {
				s.ContextPollutionCount++
			}
		}
	}
	decRows.Close()
	if err := decRows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteSession 删除指定会话的所有日志数据（#6 级联清理）
func (j *PGJournal) DeleteSession(ctx context.Context, sessionID string) error {
	tx, err := j.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, table := range []string{"journal_tool_calls", "journal_file_changes", "journal_decisions"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE session_id = $1", sessionID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM journal_sessions WHERE session_id = $1", sessionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
