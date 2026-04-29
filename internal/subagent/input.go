package subagent

// AgentInput 是固定 Agent 的统一输入协议。
// 所有固定 Agent 都应优先读取 Instruction 字段。
// 各 Agent 可以额外支持自己的专有字段（向后兼容）。
//
// Phase 3 预留（spec-driven cognition，**本 change 不实装**）：
//
//	Context["spec_ref"] 保留给 Phase 3 的 spec_ref capability token 使用。
//	目的是让主 session 把"对某个 change 的受限读能力"以 token 形式传给 subagent，
//	而不是裸给 change_id。原因：
//	  - 裸 change_id 允许 subagent 自行直写 SpecState / SpecChangeStore（违反 Guard 4
//	    和 Guard 6），而 token 模型把写权限保留在主路径，subagent 只能读 + 投递 event。
//	  - Phase 2 暂无业务需要 subagent 感知 spec——用户请求全部在 session_loop ingress
//	    处决策。
//
//	硬规约：
//	  - 禁止在 Phase 2 代码中写 Context["change_id"] / Context["spec_id"] 等裸标识
//	    来绕过未来 token 模型。
//	  - 当前 AgentInput 消费者若需要感知 spec，应改走 `session.LoadSpecCtx()` 读
//	    atomic.Pointer[Context]（已在 #14 中实装）。
//	  - 任何对 Context["spec_ref"] 的读取必须返回空——Phase 3 才会赋值。
//
// 详见 `openspec/changes/harden-spec-driven-phase2/design.md` 的 Phase 3 roadmap 段
// 和 `openspec/changes/add-spec-driven-cognition/specs/hidden-spec-layer/spec.md`。
type AgentInput struct {
	// Instruction 是统一的任务指令字段，所有固定 Agent 都支持
	Instruction string `json:"instruction"`
	// Context 是附加上下文信息（可选）。
	// Phase 3 保留键：Context["spec_ref"] —— 当前绝对禁止读写。
	Context map[string]interface{} `json:"context,omitempty"`
}

// ExtractInstruction 从 AgentInput 中提取指令。
// 如果 Instruction 为空，尝试从 Context["topic"] 或 Context["target"] 中提取（向后兼容）。
func (a AgentInput) ExtractInstruction() string {
	if a.Instruction != "" {
		return a.Instruction
	}
	if a.Context != nil {
		if v, ok := a.Context["topic"].(string); ok && v != "" {
			return v
		}
		if v, ok := a.Context["target"].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
