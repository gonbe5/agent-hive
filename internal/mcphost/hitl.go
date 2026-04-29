package mcphost

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/chef-guo/agents-hive/internal/toolctx"
)

// HITLInputRequest 是 mcphost 层与 master 解耦的 InputRequest 镜像类型。
//
// 为什么定义在 mcphost 而不是直接 import master：master 已 import tools → mcphost
// 构成反向链，若 mcphost 再 import master 即形成包循环。tasks.md §6.2a 明确
// 允许用 HITLEmitter 接口解耦——故此处定义中性镜像，bootstrap 层用适配器把
// *master.Master 转成 HITLEmitter。
//
// 字段与 master.InputRequest 一一对齐；转换由 bootstrap.masterHITLAdapter 负责。
type HITLInputRequest struct {
	ID          string          `json:"id"`
	TaskID      string          `json:"task_id"`
	StepID      string          `json:"step_id,omitempty"`
	Type        string          `json:"type"`
	Prompt      string          `json:"prompt"`
	Options     []string        `json:"options,omitempty"`
	Default     string          `json:"default,omitempty"`
	Timeout     time.Duration   `json:"timeout,omitempty"`
	ToolName    string          `json:"tool_name,omitempty"`
	ChoiceType  string          `json:"choice_type,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	Fingerprint string          `json:"fingerprint,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// HITLInputResponse 是用户响应的中性镜像。
type HITLInputResponse struct {
	RequestID string `json:"request_id"`
	TaskID    string `json:"task_id"`
	Value     string `json:"value"`
	Action    string `json:"action"`
	Remember  bool   `json:"remember,omitempty"`
}

// HITLEmitter 是 Host 对 master.EmitInputRequest 的接口抽象。
// bootstrap 用 *master.Master 的适配器实现它；mcphost 只依赖接口，避免包循环。
//
// 实现必须 goroutine-safe：tool handler 会并发调用。
type HITLEmitter interface {
	EmitInputRequest(ctx context.Context, req HITLInputRequest) (*HITLInputResponse, error)
}

// ErrHITLEmitterNotConfigured 表示 Host 未注入 HITLEmitter 就调用了 EmitInputRequest。
// 常见于：OnDemandEnabled=false 的部署仍意外注册了 skill_install，或测试路径未 mock。
var ErrHITLEmitterNotConfigured = errors.New("mcphost: HITLEmitter not configured on Host")

// SetHITLEmitter 注入 HITL 发射器。bootstrap 在 Master 就绪后调用。
// 幂等；后注入覆盖前。
func (h *Host) SetHITLEmitter(e HITLEmitter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hitlEmitter = e
}

// EmitInputRequest 透传到底层 HITLEmitter，并自动注入 ctx 里的 SessionID（MINOR 1 修复）。
//
// 契约：
//   - req.SessionID 非空 → 原样透传；
//   - req.SessionID 空 → 从 toolctx.GetSessionID(ctx) 读并填充（tool handler
//     会从 executeTool/AgentLoop 继承 sessionID ctx-key）；
//   - Emitter 未注入 → 返回 ErrHITLEmitterNotConfigured（而非 panic，利于降级与测试）。
//
// 不在此处 cancel 或 timeout——全部交给 Master.EmitInputRequest 的内部实现。
func (h *Host) EmitInputRequest(ctx context.Context, req HITLInputRequest) (*HITLInputResponse, error) {
	h.mu.RLock()
	emitter := h.hitlEmitter
	h.mu.RUnlock()
	if emitter == nil {
		return nil, ErrHITLEmitterNotConfigured
	}
	if req.SessionID == "" {
		req.SessionID = toolctx.GetSessionID(ctx)
	}
	return emitter.EmitInputRequest(ctx, req)
}
