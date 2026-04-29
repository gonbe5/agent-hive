# 2026-04-29 工具调用流式 chunk 诊断误报

## Symptom

日志出现 `[stream-diag] LLM 调用未收到任何 chunk（假流式或 provider 不 SSE）`，但同一轮 LLM 最终返回了 `tool_calls`。用户质疑：工具调用 chunk 是否不能流式返回。

## Root Cause

底层 LLM 流式实现已经支持工具调用 chunk：

- `internal/llm/stream_completions.go` 在 Chat Completions 流中持续累积 `pendingCalls` 并通过 `StreamChunk.ToolCalls` 回调上层；非终态时参数可能还是部分 JSON。
- `internal/llm/stream_responses.go` 在 Responses API 的 function call arguments done 事件上回调 `StreamChunk.ToolCalls`。

真正的问题在 master 层：`internal/master/react_processor.go` 的回调先过滤掉 `ContentSoFar == "" && ReasoningContent == ""` 的 chunk，因此“只有工具调用、没有文本”的 chunk 被当成空 chunk 丢掉，`chunkCount` 不增加，最终误报未收到任何 chunk。

## Fix

- 新增 `internal/master/stream_diagnostics.go`，用 `classifyStreamChunk` 区分文本 chunk、工具调用 chunk 和有效非终态流式事件。
- `internal/master/react_processor.go` 统计 `streamEventCount/textChunkCount/toolChunkCount/finalToolCallCount`，tool_calls-only 的非终态 chunk 会计入真实流式事件。
- 仍然只广播文本/推理 partial；tool_calls-only chunk 只用于诊断，不执行工具，也不广播 partial args，避免半截 JSON 被当作稳定参数。
- 更新 `internal/llm/client.go` 和 `internal/llm/stream_completions.go` 注释，明确非 Done 的 tool calls 可能是部分参数，只能诊断/预览。

## Regression Test

新增 `internal/master/stream_diagnostics_test.go`：

- tool_calls-only 非终态 chunk 必须计为流式事件。
- Done 终态 tool_calls 不计为 provider 的非终态流式事件。
- 文本 chunk 正常计数。
- 空 chunk 不计数。

## Verification

- `env GOCACHE=/tmp/go-build go test ./internal/master -run TestClassifyStreamChunk -count=1`
- `env GOCACHE=/tmp/go-build go test ./internal/master -run 'Test(ClassifyStreamChunk|DetectToolChoice|RefsForToolChoice|IsSuccessfulIMReferenceRead|EvaluateRequiredGuard|ShouldSuppressStreamPartial|EmitAssistantMessage)' -count=1`
- `env GOCACHE=/tmp/go-build go test ./internal/llm -count=1`
