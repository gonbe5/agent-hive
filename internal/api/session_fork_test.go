package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleForkSession(t *testing.T) {
	// 创建测试服务器
	handler, testMaster, cleanup := newTestServerForSessions(t)
	defer cleanup()

	// 创建一个会话
	reqBody := `{"name":"test-session"}`
	req := httptest.NewRequest("POST", "/api/v1/sessions", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var createResp CreateSessionResponse
	err := json.NewDecoder(rec.Body).Decode(&createResp)
	require.NoError(t, err)
	sessionID := createResp.SessionID

	// 测试 Fork 请求
	forkReqBody := ForkSessionRequest{
		ForkName:  "forked-session",
		ForkPoint: 0,
	}
	body, _ := json.Marshal(forkReqBody)

	forkReq := httptest.NewRequest("POST", "/api/v1/sessions/"+sessionID+"/fork", bytes.NewReader(body))
	forkReq.Header.Set("Content-Type", "application/json")
	forkReq.SetPathValue("id", sessionID)

	forkRec := httptest.NewRecorder()
	handler.ServeHTTP(forkRec, forkReq)

	// 验证响应
	assert.Equal(t, http.StatusCreated, forkRec.Code)

	var forkResp ForkSessionResponse
	err = json.NewDecoder(forkRec.Body).Decode(&forkResp)
	require.NoError(t, err)

	assert.NotEmpty(t, forkResp.ForkID)
	assert.Equal(t, "forked-session", forkResp.ForkName)
	assert.Equal(t, 0, forkResp.ForkPoint)

	// 验证当前会话已切换到 fork
	currentID, currentName := testMaster.GetCurrentSessionInfo()
	assert.Equal(t, forkResp.ForkID, currentID)
	assert.Equal(t, "forked-session", currentName)
}

func TestHandleRevertSession(t *testing.T) {
	// 创建测试服务器
	handler, _, cleanup := newTestServerForSessions(t)
	defer cleanup()

	// 创建一个会话
	reqBody := `{"name":"test-session"}`
	req := httptest.NewRequest("POST", "/api/v1/sessions", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var createResp CreateSessionResponse
	err := json.NewDecoder(rec.Body).Decode(&createResp)
	require.NoError(t, err)
	sessionID := createResp.SessionID

	// 测试 Revert 请求
	revertReqBody := RevertSessionRequest{
		RevertTo: 0,
	}
	body, _ := json.Marshal(revertReqBody)

	revertReq := httptest.NewRequest("POST", "/api/v1/sessions/"+sessionID+"/revert", bytes.NewReader(body))
	revertReq.Header.Set("Content-Type", "application/json")
	revertReq.SetPathValue("id", sessionID)

	revertRec := httptest.NewRecorder()
	handler.ServeHTTP(revertRec, revertReq)

	// 验证响应
	if revertRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", revertRec.Code, revertRec.Body.String())
	}

	var result map[string]string
	err = json.NewDecoder(revertRec.Body).Decode(&result)
	require.NoError(t, err)

	assert.Contains(t, result["message"], "回滚")
}

func TestHandleRevertSession_InvalidIndex(t *testing.T) {
	// 创建测试服务器
	handler, _, cleanup := newTestServerForSessions(t)
	defer cleanup()

	// 创建会话
	reqBody := `{"name":"test-session"}`
	req := httptest.NewRequest("POST", "/api/v1/sessions", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var createResp CreateSessionResponse
	err := json.NewDecoder(rec.Body).Decode(&createResp)
	require.NoError(t, err)
	sessionID := createResp.SessionID

	// 测试无效的 revert index（负数）
	revertReqBody := RevertSessionRequest{
		RevertTo: -1,
	}
	body, _ := json.Marshal(revertReqBody)

	revertReq := httptest.NewRequest("POST", "/api/v1/sessions/"+sessionID+"/revert", bytes.NewReader(body))
	revertReq.Header.Set("Content-Type", "application/json")
	revertReq.SetPathValue("id", sessionID)

	revertRec := httptest.NewRecorder()
	handler.ServeHTTP(revertRec, revertReq)

	// 应该返回 400 Bad Request
	assert.Equal(t, http.StatusBadRequest, revertRec.Code)
}
