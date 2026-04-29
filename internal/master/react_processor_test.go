package master

import (
	"testing"

	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/stretchr/testify/assert"
)

func TestLoopDetector_Ok(t *testing.T) {
	d := newLoopDetector(20)
	// 不同工具组合，每次都返回 ok
	result := d.check([]llm.ToolCall{{Name: "read_file"}})
	assert.Equal(t, "ok", result)
	result = d.check([]llm.ToolCall{{Name: "edit"}})
	assert.Equal(t, "ok", result)
	result = d.check([]llm.ToolCall{{Name: "bash"}})
	assert.Equal(t, "ok", result)
}

func TestLoopDetector_WarnAt3(t *testing.T) {
	d := newLoopDetector(20)
	calls := []llm.ToolCall{{Name: "read_file"}, {Name: "grep"}}

	assert.Equal(t, "ok", d.check(calls))   // 第 1 次
	assert.Equal(t, "ok", d.check(calls))   // 第 2 次
	assert.Equal(t, "warn", d.check(calls)) // 第 3 次 → warn
}

func TestLoopDetector_HardStopAt5(t *testing.T) {
	d := newLoopDetector(20)
	calls := []llm.ToolCall{{Name: "bash"}}

	assert.Equal(t, "ok", d.check(calls))        // 1
	assert.Equal(t, "ok", d.check(calls))        // 2
	assert.Equal(t, "warn", d.check(calls))      // 3 → warn
	assert.Equal(t, "warn", d.check(calls))      // 4 → warn
	assert.Equal(t, "hard_stop", d.check(calls)) // 5 → hard_stop
}

func TestLoopDetector_HashSortedNames(t *testing.T) {
	// 不同顺序的相同工具名应产生相同 hash
	d := newLoopDetector(20)
	callsAB := []llm.ToolCall{{Name: "read_file"}, {Name: "bash"}}
	callsBA := []llm.ToolCall{{Name: "bash"}, {Name: "read_file"}}

	assert.Equal(t, "ok", d.check(callsAB)) // 1
	assert.Equal(t, "ok", d.check(callsBA)) // 2（同 hash）
	assert.Equal(t, "warn", d.check(callsAB)) // 3 → warn（同 hash 第 3 次）
}

func TestLoopDetector_WindowSliding(t *testing.T) {
	// 窗口大小为 5，超出窗口的历史不计入
	d := newLoopDetector(5)
	target := []llm.ToolCall{{Name: "edit"}}
	other := []llm.ToolCall{{Name: "bash"}}

	// 填入 2 次 target
	assert.Equal(t, "ok", d.check(target)) // 1
	assert.Equal(t, "ok", d.check(target)) // 2

	// 填入 3 次 other，把 target 挤出窗口
	d.check(other)
	d.check(other)
	d.check(other)

	// 此时窗口内 target 只剩 0 次（被挤出），重新计数
	assert.Equal(t, "ok", d.check(target)) // 窗口内第 1 次
	assert.Equal(t, "ok", d.check(target)) // 窗口内第 2 次
}

func TestComputeToolCallHash_Deterministic(t *testing.T) {
	calls := []llm.ToolCall{{Name: "read_file"}, {Name: "bash"}}
	h1 := computeToolCallHash(calls)
	h2 := computeToolCallHash(calls)
	assert.Equal(t, h1, h2)
}

func TestComputeToolCallHash_OrderIndependent(t *testing.T) {
	callsAB := []llm.ToolCall{{Name: "a"}, {Name: "b"}}
	callsBA := []llm.ToolCall{{Name: "b"}, {Name: "a"}}
	assert.Equal(t, computeToolCallHash(callsAB), computeToolCallHash(callsBA))
}

func TestComputeToolCallHash_DifferentNames(t *testing.T) {
	calls1 := []llm.ToolCall{{Name: "read_file"}}
	calls2 := []llm.ToolCall{{Name: "bash"}}
	assert.NotEqual(t, computeToolCallHash(calls1), computeToolCallHash(calls2))
}
