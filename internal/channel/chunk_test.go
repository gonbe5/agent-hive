package channel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxBytes int
		expected []string
	}{
		{
			name:     "短文本无需分块",
			text:     "hello",
			maxBytes: 10,
			expected: []string{"hello"},
		},
		{
			name:     "精确边界",
			text:     "hello",
			maxBytes: 5,
			expected: []string{"hello"},
		},
		{
			name:     "ASCII分块",
			text:     "abcdefgh",
			maxBytes: 3,
			expected: []string{"abc", "def", "gh"},
		},
		{
			name:     "中文UTF-8不截断",
			text:     "你好世界",       // 每个中文字符3字节，共12字节
			maxBytes: 5,
			expected: []string{"你", "好", "世", "界"}, // 每块只能放一个中文字符(3字节)
		},
		{
			name:     "空文本",
			text:     "",
			maxBytes: 10,
			expected: []string{""},
		},
		{
			name:     "maxBytes为0",
			text:     "hello",
			maxBytes: 0,
			expected: []string{"hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ChunkText(tt.text, tt.maxBytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}
