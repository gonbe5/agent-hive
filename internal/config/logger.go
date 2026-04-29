package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// NewLogger 从日志配置创建 zap.Logger
// 支持同时输出到文件和控制台，并支持日志轮转
func (c *Config) NewLogger() (*zap.Logger, error) {
	// 解析主日志级别（文件日志级别）
	level, err := zapcore.ParseLevel(c.Logging.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	// 编码器配置
	encoderCfg := zap.NewProductionEncoderConfig()
	if c.Logging.Format == "console" {
		encoderCfg = zap.NewDevelopmentEncoderConfig()
	}
	// 使用 ISO8601 时间格式替代默认的 Unix 时间戳
	encoderCfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")

	// 创建编码器
	var encoder zapcore.Encoder
	if c.Logging.Format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	// 构建输出核心
	var cores []zapcore.Core

	// 1. 文件输出（如果配置了）
	if c.Logging.File != "" {
		filePath, err := expandPath(c.Logging.File)
		if err != nil {
			return nil, errs.Wrap(errs.CodeConfigInvalid, "展开日志文件路径失败", err)
		}
		filePath = filepath.Clean(filePath)

		// 确保日志目录存在
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, errs.Wrap(errs.CodeConfigInvalid, "创建日志目录失败", err)
		}

		// 确保 lumberjack 配置有合理的默认值（防止零值）
		maxSize := c.Logging.MaxSize
		if maxSize <= 0 {
			maxSize = 100 // 默认 100MB
		}
		maxBackups := c.Logging.MaxBackups
		if maxBackups < 0 {
			maxBackups = 3 // 默认保留 3 个
		}
		maxAge := c.Logging.MaxAge
		if maxAge < 0 {
			maxAge = 7 // 默认 7 天
		}

		// Lumberjack 日志轮转
		fileWriter := &lumberjack.Logger{
			Filename:   filePath,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge,
			Compress:   true, // 压缩旧日志
		}

		fileCore := zapcore.NewCore(
			encoder,
			zapcore.AddSync(fileWriter),
			level, // 文件记录所有级别的日志
		)
		cores = append(cores, fileCore)
	}

	// 2. 控制台输出（根据 ConsoleLevel 控制）
	consoleLevel := level
	if c.Logging.ConsoleLevel != "" {
		if lvl, err := zapcore.ParseLevel(c.Logging.ConsoleLevel); err == nil {
			consoleLevel = lvl
		} else {
			// ConsoleLevel 解析失败，使用文件级别并记录警告（但不能用 logger，只能用 stderr）
			fmt.Fprintf(os.Stderr, "警告: 无效的 console_level %q, 使用 %q\n",
				c.Logging.ConsoleLevel, level.String())
		}
	}

	consoleCore := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stderr),
		consoleLevel, // 控制台只显示高级别日志
	)
	cores = append(cores, consoleCore)

	// 合并输出核心
	core := zapcore.NewTee(cores...)
	return zap.New(core), nil
}

// expandPath 展开路径中的 ~ 和环境变量
// 如果展开失败，返回错误
func expandPath(path string) (string, error) {
	// 展开 ~
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("无法获取用户主目录: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	// 展开环境变量
	path = os.ExpandEnv(path)
	return path, nil
}
