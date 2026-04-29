package skills

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// HookConfig 定义 skill 的生命周期 hook
type HookConfig struct {
	PreInvoke  []string `yaml:"pre-invoke" json:"pre_invoke,omitempty"`
	PostInvoke []string `yaml:"post-invoke" json:"post_invoke,omitempty"`
}

// HookRunner 为 skill 执行生命周期 hook
type HookRunner struct {
	executor ShellExecutor
	logger   *zap.Logger
}

// NewHookRunner 创建新的 HookRunner
func NewHookRunner(executor ShellExecutor, logger *zap.Logger) *HookRunner {
	return &HookRunner{
		executor: executor,
		logger:   logger,
	}
}

// RunPreInvoke 运行所有 pre-invoke hook。遇到第一个错误时停止
func (h *HookRunner) RunPreInvoke(hooks *HookConfig, skillDir string) error {
	if hooks == nil || len(hooks.PreInvoke) == 0 {
		return nil
	}
	for _, cmd := range hooks.PreInvoke {
		h.logger.Debug("运行 pre-invoke hook", zap.String("command", cmd), zap.String("skill_dir", skillDir))
		if _, _, err := h.executor.Execute(fmt.Sprintf("cd %q && %s", skillDir, cmd)); err != nil {
			return errs.Wrap(errs.CodeSkillHookFailed, fmt.Sprintf("pre-invoke hook %q failed", cmd), err)
		}
	}
	return nil
}

// RunPostInvoke 运行所有 post-invoke hook。遇到第一个错误时停止
func (h *HookRunner) RunPostInvoke(hooks *HookConfig, skillDir string) error {
	if hooks == nil || len(hooks.PostInvoke) == 0 {
		return nil
	}
	for _, cmd := range hooks.PostInvoke {
		h.logger.Debug("运行 post-invoke hook", zap.String("command", cmd), zap.String("skill_dir", skillDir))
		if _, _, err := h.executor.Execute(fmt.Sprintf("cd %q && %s", skillDir, cmd)); err != nil {
			return errs.Wrap(errs.CodeSkillHookFailed, fmt.Sprintf("post-invoke hook %q failed", cmd), err)
		}
	}
	return nil
}
