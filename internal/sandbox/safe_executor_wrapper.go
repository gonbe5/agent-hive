package sandbox

import (
	"context"
	"fmt"
)

// SafeExecChecker 安全策略检查接口（与 tools.SafeExecChecker 一致）。
type SafeExecChecker interface {
	MatchPolicy(command string) string
}

// SafeExecutorWrapper 用 Decorator 模式包装 Executor，在执行前进行安全策略检查。
// 如果 checker 为 nil，则透传到底层 Executor。
type SafeExecutorWrapper struct {
	inner   Executor
	checker SafeExecChecker
}

// NewSafeExecutorWrapper 创建安全检查装饰器。
// checker 可以为 nil（此时所有命令直接透传），后续可通过 SetChecker 延迟注入。
func NewSafeExecutorWrapper(inner Executor, checker SafeExecChecker) *SafeExecutorWrapper {
	return &SafeExecutorWrapper{inner: inner, checker: checker}
}

// SetChecker 延迟注入安全策略检查器（支持 bootstrap 阶段先创建 wrapper，master 初始化后再绑定规则）。
func (w *SafeExecutorWrapper) SetChecker(checker SafeExecChecker) {
	w.checker = checker
}

// Execute 先检查安全策略，通过后委托给底层 Executor。
// 注意：ask 策略由上层（bash tool）负责处理 HITL 审批，此处只拦截 deny。
func (w *SafeExecutorWrapper) Execute(ctx context.Context, req ExecRequest) (ExecResult, error) {
	if w.checker != nil {
		policy := w.checker.MatchPolicy(req.Command)
		if policy == "deny" {
			return ExecResult{}, fmt.Errorf("命令被安全策略拒绝: %s", req.Command)
		}
	}
	return w.inner.Execute(ctx, req)
}

// Close 委托给底层 Executor。
func (w *SafeExecutorWrapper) Close() error {
	return w.inner.Close()
}
