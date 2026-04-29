package compaction

import "encoding/json"

// CompactionRequest 压缩请求
type CompactionRequest struct {
	Messages  json.RawMessage `json:"messages"`            // []llm.MessageWithTools
	SessionID string          `json:"session_id,omitempty"` // 来源会话 ID（用于记忆提取）
}

// CompactionResult 压缩结果
type CompactionResult struct {
	Messages json.RawMessage `json:"messages"`
	Stats    *Stats          `json:"stats,omitempty"`
}

// Stats 压缩统计信息（独立类型，不依赖 master 包）
type Stats struct {
	Original       int    `json:"original"`        // 原始消息数
	Remaining      int    `json:"remaining"`       // 保留消息数（包括摘要）
	Compressed     int    `json:"compressed"`      // 被压缩的消息数
	Strategy       string `json:"strategy"`        // 使用的策略
	OriginalToken  int    `json:"original_token"`  // 原始 token 数
	RemainingToken int    `json:"remaining_token"` // 压缩后 token 数
	LazySkipped    bool   `json:"lazy_skipped"`    // 懒惰模式下是否跳过压缩
}
