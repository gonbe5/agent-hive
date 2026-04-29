package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// ─── Binary 发现 ─────────────────────────────────────────────────────────────
//
// 设计原则：正向缓存（cache-on-found）
//   - 找到 binary → 永久缓存路径，后续调用无需重复 LookPath
//   - 未找到       → 不缓存，每次调用重新检测
//
// 这样设计的原因：agent 可以通过 bash 工具自主安装 agent-browser，
// 安装完成后立即重试工具调用时应能检测到，而不是被旧的"未找到"结果永久锁死。
// （sync.Once 的语义与此冲突，故不使用）

var (
	agentBrowserMu  sync.RWMutex
	agentBrowserBin string // 非空 = 已找到并缓存；空 = 需要重新检测
)

// lookupAgentBrowser 查找 agent-browser 二进制路径。
// 找到则缓存并返回 (path, true)；未找到则返回 ("", false)（不缓存，支持重试）。
func lookupAgentBrowser() (string, bool) {
	// 快速路径：读锁检查缓存
	agentBrowserMu.RLock()
	if agentBrowserBin != "" {
		p := agentBrowserBin
		agentBrowserMu.RUnlock()
		return p, true
	}
	agentBrowserMu.RUnlock()

	// 慢速路径：执行 LookPath（微秒级，不阻塞）
	found := ""
	if p, err := exec.LookPath("agent-browser"); err == nil {
		found = p
	} else if p := os.Getenv("AGENT_BROWSER_PATH"); p != "" {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			found = p
		}
	}

	if found == "" {
		return "", false // 未找到，不缓存
	}

	// 写锁：缓存已找到的路径
	agentBrowserMu.Lock()
	agentBrowserBin = found
	agentBrowserMu.Unlock()
	return found, true
}

// IsAgentBrowserAvailable 检查 agent-browser CLI 是否当前可用。
func IsAgentBrowserAvailable() bool {
	_, ok := lookupAgentBrowser()
	return ok
}

// agentBrowserNotInstalledError 是工具返回给 agent 的标准错误消息。
// 格式设计为可操作的：agent 看到后可直接调用 bash 工具执行安装命令。
const agentBrowserNotInstalledError = `agent-browser 未安装，无法执行浏览器操作。
请通过 bash 工具运行以下命令安装，安装完成后重试：
  npm install -g agent-browser`

// ─── 响应结构 ─────────────────────────────────────────────────────────────────

// agentBrowserResult 单条命令的响应结构
type agentBrowserResult struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Type    string          `json:"type,omitempty"` // 错误类型（可选）
}

// ─── Batch 执行 ───────────────────────────────────────────────────────────────

// runAgentBrowserBatch 通过 batch 模式在单次进程调用中执行多条 agent-browser 命令。
//
//   - sessionName：会话名称（相同名称复用同一 Chrome daemon）
//   - idleTimeoutMs：daemon 空闲超时（ms），超时后 Chrome 自动退出
//   - bailOnError：遇到失败命令立即停止（适合链式依赖场景）
//   - commands：命令列表，例如 [["open","https://..."],["get","text","body"]]
func runAgentBrowserBatch(
	ctx context.Context,
	sessionName string,
	idleTimeoutMs int,
	bailOnError bool,
	commands [][]string,
) ([]agentBrowserResult, error) {
	bin, ok := lookupAgentBrowser()
	if !ok {
		return nil, errors.New(agentBrowserNotInstalledError)
	}

	input, err := json.Marshal(commands)
	if err != nil {
		return nil, fmt.Errorf("序列化命令失败: %w", err)
	}

	args := []string{
		"--session", sessionName,
		"--json",
		fmt.Sprintf("--idle-timeout-ms=%d", idleTimeoutMs),
		"batch", "--json",
	}
	if bailOnError {
		args = append(args, "--bail")
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = bytes.NewReader(input)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if stderr != "" {
				return nil, fmt.Errorf("agent-browser 退出码 %d: %s", exitErr.ExitCode(), stderr)
			}
			return nil, fmt.Errorf("agent-browser 退出码 %d", exitErr.ExitCode())
		}
		return nil, fmt.Errorf("agent-browser 执行失败: %w", err)
	}

	return parseBatchOutput(out)
}

// parseBatchOutput 解析 batch 命令输出，兼容两种格式：
//
//	格式 A（envelope）：{"success":true,"data":[...]}
//	格式 B（array）  ：[{...},{...}]
func parseBatchOutput(out []byte) ([]agentBrowserResult, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("agent-browser 返回空输出")
	}

	// 格式 A：envelope 包装
	if out[0] == '{' {
		var envelope struct {
			Success bool            `json:"success"`
			Data    json.RawMessage `json:"data,omitempty"`
			Error   string          `json:"error,omitempty"`
		}
		if err := json.Unmarshal(out, &envelope); err != nil {
			return nil, fmt.Errorf("解析 batch envelope 失败: %w", err)
		}
		if !envelope.Success {
			return nil, fmt.Errorf("agent-browser batch 失败: %s", envelope.Error)
		}
		var results []agentBrowserResult
		if err := json.Unmarshal(envelope.Data, &results); err != nil {
			return nil, fmt.Errorf("解析 batch data 数组失败: %w", err)
		}
		return results, nil
	}

	// 格式 B：直接数组
	if out[0] == '[' {
		var results []agentBrowserResult
		if err := json.Unmarshal(out, &results); err != nil {
			return nil, fmt.Errorf("解析 batch 数组失败: %w", err)
		}
		return results, nil
	}

	return nil, fmt.Errorf("无法识别的 agent-browser 输出格式（首字节: %q）", out[0])
}

// ─── Webfetch 辅助 ────────────────────────────────────────────────────────────

// fetchViaAgentBrowser 使用 agent-browser 获取页面文本内容。
// 适用于 JS 渲染的 SPA 和动态加载内容。
// 失败时调用方应降级到 HTTP 方案。
func fetchViaAgentBrowser(ctx context.Context, pageURL string, logger *zap.Logger) (string, error) {
	commands := [][]string{
		{"open", pageURL},
		{"get", "text", "body"},
	}

	// 每次调用使用独立 session，避免并发 webfetch 调用互相干扰（两个 open 命令交错导致返回错误页面内容）
	results, err := runAgentBrowserBatch(ctx, generateAgentBrowserSession("ab_webfetch"), 60000, true, commands)
	if err != nil {
		return "", err
	}

	if len(results) < 2 {
		return "", fmt.Errorf("期望 2 条响应，实际得到 %d 条", len(results))
	}

	last := results[len(results)-1]
	if !last.Success {
		return "", fmt.Errorf("获取页面文本失败: %s", last.Error)
	}

	var textData struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(last.Data, &textData); err != nil {
		// data 可能直接是字符串
		var s string
		if err2 := json.Unmarshal(last.Data, &s); err2 != nil {
			return "", fmt.Errorf("解析文本数据失败: %w", err)
		}
		return s, nil
	}

	logger.Debug("agent-browser webfetch 成功",
		zap.String("url", pageURL),
		zap.Int("content_length", len(textData.Text)))

	return textData.Text, nil
}

// ─── browser_interact 工具注册 ────────────────────────────────────────────────

// browserInteractInput browser_interact 工具输入
type browserInteractInput struct {
	Session  string                   `json:"session,omitempty"`
	Commands []browserInteractCommand `json:"commands"`
	Timeout  int                      `json:"timeout,omitempty"` // 秒，默认 60
}

// browserInteractCommand 单条交互命令
type browserInteractCommand struct {
	Action    string `json:"action"`               // navigate/snapshot/click/fill/eval/wait/screenshot/close
	URL       string `json:"url,omitempty"`        // navigate 时使用
	Selector  string `json:"selector,omitempty"`   // click/fill/wait 时使用（@e1 或 CSS）
	Value     string `json:"value,omitempty"`      // fill 时使用
	Script    string `json:"script,omitempty"`     // eval 时使用
	TimeoutMs int    `json:"timeout_ms,omitempty"` // wait 时使用（毫秒）
}

// registerBrowserInteract 注册 browser_interact 工具
func registerBrowserInteract(host *mcphost.Host, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session": map[string]any{
				"type":        "string",
				"description": "会话名称，同一名称共享浏览器状态（cookie、登录态）。不传则自动生成临时会话。",
			},
			"commands": map[string]any{
				"type":        "array",
				"description": "按顺序执行的浏览器命令列表",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"enum":        []string{"navigate", "snapshot", "click", "fill", "eval", "wait", "screenshot", "close"},
							"description": "命令类型：navigate=导航到URL, snapshot=获取页面无障碍树, click=点击元素, fill=填写表单, eval=执行JS, wait=等待元素/时间, screenshot=截图, close=关闭会话",
						},
						"url":        map[string]any{"type": "string", "description": "navigate 时的目标 URL"},
						"selector":   map[string]any{"type": "string", "description": "元素引用（如 @e1）或 CSS 选择器"},
						"value":      map[string]any{"type": "string", "description": "fill 命令的填写内容"},
						"script":     map[string]any{"type": "string", "description": "eval 命令的 JavaScript 表达式"},
						"timeout_ms": map[string]any{"type": "integer", "description": "wait 命令的等待时长（毫秒）"},
					},
					"required": []string{"action"},
				},
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "整体超时秒数（默认 60）",
			},
		},
		"required": []string{"commands"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "browser_interact",
			Description: "使用真实 Chrome 浏览器与网页交互，支持 JS 渲染页面、点击按钮、填写表单、截图等操作。当 webfetch 无法获取内容或需要用户交互（如登录）时使用。需要先安装 agent-browser（npm install -g agent-browser）。",
			InputSchema: schema,
			Core:        true,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params browserInteractInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			if !IsAgentBrowserAvailable() {
				return errorResult(agentBrowserNotInstalledError), nil
			}

			if len(params.Commands) == 0 {
				return errorResult("commands 不能为空"), nil
			}

			// 确定 session 名称
			sessionName := params.Session
			if sessionName == "" {
				sessionName = generateAgentBrowserSession("ab_interact")
			}

			// 超时设置
			timeoutSec := params.Timeout
			if timeoutSec <= 0 {
				timeoutSec = 60
			}
			interactCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
			defer cancel()

			// 转换命令
			batchCmds, err := convertToBatchCommands(params.Commands)
			if err != nil {
				return errorResult("命令转换失败: " + err.Error()), nil
			}

			// 执行
			results, err := runAgentBrowserBatch(interactCtx, sessionName, 300000, false, batchCmds)
			if err != nil {
				logger.Warn("browser_interact 执行失败",
					zap.String("session", sessionName),
					zap.Error(err))
				return errorResult("浏览器操作失败: " + err.Error()), nil
			}

			// 格式化输出
			output, err := formatInteractResults(params.Commands, results)
			if err != nil {
				return errorResult("格式化结果失败: " + err.Error()), nil
			}

			logger.Info("browser_interact 执行成功",
				zap.String("session", sessionName),
				zap.Int("commands", len(params.Commands)))

			return textResult(output), nil
		},
	)
}

// convertToBatchCommands 将 browserInteractCommand 列表转换为 agent-browser batch 格式
func convertToBatchCommands(commands []browserInteractCommand) ([][]string, error) {
	var batch [][]string
	for _, cmd := range commands {
		var args []string
		switch cmd.Action {
		case "navigate":
			if cmd.URL == "" {
				return nil, fmt.Errorf("navigate 命令缺少 url 字段")
			}
			args = []string{"open", cmd.URL}
		case "snapshot":
			args = []string{"snapshot", "-i"}
		case "click":
			if cmd.Selector == "" {
				return nil, fmt.Errorf("click 命令缺少 selector 字段")
			}
			args = []string{"click", cmd.Selector}
		case "fill":
			if cmd.Selector == "" {
				return nil, fmt.Errorf("fill 命令缺少 selector 字段")
			}
			if cmd.Value == "" {
				return nil, fmt.Errorf("fill 命令缺少 value 字段")
			}
			args = []string{"fill", cmd.Selector, cmd.Value}
		case "eval":
			if cmd.Script == "" {
				return nil, fmt.Errorf("eval 命令缺少 script 字段")
			}
			args = []string{"eval", cmd.Script}
		case "wait":
			if cmd.Selector != "" {
				args = []string{"wait", cmd.Selector}
			} else if cmd.TimeoutMs > 0 {
				args = []string{"wait", fmt.Sprintf("%d", cmd.TimeoutMs)}
			} else {
				return nil, fmt.Errorf("wait 命令需要 selector 或 timeout_ms 之一")
			}
		case "screenshot":
			tmpPath := fmt.Sprintf("/tmp/ab_screenshot_%d.png", time.Now().UnixNano())
			args = []string{"screenshot", tmpPath}
		case "close":
			args = []string{"close"}
		default:
			return nil, fmt.Errorf("未知 action: %s", cmd.Action)
		}
		batch = append(batch, args)
	}
	return batch, nil
}

// formatInteractResults 将执行结果格式化为可读文本
func formatInteractResults(commands []browserInteractCommand, results []agentBrowserResult) (string, error) {
	var sb bytes.Buffer

	for i, result := range results {
		action := ""
		if i < len(commands) {
			action = commands[i].Action
		}

		if !result.Success {
			fmt.Fprintf(&sb, "[%s] 失败: %s\n", action, result.Error)
			continue
		}

		switch action {
		case "snapshot":
			var data struct {
				Snapshot string `json:"snapshot"`
				Origin   string `json:"origin"`
			}
			if err := json.Unmarshal(result.Data, &data); err == nil {
				fmt.Fprintf(&sb, "[snapshot] 页面: %s\n%s\n", data.Origin, data.Snapshot)
			}
		case "navigate":
			var data struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			}
			if err := json.Unmarshal(result.Data, &data); err == nil {
				fmt.Fprintf(&sb, "[navigate] 已导航到: %s (%s)\n", data.URL, data.Title)
			}
		case "eval":
			var val any
			if err := json.Unmarshal(result.Data, &val); err == nil {
				fmt.Fprintf(&sb, "[eval] 结果: %v\n", val)
			}
		case "screenshot":
			var data struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(result.Data, &data); err == nil {
				fmt.Fprintf(&sb, "[screenshot] 截图保存至: %s\n", data.Path)
			} else {
				fmt.Fprintf(&sb, "[screenshot] 截图已完成\n")
			}
		case "click":
			fmt.Fprintf(&sb, "[click] 点击成功\n")
		case "fill":
			fmt.Fprintf(&sb, "[fill] 填写成功\n")
		case "wait":
			fmt.Fprintf(&sb, "[wait] 等待完成\n")
		case "close":
			fmt.Fprintf(&sb, "[close] 会话已关闭\n")
		default:
			if len(result.Data) > 0 && string(result.Data) != "null" {
				fmt.Fprintf(&sb, "[%s] %s\n", action, string(result.Data))
			} else {
				fmt.Fprintf(&sb, "[%s] 成功\n", action)
			}
		}
	}

	return sb.String(), nil
}

// generateAgentBrowserSession 生成唯一 session 名称
func generateAgentBrowserSession(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
