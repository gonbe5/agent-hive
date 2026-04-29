package planner_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chef-guo/agents-hive/internal/specdriven/planner"
)

// TestDecode_Valid：最小合法输入——单 step 两段 task_key。
func TestDecode_Valid(t *testing.T) {
	raw := []byte(`{
		"change_id": "add-user-auth",
		"steps": [
			{"task_key": "1.1", "tool_name": "codegen", "args": {"path": "auth.go"}}
		]
	}`)
	p, err := planner.Decode(raw)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "add-user-auth", p.ChangeID)
	require.Len(t, p.Steps, 1)
	assert.Equal(t, "1.1", p.Steps[0].TaskKey)
	assert.Equal(t, "codegen", p.Steps[0].ToolName)
	assert.JSONEq(t, `{"path":"auth.go"}`, string(p.Steps[0].Args))
}

// TestDecode_ValidMultiStep：多步 + 多位小数段。
func TestDecode_ValidMultiStep(t *testing.T) {
	raw := []byte(`{
		"change_id": "c1",
		"steps": [
			{"task_key": "1.1", "tool_name": "a"},
			{"task_key": "1.10", "tool_name": "b"},
			{"task_key": "2.1.3", "tool_name": "c"}
		]
	}`)
	p, err := planner.Decode(raw)
	require.NoError(t, err)
	assert.Len(t, p.Steps, 3)
}

// TestDecode_NumericTaskKey：FM-4 核心反例——LLM 写数字。
// 期望：强类型 `task_key string` 解码阶段直接报错，归 SchemaInvalid。
func TestDecode_NumericTaskKey(t *testing.T) {
	raw := []byte(`{
		"change_id": "c1",
		"steps": [{"task_key": 3.1, "tool_name": "t"}]
	}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid),
		"numeric task_key 必须归 SchemaInvalid，得到：%v", err)
}

// TestDecode_NumericTaskKeyInteger：int 形式的 task_key（`3`）同样应被拒。
func TestDecode_NumericTaskKeyInteger(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[{"task_key": 3, "tool_name":"t"}]}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_UnknownField：LLM 幻觉字段 `priority` → DisallowUnknownFields 拦下。
func TestDecode_UnknownField(t *testing.T) {
	raw := []byte(`{
		"change_id": "c1",
		"steps": [{"task_key": "1.1", "tool_name": "t", "priority": "high"}]
	}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_UnknownTopField：顶层未知字段。
func TestDecode_UnknownTopField(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[],"owner":"x"}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_SingleSegmentTaskKey：`"1"` 单段——不合规，必须至少两段。
func TestDecode_SingleSegmentTaskKey(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[{"task_key":"1","tool_name":"t"}]}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_TrailingDot：`"1.10."` 尾点——正则必须拒。
func TestDecode_TrailingDot(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[{"task_key":"1.10.","tool_name":"t"}]}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_AlphaInKey：`"1.a"` 字母——拒。
func TestDecode_AlphaInKey(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[{"task_key":"1.a","tool_name":"t"}]}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_EmptyToolName：ToolName 为空——不合规。
func TestDecode_EmptyToolName(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[{"task_key":"1.1","tool_name":""}]}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_EmptySteps：Plan 解码成功但 steps 空 → ErrPlannerEmptyPlan（独立 sentinel）。
func TestDecode_EmptySteps(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[]}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerEmptyPlan))
	assert.False(t, errors.Is(err, planner.ErrPlannerSchemaInvalid),
		"Empty 不该误归 SchemaInvalid——metric 标签要分开")
}

// TestDecode_EmptyInput：完全空 bytes → SchemaInvalid。
func TestDecode_EmptyInput(t *testing.T) {
	_, err := planner.Decode([]byte(""))
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_Whitespace：仅空白——也归 SchemaInvalid。
func TestDecode_Whitespace(t *testing.T) {
	_, err := planner.Decode([]byte("  \n\t  "))
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_MalformedJSON：半个对象。
func TestDecode_MalformedJSON(t *testing.T) {
	_, err := planner.Decode([]byte(`{"change_id":"c","steps":[`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_TrailingData：一个对象 + 尾随 `{}` → dec.More() 必须拦下。
func TestDecode_TrailingData(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[{"task_key":"1.1","tool_name":"t"}]}{}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}

// TestDecode_ArgsPreservesInts：Args 用 json.RawMessage，必须保留整数不坍塌。
// 这是 tasks.md 1.17 的 P1 修缮——防 eval compare 时 `3` 变 `3.0` 漏检。
func TestDecode_ArgsPreservesInts(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[
		{"task_key":"1.1","tool_name":"t","args":{"count": 42, "ratio": 0.5}}
	]}`)
	p, err := planner.Decode(raw)
	require.NoError(t, err)
	// RawMessage 保留原字节——整数不会被坍塌成 "42.0"/"42.00"。
	// 允许空白保留，用 JSONEq 做语义等价 + 显式检查 `42` 是整数形式。
	assert.JSONEq(t, `{"count":42,"ratio":0.5}`, string(p.Steps[0].Args))
	// 硬红线：不允许 `.0` 出现在 count 字段——那才是 FM-4 的味道
	assert.NotContains(t, string(p.Steps[0].Args), `42.0`,
		"RawMessage 必须保留原字节，不得坍塌 int 为 float")
}

// TestDecode_SecondStepBad：多 step 时只有一个违规，整体必须 fail。
func TestDecode_SecondStepBad(t *testing.T) {
	raw := []byte(`{"change_id":"c","steps":[
		{"task_key":"1.1","tool_name":"t"},
		{"task_key":"bad","tool_name":"t"}
	]}`)
	_, err := planner.Decode(raw)
	require.Error(t, err)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
	assert.Contains(t, err.Error(), "step[1]")
}
