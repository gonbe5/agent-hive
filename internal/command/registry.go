package command

import (
	"sort"
	"sync"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/skills"
)

// Registry 统一命令注册表
type Registry struct {
	mu       sync.RWMutex
	commands map[string]*Info
	logger   *zap.Logger
}

// NewRegistry 创建新的命令注册表
func NewRegistry(logger *zap.Logger) *Registry {
	return &Registry{
		commands: make(map[string]*Info),
		logger:   logger,
	}
}

// Register 注册命令（同名按优先级覆盖: builtin > config > mcp > skill）
func (r *Registry) Register(cmd *Info) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.commands[cmd.Name]
	if ok {
		// 已存在且优先级更高则不覆盖
		if sourcePriority(existing.Source) <= sourcePriority(cmd.Source) {
			r.logger.Debug("跳过低优先级命令注册",
				zap.String("name", cmd.Name),
				zap.String("existing_source", string(existing.Source)),
				zap.String("new_source", string(cmd.Source)),
			)
			return
		}
	}

	r.commands[cmd.Name] = cmd
	r.logger.Debug("注册命令", zap.String("name", cmd.Name), zap.String("source", string(cmd.Source)))
}

// Get 根据名称获取命令
func (r *Registry) Get(name string) (*Info, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, ok := r.commands[name]
	if !ok {
		return nil, errs.New(errs.CodeNotFound, "未找到命令: /"+name)
	}
	return cmd, nil
}

// List 返回所有命令（按名称排序）
func (r *Registry) List() []*Info {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Info, 0, len(r.commands))
	for _, cmd := range r.commands {
		result = append(result, cmd)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// ListBySource 按来源过滤
func (r *Registry) ListBySource(source Source) []*Info {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Info
	for _, cmd := range r.commands {
		if cmd.Source == source {
			result = append(result, cmd)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// LoadFromSkills 从 skill 注册表加载 user-invocable skill 为命令
// UserInvocable 为 nil 或 true 的 skill 都加载
func (r *Registry) LoadFromSkills(skillReg *skills.Registry) {
	invocable := skillReg.ListUserInvocable()
	for _, sm := range invocable {
		// 构建模板：将 skill 内容作为消息发送给 master
		template := sm.Name
		if sm.ArgumentHint != "" {
			template = sm.Name + " $ARGUMENTS"
		}
		r.Register(&Info{
			Name:        sm.Name,
			Description: sm.Description,
			Source:      SourceSkill,
			Template:    template,
			Model:       sm.Model,
			Agent:       sm.Agent,
			Hints:       ExtractHints(template),
		})
	}
	r.logger.Info("从 skill 注册表加载命令", zap.Int("count", len(invocable)))
}

// LoadFromConfig 从配置加载用户自定义命令
func (r *Registry) LoadFromConfig(commands map[string]config.CommandConfig) {
	for name, cfg := range commands {
		r.Register(&Info{
			Name:        name,
			Description: cfg.Description,
			Agent:       cfg.Agent,
			Model:       cfg.Model,
			Source:      SourceConfig,
			Template:    cfg.Template,
			Subtask:     cfg.Subtask,
			Hints:       ExtractHints(cfg.Template),
		})
	}
	r.logger.Info("从配置加载命令", zap.Int("count", len(commands)))
}

// LoadBuiltins 加载内置命令
func (r *Registry) LoadBuiltins() {
	r.Register(&Info{
		Name:        "help",
		Description: "显示此帮助信息",
		Source:      SourceBuiltin,
		Template:    "",
	})
	r.Register(&Info{
		Name:        "pause",
		Description: "暂停当前任务",
		Source:      SourceBuiltin,
		Template:    "",
	})
	r.Register(&Info{
		Name:        "resume",
		Description: "恢复已暂停的任务",
		Source:      SourceBuiltin,
		Template:    "",
	})
	r.Register(&Info{
		Name:        "cancel",
		Description: "取消当前任务",
		Source:      SourceBuiltin,
		Template:    "",
	})
	r.logger.Debug("内置命令已加载")
}
