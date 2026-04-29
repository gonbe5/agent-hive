package cli

import (
	"fmt"
	"os"

	"github.com/chef-guo/agents-hive/internal/master"
)

// formatAndPrintEvent 格式化并打印事件到控制台
// 这个函数处理从 Master 广播的事件，并以用户友好的方式显示
func formatAndPrintEvent(msg master.BroadcastMessage) {
	switch msg.Type {
	case master.EventTypeToolCall:
		// 工具调用事件
		if evt, ok := msg.Payload.(master.ToolCallEvent); ok {
			switch evt.Status {
			case "start":
				fmt.Printf("🔧 调用工具: %s\n", evt.ToolName)
			case "success":
				if evt.Duration > 0 {
					fmt.Printf("✅ 工具完成: %s (%dms)\n", evt.ToolName, evt.Duration)
				}
			case "error":
				fmt.Printf("❌ 工具失败: %s — %s\n", evt.ToolName, evt.Error)
			default:
				fmt.Printf("🔧 工具: %s\n", evt.ToolName)
			}
		}

	case master.EventTypeAgentStart:
		// Agent 启动事件
		if evt, ok := msg.Payload.(master.AgentStartEvent); ok {
			fmt.Printf("🤖 启动 Agent: %s\n", evt.AgentName)
			if evt.TaskDesc != "" {
				fmt.Printf("   任务: %s\n", evt.TaskDesc)
			}
		}

	case master.EventTypeSkillExec:
		// Skill 执行事件
		if evt, ok := msg.Payload.(master.SkillExecEvent); ok {
			fmt.Printf("✨ 执行 Skill: %s\n", evt.SkillName)
			if evt.Args != "" {
				fmt.Printf("   参数: %s\n", evt.Args)
			}
		}

	case master.EventTypeError:
		// 错误消息
		if payload, ok := msg.Payload.(string); ok {
			fmt.Fprintf(os.Stderr, "❌ 错误: %s\n", payload)
		}

	case master.EventTypeMessage:
		// 通用消息
		if payload, ok := msg.Payload.(string); ok {
			fmt.Println(payload)
		}

	case master.EventTypeTaskGroup:
		// 并行任务组事件
		if evt, ok := msg.Payload.(master.TaskGroupEvent); ok {
			switch evt.Status {
			case "started":
				fmt.Printf("🚀 并行任务组 (%d 个任务)\n", evt.Total)
			case "completed":
				fmt.Printf("✅ 并行任务组完成 (%d/%d)\n", evt.Completed, evt.Total)
			case "failed":
				fmt.Printf("❌ 并行任务组失败 (%d/%d)\n", evt.Completed, evt.Total)
			}
		}

	case master.EventTypeTaskProgress:
		// 单个任务进度事件
		if evt, ok := msg.Payload.(map[string]string); ok {
			taskID := evt["task_id"]
			status := evt["status"]
			if errMsg := evt["error"]; errMsg != "" {
				fmt.Printf("  → 任务 %s: %s (%s)\n", taskID, status, errMsg)
			} else {
				fmt.Printf("  → 任务 %s: %s\n", taskID, status)
			}
		}

	case master.EventTypeAgentProgress:
		// SubAgent 工具调用级进度
		if evt, ok := msg.Payload.(map[string]interface{}); ok {
			toolName, _ := evt["tool_name"].(string)
			agentID, _ := evt["agent_id"].(string)
			line := fmt.Sprintf("  🔧 %s (Agent: %s)", toolName, agentID)
			if turn, ok := evt["turn"].(float64); ok {
				if maxTurns, ok := evt["max_turns"].(float64); ok {
					line += fmt.Sprintf(" 第 %d/%d 轮", int(turn), int(maxTurns))
				}
			}
			fmt.Println(line)
		}

	case master.EventTypeEvent:
		// 通用事件：尝试从 payload 中提取 message 字段并显示
		if evt, ok := msg.Payload.(map[string]interface{}); ok {
			if message, ok := evt["message"].(string); ok && message != "" {
				fmt.Printf("📢 事件: %s\n", message)
			} else {
				// 没有 message 字段时，显示事件类型提示
				fmt.Println("📢 收到事件通知")
			}
		} else if payload, ok := msg.Payload.(string); ok && payload != "" {
			// payload 直接是字符串时原样输出
			fmt.Printf("📢 事件: %s\n", payload)
		}

	case master.EventTypeToolListChanged:
		// 工具列表变更通知：显示最新工具数量
		if evt, ok := msg.Payload.(map[string]interface{}); ok {
			toolCount := 0
			if tc, ok := evt["tool_count"].(float64); ok {
				toolCount = int(tc)
			} else if tc, ok := evt["tool_count"].(int); ok {
				// 直接传入 int 类型（非 JSON 反序列化路径）
				toolCount = tc
			}
			fmt.Printf("🔄 工具列表已更新 (当前共 %d 个工具)\n", toolCount)
		} else {
			fmt.Println("🔄 工具列表已更新")
		}

	// 其他事件类型（如 input_request）不在控制台显示
	default:
		// 忽略未知事件类型
	}
}
