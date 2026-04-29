package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewLogger_ZeroValueProtection 测试零值保护
func TestNewLogger_ZeroValueProtection(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// 使用零值配置（模拟用户明确设置为 0）
	cfg := &Config{
		Logging: LoggingConfig{
			Level:        "info",
			Format:       "json",
			File:         logFile,
			ConsoleLevel: "error",
			MaxSize:      0, // 零值
			MaxBackups:   0, // 零值
			MaxAge:       0, // 零值
		},
	}

	// 创建 logger 不应该失败
	logger, err := cfg.NewLogger()
	require.NoError(t, err)
	require.NotNil(t, logger)

	// 写入日志验证正常工作
	logger.Info("测试消息")
	err = logger.Sync()
	if err != nil && !isIgnorableSyncError(err) {
		t.Fatalf("同步日志失败: %v", err)
	}

	// 验证文件存在
	_, err = os.Stat(logFile)
	require.NoError(t, err)
}

// TestNewLogger_InvalidConsoleLevel 测试无效的 ConsoleLevel
func TestNewLogger_InvalidConsoleLevel(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := &Config{
		Logging: LoggingConfig{
			Level:        "info",
			Format:       "json",
			File:         logFile,
			ConsoleLevel: "invalid_level", // 无效级别
			MaxSize:      100,
			MaxBackups:   3,
			MaxAge:       7,
		},
	}

	// 创建 logger 应该成功（回退到文件级别）
	logger, err := cfg.NewLogger()
	require.NoError(t, err)
	require.NotNil(t, logger)

	logger.Info("测试消息")
	err = logger.Sync()
	if err != nil && !isIgnorableSyncError(err) {
		t.Fatalf("同步日志失败: %v", err)
	}
}

// TestExpandPath_Success 测试路径展开成功
func TestExpandPath_Success(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "波浪号路径",
			input: "~/test.log",
		},
		{
			name:  "绝对路径",
			input: "/tmp/test.log",
		},
		{
			name:  "相对路径",
			input: "./test.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandPath(tt.input)
			require.NoError(t, err)
			assert.NotEmpty(t, result)

			// 波浪号应该被展开
			if tt.input == "~/test.log" {
				assert.NotContains(t, result, "~")
				assert.Contains(t, result, "test.log")
			}
		})
	}
}
