package master

import (
	"go.uber.org/zap"
)

// logSpecCtxAtReactEntry 把当前 session 挂的 specCtx 读出来打诊断日志（task 4.4）。
//
// 设计契约：
//   - 读侧严格 atomic.Pointer.Load()（SessionState.LoadSpecCtx），不持任何锁。
//     Codex P0-6 红线：runReActLoop 外层已持会话锁，这里再引入互斥 getter 会死锁。
//   - specCtx == nil 等价于"本 session 非 spec-driven（或 intake 已 fail-closed 清零）"，
//     打一行 "none" 日志便于 grep 诊断，绝不 panic。
//   - 非 nil 时打 change_id / current_task_key / revision 三字段——足够在生产排查
//     "为什么这个 session 没走 spec"时快速对齐，又不膨胀到把整个 Plan.Steps dump。
//
// 为什么不 emit metric：task 4.4 只要求"读 specCtx"证明 plumbing 到达 react 层；
// 加 metric 会改 Prom 契约（新 series），归 Sprint 3.3.e smoke 阶段再决定。
//
// 蓝军 mutation 点位：
//   - 去掉 LoadSpecCtx 调用 → TestLogSpecCtxAtReactEntry_WithCtx change_id 断言红
//   - 改 nil 分支打 present=true → _NilSession / _NoSpecCtx 断言红
func logSpecCtxAtReactEntry(logger *zap.Logger, session *SessionState) {
	if logger == nil {
		return
	}
	if session == nil {
		logger.Debug("specdriven.react_entry specCtx=none",
			zap.Bool("present", false),
			zap.String("reason", "nil_session"),
		)
		return
	}
	ctx := session.LoadSpecCtx()
	if ctx == nil {
		logger.Debug("specdriven.react_entry specCtx=none",
			zap.Bool("present", false),
			zap.String("session_id", session.ID),
		)
		return
	}
	logger.Debug("specdriven.react_entry specCtx=present",
		zap.Bool("present", true),
		zap.String("session_id", session.ID),
		zap.String("change_id", ctx.ChangeID),
		zap.String("current_task_key", ctx.CurrentTaskKey),
		zap.Int("revision", ctx.Revision),
	)
}
