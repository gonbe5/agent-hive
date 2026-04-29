package master

import (
	"sync"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// OrderedBuffer 保证结果按 tool_use 接收顺序 emit。
// 即使先完成的工具也要等待前面的工具完成，保持 LLM 看到的结果顺序与请求顺序一致。
type OrderedBuffer struct {
	mu       sync.Mutex
	received []*mcphost.ToolResult // 按接收顺序存储
	nextEmit int                   // 下一个应 emit 的索引
}

// NewOrderedBuffer 创建有序结果缓冲。
func NewOrderedBuffer(bufferSize int) *OrderedBuffer {
	if bufferSize <= 0 {
		bufferSize = 16
	}
	return &OrderedBuffer{
		received: make([]*mcphost.ToolResult, 0, bufferSize),
	}
}

// Add 添加一个结果，返回需要 emit 的批次（可能为空）。
// 结果按接收顺序存储，先完成的结果等待前面的完成后一起 emit。
func (b *OrderedBuffer) Add(result *mcphost.ToolResult) []*mcphost.ToolResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	// P0-2 修复：idx 应该是 append 前的长度（追加到末尾），而非遍历找 nil
	idx := len(b.received)
	b.received = append(b.received, result)

	// 如果是下一个应 emit 的结果，尝试 flush
	if idx == b.nextEmit {
		return b.flushReadyLocked()
	}
	// 否则已在末尾存储，无需额外操作
	return nil
}

// flushReadyLocked 在持有锁的情况下 flush 连续就绪的结果。
// 调用方必须持有 b.mu。
func (b *OrderedBuffer) flushReadyLocked() []*mcphost.ToolResult {
	var batch []*mcphost.ToolResult
	for b.nextEmit < len(b.received) && b.received[b.nextEmit] != nil {
		batch = append(batch, b.received[b.nextEmit])
		b.nextEmit++
	}
	return batch
}
