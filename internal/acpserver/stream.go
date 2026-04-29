package acpserver

import (
	"context"
	"fmt"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/master"
)

// streamSessionUpdates 订阅 EventBus 并将会话事件推送给 ACP 客户端
// 该函数在 goroutine 中运行，直到 ctx 取消或 conn 断开
func streamSessionUpdates(ctx context.Context, conn *acp.AgentSideConnection, eb *master.EventBus, sessionID string, logger *zap.Logger) {
	subID, ch := eb.Subscribe()
	defer eb.Unsubscribe(subID)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// 仅转发与此会话相关的消息（或无 SessionID 的全局消息）
			if msg.SessionID != "" && msg.SessionID != sessionID {
				continue
			}
			updates := convertToACPUpdates(msg)
			for _, update := range updates {
				if err := conn.SessionUpdate(ctx, acp.SessionNotification{
					SessionId: acp.SessionId(sessionID),
					Update:    update,
				}); err != nil {
					if ctx.Err() != nil {
						return
					}
					logger.Warn("推送 ACP 会话更新失败",
						zap.String("session_id", sessionID),
						zap.Error(err))
				}
			}
		}
	}
}

// convertToACPUpdates 将 BroadcastMessage 转换为 ACP 会话更新
func convertToACPUpdates(msg master.BroadcastMessage) []acp.SessionUpdate {
	switch msg.Type {
	case master.EventTypeMessage:
		// 通用消息：作为 agent 文本推送
		text := extractMessageText(msg.Payload)
		if text != "" {
			return []acp.SessionUpdate{acp.UpdateAgentMessageText(text)}
		}

	case master.EventTypeToolCall:
		// 工具调用事件：推送工具调用状态
		return convertToolCallEvent(msg.Payload)

	case master.EventTypeAgentStart:
		// Agent 启动事件：推送为工具调用（kind=other, status=in_progress）
		return convertAgentStartEvent(msg.Payload)

	case master.EventTypeSkillExec:
		// Skill 执行事件：推送为工具调用（kind=execute）
		return convertSkillExecEvent(msg.Payload)

	case master.EventTypeError:
		// 错误事件：推送为 agent 消息
		text := extractMessageText(msg.Payload)
		if text != "" {
			return []acp.SessionUpdate{acp.UpdateAgentMessageText("错误: " + text)}
		}

	}
	return nil
}

// convertToolCallEvent 将工具调用事件转换为 ACP tool_call 更新
func convertToolCallEvent(payload interface{}) []acp.SessionUpdate {
	m, ok := payload.(map[string]interface{})
	if !ok {
		// 尝试 ToolCallEvent 结构体类型断言
		if evt, ok := payload.(master.ToolCallEvent); ok {
			callID := evt.ToolCallID
			if callID == "" {
				callID = evt.ToolName
			}
			id := acp.ToolCallId(fmt.Sprintf("tc_%s", callID))
			kind := toolKindFromName(evt.ToolName)
			title := evt.ToolName
			// 根据状态返回对应的 ACP 更新
			switch evt.Status {
			case "success":
				return []acp.SessionUpdate{
					acp.UpdateToolCall(id, acp.WithUpdateStatus(acp.ToolCallStatusCompleted)),
				}
			case "error":
				return []acp.SessionUpdate{
					acp.UpdateToolCall(id, acp.WithUpdateStatus(acp.ToolCallStatusFailed)),
				}
			default: // "start" 或其他
				return []acp.SessionUpdate{
					acp.StartToolCall(id, title, acp.WithStartKind(kind), acp.WithStartStatus(acp.ToolCallStatusInProgress)),
				}
			}
		}
		return nil
	}

	toolName, _ := m["tool_name"].(string)
	if toolName == "" {
		return nil
	}
	status, _ := m["status"].(string)
	callID, _ := m["tool_call_id"].(string)
	if callID == "" {
		callID = toolName
	}

	id := acp.ToolCallId(fmt.Sprintf("tc_%s", callID))
	kind := toolKindFromName(toolName)

	switch status {
	case "success":
		return []acp.SessionUpdate{
			acp.UpdateToolCall(id, acp.WithUpdateStatus(acp.ToolCallStatusCompleted)),
		}
	case "error":
		return []acp.SessionUpdate{
			acp.UpdateToolCall(id, acp.WithUpdateStatus(acp.ToolCallStatusFailed)),
		}
	default: // "start" 或其他
		return []acp.SessionUpdate{
			acp.StartToolCall(id, toolName, acp.WithStartKind(kind), acp.WithStartStatus(acp.ToolCallStatusInProgress)),
		}
	}
}

// convertAgentStartEvent 将 Agent 启动事件转换为 ACP 更新
func convertAgentStartEvent(payload interface{}) []acp.SessionUpdate {
	m, ok := payload.(map[string]interface{})
	if !ok {
		if evt, ok := payload.(master.AgentStartEvent); ok {
			id := acp.ToolCallId(fmt.Sprintf("agent_%s", evt.AgentName))
			title := fmt.Sprintf("启动子 Agent: %s", evt.AgentName)
			if evt.TaskDesc != "" {
				title = fmt.Sprintf("%s — %s", evt.AgentName, evt.TaskDesc)
			}
			return []acp.SessionUpdate{
				acp.StartToolCall(id, title, acp.WithStartKind(acp.ToolKindOther), acp.WithStartStatus(acp.ToolCallStatusInProgress)),
			}
		}
		return nil
	}

	agentName, _ := m["agent_name"].(string)
	if agentName == "" {
		return nil
	}
	taskDesc, _ := m["task_desc"].(string)

	id := acp.ToolCallId(fmt.Sprintf("agent_%s", agentName))
	title := fmt.Sprintf("启动子 Agent: %s", agentName)
	if taskDesc != "" {
		title = fmt.Sprintf("%s — %s", agentName, taskDesc)
	}

	return []acp.SessionUpdate{
		acp.StartToolCall(id, title, acp.WithStartKind(acp.ToolKindOther), acp.WithStartStatus(acp.ToolCallStatusInProgress)),
	}
}

// convertSkillExecEvent 将 Skill 执行事件转换为 ACP 更新
func convertSkillExecEvent(payload interface{}) []acp.SessionUpdate {
	m, ok := payload.(map[string]interface{})
	if !ok {
		if evt, ok := payload.(master.SkillExecEvent); ok {
			id := acp.ToolCallId(fmt.Sprintf("skill_%s", evt.SkillName))
			title := fmt.Sprintf("执行 Skill: %s", evt.SkillName)
			if evt.Args != "" {
				title = fmt.Sprintf("Skill %s: %s", evt.SkillName, evt.Args)
			}
			return []acp.SessionUpdate{
				acp.StartToolCall(id, title, acp.WithStartKind(acp.ToolKindExecute), acp.WithStartStatus(acp.ToolCallStatusInProgress)),
			}
		}
		return nil
	}

	skillName, _ := m["skill_name"].(string)
	if skillName == "" {
		return nil
	}
	args, _ := m["args"].(string)

	id := acp.ToolCallId(fmt.Sprintf("skill_%s", skillName))
	title := fmt.Sprintf("执行 Skill: %s", skillName)
	if args != "" {
		title = fmt.Sprintf("Skill %s: %s", skillName, args)
	}

	return []acp.SessionUpdate{
		acp.StartToolCall(id, title, acp.WithStartKind(acp.ToolKindExecute), acp.WithStartStatus(acp.ToolCallStatusInProgress)),
	}
}

// extractMessageText 从消息载荷中提取文本内容
func extractMessageText(payload interface{}) string {
	switch v := payload.(type) {
	case string:
		return v
	case map[string]interface{}:
		if text, ok := v["text"].(string); ok {
			return text
		}
		if msg, ok := v["message"].(string); ok {
			return msg
		}
	}
	return ""
}
