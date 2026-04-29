package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// WeChatProtocolStatus 微信协议状态
type WeChatProtocolStatus struct {
	Enabled  bool                   `json:"enabled"`
	Status   string                 `json:"status"` // "not_started"|"connected"|"error"
	LoggedIn bool                   `json:"logged_in"`
	Config   map[string]interface{} `json:"config"`
}

// WeChatConfigResponse 微信配置响应
type WeChatConfigResponse struct {
	Protocols map[string]WeChatProtocolStatus `json:"protocols"`
}

// handleGetWeChatConfig 获取微信配置和状态
func (s *Server) handleGetWeChatConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed", Code: errs.CodeBadRequest})
		return
	}

	s.configMu.RLock()
	wechatCfg := s.config.Channel.WeChat

	resp := WeChatConfigResponse{
		Protocols: make(map[string]WeChatProtocolStatus),
	}

	// 1. Wechaty
	resp.Protocols["wechaty"] = s.buildProtocolStatus(
		"wechaty",
		wechatCfg.Wechaty.Enabled,
		channel.PlatformWeChatWechaty,
		map[string]interface{}{
			"endpoint": wechatCfg.Wechaty.Endpoint,
			"token":    wechatCfg.Wechaty.Token,
		},
	)

	// 2. WeChatPadPro (推荐)
	resp.Protocols["wechatpadpro"] = s.buildProtocolStatus(
		"wechatpadpro",
		wechatCfg.WeChatPadPro.Enabled,
		channel.PlatformWeChatPadPro,
		map[string]interface{}{
			"base_url": wechatCfg.WeChatPadPro.BaseURL,
			"app_id":   wechatCfg.WeChatPadPro.AppID,
			"token":    wechatCfg.WeChatPadPro.Token,
			"timeout":  wechatCfg.WeChatPadPro.Timeout,
		},
	)
	s.configMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// buildProtocolStatus 构建协议状态
func (s *Server) buildProtocolStatus(
	name string,
	enabled bool,
	platform channel.Platform,
	cfg map[string]interface{},
) WeChatProtocolStatus {
	status := WeChatProtocolStatus{
		Enabled:  enabled,
		Status:   "not_started",
		LoggedIn: false,
		Config:   cfg,
	}

	// 如果 channelRouter 为 nil 或协议未启用，直接返回
	if s.channelRouter == nil || !enabled {
		return status
	}

	// 查询插件状态
	plugin, ok := s.channelRouter.GetPlugin(platform)
	if !ok {
		return status
	}

	// 插件已注册，状态为 connected
	status.Status = "connected"

	// 尝试获取登录状态
	type LoggedInChecker interface {
		IsLoggedIn() bool
	}
	if checker, ok := plugin.(LoggedInChecker); ok {
		status.LoggedIn = checker.IsLoggedIn()
	}

	return status
}

// UpdateWeChatProtocolRequest 更新微信协议配置请求
type UpdateWeChatProtocolRequest struct {
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config"`
}

// handleUpdateWeChatProtocol 更新指定协议的配置
func (s *Server) handleUpdateWeChatProtocol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed", Code: errs.CodeBadRequest})
		return
	}

	// 从路径中提取协议名称
	protocol := r.PathValue("protocol")
	if protocol != "wechaty" && protocol != "wechatpadpro" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "不支持的协议: " + protocol, Code: errs.CodeBadRequest})
		return
	}

	var req UpdateWeChatProtocolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "无效的请求体: " + err.Error(), Code: errs.CodeBadRequest})
		return
	}

	// 更新内存配置
	s.configMu.Lock()
	switch protocol {
	case "wechaty":
		s.config.Channel.WeChat.Wechaty.Enabled = req.Enabled
		if endpoint, ok := req.Config["endpoint"].(string); ok {
			s.config.Channel.WeChat.Wechaty.Endpoint = endpoint
		}
		if token, ok := req.Config["token"].(string); ok {
			s.config.Channel.WeChat.Wechaty.Token = token
		}

	case "wechatpadpro":
		s.config.Channel.WeChat.WeChatPadPro.Enabled = req.Enabled
		if baseURL, ok := req.Config["base_url"].(string); ok {
			s.config.Channel.WeChat.WeChatPadPro.BaseURL = baseURL
		}
		if appID, ok := req.Config["app_id"].(string); ok {
			s.config.Channel.WeChat.WeChatPadPro.AppID = appID
		}
		if token, ok := req.Config["token"].(string); ok {
			s.config.Channel.WeChat.WeChatPadPro.Token = token
		}
		if timeout, ok := req.Config["timeout"].(float64); ok {
			s.config.Channel.WeChat.WeChatPadPro.Timeout = int(timeout)
		}
	}
	s.configMu.Unlock()

	s.logger.Info("微信协议配置已更新",
		zap.String("protocol", protocol),
		zap.Bool("enabled", req.Enabled))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "配置已更新（内存），调用 /api/v1/config/save 保存到文件，或重启服务生效",
	})
}

// handleSaveConfig 保存配置到文件
func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed", Code: errs.CodeBadRequest})
		return
	}

	// 确定配置文件路径
	configPath := s.configPath
	if configPath == "" {
		// 使用默认路径
		homeDir, err := os.UserHomeDir()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "无法获取用户主目录: " + err.Error(), Code: errs.CodeInternal})
			return
		}
		configPath = filepath.Join(homeDir, ".claw", "config.json")

		// 确保目录存在
		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "创建配置目录失败: " + err.Error(), Code: errs.CodeInternal})
			return
		}
	}

	// 保存到文件
	s.configMu.RLock()
	err := s.config.SaveToFile(configPath)
	s.configMu.RUnlock()
	if err != nil {
		s.logger.Error("保存配置失败", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "保存配置失败: " + err.Error(), Code: errs.CodeInternal})
		return
	}

	s.logger.Info("配置已保存到文件", zap.String("path", configPath))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "配置已保存，重启服务后生效",
		"path":    configPath,
	})
}

// handleReloadWeChatProtocol 重载指定协议（停止旧实例并启动新实例）
func (s *Server) handleReloadWeChatProtocol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed", Code: errs.CodeBadRequest})
		return
	}

	// 从路径中提取协议名称
	protocol := r.PathValue("protocol")
	if protocol != "wechaty" && protocol != "wechatpadpro" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "不支持的协议: " + protocol, Code: errs.CodeBadRequest})
		return
	}

	// 检查是否有 reload 回调函数
	if s.reloadProtocolFunc == nil {
		// 降级到旧行为：只停止
		if s.channelRouter == nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Channel 路由器未初始化", Code: errs.CodeInternal})
			return
		}

		platform := channel.Platform("wechat-" + protocol)
		if err := s.channelRouter.UnregisterPlugin(platform); err != nil {
			s.logger.Error("注销插件失败",
				zap.String("protocol", protocol),
				zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "注销插件失败: " + err.Error(), Code: errs.CodeInternal})
			return
		}

		s.logger.Info("协议已停止",
			zap.String("protocol", protocol))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "协议已停止，请重启服务以应用新配置",
			"status":  "stopped",
		})
		return
	}

	// 调用热加载回调
	if err := s.reloadProtocolFunc(protocol); err != nil {
		s.logger.Error("热加载协议失败",
			zap.String("protocol", protocol),
			zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "热加载失败: " + err.Error(), Code: errs.CodeInternal})
		return
	}

	s.logger.Info("协议已热加载",
		zap.String("protocol", protocol))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "协议已热加载成功",
		"status":  "reloaded",
	})
}

// handleReloadFeishu 触发飞书通道热重载,不重启服务。
//
// Phase 5 缺口 13 修复:之前 ReloadFromConfig 接口实装了但没 HTTP 触发,运维改
// config 必须重启。现在通过这个 API 走完整的 unregister + rebuild +
// ReloadFromConfig 链路(逻辑由 bootstrap/helpers.go 的 buildReloadProtocolFunc
// 闭包负责)。
//
// 飞书没有多协议子路径,POST 即触发。
//
// reloadProtocolFunc 未注入时降级:返回 503 + "请重启服务"。
func (s *Server) handleReloadFeishu(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed", Code: errs.CodeBadRequest})
		return
	}
	if s.reloadProtocolFunc == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{
			Error: "热重载未启用,请重启服务以应用新配置",
			Code:  errs.CodeInternal,
		})
		return
	}
	if err := s.reloadProtocolFunc("feishu"); err != nil {
		s.logger.Error("飞书热加载失败", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error: "飞书热加载失败: " + err.Error(),
			Code:  errs.CodeInternal,
		})
		return
	}
	s.logger.Info("飞书通道已热加载")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "飞书通道已热加载",
		"status":  "reloaded",
	})
}
