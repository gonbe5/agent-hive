package master

import (
	"strings"
	"testing"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/mcphost"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
	"github.com/chef-guo/agents-hive/internal/subagent"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func newTestMaster(t *testing.T) (*Master, *skills.Registry) {
	t.Helper()
	logger := zaptest.NewLogger(t)
	skillReg := skills.NewRegistry(logger)
	st := store.NewMemoryStore()
	registry := subagent.NewRegistry(logger)
	cfg := Config{}
	hitlCfg := config.HITLConfig{Enabled: false}
	return NewMaster(cfg, hitlCfg, registry, skillReg, st, logger), skillReg
}

func TestMasterPrompt_NoFixedAgentReference(t *testing.T) {
	m, _ := newTestMaster(t)
	prompt := m.buildSystemPrompt(nil)

	// 不应引用已删除的固定 Agent
	forbidden := []string{
		"general / code / research / ops",
		"code / research / ops / general",
		"固定 Agent",
		"固定Agent",
		"系统已有固定 Agent",
	}
	for _, s := range forbidden {
		assert.False(t, strings.Contains(prompt, s),
			"prompt 不应包含固定 Agent 引用: %q", s)
	}
}

func TestBuildSystemPrompt_SkillListing_DomainMetadata(t *testing.T) {
	m, skillReg := newTestMaster(t)

	// 注册一个带 domain/trigger_keywords 的 skill
	tr := true
	if err := skillReg.Register(&skills.Skill{
		Metadata: skills.SkillMetadata{
			Name:            "roi-analysis",
			Description:     "ROI 分析规范",
			Domain:          "analytics",
			TriggerKeywords: []string{"ROI", "投资回报"},
			Priority:        7,
			Complexity:      "medium",
			UserInvocable:   &tr,
		},
		Content: "ROI analysis content",
		Loaded:  skills.LevelMetadataOnly,
	}); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	prompt := m.buildSystemPrompt(nil)

	assert.True(t, strings.Contains(prompt, "roi-analysis"), "prompt 应包含 skill 名称")
	assert.True(t, strings.Contains(prompt, "领域: analytics"), "prompt 应包含域信息")
	assert.True(t, strings.Contains(prompt, "ROI"), "prompt 应包含触发词")
}

func TestMasterPrompt_ContainsKeyGuidance(t *testing.T) {
	m, _ := newTestMaster(t)
	prompt := m.buildSystemPrompt([]mcphost.ToolDefinition{
		{Name: "bash", Description: "执行命令", Core: true},
	})

	required := []struct {
		section string
		keyword string
	}{
		{"身份定义", "你是 Hive"},
		{"任务执行策略", "优先直接使用工具"},
		{"工具选择指南-并行", "parallel_dispatch"},
		{"工具选择指南-信息获取", "聚焦可执行结论"},
		{"迭代执行", "工具调用循环"},
		{"不确定时的处理", "question 工具确认意图"},
		{"代码编辑规范", "edit/multiedit"},
		{"运维安全规范", "破坏性操作前务必确认"},
		{"回复规范", "直接回答问题"},
		{"anti-hallucination-来源", "标注来源"},
		{"anti-hallucination-不确定性", "标记不确定性"},
		{"research-thoroughness", "多角度发现"},
		{"spawn_agent 使用规范", "最多同时 3 个子代理"},
		{"explore 调用方式", "task 工具委派给 explore Agent"},
		{"可用工具", "bash"},
	}
	for _, r := range required {
		assert.True(t, strings.Contains(prompt, r.keyword),
			"prompt 应包含 %s 的关键指导: %q", r.section, r.keyword)
	}
}
