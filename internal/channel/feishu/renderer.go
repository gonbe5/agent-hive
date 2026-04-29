package feishu

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/master"
)

// 编译期断言 *Plugin 实现 channel.EventRenderer。
var _ channel.EventRenderer = (*Plugin)(nil)

// rendererDefaultThrottle 是 message partial 的合并节流窗口。
// 飞书 Patch 频控 5/s = 最小间隔 200ms，本值贴在频控上限：
//   - 300ms（旧值）实测下用户感觉"卡"——每次 PATCH 一次性跳出 ~19 个中文字，中间停顿 300ms。
//   - 200ms 把刷新率提升到 5 次/秒，每次跳出 ~13 字，视觉更连续。
//   - patchWithRetry 兼容 429——真踩到频控会重试一次。
//
// Section 7 将把它移到 cfg.Renderer.ThrottleMs，运营可按 provider 速率自调。
const rendererDefaultThrottle = 200 * time.Millisecond

// rendererFinalFlushTimeout 是 ctx.Done 后最后一次 PATCH 的超时上限。
// 超过 3s 视为重大违规（契约见 channel.EventRenderer 文档注释）。
const rendererFinalFlushTimeout = 3 * time.Second

// rendererPatchRetryBackoff 是 PATCH 失败后的一次性重试间隔。
const rendererPatchRetryBackoff = 1 * time.Second

// rendererDefaultThinkingHeartbeat 是空闲心跳阈值：
// 事件流静默超过这个时间仍未终态 → 自动 PATCH 一次 "💭 思考中…" 指示，
// 防止用户把"LLM 推理中 / 工具执行中的静默"误判为对话结束。
// 2s：实测 gpt-5.2 TTFB 约 4.4s，5s 心跳跑不过 first chunk → 用户在前 4 秒看不到任何反馈。
// 调到 2s 让"思考中"在 first chunk 之前就先出现，覆盖 provider 推理窗口。
// 短了抖动也由 throttle (300ms) 兜底——心跳触发到 PATCH 前会先 drain 一次 eventCh。
// Section 7 将把它挪到 cfg.Renderer.ThinkingHeartbeatMs。
const rendererDefaultThinkingHeartbeat = 2 * time.Second

// cardTransport 是 renderer 依赖的最小 Client 子集。
// 抽接口使 renderer 的行为可被注入 mock 单测验证，不必起真实飞书 API。
// *Client 天然实现该接口。
type cardTransport interface {
	ReplyCard(ctx context.Context, replyToID, cardJSON string) (string, error)
	SendCard(ctx context.Context, chatID, cardJSON string) (string, error)
	PatchCard(ctx context.Context, messageID, cardJSON string) error
	AddReaction(ctx context.Context, messageID, emojiType string) error
}

// rendererCardState 是 renderer 对单次会话的内部状态，
// 每次事件更新后 BuildCardJSON → PatchCard（或首轮 ReplyCard 创建）。
//
// 时间水位拆两根的原因（X-3 BLOCKING 修复）：
//   - lastPatchAt：任何 PATCH 成功都刷新，心跳 scheduleHeartbeat 靠它判"距上次任何动静多久"。
//   - lastContentPatchAt：只有内容型 PATCH（message / agent_progress）刷新；heartbeat / tool_call / HITL
//     不动它。这样心跳 PATCH 不会"偷走"后续 partial 的 300ms 节流窗口，partial 仍按自己的节拍落地。
type rendererCardState struct {
	messageID          string              // 首次 create 后回填，后续 PATCH 用
	lastPatchAt        time.Time           // 任何 PATCH 成功后刷新；仅心跳空闲判定用
	lastContentPatchAt time.Time           // 仅 message/agent_progress 这类"内容密集型"PATCH 后刷新；节流 gate 用
	content            string              // 累积的 message 正文
	lastFullContent    string              // 最近一次成功落地的完整正文，RendererError.LastContent 用
	toolCalls          map[string]ToolLine // key=tool_call_id，保证同 id in-place 更新
	toolCallOrder      []string            // 保证渲染顺序稳定
	hitlButtons        []HITLButton
	hitlRenderedIDs    map[string]bool // request_id → 已渲染，防止重复追加 prompt
	progress           string          // agent_progress 底部 "{turn}/{maxTurns}"
	status             CardStatus
	terminalError      bool // error 事件后忽略后续
	awaitingInput      bool // HITL 已挂出、尚未收到后续 agent 事件期间抑制心跳——"思考中"文案在等人回复时是误导
}

// feishuRenderer 是一次 RenderEventStream 调用的执行器：
// 持有 transport/logger 的引用 + ackEmoji 解析结果，所有 handler 通过方法接收 state。
type feishuRenderer struct {
	transport         cardTransport
	logger            *zap.Logger
	ackEmoji          string
	throttle          time.Duration
	finalTimeout      time.Duration
	retryBackoff      time.Duration
	thinkingHeartbeat time.Duration
	showProgress      bool
	// 时间钩子便于单测注入虚拟时间；默认 time.Now / time.After。
	now   func() time.Time
	after func(d time.Duration) <-chan time.Time
}

// newFeishuRenderer 构造默认配置的 renderer。
// 测试可直接替换 transport/now/after 字段。
func newFeishuRenderer(transport cardTransport, logger *zap.Logger, ackEmoji string) *feishuRenderer {
	return &feishuRenderer{
		transport:         transport,
		logger:            logger,
		ackEmoji:          ackEmoji,
		throttle:          rendererDefaultThrottle,
		finalTimeout:      rendererFinalFlushTimeout,
		retryBackoff:      rendererPatchRetryBackoff,
		thinkingHeartbeat: rendererDefaultThinkingHeartbeat,
		showProgress:      false,
		now:               time.Now,
		after:             time.After,
	}
}

// RenderEventStream 订阅 EventBus → 渲染飞书卡片。
// 契约（详见 channel.EventRenderer）：
//   - 消费端按 scope.SessionID filter；
//   - ctx cancel → 3s 内最后一次 flush → return ctx.Err()；
//   - 任何 PATCH 失败 → 1s 重试一次 → *RendererError{LastContent} 上报 Router 兜底。
func (p *Plugin) RenderEventStream(ctx context.Context, scope channel.SessionScope, eventCh <-chan master.BroadcastMessage) error {
	// ackEmoji 已由 config.FeishuConfig.Normalize 校验；plugin.New 路径亦兜底一次 Normalize。
	r := newFeishuRenderer(p.client, p.logger, p.cfg.AckEmoji)
	if p.cfg.Renderer.ThrottleMs > 0 {
		r.throttle = time.Duration(p.cfg.Renderer.ThrottleMs) * time.Millisecond
	}
	r.showProgress = p.cfg.Renderer.ShowAgentProgress
	return r.run(ctx, scope, eventCh)
}

// run 是 renderer 的主循环。单独拆出便于测试直接驱动，不必经过 *Plugin。
//
// 心跳机制（X-3）：
// 当卡片已创建、仍在 Generating 态、没触发 terminalError、且距上次 PATCH 超过 thinkingHeartbeat，
// 自动 PATCH 一次 "💭 思考中…" 指示，让用户知道 LLM 仍在推理 / 工具仍在执行，不是结束。
// heartbeat PATCH 完成后 status 回退到 Generating，允许周期性重复触发——
// 每个 LLM 推理/长工具调用的静默区间里，每 5s 给一次"活着"信号。
func (r *feishuRenderer) run(ctx context.Context, scope channel.SessionScope, eventCh <-chan master.BroadcastMessage) error {
	state := &rendererCardState{
		toolCalls: make(map[string]ToolLine),
		status:    CardStatusGenerating,
	}

	for {
		heartbeatCh := r.scheduleHeartbeat(state)

		select {
		case <-ctx.Done():
			// ctx cancel：用独立超时 ctx 做最后一次 flush，无论成败都 return ctx.Err()。
			r.finalFlush(scope, state)
			return ctx.Err()

		case ev, ok := <-eventCh:
			if !ok {
				// Router Unsubscribe 后 channel 关闭 → 正常收敛。
				return nil
			}
			if ev.SessionID != "" && ev.SessionID != scope.SessionID {
				continue
			}
			if state.terminalError {
				continue
			}
			if err := r.dispatchEvent(ctx, scope, state, ev); err != nil {
				return channel.WrapRendererErr(err, state.lastFullContent)
			}

		case <-heartbeatCh:
			// Go select 在多分支同时 ready 时是随机的：即使 eventCh 已经有事件待读，
			// 也可能被 select 选中 heartbeat 分支，导致心跳"插队"在真事件前面。
			// 这里先非阻塞 drain 一次 eventCh——有事件就走正常 dispatch，没事件才进心跳。
			select {
			case ev, ok := <-eventCh:
				if !ok {
					return nil
				}
				if ev.SessionID != "" && ev.SessionID != scope.SessionID {
					continue
				}
				if state.terminalError {
					continue
				}
				if err := r.dispatchEvent(ctx, scope, state, ev); err != nil {
					return channel.WrapRendererErr(err, state.lastFullContent)
				}
				continue
			default:
			}
			// 进入前再校验一次：scheduleHeartbeat 触发时状态可能早已不满足心跳条件
			// （例如 terminalError / awaitingInput 刚被置起）。用 shouldHeartbeat 同一条件再 gate。
			if !r.shouldHeartbeat(state) {
				continue
			}
			state.status = CardStatusThinking
			if err := r.flushCard(ctx, scope, state); err != nil {
				// ctx 结束时 PATCH 会返回 context.Canceled/DeadlineExceeded，这不是渲染故障，
				// 不应 wrap 成 *RendererError 去走 Router 文本兜底——下一轮 ctx.Done 分支接管 finalFlush。
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					state.status = CardStatusGenerating
					continue
				}
				return channel.WrapRendererErr(err, state.lastFullContent)
			}
			// 回退到 Generating，使下一轮 select 仍可能触发下一次心跳。
			// 任何真实事件进来时，handler 会根据语义重置 status（partial=Generating / final=Done / error=Error）。
			state.status = CardStatusGenerating
		}
	}
}

// shouldHeartbeat 判定当前 state 是否有资格触发空闲心跳。
//
// 必须同时满足：卡片已创建 / 心跳未禁用 / 仍在 Generating / 未终态错误 / 未等待人类输入。
// awaitingInput=true 时卡片在等人点按钮或回文本，此时显示"思考中"会误导用户，
// 让他们以为还是 agent 在忙——其实球在用户脚下。
// 注意：时间判定不在这里做，由 scheduleHeartbeat 基于 lastPatchAt 算剩余等待时长。
func (r *feishuRenderer) shouldHeartbeat(state *rendererCardState) bool {
	if r.thinkingHeartbeat <= 0 {
		return false
	}
	if state.messageID == "" {
		return false
	}
	if state.terminalError {
		return false
	}
	if state.awaitingInput {
		return false
	}
	return state.status == CardStatusGenerating
}

// scheduleHeartbeat 根据 state.lastPatchAt 计算到下次心跳的等待时长，
// 返回一个触发通道；不满足心跳条件时返回 nil，nil channel 在 select 里永远阻塞，等同禁用。
func (r *feishuRenderer) scheduleHeartbeat(state *rendererCardState) <-chan time.Time {
	if !r.shouldHeartbeat(state) {
		return nil
	}
	idle := r.now().Sub(state.lastPatchAt)
	wait := r.thinkingHeartbeat - idle
	if wait <= 0 {
		wait = time.Nanosecond // 立刻触发，但不传 0 避免某些 timer 实现 panic
	}
	return r.after(wait)
}

// dispatchEvent 按事件类型分派到对应 handler。
func (r *feishuRenderer) dispatchEvent(ctx context.Context, scope channel.SessionScope, state *rendererCardState, ev master.BroadcastMessage) error {
	switch ev.Type {
	case master.EventTypeInputReceived:
		r.handleInputReceived(scope, ev)
		return nil
	case master.EventTypeMessage:
		return r.handleMessage(ctx, scope, state, ev)
	case master.EventTypeToolCall:
		return r.handleToolCall(ctx, scope, state, ev)
	case master.EventTypeInputRequest:
		return r.handleInputRequest(ctx, scope, state, ev)
	case master.EventTypeError:
		return r.handleError(ctx, scope, state, ev)
	case master.EventTypeAgentProgress:
		return r.handleAgentProgress(ctx, scope, state, ev)
	default:
		// 其他事件（agent_start / skill_exec / task_group ...）暂不渲染到卡片。
		return nil
	}
}

// handleInputReceived 飞书 ack 表情：fire-and-forget，不阻塞事件循环。
// 跳过条件：ChannelMessageID 为空 / r.ackEmoji 为空 / r.ackEmoji == "none"（用户显式禁用 ack）。
// 语义注：config.FeishuConfig.Normalize 刻意保留 "none" 字面量保证幂等——
// 归一化不会把 "none" 变成 ""，所以此处必须同时识别两种形态。
// 注：goroutine 使用独立 5s 超时 ctx，与 run 的 ctx 解耦——ack 一次性、进程停机时 OS 会回收。
func (r *feishuRenderer) handleInputReceived(scope channel.SessionScope, ev master.BroadcastMessage) {
	payload, ok := ev.Payload.(master.InputReceivedEvent)
	if !ok {
		return
	}
	if payload.ChannelMessageID == "" || r.ackEmoji == "" || r.ackEmoji == "none" {
		return
	}
	sessionID := scope.SessionID
	go func(msgID, emoji string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.transport.AddReaction(ctx, msgID, emoji); err != nil {
			r.logger.Warn("ack 表情失败",
				zap.String("session_id", sessionID),
				zap.String("message_id", msgID),
				zap.String("emoji", emoji),
				zap.Error(err))
		}
	}(payload.ChannelMessageID, r.ackEmoji)
}

// handleMessage 累积 message 正文；partial=true 走节流，partial=false 强制 flush 并切终态。
//
// 节流水位用 lastContentPatchAt（不用 lastPatchAt），避免心跳 / HITL / tool_call 的 PATCH
// 把后续 partial 的 300ms 窗口"挤掉"——partial 流要按自己的节拍落地。
//
// 防御性 guard：payload 携带 tool_calls 即视为中间状态，强制 partial=true。
// 上游 react_processor 已修对 partial 公式，但此处仍兜一层——任何后续 emitter 忘了
// 设 partial=true，渲染端不会再把卡片提前切到"✅ 完成"，工具还在跑/HITL 还在等用户。
func (r *feishuRenderer) handleMessage(ctx context.Context, scope channel.SessionScope, state *rendererCardState, ev master.BroadcastMessage) error {
	text, partial, role, hasToolCalls := extractMessagePayload(ev.Payload)
	// 非 assistant 角色单独处理：
	//   - role=user：飞书侧用户已看见自己的消息，**不写正文** 但应**立刻建一张占位卡**
	//     标题"🤖 生成中…"，给用户即时反馈。否则 provider TTFB 4.4s 内界面全空白。
	//   - role=tool：tool result 由 handleToolCall 单独呈现为工具行，message 通道 skip。
	if role != "" && role != "assistant" {
		if role == "user" && state.messageID == "" {
			state.status = CardStatusGenerating
			if err := r.flushCard(ctx, scope, state); err != nil {
				return err
			}
			state.lastContentPatchAt = state.lastPatchAt
		}
		return nil
	}
	if text != "" {
		state.content = text
	}
	// 有新 message 来了 → agent 正在继续推理，解除 HITL suppress。
	// 若消息本身是 HITL prompt，handleInputRequest 会再次 set awaitingInput=true。
	state.awaitingInput = false
	// 携带 tool_calls = 中间态，强制按 partial 处理（即便上游错标 partial=false）。
	if hasToolCalls {
		partial = true
	}
	if partial {
		if state.messageID != "" && r.now().Sub(state.lastContentPatchAt) < r.throttle {
			return nil
		}
		state.status = CardStatusGenerating
		if err := r.flushCard(ctx, scope, state); err != nil {
			return err
		}
		state.lastContentPatchAt = state.lastPatchAt
		return nil
	}
	// partial=false：最终一块，强制 flush + 终态。
	state.status = CardStatusDone
	if err := r.flushCard(ctx, scope, state); err != nil {
		return err
	}
	state.lastContentPatchAt = state.lastPatchAt
	return nil
}

// handleToolCall 按 tool_call_id 维护工具行；start/success/error 任何变化立即 PATCH。
func (r *feishuRenderer) handleToolCall(ctx context.Context, scope channel.SessionScope, state *rendererCardState, ev master.BroadcastMessage) error {
	tc := extractToolCallPayload(ev.Payload)
	if tc.id == "" {
		return nil
	}
	// tool_call 进来说明 agent 继续推理，不再等人：解除 HITL suppress。
	state.awaitingInput = false
	existing, seen := state.toolCalls[tc.id]
	if !seen {
		state.toolCallOrder = append(state.toolCallOrder, tc.id)
	}
	line := ToolLine{
		ToolName: fallbackStr(tc.name, existing.ToolName),
		Status:   fallbackStr(tc.status, existing.Status),
	}
	if tc.durationMs > 0 {
		line.Duration = time.Duration(tc.durationMs) * time.Millisecond
	} else {
		line.Duration = existing.Duration
	}
	if tc.summary != "" {
		line.Summary = tc.summary
	} else {
		line.Summary = existing.Summary
	}
	state.toolCalls[tc.id] = line
	return r.flushCard(ctx, scope, state)
}

// handleInputRequest 根据 InputRequestType 分派渲染：
//   - approval / confirmation / permission：批准/拒绝按钮（callback 回路尚未接入，见 X-2 follow-up）
//   - clarification：自由文本问答——把 Prompt 写入正文，提示用户在群里直接回复
//   - choice：选项列表——把 Prompt + Options 写入正文，回复序号或原文
//
// X-2 修复：老版本对所有 Type 硬写"批准/拒绝"按钮，同时丢失 req.Prompt 和 req.Options，
// 导致 question/clarification 场景下用户在飞书看不到被问的问题（日志里能看到但卡片不显示）。
func (r *feishuRenderer) handleInputRequest(ctx context.Context, scope channel.SessionScope, state *rendererCardState, ev master.BroadcastMessage) error {
	req, ok := ev.Payload.(*master.InputRequest)
	if !ok {
		return nil
	}
	// 幂等：同一 request_id 只渲染一次（无论走哪条分支）
	if state.hitlRenderedIDs == nil {
		state.hitlRenderedIDs = make(map[string]bool)
	}
	if state.hitlRenderedIDs[req.ID] {
		return nil
	}
	state.hitlRenderedIDs[req.ID] = true

	switch req.Type {
	case master.InputApproval, master.InputConfirmation, master.InputPermission:
		// 批准型：追加按钮（callback 回路为 X-2 follow-up）。
		state.hitlButtons = append(state.hitlButtons,
			HITLButton{Label: "✅ 批准", Action: "approve", RequestID: req.ID},
			HITLButton{Label: "❌ 拒绝", Action: "reject", RequestID: req.ID},
		)
		if req.Prompt != "" {
			state.content = appendHITLPrompt(state.content, req.Prompt, nil)
		}
	case master.InputClarification, master.InputChoice:
		// 文本问答 / 选项选择：把问题和选项写进正文，用户在群里直接回复文本。
		state.content = appendHITLPrompt(state.content, req.Prompt, req.Options)
	default:
		// 未知类型：至少把 Prompt 显示出来，不要静默吞掉。
		if req.Prompt != "" {
			state.content = appendHITLPrompt(state.content, req.Prompt, req.Options)
		}
	}
	// HITL 挂出后，卡片进入"等人"态：抑制心跳。
	// 后续任何 message/tool_call/progress 事件会重置回 false，让 agent 推理期的心跳恢复。
	state.awaitingInput = true
	return r.flushCard(ctx, scope, state)
}

// appendHITLPrompt 把 HITL 问题文本 + 选项格式化追加到卡片正文。
func appendHITLPrompt(existing, prompt string, options []string) string {
	if prompt == "" && len(options) == 0 {
		return existing
	}
	var sb strings.Builder
	sb.WriteString(existing)
	if existing != "" {
		sb.WriteString("\n\n")
	}
	if prompt != "" {
		sb.WriteString("❓ ")
		sb.WriteString(prompt)
	}
	if len(options) > 0 {
		if prompt != "" {
			sb.WriteString("\n\n")
		}
		sb.WriteString("**可选项：**")
		for i, opt := range options {
			sb.WriteString(fmt.Sprintf("\n%d. %s", i+1, opt))
		}
		sb.WriteString("\n\n_请直接在群里回复序号或原文。_")
	} else if prompt != "" {
		sb.WriteString("\n\n_请直接在群里回复。_")
	}
	return sb.String()
}

// handleError 切红框 + 错误文案，立即 PATCH；置 terminalError=true 后续事件忽略。
func (r *feishuRenderer) handleError(ctx context.Context, scope channel.SessionScope, state *rendererCardState, ev master.BroadcastMessage) error {
	msg := extractErrorText(ev.Payload)
	if msg != "" {
		if state.content != "" {
			state.content += "\n\n"
		}
		state.content += "⚠️ " + msg
	}
	state.status = CardStatusError
	state.terminalError = true
	return r.flushCard(ctx, scope, state)
}

// handleAgentProgress 底部进度条；与 message 共享节流（lastContentPatchAt）。
// Section 7 将接入 cfg.Renderer.ShowAgentProgress 开关——当前默认不展示。
func (r *feishuRenderer) handleAgentProgress(ctx context.Context, scope channel.SessionScope, state *rendererCardState, ev master.BroadcastMessage) error {
	if !r.showProgress {
		return nil
	}
	turn, maxTurns := extractProgressPayload(ev.Payload)
	if maxTurns <= 0 {
		return nil
	}
	// progress 推进 → agent 还在推理，解除 HITL suppress。
	state.awaitingInput = false
	state.progress = fmt.Sprintf("%d/%d", turn, maxTurns)
	if state.messageID != "" && r.now().Sub(state.lastContentPatchAt) < r.throttle {
		return nil
	}
	if err := r.flushCard(ctx, scope, state); err != nil {
		return err
	}
	state.lastContentPatchAt = state.lastPatchAt
	return nil
}

// flushCard 是所有 handler 统一的 PATCH 出口：
//   - messageID 为空 → 首轮 create（ReplyCard → SendCard fallback）
//   - 否则 PatchCard
//   - 失败 → 1s 重试一次
//   - 仍失败 → return err（上层包 RendererError）
func (r *feishuRenderer) flushCard(ctx context.Context, scope channel.SessionScope, state *rendererCardState) error {
	cardJSON := r.buildCardFromState(state)
	// 先记录"意图发送"的完整内容：即使 PATCH 失败，Router 兜底 Send 也会用这份最新内容，
	// 而不是回退到上一次成功 PATCH 的旧内容——契约见 D8 与 Router.processViaRenderer。
	// tool-only / HITL-only 场景下 content 为空：用 buildTextFallback 兜底，保证 Router 有非空文案可 Send。
	if snapshot := r.buildTextFallback(state); snapshot != "" {
		state.lastFullContent = snapshot
	}
	if state.messageID == "" {
		id, err := r.createCard(ctx, scope, cardJSON)
		if err != nil {
			return err
		}
		state.messageID = id
	} else {
		if err := r.patchWithRetry(ctx, state.messageID, cardJSON); err != nil {
			return err
		}
	}
	state.lastPatchAt = r.now()
	return nil
}

func (r *feishuRenderer) createCard(ctx context.Context, scope channel.SessionScope, cardJSON string) (string, error) {
	if scope.ReplyToID != "" {
		id, err := r.transport.ReplyCard(ctx, scope.ReplyToID, cardJSON)
		if err == nil {
			return id, nil
		}
		// ctx 已取消：不再 fallback（避免无谓的二次 API 调用 + 被限流放大）。
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		r.logger.Warn("ReplyCard 失败，fallback SendCard",
			zap.String("reply_to", scope.ReplyToID),
			zap.String("chat_id", scope.ChatID),
			zap.Error(err))
	}
	if scope.ChatID == "" {
		return "", fmt.Errorf("feishu renderer: 无 chat_id 可 fallback，首轮卡片创建失败")
	}
	return r.transport.SendCard(ctx, scope.ChatID, cardJSON)
}

func (r *feishuRenderer) patchWithRetry(ctx context.Context, messageID, cardJSON string) error {
	err := r.transport.PatchCard(ctx, messageID, cardJSON)
	if err == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.after(r.retryBackoff):
	}
	if retryErr := r.transport.PatchCard(ctx, messageID, cardJSON); retryErr != nil {
		return fmt.Errorf("PATCH 重试仍失败（首次 %v；重试 %w）", err, retryErr)
	}
	return nil
}

// finalFlush 是 ctx.Done 后的末班车：用独立 3s 超时 ctx 做最后一次 PATCH。
// 不返回 error，失败只打日志——契约要求 3s 内 return。
//
// Thinking 态保险：如果 ctx 正好在 heartbeat flush 过程中被 cancel，
// state.status 可能停在 Thinking 而未回退到 Generating。这里强制矫正——
// ctx 结束通常意味着业务已经到尾声或被主动取消，让卡片停在"思考中"给用户最差的观感。
// 优先级：terminalError（Error 态） > 已经终态（Done/Error 保留） > Thinking→Generating。
func (r *feishuRenderer) finalFlush(scope channel.SessionScope, state *rendererCardState) {
	if state.messageID == "" && state.content == "" && len(state.toolCalls) == 0 {
		return
	}
	if state.status == CardStatusThinking {
		state.status = CardStatusGenerating
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.finalTimeout)
	defer cancel()
	if err := r.flushCard(ctx, scope, state); err != nil {
		r.logger.Warn("renderer ctx.Done 末次 flush 失败",
			zap.String("session_id", scope.SessionID),
			zap.Error(err))
	}
}

// buildTextFallback 把 state 压扁成纯文本，作为 RendererError.LastContent 的兜底内容。
// 优先顺序：message content > 工具执行概要 > HITL 提示 > ""。
// 目的：PATCH 失败后 Router 用 SendMessage(text) 兜底时仍能给出非空有效内容（MF-4）。
func (r *feishuRenderer) buildTextFallback(state *rendererCardState) string {
	if state.content != "" {
		return state.content
	}
	if len(state.toolCallOrder) > 0 {
		parts := make([]string, 0, len(state.toolCallOrder))
		for _, id := range state.toolCallOrder {
			tl, ok := state.toolCalls[id]
			if !ok {
				continue
			}
			seg := tl.ToolName
			if tl.Status != "" {
				seg = "[" + tl.Status + "] " + seg
			}
			if tl.Summary != "" {
				seg += " — " + tl.Summary
			}
			parts = append(parts, seg)
		}
		if len(parts) > 0 {
			return "（卡片渲染失败，工具执行概要：" + strings.Join(parts, "；") + "）"
		}
	}
	if len(state.hitlButtons) > 0 {
		return "（卡片渲染失败，有 HITL 审批请求待处理，请在飞书原卡片上操作）"
	}
	return ""
}

// buildCardFromState 把 state 快照喂给 BuildCardJSON——解耦 state 与 JSON 层。
func (r *feishuRenderer) buildCardFromState(state *rendererCardState) string {
	toolLines := make([]ToolLine, 0, len(state.toolCallOrder))
	for _, id := range state.toolCallOrder {
		if tl, ok := state.toolCalls[id]; ok {
			toolLines = append(toolLines, tl)
		}
	}
	body := state.content
	if state.progress != "" {
		if body != "" {
			body += "\n\n"
		}
		body += "`" + state.progress + "`"
	}
	return BuildCardJSON(CardState{
		Body:        body,
		ToolLines:   toolLines,
		HITLButtons: state.hitlButtons,
		Status:      state.status,
	})
}

// --- payload 解析 helpers ---

type toolCallPayload struct {
	id         string
	name       string
	status     string
	durationMs int64
	summary    string
}

func extractToolCallPayload(payload any) toolCallPayload {
	var out toolCallPayload
	switch v := payload.(type) {
	case master.ToolCallEvent:
		out.id = v.ToolCallID
		out.name = v.ToolName
		out.status = v.Status
		out.durationMs = v.Duration
		if v.Error != "" {
			out.summary = v.Error
		}
	case *master.ToolCallEvent:
		if v != nil {
			out.id = v.ToolCallID
			out.name = v.ToolName
			out.status = v.Status
			out.durationMs = v.Duration
			if v.Error != "" {
				out.summary = v.Error
			}
		}
	case map[string]any:
		out.id, _ = v["tool_call_id"].(string)
		out.name, _ = v["tool_name"].(string)
		out.status, _ = v["status"].(string)
		if d, ok := v["duration"].(int64); ok {
			out.durationMs = d
		} else if d, ok := v["duration"].(float64); ok {
			out.durationMs = int64(d)
		}
		if s, ok := v["summary"].(string); ok {
			out.summary = s
		} else if s, ok := v["error"].(string); ok {
			out.summary = s
		}
	}
	if out.id == "" && out.name != "" {
		out.id = "tool:" + out.name
	}
	return out
}

// extractMessagePayload 解出消息文本、partial 标志、role、是否携带 tool_calls。
// role 用于过滤非 assistant 的回显（user echo / tool result），
// 避免 partial 缺省=false 的用户消息被误判为终态 → 卡片先显示"✅ 完成"再退回"生成中"。
// hasToolCalls 用于防御上游 partial 公式失误（比如把 finish_reason=tool_calls 当终态广播）：
// 只要 payload 携带 tool_calls，本轮就不是真正的终止，handleMessage 据此拒绝切到 Done。
func extractMessagePayload(payload any) (text string, partial bool, role string, hasToolCalls bool) {
	switch v := payload.(type) {
	case string:
		return v, false, "", false
	case map[string]any:
		if c, ok := v["content"].(string); ok {
			text = c
		} else if c, ok := v["text"].(string); ok {
			text = c
		} else if c, ok := v["message"].(string); ok {
			text = c
		}
		if p, ok := v["partial"].(bool); ok {
			partial = p
		}
		if r, ok := v["role"].(string); ok {
			role = r
		}
		if tcs, ok := v["tool_calls"].([]any); ok && len(tcs) > 0 {
			hasToolCalls = true
		} else if tcs, ok := v["tool_calls"].([]map[string]any); ok && len(tcs) > 0 {
			hasToolCalls = true
		}
	}
	return text, partial, role, hasToolCalls
}

func extractErrorText(payload any) string {
	switch v := payload.(type) {
	case string:
		return v
	case error:
		return v.Error()
	case map[string]any:
		if m, ok := v["message"].(string); ok && m != "" {
			return m
		}
		if e, ok := v["error"].(string); ok {
			return e
		}
	}
	return ""
}

func extractProgressPayload(payload any) (turn, maxTurns int) {
	switch v := payload.(type) {
	case master.AgentProgressEvent:
		return v.Turn, v.MaxTurns
	case *master.AgentProgressEvent:
		if v != nil {
			return v.Turn, v.MaxTurns
		}
	case map[string]any:
		turn = toInt(v["turn"])
		maxTurns = toInt(v["max_turns"])
	}
	return
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

func fallbackStr(primary, secondary string) string {
	if primary != "" {
		return primary
	}
	return secondary
}
