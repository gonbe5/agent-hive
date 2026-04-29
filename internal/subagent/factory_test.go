package subagent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/toolctx"
)

// mockRegistrar 模拟 AgentRegistrar 用于测试
type mockRegistrar struct {
	mu           sync.Mutex
	registered   map[string]bool
	unregistered map[string]bool
}

func newMockRegistrar() *mockRegistrar {
	return &mockRegistrar{
		registered:   make(map[string]bool),
		unregistered: make(map[string]bool),
	}
}

func (r *mockRegistrar) RegisterDynamic(agent Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registered[agent.ID()] = true
	return nil
}

func (r *mockRegistrar) UnregisterDynamic(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unregistered[id] = true
}

func (r *mockRegistrar) isRegistered(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registered[id]
}

func (r *mockRegistrar) isUnregistered(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.unregistered[id]
}

func newTestFactory(registrar AgentRegistrar) *AgentFactory {
	return NewAgentFactory(nil, nil, nil, testSkillReg(), registrar, testLogger())
}

// testCtxWithSession 创建带 sessionID 的测试 context
func testCtxWithSession(sessionID string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return toolctx.WithSessionID(ctx, sessionID), cancel
}

func newTestToolPolicy() *skills.ToolPolicy {
	return skills.NewToolPolicy(skills.ToolPolicyInput{
		SubagentDeny:     []string{"spawn_agent"},
		SubagentLeafDeny: []string{"parallel_dispatch", "task"},
	})
}

func TestAgentFactory_CreateAndDestroy(t *testing.T) {
	reg := newMockRegistrar()
	factory := newTestFactory(reg)

	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	spec := AgentSpec{
		ID:          "test-dyn-1",
		Name:        "测试动态 Agent",
		Description: "用于测试的动态 Agent",
	}

	agent, err := factory.CreateAgent(ctx, spec)
	if err != nil {
		t.Fatalf("CreateAgent 失败: %v", err)
	}

	if agent.ID() != "test-dyn-1" {
		t.Errorf("期望 ID test-dyn-1，得到 %s", agent.ID())
	}

	if !reg.isRegistered("test-dyn-1") {
		t.Error("Agent 未注册到 registrar")
	}

	if factory.DynamicCount() != 1 {
		t.Errorf("期望 1 个动态 agent，得到 %d", factory.DynamicCount())
	}

	// 等待 agent 启动
	time.Sleep(10 * time.Millisecond)

	// 销毁
	if err := factory.DestroyAgent("test-dyn-1"); err != nil {
		t.Fatalf("DestroyAgent 失败: %v", err)
	}

	if !reg.isUnregistered("test-dyn-1") {
		t.Error("Agent 未从 registrar 注销")
	}

	if factory.DynamicCount() != 0 {
		t.Errorf("期望 0 个动态 agent，得到 %d", factory.DynamicCount())
	}
}

func TestAgentFactory_AutoGenerateID(t *testing.T) {
	factory := newTestFactory(newMockRegistrar())

	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	spec := AgentSpec{
		Name: "自动 ID Agent",
	}

	agent, err := factory.CreateAgent(ctx, spec)
	if err != nil {
		t.Fatalf("CreateAgent 失败: %v", err)
	}

	if agent.ID() == "" {
		t.Error("自动生成的 ID 不应为空")
	}

	if len(agent.ID()) < 4 {
		t.Errorf("自动生成的 ID 太短: %s", agent.ID())
	}
}

func TestAgentFactory_DuplicateID(t *testing.T) {
	factory := newTestFactory(newMockRegistrar())

	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	spec := AgentSpec{ID: "dup-agent", Name: "First"}
	_, err := factory.CreateAgent(ctx, spec)
	if err != nil {
		t.Fatalf("第一次创建失败: %v", err)
	}

	// 重复 ID 应失败
	spec2 := AgentSpec{ID: "dup-agent", Name: "Second"}
	_, err = factory.CreateAgent(ctx, spec2)
	if err == nil {
		t.Fatal("重复 ID 应返回错误")
	}
}

func TestAgentFactory_MaxLimit(t *testing.T) {
	factory := newTestFactory(newMockRegistrar())
	factory.SetMaxPerSession(2)

	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	// 创建 2 个（上限）
	for i := 0; i < 2; i++ {
		spec := AgentSpec{Name: "Agent"}
		_, err := factory.CreateAgent(ctx, spec)
		if err != nil {
			t.Fatalf("创建第 %d 个 agent 失败: %v", i+1, err)
		}
	}

	// 第 3 个应失败
	_, err := factory.CreateAgent(ctx, AgentSpec{Name: "Overflow"})
	if err == nil {
		t.Fatal("超过上限应返回错误")
	}
}

func TestAgentFactory_CleanupBySession(t *testing.T) {
	reg := newMockRegistrar()
	factory := newTestFactory(reg)

	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	// 创建 3 个 agent（session "test-session"）
	ids := []string{"dyn-a", "dyn-b", "dyn-c"}
	for _, id := range ids {
		spec := AgentSpec{ID: id, Name: id}
		_, err := factory.CreateAgent(ctx, spec)
		if err != nil {
			t.Fatalf("创建 %s 失败: %v", id, err)
		}
	}

	if factory.DynamicCount() != 3 {
		t.Fatalf("期望 3 个动态 agent，得到 %d", factory.DynamicCount())
	}

	// 等待 agents 启动
	time.Sleep(10 * time.Millisecond)

	// 按 session 清理
	factory.CleanupBySession("test-session")

	if factory.DynamicCount() != 0 {
		t.Errorf("CleanupBySession 后期望 0 个动态 agent，得到 %d", factory.DynamicCount())
	}

	// 验证所有 agent 都已注销
	for _, id := range ids {
		if !reg.isUnregistered(id) {
			t.Errorf("Agent %s 未从 registrar 注销", id)
		}
	}
}

func TestAgentFactory_ListDynamic(t *testing.T) {
	factory := newTestFactory(newMockRegistrar())

	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	_, _ = factory.CreateAgent(ctx, AgentSpec{ID: "list-a", Name: "A", Description: "Agent A"})
	_, _ = factory.CreateAgent(ctx, AgentSpec{ID: "list-b", Name: "B", Description: "Agent B"})

	cards := factory.ListDynamic()
	if len(cards) != 2 {
		t.Fatalf("期望 2 个卡片，得到 %d", len(cards))
	}

	// 验证 ID 存在
	found := map[string]bool{}
	for _, c := range cards {
		found[c.ID] = true
	}
	if !found["list-a"] || !found["list-b"] {
		t.Error("卡片列表缺少预期的 agent")
	}
}

func TestAgentFactory_IsDynamic(t *testing.T) {
	factory := newTestFactory(newMockRegistrar())

	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	_, _ = factory.CreateAgent(ctx, AgentSpec{ID: "check-dyn", Name: "Check"})

	if !factory.IsDynamic("check-dyn") {
		t.Error("check-dyn 应被识别为动态 agent")
	}

	if factory.IsDynamic("nonexistent") {
		t.Error("nonexistent 不应被识别为动态 agent")
	}
}

func TestAgentFactory_DestroyNonexistent(t *testing.T) {
	factory := newTestFactory(newMockRegistrar())

	err := factory.DestroyAgent("no-such-agent")
	if err == nil {
		t.Fatal("销毁不存在的 agent 应返回错误")
	}
}

func TestAgentFactory_IsLeafBySpawnDepth(t *testing.T) {
	factory := newTestFactory(newMockRegistrar())

	// 设置 ToolPolicy 以启用 leaf deny
	policy := newTestToolPolicy()
	factory.SetToolPolicy(policy)

	// 默认 maxSpawnDepth=1，SpawnDepth=0 → depth+1=1 >= 1 → isLeaf=true
	// 验证方式：通过日志或间接验证 leaf deny 生效
	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	spec := AgentSpec{
		ID:         "leaf-test",
		Name:       "Leaf Agent",
		SpawnDepth: 0,
	}
	_, err := factory.CreateAgent(ctx, spec)
	if err != nil {
		t.Fatalf("CreateAgent 失败: %v", err)
	}

	// SpawnDepth=0, maxSpawnDepth=1 → isLeaf=true，subagent_leaf_deny 应生效
	// 由于无法直接检查 toolFilter（内部变量），通过 factory 状态间接验证创建成功
	if !factory.IsDynamic("leaf-test") {
		t.Error("leaf-test 应存在")
	}
}

func TestAgentFactory_MaxSpawnDepth(t *testing.T) {
	factory := newTestFactory(newMockRegistrar())
	factory.maxSpawnDepth = 3

	// SpawnDepth=1, maxSpawnDepth=3 → 1+1=2 < 3 → isLeaf=false
	// SpawnDepth=2, maxSpawnDepth=3 → 2+1=3 >= 3 → isLeaf=true
	// 验证不同深度的 agent 都能正确创建
	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	_, err := factory.CreateAgent(ctx, AgentSpec{ID: "depth-1", Name: "D1", SpawnDepth: 1})
	if err != nil {
		t.Fatalf("depth=1 创建失败: %v", err)
	}

	_, err = factory.CreateAgent(ctx, AgentSpec{ID: "depth-2", Name: "D2", SpawnDepth: 2})
	if err != nil {
		t.Fatalf("depth=2 创建失败: %v", err)
	}

	if factory.DynamicCount() != 2 {
		t.Errorf("期望 2 个 agent，得到 %d", factory.DynamicCount())
	}
}

// failRegistrar 模拟注册失败的 AgentRegistrar
type failRegistrar struct{}

func (r *failRegistrar) RegisterDynamic(agent Agent) error {
	return errs.New(errs.CodeInvalidInput, "ID 与静态 agent 冲突")
}

func (r *failRegistrar) UnregisterDynamic(id string) {}

func TestAgentFactory_RegistrarFailure(t *testing.T) {
	factory := newTestFactory(&failRegistrar{})

	ctx, cancel := testCtxWithSession("test-session")
	defer cancel()

	_, err := factory.CreateAgent(ctx, AgentSpec{ID: "conflict", Name: "Conflict"})
	if err == nil {
		t.Fatal("registrar 注册失败时 CreateAgent 应返回错误")
	}

	// agent 不应被跟踪
	if factory.DynamicCount() != 0 {
		t.Errorf("registrar 失败后不应有动态 agent，实际 %d", factory.DynamicCount())
	}
}

// TestAgentFactory_PerSession_Concurrent 验证 per-session 动态 Agent 的并发安全性（P0-3 Phase 6, CEO Review 修正 7）
func TestAgentFactory_PerSession_Concurrent(t *testing.T) {
	reg := newMockRegistrar()
	factory := newTestFactory(reg)
	factory.SetMaxPerSession(50) // 放大上限以容纳并发创建
	factory.SetMaxGlobal(100)

	const goroutines = 10
	const sessionsPerGoroutine = 3

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*sessionsPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gIdx int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("concurrent-session-%d", gIdx%3) // 3 个 session 共享
			ctx, cancel := testCtxWithSession(sessionID)
			defer cancel()

			for s := 0; s < sessionsPerGoroutine; s++ {
				spec := AgentSpec{
					Name: fmt.Sprintf("agent-g%d-s%d", gIdx, s),
				}
				_, err := factory.CreateAgent(ctx, spec)
				if err != nil {
					// per-session 或 global 限制导致的错误是预期的
					if !isResourceExhausted(err) && !isDuplicateID(err) {
						errCh <- fmt.Errorf("goroutine %d: unexpected error: %v", gIdx, err)
					}
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	// 验证数据一致性：DynamicCount 应等于所有 session 桶的总和
	totalCount := factory.DynamicCount()
	sessionCounts := 0
	for i := 0; i < 3; i++ {
		sessionCounts += factory.DynamicCountBySession(fmt.Sprintf("concurrent-session-%d", i))
	}
	if totalCount != sessionCounts {
		t.Errorf("DynamicCount (%d) 与各 session 计数之和 (%d) 不一致", totalCount, sessionCounts)
	}

	// 并发 CleanupBySession
	var wg2 sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg2.Add(1)
		go func(idx int) {
			defer wg2.Done()
			factory.CleanupBySession(fmt.Sprintf("concurrent-session-%d", idx))
		}(i)
	}
	wg2.Wait()

	if factory.DynamicCount() != 0 {
		t.Errorf("CleanupBySession 后期望 0 个动态 agent，得到 %d", factory.DynamicCount())
	}
}

func isResourceExhausted(err error) bool {
	return err != nil && (contains(err.Error(), "已达上限") || contains(err.Error(), "exhausted"))
}

func isDuplicateID(err error) bool {
	return err != nil && contains(err.Error(), "已存在")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
