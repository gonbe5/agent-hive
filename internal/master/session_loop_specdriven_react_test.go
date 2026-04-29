package master

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// newObservedLogger 用 zap/observer 构造可断言日志字段的 logger，复用在本文件四路断言。
// Debug level 打开——logSpecCtxAtReactEntry 用的是 Debug，生产默认 Info 不会打，
// 但测试需要可观察，明确拉低阈值。
func newObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, recorded := observer.New(zapcore.DebugLevel)
	return zap.New(core), recorded
}

// TestLogSpecCtxAtReactEntry_WithCtx — 主路径锁：session 挂了 specCtx 时，日志字段
// 必须透传 change_id / current_task_key / revision。这是 task 4.4 "runReActLoop
// 读侧真到达" 的直接证据。
//
// 蓝军反锁（R-mutation）：
//   - 去掉 session.LoadSpecCtx() 调用 → change_id 字段缺失断言红 ✓ 会杀穿
//   - 把 ctx.ChangeID 换成 session.ID → 字段值不匹配断言红 ✓ 会杀穿
func TestLogSpecCtxAtReactEntry_WithCtx(t *testing.T) {
	logger, recorded := newObservedLogger(t)
	session := &SessionState{ID: "sess-react-1"}
	session.StoreSpecCtx(&specdriven.Context{
		ChangeID:       "add-login",
		CurrentTaskKey: "1.1",
		Revision:       3,
	})

	logSpecCtxAtReactEntry(logger, session)

	entries := recorded.FilterMessage("specdriven.react_entry specCtx=present").All()
	require.Len(t, entries, 1, "specCtx 非 nil 时必须打 'present' 分支日志")

	fields := entries[0].ContextMap()
	assert.Equal(t, true, fields["present"], "present 字段必须 true")
	assert.Equal(t, "sess-react-1", fields["session_id"])
	assert.Equal(t, "add-login", fields["change_id"], "change_id 必须从 specCtx 透传")
	assert.Equal(t, "1.1", fields["current_task_key"])
	assert.Equal(t, int64(3), fields["revision"], "revision 必须原值透传（int→int64 zap 编码）")
}

// TestLogSpecCtxAtReactEntry_NoSpecCtx — nil specCtx 路径：session 未挂 ctx 时
// 必须打 "none" 分支，present=false，不能 panic，不能访问 nil pointer 字段。
//
// 蓝军 mutation：if ctx == nil 改成 ctx != nil → panic on field access，本测试红。
func TestLogSpecCtxAtReactEntry_NoSpecCtx(t *testing.T) {
	logger, recorded := newObservedLogger(t)
	session := &SessionState{ID: "sess-no-ctx"}
	// 刻意不 StoreSpecCtx——模拟 legacy intake 路径

	assert.NotPanics(t, func() {
		logSpecCtxAtReactEntry(logger, session)
	}, "nil specCtx 必须零 panic")

	entries := recorded.FilterMessage("specdriven.react_entry specCtx=none").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	assert.Equal(t, false, fields["present"], "present 字段必须 false")
	assert.Equal(t, "sess-no-ctx", fields["session_id"])
	// 关键反锁：nil 分支绝不应有 change_id 字段（防 mutation 把 ctx 字段漏进 nil 分支）
	assert.NotContains(t, fields, "change_id", "nil 分支禁止携带 change_id——否则掩盖 specCtx 真 nil 语义")
}

// TestLogSpecCtxAtReactEntry_NilSession — 防御路径：session 自己是 nil 时
// 不能崩。runReActLoop 外层保证 session 非 nil，但 helper 独立可测，契约要钉死。
func TestLogSpecCtxAtReactEntry_NilSession(t *testing.T) {
	logger, recorded := newObservedLogger(t)

	assert.NotPanics(t, func() {
		logSpecCtxAtReactEntry(logger, nil)
	})

	entries := recorded.FilterMessage("specdriven.react_entry specCtx=none").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	assert.Equal(t, "nil_session", fields["reason"], "nil session 路径必须标 reason=nil_session 便于生产区分两种 none")
}

// TestLogSpecCtxAtReactEntry_NilLogger — logger nil 时必须 no-op 不 panic。
// 生产 Master 初始化顺序可能存在 logger 未 wire 的窗口，zap.Logger.Debug(nil)
// 会 panic，必须在 helper 层防御。
func TestLogSpecCtxAtReactEntry_NilLogger(t *testing.T) {
	session := &SessionState{ID: "x"}
	assert.NotPanics(t, func() {
		logSpecCtxAtReactEntry(nil, session)
	}, "nil logger 必须 no-op")
}

// TestLogSpecCtxAtReactEntry_ZeroLocks — 契约文档反锁：helper 内部只能走
// atomic Load，不能引入任何互斥操作（防 Codex P0-6 回归：runReActLoop 外层
// 已持会话锁，内层再加互斥 getter = 死锁）。
//
// 用 200 reader goroutine × 50 iter 并发跑 helper 配一个滚动 Store writer——
// 如果内部加了互斥锁 `-race` 不会红（atomic 和 mutex 本身都是 race-safe），
// 但 mutex 会在外层锁竞争场景死锁。此测试不能完美证"零锁"，但能证本 helper
// 在高并发下既不 panic 也不死锁（10s 超时内跑完 10000 次调用）。
func TestLogSpecCtxAtReactEntry_ZeroLocks(t *testing.T) {
	logger, _ := newObservedLogger(t)
	session := &SessionState{ID: "sess-race"}
	session.StoreSpecCtx(&specdriven.Context{ChangeID: "c1", Revision: 1})

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			session.StoreSpecCtx(&specdriven.Context{ChangeID: "c1", Revision: i + 1})
		}
		close(done)
	}()

	readers := 200
	startCh := make(chan struct{})
	doneReaders := make(chan struct{}, readers)
	for r := 0; r < readers; r++ {
		go func() {
			<-startCh
			for j := 0; j < 50; j++ {
				logSpecCtxAtReactEntry(logger, session)
			}
			doneReaders <- struct{}{}
		}()
	}
	close(startCh)
	for r := 0; r < readers; r++ {
		<-doneReaders
	}
	<-done
	// 无断言——测试过程中 -race 不红 + 不死锁即通过
}
