package master

import (
	"sync"
)

// SiblingAbortController 管理并发工具间的取消信号。
// 当 bash 或其他危险工具失败时，通知所有"兄弟"工具取消。
type SiblingAbortController struct {
	mu      sync.Mutex
	abortCh chan struct{}
	active  bool
}

// NewSiblingAbortController 创建控制器。
func NewSiblingAbortController() *SiblingAbortController {
	return &SiblingAbortController{
		abortCh: make(chan struct{}, 1),
	}
}

// SignalAbort 当 bash 或其他危险工具失败时调用。
// 通知所有监听者取消当前批次的所有工具执行。
func (c *SiblingAbortController) SignalAbort() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active {
		select {
		case c.abortCh <- struct{}{}:
		default:
			// channel 已有值，说明已有 abort 信号
		}
	}
}

// Subscribe 返回一个 channel，当收到 abort 信号时会关闭。
// 调用方应在处理 abort 时立即返回。
func (c *SiblingAbortController) Subscribe() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = true
	return c.abortCh
}

// Reset 重置 abort 状态（开始新一轮工具批次时调用）。
// 旧 channel 保留原样（旧订阅者仍可读到信号），新建 channel 供下一轮订阅者使用。
func (c *SiblingAbortController) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = false
	c.abortCh = make(chan struct{}, 1)
}
