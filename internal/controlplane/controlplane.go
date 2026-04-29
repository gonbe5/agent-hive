package controlplane

import (
	"context"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/master"
)

// ControlPlane 是 Agent 控制平面，包装 Master 提供多会话管理能力
// 实现 channel.MessageProcessor 接口
type ControlPlane struct {
	master      *master.Master
	pool        *SessionPool
	rateLimiter *RateLimiter
	bindings    *BindingStore
	logger      *zap.Logger
}

// Config 控制平面配置
type Config struct {
	MaxSessions  int     `json:"max_sessions"`
	RateLimit    float64 `json:"rate_limit"`
	RateBurst    int     `json:"rate_burst"`
	BindingsFile string  `json:"bindings_file"`
}

// New 创建控制平面实例
func New(m *master.Master, cfg Config, logger *zap.Logger) (*ControlPlane, error) {
	pool := NewSessionPool(cfg.MaxSessions, logger)
	rl := NewRateLimiter(cfg.RateLimit, cfg.RateBurst)

	bs, err := NewBindingStore(cfg.BindingsFile, logger)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "初始化绑定存储失败", err)
	}

	return &ControlPlane{
		master:      m,
		pool:        pool,
		rateLimiter: rl,
		bindings:    bs,
		logger:      logger,
	}, nil
}

// ProcessMessage 实现 channel.MessageProcessor 接口
// 通过绑定查找或使用当前会话，然后委托给 Master
func (cp *ControlPlane) ProcessMessage(ctx context.Context, sessionID string, input string) (master.TaskResponse, error) {
	if sessionID == "" {
		sid, name := cp.master.GetCurrentSessionInfo()
		sessionID = sid
		cp.logger.Debug("使用当前会话", zap.String("session_id", sid), zap.String("name", name))
	}

	// 检查会话池
	if !cp.pool.Acquire(sessionID) {
		return master.TaskResponse{
			Error: "已达最大并发会话数",
		}, nil
	}
	defer cp.pool.Release(sessionID)

	// 委托给 Master
	return cp.master.ProcessMessage(ctx, sessionID, input)
}

// CreateSession 创建新会话（带速率限制）
func (cp *ControlPlane) CreateSession(ctx context.Context, source string) (string, error) {
	if !cp.rateLimiter.Allow(source) {
		return "", errs.New(errs.CodeCPRateLimited, "来源 "+source+" 已超出速率限制")
	}

	resp, err := cp.master.ProcessMessage(ctx, "", "/new")
	if err != nil {
		return "", err
	}

	cp.logger.Info("通过控制平面创建会话",
		zap.String("source", source),
		zap.String("message", resp.Message))

	sid, _ := cp.master.GetCurrentSessionInfo()
	return sid, nil
}

// RouteMessage 根据平台和 chatID 路由消息到绑定的会话
func (cp *ControlPlane) RouteMessage(ctx context.Context, platform, chatID, content string) error {
	sessionID := cp.bindings.Lookup(platform, chatID)
	_, err := cp.ProcessMessage(ctx, sessionID, content)
	return err
}

// Bind 创建绑定
func (cp *ControlPlane) Bind(platform, chatID, sessionID string) error {
	return cp.bindings.Bind(platform, chatID, sessionID)
}

// Unbind 删除绑定
func (cp *ControlPlane) Unbind(platform, chatID string) {
	cp.bindings.Unbind(platform, chatID)
}

// ActiveSessions 返回活跃会话数
func (cp *ControlPlane) ActiveSessions() int {
	return cp.pool.Active()
}

// Bindings 返回绑定存储（供外部查询）
func (cp *ControlPlane) Bindings() *BindingStore {
	return cp.bindings
}

// Shutdown 优雅关闭控制平面
func (cp *ControlPlane) Shutdown() {
	cp.rateLimiter.Stop()
	cp.logger.Info("控制平面已关闭")
}
