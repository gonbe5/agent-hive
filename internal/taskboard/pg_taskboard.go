package taskboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// pgTaskBoardInitSQL 建表 SQL
const pgTaskBoardInitSQL = `
CREATE TABLE IF NOT EXISTS taskboard_tasks (
	id          BIGSERIAL PRIMARY KEY,
	session_id  TEXT NOT NULL DEFAULT '',
	title       TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL DEFAULT 'pending',
	priority    TEXT NOT NULL DEFAULT 'medium',
	assignee    TEXT NOT NULL DEFAULT '',
	parent_id   BIGINT NOT NULL DEFAULT 0,
	tags        JSONB NOT NULL DEFAULT '[]',
	created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_taskboard_session ON taskboard_tasks(session_id);
CREATE INDEX IF NOT EXISTS idx_taskboard_status ON taskboard_tasks(status);
CREATE INDEX IF NOT EXISTS idx_taskboard_parent ON taskboard_tasks(parent_id) WHERE parent_id != 0;
`

// PGTaskBoard 基于 PostgreSQL 的 TaskBoard 实现（生产环境）。
type PGTaskBoard struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewPGTaskBoard 创建 PGTaskBoard 并执行建表迁移。
func NewPGTaskBoard(ctx context.Context, pool *pgxpool.Pool, logger *zap.Logger) (*PGTaskBoard, error) {
	if _, err := pool.Exec(ctx, pgTaskBoardInitSQL); err != nil {
		return nil, fmt.Errorf("taskboard 建表失败: %w", err)
	}
	logger.Info("taskboard 表已初始化")
	return &PGTaskBoard{pool: pool, logger: logger}, nil
}

func (b *PGTaskBoard) Create(ctx context.Context, task *Task) (string, error) {
	t := copyTaskValue(task)

	if err := validateStatus(t.Status); t.Status != "" && err != nil {
		return "", err
	}
	if err := validatePriority(t.Priority); t.Priority != "" && err != nil {
		return "", err
	}

	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = StatusPending
	}
	if t.Priority == "" {
		t.Priority = PriorityMedium
	}

	tagsJSON, _ := json.Marshal(t.Tags)
	if t.Tags == nil {
		tagsJSON = []byte("[]")
	}

	parentID := parseID(t.ParentID) // "" -> 0, "task-5" -> 5

	var id int64
	err := b.pool.QueryRow(ctx,
		`INSERT INTO taskboard_tasks (session_id, title, description, status, priority, assignee, parent_id, tags, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id`,
		t.SessionID, t.Title, t.Description,
		string(t.Status), string(t.Priority), t.Assignee, parentID,
		tagsJSON, t.CreatedAt, t.UpdatedAt).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("taskboard create: %w", err)
	}
	return fmt.Sprintf("task-%d", id), nil
}

func (b *PGTaskBoard) Get(ctx context.Context, id string) (*Task, error) {
	dbID, err := mustParseID(id)
	if err != nil {
		return nil, err
	}

	t := &Task{}
	var parentID int64
	var tagsJSON []byte
	err = b.pool.QueryRow(ctx,
		`SELECT id, session_id, title, description, status, priority, assignee, parent_id, tags, created_at, updated_at
		 FROM taskboard_tasks WHERE id = $1`, dbID).
		Scan(&dbID, &t.SessionID, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.Assignee, &parentID,
			&tagsJSON, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("task %q not found", id)
		}
		return nil, fmt.Errorf("taskboard get: %w", err)
	}
	t.ID = fmt.Sprintf("task-%d", dbID)
	if parentID != 0 {
		t.ParentID = fmt.Sprintf("task-%d", parentID)
	}
	_ = json.Unmarshal(tagsJSON, &t.Tags)
	return t, nil
}

func (b *PGTaskBoard) Update(ctx context.Context, id string, patch TaskPatch) error {
	dbID, err := mustParseID(id)
	if err != nil {
		return err
	}

	if patch.Status != nil {
		if err := validateStatus(*patch.Status); err != nil {
			return err
		}
	}
	if patch.Priority != nil {
		if err := validatePriority(*patch.Priority); err != nil {
			return err
		}
	}

	var setClauses []string
	var args []any
	argIdx := 1

	if patch.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *patch.Title)
		argIdx++
	}
	if patch.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *patch.Description)
		argIdx++
	}
	if patch.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*patch.Status))
		argIdx++
	}
	if patch.Priority != nil {
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, string(*patch.Priority))
		argIdx++
	}
	if patch.Assignee != nil {
		setClauses = append(setClauses, fmt.Sprintf("assignee = $%d", argIdx))
		args = append(args, *patch.Assignee)
		argIdx++
	}
	if patch.Tags != nil {
		tagsJSON, _ := json.Marshal(patch.Tags)
		setClauses = append(setClauses, fmt.Sprintf("tags = $%d", argIdx))
		args = append(args, tagsJSON)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, dbID)

	query := fmt.Sprintf("UPDATE taskboard_tasks SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), argIdx)

	tag, err := b.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("taskboard update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("task %q not found", id)
	}
	return nil
}

func (b *PGTaskBoard) List(ctx context.Context, filter TaskFilter) ([]*Task, error) {
	var whereClauses []string
	var args []any
	argIdx := 1

	if filter.SessionID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("session_id = $%d", argIdx))
		args = append(args, filter.SessionID)
		argIdx++
	}
	if filter.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(filter.Status))
		argIdx++
	}
	if filter.Priority != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, string(filter.Priority))
		argIdx++
	}
	if filter.Assignee != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("assignee = $%d", argIdx))
		args = append(args, filter.Assignee)
		argIdx++
	}
	if filter.ParentID != "" {
		pid, err := mustParseID(filter.ParentID)
		if err != nil {
			return nil, err
		}
		whereClauses = append(whereClauses, fmt.Sprintf("parent_id = $%d", argIdx))
		args = append(args, pid)
		argIdx++
	}
	if len(filter.Tags) > 0 {
		// JSONB 数组包含任一标签: tags ?| array['tag1','tag2']
		whereClauses = append(whereClauses, fmt.Sprintf("tags ?| $%d", argIdx))
		args = append(args, filter.Tags)
		argIdx++
	}

	query := "SELECT id, session_id, title, description, status, priority, assignee, parent_id, tags, created_at, updated_at FROM taskboard_tasks"
	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	query += " ORDER BY created_at, id"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filter.Limit)
		argIdx++
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filter.Offset)
		argIdx++
	}

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("taskboard list: %w", err)
	}
	defer rows.Close()

	var result []*Task
	for rows.Next() {
		t := &Task{}
		var dbID int64
		var parentID int64
		var tagsJSON []byte
		if err := rows.Scan(&dbID, &t.SessionID, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.Assignee, &parentID,
			&tagsJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.ID = fmt.Sprintf("task-%d", dbID)
		if parentID != 0 {
			t.ParentID = fmt.Sprintf("task-%d", parentID)
		}
		_ = json.Unmarshal(tagsJSON, &t.Tags)
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Delete 删除任务及其所有子任务（级联）。
func (b *PGTaskBoard) Delete(ctx context.Context, id string) error {
	dbID, err := mustParseID(id)
	if err != nil {
		return err
	}

	// 递归 CTE：收集自身及所有后代（任意深度），一次删除
	const deleteSQL = `
WITH RECURSIVE descendants AS (
	SELECT id FROM taskboard_tasks WHERE id = $1
	UNION ALL
	SELECT t.id FROM taskboard_tasks t
	INNER JOIN descendants d ON t.parent_id = d.id
)
DELETE FROM taskboard_tasks WHERE id IN (SELECT id FROM descendants)`

	tag, err := b.pool.Exec(ctx, deleteSQL, dbID)
	if err != nil {
		return fmt.Errorf("taskboard delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("task %q not found", id)
	}
	return nil
}

// parseID 从 "task-123" 格式提取数字 ID，空字符串返回 0，无效格式返回 0。
func parseID(id string) int64 {
	if id == "" {
		return 0
	}
	var n int64
	fmt.Sscanf(id, "task-%d", &n)
	return n
}

// mustParseID 从 "task-123" 格式提取数字 ID，无效格式返回错误。
func mustParseID(id string) (int64, error) {
	var n int64
	if _, err := fmt.Sscanf(id, "task-%d", &n); err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid task ID: %q", id)
	}
	return n, nil
}

// copyTaskValue 值拷贝 Task（避免修改调用方入参）
func copyTaskValue(src *Task) Task {
	t := *src
	if src.Tags != nil {
		t.Tags = make([]string, len(src.Tags))
		copy(t.Tags, src.Tags)
	}
	return t
}
