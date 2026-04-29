package observability

import (
	"crypto/rand"
	"encoding/hex"
)

// NewTraceID 生成一个 16 字节随机 trace ID（32 位 hex）
func NewTraceID() string {
	return newHexID(16)
}

// NewSpanID 生成一个 8 字节随机 span ID（16 位 hex）
func NewSpanID() string {
	return newHexID(8)
}

func newHexID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
