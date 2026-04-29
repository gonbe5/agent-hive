package cli

import (
	"fmt"
	"os"
	"strings"
)

// Command 表示一个 CLI 命令
type Command struct {
	Name        string
	Description string
	Run         func(args []string) error
}

// CLIOptions 包含解析后的 CLI 选项
type CLIOptions struct {
	Request     string
	Interactive bool
	ConfigPath  string
	Model       string
	BaseURL     string
	APIKey      string
	LogLevel    string
	HITL        bool
	ACP         bool // 以 ACP 协议模式启动（IDE 零配置接入）
	Verbose     bool // 启用详细日志到控制台
}

// ParseArgs 解析 CLI 参数并返回结构化选项
// 标志优先级: CLI 标志 > 环境变量 > 配置文件 > 默认值
func ParseArgs(args []string) CLIOptions {
	opts := CLIOptions{}

	if len(args) == 0 {
		opts.Interactive = true
		return opts
	}

	var remaining []string
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		// 交互模式
		case arg == "-i" || arg == "--interactive":
			opts.Interactive = true

		// 帮助
		case arg == "-h" || arg == "--help":
			printUsage()
			os.Exit(0)

		// 版本
		case arg == "-v" || arg == "--version":
			fmt.Println("agents-hive v1.0.0")
			os.Exit(0)

		// 模型: --model <value> 或 -m <value>
		case arg == "-m" || arg == "--model":
			if i+1 < len(args) {
				i++
				opts.Model = args[i]
			} else {
				fmt.Fprintf(os.Stderr, "错误: %s 需要一个值\n", arg)
				os.Exit(1)
			}
		case strings.HasPrefix(arg, "--model="):
			opts.Model = strings.TrimPrefix(arg, "--model=")

		// 配置文件: --config <path> 或 -c <path>
		case arg == "-c" || arg == "--config":
			if i+1 < len(args) {
				i++
				opts.ConfigPath = args[i]
			} else {
				fmt.Fprintf(os.Stderr, "错误: %s 需要一个值\n", arg)
				os.Exit(1)
			}
		case strings.HasPrefix(arg, "--config="):
			opts.ConfigPath = strings.TrimPrefix(arg, "--config=")

		// Base URL: --base-url <url>
		case arg == "--base-url":
			if i+1 < len(args) {
				i++
				opts.BaseURL = args[i]
			} else {
				fmt.Fprintf(os.Stderr, "错误: %s 需要一个值\n", arg)
				os.Exit(1)
			}
		case strings.HasPrefix(arg, "--base-url="):
			opts.BaseURL = strings.TrimPrefix(arg, "--base-url=")

		// API 密钥: --api-key <key>
		case arg == "--api-key":
			if i+1 < len(args) {
				i++
				opts.APIKey = args[i]
			} else {
				fmt.Fprintf(os.Stderr, "错误: %s 需要一个值\n", arg)
				os.Exit(1)
			}
		case strings.HasPrefix(arg, "--api-key="):
			opts.APIKey = strings.TrimPrefix(arg, "--api-key=")

		// 日志级别: --log-level <level>
		case arg == "--log-level":
			if i+1 < len(args) {
				i++
				opts.LogLevel = args[i]
			} else {
				fmt.Fprintf(os.Stderr, "错误: %s 需要一个值\n", arg)
				os.Exit(1)
			}
		case strings.HasPrefix(arg, "--log-level="):
			opts.LogLevel = strings.TrimPrefix(arg, "--log-level=")

		// HITL 模式: --hitl
		case arg == "--hitl":
			opts.HITL = true

		// ACP 协议模式: --acp
		case arg == "--acp":
			opts.ACP = true

		// 详细日志: --verbose
		case arg == "--verbose":
			opts.Verbose = true

		default:
			remaining = append(remaining, arg)
		}
	}

	if len(remaining) > 0 && !opts.Interactive {
		opts.Request = strings.Join(remaining, " ")
	}

	if opts.Request == "" && !opts.Interactive {
		opts.Interactive = true
	}

	return opts
}

func printUsage() {
	fmt.Println(`agents-hive - 多 Agent AI 系统

用法:
  claw [选项] <请求>
  claw -i

选项:
  -i, --interactive          启动交互模式
  -m, --model <model>        LLM 模型名称 (例如 gpt-5.2, deepseek-chat, claude-3-5-sonnet)
  -c, --config <path>        配置文件路径 (JSON)
      --base-url <url>       LLM API Base URL
      --api-key <key>        LLM API 密钥
      --log-level <level>    日志级别 (debug, info, warn, error)
      --hitl                 启用人机协同模式 (计划审批、步骤确认)
      --acp                  以 ACP 协议模式启动 (供 IDE 零配置接入)
      --verbose              启用详细日志到控制台 (覆盖 console_level 配置)
  -h, --help                 显示帮助
  -v, --version              显示版本

环境变量:
  CLAW_MODEL                 LLM 模型名称 (被 --model 覆盖)
  CLAW_BASE_URL              LLM API Base URL (被 --base-url 覆盖)
  CLAW_API_KEY               LLM API 密钥 (被 --api-key 覆盖)
  OPENAI_API_KEY             LLM API 密钥 (备用)
  OPENAI_BASE_URL            LLM API Base URL (备用)

示例:
  claw "审查这段 Go 代码的安全问题"
  claw --hitl "审查这段 Go 代码的安全问题"
  claw -m deepseek-chat --base-url https://api.deepseek.com/v1 "分析这段代码"
  claw -c config.json -i
  CLAW_MODEL=gpt-5.2 claw "研究 AI 趋势"`)
}
