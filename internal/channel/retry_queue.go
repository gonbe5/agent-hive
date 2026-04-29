package channel

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// RetryReason 标记一次 enqueue 的语义来源，便于日志/告警按类型分桶。
type RetryReason string

const (
	RetryReasonHandlerError    RetryReason = "handler_error"     // 业务处理失败（router.HandleMessage 返回 err）
	RetryReasonHandlerPanic    RetryReason = "handler_panic"     // 业务 goroutine panic recover
	RetryReasonRouterNil       RetryReason = "router_nil"        // 编排错误：handler 拿到 nil router
	RetryReasonNotifyFailed    RetryReason = "notify_failed"     // NotifyError 自身也失败
	RetryReasonDedupBackend    RetryReason = "dedup_backend"     // dedup 后端故障导致 fail-closed 拒绝
	RetryReasonClaimLost       RetryReason = "claim_lost"        // 两阶段 claim 取得后业务 panic/崩溃
	RetryReasonWelcomeSend     RetryReason = "welcome_send"      // 生命周期欢迎消息发送失败，可安全重放
	RetryReasonPushSend        RetryReason = "push_send"         // 主动推送发送失败，可安全重放
)

// RetryItem 是一条待重试入站事件的快照。
//
// 不变量：
//   - 入队时不得 mutate（实现方应做 deep-copy 或 marshal 到 bytes）
//   - Reason 不可为空字符串
//   - EnqueuedAt 必须为 UTC 单调时间，便于跨机比较
type RetryItem struct {
	// MessageID / EventID 至少一个非空，用于运维定位和后续 reclaim worker 去重
	MessageID string `json:"message_id,omitempty"`
	EventID   string `json:"event_id,omitempty"`
	Platform  string `json:"platform,omitempty"`
	TenantKey string `json:"tenant_key,omitempty"`
	ChatID    string `json:"chat_id,omitempty"`
	SenderID  string `json:"sender_id,omitempty"`

	Reason RetryReason `json:"reason"`
	// ErrorMsg 是触发入队的最后一个错误（panic 时为 fmt.Sprint(rec)），便于日志检索
	ErrorMsg string `json:"error_msg,omitempty"`

	// Payload 是入站事件的可序列化载荷（通常为 InboundMessage 的 JSON），用于离线分析
	// 不要求 retry 时回放（飞书 webhook 不允许我们重放对方推送），主用途是落档 + 告警
	Payload json.RawMessage `json:"payload,omitempty"`

	EnqueuedAt time.Time `json:"enqueued_at"`
	Attempts   int       `json:"attempts"`
}

// RetryQueue 是 P0-#7 的最小契约：handler 错误路径必须能"原子入队"，
// 不允许在持锁路径上做 IO。Stop 必须幂等。
//
// 实现方需保证：
//   - Enqueue 在 nil 接收者上是 no-op（fail-safe，避免上线编排错误导致 nil-deref）
//   - Enqueue 失败必须返回 error，让 wrapper 走 Logger 落兜底（不得静默吞）
//   - Snapshot 仅用于测试和运维，不得阻塞 Enqueue
type RetryQueue interface {
	Enqueue(item RetryItem) error
	Snapshot() []RetryItem
	Len() int
	Stop() error
}

// MemoryRetryQueue 默认内存实现。
//
// 适用场景：单实例进程内，重启即丢；用作 P0-#7 的最小可用实现，
// 需要持久化时 wrap 在 FileBackedRetryQueue 中。
//
// 不变量：
//   - 容量软上限：超过 maxItems 时丢最旧（FIFO drop），并通过 logger.Warn 告警
//   - 永远不阻塞 Enqueue（哪怕 backend 满）
type MemoryRetryQueue struct {
	mu       sync.Mutex
	items    []RetryItem
	maxItems int
	logger   *zap.Logger
	closed   atomic.Bool
}

// NewMemoryRetryQueue 创建内存 RetryQueue。maxItems<=0 时取默认 1024。
func NewMemoryRetryQueue(maxItems int, logger *zap.Logger) *MemoryRetryQueue {
	if maxItems <= 0 {
		maxItems = 1024
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MemoryRetryQueue{
		items:    make([]RetryItem, 0, 64),
		maxItems: maxItems,
		logger:   logger,
	}
}

// Enqueue 写入一条 RetryItem。nil receiver 时 no-op，返回 nil（fail-safe）。
func (q *MemoryRetryQueue) Enqueue(item RetryItem) error {
	if q == nil {
		return nil
	}
	if q.closed.Load() {
		return errors.New("retry queue closed")
	}
	if item.Reason == "" {
		return errors.New("retry item missing reason")
	}
	if item.EnqueuedAt.IsZero() {
		item.EnqueuedAt = time.Now().UTC()
	}
	q.mu.Lock()
	if len(q.items) >= q.maxItems {
		dropped := q.items[0]
		q.items = q.items[1:]
		q.logger.Warn("retry_queue 容量满，丢弃最旧条目",
			zap.String("dropped_message_id", dropped.MessageID),
			zap.String("dropped_event_id", dropped.EventID),
			zap.String("dropped_reason", string(dropped.Reason)),
			zap.Int("max_items", q.maxItems))
	}
	q.items = append(q.items, item)
	n := len(q.items)
	q.mu.Unlock()
	q.logger.Warn("retry_queue 写入条目",
		zap.String("message_id", item.MessageID),
		zap.String("event_id", item.EventID),
		zap.String("platform", item.Platform),
		zap.String("reason", string(item.Reason)),
		zap.String("error_msg", item.ErrorMsg),
		zap.Int("queue_len", n))
	return nil
}

// Snapshot 返回当前队列的浅拷贝（item 是 value type，浅拷贝即可）。
func (q *MemoryRetryQueue) Snapshot() []RetryItem {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]RetryItem, len(q.items))
	copy(out, q.items)
	return out
}

// Len 返回队列当前长度。
func (q *MemoryRetryQueue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Stop 标记队列关闭，后续 Enqueue 会拒绝。幂等。
func (q *MemoryRetryQueue) Stop() error {
	if q == nil {
		return nil
	}
	q.closed.Store(true)
	return nil
}

// FileBackedRetryQueue 在 MemoryRetryQueue 之上叠加 append-only JSONL 持久化。
//
// 设计目标：进程崩溃 / OOM / panic 后，retry 项必须能在磁盘上恢复，
// 否则与"handler 永返 nil + 仅 in-memory 入队"组合等价于"消息永久丢失"。
//
// 文件格式：每行一条 JSON-marshal 的 RetryItem。Append 失败也不能阻断 Enqueue
// （否则一旦磁盘满，handler 会卡住或 panic，破坏 P0-#7 不变量），失败仅 logger.Error。
type FileBackedRetryQueue struct {
	*MemoryRetryQueue
	path string

	mu sync.Mutex // 保护 path 文件的串行 append
}

// NewFileBackedRetryQueue 创建文件持久化 RetryQueue。
// path 为空时退化为纯内存（不写文件）。父目录不存在时自动创建。
func NewFileBackedRetryQueue(path string, maxItems int, logger *zap.Logger) (*FileBackedRetryQueue, error) {
	mem := NewMemoryRetryQueue(maxItems, logger)
	q := &FileBackedRetryQueue{
		MemoryRetryQueue: mem,
		path:             path,
	}
	if path != "" {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
	}
	return q, nil
}

// Enqueue 先入内存（保证查询可见），再 best-effort append 到文件。
func (q *FileBackedRetryQueue) Enqueue(item RetryItem) error {
	if q == nil {
		return nil
	}
	if err := q.MemoryRetryQueue.Enqueue(item); err != nil {
		return err
	}
	if q.path == "" {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	f, err := os.OpenFile(q.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		q.logger.Error("retry_queue 文件打开失败（仅内存兜底）",
			zap.String("path", q.path), zap.Error(err))
		return nil
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(item); err != nil {
		q.logger.Error("retry_queue 文件写入失败（仅内存兜底）",
			zap.String("path", q.path), zap.Error(err))
	}
	return nil
}

// Path 返回持久化文件路径，便于运维/测试断言。
func (q *FileBackedRetryQueue) Path() string {
	if q == nil {
		return ""
	}
	return q.path
}
