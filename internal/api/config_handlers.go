package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/chef-guo/agents-hive/internal/errs"
	"go.uber.org/zap"
)

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
