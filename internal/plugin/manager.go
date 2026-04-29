package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goplugin "plugin"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// maxPlugins 最大插件数量限制
const maxPlugins = 50

// hookTimeout hook 执行超时时间（Manager 内部使用）
const hookTimeout = 30 * time.Second

// HookCallTimeout hook 触发调用的默认超时（供调用方使用）
const HookCallTimeout = 5 * time.Second

// pluginEntry 已加载插件的内部记录
type pluginEntry struct {
	id    string      // 插件唯一标识
	path  string      // .so 文件路径（为空表示内置/手动注册）
	hooks Hooks       // 该插件提供的 hooks
	input PluginInput // 初始化时传入的参数（重载时复用）
}

// Manager 管理插件的加载和 hook 触发
type Manager struct {
	mu      sync.RWMutex
	hooks   []Hooks                 // 扁平列表，用于高效遍历触发
	plugins map[string]*pluginEntry // 按 ID 索引，用于卸载/重载
	order   []string                // 插入顺序，保证 hook 触发顺序确定
	nextID  int                     // 自增 ID 计数器（RegisterHooks 匿名注册时使用）
	logger  *zap.Logger
}

// NewManager 创建插件管理器
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		plugins: make(map[string]*pluginEntry),
		logger:  logger,
	}
}

// RegisterHooks 直接注册 hooks（用于内置插件或测试）
// 自动分配唯一 ID，格式为 "hook-N"
func (m *Manager) RegisterHooks(hooks Hooks) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.plugins) >= maxPlugins {
		m.logger.Warn("插件数量已达上限，跳过注册",
			zap.Int("当前数量", len(m.plugins)),
			zap.Int("上限", maxPlugins),
		)
		return
	}

	id := fmt.Sprintf("hook-%d", m.nextID)
	m.nextID++

	m.plugins[id] = &pluginEntry{
		id:    id,
		hooks: hooks,
	}
	m.order = append(m.order, id)
	m.rebuildHooksLocked()
}

// rebuildHooksLocked 根据 order 切片重建扁平 hooks 列表
// 调用前必须持有写锁
func (m *Manager) rebuildHooksLocked() {
	result := make([]Hooks, 0, len(m.order))
	for _, id := range m.order {
		if entry, ok := m.plugins[id]; ok {
			result = append(result, entry.hooks)
		}
	}
	m.hooks = result
}

// LoadFromDir 从目录加载 Go 插件
// 扫描 {dir}/*.so 文件，每个必须导出 NewPlugin 符号
func (m *Manager) LoadFromDir(dir string, input PluginInput) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			m.logger.Debug("插件目录不存在，跳过加载", zap.String("dir", dir))
			return nil
		}
		return errs.Wrap(errs.CodePluginLoadFailed, "读取插件目录失败", err)
	}
	if !info.IsDir() {
		return nil
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.so"))
	if err != nil {
		return errs.Wrap(errs.CodePluginLoadFailed, "扫描插件文件失败", err)
	}
	if len(matches) == 0 {
		m.logger.Debug("插件目录为空", zap.String("dir", dir))
		return nil
	}

	for _, path := range matches {
		// 检查插件数量限制
		m.mu.RLock()
		count := len(m.plugins)
		m.mu.RUnlock()
		if count >= maxPlugins {
			m.logger.Warn("插件数量已达上限，停止加载",
				zap.Int("当前数量", count),
				zap.Int("上限", maxPlugins),
			)
			break
		}

		pluginID := pluginIDFromPath(path)

		p, err := goplugin.Open(path)
		if err != nil {
			m.logger.Error("打开插件失败", zap.String("path", path), zap.Error(err))
			continue
		}

		sym, err := p.Lookup("NewPlugin")
		if err != nil {
			m.logger.Error("插件缺少 NewPlugin 符号", zap.String("path", path), zap.Error(err))
			continue
		}

		newPlugin, ok := sym.(*Plugin)
		if !ok {
			m.logger.Error("NewPlugin 符号类型不匹配", zap.String("path", path))
			continue
		}

		hooks, err := (*newPlugin)(input)
		if err != nil {
			m.logger.Error("初始化插件失败", zap.String("path", path), zap.Error(err))
			continue
		}

		m.mu.Lock()
		m.plugins[pluginID] = &pluginEntry{
			id:    pluginID,
			path:  path,
			hooks: hooks,
			input: input,
		}
		m.order = append(m.order, pluginID)
		m.rebuildHooksLocked()
		m.mu.Unlock()

		m.logger.Info("已加载插件", zap.String("id", pluginID), zap.String("path", path))
	}

	return nil
}

// TriggerToolBefore 触发所有 ToolExecuteBefore hooks
// 任一 hook 设置 input.Blocked=true 则中止
func (m *Manager) TriggerToolBefore(ctx context.Context, input *ToolExecuteInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.ToolExecuteBefore == nil {
			continue
		}
		if err := h.ToolExecuteBefore(hookCtx, input); err != nil {
			return err
		}
		if input.Blocked {
			return errs.New(errs.CodeSkillToolBlocked, input.Reason)
		}
	}
	return nil
}

// TriggerToolAfter 触发所有 ToolExecuteAfter hooks
func (m *Manager) TriggerToolAfter(ctx context.Context, input ToolExecuteInput, output *ToolExecuteOutput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.ToolExecuteAfter == nil {
			continue
		}
		if err := h.ToolExecuteAfter(hookCtx, input, output); err != nil {
			return err
		}
	}
	return nil
}

// TriggerChatBefore 触发所有 ChatMessageBefore hooks
func (m *Manager) TriggerChatBefore(ctx context.Context, input *ChatMessageInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.ChatMessageBefore == nil {
			continue
		}
		if err := h.ChatMessageBefore(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerChatAfter 触发所有 ChatMessageAfter hooks
func (m *Manager) TriggerChatAfter(ctx context.Context, input ChatMessageInput, output *ChatMessageOutput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.ChatMessageAfter == nil {
			continue
		}
		if err := h.ChatMessageAfter(hookCtx, input, output); err != nil {
			return err
		}
	}
	return nil
}

// TriggerPermissionAsk 触发所有 PermissionAsk hooks
// 遍历所有已注册的 hook，第一个返回非空 Decision 的结果生效。
// 如果所有 hook 都返回空 Decision，则返回 nil 表示走默认流程。
func (m *Manager) TriggerPermissionAsk(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.PermissionAsk == nil {
			continue
		}
		out, err := h.PermissionAsk(hookCtx, input)
		if err != nil {
			return nil, err
		}
		if out != nil && out.Decision != "" {
			m.logger.Debug("插件处理权限请求",
				zap.String("hook", HookTypePermissionAsk),
				zap.String("tool", input.ToolName),
				zap.String("decision", out.Decision),
				zap.String("reason", out.Reason),
			)
			return out, nil
		}
	}
	// 所有 hook 均未做出决策，返回 nil 走默认流程
	return nil, nil
}

// TriggerToolDefinition 触发所有 ToolDefinition hooks
// 依次调用所有 hook，每个 hook 可修改 input 中的 Description 和 ArgsSchema。
// 集成点 — 在工具注册到 host 时调用此方法（如 internal/tools/tools.go 或 internal/mcphost/host.go）
func (m *Manager) TriggerToolDefinition(ctx context.Context, input *ToolDefinitionInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.ToolDefinitionHook == nil {
			continue
		}
		if err := h.ToolDefinitionHook(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerShellEnv 触发所有 ShellEnv hooks
// 合并所有 hook 返回的环境变量。后注册的 hook 可覆盖先注册的同名变量。
// 集成点 — 在 Bash 工具执行命令前调用此方法（如 internal/tools/shell.go）
func (m *Manager) TriggerShellEnv(ctx context.Context, input *ShellEnvInput) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	merged := make(map[string]string)
	for _, h := range m.hooks {
		if h.ShellEnv == nil {
			continue
		}
		out, err := h.ShellEnv(hookCtx, input)
		if err != nil {
			return nil, err
		}
		if out == nil {
			continue
		}
		for k, v := range out.Env {
			merged[k] = v
		}
	}
	return merged, nil
}

// CustomTools 返回所有插件注册的自定义工具
func (m *Manager) CustomTools() map[string]ToolDefinition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]ToolDefinition)
	for _, h := range m.hooks {
		for name, td := range h.Tools {
			result[name] = td
		}
	}
	return result
}

// HookCount 返回已注册的 hook 数量
func (m *Manager) HookCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.hooks)
}

// PluginInfo 插件信息
type PluginInfo struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ListPlugins 返回已加载的插件列表（网关 API 用）
func (m *Manager) ListPlugins() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PluginInfo, 0, len(m.plugins))
	for _, entry := range m.plugins {
		result = append(result, PluginInfo{
			ID:     entry.id,
			Status: "loaded",
		})
	}
	return result
}

// Reload 重载指定插件
// 对于有 .so 文件路径的插件，先卸载再从原路径重新加载
// 对于手动注册的插件（无文件路径），直接返回错误
func (m *Manager) Reload(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.plugins[id]
	if !ok {
		return errs.New(errs.CodePluginNotFound, "插件未找到: "+id)
	}

	if entry.path == "" {
		return errs.New(errs.CodePluginLoadFailed, "内置插件不支持重载: "+id)
	}

	// 保存路径和初始化参数，用于重新加载
	pluginPath := entry.path
	pluginInput := entry.input

	// 先移除旧的（保留在 order 中的位置）
	delete(m.plugins, id)
	m.rebuildHooksLocked()
	m.logger.Info("重载插件: 已卸载旧实例", zap.String("id", id))

	// 重新加载
	p, err := goplugin.Open(pluginPath)
	if err != nil {
		m.logger.Error("重载插件失败: 无法打开文件", zap.String("id", id), zap.String("path", pluginPath), zap.Error(err))
		return errs.Wrap(errs.CodePluginLoadFailed, "重载插件失败: "+id, err)
	}

	sym, err := p.Lookup("NewPlugin")
	if err != nil {
		m.logger.Error("重载插件失败: 缺少 NewPlugin 符号", zap.String("id", id), zap.Error(err))
		return errs.Wrap(errs.CodePluginLoadFailed, "重载插件失败: "+id, err)
	}

	newPlugin, ok := sym.(*Plugin)
	if !ok {
		return errs.New(errs.CodePluginLoadFailed, "重载插件失败: NewPlugin 类型不匹配: "+id)
	}

	hooks, err := (*newPlugin)(pluginInput)
	if err != nil {
		m.logger.Error("重载插件失败: 初始化异常", zap.String("id", id), zap.Error(err))
		return errs.Wrap(errs.CodePluginLoadFailed, "重载插件失败: "+id, err)
	}

	m.plugins[id] = &pluginEntry{
		id:    id,
		path:  pluginPath,
		hooks: hooks,
		input: pluginInput,
	}
	m.rebuildHooksLocked()
	m.logger.Info("已重载插件", zap.String("id", id), zap.String("path", pluginPath))
	return nil
}

// Unload 卸载指定插件，从管理器中移除其 hooks
func (m *Manager) Unload(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.plugins[id]; !ok {
		return errs.New(errs.CodePluginNotFound, "插件未找到: "+id)
	}

	delete(m.plugins, id)
	m.removeFromOrder(id)
	m.rebuildHooksLocked()
	m.logger.Info("已卸载插件", zap.String("id", id))
	return nil
}

// removeFromOrder 从 order 切片中移除指定 ID
func (m *Manager) removeFromOrder(id string) {
	for i, oid := range m.order {
		if oid == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			return
		}
	}
}

// TriggerSessionStart 触发所有 SessionStart hooks
func (m *Manager) TriggerSessionStart(ctx context.Context, input *SessionStartInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.SessionStart == nil {
			continue
		}
		if err := h.SessionStart(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerSessionEnd 触发所有 SessionEnd hooks
func (m *Manager) TriggerSessionEnd(ctx context.Context, input *SessionEndInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.SessionEnd == nil {
			continue
		}
		if err := h.SessionEnd(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerPreCompact 触发所有 PreCompact hooks
func (m *Manager) TriggerPreCompact(ctx context.Context, input *CompactInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.PreCompact == nil {
			continue
		}
		if err := h.PreCompact(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerPostCompact 触发所有 PostCompact hooks
func (m *Manager) TriggerPostCompact(ctx context.Context, input *CompactInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.PostCompact == nil {
			continue
		}
		if err := h.PostCompact(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerTaskCreated 触发所有 TaskCreated hooks
func (m *Manager) TriggerTaskCreated(ctx context.Context, input *TaskEventInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.TaskCreated == nil {
			continue
		}
		if err := h.TaskCreated(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerTaskCompleted 触发所有 TaskCompleted hooks
func (m *Manager) TriggerTaskCompleted(ctx context.Context, input *TaskEventInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.TaskCompleted == nil {
			continue
		}
		if err := h.TaskCompleted(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerConfigChange 触发所有 ConfigChange hooks
func (m *Manager) TriggerConfigChange(ctx context.Context, input *ConfigChangeInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.ConfigChange == nil {
			continue
		}
		if err := h.ConfigChange(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerFileChanged 触发所有 FileChanged hooks
func (m *Manager) TriggerFileChanged(ctx context.Context, input *FileChangedInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.FileChanged == nil {
			continue
		}
		if err := h.FileChanged(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerAgentSpawned 触发所有 AgentSpawned hooks
func (m *Manager) TriggerAgentSpawned(ctx context.Context, input *AgentLifecycleInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.AgentSpawned == nil {
			continue
		}
		if err := h.AgentSpawned(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerAgentDestroyed 触发所有 AgentDestroyed hooks
func (m *Manager) TriggerAgentDestroyed(ctx context.Context, input *AgentLifecycleInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.AgentDestroyed == nil {
			continue
		}
		if err := h.AgentDestroyed(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// TriggerJournalEntry 触发所有 JournalEntry hooks
func (m *Manager) TriggerJournalEntry(ctx context.Context, input *JournalEntryInput) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hookCtx, cancel := context.WithTimeout(ctx, hookTimeout)
	defer cancel()

	for _, h := range m.hooks {
		if h.JournalEntry == nil {
			continue
		}
		if err := h.JournalEntry(hookCtx, input); err != nil {
			return err
		}
	}
	return nil
}

// pluginIDFromPath 从 .so 文件路径提取插件 ID
// 例如 "/path/to/myplugin.so" → "myplugin"
func pluginIDFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		base = base[:len(base)-len(ext)]
	}
	return base
}
