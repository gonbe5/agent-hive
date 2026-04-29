package master

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// content wraps a string as json.RawMessage for ToolResult.Content.
func content(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// ToolExecutorFunc 是工具执行函数的签名。
type ToolExecutorFunc func(ctx context.Context, name string, input json.RawMessage) (*mcphost.ToolResult, error)

// StreamingExecutor 边收边执行，按并发安全策略调度。
type StreamingExecutor struct {
	mu          sync.Mutex
	toolDef     map[string]*mcphost.ToolDefinition
	executor    ToolExecutorFunc
	pending     []*TrackedTool       // 正在执行 + 排队中
	ordered     *OrderedBuffer       // 有序缓冲
	unsafeCount int                  // 正在执行的不安全工具数
	unsafeQueue []*TrackedTool      // unsafe 工具排队队列

	// emit 回调（由调用方通过 SetEmitFunc 注入）
	emitFunc func(result *mcphost.ToolResult)
}

// TrackedTool 跟踪单个工具的执行状态。
type TrackedTool struct {
	ID        string                 // tool_use ID
	Name      string                 // 工具名
	Input     json.RawMessage        // 工具参数
	IsSafe    bool                   // 是否并发安全
	Result    *mcphost.ToolResult   // 执行结果
	Done      chan struct{}          // 完成信号
	Aborted   bool                   // 是否被 sibling 取消
	ctx       context.Context        // 执行上下文，用于传播取消信号
	closeOnce sync.Once              // Bug 1 fix: 防止 Done channel 被多次 close
}

// closeDone 安全关闭 Done channel，通过 sync.Once 防止 double-close panic。
func (t *TrackedTool) closeDone() {
	t.closeOnce.Do(func() { close(t.Done) })
}

// listToolsToMap 将 []ToolDefinition 转为 map[string]*ToolDefinition。
// P1-2 修复：替代不存在的 GetToolDefinitions()
func listToolsToMap(defs []mcphost.ToolDefinition) map[string]*mcphost.ToolDefinition {
	m := make(map[string]*mcphost.ToolDefinition, len(defs))
	for i := range defs {
		m[defs[i].Name] = &defs[i]
	}
	return m
}

// NewStreamingExecutor 创建执行器。
func NewStreamingExecutor(
	toolDefs []mcphost.ToolDefinition,
	executor ToolExecutorFunc,
) *StreamingExecutor {
	return &StreamingExecutor{
		toolDef: listToolsToMap(toolDefs),
		executor: executor,
		ordered:  NewOrderedBuffer(16),
		pending:  make([]*TrackedTool, 0, 16),
	}
}

// SetEmitFunc 设置结果 emit 回调。
func (se *StreamingExecutor) SetEmitFunc(fn func(result *mcphost.ToolResult)) {
	se.emitFunc = fn
}

// AddTool 添加一个工具调用（非阻塞）。
// ctx 用于传播取消信号到底层 executor（Bug 4 fix）。
func (se *StreamingExecutor) AddTool(ctx context.Context, id, name string, input json.RawMessage) {
	se.mu.Lock()

	def := se.toolDef[name]
	isSafe := def != nil && def.IsConcurrencySafe

	tool := &TrackedTool{
		ID:     id,
		Name:   name,
		Input:  input,
		IsSafe: isSafe,
		Done:   make(chan struct{}),
		ctx:    ctx,
	}
	se.pending = append(se.pending, tool)
	se.mu.Unlock()

	// P0-3 修复：unsafe 工具需要排队等待，不能和另一个 unsafe 并发
	if isSafe {
		go se.runTool(tool)
	} else {
		se.enqueueUnsafe(tool)
	}
}

// runTool 执行单个工具。
func (se *StreamingExecutor) runTool(tool *TrackedTool) {
	// Bug 1 fix: 使用 closeDone() 而非直接 close，防止与 AbortAll 的 double-close panic
	defer tool.closeDone()

	se.mu.Lock()
	if tool.Aborted {
		// unsafe 工具被 abort 时需归还占用的 slot，并尝试调度下一个
		if !tool.IsSafe {
			se.unsafeCount--
			se.tryScheduleUnsafeLocked()
		}
		se.mu.Unlock()
		return
	}
	se.mu.Unlock()

	// Bug 4 fix: 使用 tool.ctx 传播父 context 取消信号，替代 context.Background()
	result, err := se.executor(tool.ctx, tool.Name, tool.Input)

	se.mu.Lock()
	defer se.mu.Unlock()

	if !tool.IsSafe {
		se.unsafeCount--
		se.tryScheduleUnsafeLocked()
	}

	if tool.Aborted {
		return
	}

	tool.Result = result
	if err != nil {
		tool.Result = &mcphost.ToolResult{
			Content: content(fmt.Sprintf("execution error: %v", err)),
			IsError: true,
		}
	}

	// 尝试 emit
	if batch := se.ordered.Add(tool.Result); batch != nil {
		se.emitBatch(batch)
	}
}

// canExecute 检查是否可执行（无 unsafe 工具正在运行）。
// P0-3 修复：现在会在 AddTool 中被调用（见 AddTool）
func (se *StreamingExecutor) canExecute(tool *TrackedTool) bool {
	if tool.IsSafe {
		return true
	}
	return se.unsafeCount == 0
}

// enqueueUnsafe 将 unsafe 工具加入排队队列。
// 当 unsafeCount 降为 0 时自动调度队列中的工具。
func (se *StreamingExecutor) enqueueUnsafe(tool *TrackedTool) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.unsafeQueue = append(se.unsafeQueue, tool)
	se.tryScheduleUnsafeLocked()
}

// tryScheduleUnsafeLocked 尝试调度下一个 unsafe 工具。
// 调用方必须持有 se.mu。
// unsafeCount 在调度时立即递增（持有锁），不在 goroutine 内递增，消除竞态窗口。
func (se *StreamingExecutor) tryScheduleUnsafeLocked() {
	if se.unsafeCount > 0 || len(se.unsafeQueue) == 0 {
		return
	}
	next := se.unsafeQueue[0]
	se.unsafeQueue = se.unsafeQueue[1:]
	se.unsafeCount++ // 持有锁时递增，消除 goroutine 启动前的竞态窗口
	go se.runTool(next)
}

// AbortAll 取消所有 pending 工具（bash 失败时调用）。
func (se *StreamingExecutor) AbortAll(reason string) {
	se.mu.Lock()
	defer se.mu.Unlock()

	for _, tool := range se.pending {
		if !tool.IsSafe {
			tool.Aborted = true
			// Bug 1 fix: 使用 closeDone() 防止与 runTool 的 defer closeDone() double-close panic
			tool.closeDone()
		}
	}

	// 注入合成错误到结果流
	se.ordered.Add(&mcphost.ToolResult{
		Content: content(fmt.Sprintf("cancelled due to sibling tool failure: %s", reason)),
		IsError: true,
	})
}

// GetResults 等待所有工具完成并返回有序结果。
func (se *StreamingExecutor) GetResults() []*mcphost.ToolResult {
	se.mu.Lock()
	pending := make([]*TrackedTool, len(se.pending))
	copy(pending, se.pending)
	se.mu.Unlock()

	for _, tool := range pending {
		<-tool.Done
	}

	se.mu.Lock()
	defer se.mu.Unlock()

	results := make([]*mcphost.ToolResult, 0, len(se.pending))
	for _, tool := range se.pending {
		if tool.Result != nil && !tool.Aborted {
			results = append(results, tool.Result)
		}
	}
	return results
}

// GetResultsByID 等待所有工具完成并返回以工具 ID 为键的结果 map。
// Bug 2 fix: 调用方可通过 toolCall.ID 精确查找对应结果，避免 index 对齐错误。
func (se *StreamingExecutor) GetResultsByID() map[string]*mcphost.ToolResult {
	se.mu.Lock()
	pending := make([]*TrackedTool, len(se.pending))
	copy(pending, se.pending)
	se.mu.Unlock()

	for _, tool := range pending {
		<-tool.Done
	}

	se.mu.Lock()
	defer se.mu.Unlock()

	results := make(map[string]*mcphost.ToolResult, len(se.pending))
	for _, tool := range se.pending {
		if tool.Result != nil && !tool.Aborted {
			results[tool.ID] = tool.Result
		}
	}
	return results
}

// emitBatch 将结果 emit 到 session（调用方注入的 emitFunc）。
func (se *StreamingExecutor) emitBatch(batch []*mcphost.ToolResult) {
	if se.emitFunc != nil {
		for _, r := range batch {
			se.emitFunc(r)
		}
	}
}
