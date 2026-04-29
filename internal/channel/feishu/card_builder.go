package feishu

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CardStatus 卡片终态。
type CardStatus string

const (
	CardStatusGenerating CardStatus = "generating"
	// CardStatusThinking 空闲心跳态：事件流静默超阈值时短暂切入，
	// 让用户知道 Agent 仍在推理 / 等工具响应，不是结束。
	CardStatusThinking CardStatus = "thinking"
	CardStatusDone     CardStatus = "done"
	CardStatusError    CardStatus = "error"
)

// ToolLine 卡片中一个工具调用行。
// renderer 按 tool_call_id 维护，同一 id 的后续事件 in-place 更新这一行。
type ToolLine struct {
	ToolName string
	Status   string        // "start" | "success" | "error"
	Duration time.Duration // success/error 时填
	Summary  string        // 一行简介，如参数摘要或错误信息
}

// HITLButton 卡片下方 HITL 审批按钮。
// value 透传 request_id，callback 路由据此定位 InputRequest。
type HITLButton struct {
	Label     string
	Action    string // "approve" | "reject"
	RequestID string
}

// CardState 渲染一张卡片所需的完整状态。
// renderer 持有一个 CardState 实例，每次事件更新后调 BuildCardJSON → PatchCard。
type CardState struct {
	Title       string // 可选覆盖；为空时按 Status 自动推导
	Body        string
	ToolLines   []ToolLine
	HITLButtons []HITLButton
	Status      CardStatus
}

// BuildCardJSON 按 CardState 生成飞书互动卡片 JSON（v1 schema）。
// 飞书 PatchMessage 要求整卡替换，这里输出的字符串可直接作为 cardJSON 入参。
func BuildCardJSON(state CardState) string {
	title := state.Title
	if title == "" {
		title = titleForStatus(state.Status)
	}

	elements := make([]map[string]any, 0, 4+len(state.ToolLines))

	if strings.TrimSpace(state.Body) != "" {
		elements = append(elements, map[string]any{
			"tag":     "markdown",
			"content": state.Body,
		})
	}

	if len(state.ToolLines) > 0 {
		if len(elements) > 0 {
			elements = append(elements, map[string]any{"tag": "hr"})
		}
		elements = append(elements, map[string]any{
			"tag":     "markdown",
			"content": renderToolLines(state.ToolLines),
		})
	}

	if len(state.HITLButtons) > 0 {
		// 仅当前面已有内容时才插分隔线，避免 HITL-only 卡片出现孤儿 hr。
		if len(elements) > 0 {
			elements = append(elements, map[string]any{"tag": "hr"})
		}
		elements = append(elements, map[string]any{
			"tag":     "action",
			"actions": renderHITLButtons(state.HITLButtons),
		})
	}

	// 兜底：elements 不能为空，否则飞书 Patch 会 400
	if len(elements) == 0 {
		elements = append(elements, map[string]any{
			"tag":     "markdown",
			"content": " ",
		})
	}

	card := map[string]any{
		"config": map[string]any{
			"update_multi":     true,
			"wide_screen_mode": true,
		},
		"header": map[string]any{
			"title": map[string]any{
				"tag":     "plain_text",
				"content": title,
			},
			"template": templateForStatus(state.Status),
		},
		"elements": elements,
	}

	b, err := json.Marshal(card)
	if err != nil {
		// 理论上只有 map[string]any 包含不可序列化内容时才触发，这里兜底成最小可用卡片
		fallback, _ := json.Marshal(map[string]any{
			"config":   map[string]any{"update_multi": true},
			"header":   map[string]any{"title": map[string]any{"tag": "plain_text", "content": title}},
			"elements": []map[string]any{{"tag": "markdown", "content": " "}},
		})
		return string(fallback)
	}
	return string(b)
}

func titleForStatus(s CardStatus) string {
	switch s {
	case CardStatusDone:
		return "✅ 完成"
	case CardStatusError:
		return "❌ 失败"
	case CardStatusThinking:
		return "💭 思考中…"
	default:
		return "🤖 生成中…"
	}
}

func templateForStatus(s CardStatus) string {
	switch s {
	case CardStatusDone:
		return "green"
	case CardStatusError:
		return "red"
	default:
		// generating / thinking 同色（blue），仅 title 文案区分，避免频繁变色晃眼
		return "blue"
	}
}

func renderToolLines(lines []ToolLine) string {
	var b strings.Builder
	for _, l := range lines {
		icon := toolIcon(l.Status)
		b.WriteString(fmt.Sprintf("%s 调用工具：**%s**", icon, l.ToolName))
		if l.Status != "start" && l.Duration > 0 {
			b.WriteString(fmt.Sprintf("  `%s`", l.Duration.Round(10*time.Millisecond)))
		}
		if strings.TrimSpace(l.Summary) != "" {
			b.WriteString("\n  ")
			b.WriteString(l.Summary)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func toolIcon(status string) string {
	switch status {
	case "success":
		return "✅"
	case "error":
		return "❌"
	default:
		return "🔧"
	}
}

func renderHITLButtons(buttons []HITLButton) []map[string]any {
	out := make([]map[string]any, 0, len(buttons))
	for _, btn := range buttons {
		var btnType string
		switch btn.Action {
		case "approve":
			btnType = "primary"
		case "reject":
			btnType = "danger"
		default:
			btnType = "default"
		}
		out = append(out, map[string]any{
			"tag": "button",
			"text": map[string]any{
				"tag":     "plain_text",
				"content": btn.Label,
			},
			"type": btnType,
			"value": map[string]any{
				"action":     btn.Action,
				"request_id": btn.RequestID,
			},
		})
	}
	return out
}
