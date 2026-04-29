package api

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
)

const feishuWebhookIngressDisabledMessage = "feishu webhook ingress disabled"

// FeishuIngressGateHandler 在运行时根据当前 ingress_mode 决定是否放行 webhook。
type FeishuIngressGateHandler struct {
	modeFn    func() config.FeishuIngressMode
	handlerFn func() http.Handler
	logger    *zap.Logger
}

// NewFeishuIngressGateHandler 创建飞书 webhook 永久入口 gate。
func NewFeishuIngressGateHandler(
	modeFn func() config.FeishuIngressMode,
	handlerFn func() http.Handler,
	logger *zap.Logger,
) *FeishuIngressGateHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &FeishuIngressGateHandler{
		modeFn:    modeFn,
		handlerFn: handlerFn,
		logger:    logger,
	}
}

func (h *FeishuIngressGateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := zap.NewNop()
	if h != nil && h.logger != nil {
		logger = h.logger
	}

	if h == nil || h.modeFn == nil {
		logger.Debug("飞书 webhook 入口已禁用",
			zap.String("mode", ""),
			zap.String("path", r.URL.Path))
		http.Error(w, feishuWebhookIngressDisabledMessage, http.StatusNotFound)
		return
	}

	mode := h.modeFn()
	if mode != config.FeishuIngressModeWebhook {
		logger.Debug("飞书 webhook 入口已禁用",
			zap.String("mode", string(mode)),
			zap.String("path", r.URL.Path))
		http.Error(w, feishuWebhookIngressDisabledMessage, http.StatusNotFound)
		return
	}

	if h.handlerFn == nil {
		http.Error(w, "feishu webhook handler unavailable", http.StatusServiceUnavailable)
		return
	}

	handler := h.handlerFn()
	if handler == nil {
		http.Error(w, "feishu webhook handler unavailable", http.StatusServiceUnavailable)
		return
	}

	handler.ServeHTTP(w, r)
}
