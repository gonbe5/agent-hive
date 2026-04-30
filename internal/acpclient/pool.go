package acpclient

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/subagent"
	"github.com/chef-guo/agents-hive/internal/tools"
)

// ACPClientPool 管理多个远程 ACP Agent 连接
type ACPClientPool struct {
	agents   map[string]*RemoteACPAgent
	mu       sync.RWMutex
	logger   *zap.Logger
	observer tools.DelegationObserver
}

// NewPool 创建新的 ACP 客户端连接池
func NewPool(logger *zap.Logger) *ACPClientPool {
	return NewPoolWithObserver(logger, nil)
}

func NewPoolWithObserver(logger *zap.Logger, observer tools.DelegationObserver) *ACPClientPool {
	return &ACPClientPool{
		agents:   make(map[string]*RemoteACPAgent),
		logger:   logger.With(zap.String("component", "acp_client_pool")),
		observer: observer,
	}
}

// Connect 连接远程 ACP Agent 并加入连接池
func (p *ACPClientPool) Connect(ctx context.Context, cfg RemoteAgentConfig) (*RemoteACPAgent, error) {
	p.mu.Lock()
	if _, exists := p.agents[cfg.Name]; exists {
		p.mu.Unlock()
		return nil, errs.New(errs.CodeInvalidInput,
			fmt.Sprintf("远程 Agent %q 已存在于连接池", cfg.Name))
	}
	p.mu.Unlock()

	agent, err := ConnectAndInit(ctx, cfg, p.logger)
	if err != nil {
		return nil, err
	}
	agent.observer = p.observer

	p.mu.Lock()
	p.agents[cfg.Name] = agent
	p.mu.Unlock()

	p.logger.Info("远程 ACP Agent 已加入连接池",
		zap.String("name", cfg.Name),
		zap.String("transport", cfg.Transport))

	return agent, nil
}

// Disconnect 断开指定远程 Agent 并从连接池移除
func (p *ACPClientPool) Disconnect(name string) error {
	p.mu.Lock()
	agent, exists := p.agents[name]
	if !exists {
		p.mu.Unlock()
		return errs.New(errs.CodeAgentNotFound,
			fmt.Sprintf("远程 Agent %q 不在连接池中", name))
	}
	delete(p.agents, name)
	p.mu.Unlock()

	agent.Stop()
	p.logger.Info("远程 ACP Agent 已从连接池移除", zap.String("name", name))
	return nil
}

// Get 根据名称获取远程 Agent
func (p *ACPClientPool) Get(name string) (*RemoteACPAgent, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	agent, ok := p.agents[name]
	return agent, ok
}

// List 列出所有远程 Agent 配置
func (p *ACPClientPool) List() []RemoteAgentConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	configs := make([]RemoteAgentConfig, 0, len(p.agents))
	for _, agent := range p.agents {
		configs = append(configs, agent.cfg)
	}
	return configs
}

// CloseAll 关闭所有远程 Agent 连接
func (p *ACPClientPool) CloseAll() {
	p.mu.Lock()
	agents := make([]*RemoteACPAgent, 0, len(p.agents))
	for _, agent := range p.agents {
		agents = append(agents, agent)
	}
	p.agents = make(map[string]*RemoteACPAgent)
	p.mu.Unlock()

	for _, agent := range agents {
		agent.Stop()
	}
	p.logger.Info("所有远程 ACP Agent 连接已关闭", zap.Int("count", len(agents)))
}

// HealthCheckAll 检测所有远程 Agent 的健康状态
func (p *ACPClientPool) HealthCheckAll(ctx context.Context) map[string]subagent.HealthStatus {
	p.mu.RLock()
	agents := make([]*RemoteACPAgent, 0, len(p.agents))
	for _, agent := range p.agents {
		agents = append(agents, agent)
	}
	p.mu.RUnlock()

	results := make(map[string]subagent.HealthStatus, len(agents))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, agent := range agents {
		wg.Add(1)
		go func(a *RemoteACPAgent) {
			defer wg.Done()
			status, err := a.Ping(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results[a.ID()] = subagent.HealthStatus{
					AgentID: a.ID(),
					Status:  subagent.StatusError,
				}
			} else {
				results[a.ID()] = status
			}
		}(agent)
	}

	wg.Wait()
	return results
}
