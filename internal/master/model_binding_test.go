package master

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// TestAgentModelBinding 测试 Session LLM 绑定功能
func TestAgentModelBinding(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个 LLM Pool
	pool := llm.NewClientPool(logger)

	// 测试 1: 无 activeLLM，应该使用全局 llmClient
	t.Run("使用全局LLM", func(t *testing.T) {
		session := &SessionState{}

		globalLLM := llm.NewClient(llm.ClientConfig{
			APIKey:   "global-key",
			Model:    "gpt-4",
			Provider: llm.LookupProvider("openai"),
		}, logger)

		m := &Master{
			config: Config{
				Model:    "gpt-4",
				APIKey:   "global-key",
				Provider: "openai",
			},
			llmClient: globalLLM,
			llmPool:   pool,
			logger:    logger,
		}

		sessionLLM := m.getSessionLLM(session)

		assert.NotNil(t, sessionLLM)
		assert.Equal(t, globalLLM, sessionLLM, "应该返回全局 LLM")
		assert.Equal(t, "gpt-4", session.activeModel)
	})

	// 测试 2: 已有 activeLLM，应该复用缓存
	t.Run("复用已有activeLLM", func(t *testing.T) {
		cachedLLM := llm.NewClient(llm.ClientConfig{
			APIKey:   "cached-key",
			Model:    "gpt-4-turbo",
			Provider: llm.LookupProvider("openai"),
		}, logger)

		session := &SessionState{
			activeLLM: cachedLLM,
		}

		globalLLM := llm.NewClient(llm.ClientConfig{
			APIKey:   "global-key",
			Model:    "gpt-4",
			Provider: llm.LookupProvider("openai"),
		}, logger)

		m := &Master{
			config: Config{
				Model:    "gpt-4",
				APIKey:   "global-key",
				Provider: "openai",
			},
			llmClient: globalLLM,
			llmPool:   pool,
			logger:    logger,
		}

		sessionLLM := m.getSessionLLM(session)

		assert.NotNil(t, sessionLLM)
		assert.Equal(t, cachedLLM, sessionLLM, "应该返回缓存的 Client")
	})

	// 测试 3: 多次调用应该使用缓存
	t.Run("多次调用使用缓存", func(t *testing.T) {
		session := &SessionState{}

		globalLLM := llm.NewClient(llm.ClientConfig{
			APIKey:   "global-key",
			Model:    "gpt-4",
			Provider: llm.LookupProvider("openai"),
		}, logger)

		m := &Master{
			config: Config{
				Model:    "gpt-4",
				APIKey:   "global-key",
				Provider: "openai",
			},
			llmClient: globalLLM,
			llmPool:   pool,
			logger:    logger,
		}

		// 第一次调用
		sessionLLM1 := m.getSessionLLM(session)
		assert.NotNil(t, sessionLLM1)

		// 第二次调用应该返回缓存的 Client
		sessionLLM2 := m.getSessionLLM(session)
		assert.Equal(t, sessionLLM1, sessionLLM2, "应该返回缓存的 Client")
	})
}
