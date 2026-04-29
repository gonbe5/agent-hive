package master

import (
	"context"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
	"github.com/chef-guo/agents-hive/internal/subagent"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// TestListAgents 测试列出所有 Agent
func TestListAgents(t *testing.T) {
	logger := zaptest.NewLogger(t)
	skillReg := skills.NewRegistry(logger)
	st := store.NewMemoryStore()
	registry := subagent.NewRegistry(logger)

	cfg := Config{}
	hitlCfg := config.HITLConfig{Enabled: false}

	master := NewMaster(cfg, hitlCfg, registry, skillReg, st, logger)

	// 初始应该为空或有默认 agents
	agents := master.ListAgents()
	initialCount := len(agents)

	// 验证返回了列表（可能为空）
	assert.NotNil(t, agents)
	_ = initialCount
}

// TestHITLEnabled 测试 HITL 是否启用
func TestHITLEnabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	skillReg := skills.NewRegistry(logger)
	st := store.NewMemoryStore()
	registry := subagent.NewRegistry(logger)

	t.Run("HITL enabled", func(t *testing.T) {
		cfg := Config{}
		hitlCfg := config.HITLConfig{Enabled: true}

		master := NewMaster(cfg, hitlCfg, registry, skillReg, st, logger)
		assert.True(t, master.HITLEnabled())
	})

	t.Run("HITL disabled", func(t *testing.T) {
		cfg := Config{}
		hitlCfg := config.HITLConfig{Enabled: false}

		master := NewMaster(cfg, hitlCfg, registry, skillReg, st, logger)
		assert.False(t, master.HITLEnabled())
	})
}

// TestAskQuestion_Timeout 测试问题询问超时
func TestAskQuestion_Timeout(t *testing.T) {
	logger := zaptest.NewLogger(t)
	skillReg := skills.NewRegistry(logger)
	st := store.NewMemoryStore()
	registry := subagent.NewRegistry(logger)

	cfg := Config{}
	hitlCfg := config.HITLConfig{
		Enabled:      true,
		InputTimeout: 100 * time.Millisecond,
	}

	master := NewMaster(cfg, hitlCfg, registry, skillReg, st, logger)

	master.sessionMgr.SetActiveSessionID("test-session")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := master.AskQuestion(
		ctx,
		"这个问题不会被回答",
		[]string{"option1", "option2"},
		50*time.Millisecond,
	)

	// 应该超时
	assert.Error(t, err)
}

// TestExecuteTask_EmptyAgentID 验证 agent_id 为空时返回错误（P0-3 Phase 3+4：general Agent 已删除）
func TestExecuteTask_EmptyAgentID(t *testing.T) {
	logger := zaptest.NewLogger(t)
	skillReg := skills.NewRegistry(logger)
	st := store.NewMemoryStore()
	registry := subagent.NewRegistry(logger)

	cfg := Config{}
	hitlCfg := config.HITLConfig{Enabled: false}
	master := NewMaster(cfg, hitlCfg, registry, skillReg, st, logger)

	_, err := master.ExecuteTask(context.Background(), "", "测试任务", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent_id 不能为空")
}
