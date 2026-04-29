package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"简单命令", "ls -la", []string{"ls", "-la"}},
		{"双引号", `echo "hello world"`, []string{"echo", "hello world"}},
		{"单引号", `echo 'hello world'`, []string{"echo", "hello world"}},
		{"转义空格", `echo hello\ world`, []string{"echo", "hello world"}},
		{"混合引号", `git commit -m "feat: 新功能"`, []string{"git", "commit", "-m", "feat: 新功能"}},
		{"空输入", "", nil},
		{"多空格", "ls   -la   /tmp", []string{"ls", "-la", "/tmp"}},
		{"管道符", "cat file | grep test", []string{"cat", "file", "|", "grep", "test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
