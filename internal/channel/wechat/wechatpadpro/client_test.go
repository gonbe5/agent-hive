package wechatpadpro

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestHTTPClient_CheckLoginStatus 测试登录状态查询
func TestHTTPClient_CheckLoginStatus(t *testing.T) {
	tests := []struct {
		name       string
		mockResp   APIResponse
		wantErr    bool
		wantLogin  bool
		wantWxID   string
	}{
		{
			name: "已登录",
			mockResp: APIResponse{
				Code: 200,
				Text: "success",
				Data: map[string]interface{}{
					"isLogin":  true,
					"wxid":     "test_user_001",
					"nickname": "测试用户",
				},
			},
			wantErr:   false,
			wantLogin: true,
			wantWxID:  "test_user_001",
		},
		{
			name: "未登录",
			mockResp: APIResponse{
				Code: 200,
				Text: "success",
				Data: map[string]interface{}{
					"isLogin": false,
				},
			},
			wantErr:   false,
			wantLogin: false,
			wantWxID:  "",
		},
		{
			name: "API 错误",
			mockResp: APIResponse{
				Code: 500,
				Text: "internal error",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建 mock 服务器
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/login/CheckLoginStatus", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)
				// 验证 key 参数
				assert.Equal(t, "test_key", r.URL.Query().Get("key"))

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResp)
			}))
			defer server.Close()

			// 创建客户端
			client := NewHTTPClient(HTTPClientConfig{
				BaseURL: server.URL,
				Key:     "test_key",
				Timeout: 5 * time.Second,
				Logger:  zap.NewNop(),
			})

			// 调用方法
			ctx := context.Background()
			status, err := client.CheckLoginStatus(ctx)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantLogin, status.IsLogin)
			assert.Equal(t, tt.wantWxID, status.WxID)
		})
	}
}

// TestHTTPClient_SendTextMessage 测试发送文本消息
func TestHTTPClient_SendTextMessage(t *testing.T) {
	tests := []struct {
		name     string
		mockResp APIResponse
		wantErr  bool
	}{
		{
			name: "发送成功",
			mockResp: APIResponse{
				Code: 200,
				Text: "success",
			},
			wantErr: false,
		},
		{
			name: "API 错误",
			mockResp: APIResponse{
				Code: 400,
				Text: "invalid wxid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建 mock 服务器
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/message/SendTextMessage", r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				// 验证 key 参数
				assert.Equal(t, "test_key", r.URL.Query().Get("key"))

				// 验证请求体
				var req SendMessageModel
				err := json.NewDecoder(r.Body).Decode(&req)
				require.NoError(t, err)
				assert.Len(t, req.MsgItem, 1)
				assert.Equal(t, "wxid_test", req.MsgItem[0].ToUserName)
				assert.Equal(t, "Hello", req.MsgItem[0].TextContent)
				assert.Equal(t, 1, req.MsgItem[0].MsgType)

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResp)
			}))
			defer server.Close()

			// 创建客户端
			client := NewHTTPClient(HTTPClientConfig{
				BaseURL: server.URL,
				Key:     "test_key",
				Timeout: 5 * time.Second,
				Logger:  zap.NewNop(),
			})

			// 调用方法
			ctx := context.Background()
			err := client.SendTextMessage(ctx, "wxid_test", "Hello")

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestHTTPClient_APIError 测试 API 错误处理
func TestHTTPClient_APIError(t *testing.T) {
	tests := []struct {
		name         string
		mockResponse string // 原始 JSON 字符串
		wantErr      bool
		errContains  string
	}{
		{
			name:         "非法 JSON",
			mockResponse: `{invalid json}`,
			wantErr:      true,
			errContains:  "响应解析失败",
		},
		{
			name:         "API 返回错误码",
			mockResponse: `{"Code": 401, "Text": "unauthorized"}`,
			wantErr:      true,
			errContains:  "API 错误 (code=401)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建 mock 服务器
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tt.mockResponse))
			}))
			defer server.Close()

			// 创建客户端
			client := NewHTTPClient(HTTPClientConfig{
				BaseURL: server.URL,
				Key:     "test_key",
				Timeout: 5 * time.Second,
				Logger:  zap.NewNop(),
			})

			// 调用方法（使用 CheckLoginStatus 作为测试）
			ctx := context.Background()
			_, err := client.CheckLoginStatus(ctx)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
