package eval

// Case 描述一条 memory/context 注入回归用例。
type Case struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Query           string          `json:"query"`
	UserID          string          `json:"user_id"`
	Memories        []MemoryFixture `json:"memories"`
	WantInjectedIDs []int64         `json:"want_injected_ids"`
	WantSkippedIDs  []int64         `json:"want_skipped_ids"`
	ForbiddenText   []string        `json:"forbidden_text"`
	Required        bool            `json:"required"`
}

// MemoryFixture 是 eval fixture 中的记忆输入。
type MemoryFixture struct {
	ID         int64   `json:"id"`
	UserID     string  `json:"user_id"`
	Type       string  `json:"type"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence,omitempty"`
	ExpiresAt  string  `json:"expires_at,omitempty"`
	Source     string  `json:"source,omitempty"`
}
