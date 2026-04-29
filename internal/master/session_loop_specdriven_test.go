package master

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/intake"
)

// newSpecDrivenTestMaster 构造最小 Master，仅填满 applySpecDrivenIntake 所需字段。
// 不启动 Start/worker，不接 store——确保测试纯跑 intake 逻辑，不触碰 LLM/DB。
func newSpecDrivenTestMaster(t *testing.T, mode string) *Master {
	t.Helper()
	return &Master{
		config: Config{
			SpecDriven: config.SpecDrivenConfig{
				Mode: mode,
				Continuation: config.SpecContinuationConfig{
					Default: config.DefaultSpecContinuationDefault,
				},
				Planner: config.SpecPlannerConfig{
					TokenBudget: config.DefaultSpecPlannerTokenBudget,
				},
			},
		},
		logger: zaptest.NewLogger(t),
		// obsCh 留 nil——enqueueMetric 自带 nil 兜底（pg_writer.go 设计），不 panic 即可。
	}
}

func newSpecDrivenTestSession(id string) *SessionState {
	s := &SessionState{ID: id}
	return s
}

// TestApplySpecDrivenIntake_LegacyMode_ShortCircuits 验证 TG10 10.4：
// mode=legacy 时，任何 request 都走 PathLegacy，specCtx 清零。
// 这是 FM-1 反例（默认 off） + FM-3 反例（非流式也统一走 intake）的综合验证。
func TestApplySpecDrivenIntake_LegacyMode_ShortCircuits(t *testing.T) {
	m := newSpecDrivenTestMaster(t, string(intake.ModeLegacy))
	session := newSpecDrivenTestSession("sess-legacy")
	// 预置 specCtx 非 nil——验证 hook 会清零（防止老值残留）。
	session.StoreSpecCtx(&specdriven.Context{ChangeID: "stale-ctx"})

	path := m.applySpecDrivenIntake(session, "hello world")

	assert.Equal(t, intake.PathLegacy, path, "mode=legacy 必须走 PathLegacy")
	assert.Nil(t, session.LoadSpecCtx(), "legacy 分支必须 StoreSpecCtx(nil) 清零，防止老值污染")
}

// TestApplySpecDrivenIntake_InvalidMode_FailsClosed 验证 design.md fail-closed 纪律：
// 非法 mode 值 → 当作 legacy，而不是 panic 或走 spec 路径。
func TestApplySpecDrivenIntake_InvalidMode_FailsClosed(t *testing.T) {
	m := newSpecDrivenTestMaster(t, "totally-bogus-mode")
	session := newSpecDrivenTestSession("sess-invalid")

	path := m.applySpecDrivenIntake(session, "hello")

	assert.Equal(t, intake.PathLegacy, path, "非法 mode 必须 fail-closed 到 legacy")
	assert.Nil(t, session.LoadSpecCtx(), "非法 mode 禁止挂 specCtx")
}

// TestApplySpecDrivenIntake_EmptyRequest_ShortCircuits 验证空请求跳过 spec 路径。
// 理由：没有 user intent，planner 没有 prompt 可 plan，强制 legacy 避免空 LLM 调用。
func TestApplySpecDrivenIntake_EmptyRequest_ShortCircuits(t *testing.T) {
	m := newSpecDrivenTestMaster(t, string(intake.ModeSpec))
	session := newSpecDrivenTestSession("sess-empty")

	path := m.applySpecDrivenIntake(session, "")

	assert.Equal(t, intake.PathLegacy, path, "空 request 即使 mode=spec 也必须 short-circuit")
	assert.Nil(t, session.LoadSpecCtx())
}

// TestApplySpecDrivenIntake_DualMode_DownshiftsStub 验证 TG10 10.5 plumbing 就位：
// 当前 Mode=dual 进入 stub 分支，spec runner 未实现 → DowngradeOnError 把路径降级到 legacy。
// 这不是 dual 真正的"双跑 + diff"语义——而是 fail-closed 占位：
// 未来 spec runner 上线后，本测试会改为断言 PathDual。
func TestApplySpecDrivenIntake_DualMode_DownshiftsStub(t *testing.T) {
	m := newSpecDrivenTestMaster(t, string(intake.ModeDual))
	session := newSpecDrivenTestSession("sess-dual")

	path := m.applySpecDrivenIntake(session, "do something")

	// Phase 2 MVP 契约：runner 未接 LLM client → downshift。
	// 用例意图是保证 plumbing 正确（metric + specCtx 清零 + 路径返 legacy）。
	assert.Equal(t, intake.PathLegacy, path, "dual 模式在 spec runner 未就绪时必须 fail-closed 到 legacy")
	assert.Nil(t, session.LoadSpecCtx(), "downshift 分支必须 StoreSpecCtx(nil)")
}

// TestApplySpecDrivenIntake_SpecMode_DownshiftsStub 同上，验证 Mode=spec plumbing。
func TestApplySpecDrivenIntake_SpecMode_DownshiftsStub(t *testing.T) {
	m := newSpecDrivenTestMaster(t, string(intake.ModeSpec))
	session := newSpecDrivenTestSession("sess-spec")

	path := m.applySpecDrivenIntake(session, "do something else")

	assert.Equal(t, intake.PathLegacy, path, "spec 模式当前桩化为 downshift legacy")
	assert.Nil(t, session.LoadSpecCtx())
}

// TestApplySpecDrivenIntake_DefaultConfigIsLegacy 保护 FM-1 反例的系统级默认：
// 未显式设置 spec_driven.mode 的 config（Mode=""）必须被当作非法/未知处理，
// 即 fail-closed 到 legacy，而不是静默选 spec 路径。
func TestApplySpecDrivenIntake_DefaultConfigIsLegacy(t *testing.T) {
	// Mode="" 未设置时的行为——intake.Mode("") 不在 Valid 枚举里 → fail-closed
	m := newSpecDrivenTestMaster(t, "")
	session := newSpecDrivenTestSession("sess-default")

	path := m.applySpecDrivenIntake(session, "whatever")

	assert.Equal(t, intake.PathLegacy, path, "Mode='' 必须 fail-closed")
}

// TestDefaultSpecDrivenConfig_SystemLevelInvariant 系统级不变量检查——防止默认值被误改。
// FM-1/FM-4 纪律：mode=legacy 默认 + continuation=off 默认 + token_budget=800。
func TestDefaultSpecDrivenConfig_SystemLevelInvariant(t *testing.T) {
	cfg := config.DefaultSpecDrivenConfig

	assert.Equal(t, "legacy", cfg.Mode,
		"FM-1 反例：默认 mode 必须 legacy——任何非 legacy 默认值都会把所有老 session 拖进 spec 路径")
	assert.Equal(t, "off", cfg.Continuation.Default,
		"FM-1 反例：continuation 默认 off——禁止静默 MRU 续写")
	assert.Equal(t, 800, cfg.Planner.TokenBudget,
		"FM-4 反例：planner token_budget 默认 800——防 schema 漏洞推高成本")
}
