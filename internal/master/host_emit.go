package master

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EmitInputRequestOptions 覆盖 EmitInputRequest 的默认参数。
// 0 值表示复用 req.Timeout；若 req.Timeout 也为 0，则使用 Master HITL 配置的 InputTimeout。
type EmitInputRequestOptions struct {
	Timeout time.Duration
}

// ErrInputRequestTimeout 表示 EmitInputRequest 等待 InputResponse 时达到超时。
var ErrInputRequestTimeout = errors.New("input request timed out")

// EmitInputRequest 是 Skill/Tool 作者的业务决策 HITL 一站式入口：
//  1. 若 req.ChoiceType 非空，校验其已在 choice_type_registry 注册（硬校验）；
//  2. req.ID 为空时生成 UUID；req.CreatedAt 为零值时填充当前时间；
//  3. 通过 HITLBroker.BeginEmit 登记 pendingInput（让 SubmitInput 接受响应）；
//  4. 广播 input_request；
//  5. 同时订阅 EventBus 的 input_response 与 pendingInputChans；
//  6. 等待响应 / ctx 取消 / 超时三选一；
//  7. 退出时无论成败都调用 EndEmit 清理 pendingInput，保证零泄漏。
//
// 超时优先级：opts[0].Timeout > req.Timeout > Master HITL InputTimeout > 永不超时。
func (m *Master) EmitInputRequest(ctx context.Context, req InputRequest, opts ...EmitInputRequestOptions) (*InputResponse, error) {
	if req.ChoiceType != "" && !IsRegisteredChoiceType(req.ChoiceType) {
		return nil, fmt.Errorf("%w: %q", ErrUnregisteredChoiceType, req.ChoiceType)
	}

	if req.ID == "" {
		req.ID = uuid.NewString()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}

	timeout := time.Duration(0)
	if len(opts) > 0 && opts[0].Timeout > 0 {
		timeout = opts[0].Timeout
	} else if req.Timeout > 0 {
		timeout = req.Timeout
	} else if m.hitlBroker != nil && m.hitlBroker.config.InputTimeout > 0 {
		timeout = m.hitlBroker.config.InputTimeout
	}

	subCtx, cancelSub := context.WithCancel(ctx)
	defer cancelSub()

	primaryCh := m.hitlBroker.BeginEmit(&req)
	defer m.hitlBroker.EndEmit(req.ID)

	busCh := m.SubscribeInputResponse(subCtx, req.ID)
	m.BroadcastInputRequest(&req)

	var timeoutCh <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timeoutCh = t.C
	}

	select {
	case resp, ok := <-primaryCh:
		if !ok {
			return nil, errors.New("primary channel closed without response")
		}
		return &resp, nil
	case resp, ok := <-busCh:
		if !ok {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("input response channel closed")
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timeoutCh:
		return nil, ErrInputRequestTimeout
	}
}
