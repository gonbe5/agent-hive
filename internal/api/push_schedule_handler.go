package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/channel/push"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/store"
)

type pushScheduleRequest struct {
	Name        string `json:"name"`
	Platform    string `json:"platform"`
	Prompt      string `json:"prompt"`
	IntervalSec int    `json:"interval_sec"`
	Enabled     bool   `json:"enabled"`
}

func (s *Server) handleCreatePushSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.canManagePushSchedules(w, r) {
		return
	}
	scheduleStore, ok := s.store.(pushScheduleStore)
	if !ok || scheduleStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "schedule store 未初始化", Code: http.StatusServiceUnavailable})
		return
	}

	var req pushScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "无效的请求体", Code: http.StatusBadRequest})
		return
	}
	rec, err := s.buildScheduledPushRecord(req, auth.UserFrom(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error(), Code: http.StatusBadRequest})
		return
	}
	if err := scheduleStore.SaveScheduledPush(r.Context(), rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: http.StatusInternalServerError})
		return
	}
	if err := s.registerScheduledPush(rec); err != nil {
		_ = scheduleStore.DeleteScheduledPush(r.Context(), rec.ID)
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error(), Code: http.StatusBadRequest})
		return
	}

	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) handleListPushSchedules(w http.ResponseWriter, r *http.Request) {
	if !s.canManagePushSchedules(w, r) {
		return
	}
	scheduleStore, ok := s.store.(pushScheduleStore)
	if !ok || scheduleStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "schedule store 未初始化", Code: http.StatusServiceUnavailable})
		return
	}
	records, err := scheduleStore.ListScheduledPushes(r.Context(), string(channel.PlatformFeishu))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: http.StatusInternalServerError})
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleDeletePushSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.canManagePushSchedules(w, r) {
		return
	}
	scheduleStore, ok := s.store.(pushScheduleStore)
	if !ok || scheduleStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "schedule store 未初始化", Code: http.StatusServiceUnavailable})
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "schedule id 不能为空", Code: http.StatusBadRequest})
		return
	}
	if err := scheduleStore.DeleteScheduledPush(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "schedule 未找到", Code: http.StatusNotFound})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error(), Code: http.StatusInternalServerError})
		return
	}
	if s.master != nil {
		s.master.StopCron(pushScheduleCronName(id))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) canManagePushSchedules(w http.ResponseWriter, r *http.Request) bool {
	if s == nil || s.config == nil || !s.config.Channel.Feishu.Push.Enabled {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "push 未启用", Code: http.StatusNotFound})
		return false
	}
	if auth.IsAuthEnabled(r.Context()) && auth.UserFrom(r.Context()) == nil {
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "未授权", Code: http.StatusUnauthorized})
		return false
	}
	if auth.IsAuthEnabled(r.Context()) {
		user := auth.UserFrom(r.Context())
		if user == nil || user.Role != "admin" {
			writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "schedule 管理需要管理员权限", Code: http.StatusForbidden})
			return false
		}
		if !canWritePush(r.Context()) {
			writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "缺少 push:write 权限", Code: http.StatusForbidden})
			return false
		}
	}
	if s.pushService == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "push service 未初始化", Code: http.StatusServiceUnavailable})
		return false
	}
	if s.master == nil {
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{Error: "master 未初始化", Code: http.StatusServiceUnavailable})
		return false
	}
	return true
}

func (s *Server) buildScheduledPushRecord(req pushScheduleRequest, user *auth.User) (*store.ScheduledPushRecord, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("schedule name 不能为空")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("schedule prompt 不能为空")
	}
	if req.IntervalSec <= 0 {
		return nil, errors.New("interval_sec 必须大于 0")
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = string(channel.PlatformFeishu)
	}
	if platform != string(channel.PlatformFeishu) {
		return nil, errors.New("当前仅支持 feishu schedule")
	}
	parsed, matched, err := push.ParseScheduledPrompt(req.Prompt)
	if err != nil {
		return nil, err
	} else if !matched {
		return nil, errors.New("schedule prompt 必须是 scheduled_push:*")
	}
	if parsed.Platform != "" && string(parsed.Platform) != platform {
		return nil, errors.New("schedule prompt platform 与请求 platform 不一致")
	}

	rec := &store.ScheduledPushRecord{
		ID:          newScheduleID(),
		Name:        strings.TrimSpace(req.Name),
		Platform:    platform,
		Prompt:      strings.TrimSpace(req.Prompt),
		IntervalSec: req.IntervalSec,
		Enabled:     req.Enabled,
	}
	if user != nil {
		rec.CreatedBy = user.ID
	}
	if rec.Enabled {
		rec.NextRunAt = time.Now().UTC().Add(time.Duration(rec.IntervalSec) * time.Second)
	}
	return rec, nil
}

func pushScheduleCronName(id string) string {
	return "scheduled-push:" + id
}

func (s *Server) registerScheduledPush(rec *store.ScheduledPushRecord) error {
	if s == nil || s.master == nil || rec == nil || !rec.Enabled {
		return nil
	}
	s.master.StopCron(pushScheduleCronName(rec.ID))
	scheduleStore, _ := s.store.(pushScheduleStore)
	return s.master.CronCreate(master.CronJob{
		ID:       rec.ID,
		Name:     pushScheduleCronName(rec.ID),
		Interval: time.Duration(rec.IntervalSec) * time.Second,
		Prompt:   rec.Prompt,
		Callback: func(ctx context.Context) error {
			return s.executeScheduledPush(ctx, rec, scheduleStore)
		},
	})
}

func (s *Server) executeScheduledPush(ctx context.Context, rec *store.ScheduledPushRecord, scheduleStore pushScheduleStore) error {
	runAt := time.Now().UTC()
	var lastError string
	err := s.pushService.DispatchScheduledPrompt(ctx, rec.Prompt)
	if err != nil {
		lastError = err.Error()
	}
	if scheduleStore != nil {
		_ = scheduleStore.UpdateScheduledPushRun(ctx, rec.ID, runAt, runAt.Add(time.Duration(rec.IntervalSec)*time.Second), lastError)
	}
	return err
}
