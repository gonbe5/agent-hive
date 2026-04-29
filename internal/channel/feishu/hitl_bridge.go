package feishu

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/imctx"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	"go.uber.org/zap"
)

// InputSubmitter 是 master.HITLBroker 暴露给 IM 桥接层的最小契约。
// 抽接口便于单测，且明示桥接层只能调 SubmitInput 这一个面，禁止 reach 其它 broker 内部状态。
type InputSubmitter interface {
	SubmitInput(resp master.InputResponse) error
}

// 飞书 HITL 桥接错误（包级常量便于上游做 errors.Is 路由）。
var (
	ErrHITLNoSubmitter = errors.New("feishu: hitl bridge has nil submitter")
	ErrHITLBadAction   = errors.New("feishu: hitl bridge received unsupported action")
)

// 与 master/hitl_broker.go SubmitInput 白名单严格一致；
// 凡未列入此 set 的 action 在桥接层就拒，绝不让其进入 master。
var allowedHITLActions = map[string]struct{}{
	"":        {}, // 允许空，broker 内部当作"提交 value"
	"approve": {},
	"reject":  {},
	"modify":  {},
	"proceed": {},
	"skip":    {},
	"cancel":  {},
}

// FeishuHITLBridge 把飞书 card.action.trigger 回调翻译成 master.InputResponse。
//
// 调用契约：
//   - HandleCardActionTrigger 始终返回 nil error，确保 SDK dispatcher 不会回 5xx 让飞书重试；
//     业务失败由 toast 与 logger 表达。Phase 0 P0-#7 wrapper 永返 nil 不变量在此预先生效。
//   - Submitter 在构造时注入，nil 视为编程错误，HandleCardActionTrigger 立即记录并返回友好 toast。
type FeishuHITLBridge struct {
	submitter     InputSubmitter
	logger        *zap.Logger
	now           func() time.Time
	metricsWriter observability.MetricsWriter
}

// NewFeishuHITLBridge 注入 submitter（通常是 *master.HITLBroker）与 logger。
// now 留作可注入便于单测；nil 时退化为 time.Now。
func NewFeishuHITLBridge(submitter InputSubmitter, logger *zap.Logger, now func() time.Time) *FeishuHITLBridge {
	if logger == nil {
		logger = zap.NewNop()
	}
	if now == nil {
		now = time.Now
	}
	return &FeishuHITLBridge{
		submitter: submitter,
		logger:    logger,
		now:       now,
	}
}

func (b *FeishuHITLBridge) WithMetricsWriter(w observability.MetricsWriter) *FeishuHITLBridge {
	if b == nil {
		return nil
	}
	b.metricsWriter = w
	return b
}

// HandleCardActionTrigger 是 dispatcher.OnP2CardActionTrigger 的目标 handler。
// 即使 submit 失败也不返回 error——见上方契约说明。
func (b *FeishuHITLBridge) HandleCardActionTrigger(_ context.Context, ev *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
	if b.submitter == nil {
		b.logger.Error("hitl bridge nil submitter — programming error")
		b.emitCallbackStatusMetric("", "no_submitter")
		return errToast("HITL 服务未就绪"), nil
	}

	action, err := decodeCardAction(ev, b.now())
	if err != nil {
		// 解码失败有几类：missing request_id 说明用户点了非 HITL 卡片，DEBUG 级即可；
		// 其它 nil event/value 是上游异常，WARN 记录便于排查。
		if errors.Is(err, ErrCardActionMissingReqID) {
			b.logger.Debug("card action without request_id — likely non-HITL card",
				zap.Error(err))
			return nil, nil
		}
		b.logger.Warn("decode card action failed", zap.Error(err))
		b.emitCallbackStatusMetric("", "decode_failed")
		return errToast("卡片回调格式异常"), nil
	}

	if _, ok := allowedHITLActions[action.Action]; !ok {
		b.logger.Warn("hitl action not in whitelist",
			zap.String("action", action.Action),
			zap.String("request_id", action.RequestID),
			zap.String("safe_operator_id", action.SafeOperatorID))
		b.emitCallbackStatusMetric(action.TenantKey, "bad_action")
		return errToast("不支持的操作"), nil
	}

	resp := master.InputResponse{
		RequestID: action.RequestID,
		TaskID:    action.TaskID,
		Value:     action.Value,
		Action:    action.Action,
		// Remember 字段当前只服务于 permission 类请求，由 value 中显式字段决定，
		// 飞书 HITL 卡片 Phase 0 不渲染 Remember 控件，留默认 false。
	}

	if err := b.submitter.SubmitInput(resp); err != nil {
		b.logger.Warn("submit input to master failed",
			zap.String("request_id", resp.RequestID),
			zap.Error(err))
		b.emitCallbackStatusMetric(action.TenantKey, "submit_failed")
		return errToast(fmt.Sprintf("提交失败: %s", err.Error())), nil
	}

	b.logger.Info("hitl response submitted",
		zap.String("request_id", resp.RequestID),
		zap.String("action", resp.Action),
		zap.String("safe_operator_id", action.SafeOperatorID))
	b.emitCallbackStatusMetric(action.TenantKey, "submitted")
	return successToast("已收到"), nil
}

// imctx.CardAction 直接给桥接做单测调用入口，跳过 SDK 解码层。
// 调用方仅为单测，不导出。
func (b *FeishuHITLBridge) submitFromCardAction(action imctx.CardAction) error {
	if b.submitter == nil {
		return ErrHITLNoSubmitter
	}
	if _, ok := allowedHITLActions[action.Action]; !ok {
		return fmt.Errorf("%w: %s", ErrHITLBadAction, action.Action)
	}
	return b.submitter.SubmitInput(master.InputResponse{
		RequestID: action.RequestID,
		TaskID:    action.TaskID,
		Value:     action.Value,
		Action:    action.Action,
	})
}

func successToast(msg string) *callback.CardActionTriggerResponse {
	return &callback.CardActionTriggerResponse{
		Toast: &callback.Toast{
			Type:    "success",
			Content: msg,
		},
	}
}

func errToast(msg string) *callback.CardActionTriggerResponse {
	return &callback.CardActionTriggerResponse{
		Toast: &callback.Toast{
			Type:    "error",
			Content: msg,
		},
	}
}

func (b *FeishuHITLBridge) emitCallbackStatusMetric(tenantKey, status string) {
	if b == nil || b.metricsWriter == nil {
		return
	}
	labels := map[string]any{
		"status": status,
	}
	if tenantKey != "" {
		labels["tenant_key_hash"] = channel.TenantKeyHashLabel(tenantKey)
	}
	_ = b.metricsWriter.Record(context.Background(), observability.Metric{
		Name:   MetricHITLCallbackStatus,
		Value:  1,
		Labels: labels,
		Ts:     time.Now(),
	})
}
