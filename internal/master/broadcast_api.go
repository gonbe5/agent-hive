package master

import (
	"context"

	"github.com/chef-guo/agents-hive/internal/plugin"
)

// SubscribeWSBroadcast 创建一个 WebSocket 广播订阅（委托给 EventBus）
func (m *Master) SubscribeWSBroadcast() (uint64, chan BroadcastMessage) {
	return m.eventBus.Subscribe()
}

// UnsubscribeWSBroadcast 取消 WebSocket 广播订阅（委托给 EventBus）
func (m *Master) UnsubscribeWSBroadcast(subID uint64) {
	m.eventBus.Unsubscribe(subID)
}

// BroadcastSessionMessage 广播与特定会话关联的消息（委托给 EventBus）。
func (m *Master) BroadcastSessionMessage(sessionID string, msg BroadcastMessage) {
	m.eventBus.BroadcastSessionMessage(sessionID, msg)
}

// broadcast 向所有订阅者广播消息（委托给 EventBus）
func (m *Master) broadcast(msg BroadcastMessage) {
	m.eventBus.Broadcast(msg)
}

// BroadcastInputRequest 广播 input_request 消息（委托给 EventBus）
func (m *Master) BroadcastInputRequest(req *InputRequest) {
	m.eventBus.BroadcastInputRequest(req)
}

// SubscribeInputResponse 订阅指定 reqID 的 InputResponse。
//
// 返回一个容量 1 的只读通道：收到匹配的响应后发送并关闭；ctx 取消或 EventBus
// 关闭时也会关闭（不发送值）。调用方必须总是 <-ch 读到 ok=false 才能确认清理完成。
//
// 设计：挂在 *Master 与 BroadcastInputRequest 同位置，业务 Skill/Tool 不直接
// 感知 EventBus。goroutine 在所有退出路径（匹配 / ctx / eventbus close）都会
// 调用 Unsubscribe，确保零泄漏（goleak 验证）。
func (m *Master) SubscribeInputResponse(ctx context.Context, reqID string) <-chan *InputResponse {
	out := make(chan *InputResponse, 1)
	subID, sub := m.eventBus.Subscribe()
	if sub == nil {
		close(out)
		return out
	}
	go func() {
		defer m.eventBus.Unsubscribe(subID)
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub:
				if !ok {
					return
				}
				if msg.Type != EventTypeInputResponse {
					continue
				}
				resp, ok := msg.Payload.(*InputResponse)
				if !ok || resp == nil || resp.RequestID != reqID {
					continue
				}
				out <- resp
				return
			}
		}
	}()
	return out
}

// BroadcastGenericMessage 广播通用消息（委托给 EventBus）
func (m *Master) BroadcastGenericMessage(msgType string, payload interface{}) {
	m.eventBus.BroadcastGenericMessage(msgType, payload)
}

// BroadcastTaskGroup 实现 tools.ParallelDispatchBroadcaster 接口 — 广播任务组生命周期事件
func (m *Master) BroadcastTaskGroup(groupID string, status string, total int, tasks interface{}, results interface{}) {
	completed := 0
	if briefs, ok := tasks.([]interface{}); ok {
		for _, b := range briefs {
			if bm, ok := b.(map[string]string); ok && bm["status"] == "completed" {
				completed++
			}
		}
	}
	m.broadcast(BroadcastMessage{
		Type: EventTypeTaskGroup,
		Payload: TaskGroupEvent{
			GroupID:   groupID,
			Status:    status,
			Total:     total,
			Completed: completed,
			Results:   results,
		},
	})
}

// BroadcastTaskProgress 实现 tools.ParallelDispatchBroadcaster 接口 — 广播单个任务进度事件
func (m *Master) BroadcastTaskProgress(groupID string, taskID string, status string, errMsg string) {
	m.broadcast(BroadcastMessage{
		Type: EventTypeTaskProgress,
		Payload: map[string]string{
			"group_id": groupID,
			"task_id":  taskID,
			"status":   status,
			"error":    errMsg,
		},
	})
	// 触发 TaskCreated / TaskCompleted hook（非阻塞）
	if m.pluginMgr != nil {
		input := &plugin.TaskEventInput{
			TaskID: taskID,
			Status: status,
			Error:  errMsg,
		}
		go func() {
			hookCtx, cancel := context.WithTimeout(context.Background(), plugin.HookCallTimeout)
			defer cancel()
			switch status {
			case "pending", "running":
				_ = m.pluginMgr.TriggerTaskCreated(hookCtx, input)
			case "completed", "failed":
				_ = m.pluginMgr.TriggerTaskCompleted(hookCtx, input)
			}
		}()
	}
}
