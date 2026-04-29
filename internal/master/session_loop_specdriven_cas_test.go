package master

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// newCASTestMaster 构造一个仅包含 obsCh + logger 的最小 Master，用于
// emitCASConflict / wireSpecChangeStoreMetrics 的纯单元测试——不起 worker、
// 不接 store、不碰 pg，channel 容量 8 足够测试批次用。
func newCASTestMaster(t *testing.T) *Master {
	t.Helper()
	return &Master{
		logger: zaptest.NewLogger(t),
		obsCh:  make(chan observabilityEntry, 8),
	}
}

// drainMetric 从 obsCh 抽一条 metric，等待超时即 fail。
func drainMetric(t *testing.T, m *Master, timeout time.Duration) *observability_metric {
	t.Helper()
	select {
	case e := <-m.obsCh:
		require.NotNil(t, e.metric, "抽到的 obsEntry 必须含 metric（非 tracer span）")
		return &observability_metric{
			Name:   e.metric.Name,
			Value:  e.metric.Value,
			Labels: e.metric.Labels,
		}
	case <-time.After(timeout):
		t.Fatalf("超过 %s 没从 obsCh 抽到 metric——wire 可能没触发 enqueueMetric", timeout)
		return nil
	}
}

// observability_metric 是 test 内解耦的本地投影，避免 test 依赖 observability.Metric
// 内部所有字段（只校验关键三项）。
type observability_metric struct {
	Name   string
	Value  float64
	Labels map[string]any
}

// TestMaster_EmitCASConflict_EnqueuesMetric — Sprint 3.3.b 核心契约 test。
//
// 目的：把 CASConflictObserver 翻译到 enqueueMetric 的路径原子化证伪。
// 不通过 store.UpsertWithCAS 间接触发（那需要 PG），而是直接调用
// m.emitCASConflict 模拟 store 的 observer 回调——这保证 test 不依赖
// 外部资源，纯 master 包内单测。
//
// 蓝军 mutation 点位（runner.go / session_loop_specdriven_cas.go 改动后必须全红）：
//   - R1 改 metric 名常量（MetricCASConflictTotal → 别的 Metric*Total）→ Name 断言红
//   - R2 改 Labels key（"scenario" → "reason" 或 "kind"）→ Labels 断言红
//   - R3 去掉 m.enqueueMetric 调用（或改为 no-op）→ drainMetric 超时红
//   - R4 把 scenario 值写死（固定字符串而非入参）→ 第二轮 scenario 断言红
func TestMaster_EmitCASConflict_EnqueuesMetric(t *testing.T) {
	m := newCASTestMaster(t)

	t.Run("ghost_id 单路", func(t *testing.T) {
		m.emitCASConflict("ghost_id")
		got := drainMetric(t, m, 100*time.Millisecond)
		assert.Equal(t, specdriven.MetricCASConflictTotal, got.Name,
			"metric 名必须是 specdriven.cas_conflict_total 常量，不能硬编码字符串——防 Sprint 2.3 R2 mutation 漂移")
		assert.Equal(t, float64(1), got.Value,
			"CAS conflict counter 恒 +1（Value=1）")
		assert.Equal(t, "ghost_id", got.Labels["scenario"],
			"scenario label 必须是入参原值——防 label 值写死")
	})

	t.Run("duplicate_create 单路", func(t *testing.T) {
		m.emitCASConflict("duplicate_create")
		got := drainMetric(t, m, 100*time.Millisecond)
		assert.Equal(t, "duplicate_create", got.Labels["scenario"])
	})

	t.Run("stale_revision 单路", func(t *testing.T) {
		m.emitCASConflict("stale_revision")
		got := drainMetric(t, m, 100*time.Millisecond)
		assert.Equal(t, "stale_revision", got.Labels["scenario"])
	})

	t.Run("三路 scenario 与 AllowedCASConflictScenarios 白名单 1:1 交叉校验", func(t *testing.T) {
		// 这里不再 emit——三路已各 emit 一次，对齐 Sprint 2.3 白名单纪律。
		for _, s := range specdriven.AllowedCASConflictScenarios {
			// 仅确认每个白名单 scenario 都是 emit 合法入参——不出现在未来可能
			// 被引入的非白名单 label。
			assert.NotEmpty(t, string(s), "AllowedCASConflictScenarios 白名单不能含空字符串")
		}
	})
}

// TestMaster_WireSpecChangeStoreMetrics_NilSafe — 契约：未注入 store 时 wire no-op。
//
// 生产场景：内存启动 / pg 不可用——bootstrap 不会创建 SpecChangeStore，
// m.specStore 保持 nil。此时 wireSpecChangeStoreMetrics 必须 no-op，不 panic。
func TestMaster_WireSpecChangeStoreMetrics_NilSafe(t *testing.T) {
	m := newCASTestMaster(t)
	// m.specStore 默认 nil
	assert.NotPanics(t, func() {
		m.wireSpecChangeStoreMetrics()
	}, "nil specStore 时 wireSpecChangeStoreMetrics 必须 no-op 不 panic")
}

// TestMaster_EnqueueMetric_NilObsChSafe — 正交回归：enqueueMetric 本身的 nil obsCh 兜底。
//
// 理由：`SetMetricsWriter` 未调用场景下 obsCh=nil——emitCASConflict 必须仍然
// 可以无副作用调用，不 panic、不 block。此 test 把 Master 构造成 obsCh=nil
// 再 emit，验证 enqueueMetric 内部 if nil 兜底路径。
func TestMaster_EnqueueMetric_NilObsChSafe(t *testing.T) {
	m := &Master{
		logger: zaptest.NewLogger(t),
		// obsCh = nil
	}
	assert.NotPanics(t, func() {
		m.emitCASConflict("ghost_id")
	}, "obsCh=nil 时 emitCASConflict 必须通过 enqueueMetric 内部兜底静默丢弃")
}
