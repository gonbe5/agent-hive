package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/toolctx"
)

// AgentSpec 定义动态创建 Agent 的规格
type AgentSpec struct {
	ID           string   `json:"id"`                    // 唯一标识（空则自动生成 "dyn-<uuid[:8]>"）
	Name         string   `json:"name"`                  // 显示名称
	Description  string   `json:"description"`           // 能力描述
	SystemPrompt string   `json:"system_prompt"`         // 定制的系统提示词
	Tools        []string `json:"tools,omitempty"`       // 允许的工具白名单（空=全部）
	MaxTurns     int      `json:"max_turns,omitempty"`   // 最大迭代轮次（默认 25）
	SpawnDepth   int      `json:"spawn_depth,omitempty"` // 当前 spawn 深度（Master=0, SubAgent=1, ...）
}

// AgentRegistrar 用于将 Agent 注册到外部系统（Registry + Transport）
type AgentRegistrar interface {
	// RegisterDynamic 注册动态 Agent，返回 error 表示注册失败（如 ID 与静态 agent 冲突）
	RegisterDynamic(agent Agent) error
	UnregisterDynamic(id string)
}

// AgentFactory 负责在运行时动态创建和管理临时 Agent
type AgentFactory struct {
	llmClient   *llm.Client
	llmResolver LLMClientResolver // 动态获取 LLM client（优先于 llmClient）
	toolBridge  *skills.ToolBridge
	permMgr     *skills.PermissionManager
	skillReg    *skills.Registry
	registrar   AgentRegistrar
	logger      *zap.Logger
	progressFn    ProgressCallback
	streamFn      StreamCallback
	llmCompleteFn LLMCompleteCallback  // LLM 调用完成回调（可选，用于成本追踪）
	toolPolicy    *skills.ToolPolicy   // 工具策略引擎（可选，用于 group/profile 展开和 subagent deny）

	dynamicAgents map[string]map[string]*BaseAgent // sessionID -> agentID -> agent（per-session 隔离）
	mu            sync.RWMutex
	maxPerSession int // 单 session 最大动态 agent 数（默认 3）
	maxGlobal     int // 全局最大动态 agent 数（默认 30）
	maxSpawnDepth int // 最大 spawn 深度（达到此深度的 agent 被标记为 leaf，默认 1）
}

// NewAgentFactory 创建新的 AgentFactory
func NewAgentFactory(
	llmClient *llm.Client,
	toolBridge *skills.ToolBridge,
	permMgr *skills.PermissionManager,
	skillReg *skills.Registry,
	registrar AgentRegistrar,
	logger *zap.Logger,
) *AgentFactory {
	return &AgentFactory{
		llmClient:     llmClient,
		toolBridge:    toolBridge,
		permMgr:       permMgr,
		skillReg:      skillReg,
		registrar:     registrar,
		logger:        logger,
		dynamicAgents: make(map[string]map[string]*BaseAgent),
		maxPerSession: 3,
		maxGlobal:     30,
		maxSpawnDepth: 1,
	}
}

// SetLLMClient 延迟设置 LLM 客户端（支持延迟初始化）
func (f *AgentFactory) SetLLMClient(client *llm.Client) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llmClient = client
}

// SetLLMResolver 设置动态 LLM 客户端获取函数（优先于静态 llmClient）。
// 动态 Agent 在创建 AgentLoop 时使用 resolver，支持 session 模型切换和 task-type 选路。
func (f *AgentFactory) SetLLMResolver(resolver LLMClientResolver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llmResolver = resolver
}

// SetProgressCallback 设置进度回调函数（线程安全）
func (f *AgentFactory) SetProgressCallback(fn ProgressCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.progressFn = fn
}

// SetToolPolicy 设置工具策略引擎（用于 group/profile 展开和 subagent deny 列表）
func (f *AgentFactory) SetToolPolicy(policy *skills.ToolPolicy) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toolPolicy = policy
}

// SetStreamCallback 设置流式内容回调函数（线程安全）
func (f *AgentFactory) SetStreamCallback(fn StreamCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.streamFn = fn
}

// SetLLMCompleteCallback 设置 LLM 调用完成回调（用于成本追踪等）
func (f *AgentFactory) SetLLMCompleteCallback(fn LLMCompleteCallback) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llmCompleteFn = fn
}

// SetMaxPerSession 设置单 session 最大动态 agent 数量
func (f *AgentFactory) SetMaxPerSession(max int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.maxPerSession = max
}

// SetMaxGlobal 设置全局最大动态 agent 数量
func (f *AgentFactory) SetMaxGlobal(max int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.maxGlobal = max
}

// CreateAgent 根据规格创建动态 Agent 并启动
func (f *AgentFactory) CreateAgent(ctx context.Context, spec AgentSpec) (Agent, error) {
	// 第一阶段：持锁完成验证、构建、注册、跟踪
	agent, err := f.createAgentLocked(ctx, spec)
	if err != nil {
		return nil, err
	}

	// 第二阶段：锁外启动 goroutine 并等待就绪（避免持锁期间 sleep）
	go agent.Run(ctx)

	// 等待 agent 启动完成（避免 SendTask 时 status 仍为 Stopped 的竞态）
	for i := 0; i < 100; i++ {
		if agent.Status() == StatusRunning {
			break
		}
		select {
		case <-ctx.Done():
			// 上下文取消，清理已注册但未就绪的 agent
			if destroyErr := f.DestroyAgent(agent.ID()); destroyErr != nil {
				f.logger.Warn("清理未就绪的动态 agent 失败", zap.String("id", agent.ID()), zap.Error(destroyErr))
			}
			return nil, errs.Wrap(errs.CodeCanceled, "等待 agent 启动时上下文已取消", ctx.Err())
		default:
		}
		time.Sleep(time.Millisecond)
	}

	return agent, nil
}

// createAgentLocked 在持锁状态下完成 agent 的验证、构建、注册和跟踪
func (f *AgentFactory) createAgentLocked(ctx context.Context, spec AgentSpec) (*BaseAgent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 自动生成 ID
	if spec.ID == "" {
		spec.ID = "dyn-" + uuid.New().String()[:8]
	}

	// 从 ctx 提取 sessionID（必须由调用方注入）
	sessionID := toolctx.GetSessionID(ctx)
	if sessionID == "" {
		return nil, errs.New(errs.CodeInvalidInput, "CreateAgent 要求 ctx 中包含 sessionID（通过 toolctx.WithSessionID 注入）")
	}

	// 初始化 session 桶
	if f.dynamicAgents[sessionID] == nil {
		f.dynamicAgents[sessionID] = make(map[string]*BaseAgent)
	}

	// 检查 ID 唯一性（session 内）
	if _, exists := f.dynamicAgents[sessionID][spec.ID]; exists {
		return nil, errs.New(errs.CodeInvalidInput, fmt.Sprintf("动态 agent %q 已存在（session %s）", spec.ID, sessionID))
	}

	// 检查 per-session 数量限制
	if len(f.dynamicAgents[sessionID]) >= f.maxPerSession {
		return nil, errs.New(errs.CodeResourceExhausted, fmt.Sprintf("session %s 动态 agent 数量已达上限 (%d)", sessionID, f.maxPerSession))
	}

	// 检查全局数量限制
	globalCount := 0
	for _, bucket := range f.dynamicAgents {
		globalCount += len(bucket)
	}
	if globalCount >= f.maxGlobal {
		return nil, errs.New(errs.CodeResourceExhausted, fmt.Sprintf("全局动态 agent 数量已达上限 (%d)", f.maxGlobal))
	}

	// 验证工具白名单
	if len(spec.Tools) > 0 && f.toolBridge != nil {
		allTools := f.toolBridge.AvailableTools(nil)
		toolSet := make(map[string]bool, len(allTools))
		for _, t := range allTools {
			toolSet[t.Name] = true
		}
		for _, t := range spec.Tools {
			if !toolSet[t] {
				return nil, errs.New(errs.CodeInvalidInput, fmt.Sprintf("工具 %q 不存在", t))
			}
		}
	}

	// 设置默认值
	if spec.MaxTurns <= 0 {
		spec.MaxTurns = 50
	}
	if spec.Name == "" {
		spec.Name = spec.ID
	}

	// 创建 AgentLoop（优先使用 resolver 实现动态模型路由）
	var loop *AgentLoop
	if f.llmResolver != nil && f.toolBridge != nil {
		loop = NewAgentLoopWithResolver(spec.ID, f.llmResolver, f.toolBridge, f.permMgr, f.logger)
		loop.SetMaxTurns(spec.MaxTurns)
		if sid := toolctx.GetSessionID(ctx); sid != "" {
			loop.SetSessionID(sid)
		}
		if uid := auth.UserIDFrom(ctx); uid != "" {
			loop.SetUserID(uid)
		}
		if f.progressFn != nil {
			loop.SetProgressCallback(f.progressFn)
		}
		if f.streamFn != nil {
			loop.SetStreamCallback(f.streamFn)
		}
		if f.llmCompleteFn != nil {
			loop.SetLLMCompleteCallback(f.llmCompleteFn)
		}
	} else if f.llmClient != nil && f.toolBridge != nil {
		loop = NewAgentLoop(spec.ID, f.llmClient, f.toolBridge, f.permMgr, f.logger)
		loop.SetMaxTurns(spec.MaxTurns)
		if sid := toolctx.GetSessionID(ctx); sid != "" {
			loop.SetSessionID(sid)
		}
		if uid := auth.UserIDFrom(ctx); uid != "" {
			loop.SetUserID(uid)
		}
		if f.progressFn != nil {
			loop.SetProgressCallback(f.progressFn)
		}
		if f.streamFn != nil {
			loop.SetStreamCallback(f.streamFn)
		}
		if f.llmCompleteFn != nil {
			loop.SetLLMCompleteCallback(f.llmCompleteFn)
		}
	}

	// 构造 handler（复用 general agent 模式）
	systemPrompt := spec.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "你是一个专用任务执行代理。请根据指令完成任务。"
	}

	// 判断是否为叶子节点：当前深度 + 1 >= maxSpawnDepth 时，该 agent 不应再 spawn 子 agent
	isLeaf := spec.SpawnDepth+1 >= f.maxSpawnDepth

	// 构建工具过滤器：优先使用 ToolPolicy（支持 group:/profile: 展开 + subagent deny），
	// fallback 到直接 NewToolFilter（向后兼容）
	var toolFilter *skills.ToolFilter
	if f.toolPolicy != nil && len(spec.Tools) > 0 {
		expanded := f.toolPolicy.ExpandGroups(spec.Tools)
		toolFilter = f.toolPolicy.BuildFilter("", expanded, true, isLeaf)
	} else if f.toolPolicy != nil && len(spec.Tools) == 0 {
		// 未指定工具列表但有策略引擎 → 应用 subagent deny
		toolFilter = f.toolPolicy.BuildFilter("", nil, true, isLeaf)
	} else if len(spec.Tools) > 0 {
		toolFilter = skills.NewToolFilter(spec.Tools)
	}

	handler := func(ctx context.Context, req TaskRequest) TaskResponse {
		payload, _ := ExtractPayload(req)

		// 兼容两种输入格式：
		//   - spawn_agent 路径: {"instruction": "...", "context": {...}}
		//   - 简化路径:         {"task": "...", ...}
		var taskReq struct {
			Instruction string                 `json:"instruction"`
			Task        string                 `json:"task"`
			Context     map[string]interface{} `json:"context,omitempty"`
		}
		if err := json.Unmarshal(payload, &taskReq); err != nil {
			return TaskResponse{
				Status: "failed",
				Error:  "无效的任务请求: " + err.Error(),
			}
		}

		// 优先使用 instruction，回退到 task
		instruction := taskReq.Instruction
		if instruction == "" {
			instruction = taskReq.Task
		}
		if instruction == "" {
			// 最后兜底：将整个 payload 当作指令文本
			instruction = string(payload)
		}

		if loop == nil {
			return TaskResponse{
				Status: "failed",
				Error:  "LLM 未配置，无法执行任务",
			}
		}

		// 构建消息
		userMsg := fmt.Sprintf("任务指令: %s\n", instruction)
		if taskReq.Context != nil {
			ctxJSON, _ := json.Marshal(taskReq.Context)
			userMsg += fmt.Sprintf("附加上下文: %s\n", string(ctxJSON))
		}

		messages := []llm.MessageWithTools{
			{Role: "user", Content: llm.NewTextContent(userMsg)},
		}

		result, err := loop.Run(ctx, systemPrompt, messages, toolFilter)
		if err != nil {
			return TaskResponse{
				Status: "failed",
				Error:  fmt.Sprintf("agent loop 执行失败: %v", err),
			}
		}

		resultJSON, marshalErr := json.Marshal(map[string]string{
			"summary": result,
		})
		if marshalErr != nil {
			return TaskResponse{
				Status: "failed",
				Error:  fmt.Sprintf("序列化动态 Agent 结果失败: %v", marshalErr),
			}
		}
		return TaskResponse{
			Status: "completed",
			Result: resultJSON,
		}
	}

	// 创建 BaseAgent（动态 Agent 打 Dynamic=true 标记，供前端/事件区分 fixed vs dynamic）
	card := AgentCard{
		ID:          spec.ID,
		Name:        spec.Name,
		Description: spec.Description,
		Dynamic:     true,
	}
	agent := NewBaseAgent(card, handler, f.skillReg, f.logger)

	// 注册到 Registry 和 Transport
	if f.registrar != nil {
		if err := f.registrar.RegisterDynamic(agent); err != nil {
			return nil, errs.Wrap(errs.CodeInvalidInput, fmt.Sprintf("注册动态 agent %q 失败", spec.ID), err)
		}
	}

	// 加入跟踪列表（在 goroutine 启动前，确保 DestroyAgent 能找到它）
	f.dynamicAgents[sessionID][spec.ID] = agent

	f.logger.Info("动态 agent 已创建",
		zap.String("id", spec.ID),
		zap.String("name", spec.Name),
		zap.Int("max_turns", spec.MaxTurns),
		zap.Int("tools", len(spec.Tools)),
		zap.Int("spawn_depth", spec.SpawnDepth),
		zap.Bool("is_leaf", isLeaf),
	)

	return agent, nil
}

// DestroyAgent 停止并注销指定的动态 Agent（遍历所有 session 查找）
func (f *AgentFactory) DestroyAgent(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for sessionID, bucket := range f.dynamicAgents {
		if agent, exists := bucket[id]; exists {
			agent.Stop()
			if f.registrar != nil {
				f.registrar.UnregisterDynamic(id)
			}
			delete(bucket, id)
			// 清理空桶
			if len(bucket) == 0 {
				delete(f.dynamicAgents, sessionID)
			}
			f.logger.Info("动态 agent 已销毁", zap.String("id", id), zap.String("session_id", sessionID))
			return nil
		}
	}
	return errs.New(errs.CodeAgentNotFound, fmt.Sprintf("动态 agent %q 不存在", id))
}

// CleanupBySession 销毁指定 session 的所有动态 Agent
func (f *AgentFactory) CleanupBySession(sessionID string) {
	f.mu.Lock()
	bucket, exists := f.dynamicAgents[sessionID]
	if !exists || len(bucket) == 0 {
		f.mu.Unlock()
		return
	}
	// 收集 ID 列表，释放锁后逐个销毁（避免持锁期间执行 Stop）
	ids := make([]string, 0, len(bucket))
	for id := range bucket {
		ids = append(ids, id)
	}
	f.mu.Unlock()

	for _, id := range ids {
		if err := f.DestroyAgent(id); err != nil {
			f.logger.Warn("清理动态 agent 失败", zap.String("id", id), zap.String("session_id", sessionID), zap.Error(err))
		}
	}
}

// CleanupAll 销毁所有 session 的所有动态 Agent（仅用于 shutdown）
func (f *AgentFactory) CleanupAll() {
	f.mu.RLock()
	sessionIDs := make([]string, 0, len(f.dynamicAgents))
	for sid := range f.dynamicAgents {
		sessionIDs = append(sessionIDs, sid)
	}
	f.mu.RUnlock()

	for _, sid := range sessionIDs {
		f.CleanupBySession(sid)
	}
}

// ListDynamic 返回所有动态 Agent 的卡片列表
func (f *AgentFactory) ListDynamic() []AgentCard {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var cards []AgentCard
	for _, bucket := range f.dynamicAgents {
		for _, agent := range bucket {
			cards = append(cards, agent.Card())
		}
	}
	return cards
}

// DynamicCount 返回当前全局动态 Agent 数量
func (f *AgentFactory) DynamicCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	count := 0
	for _, bucket := range f.dynamicAgents {
		count += len(bucket)
	}
	return count
}

// DynamicCountBySession 返回指定 session 的动态 Agent 数量
func (f *AgentFactory) DynamicCountBySession(sessionID string) int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.dynamicAgents[sessionID])
}

// IsDynamic 检查指定 ID 是否为动态 Agent（遍历所有 session）
func (f *AgentFactory) IsDynamic(id string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, bucket := range f.dynamicAgents {
		if _, exists := bucket[id]; exists {
			return true
		}
	}
	return false
}
