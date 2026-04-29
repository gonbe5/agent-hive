package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/channel/feishu"
	"github.com/chef-guo/agents-hive/internal/channel/push"
	"github.com/chef-guo/agents-hive/internal/errs"
)

func (s *Server) handleChannelPush(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.config == nil || !s.config.Channel.Feishu.Push.Enabled {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "push 未启用", Code: http.StatusNotFound})
		return
	}
	if s.pushService == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "push service 未初始化", Code: http.StatusServiceUnavailable})
		return
	}
	if auth.IsAuthEnabled(r.Context()) && auth.UserFrom(r.Context()) == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "未授权", Code: http.StatusUnauthorized})
		return
	}
	if auth.IsAuthEnabled(r.Context()) && !canWritePush(r.Context()) {
		writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "缺少 push:write 权限", Code: errs.CodePermissionDenied})
		return
	}
	startedAt := time.Now()

	var req push.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "无效的请求体", Code: http.StatusBadRequest})
		return
	}
	if req.Platform == "" {
		req.Platform = channel.PlatformFeishu
	}
	if err := s.pushService.Push(r.Context(), req); err != nil {
		s.writePushAudit(r, req, "error", err.Error(), startedAt)
		status, code := mapPushError(err)
		writeJSON(w, status, ErrorResponse{Error: err.Error(), Code: code})
		return
	}
	s.writePushAudit(r, req, "ok", "", startedAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"sent":     true,
		"platform": req.Platform,
		"chat_id":  req.ChatID,
	})
}

func canWritePush(ctx context.Context) bool {
	if claims := auth.ClaimsFrom(ctx); claims != nil {
		for _, scope := range claims.Scopes {
			if scope == "push:write" || scope == "admin" {
				return true
			}
		}
	}
	user := auth.UserFrom(ctx)
	return user != nil && user.Role == "admin"
}

func mapPushError(err error) (int, int) {
	if err == nil {
		return http.StatusOK, 0
	}
	if errs.IsCode(err, errs.CodeChannelSendFailed) {
		return http.StatusBadGateway, errs.CodeChannelSendFailed
	}
	if errs.IsCode(err, errs.CodePermissionDenied) {
		return http.StatusForbidden, errs.CodePermissionDenied
	}
	if strings.Contains(strings.ToLower(err.Error()), "rate limited") {
		return http.StatusTooManyRequests, errs.CodeResourceExhausted
	}
	return http.StatusBadRequest, errs.CodeBadRequest
}

func (s *Server) writePushAudit(r *http.Request, req push.Request, outcome string, errMsg string, startedAt time.Time) {
	if s == nil || s.feishuAuditSink == nil {
		return
	}
	_ = s.feishuAuditSink.Write(r.Context(), feishu.AuditRecord{
		TS:         time.Now().UTC(),
		Platform:   "feishu",
		Action:     "push.api",
		Outcome:    outcome,
		DurationMS: time.Since(startedAt).Milliseconds(),
		Actor:      feishu.AuditActorFromContext(r.Context()),
		Target: map[string]any{
			"chat_id":  req.ChatID,
			"open_id":  req.OpenID,
			"msg_type": req.MsgType,
		},
		Error: errMsg,
	})
}
