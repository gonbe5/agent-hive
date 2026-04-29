package master

import (
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/i18n"
)

// PromptContext 封装提示词相关的状态（PromptManager、Provider、指令、Agent 定义）。
// 并发安全约定：SetXxx 方法仅在初始化阶段调用，运行时只读，因此无需加锁。
type PromptContext struct {
	promptMgr   *i18n.PromptManager
	providerKey i18n.ProviderKey
	// 自定义指令（从 .claw/AGENTS.md 或 CLAUDE.md 加载）
	instructions string
	// 自定义 Agent 定义（从 .claw/agents/ 目录加载）
	agentDefs []config.AgentDefinition
}

// NewPromptContext 创建新的 PromptContext
func NewPromptContext(promptMgr *i18n.PromptManager, providerKey i18n.ProviderKey) *PromptContext {
	return &PromptContext{
		promptMgr:   promptMgr,
		providerKey: providerKey,
	}
}

// PromptManager 返回 PromptManager 实例
func (pc *PromptContext) PromptManager() *i18n.PromptManager {
	return pc.promptMgr
}

// ProviderKey 返回 Provider 标识
func (pc *PromptContext) ProviderKey() i18n.ProviderKey {
	return pc.providerKey
}

// Instructions 返回自定义指令
func (pc *PromptContext) Instructions() string {
	return pc.instructions
}

// SetInstructions 设置自定义指令
func (pc *PromptContext) SetInstructions(instructions string) {
	pc.instructions = instructions
}

// AgentDefs 返回自定义 Agent 定义
func (pc *PromptContext) AgentDefs() []config.AgentDefinition {
	return pc.agentDefs
}

// SetAgentDefs 设置自定义 Agent 定义
func (pc *PromptContext) SetAgentDefs(defs []config.AgentDefinition) {
	pc.agentDefs = defs
}
