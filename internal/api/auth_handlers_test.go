package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- mapAuthError tests ---

func TestMapAuthError_State(t *testing.T) {
	err := &testAuthError{msg: "state mismatch in cookie"}
	if got := mapAuthError(err); got != "state_mismatch" {
		t.Errorf("mapAuthError(%q) = %q, want state_mismatch", err.msg, got)
	}
}

func TestMapAuthError_Disabled(t *testing.T) {
	err := &testAuthError{msg: "用户已被禁用"}
	if got := mapAuthError(err); got != "user_disabled" {
		t.Errorf("mapAuthError(%q) = %q, want user_disabled", err.msg, got)
	}
}

func TestMapAuthError_RateLimited(t *testing.T) {
	err := &testAuthError{msg: "登录尝试过于频繁，请 15 分钟后再试"}
	if got := mapAuthError(err); got != "rate_limited" {
		t.Errorf("mapAuthError(%q) = %q, want rate_limited", err.msg, got)
	}
}

func TestMapAuthError_Default(t *testing.T) {
	err := &testAuthError{msg: "some unrelated error"}
	if got := mapAuthError(err); got != "auth_failed" {
		t.Errorf("mapAuthError(%q) = %q, want auth_failed", err.msg, got)
	}
}

// testAuthError implements error for testing mapAuthError
type testAuthError struct {
	msg string
}

func (e *testAuthError) Error() string {
	return e.msg
}

// --- handleAuthCallback tests ---

func TestHandleAuthCallback_AuthNotEnabled(t *testing.T) {
	srv := newTestServerNoAuth(t)
	req := httptest.NewRequest("GET", "/api/v1/auth/callback/feishu?code=abc&state=xyz", nil)
	rec := httptest.NewRecorder()

	srv.handleAuthCallback(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleAuthCallback_StateMismatch_MissingCookie(t *testing.T) {
	srv := newTestServerWithAuth(t)
	req := httptest.NewRequest("GET", "/api/v1/auth/callback/feishu?code=abc&state=xyz", nil)
	rec := httptest.NewRecorder()

	srv.handleAuthCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "error=state_mismatch") {
		t.Errorf("expected redirect with error=state_mismatch, got %s", rec.Header().Get("Location"))
	}
}

func TestHandleAuthCallback_StateMismatch_WrongState(t *testing.T) {
	srv := newTestServerWithAuth(t)
	// 先访问 handleAuthLogin 设置正确 cookie
	loginReq := httptest.NewRequest("GET", "/api/v1/auth/login/feishu", nil)
	loginRec := httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)
	cookies := loginRec.Result().Cookies()

	// 用错误的 state 访问 callback
	req := httptest.NewRequest("GET", "/api/v1/auth/callback/feishu?code=abc&state=wrongstate", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()

	srv.handleAuthCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "error=state_mismatch") {
		t.Errorf("expected redirect with error=state_mismatch, got %s", rec.Header().Get("Location"))
	}
}

func TestHandleAuthCallback_StateMismatch_EmptyState(t *testing.T) {
	srv := newTestServerWithAuth(t)
	req := httptest.NewRequest("GET", "/api/v1/auth/callback/feishu?code=abc&state=", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "somestate"})
	rec := httptest.NewRecorder()

	srv.handleAuthCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "error=state_mismatch") {
		t.Errorf("expected redirect with error=state_mismatch, got %s", rec.Header().Get("Location"))
	}
}

func TestHandleAuthCallback_MissingCode(t *testing.T) {
	srv := newTestServerWithAuth(t)
	state := "teststate123"
	req := httptest.NewRequest("GET", "/api/v1/auth/callback/feishu?code=&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: state})
	rec := httptest.NewRecorder()

	srv.handleAuthCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "error=auth_failed") {
		t.Errorf("expected redirect with error=auth_failed, got %s", rec.Header().Get("Location"))
	}
}

func TestHandleAuthCallback_InvalidProvider(t *testing.T) {
	srv := newTestServerWithAuth(t)
	req := httptest.NewRequest("GET", "/api/v1/auth/callback/nonexistent?code=abc&state=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "xyz"})
	rec := httptest.NewRecorder()

	srv.handleAuthCallback(rec, req)

	// 实际走 mapAuthError → auth_failed（provider 不存在返回错误字符串）
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
}

// --- handleLDAPLogin tests ---

func TestHandleLDAPLogin_AuthNotEnabled(t *testing.T) {
	srv := newTestServerNoAuth(t)
	body := `{"username":"test","password":"pass"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login/ldap", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleLDAPLogin(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleLDAPLogin_InvalidJSON(t *testing.T) {
	srv := newTestServerWithAuth(t)
	body := `{invalid json}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login/ldap", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleLDAPLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "请求格式错误") {
		t.Errorf("expected '请求格式错误', got %s", rec.Body.String())
	}
}

func TestHandleLDAPLogin_EmptyUsername(t *testing.T) {
	srv := newTestServerWithAuth(t)
	body := `{"username":"","password":"pass"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login/ldap", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleLDAPLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "用户名和密码不能为空") {
		t.Errorf("expected '用户名和密码不能为空', got %s", rec.Body.String())
	}
}

func TestHandleLDAPLogin_EmptyPassword(t *testing.T) {
	srv := newTestServerWithAuth(t)
	body := `{"username":"testuser","password":""}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login/ldap", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleLDAPLogin(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleLDAPLogin_WrongPassword(t *testing.T) {
	srv := newTestServerWithAuth(t)
	// CredentialLogin 会因为没有真实的 LDAP/dingtalk 后端而失败
	body := `{"username":"testuser","password":"wrongpass"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login/ldap", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleLDAPLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "用户名或密码错误") {
		t.Errorf("expected '用户名或密码错误', got %s", rec.Body.String())
	}
}

func TestHandleLDAPLogin_DefaultProvider(t *testing.T) {
	srv := newTestServerWithAuth(t)
	// 没有传 provider，默认用 ldap
	body := `{"username":"test","password":"pass"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login/ldap", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleLDAPLogin(rec, req)

	// 没有真实 provider，应该返回 401
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// --- handleListAuthProviders tests ---

func TestHandleListAuthProviders_NoAuth(t *testing.T) {
	srv := newTestServerNoAuth(t)
	req := httptest.NewRequest("GET", "/api/v1/auth/providers", nil)
	rec := httptest.NewRecorder()

	srv.handleListAuthProviders(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"providers":[]`) {
		t.Errorf("expected empty providers, got %s", rec.Body.String())
	}
}
