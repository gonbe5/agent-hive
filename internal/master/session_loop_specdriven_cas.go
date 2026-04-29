package master

import (
	"time"

	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/store"
)

// SetSpecChangeStore 注入 spec-driven CAS 写入存储（Sprint 3.3.b）。
//
// 生产路径：bootstrap/server.go 在 pg pool 可用时调用
// `master.SetSpecChangeStore(store.NewSpecChangeStore(pool, logger))`，随后
// 立即调用 `wireSpecChangeStoreMetrics` 把 conflict observer 接到 metric 队列。
//
// 测试路径：可传 nil（wire 函数 no-op）或 real store（PG 集成 test 场景）。
//
// 纪律：Set + wire 一次性完成——避免 bootstrap 忘记 wire 造成 observer 悬空。
// 传 nil 等价于禁用 spec-change 写路径（测试或 pg 不可用场景）。
func (m *Master) SetSpecChangeStore(s *store.SpecChangeStore) {
	m.specStore = s
	m.wireSpecChangeStoreMetrics()
}

// wireSpecChangeStoreMetrics 把 Sprint 2.3 的 CASConflictObserver 基础设施
// 翻译成 master.enqueueMetric 调用（Sprint 3.3.b task 12.13.a ADD 项）。
//
// 流向：
//
//	store.UpsertWithCAS 三路冲突分支
//	  → store.emitConflict(scenario)
//	  → observer callback（此处注入）
//	  → m.emitCASConflict(scenario)
//	  → m.enqueueMetric(MetricCASConflictTotal, {scenario})
//	  → obsCh → pg_writer flush
//
// 语义契约（design.md FM-7 + tasks.md Sprint 2.3 蓝军 R1/R2 锁死）：
//   - observer 必须是 O(1) 非阻塞——emit via enqueueMetric 走 channel，队列满丢弃。
//   - scenario 字符串来自 AllowedCASConflictScenarios 白名单，无 cardinality 漂移风险。
//   - nil store 安全——未注入时 no-op（测试或 pg 不可用场景）。
//
// nil 安全：m.specStore == nil → 直接 return；m.obsCh == nil（未 SetMetricsWriter）→
// enqueueMetric 内部兜底丢弃。
func (m *Master) wireSpecChangeStoreMetrics() {
	if m.specStore == nil {
		return
	}
	m.specStore.SetConflictObserver(m.emitCASConflict)
	// Round 5 G1：成功 upsert 计 spec_change_upsert_total（cas_conflict 的 SLO 分母）。
	m.specStore.SetUpsertObserver(m.emitSpecChangeUpsert)
}

// emitCASConflict 是 CAS observer callback 的 master-side 实现。
//
// 提取为独立 method（非匿名闭包）的目的：
//  1. 单元可测——test 可以直接调 `m.emitCASConflict("ghost_id")` 验证 metric 入队，
//     不需要真跑 PG 触发 store.UpsertWithCAS。
//  2. 蓝军 mutation 点位清晰——R1 改 Name 常量、R2 改 label key、R3 去掉 enqueue
//     都可独立杀穿。
//
// 设计契约（Sprint 2.3 蓝军 R2）：
//   - metric 名固定为 `specdriven.MetricCASConflictTotal`（常量，不能硬编码字符串）
//   - Labels key `scenario` 与 AllowedCASConflictScenarios 1:1 对应
//   - Value 恒为 1（counter 增量语义）
func (m *Master) emitCASConflict(scenario string) {
	m.enqueueMetric(observability.Metric{
		Name:  specdriven.MetricCASConflictTotal,
		Value: 1,
		Labels: map[string]any{
			"scenario": scenario,
		},
		Ts: time.Now(),
	})
}

// emitSpecChangeUpsert 计成功 commit 的 upsert 次数（Round 5 G1）。无 label。
// 与 emitCASConflict 同一队列，operators 直接 cas_conflict_total / spec_change_upsert_total
// 即得 CAS 冲突率（runbook §1 Stage 1 SLO）。
func (m *Master) emitSpecChangeUpsert() {
	m.enqueueMetric(observability.Metric{
		Name:  specdriven.MetricSpecChangeUpsertTotal,
		Value: 1,
		Ts:    time.Now(),
	})
}

// EmitSpecChangeStoreDisabled 启动时由 bootstrap 在 PG 缺席分支调用一次（Round 5 N3）。
// Public 是因为 bootstrap 在 master 包外。无 label。
func (m *Master) EmitSpecChangeStoreDisabled() {
	m.enqueueMetric(observability.Metric{
		Name:  specdriven.MetricSpecChangeStoreDisabled,
		Value: 1,
		Ts:    time.Now(),
	})
}
