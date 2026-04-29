package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/skills"
)

func TestHandleGetWeChatConfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewOverlayRegistry(logger)

	cfg := config.Default()
	cfg.Channel.WeChat.Wechaty.Enabled = true
	cfg.Channel.WeChat.Wechaty.Endpoint = "localhost:8788"

	srv := NewServer(
		config.ServerConfig{Port: 0},
		config.HITLConfig{},
		config.WebUIConfig{},
		nil, // master 在这个测试中不需要
		skillReg,
		cfg,
		"",  // configPath 空字符串用于测试
		nil, // channelRouter 为 nil，所有协议状态应为 not_started
		nil, // store
		nil, // authEngine
		logger,
	)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/config/channels/wechat", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp WeChatConfigResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	// 验证响应结构
	assert.Contains(t, resp.Protocols, "wechaty")
	assert.Contains(t, resp.Protocols, "wechatpadpro")

	// 验证 wechaty 状态
	wechaty := resp.Protocols["wechaty"]
	assert.True(t, wechaty.Enabled)
	assert.Equal(t, "not_started", wechaty.Status)
	assert.False(t, wechaty.LoggedIn)
	assert.Equal(t, "localhost:8788", wechaty.Config["endpoint"])

	// 验证 wechatpadpro 未启用
	assert.False(t, resp.Protocols["wechatpadpro"].Enabled)
}

func TestHandleGetWeChatConfig_WithChannelRouter(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewOverlayRegistry(logger)

	cfg := config.Default()
	cfg.Channel.WeChat.WeChatPadPro.Enabled = true

	// 创建 channel router 和 mock processor
	mockProcessor := &mockChannelProcessor{}
	channelRouter := channel.NewRouter(mockProcessor, logger)

	srv := NewServer(
		config.ServerConfig{Port: 0},
		config.HITLConfig{},
		config.WebUIConfig{},
		nil,
		skillReg,
		cfg,
		"", // configPath 空字符串用于测试
		channelRouter,
		nil, // store
		nil, // authEngine
		logger,
	)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/config/channels/wechat", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp WeChatConfigResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	// 验证响应
	assert.True(t, resp.Protocols["wechatpadpro"].Enabled)
	assert.Equal(t, "not_started", resp.Protocols["wechatpadpro"].Status)
}

// mockChannelProcessor 用于测试的 mock processor
type mockChannelProcessor struct{}

func (m *mockChannelProcessor) ProcessMessage(ctx context.Context, sessionID string, input string) (master.TaskResponse, error) {
	return master.TaskResponse{}, nil
}

func TestHandleUpdateWeChatProtocol(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewOverlayRegistry(logger)

	cfg := config.Default()

	srv := NewServer(
		config.ServerConfig{Port: 0},
		config.HITLConfig{},
		config.WebUIConfig{},
		nil,
		skillReg,
		cfg,
		"", // configPath 空字符串用于测试
		nil,
		nil, // store
		nil, // authEngine
		logger,
	)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	// 更新 wechatpadpro 配置
	reqBody := `{"enabled": true, "config": {"base_url": "http://localhost:8848", "timeout": 30}}`
	req := httptest.NewRequest("PATCH", "/api/v1/config/channels/wechat/wechatpadpro", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// 验证配置已更新
	assert.True(t, srv.config.Channel.WeChat.WeChatPadPro.Enabled)
	assert.Equal(t, "http://localhost:8848", srv.config.Channel.WeChat.WeChatPadPro.BaseURL)
	assert.Equal(t, 30, srv.config.Channel.WeChat.WeChatPadPro.Timeout)
}

func TestHandleUpdateWeChatProtocol_InvalidProtocol(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewOverlayRegistry(logger)

	cfg := config.Default()

	srv := NewServer(
		config.ServerConfig{Port: 0},
		config.HITLConfig{},
		config.WebUIConfig{},
		nil,
		skillReg,
		cfg,
		"",
		nil,
		nil, // store
		nil, // authEngine
		logger,
	)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	// 使用不支持的协议名称
	reqBody := `{"enabled": true}`
	req := httptest.NewRequest("PATCH", "/api/v1/config/channels/wechat/openwechat", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	assert.Contains(t, resp.Error, "不支持的协议")
}

func TestHandleSaveConfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewOverlayRegistry(logger)

	// 创建临时配置文件
	tmpFile, err := os.CreateTemp("", "config-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cfg := config.Default()
	cfg.Server.Port = 9999

	srv := NewServer(
		config.ServerConfig{Port: 0},
		config.HITLConfig{},
		config.WebUIConfig{},
		nil,
		skillReg,
		cfg,
		tmpFile.Name(),
		nil,
		nil, // store
		nil, // authEngine
		logger,
	)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	req := httptest.NewRequest("POST", "/api/v1/config/save", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// 验证配置已保存（SaveToFile 现在只写引导参数：server/store/logging/gateway）
	savedData, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)

	var savedCfg map[string]json.RawMessage
	err = json.Unmarshal(savedData, &savedCfg)
	require.NoError(t, err)

	// 引导参数应存在
	assert.Contains(t, savedCfg, "server")
	assert.Contains(t, savedCfg, "logging")

	// 运行时配置（llm/channel/agent/hitl 等）不应出现在文件中
	assert.NotContains(t, savedCfg, "llm")
	assert.NotContains(t, savedCfg, "channel")
	assert.NotContains(t, savedCfg, "agent")
	assert.NotContains(t, savedCfg, "hitl")

	// 验证引导参数值
	var serverCfg config.ServerConfig
	require.NoError(t, json.Unmarshal(savedCfg["server"], &serverCfg))
	assert.Equal(t, 9999, serverCfg.Port)
}

func TestHandleReloadWeChatProtocol_NoReloadFunc(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewOverlayRegistry(logger)

	cfg := config.Default()
	cfg.Channel.WeChat.Wechaty.Enabled = true

	// 创建 channel router
	mockProcessor := &mockChannelProcessor{}
	channelRouter := channel.NewRouter(mockProcessor, logger)

	srv := NewServer(
		config.ServerConfig{Port: 0},
		config.HITLConfig{},
		config.WebUIConfig{},
		nil,
		skillReg,
		cfg,
		"",
		channelRouter,
		nil, // store
		nil, // authEngine
		logger,
	)

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	req := httptest.NewRequest("POST", "/api/v1/config/channels/wechat/wechatpadpro/reload", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	assert.Equal(t, "stopped", resp["status"])
}

func TestHandleReloadWeChatProtocol_WithReloadFunc(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewOverlayRegistry(logger)

	cfg := config.Default()

	// 创建 channel router
	mockProcessor := &mockChannelProcessor{}
	channelRouter := channel.NewRouter(mockProcessor, logger)

	srv := NewServer(
		config.ServerConfig{Port: 0},
		config.HITLConfig{},
		config.WebUIConfig{},
		nil,
		skillReg,
		cfg,
		"",
		channelRouter,
		nil, // store
		nil, // authEngine
		logger,
	)

	// 设置 reload 回调
	reloadCalled := false
	srv.SetReloadProtocolFunc(func(protocol string) error {
		reloadCalled = true
		assert.Equal(t, "wechatpadpro", protocol)
		return nil
	})

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	req := httptest.NewRequest("POST", "/api/v1/config/channels/wechat/wechatpadpro/reload", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, reloadCalled)
}
