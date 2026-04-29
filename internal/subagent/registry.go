package subagent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// Agent 是所有 sub-agent 需要实现的接口
type Agent interface {
	ID() string
	Card() AgentCard
	Mailbox() *SubAgentMailbox
	Status() AgentStatus
	Run(ctx context.Context)
	Stop()
	SendTask(ctx context.Context, req TaskRequest) (TaskResponse, error)
	Ping(ctx context.Context) (HealthStatus, error)
}

// Registry 管理 sub-agent 的注册、发现和健康检查
type Registry struct {
	mu     sync.RWMutex
	agents map[string]Agent
	logger *zap.Logger
}

// NewRegistry 创建一个新的 sub-agent 注册表
func NewRegistry(logger *zap.Logger) *Registry {
	return &Registry{
		agents: make(map[string]Agent),
		logger: logger,
	}
}

// Register 将一个 agent 添加到注册表
func (r *Registry) Register(agent Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := agent.ID()
	if _, exists := r.agents[id]; exists {
		return errs.New(errs.CodeInvalidInput, fmt.Sprintf("agent %q already registered", id))
	}
	r.agents[id] = agent
	r.logger.Info("已注册 agent", zap.String("id", id), zap.String("name", agent.Card().Name))
	return nil
}

// Unregister 从注册表中移除一个 agent
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[id]; !exists {
		return errs.New(errs.CodeAgentNotFound, fmt.Sprintf("agent %q not found", id))
	}
	delete(r.agents, id)
	r.logger.Info("已取消注册 agent", zap.String("id", id))
	return nil
}

// Get 根据 ID 返回一个 agent
func (r *Registry) Get(id string) (Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[id]
	if !ok {
		return nil, errs.New(errs.CodeAgentNotFound, fmt.Sprintf("agent %q not found", id))
	}
	return a, nil
}

// List 返回所有已注册的 agent 卡片
func (r *Registry) List() []AgentCard {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cards := make([]AgentCard, 0, len(r.agents))
	for _, a := range r.agents {
		cards = append(cards, a.Card())
	}
	return cards
}

// StartAll 启动所有已注册的 agent，每个 agent 在自己的 goroutine 中运行
func (r *Registry) StartAll(ctx context.Context) {
	r.mu.RLock()
	agents := make([]Agent, 0, len(r.agents))
	for _, a := range r.agents {
		agents = append(agents, a)
	}
	r.mu.RUnlock()

	// 释放锁后启动，避免 agent 启动时回调 registry 导致死锁
	for _, a := range agents {
		go a.Run(ctx)
	}
	r.logger.Info("已启动所有 agent", zap.Int("count", len(agents)))
}

// StopAll 向所有 agent 发送停止信号
func (r *Registry) StopAll() {
	r.mu.RLock()
	agents := make([]Agent, 0, len(r.agents))
	for _, a := range r.agents {
		agents = append(agents, a)
	}
	r.mu.RUnlock()

	// 释放锁后停止，避免 agent 停止时回调 registry 导致死锁
	for _, a := range agents {
		a.Stop()
	}
	r.logger.Info("已停止所有 agent", zap.Int("count", len(agents)))
}

// HealthCheckAll 检测所有 agent 并返回它们的健康状态
func (r *Registry) HealthCheckAll(ctx context.Context) map[string]HealthStatus {
	r.mu.RLock()
	agents := make([]Agent, 0, len(r.agents))
	for _, a := range r.agents {
		agents = append(agents, a)
	}
	r.mu.RUnlock()

	results := make(map[string]HealthStatus, len(agents))
	var mu sync.Mutex
	var wg sync.WaitGroup

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for _, a := range agents {
		wg.Add(1)
		go func(agent Agent) {
			defer wg.Done()
			status, err := agent.Ping(pingCtx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results[agent.ID()] = HealthStatus{
					AgentID: agent.ID(),
					Status:  StatusError,
				}
			} else {
				results[agent.ID()] = status
			}
		}(a)
	}

	wg.Wait()
	return results
}
