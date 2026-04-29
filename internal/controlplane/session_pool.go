package controlplane

import (
	"sync"

	"go.uber.org/zap"
)

// SessionPool 基于 sync.Map 的并发安全会话池
type SessionPool struct {
	active   sync.Map // sessionID -> struct{}
	count    int
	maxCount int
	mu       sync.Mutex
	logger   *zap.Logger
}

// NewSessionPool 创建并发会话池
func NewSessionPool(maxSessions int, logger *zap.Logger) *SessionPool {
	if maxSessions <= 0 {
		maxSessions = 100 // 默认最大 100 个并发会话
	}
	return &SessionPool{
		maxCount: maxSessions,
		logger:   logger,
	}
}

// Acquire 获取会话槽位，已存在的会话直接通过
func (p *SessionPool) Acquire(sessionID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 在锁保护内执行 LoadOrStore，避免 TOCTOU 竞态
	if _, loaded := p.active.LoadOrStore(sessionID, struct{}{}); loaded {
		return true
	}
	if p.count >= p.maxCount {
		p.active.Delete(sessionID)
		p.logger.Warn("会话池已满",
			zap.String("session_id", sessionID),
			zap.Int("max", p.maxCount))
		return false
	}
	p.count++
	return true
}

// Release 释放会话槽位
func (p *SessionPool) Release(sessionID string) {
	if _, loaded := p.active.LoadAndDelete(sessionID); loaded {
		p.mu.Lock()
		p.count--
		p.mu.Unlock()
	}
}

// Active 返回活跃会话数
func (p *SessionPool) Active() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}
