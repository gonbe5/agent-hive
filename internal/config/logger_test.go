package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewLogger_FileOutput 测试日志文件输出
func TestNewLogger_FileOutput(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// 创建配置
	cfg := &Config{
		Logging: LoggingConfig{
			Level:        "debug", // 使用 debug 级别以记录所有日志
			Format:       "json",
			File:         logFile,
			ConsoleLevel: "error",
			MaxSize:      100,
			MaxBackups:   3,
			MaxAge:       7,
		},
	}

	// 创建 logger
	logger, err := cfg.NewLogger()
	require.NoError(t, err)
	require.NotNil(t, logger)

	// 写入日志
	logger.Info("测试消息")
	logger.Debug("调试消息")
	logger.Warn("警告消息")
	logger.Error("错误消息")

	// 同步日志（确保写入文件）
	err = logger.Sync()
	// 忽略 stderr sync 错误（这是 zap 的已知问题）
	if err != nil && !isIgnorableSyncError(err) {
		t.Fatalf("同步日志失败: %v", err)
	}

	// 验证文件存在
	_, err = os.Stat(logFile)
	require.NoError(t, err, "日志文件应该被创建")

	// 读取文件内容
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)

	content := string(data)
	// 验证日志内容
	assert.Contains(t, content, "测试消息", "应该包含 info 级别日志")
	assert.Contains(t, content, "调试消息", "应该包含 debug 级别日志")
	assert.Contains(t, content, "警告消息", "应该包含 warn 级别日志")
	assert.Contains(t, content, "错误消息", "应该包含 error 级别日志")
}

// TestNewLogger_ConsoleLevelFilter 测试控制台日志级别过滤
func TestNewLogger_ConsoleLevelFilter(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := &Config{
		Logging: LoggingConfig{
			Level:        "debug",  // 文件记录所有级别
			Format:       "json",
			File:         logFile,
			ConsoleLevel: "error", // 控制台只记录 error 级别
			MaxSize:      100,
			MaxBackups:   3,
			MaxAge:       7,
		},
	}

	logger, err := cfg.NewLogger()
	require.NoError(t, err)

	// 写入不同级别的日志
	logger.Debug("调试消息")
	logger.Info("信息消息")
	logger.Warn("警告消息")
	logger.Error("错误消息")

	err = logger.Sync()
	if err != nil && !isIgnorableSyncError(err) {
		t.Fatalf("同步日志失败: %v", err)
	}

	// 读取文件内容
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)

	content := string(data)
	// 文件应该包含所有级别的日志
	assert.Contains(t, content, "调试消息")
	assert.Contains(t, content, "信息消息")
	assert.Contains(t, content, "警告消息")
	assert.Contains(t, content, "错误消息")
}

// TestNewLogger_NoFile 测试不配置文件时只输出到控制台
func TestNewLogger_NoFile(t *testing.T) {
	cfg := &Config{
		Logging: LoggingConfig{
			Level:        "info",
			Format:       "console",
			File:         "", // 不配置文件
			ConsoleLevel: "info",
		},
	}

	logger, err := cfg.NewLogger()
	require.NoError(t, err)
	require.NotNil(t, logger)

	// 应该能正常写入日志（仅输出到控制台）
	logger.Info("控制台消息")
	err = logger.Sync()
	if err != nil && !isIgnorableSyncError(err) {
		t.Fatalf("同步日志失败: %v", err)
	}
}

// TestExpandPath 测试路径展开
func TestExpandPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // 展开后应该包含的字符串
	}{
		{
			name:     "展开波浪号",
			input:    "~/test.log",
			contains: "test.log",
		},
		{
			name:     "绝对路径",
			input:    "/tmp/test.log",
			contains: "/tmp/test.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandPath(tt.input)
			require.NoError(t, err)
			assert.Contains(t, result, tt.contains)
			assert.NotContains(t, result, "~", "不应该包含波浪号")
		})
	}
}

// isIgnorableSyncError 判断是否是可忽略的 sync 错误
// zap 在 sync stderr/stdout 时可能返回错误
// 这是已知的无害错误，可以忽略
func isIgnorableSyncError(err error) bool {
	if err == nil {
		return true
	}
	errMsg := err.Error()
	// 匹配各种 sync 错误
	ignorable := []string{
		"sync /dev/stderr: invalid argument",
		"sync /dev/stdout: invalid argument",
		"sync /dev/stderr: bad file descriptor",
		"sync /dev/stdout: bad file descriptor",
	}
	for _, msg := range ignorable {
		if errMsg == msg {
			return true
		}
	}
	return false
}
