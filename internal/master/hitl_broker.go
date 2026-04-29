package master

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// HITLBroker 管理人机交互（Human-In-The-Loop）的所有状态和逻辑
type HITLBroker struct {
	config                config.HITLConfig
	inputCh               chan InputResponse
	commandCh             chan UserCommand
	pendingInput          map[string]*InputRequest
	pendingInputChans     map[string]chan InputResponse   // requestID → 主响应通道
	pendingInputListeners map[string][]chan InputResponse // requestID → 去重订阅者（额外监听）
	pendingFingerprints   map[string]string               // fingerprint → requestID（去重索引）
	inputMu               sync.Mutex
	inputCounter          uint64
	eventBus              *EventBus // 用于广播事件
	stopCh                chan struct{}
	logger                *zap.Logger
}

// NewHITLBroker 创建新的 HITL 协调器
func NewHITLBroker(cfg config.HITLConfig, eventBus *EventBus, stopCh chan struct{}, logger *zap.Logger) *HITLBroker {
	return &HITLBroker{
		config:                cfg,
		inputCh:               make(chan InputResponse, 16),
		commandCh:             make(chan UserCommand, 16),
		pendingInput:          make(map[string]*InputRequest),
		pendingInputChans:     make(map[string]chan InputResponse),
		pendingInputListeners: make(map[string][]chan InputResponse),
		pendingFingerprints:   make(map[string]string),
		eventBus:              eventBus,
		stopCh:                stopCh,
		logger:                logger,
	}
}

// BeginEmit 将 EmitInputRequest 发起的 req 注册到 pendingInput，使得后续
// SubmitInput 能够接受对应 reqID 的响应。返回主响应通道，EmitInputRequest 可
// 与 SubscribeInputResponse 的 EventBus 通道双路 select。
//
// 必须配对调用 EndEmit 清理，无论成功 / 超时 / ctx 取消。
func (hb *HITLBroker) BeginEmit(req *InputRequest) chan InputResponse {
	ch := make(chan InputResponse, 1)
	hb.inputMu.Lock()
	hb.pendingInput[req.ID] = req
	hb.pendingInputChans[req.ID] = ch
	hb.inputMu.Unlock()
	return ch
}

// EndEmit 清理 EmitInputRequest 留下的 pendingInput 条目。幂等；若 SubmitInput
// 已经消费并清理了 map，这里也不会 panic。
func (hb *HITLBroker) EndEmit(reqID string) {
	hb.inputMu.Lock()
	defer hb.inputMu.Unlock()
	delete(hb.pendingInput, reqID)
	delete(hb.pendingInputChans, reqID)
	delete(hb.pendingInputListeners, reqID)
}

// permFingerprint 计算权限请求的去重指纹（toolName + input 的 sha256 前16位）
func permFingerprint(toolName string, input json.RawMessage) string {
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write(input)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// RequestInput 创建 InputRequest，在 pendingInput 中注册它，并发出事件
// sessionID 用于前端过滤，确保审批请求只推送给对应会话的用户
func (hb *HITLBroker) RequestInput(taskID, stepID string, reqType InputRequestType, prompt string, options []string, sessionID ...string) *InputRequest {
	id := fmt.Sprintf("input-%d", atomic.AddUint64(&hb.inputCounter, 1))
	sid := taskID // 默认用 taskID 作为 sessionID
	if len(sessionID) > 0 && sessionID[0] != "" {
		sid = sessionID[0]
	}
	req := &InputRequest{
		ID:        id,
		TaskID:    taskID,
		StepID:    stepID,
		SessionID: sid,
		Type:      reqType,
		Prompt:    prompt,
		Options:   options,
		Timeout:   hb.config.InputTimeout,
		CreatedAt: time.Now(),
	}

	hb.inputMu.Lock()
	hb.pendingInput[id] = req
	hb.pendingInputChans[id] = make(chan InputResponse, 1) // 创建专用通道
	hb.inputMu.Unlock()

	hb.logger.Info("请求输入",
		zap.String("request_id", id),
		zap.String("task_id", taskID),
		zap.String("type", string(reqType)),
		zap.String("prompt", prompt),
	)

	// 广播到所有 WebSocket 订阅者
	hb.eventBus.BroadcastInputRequest(req)

	return req
}

// WaitForInput 阻塞直到收到匹配的响应、超时或取消
func (hb *HITLBroker) WaitForInput(ctx context.Context, taskID string, req *InputRequest) (*InputResponse, error) {
	timeout := req.Timeout
	if timeout == 0 {
		timeout = hb.config.InputTimeout
	}
	if timeout == 0 {
		timeout = config.DefaultHITLInputTimeout
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	// 获取专用响应通道
	hb.inputMu.Lock()
	respCh, ok := hb.pendingInputChans[req.ID]
	hb.inputMu.Unlock()

	if !ok {
		return nil, errs.New(errs.CodeInternal, "no response channel for request "+req.ID)
	}

	// 确保退出时清理
	defer func() {
		hb.inputMu.Lock()
		delete(hb.pendingInput, req.ID)
		delete(hb.pendingInputChans, req.ID)
		hb.inputMu.Unlock()
	}()

	for {
		select {
		case resp := <-respCh:
			// 收到此请求的响应
			return &resp, nil

		case cmd := <-hb.commandCh:
			if cmd.TaskID != "" && cmd.TaskID != taskID {
				select {
				case hb.commandCh <- cmd:
				default:
				}
				continue
			}
			if cmd.Type == CmdCancel {
				return nil, errs.New(errs.CodeTaskCanceled, "task canceled by user")
			}

		case <-timer.C:
			return nil, errs.New(errs.CodeInputTimeout, "waiting for human input timed out")

		case <-ctx.Done():
			return nil, ctx.Err()

		case <-hb.stopCh:
			return nil, errs.New(errs.CodeCanceled, "master stopped")
		}
	}
}

// SubmitInput 是提交用户响应的外部入口点
// 从 CLI、WebSocket 或 REST handlers 调用
func (hb *HITLBroker) SubmitInput(resp InputResponse) error {
	// 验证 action
	switch resp.Action {
	case "", "approve", "reject", "modify", "proceed", "skip", "cancel":
		// 合法
	default:
		return errs.New(errs.CodeInputInvalid, "invalid action: "+resp.Action)
	}

	// 将 map 查找、验证、channel 发送全部置于同一把锁内，
	// 防止在 Unlock 与 channel send 之间 WaitForInput 清理条目并关闭 channel 引发 panic
	hb.inputMu.Lock()
	req, existsReq := hb.pendingInput[resp.RequestID]
	respCh, existsCh := hb.pendingInputChans[resp.RequestID]

	if !existsReq || !existsCh {
		hb.inputMu.Unlock()
		return errs.New(errs.CodeInputNotPending, "no pending input request: "+resp.RequestID)
	}

	// 验证 task ID 是否匹配
	if resp.TaskID != "" && resp.TaskID != req.TaskID {
		hb.inputMu.Unlock()
		return errs.New(errs.CodeInputInvalid, "request ID does not belong to specified task")
	}

	// 在持有锁的情况下非阻塞发送：channel 容量为 1，WaitForInput 尚未消费时 select 直接入队；
	// 若 channel 已满（理论上不应发生），立即返回错误而非阻塞，避免死锁。
	// 发送成功后立即从 map 中删除该请求，确保同一请求只有一次 SubmitInput 能成功——
	// 即使 WaitForInput 已消费 channel 导致 channel 重新变空，后续 Submit 也会因 map 中无条目而失败。
	select {
	case respCh <- resp:
		// fan-out：通知通过 fingerprint 去重挂载的额外监听者
		for _, lch := range hb.pendingInputListeners[resp.RequestID] {
			select {
			case lch <- resp:
			default: // 监听者 channel 满或已处理，跳过
			}
		}
		// 清理指纹索引
		if req.Fingerprint != "" && hb.pendingFingerprints[req.Fingerprint] == resp.RequestID {
			delete(hb.pendingFingerprints, req.Fingerprint)
		}
		delete(hb.pendingInput, resp.RequestID)
		delete(hb.pendingInputChans, resp.RequestID)
		delete(hb.pendingInputListeners, resp.RequestID)
		hb.inputMu.Unlock()
		// 广播 input_response 以支持 EmitInputRequest 订阅路径。
		// 必须在 Unlock 之后，EventBus.Broadcast 本身异步非阻塞。
		if hb.eventBus != nil {
			hb.eventBus.BroadcastInputResponse(&resp)
		}
		return nil
	default:
		hb.inputMu.Unlock()
		return errs.New(errs.CodeInternal, "response channel is full, request may have already been handled")
	}
}

// SendCommand 是发送用户控制命令的外部入口点
func (hb *HITLBroker) SendCommand(cmd UserCommand) error {
	switch cmd.Type {
	case CmdPause, CmdResume, CmdCancel:
		// 合法
	default:
		return errs.New(errs.CodeInputInvalid, "invalid command type: "+string(cmd.Type))
	}

	select {
	case hb.commandCh <- cmd:
		return nil
	default:
		return errs.New(errs.CodeInternal, "command channel full")
	}
}

// PendingInputs 返回给定任务 ID 的待处理输入请求
func (hb *HITLBroker) PendingInputs(taskID string) []*InputRequest {
	hb.inputMu.Lock()
	defer hb.inputMu.Unlock()

	var result []*InputRequest
	for _, req := range hb.pendingInput {
		if taskID == "" || req.TaskID == taskID {
			result = append(result, req)
		}
	}
	return result
}

// Enabled 返回 HITL 是否启用
func (hb *HITLBroker) Enabled() bool {
	return hb.config.Enabled
}

// SetEnabled 动态开关 HITL（热更新）
func (hb *HITLBroker) SetEnabled(enabled bool) {
	hb.config.Enabled = enabled
}

// NextInputID 生成下一个输入 ID（用于权限请求等内部场景）
func (hb *HITLBroker) NextInputID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddUint64(&hb.inputCounter, 1))
}

// RegisterPendingInput 注册一个待处理输入请求和对应的响应通道。
// 若 req.Fingerprint 非空且已有相同指纹的 pending 请求，则不重复注册，
// 将 respCh 作为额外监听者挂到已有请求上，返回 false（调用方无需再广播）。
// 返回 true 表示首次注册，调用方应广播该请求。
func (hb *HITLBroker) RegisterPendingInput(req *InputRequest, respCh chan InputResponse) bool {
	hb.inputMu.Lock()
	defer hb.inputMu.Unlock()

	// 去重：fingerprint 已存在 → 复用已有请求
	if req.Fingerprint != "" {
		if existingID, ok := hb.pendingFingerprints[req.Fingerprint]; ok {
			hb.pendingInputListeners[existingID] = append(hb.pendingInputListeners[existingID], respCh)
			hb.logger.Info("HITL 去重：复用已有 pending 请求",
				zap.String("fingerprint", req.Fingerprint),
				zap.String("existing_id", existingID),
			)
			return false
		}
		hb.pendingFingerprints[req.Fingerprint] = req.ID
	}

	hb.pendingInput[req.ID] = req
	hb.pendingInputChans[req.ID] = respCh
	return true
}

// UnregisterPendingInput 取消注册待处理输入请求，同时清理指纹索引和监听者
func (hb *HITLBroker) UnregisterPendingInput(reqID string) {
	hb.inputMu.Lock()
	if req, ok := hb.pendingInput[reqID]; ok && req.Fingerprint != "" {
		if hb.pendingFingerprints[req.Fingerprint] == reqID {
			delete(hb.pendingFingerprints, req.Fingerprint)
		}
	}
	delete(hb.pendingInput, reqID)
	delete(hb.pendingInputChans, reqID)
	delete(hb.pendingInputListeners, reqID)
	hb.inputMu.Unlock()
}

// CreatePermissionPromptFn 创建权限请求提示函数
func (hb *HITLBroker) CreatePermissionPromptFn() func(ctx context.Context, toolName, description string, input json.RawMessage) (bool, bool, error) {
	return func(ctx context.Context, toolName, description string, input json.RawMessage) (bool, bool, error) {
		inputReq := &InputRequest{
			ID:          hb.NextInputID("perm"),
			TaskID:      "",
			Type:        InputPermission,
			Prompt:      description,
			Options:     []string{"approve", "deny"},
			ToolName:    toolName,
			Data:        input,
			Timeout:     60 * time.Minute,
			Fingerprint: permFingerprint(toolName, input),
			CreatedAt:   time.Now(),
		}

		// 使用 per-request channel，避免通道饥饿
		respCh := make(chan InputResponse, 1)
		isNew := hb.RegisterPendingInput(inputReq, respCh)

		// 仅首次注册时广播，去重请求不重复广播审批卡片
		if isNew {
			hb.eventBus.BroadcastInputRequest(inputReq)
		}

		hb.logger.Info("请求权限",
			zap.String("request_id", inputReq.ID),
			zap.String("tool", toolName))

		timeout := time.NewTimer(60 * time.Minute)
		defer timeout.Stop()

		// 确保退出时清理
		defer hb.UnregisterPendingInput(inputReq.ID)

		select {
		case resp := <-respCh:
			return resp.Action == "approve", resp.Remember, nil

		case <-timeout.C:
			return false, false, errs.New(errs.CodeInputTimeout, "permission request timed out")

		case <-ctx.Done():
			return false, false, ctx.Err()

		case <-hb.stopCh:
			return false, false, errs.New(errs.CodeCanceled, "master stopped")
		}
	}
}
