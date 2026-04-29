package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// PostgresConfig PostgreSQL 连接配置
// 注意：config.PostgresConfig 是用户侧配置结构体（JSON 反序列化），
// 此处是 store 包内部使用的连接参数，字段一致但包独立（避免 import cycle）。
type PostgresConfig struct {
	DSN      string // 完整连接串（优先）
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
	MaxConns int // 连接池大小，默认 10
}

// BuildDSN 根据配置构建 DSN 连接串
func (c PostgresConfig) BuildDSN() string {
	if c.DSN != "" {
		return c.DSN
	}
	host := c.Host
	if host == "" {
		host = "localhost"
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	db := c.Database
	if db == "" {
		db = "agents_claw"
	}
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "prefer"
	}

	dsn := fmt.Sprintf("host=%s port=%d dbname=%s sslmode=%s", host, port, db, sslMode)
	if c.User != "" {
		dsn += " user=" + c.User
	}
	if c.Password != "" {
		dsn += " password=" + c.Password
	}
	return dsn
}

// 编译期接口合规检查
var _ Store = (*PostgresStore)(nil)

// PostgresStore PostgreSQL 存储后端
type PostgresStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger

	mu       sync.Mutex
	handlers []func(key string)
	cancel   context.CancelFunc // 用于停止 LISTEN 协程
}

// Pool 返回底层连接池（用于 memory 等子模块共享连接）
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// NewPostgresStore 创建 PostgreSQL 存储
func NewPostgresStore(ctx context.Context, cfg PostgresConfig, logger *zap.Logger) (*PostgresStore, error) {
	dsn := cfg.BuildDSN()

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreError, "解析 PostgreSQL DSN 失败", err)
	}

	maxConns := cfg.MaxConns
	if maxConns <= 0 {
		maxConns = 10
	}
	poolCfg.MaxConns = int32(maxConns)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreError, "连接 PostgreSQL 失败", err)
	}

	// 测试连接
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errs.Wrap(errs.CodeStoreError, "PostgreSQL ping 失败", err)
	}

	// 执行迁移
	if err := pgMigrate(ctx, pool, logger); err != nil {
		pool.Close()
		return nil, errs.Wrap(errs.CodeStoreError, "PostgreSQL 迁移失败", err)
	}

	listenCtx, cancel := context.WithCancel(ctx)

	s := &PostgresStore{
		pool:   pool,
		logger: logger,
		cancel: cancel,
	}

	// 启动 LISTEN 协程
	go s.listenForNotifications(listenCtx)

	logger.Info("PostgreSQL 存储已初始化", zap.String("dsn", maskDSN(dsn)))
	return s, nil
}

// maskDSN 脱敏 DSN 中的密码
func maskDSN(dsn string) string {
	if idx := strings.Index(dsn, "password="); idx >= 0 {
		end := strings.IndexByte(dsn[idx:], ' ')
		if end < 0 {
			return dsn[:idx] + "password=***"
		}
		return dsn[:idx] + "password=***" + dsn[idx+end:]
	}
	return dsn
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// ---------------------------------------------------------------------------
// 会话 CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateSession(ctx context.Context, record *SessionRecord) error {
	tagsJSON, err := json.Marshal(record.Tags)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "序列化 tags 失败", err)
	}
	childrenJSON, err := json.Marshal(record.Children)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "序列化 children 失败", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO sessions (id, name, created_at, updated_at, last_accessed_at,
			message_count, total_tokens, profile_name, deleted,
			tags, parent_id, fork_point, children, user_id, is_starred)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		record.ID, record.Name, record.CreatedAt, record.UpdatedAt, record.LastAccessedAt,
		record.MessageCount, record.TotalTokens, "", boolToInt(record.Deleted),
		string(tagsJSON), record.ParentID, record.ForkPoint, string(childrenJSON),
		record.UserID, record.IsStarred,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return errs.New(errs.CodeStoreWriteFailed, "会话已存在: "+record.ID)
		}
		return errs.Wrap(errs.CodeStoreWriteFailed, "创建会话失败", err)
	}
	return nil
}

func (s *PostgresStore) SaveSession(ctx context.Context, record *SessionRecord) error {
	tagsJSON, err := json.Marshal(record.Tags)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "序列化 tags 失败", err)
	}
	childrenJSON, err := json.Marshal(record.Children)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "序列化 children 失败", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO sessions (id, name, created_at, updated_at, last_accessed_at,
			message_count, total_tokens, profile_name, deleted,
			tags, parent_id, fork_point, children, user_id, is_starred)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT(id) DO UPDATE SET
			name=EXCLUDED.name, updated_at=EXCLUDED.updated_at,
			last_accessed_at=EXCLUDED.last_accessed_at,
			total_tokens=EXCLUDED.total_tokens,
			profile_name=EXCLUDED.profile_name, deleted=EXCLUDED.deleted,
			parent_id=EXCLUDED.parent_id,
			fork_point=EXCLUDED.fork_point, children=EXCLUDED.children,
			user_id=CASE WHEN EXCLUDED.user_id != '' THEN EXCLUDED.user_id ELSE sessions.user_id END`,
		record.ID, record.Name, record.CreatedAt, record.UpdatedAt, record.LastAccessedAt,
		record.MessageCount, record.TotalTokens, "", boolToInt(record.Deleted),
		string(tagsJSON), record.ParentID, record.ForkPoint, string(childrenJSON), record.UserID, record.IsStarred,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "保存会话失败", err)
	}
	return nil
}

func (s *PostgresStore) LoadSession(ctx context.Context, sessionID string) (*SessionRecord, error) {
	var record SessionRecord
	var tagsJSON, childrenJSON string
	var deletedInt int
	var _profileName string

	err := s.pool.QueryRow(ctx,
		`SELECT id, name, created_at::text, updated_at::text, last_accessed_at::text,
			message_count, total_tokens, profile_name, deleted,
			tags, parent_id, fork_point, children, user_id, is_starred
		FROM sessions WHERE id = $1 AND deleted = 0`, sessionID,
	).Scan(
		&record.ID, &record.Name, &record.CreatedAt, &record.UpdatedAt, &record.LastAccessedAt,
		&record.MessageCount, &record.TotalTokens, &_profileName, &deletedInt,
		&tagsJSON, &record.ParentID, &record.ForkPoint, &childrenJSON,
		&record.UserID, &record.IsStarred,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "读取会话失败", err)
	}
	record.Deleted = deletedInt != 0

	if tagsJSON != "" {
		if err := json.Unmarshal([]byte(tagsJSON), &record.Tags); err != nil {
			s.logger.Warn("反序列化会话 tags 失败", zap.String("session_id", sessionID), zap.Error(err))
		}
	}
	if record.Tags == nil {
		record.Tags = []string{}
	}
	if childrenJSON != "" {
		if err := json.Unmarshal([]byte(childrenJSON), &record.Children); err != nil {
			s.logger.Warn("反序列化会话 children 失败", zap.String("session_id", sessionID), zap.Error(err))
		}
	}
	if record.Children == nil {
		record.Children = []string{}
	}

	return &record, nil
}

func (s *PostgresStore) DeleteSession(ctx context.Context, sessionID string) error {
	// messages 表有 ON DELETE CASCADE 外键，删除 sessions 记录时自动级联删除关联消息
	ct, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE id = $1", sessionID)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除会话失败", err)
	}
	if ct.RowsAffected() == 0 {
		return errs.New(errs.CodeStoreReadFailed, "会话未找到: "+sessionID)
	}
	return nil
}

func (s *PostgresStore) ListSessions(ctx context.Context) ([]*SessionRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, created_at::text, updated_at::text, last_accessed_at::text,
			message_count, total_tokens, profile_name, deleted,
			tags, parent_id, fork_point, children, user_id, is_starred
		FROM sessions WHERE deleted = 0
		ORDER BY last_accessed_at DESC`)
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询会话列表失败", err)
	}
	defer rows.Close()

	var records []*SessionRecord
	for rows.Next() {
		var record SessionRecord
		var tagsJSON, childrenJSON string
		var deletedInt int
		var _profileName string
		if err := rows.Scan(
			&record.ID, &record.Name, &record.CreatedAt, &record.UpdatedAt, &record.LastAccessedAt,
			&record.MessageCount, &record.TotalTokens, &_profileName, &deletedInt,
			&tagsJSON, &record.ParentID, &record.ForkPoint, &childrenJSON,
			&record.UserID, &record.IsStarred,
		); err != nil {
			s.logger.Warn("扫描会话记录失败", zap.Error(err))
			continue
		}
		record.Deleted = deletedInt != 0
		if tagsJSON != "" {
			if err := json.Unmarshal([]byte(tagsJSON), &record.Tags); err != nil {
				s.logger.Warn("反序列化会话 tags 失败", zap.String("session_id", record.ID), zap.Error(err))
			}
		}
		if record.Tags == nil {
			record.Tags = []string{}
		}
		if childrenJSON != "" {
			if err := json.Unmarshal([]byte(childrenJSON), &record.Children); err != nil {
				s.logger.Warn("反序列化会话 children 失败", zap.String("session_id", record.ID), zap.Error(err))
			}
		}
		if record.Children == nil {
			record.Children = []string{}
		}
		records = append(records, &record)
	}
	return records, rows.Err()
}

func (s *PostgresStore) ListSessionsByUser(ctx context.Context, userID string, _ bool) ([]*SessionRecord, error) {
	if userID == "" {
		return []*SessionRecord{}, nil
	}

	// 严格按 user_id 过滤，遗留无主 session 不可见
	query := `SELECT s.id, s.name, s.created_at::text, s.updated_at::text, s.last_accessed_at::text,
		s.message_count, s.total_tokens, s.deleted, s.tags, s.parent_id, s.fork_point, s.children,
		s.user_id, COALESCE(p.is_starred, false) AS is_starred
		FROM sessions s
		LEFT JOIN user_session_prefs p ON s.id = p.session_id AND p.user_id = $1
		WHERE s.deleted = 0 AND s.user_id = $1
		ORDER BY COALESCE(p.is_starred, false) DESC, s.last_accessed_at DESC`
	args := []any{userID}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询用户会话失败", err)
	}
	defer rows.Close()

	var records []*SessionRecord
	for rows.Next() {
		var r SessionRecord
		var tagsJSON, childrenJSON string
		var deletedInt int
		if err := rows.Scan(
			&r.ID, &r.Name, &r.CreatedAt, &r.UpdatedAt, &r.LastAccessedAt,
			&r.MessageCount, &r.TotalTokens, &deletedInt, &tagsJSON,
			&r.ParentID, &r.ForkPoint, &childrenJSON,
			&r.UserID, &r.IsStarred,
		); err != nil {
			return nil, errs.Wrap(errs.CodeStoreReadFailed, "扫描用户会话失败", err)
		}
		r.Deleted = deletedInt != 0
		if err := json.Unmarshal([]byte(tagsJSON), &r.Tags); err != nil {
			zap.L().Warn("session tags JSON 解析失败", zap.String("session_id", r.ID), zap.Error(err))
		}
		if err := json.Unmarshal([]byte(childrenJSON), &r.Children); err != nil {
			zap.L().Warn("session children JSON 解析失败", zap.String("session_id", r.ID), zap.Error(err))
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}

func (s *PostgresStore) GetLastActiveSession(ctx context.Context) (*SessionRecord, error) {
	var record SessionRecord
	var tagsJSON, childrenJSON string
	var deletedInt int
	var _profileName string

	err := s.pool.QueryRow(ctx,
		`SELECT id, name, created_at::text, updated_at::text, last_accessed_at::text,
			message_count, total_tokens, profile_name, deleted,
			tags, parent_id, fork_point, children
		FROM sessions WHERE deleted = 0 AND id NOT LIKE 'im-%'
		ORDER BY last_accessed_at DESC LIMIT 1`,
	).Scan(
		&record.ID, &record.Name, &record.CreatedAt, &record.UpdatedAt, &record.LastAccessedAt,
		&record.MessageCount, &record.TotalTokens, &_profileName, &deletedInt,
		&tagsJSON, &record.ParentID, &record.ForkPoint, &childrenJSON,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errs.New(errs.CodeStoreReadFailed, "未找到有效会话")
		}
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询最近会话失败", err)
	}
	record.Deleted = deletedInt != 0

	if tagsJSON != "" {
		if err := json.Unmarshal([]byte(tagsJSON), &record.Tags); err != nil {
			s.logger.Warn("反序列化会话 tags 失败", zap.String("session_id", record.ID), zap.Error(err))
		}
	}
	if record.Tags == nil {
		record.Tags = []string{}
	}
	if childrenJSON != "" {
		if err := json.Unmarshal([]byte(childrenJSON), &record.Children); err != nil {
			s.logger.Warn("反序列化会话 children 失败", zap.String("session_id", record.ID), zap.Error(err))
		}
	}
	if record.Children == nil {
		record.Children = []string{}
	}

	return &record, nil
}

// ---------------------------------------------------------------------------
// 消息 CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) AddMessage(ctx context.Context, sessionID, role, content string, metadata map[string]any) error {
	var metadataJSON []byte
	if metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return errs.Wrap(errs.CodeStoreWriteFailed, "序列化消息元数据失败", err)
		}
	}
	// 消息的 created_at：优先使用 metadata 中的原始时间（消息实际产生时间），
	// 避免批量保存时所有消息的 DB created_at 几乎相同导致前端排序失效
	now := time.Now()
	msgCreatedAt := now
	if metadata != nil {
		if origTS, ok := metadata["created_at"].(string); ok && origTS != "" {
			if parsed, parseErr := time.Parse(time.RFC3339, origTS); parseErr == nil {
				msgCreatedAt = parsed
			}
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "开始事务失败", err)
	}
	defer tx.Rollback(ctx)

	// metadata 列已迁移为 JSONB，created_at 已迁移为 TIMESTAMPTZ，直接传原生类型
	var metaArg any
	if len(metadataJSON) > 0 {
		metaArg = string(metadataJSON)
	}
	// tokens_in/tokens_out/cost 已废弃，成本数据迁移到 usage_records 表（P2-4）
	_, err = tx.Exec(ctx,
		`INSERT INTO messages (session_id, role, content, metadata, tokens_in, tokens_out, cost, created_at)
		VALUES ($1, $2, $3, $4, 0, 0, 0, $5)`,
		sessionID, role, content, metaArg, msgCreatedAt)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "写入消息失败", err)
	}

	// sessions 时间列用当前时间（会话级别的"最后更新"语义）
	_, err = tx.Exec(ctx,
		`UPDATE sessions SET message_count = message_count + 1, updated_at = $1, last_accessed_at = $2 WHERE id = $3`,
		now, now, sessionID)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "更新会话元数据失败", err)
	}

	return tx.Commit(ctx)
}

func (s *PostgresStore) GetMessages(ctx context.Context, sessionID string, limit int) ([]MessageRecord, error) {
	var query string
	var args []any

	if limit > 0 {
		query = `SELECT id, session_id, role, content, metadata, created_at
			FROM (
				SELECT id, session_id, role, content, metadata, created_at
				FROM messages WHERE session_id = $1
				ORDER BY id DESC LIMIT $2
			) sub ORDER BY id ASC`
		args = []any{sessionID, limit}
	} else {
		query = `SELECT id, session_id, role, content, metadata, created_at
			FROM messages WHERE session_id = $1 ORDER BY id ASC`
		args = []any{sessionID}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询消息失败", err)
	}
	defer rows.Close()

	var messages []MessageRecord
	for rows.Next() {
		var msg MessageRecord
		var metadataBytes []byte

		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &metadataBytes, &msg.CreatedAt); err != nil {
			s.logger.Warn("扫描消息记录失败", zap.Error(err))
			continue
		}

		if len(metadataBytes) > 0 {
			msg.Metadata = json.RawMessage(metadataBytes)
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (s *PostgresStore) ForkSession(ctx context.Context, parentID string, forkPoint int, newSessionID, newName, userID string) error {
	parent, err := s.LoadSession(ctx, parentID)
	if err != nil {
		return err
	}
	messages, err := s.GetMessages(ctx, parentID, 0)
	if err != nil {
		return err
	}
	if forkPoint < 0 || forkPoint > len(messages) {
		return errs.New(errs.CodeInvalidInput, fmt.Sprintf("无效的 fork 点: %d (消息数: %d)", forkPoint, len(messages)))
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "开始事务失败", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now()
	tagsJSON, err := json.Marshal(parent.Tags)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "序列化 fork 会话 tags 失败", err)
	}
	childrenJSON, err := json.Marshal([]string{})
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "序列化 fork 会话 children 失败", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO sessions (id, name, created_at, updated_at, last_accessed_at,
			message_count, total_tokens, profile_name, deleted,
			tags, parent_id, fork_point, children, user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		newSessionID, newName, now, now, now,
		forkPoint, 0, "", 0,
		string(tagsJSON), parentID, forkPoint, string(childrenJSON), userID,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "创建 fork 会话失败", err)
	}

	for i := 0; i < forkPoint && i < len(messages); i++ {
		msg := messages[i]
		var metaArg any
		if len(msg.Metadata) > 0 {
			metaArg = string(msg.Metadata)
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO messages (session_id, role, content, metadata, tokens_in, tokens_out, cost, created_at)
			VALUES ($1, $2, $3, $4, 0, 0, 0, $5)`,
			newSessionID, msg.Role, msg.Content, metaArg, msg.CreatedAt,
		)
		if err != nil {
			return errs.Wrap(errs.CodeStoreWriteFailed, "复制消息到 fork 会话失败", err)
		}
	}

	parent.Children = append(parent.Children, newSessionID)
	parentChildrenJSON, err := json.Marshal(parent.Children)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "序列化父会话 children 失败", err)
	}
	_, err = tx.Exec(ctx,
		"UPDATE sessions SET children = $1, updated_at = $2 WHERE id = $3",
		string(parentChildrenJSON), time.Now(), parentID)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "更新父会话 children 失败", err)
	}

	return tx.Commit(ctx)
}

func (s *PostgresStore) RevertSession(ctx context.Context, sessionID string, revertTo int) error {
	messages, err := s.GetMessages(ctx, sessionID, 0)
	if err != nil {
		return err
	}
	if revertTo < 0 || revertTo > len(messages) {
		return errs.New(errs.CodeInvalidInput, fmt.Sprintf("无效的回滚点: %d (消息数: %d)", revertTo, len(messages)))
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "开始事务失败", err)
	}
	defer tx.Rollback(ctx)

	if revertTo < len(messages) {
		cutoffID := messages[revertTo].ID
		_, err = tx.Exec(ctx,
			"DELETE FROM messages WHERE session_id = $1 AND id >= $2", sessionID, cutoffID)
		if err != nil {
			return errs.Wrap(errs.CodeStoreWriteFailed, "删除回滚消息失败", err)
		}
	}

	now := time.Now()
	_, err = tx.Exec(ctx,
		"UPDATE sessions SET message_count = $1, updated_at = $2, last_accessed_at = $3 WHERE id = $4",
		revertTo, now, now, sessionID)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "更新回滚后的会话元数据失败", err)
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// 收藏偏好 & 标签
// ---------------------------------------------------------------------------

func (s *PostgresStore) UpsertSessionPref(ctx context.Context, userID, sessionID string, starred bool) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_session_prefs (user_id, session_id, is_starred)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, session_id) DO UPDATE SET is_starred = $3, updated_at = NOW()`,
		userID, sessionID, starred)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "更新收藏状态失败", err)
	}
	return nil
}

func (s *PostgresStore) GetSessionStarred(ctx context.Context, userID, sessionID string) (bool, error) {
	var starred bool
	err := s.pool.QueryRow(ctx,
		`SELECT is_starred FROM user_session_prefs WHERE user_id = $1 AND session_id = $2`,
		userID, sessionID).Scan(&starred)
	if err == pgx.ErrNoRows {
		return false, nil // 无记录 = 未收藏
	}
	if err != nil {
		return false, errs.Wrap(errs.CodeStoreReadFailed, "查询收藏状态失败", err)
	}
	return starred, nil
}

func (s *PostgresStore) UpdateSessionTags(ctx context.Context, sessionID string, tags []string) error {
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "序列化 tags 失败", err)
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE sessions SET tags = $1, updated_at = NOW() WHERE id = $2`,
		string(tagsJSON), sessionID)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "更新标签失败", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 权限 CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) SaveGrant(ctx context.Context, rec *PermissionGrantRecord) error {
	var expiresAt *string
	if rec.ExpiresAt != "" {
		expiresAt = &rec.ExpiresAt
	}

	err := s.pool.QueryRow(ctx,
		`INSERT INTO permission_grants (tool, pattern, action, expires_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		rec.Tool, rec.Pattern, rec.Action, expiresAt,
	).Scan(&rec.ID)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "保存权限授予记录失败", err)
	}
	return nil
}

func (s *PostgresStore) LoadGrants(ctx context.Context) ([]PermissionGrantRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tool, pattern, action, created_at::text, COALESCE(expires_at::text, '')
		FROM permission_grants
		WHERE expires_at IS NULL OR expires_at > NOW()
		ORDER BY id ASC`)
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询权限授予记录失败", err)
	}
	defer rows.Close()

	var records []PermissionGrantRecord
	for rows.Next() {
		var rec PermissionGrantRecord
		if err := rows.Scan(&rec.ID, &rec.Tool, &rec.Pattern, &rec.Action, &rec.CreatedAt, &rec.ExpiresAt); err != nil {
			s.logger.Warn("扫描权限授予记录失败", zap.Error(err))
			continue
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (s *PostgresStore) DeleteGrant(ctx context.Context, id int64) error {
	ct, err := s.pool.Exec(ctx, "DELETE FROM permission_grants WHERE id = $1", id)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除权限授予记录失败", err)
	}
	if ct.RowsAffected() == 0 {
		return errs.New(errs.CodeStoreReadFailed, fmt.Sprintf("权限授予记录未找到: %d", id))
	}
	return nil
}

func (s *PostgresStore) DeleteAllGrants(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM permission_grants")
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除所有权限授予记录失败", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// OAuth Token CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) SaveOAuthToken(ctx context.Context, token *OAuthTokenRecord) error {
	if !isEncryptionEnabled() {
		s.logger.Warn("OAuth token 将以明文存储，建议设置 OAUTH_ENCRYPTION_KEY")
	}

	encAccessToken, err := encryptToken(token.AccessToken)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "加密 access_token 失败", err)
	}
	encRefreshToken, err := encryptToken(token.RefreshToken)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "加密 refresh_token 失败", err)
	}

	var expiresAt *string
	if token.ExpiresAt != "" {
		expiresAt = &token.ExpiresAt
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO oauth_tokens (server_url, access_token, refresh_token, token_type, scopes, expires_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT(server_url) DO UPDATE SET
			access_token=EXCLUDED.access_token, refresh_token=EXCLUDED.refresh_token,
			token_type=EXCLUDED.token_type, scopes=EXCLUDED.scopes,
			expires_at=EXCLUDED.expires_at, updated_at=NOW()`,
		token.ServerURL, encAccessToken, encRefreshToken, token.TokenType, token.Scopes, expiresAt,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "保存 OAuth token 失败", err)
	}
	return nil
}

func (s *PostgresStore) LoadOAuthToken(ctx context.Context, serverURL string) (*OAuthTokenRecord, error) {
	var record OAuthTokenRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, server_url, access_token, refresh_token, token_type, scopes,
			COALESCE(expires_at::text, ''), created_at::text, updated_at::text
		FROM oauth_tokens WHERE server_url = $1`, serverURL,
	).Scan(
		&record.ID, &record.ServerURL, &record.AccessToken, &record.RefreshToken,
		&record.TokenType, &record.Scopes, &record.ExpiresAt, &record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errs.New(errs.CodeStoreReadFailed, "OAuth token 未找到: "+serverURL)
		}
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "读取 OAuth token 失败", err)
	}

	if plain, err := decryptToken(record.AccessToken); err == nil {
		record.AccessToken = plain
	}
	if plain, err := decryptToken(record.RefreshToken); err == nil {
		record.RefreshToken = plain
	}

	return &record, nil
}

func (s *PostgresStore) DeleteOAuthToken(ctx context.Context, serverURL string) error {
	ct, err := s.pool.Exec(ctx, "DELETE FROM oauth_tokens WHERE server_url = $1", serverURL)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除 OAuth token 失败", err)
	}
	if ct.RowsAffected() == 0 {
		return errs.New(errs.CodeStoreReadFailed, "OAuth token 未找到: "+serverURL)
	}
	return nil
}

// ---------------------------------------------------------------------------
// LLM 提供商 CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) GetLLMProvider(ctx context.Context, name string) (*LLMProviderRecord, error) {
	var rec LLMProviderRecord
	err := s.pool.QueryRow(ctx,
		`SELECT name, provider_type, api_key, base_url, is_default, enabled, config_json, api_format, service_type,
			created_at::text, updated_at::text
		FROM llm_providers WHERE name = $1`, name,
	).Scan(&rec.Name, &rec.ProviderType, &rec.APIKey, &rec.BaseURL,
		&rec.IsDefault, &rec.Enabled, &rec.ConfigJSON, &rec.APIFormat, &rec.ServiceType,
		&rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errs.New(errs.CodeNotFound, "LLM 提供商未找到: "+name)
		}
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "读取 LLM 提供商失败", err)
	}
	return &rec, nil
}

func (s *PostgresStore) SaveLLMProvider(ctx context.Context, rec *LLMProviderRecord) error {
	rec.ConfigJSON = ensureValidJSON(rec.ConfigJSON, "{}")
	if rec.APIFormat == "" {
		rec.APIFormat = "chat"
	}
	if rec.ServiceType == "" {
		rec.ServiceType = "llm"
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO llm_providers (name, provider_type, api_key, base_url, is_default, enabled, config_json, api_format, service_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(name) DO UPDATE SET
			provider_type=EXCLUDED.provider_type, api_key=EXCLUDED.api_key,
			base_url=EXCLUDED.base_url, is_default=EXCLUDED.is_default,
			enabled=EXCLUDED.enabled, config_json=EXCLUDED.config_json,
			api_format=EXCLUDED.api_format, service_type=EXCLUDED.service_type,
			updated_at=NOW()`,
		rec.Name, rec.ProviderType, rec.APIKey, rec.BaseURL,
		rec.IsDefault, rec.Enabled, rec.ConfigJSON, rec.APIFormat, rec.ServiceType,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "保存 LLM 提供商失败", err)
	}
	if _, err := s.pool.Exec(ctx, "SELECT pg_notify('config_change', $1)", "llm_provider:"+rec.Name); err != nil {
		s.logger.Warn("发送 LLM 提供商配置变更通知失败", zap.String("name", rec.Name), zap.Error(err))
	}
	return nil
}

func (s *PostgresStore) DeleteLLMProvider(ctx context.Context, name string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "开启事务失败", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 级联删除关联的模型
	if _, err := tx.Exec(ctx, "DELETE FROM llm_models WHERE provider_name = $1", name); err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除关联 LLM 模型失败", err)
	}
	ct, err := tx.Exec(ctx, "DELETE FROM llm_providers WHERE name = $1", name)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除 LLM 提供商失败", err)
	}
	if ct.RowsAffected() == 0 {
		return errs.New(errs.CodeNotFound, "LLM 提供商未找到: "+name)
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ListLLMProviders(ctx context.Context) ([]*LLMProviderRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT name, provider_type, api_key, base_url, is_default, enabled, config_json, api_format, service_type,
			created_at::text, updated_at::text
		FROM llm_providers ORDER BY name`)
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询 LLM 提供商列表失败", err)
	}
	defer rows.Close()

	var records []*LLMProviderRecord
	for rows.Next() {
		var rec LLMProviderRecord
		if err := rows.Scan(&rec.Name, &rec.ProviderType, &rec.APIKey, &rec.BaseURL,
			&rec.IsDefault, &rec.Enabled, &rec.ConfigJSON, &rec.APIFormat, &rec.ServiceType,
			&rec.CreatedAt, &rec.UpdatedAt); err != nil {
			s.logger.Warn("扫描 LLM 提供商记录失败", zap.Error(err))
			continue
		}
		records = append(records, &rec)
	}
	return records, rows.Err()
}

// ---------------------------------------------------------------------------
// LLM 模型 CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) GetLLMModel(ctx context.Context, name string) (*LLMModelRecord, error) {
	var rec LLMModelRecord
	err := s.pool.QueryRow(ctx,
		`SELECT name, provider_name, model, base_url, api_key, is_default, enabled, service_type, config_json,
			created_at::text, updated_at::text
		FROM llm_models WHERE name = $1`, name,
	).Scan(&rec.Name, &rec.ProviderName, &rec.Model, &rec.BaseURL, &rec.APIKey,
		&rec.IsDefault, &rec.Enabled, &rec.ServiceType, &rec.ConfigJSON,
		&rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errs.New(errs.CodeNotFound, "LLM 模型未找到: "+name)
		}
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "读取 LLM 模型失败", err)
	}
	return &rec, nil
}

func (s *PostgresStore) SaveLLMModel(ctx context.Context, rec *LLMModelRecord) error {
	rec.ConfigJSON = ensureValidJSON(rec.ConfigJSON, "{}")
	_, err := s.pool.Exec(ctx,
		`INSERT INTO llm_models (name, provider_name, model, base_url, api_key, is_default, enabled, service_type, config_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(name) DO UPDATE SET
			provider_name=EXCLUDED.provider_name, model=EXCLUDED.model,
			base_url=EXCLUDED.base_url, api_key=EXCLUDED.api_key,
			is_default=EXCLUDED.is_default, enabled=EXCLUDED.enabled,
			service_type=EXCLUDED.service_type,
			config_json=EXCLUDED.config_json, updated_at=NOW()`,
		rec.Name, rec.ProviderName, rec.Model, rec.BaseURL, rec.APIKey,
		rec.IsDefault, rec.Enabled, rec.ServiceType, rec.ConfigJSON,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "保存 LLM 模型失败", err)
	}
	if _, err := s.pool.Exec(ctx, "SELECT pg_notify('config_change', $1)", "llm_model:"+rec.Name); err != nil {
		s.logger.Warn("发送 LLM 模型配置变更通知失败", zap.String("name", rec.Name), zap.Error(err))
	}
	return nil
}

func (s *PostgresStore) DeleteLLMModel(ctx context.Context, name string) error {
	ct, err := s.pool.Exec(ctx, "DELETE FROM llm_models WHERE name = $1", name)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除 LLM 模型失败", err)
	}
	if ct.RowsAffected() == 0 {
		return errs.New(errs.CodeNotFound, "LLM 模型未找到: "+name)
	}
	return nil
}

func (s *PostgresStore) ListLLMModels(ctx context.Context) ([]*LLMModelRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT name, provider_name, model, base_url, api_key, is_default, enabled, service_type, config_json,
			created_at::text, updated_at::text
		FROM llm_models ORDER BY name`)
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询 LLM 模型列表失败", err)
	}
	defer rows.Close()

	var records []*LLMModelRecord
	for rows.Next() {
		var rec LLMModelRecord
		if err := rows.Scan(&rec.Name, &rec.ProviderName, &rec.Model, &rec.BaseURL, &rec.APIKey,
			&rec.IsDefault, &rec.Enabled, &rec.ServiceType, &rec.ConfigJSON,
			&rec.CreatedAt, &rec.UpdatedAt); err != nil {
			s.logger.Warn("扫描 LLM 模型记录失败", zap.Error(err))
			continue
		}
		records = append(records, &rec)
	}
	return records, rows.Err()
}

// SetDefaultLLMProvider 原子化地将指定 Provider 设为默认，同时清除其他 Provider 的默认标记。
// 使用单个事务保证原子性，避免并发请求导致多个 Provider 同时为默认。
func (s *PostgresStore) SetDefaultLLMProvider(ctx context.Context, name string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "开启事务失败", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, "UPDATE llm_providers SET is_default=false, updated_at=NOW() WHERE is_default=true AND name != $1", name); err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "清除 LLM Provider 默认标记失败", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE llm_providers SET is_default=true, updated_at=NOW() WHERE name = $1", name); err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "设置 LLM Provider 默认标记失败", err)
	}
	return tx.Commit(ctx)
}

// SetDefaultLLMModel 原子化地将指定 Model 设为默认，同时清除其他 Model 的默认标记。
func (s *PostgresStore) SetDefaultLLMModel(ctx context.Context, name string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "开启事务失败", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, "UPDATE llm_models SET is_default=false, updated_at=NOW() WHERE is_default=true AND name != $1", name); err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "清除 LLM Model 默认标记失败", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE llm_models SET is_default=true, updated_at=NOW() WHERE name = $1", name); err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "设置 LLM Model 默认标记失败", err)
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// 配置键值表 CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) GetConfig(ctx context.Context, key string) (string, error) {
	var value string
	err := s.pool.QueryRow(ctx, "SELECT value FROM configs WHERE key = $1", key).Scan(&value)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", errs.New(errs.CodeStoreReadFailed, "配置项未找到: "+key)
		}
		return "", errs.Wrap(errs.CodeStoreReadFailed, "读取配置失败", err)
	}
	return value, nil
}

func (s *PostgresStore) SetConfig(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO configs (key, value, updated_at) VALUES ($1, $2, NOW())
		ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value, updated_at=NOW()`, key, value)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "写入配置失败", err)
	}
	// 发送 PG NOTIFY
	if _, err := s.pool.Exec(ctx, "SELECT pg_notify('config_change', $1)", key); err != nil {
		s.logger.Warn("发送配置变更通知失败", zap.String("key", key), zap.Error(err))
	}
	return nil
}

func (s *PostgresStore) GetAllConfig(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, "SELECT key, value FROM configs")
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询全部配置失败", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			s.logger.Warn("扫描配置记录失败", zap.Error(err))
			continue
		}
		result[k] = v
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// IM 通道配置 CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) GetChannelConfig(ctx context.Context, platform string) (*ChannelConfigRecord, error) {
	var rec ChannelConfigRecord
	err := s.pool.QueryRow(ctx,
		"SELECT platform, enabled, config_json, updated_at FROM channel_configs WHERE platform = $1", platform,
	).Scan(&rec.Platform, &rec.Enabled, &rec.ConfigJSON, &rec.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errs.New(errs.CodeStoreReadFailed, "通道配置未找到: "+platform)
		}
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "读取通道配置失败", err)
	}
	return &rec, nil
}

func (s *PostgresStore) SaveChannelConfig(ctx context.Context, rec *ChannelConfigRecord) error {
	rec.ConfigJSON = ensureValidJSON(rec.ConfigJSON, "{}")
	_, err := s.pool.Exec(ctx,
		`INSERT INTO channel_configs (platform, enabled, config_json, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT(platform) DO UPDATE SET
			enabled=EXCLUDED.enabled, config_json=EXCLUDED.config_json, updated_at=NOW()`,
		rec.Platform, rec.Enabled, rec.ConfigJSON,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "保存通道配置失败", err)
	}
	if _, err := s.pool.Exec(ctx, "SELECT pg_notify('config_change', $1)", "channel:"+rec.Platform); err != nil {
		s.logger.Warn("发送通道配置变更通知失败", zap.String("platform", rec.Platform), zap.Error(err))
	}
	return nil
}

func (s *PostgresStore) ListChannelConfigs(ctx context.Context) ([]*ChannelConfigRecord, error) {
	rows, err := s.pool.Query(ctx,
		"SELECT platform, enabled, config_json, updated_at FROM channel_configs ORDER BY platform")
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询通道配置列表失败", err)
	}
	defer rows.Close()

	var records []*ChannelConfigRecord
	for rows.Next() {
		var rec ChannelConfigRecord
		if err := rows.Scan(&rec.Platform, &rec.Enabled, &rec.ConfigJSON, &rec.UpdatedAt); err != nil {
			s.logger.Warn("扫描通道配置记录失败", zap.Error(err))
			continue
		}
		records = append(records, &rec)
	}
	return records, rows.Err()
}

func (s *PostgresStore) SaveScheduledPush(ctx context.Context, rec *ScheduledPushRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO scheduled_pushes (id, name, platform, prompt, interval_sec, enabled, created_by, last_run_at, next_run_at, last_error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		ON CONFLICT(id) DO UPDATE SET
			name=EXCLUDED.name,
			platform=EXCLUDED.platform,
			prompt=EXCLUDED.prompt,
			interval_sec=EXCLUDED.interval_sec,
			enabled=EXCLUDED.enabled,
			created_by=EXCLUDED.created_by,
			last_run_at=EXCLUDED.last_run_at,
			next_run_at=EXCLUDED.next_run_at,
			last_error=EXCLUDED.last_error,
			updated_at=NOW()`,
		rec.ID, rec.Name, rec.Platform, rec.Prompt, rec.IntervalSec, rec.Enabled, rec.CreatedBy, nullableTime(rec.LastRunAt), nullableTime(rec.NextRunAt), rec.LastError,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "保存定时推送失败", err)
	}
	return nil
}

func (s *PostgresStore) GetScheduledPush(ctx context.Context, id string) (*ScheduledPushRecord, error) {
	var rec ScheduledPushRecord
	var lastRunAt, nextRunAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, platform, prompt, interval_sec, enabled, created_by, last_run_at, next_run_at, last_error, created_at, updated_at
		FROM scheduled_pushes WHERE id = $1`, id,
	).Scan(&rec.ID, &rec.Name, &rec.Platform, &rec.Prompt, &rec.IntervalSec, &rec.Enabled, &rec.CreatedBy, &lastRunAt, &nextRunAt, &rec.LastError, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "读取定时推送失败", err)
	}
	if lastRunAt != nil {
		rec.LastRunAt = *lastRunAt
	}
	if nextRunAt != nil {
		rec.NextRunAt = *nextRunAt
	}
	return &rec, nil
}

func (s *PostgresStore) DeleteScheduledPush(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, "DELETE FROM scheduled_pushes WHERE id = $1", id)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除定时推送失败", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListScheduledPushes(ctx context.Context, platform string) ([]*ScheduledPushRecord, error) {
	query := `SELECT id, name, platform, prompt, interval_sec, enabled, created_by, last_run_at, next_run_at, last_error, created_at, updated_at
		FROM scheduled_pushes`
	args := []any{}
	if platform != "" {
		query += ` WHERE platform = $1`
		args = append(args, platform)
	}
	query += ` ORDER BY created_at ASC, id ASC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询定时推送列表失败", err)
	}
	defer rows.Close()

	var records []*ScheduledPushRecord
	for rows.Next() {
		var rec ScheduledPushRecord
		var lastRunAt, nextRunAt *time.Time
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Platform, &rec.Prompt, &rec.IntervalSec, &rec.Enabled, &rec.CreatedBy, &lastRunAt, &nextRunAt, &rec.LastError, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			s.logger.Warn("扫描定时推送记录失败", zap.Error(err))
			continue
		}
		if lastRunAt != nil {
			rec.LastRunAt = *lastRunAt
		}
		if nextRunAt != nil {
			rec.NextRunAt = *nextRunAt
		}
		records = append(records, &rec)
	}
	return records, rows.Err()
}

func (s *PostgresStore) UpdateScheduledPushRun(ctx context.Context, id string, lastRunAt, nextRunAt time.Time, lastError string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE scheduled_pushes
		SET last_run_at = $2, next_run_at = $3, last_error = $4, updated_at = NOW()
		WHERE id = $1`,
		id, nullableTime(lastRunAt), nullableTime(nextRunAt), lastError,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "更新定时推送运行状态失败", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// MCP 服务端配置 CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) GetMCPServer(ctx context.Context, name string) (*MCPServerRecord, error) {
	var rec MCPServerRecord
	err := s.pool.QueryRow(ctx,
		"SELECT name, transport, command, args, env, url, headers, timeout, enabled, updated_at FROM mcp_servers WHERE name = $1", name,
	).Scan(&rec.Name, &rec.Transport, &rec.Command, &rec.Args, &rec.Env, &rec.URL, &rec.Headers, &rec.Timeout, &rec.Enabled, &rec.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errs.New(errs.CodeStoreReadFailed, "MCP 服务端配置未找到: "+name)
		}
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "读取 MCP 服务端配置失败", err)
	}
	return &rec, nil
}

func (s *PostgresStore) SaveMCPServer(ctx context.Context, rec *MCPServerRecord) error {
	rec.Args = ensureValidJSON(rec.Args, "[]")
	rec.Env = ensureValidJSON(rec.Env, "{}")
	rec.Headers = ensureValidJSON(rec.Headers, "{}")
	_, err := s.pool.Exec(ctx,
		`INSERT INTO mcp_servers (name, transport, command, args, env, url, headers, timeout, enabled, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT(name) DO UPDATE SET
			transport=EXCLUDED.transport, command=EXCLUDED.command, args=EXCLUDED.args,
			env=EXCLUDED.env, url=EXCLUDED.url, headers=EXCLUDED.headers, timeout=EXCLUDED.timeout,
			enabled=EXCLUDED.enabled, updated_at=NOW()`,
		rec.Name, rec.Transport, rec.Command, rec.Args, rec.Env, rec.URL, rec.Headers, rec.Timeout, rec.Enabled,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "保存 MCP 服务端配置失败", err)
	}
	if _, err := s.pool.Exec(ctx, "SELECT pg_notify('config_change', $1)", "mcp:"+rec.Name); err != nil {
		s.logger.Warn("发送 MCP 服务端配置变更通知失败", zap.String("name", rec.Name), zap.Error(err))
	}
	return nil
}

func (s *PostgresStore) DeleteMCPServer(ctx context.Context, name string) error {
	ct, err := s.pool.Exec(ctx, "DELETE FROM mcp_servers WHERE name = $1", name)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除 MCP 服务端配置失败", err)
	}
	if ct.RowsAffected() == 0 {
		return errs.New(errs.CodeStoreReadFailed, "MCP 服务端配置未找到: "+name)
	}
	if _, err := s.pool.Exec(ctx, "SELECT pg_notify('config_change', $1)", "mcp:"+name); err != nil {
		s.logger.Warn("发送 MCP 服务端配置变更通知失败", zap.String("name", name), zap.Error(err))
	}
	return nil
}

func (s *PostgresStore) ListMCPServers(ctx context.Context) ([]*MCPServerRecord, error) {
	rows, err := s.pool.Query(ctx,
		"SELECT name, transport, command, args, env, url, headers, timeout, enabled, updated_at FROM mcp_servers ORDER BY name")
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询 MCP 服务端配置列表失败", err)
	}
	defer rows.Close()

	var records []*MCPServerRecord
	for rows.Next() {
		var rec MCPServerRecord
		if err := rows.Scan(&rec.Name, &rec.Transport, &rec.Command, &rec.Args, &rec.Env, &rec.URL, &rec.Headers, &rec.Timeout, &rec.Enabled, &rec.UpdatedAt); err != nil {
			s.logger.Warn("扫描 MCP 服务端配置记录失败", zap.Error(err))
			continue
		}
		records = append(records, &rec)
	}
	return records, rows.Err()
}

// ---------------------------------------------------------------------------
// 外部资源配置 CRUD
// ---------------------------------------------------------------------------

func (s *PostgresStore) GetExternalResource(ctx context.Context, name string) (*ExternalResourceRecord, error) {
	var rec ExternalResourceRecord
	err := s.pool.QueryRow(ctx,
		"SELECT name, type, environment, description, connection, endpoint, credentials, read_only, enabled, updated_at FROM external_resources WHERE name = $1", name,
	).Scan(&rec.Name, &rec.Type, &rec.Environment, &rec.Description, &rec.Connection, &rec.Endpoint, &rec.Credentials, &rec.ReadOnly, &rec.Enabled, &rec.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errs.New(errs.CodeStoreReadFailed, "外部资源配置未找到: "+name)
		}
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "读取外部资源配置失败", err)
	}
	return &rec, nil
}

func (s *PostgresStore) SaveExternalResource(ctx context.Context, rec *ExternalResourceRecord) error {
	rec.Credentials = ensureValidJSON(rec.Credentials, "{}")
	_, err := s.pool.Exec(ctx,
		`INSERT INTO external_resources (name, type, environment, description, connection, endpoint, credentials, read_only, enabled, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT(name) DO UPDATE SET
			type=EXCLUDED.type, environment=EXCLUDED.environment, description=EXCLUDED.description,
			connection=EXCLUDED.connection, endpoint=EXCLUDED.endpoint, credentials=EXCLUDED.credentials,
			read_only=EXCLUDED.read_only, enabled=EXCLUDED.enabled, updated_at=NOW()`,
		rec.Name, rec.Type, rec.Environment, rec.Description, rec.Connection, rec.Endpoint, rec.Credentials, rec.ReadOnly, rec.Enabled,
	)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "保存外部资源配置失败", err)
	}
	if _, err := s.pool.Exec(ctx, "SELECT pg_notify('config_change', $1)", "resource:"+rec.Name); err != nil {
		s.logger.Warn("发送外部资源配置变更通知失败", zap.String("name", rec.Name), zap.Error(err))
	}
	return nil
}

func (s *PostgresStore) DeleteExternalResource(ctx context.Context, name string) error {
	ct, err := s.pool.Exec(ctx, "DELETE FROM external_resources WHERE name = $1", name)
	if err != nil {
		return errs.Wrap(errs.CodeStoreWriteFailed, "删除外部资源配置失败", err)
	}
	if ct.RowsAffected() == 0 {
		return errs.New(errs.CodeStoreReadFailed, "外部资源配置未找到: "+name)
	}
	if _, err := s.pool.Exec(ctx, "SELECT pg_notify('config_change', $1)", "resource:"+name); err != nil {
		s.logger.Warn("发送外部资源配置变更通知失败", zap.String("name", name), zap.Error(err))
	}
	return nil
}

func (s *PostgresStore) ListExternalResources(ctx context.Context) ([]*ExternalResourceRecord, error) {
	rows, err := s.pool.Query(ctx,
		"SELECT name, type, environment, description, connection, endpoint, credentials, read_only, enabled, updated_at FROM external_resources ORDER BY name")
	if err != nil {
		return nil, errs.Wrap(errs.CodeStoreReadFailed, "查询外部资源配置列表失败", err)
	}
	defer rows.Close()

	var records []*ExternalResourceRecord
	for rows.Next() {
		var rec ExternalResourceRecord
		if err := rows.Scan(&rec.Name, &rec.Type, &rec.Environment, &rec.Description, &rec.Connection, &rec.Endpoint, &rec.Credentials, &rec.ReadOnly, &rec.Enabled, &rec.UpdatedAt); err != nil {
			s.logger.Warn("扫描外部资源配置记录失败", zap.Error(err))
			continue
		}
		records = append(records, &rec)
	}
	return records, rows.Err()
}

// ---------------------------------------------------------------------------
// 配置变更通知（PG LISTEN/NOTIFY）
// ---------------------------------------------------------------------------

func (s *PostgresStore) OnConfigChange(handler func(key string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = append(s.handlers, handler)
}

// listenForNotifications 监听 PG LISTEN/NOTIFY 通道
func (s *PostgresStore) listenForNotifications(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		// 获取独立连接用于 LISTEN
		conn, err := s.pool.Acquire(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Warn("获取 LISTEN 连接失败，5 秒后重试", zap.Error(err))
			time.Sleep(5 * time.Second)
			continue
		}

		_, err = conn.Exec(ctx, "LISTEN config_change")
		if err != nil {
			conn.Release()
			s.logger.Warn("执行 LISTEN 失败，5 秒后重试", zap.Error(err))
			time.Sleep(5 * time.Second)
			continue
		}

		s.logger.Debug("PG LISTEN config_change 已启动")

		// 持续等待通知
		for {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				if ctx.Err() != nil {
					conn.Release()
					return
				}
				s.logger.Warn("等待 PG 通知失败，重新连接", zap.Error(err))
				break
			}

			key := notification.Payload
			s.logger.Debug("收到配置变更通知", zap.String("key", key))

			s.mu.Lock()
			handlers := make([]func(string), len(s.handlers))
			copy(handlers, s.handlers)
			s.mu.Unlock()

			for _, h := range handlers {
				go h(key)
			}
		}

		conn.Release()
		time.Sleep(time.Second)
	}
}

// Close 关闭连接池
func (s *PostgresStore) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
}

// boolToInt 将 bool 转换为 int（PG 表中 deleted 列定义为 INTEGER）
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ensureValidJSON 确保字符串是合法 JSON，否则返回默认值
// 用于写入 PG JSONB 列前的校验，防止非法 JSON 导致 PG 报错
func ensureValidJSON(s, fallback string) string {
	if json.Valid([]byte(s)) {
		return s
	}
	return fallback
}
