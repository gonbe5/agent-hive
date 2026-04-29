package api

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/channel/feishu"
	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/master"
)

// ErrorResponse 是标准错误响应
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// AgentInfo 表示 API 响应中的子 Agent 信息
type AgentInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills,omitempty"`
	Dynamic     bool     `json:"dynamic,omitempty"`
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents := s.master.ListAgents()
	result := make([]AgentInfo, 0, len(agents))
	for _, a := range agents {
		result = append(result, AgentInfo{
			ID:          a.ID,
			Name:        a.Name,
			Description: a.Description,
			Skills:      a.Skills,
			Dynamic:     a.Dynamic,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.skillRegistry == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	skillList := s.skillRegistry.List()
	if skillList == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, skillList)
}

func (s *Server) handleSkillMetrics(w http.ResponseWriter, r *http.Request) {
	m := s.skillRegistry.GetMetrics()
	if m == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, m.Snapshot())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func (s *Server) handleFeishuHealth(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.config == nil || !s.config.Channel.Feishu.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"platform": "feishu",
			"status":   "disabled",
			"degraded": false,
		})
		return
	}

	status := map[string]any{
		"platform":                "feishu",
		"status":                  "healthy",
		"degraded":                false,
		"token_configured":        s.config.Channel.Feishu.AppID != "" && s.config.Channel.Feishu.AppSecret != "",
		"encrypt_key_configured":  s.config.Channel.Feishu.EncryptKey != "",
		"verification_configured": s.config.Channel.Feishu.VerificationToken != "",
	}
	healthClient := s.feishuHealthClient
	if s.channelRouter != nil {
		if plugin, ok := s.channelRouter.GetPlugin(channel.PlatformFeishu); ok {
			if provider, ok := plugin.(interface{ Client() *feishu.Client }); ok && provider.Client() != nil {
				healthClient = provider.Client()
			}
		}
	}
	if healthClient != nil {
		health := healthClient.HealthStatus(r.Context())
		status["status"] = health.Status
		status["degraded"] = health.Degraded
		status["token_configured"] = health.TokenConfigured
		status["verification_configured"] = health.VerificationConfigured
		status["encrypt_key_configured"] = health.EncryptKeyConfigured
		if health.PermissionDeniedCount > 0 {
			status["permission_denied_count"] = health.PermissionDeniedCount
		}
		if health.LastAPIError != "" {
			status["last_api_error"] = health.LastAPIError
		}
		if health.BotOpenID != "" {
			status["bot_open_id"] = health.BotOpenID
		}
	}

	httpStatus := http.StatusOK
	if degraded, _ := status["degraded"].(bool); degraded {
		httpStatus = http.StatusServiceUnavailable
	}
	writeJSON(w, httpStatus, status)
}

// handleListCapabilities 处理 GET /api/v1/capabilities
func (s *Server) handleListCapabilities(w http.ResponseWriter, r *http.Request) {
	router := s.master.GetRouter()
	if router == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	caps := router.ListCapabilities()
	writeJSON(w, http.StatusOK, caps)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// 响应头已发送，无法回写错误码，仅记录日志
		// 使用全局 logger 或忽略（HTTP handler 无法在 WriteHeader 后改状态）
		_ = err
	}
}

// handleSubmitInput 处理 POST /api/v1/tasks/{id}/input
func (s *Server) handleSubmitInput(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "需要任务 ID", Code: errs.CodeBadRequest})
		return
	}

	var resp master.InputResponse
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "无效的请求体", Code: errs.CodeBadRequest})
		return
	}
	resp.TaskID = taskID

	if err := s.master.SubmitInput(resp); err != nil {
		s.logger.Error("提交输入失败", zap.Error(err))
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error(), Code: errs.GetCode(err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "已接受"})
}

// handleSendCommand 处理 POST /api/v1/tasks/{id}/command
func (s *Server) handleSendCommand(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "需要任务 ID", Code: errs.CodeBadRequest})
		return
	}

	var cmd master.UserCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "无效的请求体", Code: errs.CodeBadRequest})
		return
	}
	cmd.TaskID = taskID

	if err := s.master.SendCommand(cmd); err != nil {
		s.logger.Error("发送命令失败", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.GetCode(err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "已接受"})
}

// handleGetPendingInput 处理 GET /api/v1/tasks/{id}/pending-input
func (s *Server) handleGetPendingInput(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "需要任务 ID", Code: errs.CodeBadRequest})
		return
	}

	pending := s.master.PendingInputs(taskID)
	if pending == nil {
		pending = make([]*master.InputRequest, 0)
	}
	writeJSON(w, http.StatusOK, pending)
}

// handleWebSocket 处理 GET /api/v1/ws — 升级到 WebSocket
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	s.wsHandler.HandleConnection(w, r)
}

// ModelInfoResponse 表示 API 响应中的模型信息
type ModelInfoResponse struct {
	Name     string `json:"name"`
	Model    string `json:"model"`
	Provider string `json:"provider,omitempty"`
	IsActive bool   `json:"is_active"`
}

// handleListModels 处理 GET /api/v1/models — 列出可用模型（从数据库加载）
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "存储未初始化", Code: errs.CodeInternal})
		return
	}

	models, err := s.store.ListLLMModels(r.Context())
	if err != nil {
		s.logger.Error("查询模型列表失败", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "查询模型列表失败", Code: errs.CodeStoreReadFailed})
		return
	}

	var active string
	result := make([]ModelInfoResponse, 0, len(models))
	for _, m := range models {
		if !m.Enabled {
			continue
		}
		if m.IsDefault {
			active = m.Name
		}
		result = append(result, ModelInfoResponse{
			Name:     m.Name,
			Model:    m.Model,
			Provider: m.ProviderName,
			IsActive: m.IsDefault,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"models": result,
		"active": active,
	})
}

// handleSwitchModel 处理 PUT /api/v1/model — 切换全局模型（操作数据库）
func (s *Server) handleSwitchModel(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "存储未初始化", Code: errs.CodeInternal})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "无效的请求体", Code: errs.CodeBadRequest})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "需要模型名称", Code: errs.CodeBadRequest})
		return
	}

	ctx := r.Context()

	// 从数据库查找目标模型
	target, err := s.store.GetLLMModel(ctx, req.Name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error: "未找到模型 " + req.Name,
			Code:  errs.CodeNotFound,
		})
		return
	}

	// 取消所有模型的 is_default，再将目标模型设为 default
	allModels, err := s.store.ListLLMModels(ctx)
	if err != nil {
		s.logger.Error("查询模型列表失败", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "查询模型列表失败", Code: errs.CodeStoreReadFailed})
		return
	}
	for _, m := range allModels {
		if m.IsDefault && m.Name != req.Name {
			m.IsDefault = false
			if err := s.store.SaveLLMModel(ctx, m); err != nil {
				s.logger.Error("取消默认模型失败", zap.String("name", m.Name), zap.Error(err))
			}
		}
	}
	target.IsDefault = true
	if err := s.store.SaveLLMModel(ctx, target); err != nil {
		s.logger.Error("设置默认模型失败", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "设置默认模型失败", Code: errs.CodeStoreWriteFailed})
		return
	}

	// 查询提供商的 api_format
	var apiFormat string
	if target.ProviderName != "" {
		if prov, err := s.store.GetLLMProvider(ctx, target.ProviderName); err == nil {
			apiFormat = prov.APIFormat
		}
	}

	// 同步更新 Router 中的 LLM 客户端（运行时配置完全由 DB + Router 管理）
	s.master.SwitchModel(target.Name, target.Model, target.BaseURL, target.ProviderName, apiFormat)

	s.logger.Info("全局模型已切换", zap.String("model", target.Model), zap.String("name", target.Name))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "model": target.Model, "name": target.Name})
}

// invokeToolWhitelist 仅允许通过 /api/v1/tools/invoke 调用的工具（预览类，无副作用）
var invokeToolWhitelist = map[string]bool{
	"wenyan__preview_article": true,
}

// handleInvokeTool 处理 POST /api/v1/tools/invoke
// 允许前端直接调用白名单内的 MCP 工具（如 wenyan 预览），绕过 HITL 权限审批
func (s *Server) handleInvokeTool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ToolName string          `json:"tool_name"`
		Args     json.RawMessage `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "请求体解析失败: " + err.Error(), Code: errs.CodeInputInvalid})
		return
	}
	if !invokeToolWhitelist[req.ToolName] {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "工具不在白名单中: " + req.ToolName, Code: errs.CodePermissionDenied})
		return
	}
	result, err := s.master.InvokeTool(r.Context(), req.ToolName, req.Args)
	if err != nil {
		s.logger.Warn("工具调用失败", zap.String("tool", req.ToolName), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: errs.CodeInternal})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": result})
}
