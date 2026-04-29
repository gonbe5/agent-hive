package acpserver

import (
	"context"
	"encoding/json"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
)

func newTestAgent(t *testing.T, authToken string) *ClawAgent {
	t.Helper()
	return &ClawAgent{
		cfg:    config.ACPServerConfig{AuthToken: authToken},
		logger: zaptest.NewLogger(t),
	}
}

func TestAuthenticate_NoTokenConfigured_AllowsAll(t *testing.T) {
	agent := newTestAgent(t, "")
	_, err := agent.Authenticate(context.Background(), acp.AuthenticateRequest{})
	assert.NoError(t, err, "空 AuthToken 应允许所有连接")
}

func TestAuthenticate_TokenMatch_MapMeta(t *testing.T) {
	agent := newTestAgent(t, "secret")
	req := acp.AuthenticateRequest{
		Meta: map[string]any{"token": "secret"},
	}
	_, err := agent.Authenticate(context.Background(), req)
	assert.NoError(t, err, "token 匹配时应认证成功")
}

func TestAuthenticate_TokenMismatch_MapMeta(t *testing.T) {
	agent := newTestAgent(t, "secret")
	req := acp.AuthenticateRequest{
		Meta: map[string]any{"token": "wrong"},
	}
	_, err := agent.Authenticate(context.Background(), req)
	require.Error(t, err)
	var e *errs.Error
	assert.ErrorAs(t, err, &e, "应返回认证错误")
}

func TestAuthenticate_TokenMatch_JSONByteMeta(t *testing.T) {
	agent := newTestAgent(t, "secret")
	raw, _ := json.Marshal(map[string]any{"token": "secret"})
	req := acp.AuthenticateRequest{Meta: raw}
	_, err := agent.Authenticate(context.Background(), req)
	assert.NoError(t, err, "json.RawMessage meta 中 token 匹配应认证成功")
}

func TestAuthenticate_TokenMatch_StringMeta(t *testing.T) {
	agent := newTestAgent(t, "secret")
	req := acp.AuthenticateRequest{Meta: `{"token":"secret"}`}
	_, err := agent.Authenticate(context.Background(), req)
	assert.NoError(t, err, "string meta 中 token 匹配应认证成功")
}

func TestAuthenticate_MetaTypeWrong_Rejects(t *testing.T) {
	agent := newTestAgent(t, "secret")
	// Meta 是整数，无法提取 token
	req := acp.AuthenticateRequest{Meta: 12345}
	_, err := agent.Authenticate(context.Background(), req)
	require.Error(t, err, "无法提取 token 时应拒绝")
}

func TestAuthenticate_MetaNil_Rejects(t *testing.T) {
	agent := newTestAgent(t, "secret")
	req := acp.AuthenticateRequest{Meta: nil}
	_, err := agent.Authenticate(context.Background(), req)
	require.Error(t, err, "nil meta 时应拒绝")
}

func TestExtractToken(t *testing.T) {
	tests := []struct {
		name string
		meta any
		want string
	}{
		{"map[string]any", map[string]any{"token": "abc"}, "abc"},
		{"json bytes", []byte(`{"token":"xyz"}`), "xyz"},
		{"json string", `{"token":"str"}`, "str"},
		{"missing token key", map[string]any{"other": "val"}, ""},
		{"nil", nil, ""},
		{"invalid type", 42, ""},
		{"invalid json bytes", []byte("not json"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToken(tt.meta)
			assert.Equal(t, tt.want, got)
		})
	}
}
