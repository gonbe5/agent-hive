package master

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// session_compact_specstate_test.go 覆盖 harden-spec-driven-phase2 task 3.8：
// PreserveSpecStateOnCompact 将 session.specCtx 注入 compaction 产物成 [SPEC-STATE] pin。
//
// 蓝军 mutation 对照（本文件每条 test 必须杀穿至少一条）：
//   R1 去掉 ctx.ChangeID == "" 短路 → TestPreserveSpecStateOnCompact_NilOrEmpty 红
//   R2 去掉 idempotent replace 分支（always prepend）→ TestPreserveSpecStateOnCompact_Idempotent 红
//   R3 formatSpecStatePin 删除 current_task_key 字段 → TestPreserveSpecStateOnCompact_ContentFields 红
//   R4 session 参数改为 value receiver 误吞 nil → TestPreserveSpecStateOnCompact_NilSession 红

func newSessionWithSpecCtx(ctx *specdriven.Context) *SessionState {
	s := &SessionState{ID: "sess-compact"}
	if ctx != nil {
		s.StoreSpecCtx(ctx)
	}
	return s
}

// TestPreserveSpecStateOnCompact_NilSession ——
// 契约：session=nil 必须 no-op，不能 panic、不能修改 messages。
// 测试路径场景：prepareMessagesWithCompression 在单元测试里传 nil session 时的兜底。
func TestPreserveSpecStateOnCompact_NilSession(t *testing.T) {
	msgs := []llm.MessageWithTools{
		{Role: "user", Content: llm.NewTextContent("hi")},
	}
	out := PreserveSpecStateOnCompact(nil, msgs)
	require.Len(t, out, 1, "nil session 不得注入 pin")
	assert.Equal(t, "user", out[0].Role, "原消息必须原样透传")
	assert.Equal(t, "hi", out[0].Content.Text())
}

// TestPreserveSpecStateOnCompact_NilOrEmptySpecCtx ——
// 契约：非 spec 会话（LoadSpecCtx 返回 nil 或 ctx.ChangeID 空）→ no-op。
// 防止"空 ChangeID 污染"：legacy 会话被加入空锚就会在 messages 里出现
// `[SPEC-STATE] change_id=` 这种奇怪字符串，搜不到 change_id 值反而误导调试。
func TestPreserveSpecStateOnCompact_NilOrEmptySpecCtx(t *testing.T) {
	msgs := []llm.MessageWithTools{
		{Role: "user", Content: llm.NewTextContent("legacy request")},
	}

	// case A: session 有但 specCtx 没 Store（legacy 会话）
	sess := &SessionState{ID: "legacy"}
	out := PreserveSpecStateOnCompact(sess, msgs)
	require.Len(t, out, 1, "specCtx=nil 不得注入 pin，防止 legacy 会话污染")

	// case B: session 有 specCtx 但 ChangeID 是空串（异常状态）
	sess2 := newSessionWithSpecCtx(&specdriven.Context{ChangeID: "", CurrentTaskKey: "1.1"})
	out = PreserveSpecStateOnCompact(sess2, msgs)
	require.Len(t, out, 1, "ChangeID 空串也必须 no-op——锚点无实际信息可锚")
}

// TestPreserveSpecStateOnCompact_InjectsPinAtHead ——
// 契约：active spec 会话 → pin 在 messages[0]，内容包含 change_id / current_task_key / revision。
// 这是本函数的核心正向路径。
func TestPreserveSpecStateOnCompact_InjectsPinAtHead(t *testing.T) {
	sess := newSessionWithSpecCtx(&specdriven.Context{
		ChangeID:       "harden-spec-driven-phase2",
		CurrentTaskKey: "3.8",
		Revision:       5,
	})
	msgs := []llm.MessageWithTools{
		{Role: "user", Content: llm.NewTextContent("请继续完成 3.8")},
		{Role: "assistant", Content: llm.NewTextContent("好")},
	}
	out := PreserveSpecStateOnCompact(sess, msgs)
	require.Len(t, out, 3, "pin 必须前插，原消息保留")

	assert.Equal(t, "system", out[0].Role, "pin 必须是 system role")
	pinText := out[0].Content.Text()
	assert.True(t, strings.HasPrefix(pinText, "[SPEC-STATE]"),
		"pin 必须以 [SPEC-STATE] 开头——grep 定位依赖此前缀")
	assert.Contains(t, pinText, "change_id=harden-spec-driven-phase2")
	assert.Contains(t, pinText, "current_task_key=3.8")
	assert.Contains(t, pinText, "revision=5")

	// 原消息位置必须右移
	assert.Equal(t, "user", out[1].Role)
	assert.Equal(t, "请继续完成 3.8", out[1].Content.Text())
	assert.Equal(t, "assistant", out[2].Role)
}

// TestPreserveSpecStateOnCompact_IdempotentReplace ——
// 关键契约：重复调用不得累积 pin，每次必须原位替换。
// 这是 prepareMessagesWithCompression 每轮 LLM 调用都跑一次的幂等保证——
// 100 轮对话后如果每轮都前插，messages 前面会有 100 条 pin，把真实 context 全挤掉。
//
// 蓝军 R2：去掉 replace 分支（always prepend）→ 二次调用后 len=3 断言红 ✓
func TestPreserveSpecStateOnCompact_IdempotentReplace(t *testing.T) {
	sess := newSessionWithSpecCtx(&specdriven.Context{
		ChangeID:       "c1",
		CurrentTaskKey: "1.1",
		Revision:       1,
	})
	msgs := []llm.MessageWithTools{
		{Role: "user", Content: llm.NewTextContent("first")},
	}
	once := PreserveSpecStateOnCompact(sess, msgs)
	require.Len(t, once, 2, "首次调用前插 pin")

	// 第二次调用——pin 已存在 [0]，必须原位替换而非前插
	twice := PreserveSpecStateOnCompact(sess, once)
	require.Len(t, twice, 2,
		"幂等失败：重复调用产生了重复 pin。"+
			"未来每轮 LLM 调用会累积到 N 条 pin，真实消息被挤掉")
	assert.True(t, strings.HasPrefix(twice[0].Content.Text(), "[SPEC-STATE]"))
	assert.Equal(t, "user", twice[1].Role, "用户消息必须保留在 [1]")
}

// TestPreserveSpecStateOnCompact_IdempotentWithStaleVersion ——
// 契约：specCtx.Store 换新 Context（revision 变化）后再调用，pin 必须更新为新版本。
// 实际场景：task 推进 1.1 → 1.2，revision 从 1 → 2，pin 必须反映最新 revision。
//
// 这是幂等的反面——幂等指"调 N 次结果相同"，但 state 变化时必须吸收变化。
func TestPreserveSpecStateOnCompact_IdempotentWithStaleVersion(t *testing.T) {
	sess := newSessionWithSpecCtx(&specdriven.Context{
		ChangeID:       "c1",
		CurrentTaskKey: "1.1",
		Revision:       1,
	})
	msgs := []llm.MessageWithTools{
		{Role: "user", Content: llm.NewTextContent("start")},
	}
	first := PreserveSpecStateOnCompact(sess, msgs)
	assert.Contains(t, first[0].Content.Text(), "current_task_key=1.1")
	assert.Contains(t, first[0].Content.Text(), "revision=1")

	// 推进 task：Store 新 Context
	sess.StoreSpecCtx(&specdriven.Context{
		ChangeID:       "c1",
		CurrentTaskKey: "1.2",
		Revision:       2,
	})
	second := PreserveSpecStateOnCompact(sess, first)
	require.Len(t, second, 2, "仍是替换不是前插")
	assert.Contains(t, second[0].Content.Text(), "current_task_key=1.2",
		"pin 必须吸收新 specCtx 的 task_key")
	assert.Contains(t, second[0].Content.Text(), "revision=2",
		"pin 必须吸收新 specCtx 的 revision")
}

// TestPreserveSpecStateOnCompact_ExistingSessionMemoryDoesNotBlock ——
// 场景：session_memory 在 [0] 插了 [会话记忆] 前缀的 system 消息，
// PreserveSpecStateOnCompact 要能在 [0] 或 [1] 任意位置插 pin 不和前者冲突。
//
// 契约：[会话记忆] 在 [0]，[SPEC-STATE] 前插到 [0]（messages 右移），
// 最终两条 system 前置共存。这里不要求特定顺序——只要求 pin 不被
// mis-identified 成 [会话记忆] 覆盖。
func TestPreserveSpecStateOnCompact_ExistingSessionMemoryDoesNotBlock(t *testing.T) {
	sess := newSessionWithSpecCtx(&specdriven.Context{
		ChangeID:       "c2",
		CurrentTaskKey: "2.1",
		Revision:       3,
	})
	msgs := []llm.MessageWithTools{
		{Role: "system", Content: llm.NewTextContent("[会话记忆]\n用户目标：...")},
		{Role: "user", Content: llm.NewTextContent("q")},
	}
	out := PreserveSpecStateOnCompact(sess, msgs)
	require.Len(t, out, 3, "pin 前插；[会话记忆] 被右移保留")
	assert.True(t, strings.HasPrefix(out[0].Content.Text(), "[SPEC-STATE]"),
		"[SPEC-STATE] 必须在首位——idempotent replace 判据看的是前缀，"+
			"[会话记忆] 前缀不同不会被误当成已有 pin")
	assert.True(t, strings.HasPrefix(out[1].Content.Text(), "[会话记忆]"),
		"[会话记忆] 保留不被覆盖")
}

// TestPreserveSpecStateOnCompact_ContentFieldsComplete ——
// 契约：pin 的 3 个关键字段（change_id / current_task_key / revision）必须都在 Content 里。
// 蓝军 R3：删除 current_task_key 字段 → 本测试 assert.Contains "current_task_key" 红 ✓
func TestPreserveSpecStateOnCompact_ContentFieldsComplete(t *testing.T) {
	sess := newSessionWithSpecCtx(&specdriven.Context{
		ChangeID:       "add-login",
		CurrentTaskKey: "4.2",
		Revision:       7,
	})
	out := PreserveSpecStateOnCompact(sess, nil)
	require.Len(t, out, 1, "msgs=nil 时 pin 仍要插入（msgs=nil 和 empty slice 同语义）")
	text := out[0].Content.Text()

	// 3 个字段缺一不可——蓝军 R3 杀穿点
	assert.Contains(t, text, "change_id=add-login",
		"change_id 字段丢失，LLM 不知道当前工作的 change")
	assert.Contains(t, text, "current_task_key=4.2",
		"current_task_key 字段丢失，LLM 不知道推进到哪一步")
	assert.Contains(t, text, "revision=7",
		"revision 字段丢失，LLM 无法判断是否与 canonical store CAS 冲突")
}
