package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore PostgreSQL 实现的 auth Store
type PGStore struct {
	pool *pgxpool.Pool
}

// NewPGStore 创建 PGStore
func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

// ListEnabledProviders 查询所有已启用的 provider
func (s *PGStore) ListEnabledProviders(ctx context.Context) ([]ProviderConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT name, provider_type, enabled, config_json FROM auth_providers WHERE enabled = TRUE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []ProviderConfig
	for rows.Next() {
		var p ProviderConfig
		var cfgJSON []byte
		if err := rows.Scan(&p.Name, &p.ProviderType, &p.Enabled, &cfgJSON); err != nil {
			return nil, err
		}
		p.ConfigJSON = json.RawMessage(cfgJSON)
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

// FindUserByExternalID 按 external_id + provider 查找用户
func (s *PGStore) FindUserByExternalID(ctx context.Context, externalID, provider string) (*User, error) {
	u := &User{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, external_id, auth_provider, display_name, email, avatar_url, department,
		        role, status, last_login_at, last_login_ip, created_at, updated_at
		 FROM users WHERE external_id = $1 AND auth_provider = $2`,
		externalID, provider,
	).Scan(&u.ID, &u.ExternalID, &u.AuthProvider, &u.DisplayName, &u.Email,
		&u.AvatarURL, &u.Department, &u.Role, &u.Status,
		&u.LastLoginAt, &u.LastLoginIP, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// FindUserByExternalIDAndProviderType 按 external_id + provider_type 查找用户
// IM 路径使用：platformToProvider 返回 type，users.auth_provider 存的是 name，需要 JOIN
func (s *PGStore) FindUserByExternalIDAndProviderType(ctx context.Context, externalID, providerType string) (*User, error) {
	u := &User{}
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.external_id, u.auth_provider, u.display_name, u.email, u.avatar_url, u.department,
		        u.role, u.status, u.last_login_at, u.last_login_ip, u.created_at, u.updated_at
		 FROM users u
		 JOIN auth_providers p ON u.auth_provider = p.name
		 WHERE u.external_id = $1 AND p.provider_type = $2 AND p.enabled = TRUE`,
		externalID, providerType,
	).Scan(&u.ID, &u.ExternalID, &u.AuthProvider, &u.DisplayName, &u.Email,
		&u.AvatarURL, &u.Department, &u.Role, &u.Status,
		&u.LastLoginAt, &u.LastLoginIP, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByID 按 ID 查找用户
func (s *PGStore) GetUserByID(ctx context.Context, userID string) (*User, error) {
	u := &User{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, external_id, auth_provider, display_name, email, avatar_url, department,
		        role, status, last_login_at, last_login_ip, created_at, updated_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&u.ID, &u.ExternalID, &u.AuthProvider, &u.DisplayName, &u.Email,
		&u.AvatarURL, &u.Department, &u.Role, &u.Status,
		&u.LastLoginAt, &u.LastLoginIP, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// CreateUser 创建用户
func (s *PGStore) CreateUser(ctx context.Context, user *User) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, external_id, auth_provider, display_name, email, avatar_url, department, role, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		user.ID, user.ExternalID, user.AuthProvider, user.DisplayName,
		user.Email, user.AvatarURL, user.Department, user.Role, user.Status,
	)
	return err
}

// CountUsers 统计用户总数（Bootstrap admin 用）
func (s *PGStore) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

// UpdateUserProfile 更新用户资料
func (s *PGStore) UpdateUserProfile(ctx context.Context, userID string, info *UserInfo) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET display_name=$1, email=$2, avatar_url=$3, department=$4, updated_at=NOW()
		 WHERE id=$5`,
		info.DisplayName, info.Email, info.AvatarURL, info.Department, userID,
	)
	return err
}

// UpdateLoginInfo 更新最后登录时间和 IP
func (s *PGStore) UpdateLoginInfo(ctx context.Context, userID, ip string) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET last_login_at=$1, last_login_ip=$2, updated_at=NOW() WHERE id=$3`,
		now, ip, userID,
	)
	return err
}

// RecordLogin 记录登录历史
func (s *PGStore) RecordLogin(ctx context.Context, record *LoginRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO login_history (user_id, auth_provider, ip_address, user_agent)
		 VALUES ($1, $2, $3, $4)`,
		record.UserID, record.AuthProvider, record.IPAddress, record.UserAgent,
	)
	return err
}

// beginningOfNextMonth 返回指定时间所在月份的下个月第一天（UTC 0点）
func beginningOfNextMonth(t time.Time) time.Time {
	t = t.UTC()
	y, m, _ := t.Date()
	return time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC)
}

// GetUserQuota 查询用户配额，无记录返回 nil（表示无限制）
func (s *PGStore) GetUserQuota(ctx context.Context, userID string) (*UserQuota, error) {
	q := &UserQuota{}
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, token_quota, token_used, quota_reset_at FROM user_quotas WHERE user_id = $1`,
		userID,
	).Scan(&q.UserID, &q.TokenQuota, &q.TokenUsed, &q.QuotaResetAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return q, nil
}

// UpsertUserQuota 插入或更新用户配额
// ON CONFLICT 只更新 token_quota，不重置 quota_reset_at（管理员月中改配额不应重置月度计时器）
func (s *PGStore) UpsertUserQuota(ctx context.Context, userID string, tokenQuota int64) error {
	nextReset := beginningOfNextMonth(time.Now())
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_quotas (user_id, token_quota, token_used, quota_reset_at, updated_at)
		 VALUES ($1, $2, 0, $3, NOW())
		 ON CONFLICT (user_id) DO UPDATE SET token_quota = $2, updated_at = NOW()`,
		userID, tokenQuota, nextReset,
	)
	return err
}

// IncrementTokenUsage 累加 token 使用量（含零行保护）
// 零行保护：INSERT ON CONFLICT DO NOTHING 确保行存在，不覆盖已有的 token_quota
func (s *PGStore) IncrementTokenUsage(ctx context.Context, userID string, tokens int64) error {
	nextReset := beginningOfNextMonth(time.Now())
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_quotas (user_id, token_quota, token_used, quota_reset_at, updated_at)
		 VALUES ($1, 0, 0, $2, NOW())
		 ON CONFLICT (user_id) DO NOTHING`,
		userID, nextReset)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE user_quotas SET token_used = token_used + $1, updated_at = NOW() WHERE user_id = $2`,
		tokens, userID)
	return err
}

// ResetQuotaIfExpired 检查配额是否过期，过期则重置并返回最新记录
// 返回 nil 表示无配额记录（无限制）
func (s *PGStore) ResetQuotaIfExpired(ctx context.Context, userID string, now time.Time) (*UserQuota, error) {
	nextReset := beginningOfNextMonth(now)
	_, err := s.pool.Exec(ctx,
		`UPDATE user_quotas SET token_used = 0, quota_reset_at = $1, updated_at = NOW()
		 WHERE user_id = $2 AND quota_reset_at <= $3`,
		nextReset, userID, now)
	if err != nil {
		return nil, err
	}
	return s.GetUserQuota(ctx, userID)
}

// ListUsers 分页查询用户列表（含配额信息），支持按 display_name/email 模糊搜索
// LEFT JOIN user_quotas 避免 N+1 查询
func (s *PGStore) ListUsers(ctx context.Context, query string, page, size int) ([]*UserWithQuota, int64, error) {
	offset := (page - 1) * size

	// BUG-4 修复：短路条件必须用原始 query 判断是否为空，
	// pattern 永远是 "%...%" 格式，$1 = '' 永远为 false 是死代码。
	var total int64
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE ($1 = '' OR display_name ILIKE $2 OR email ILIKE $2)`,
		query, "%"+query+"%",
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT u.id, u.display_name, u.email, u.role, u.status, u.auth_provider,
		        u.created_at, u.last_login_at,
		        COALESCE(q.token_quota, 0), COALESCE(q.token_used, 0)
		 FROM users u
		 LEFT JOIN user_quotas q ON u.id = q.user_id
		 WHERE ($1 = '' OR u.display_name ILIKE $2 OR u.email ILIKE $2)
		 ORDER BY u.created_at DESC LIMIT $3 OFFSET $4`,
		query, "%"+query+"%", size, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*UserWithQuota
	for rows.Next() {
		uwq := &UserWithQuota{User: &User{}}
		if err := rows.Scan(&uwq.ID, &uwq.DisplayName, &uwq.Email, &uwq.Role, &uwq.Status,
			&uwq.AuthProvider, &uwq.CreatedAt, &uwq.LastLoginAt,
			&uwq.TokenQuota, &uwq.TokenUsed); err != nil {
			return nil, 0, err
		}
		users = append(users, uwq)
	}
	return users, total, rows.Err()
}

// GetUserWithQuota 获取用户详情含配额（Admin 详情接口）
func (s *PGStore) GetUserWithQuota(ctx context.Context, userID string) (*UserWithQuota, error) {
	uwq := &UserWithQuota{User: &User{}}
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.display_name, u.email, u.role, u.status, u.auth_provider,
		        u.created_at, u.last_login_at, u.last_login_ip,
		        COALESCE(q.token_quota, 0), COALESCE(q.token_used, 0),
		        COALESCE(q.quota_reset_at, NOW())
		 FROM users u LEFT JOIN user_quotas q ON u.id = q.user_id
		 WHERE u.id = $1`,
		userID,
	).Scan(&uwq.ID, &uwq.DisplayName, &uwq.Email, &uwq.Role, &uwq.Status,
		&uwq.AuthProvider, &uwq.CreatedAt, &uwq.LastLoginAt, &uwq.LastLoginIP,
		&uwq.TokenQuota, &uwq.TokenUsed, &uwq.QuotaResetAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return uwq, nil
}

// UpdateUserRole 更新用户角色
func (s *PGStore) UpdateUserRole(ctx context.Context, userID, role string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET role = $1 WHERE id = $2`,
		role, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// UpdateUserStatus 更新用户状态
func (s *PGStore) UpdateUserStatus(ctx context.Context, userID, status string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET status = $1 WHERE id = $2`,
		status, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetLoginHistory 获取用户登录历史
func (s *PGStore) GetLoginHistory(ctx context.Context, userID string, limit int) ([]*LoginRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT user_id, auth_provider, ip_address, user_agent, created_at
		 FROM login_history WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*LoginRecord
	for rows.Next() {
		r := &LoginRecord{}
		if err := rows.Scan(&r.UserID, &r.AuthProvider, &r.IPAddress, &r.UserAgent, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// ListAllProviders 列出所有 provider（含禁用的，Admin 用）
func (s *PGStore) ListAllProviders(ctx context.Context) ([]ProviderConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT name, provider_type, enabled, config_json FROM auth_providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []ProviderConfig
	for rows.Next() {
		var p ProviderConfig
		if err := rows.Scan(&p.Name, &p.ProviderType, &p.Enabled, &p.ConfigJSON); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

// CreateProvider 创建 auth provider
func (s *PGStore) CreateProvider(ctx context.Context, cfg ProviderConfig) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO auth_providers (name, provider_type, enabled, config_json)
		 VALUES ($1, $2, $3, $4)`,
		cfg.Name, cfg.ProviderType, cfg.Enabled, cfg.ConfigJSON,
	)
	return err
}

// UpdateProvider 全量替换 provider 配置（auth handler 内部已用 UpdateProviderFields，
// 此方法保留用于向后兼容，语义为全量替换，不适合 PATCH 场景）
func (s *PGStore) UpdateProvider(ctx context.Context, name string, cfg ProviderConfig) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE auth_providers SET provider_type=$1, enabled=$2, config_json=$3, updated_at=NOW()
		 WHERE name=$4`,
		cfg.ProviderType, cfg.Enabled, cfg.ConfigJSON, name,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// UpsertProvider 按 name 插入或更新 provider（seed 逻辑专用）
func (s *PGStore) UpsertProvider(ctx context.Context, cfg ProviderConfig) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO auth_providers (name, provider_type, enabled, config_json)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (name) DO UPDATE SET
			provider_type = EXCLUDED.provider_type,
			enabled       = EXCLUDED.enabled,
			config_json   = EXCLUDED.config_json,
			updated_at    = NOW()`,
		cfg.Name, cfg.ProviderType, cfg.Enabled, cfg.ConfigJSON,
	)
	return err
}

// UpdateProviderFields partial-updates only the non-nil fields in update.
// This avoids the Go bool zero-value trap where PATCH omitting "enabled" defaults to false.
func (s *PGStore) UpdateProviderFields(ctx context.Context, name string, update ProviderUpdate) error {
	// Build dynamic SQL based on which fields are present.
	// provider_type: update only if pointer is non-nil and non-empty.
	// enabled:      update only if pointer is non-nil.
	// config_json:   update only if non-empty (not {} or "").
	updates := []string{"updated_at = NOW()"}
	args := []any{}
	argIdx := 1

	if update.ProviderType != nil && *update.ProviderType != "" {
		updates = append(updates, fmt.Sprintf("provider_type = $%d", argIdx))
		args = append(args, *update.ProviderType)
		argIdx++
	}
	if update.Enabled != nil {
		updates = append(updates, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *update.Enabled)
		argIdx++
	}
	if len(update.ConfigJSON) > 0 && string(update.ConfigJSON) != "{}" && string(update.ConfigJSON) != "null" {
		updates = append(updates, fmt.Sprintf("config_json = $%d", argIdx))
		args = append(args, update.ConfigJSON)
		argIdx++
	}
	if len(updates) == 1 {
		// Nothing to update, just touch updated_at.
		// (updated_at = NOW() already in updates, skip duplicate)
	}

	args = append(args, name)
	query := fmt.Sprintf(
		"UPDATE auth_providers SET %s WHERE name = $%d",
		strings.Join(updates, ", "), argIdx,
	)
	tag, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteProvider 删除 auth provider。
// 使用事务级 advisory lock 防止并发删除竞态，消除 TOCTOU。
// - 如果 provider 不存在，返回 pgx.ErrNoRows。
// - 如果 provider 已启用且删除后没有其他启用的 provider，返回错误（拒绝删除）。
// - 如果 provider 已禁用，直接删除（不受"最后一个启用"保护）。
func (s *PGStore) DeleteProvider(ctx context.Context, name string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 获取事务级 advisory lock，防止并发删除竞态
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('auth_providers_delete'))`); err != nil {
		return err
	}

	// 检查 provider 是否存在
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT enabled FROM auth_providers WHERE name = $1`, name).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	if err != nil {
		return err
	}

	// 如果是 enabled provider，检查是否是最后一个
	if enabled {
		var remaining int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM auth_providers WHERE enabled = TRUE AND name != $1`, name,
		).Scan(&remaining); err != nil {
			return err
		}
		if remaining == 0 {
			return errors.New("不能删除最后一个启用的认证 Provider")
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM auth_providers WHERE name = $1`, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CountEnabledProviders 统计启用的 provider 数量
func (s *PGStore) CountEnabledProviders(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM auth_providers WHERE enabled = TRUE`,
	).Scan(&count)
	return count, err
}

// CountUsersByProvider 统计关联到指定 provider name 的用户数量
func (s *PGStore) CountUsersByProvider(ctx context.Context, providerName string) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE auth_provider = $1`,
		providerName,
	).Scan(&count)
	return count, err
}
