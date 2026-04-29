package skills

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics 追踪 skill 系统的运行时统计信息。线程安全。
type Metrics struct {
	mu sync.RWMutex

	invocationCount  map[string]*atomic.Int64 // skill_name → 调用次数
	invocationErrors map[string]*atomic.Int64 // skill_name → 错误次数
	toolCallCount    map[string]*atomic.Int64 // tool_name → 调用次数
	toolCallErrors   map[string]*atomic.Int64 // tool_name → 错误次数
	toolCallDuration map[string]*atomic.Int64 // tool_name → 累计耗时（纳秒）
	registryDup      map[string]*atomic.Int64 // skill_name → 同版本重复注册次数

	PermissionAsks   atomic.Int64
	PermissionGrants atomic.Int64
	PermissionDenies atomic.Int64
}

// NewMetrics 创建新的 Metrics 实例
func NewMetrics() *Metrics {
	return &Metrics{
		invocationCount:  make(map[string]*atomic.Int64),
		invocationErrors: make(map[string]*atomic.Int64),
		toolCallCount:    make(map[string]*atomic.Int64),
		toolCallErrors:   make(map[string]*atomic.Int64),
		toolCallDuration: make(map[string]*atomic.Int64),
		registryDup:      make(map[string]*atomic.Int64),
	}
}

// RecordDup 记录一次同版本重复注册（用于 Registry.Register 的 skill.registry.dup 信号）
func (m *Metrics) RecordDup(skillName string) {
	m.mu.Lock()
	if m.registryDup == nil {
		m.registryDup = make(map[string]*atomic.Int64)
	}
	if m.registryDup[skillName] == nil {
		m.registryDup[skillName] = &atomic.Int64{}
	}
	cnt := m.registryDup[skillName]
	m.mu.Unlock()
	cnt.Add(1)
}

// RecordInvocation 记录一次 skill 调用
func (m *Metrics) RecordInvocation(skillName string, _ time.Duration, err error) {
	m.mu.Lock()
	if m.invocationCount[skillName] == nil {
		m.invocationCount[skillName] = &atomic.Int64{}
		m.invocationErrors[skillName] = &atomic.Int64{}
	}
	cnt := m.invocationCount[skillName]
	errCnt := m.invocationErrors[skillName]
	m.mu.Unlock()

	cnt.Add(1)
	if err != nil {
		errCnt.Add(1)
	}
}

// RecordToolCall 记录一次工具调用
func (m *Metrics) RecordToolCall(toolName string, duration time.Duration, err error) {
	m.mu.Lock()
	if m.toolCallCount[toolName] == nil {
		m.toolCallCount[toolName] = &atomic.Int64{}
		m.toolCallErrors[toolName] = &atomic.Int64{}
		m.toolCallDuration[toolName] = &atomic.Int64{}
	}
	cnt := m.toolCallCount[toolName]
	errCnt := m.toolCallErrors[toolName]
	dur := m.toolCallDuration[toolName]
	m.mu.Unlock()

	cnt.Add(1)
	dur.Add(duration.Nanoseconds())
	if err != nil {
		errCnt.Add(1)
	}
}

// Snapshot 返回当前指标的快照（JSON 可序列化）
func (m *Metrics) Snapshot() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	invocations := make(map[string]any, len(m.invocationCount))
	for name, cnt := range m.invocationCount {
		errCnt := int64(0)
		if m.invocationErrors[name] != nil {
			errCnt = m.invocationErrors[name].Load()
		}
		invocations[name] = map[string]int64{
			"count":  cnt.Load(),
			"errors": errCnt,
		}
	}

	tools := make(map[string]any, len(m.toolCallCount))
	for name, cnt := range m.toolCallCount {
		errCnt := int64(0)
		durNs := int64(0)
		if m.toolCallErrors[name] != nil {
			errCnt = m.toolCallErrors[name].Load()
		}
		if m.toolCallDuration[name] != nil {
			durNs = m.toolCallDuration[name].Load()
		}
		tools[name] = map[string]int64{
			"count":       cnt.Load(),
			"errors":      errCnt,
			"duration_ms": durNs / int64(time.Millisecond),
		}
	}

	return map[string]any{
		"invocations": invocations,
		"tools":       tools,
		"permissions": map[string]int64{
			"asks":   m.PermissionAsks.Load(),
			"grants": m.PermissionGrants.Load(),
			"denies": m.PermissionDenies.Load(),
		},
	}
}
