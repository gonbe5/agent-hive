package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestGatewayDispatch(t *testing.T) {
	logger := zap.NewNop()
	auth := NewAuthManager(nil)
	gw := New(auth, logger)

	gw.Register(MethodDef{
		Name:        "test.echo",
		Description: "测试回声",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return params, nil
		},
	})

	// 测试正常调用
	body, _ := json.Marshal(RPCRequest{ID: "1", Method: "test.echo", Params: json.RawMessage(`{"msg":"hello"}`)})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	gw.HandleHTTP(w, req)

	var resp RPCResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "1", resp.ID)
	assert.Nil(t, resp.Error)
	assert.Equal(t, `{"msg":"hello"}`, string(resp.Result))
}

func TestGatewayMethodNotFound(t *testing.T) {
	logger := zap.NewNop()
	auth := NewAuthManager(nil)
	gw := New(auth, logger)

	body, _ := json.Marshal(RPCRequest{ID: "1", Method: "nonexistent"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	gw.HandleHTTP(w, req)

	var resp RPCResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, 404, resp.Error.Code)
}

func TestGatewayAuth(t *testing.T) {
	logger := zap.NewNop()
	auth := NewAuthManager([]string{"secret-token"})
	gw := New(auth, logger)

	gw.Register(MethodDef{
		Name:      "protected.method",
		AuthScope: "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return json.Marshal(map[string]string{"status": "ok"})
		},
	})

	// 无 token 应返回 401
	body, _ := json.Marshal(RPCRequest{ID: "1", Method: "protected.method"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	gw.HandleHTTP(w, req)

	var resp RPCResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, 401, resp.Error.Code)

	// 有 token 应返回成功
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/rpc", bytes.NewReader(body))
	req2.Header.Set("Authorization", "Bearer secret-token")
	w2 := httptest.NewRecorder()
	gw.HandleHTTP(w2, req2)

	var resp2 RPCResponse
	json.NewDecoder(w2.Body).Decode(&resp2)
	assert.Nil(t, resp2.Error)
}
