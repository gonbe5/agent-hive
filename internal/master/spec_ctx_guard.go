package master

import (
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// Spec-driven Phase 2 Guard 4：runtime guard 检测非法 StoreSpecCtx。
//
// 设计目标（design.md D5）：SessionSpecState 只允许 user ingress 路径写——
// subagent / tool / background worker 写了就意味着"分裂的 state"会把 continuation 决策搞坏。
// 我们没有 go-runtime 层的 goroutine 类型系统，只能靠：
//  1. panic-in-test（通过 specCtxGuardPanic flag；测试 setup 时启用）
//  2. counter + structured log（prod）——作为 `spec_state_unauthorized_write_total` metric 的源
//
// 关键：guard 不应降低合法写入的性能。Store 正常路径里加一次 atomic.Bool.Load()
// 读 guard 标记 + 条件判定，几乎零开销（atomic load on ARM/x86 是普通 mov）。

// specCtxGuardPanic 为 true 时，StoreSpecCtxGuarded 在 !allowed 调用上 panic。
// 测试启用；prod 保持 false（只走 counter + log）。
var specCtxGuardPanic atomic.Bool

// specCtxUnauthorizedWrites 累计未授权 Store 次数。
// Prom metric exporter 应周期性读取并清零（或保留累计值）。
var specCtxUnauthorizedWrites atomic.Uint64

// SetSpecCtxGuardPanic 调节 panic 模式。**仅测试使用**。
// 生产绝不调用——否则一次意外 spawn 的 subagent 会把用户的 session 炸掉。
func SetSpecCtxGuardPanic(on bool) { specCtxGuardPanic.Store(on) }

// SpecCtxUnauthorizedWrites 返回当前未授权写计数（不重置）。
// Prom metric handler 暴露这个值作为 spec_state_unauthorized_write_total。
func SpecCtxUnauthorizedWrites() uint64 { return specCtxUnauthorizedWrites.Load() }

// ResetSpecCtxGuardCounter 供测试用，prod 勿调。
func ResetSpecCtxGuardCounter() { specCtxUnauthorizedWrites.Store(0) }

// StoreSpecCtxGuarded 是对 StoreSpecCtx 的"授权版"包装：
//   - allowed=true（user ingress 路径）直接写入
//   - allowed=false（subagent / tool 误触）在 test build 下 panic；prod 下 counter++ + warn log
//
// 这是 Guard 4 的软 landing：我们没法阻止 subagent 拿到 session 指针，
// 但调用 StoreSpecCtxGuarded 会被明确拦截。直接 StoreSpecCtx 的裸调用应由
// code review / lint 禁掉。
func (s *SessionState) StoreSpecCtxGuarded(ctx *specdriven.Context, allowed bool, logger *zap.Logger) {
	if allowed {
		s.specCtx.Store(ctx)
		return
	}
	specCtxUnauthorizedWrites.Add(1)
	if logger != nil {
		logger.Warn("spec_ctx unauthorized Store attempted",
			zap.String("session_id", s.ID),
			zap.Uint64("total_unauthorized", specCtxUnauthorizedWrites.Load()),
		)
	}
	if specCtxGuardPanic.Load() {
		panic("spec_ctx unauthorized Store: subagent/tool path must not write SpecState")
	}
}
