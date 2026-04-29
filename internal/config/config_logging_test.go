package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoad_PartialLoggingConfig 测试部分 logging 配置不会丢失默认值
func TestLoad_PartialLoggingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "partial.json")

	// 只包含部分 logging 字段的配置
	partialCfg := map[string]interface{}{
		"logging": map[string]interface{}{
			"level":  "info",
			"format": "json",
			// 注意：没有 file, console_level, max_size 等字段
		},
	}

	data, err := json.Marshal(partialCfg)
	require.NoError(t, err)
	err = os.WriteFile(cfgPath, data, 0600)
	require.NoError(t, err)

	// 加载配置
	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	// 验证明确指定的字段
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)

	// ❌ 关键问题：未指定的字段应该保留默认值，而不是零值
	t.Logf("File: %q", cfg.Logging.File)
	t.Logf("ConsoleLevel: %q", cfg.Logging.ConsoleLevel)
	t.Logf("MaxSize: %d", cfg.Logging.MaxSize)
	t.Logf("MaxBackups: %d", cfg.Logging.MaxBackups)
	t.Logf("MaxAge: %d", cfg.Logging.MaxAge)

	assert.NotEmpty(t, cfg.Logging.File, "File 应该有默认值，不应该是空字符串")
	assert.NotEmpty(t, cfg.Logging.ConsoleLevel, "ConsoleLevel 应该有默认值")
	assert.NotEqual(t, 0, cfg.Logging.MaxSize, "MaxSize 应该有默认值，不应该是 0")
	assert.NotEqual(t, 0, cfg.Logging.MaxBackups, "MaxBackups 应该有默认值")
	assert.NotEqual(t, 0, cfg.Logging.MaxAge, "MaxAge 应该有默认值")
}
