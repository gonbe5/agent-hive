package master

import "go.uber.org/zap"

// NewForRegressionTest 构造最小化 Master 给外部 regression 测试包使用
// (tests/regression/).  仅 eventBus + logger，不拉起 PG/transport/registry，
// 用于纯 envelope invariant 断言.
//
// 使用场景：session-scope-regression-matrix spec R-1/R-2 envelope invariant
// 红测从外部 tests/regression/ 包驱动 Master 的 Callback，订阅 eventBus 观察
// BroadcastMessage.SessionID 是否被正确携带。
//
// 生产代码禁止使用。
func NewForRegressionTest(logger *zap.Logger, eb *EventBus) *Master {
	return &Master{
		eventBus: eb,
		logger:   logger,
	}
}
