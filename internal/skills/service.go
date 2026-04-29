package skills

import (
	"context"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/store"
)

// SkillService 管理 DB 层 skill 的生命周期：
// - 启动时从 DB 加载所有 skill 到 OverlayRegistry 的 dbCache
// - 监听 pg_notify 通知，热更新 dbCache（增量刷新）
type SkillService struct {
	store    *store.SkillStore
	registry *OverlayRegistry
	logger   *zap.Logger
}

// NewSkillService 创建 SkillService。
func NewSkillService(store *store.SkillStore, registry *OverlayRegistry, logger *zap.Logger) *SkillService {
	return &SkillService{
		store:    store,
		registry: registry,
		logger:   logger,
	}
}

// LoadAll 从 DB 加载所有 skill 到内存缓存（启动时调用一次）。
// personal skill（r.UserID != ""）与 public skill（r.UserID == ""）按 dbCacheKey{Name, UserID} 独立索引，
// 解决原 pg_notify 按 name 单索引导致的跨租户覆盖问题（hive-skill-on-demand MAJOR 2）。
func (s *SkillService) LoadAll(ctx context.Context) error {
	records, err := s.store.LoadAll(ctx)
	if err != nil {
		return err
	}
	for _, r := range records {
		s.registry.UpsertDB(r.Name, r.UserID, r.Content, r.Path, r.Revision)
	}
	s.logger.Info("DB skill 已全量加载", zap.Int("count", len(records)))
	return nil
}

// Start 启动 pg_notify 监听器，ctx cancel 时自动退出。
// 应在 goroutine 中或 LoadAll 之后调用。
func (s *SkillService) Start(ctx context.Context) {
	s.store.StartNotifyListener(ctx, func(name, userID string) {
		s.reload(ctx, name, userID)
	})
}

// reload 收到 pg_notify 后重新从 DB 加载单个 (name, userID) 的 skill（增量）。
func (s *SkillService) reload(ctx context.Context, name, userID string) {
	record, found, err := s.store.Get(ctx, name, userID)
	if err != nil {
		s.logger.Warn("reload skill 失败",
			zap.String("name", name),
			zap.String("user_id", userID),
			zap.Error(err))
		return
	}
	if !found {
		// skill 已被删除，从缓存移除（按 {name, user_id} 精准删，不影响另一层）
		s.registry.DeleteDB(name, userID)
		s.logger.Info("DB skill 已删除，缓存已清除",
			zap.String("name", name),
			zap.String("user_id", userID))
		return
	}
	s.registry.UpsertDB(record.Name, record.UserID, record.Content, record.Path, record.Revision)
	s.logger.Info("DB skill 已热更新",
		zap.String("name", name),
		zap.String("user_id", userID),
		zap.Int("revision", record.Revision))
}
