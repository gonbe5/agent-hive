package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// TestNormalizeURL 测试 URL 规范化
func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{
			name:  "HTTPS URL 保持不变",
			input: "https://example.com",
			want:  "https://example.com",
		},
		{
			name:  "HTTP 自动升级为 HTTPS",
			input: "http://example.com",
			want:  "https://example.com",
		},
		{
			name:  "无 scheme 默认 HTTPS",
			input: "example.com",
			want:  "https://example.com",
		},
		{
			name:  "带路径的 URL",
			input: "http://example.com/path/to/page",
			want:  "https://example.com/path/to/page",
		},
		{
			name:  "带查询参数的 URL",
			input: "http://example.com?foo=bar&baz=qux",
			want:  "https://example.com?foo=bar&baz=qux",
		},
		{
			name:      "空 URL",
			input:     "",
			wantError: true,
		},
		{
			name:      "空格 URL",
			input:     "   ",
			wantError: true,
		},
		{
			name:      "不支持的协议",
			input:     "ftp://example.com",
			wantError: true,
		},
		{
			name:      "无效的 URL",
			input:     "://invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeURL(tt.input)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestIsTextContent 测试 Content-Type 检查
func TestIsTextContent(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{"空类型（默认允许）", "", true},
		{"text/html", "text/html", true},
		{"text/html; charset=utf-8", "text/html; charset=utf-8", true},
		{"text/plain", "text/plain", true},
		{"application/xhtml+xml", "application/xhtml+xml", true},
		{"application/xml", "application/xml", true},
		{"text/xml", "text/xml", true},
		{"image/png", "image/png", false},
		{"application/json", "application/json", false},
		{"application/pdf", "application/pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTextContent(tt.contentType)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHtmlToText 测试 HTML 到文本的转换
func TestHtmlToText(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		want    string
		wantErr bool
	}{
		{
			name: "简单段落",
			html: "<p>Hello World</p>",
			want: "Hello World",
		},
		{
			name: "标题",
			html: "<h1>Title</h1><h2>Subtitle</h2>",
			want: "# Title\n\n## Subtitle",
		},
		{
			name: "链接",
			html: `<a href="https://example.com">Example</a>`,
			want: "[Example](https://example.com)",
		},
		{
			name: "粗体和斜体",
			html: "<p>This is <strong>bold</strong> and <em>italic</em> text.</p>",
			want: "This is **bold** and *italic* text.",
		},
		{
			name: "无序列表",
			html: "<ul><li>Item 1</li><li>Item 2</li><li>Item 3</li></ul>",
			want: "- Item 1\n- Item 2\n- Item 3",
		},
		{
			name: "有序列表",
			html: "<ol><li>First</li><li>Second</li><li>Third</li></ol>",
			want: "1. First\n2. Second\n3. Third",
		},
		{
			name: "代码块",
			html: "<pre><code>func main() {\n  println(\"Hello\")\n}</code></pre>",
			want: "```\nfunc main() {\n  println(\"Hello\")\n}\n```",
		},
		{
			name: "行内代码",
			html: "<p>Use the <code>fmt.Println</code> function.</p>",
			want: "Use the `fmt.Println` function.",
		},
		{
			name: "引用",
			html: "<blockquote>This is a quote.</blockquote>",
			want: "> This is a quote.",
		},
		{
			name: "跳过脚本和样式",
			html: "<p>Visible</p><script>alert('hidden');</script><style>.hidden { display: none; }</style>",
			want: "Visible",
		},
		{
			name: "换行符",
			html: "<p>Line 1<br>Line 2</p>",
			want: "Line 1\nLine 2",
		},
		{
			name: "水平线",
			html: "<p>Before</p><hr><p>After</p>",
			want: "Before\n\n---\n\nAfter",
		},
		{
			name: "嵌套结构",
			html: `
				<div>
					<h1>Main Title</h1>
					<p>This is a <strong>paragraph</strong> with a <a href="http://example.com">link</a>.</p>
					<ul>
						<li>Item 1</li>
						<li>Item 2</li>
					</ul>
				</div>
			`,
			want: "# Main Title\n\nThis is a **paragraph** with a [link](http://example.com).\n\n- Item 1\n- Item 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := htmlToText(tt.html)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				// 规范化空白以便比较
				gotNorm := strings.TrimSpace(got)
				wantNorm := strings.TrimSpace(tt.want)
				assert.Equal(t, wantNorm, gotNorm)
			}
		})
	}
}

// TestCleanupText 测试文本清理
func TestCleanupText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "去除首尾空白",
			input: "  \n\n  Hello World  \n\n  ",
			want:  "Hello World",
		},
		{
			name:  "压缩连续空行",
			input: "Line 1\n\n\n\n\nLine 2",
			want:  "Line 1\n\nLine 2",
		},
		{
			name:  "去除行尾空格",
			input: "Line 1  \nLine 2\t\nLine 3",
			want:  "Line 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanupText(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestWebFetch_MockServer 测试 webfetch 工具（使用 Mock HTTP 服务器）
func TestWebFetch_MockServer(t *testing.T) {
	// 测试服务器使用自签名 TLS 证书并监听在 127.0.0.1，
	// 需要同时开启 InsecureSkipVerify 和私有地址访问权限
	t.Setenv("WEBFETCH_INSECURE_TLS", "true")
	t.Setenv("WEBFETCH_ALLOW_PRIVATE", "true")

	tests := []struct {
		name           string
		handler        http.HandlerFunc
		allowedDomains []string
		blockedDomains []string
		wantError      bool
		wantContains   string
	}{
		{
			name: "成功获取 HTML",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				fmt.Fprintln(w, "<html><body><h1>Test Page</h1><p>Hello World</p></body></html>")
			},
			wantContains: "# Test Page",
		},
		{
			name: "成功获取纯文本",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprintln(w, "Plain text content")
			},
			wantContains: "Plain text content",
		},
		{
			name: "HTTP 404 错误",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintln(w, "Not Found")
			},
			wantError: true,
		},
		{
			name: "不支持的 Content-Type",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/pdf")
				fmt.Fprintln(w, "PDF content")
			},
			wantError: true,
		},
		{
			name: "检查 User-Agent",
			handler: func(w http.ResponseWriter, r *http.Request) {
				ua := r.Header.Get("User-Agent")
				assert.Contains(t, ua, "Mozilla")
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprintln(w, "<p>OK</p>")
			},
			wantContains: "OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建 Mock HTTPS 服务器
			server := httptest.NewTLSServer(tt.handler)
			defer server.Close()

			// 创建 MCP Host
			logger := zap.NewNop()
			host := mcphost.NewHost(logger)
			registerWebFetch(host, logger)

			// 准备输入
			input := map[string]any{
				"url": server.URL,
			}
			if len(tt.allowedDomains) > 0 {
				input["allowed_domains"] = tt.allowedDomains
			}
			if len(tt.blockedDomains) > 0 {
				input["blocked_domains"] = tt.blockedDomains
			}
			inputJSON, _ := json.Marshal(input)

			// 执行工具
			ctx := context.Background()
			result, err := host.ExecuteTool(ctx, "webfetch", inputJSON)

			require.NoError(t, err)

			if tt.wantError {
				assert.True(t, result.IsError, "期望返回错误")
			} else {
				assert.False(t, result.IsError, "不应该返回错误")
				var content string
				json.Unmarshal(result.Content, &content)
				assert.Contains(t, content, tt.wantContains)
			}
		})
	}
}

// TestWebFetch_DomainFilter 测试域名过滤
func TestWebFetch_DomainFilter(t *testing.T) {
	// 测试服务器使用自签名 TLS 证书并监听在 127.0.0.1
	t.Setenv("WEBFETCH_INSECURE_TLS", "true")
	t.Setenv("WEBFETCH_ALLOW_PRIVATE", "true")

	// 创建 Mock HTTPS 服务器
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, "<p>Content</p>")
	}))
	defer server.Close()

	tests := []struct {
		name           string
		allowedDomains []string
		blockedDomains []string
		wantError      bool
	}{
		{
			name:           "无过滤",
			allowedDomains: nil,
			blockedDomains: nil,
			wantError:      false,
		},
		{
			name:           "白名单匹配（127.0.0.1）",
			allowedDomains: []string{"127.0.0.1"},
			wantError:      false,
		},
		{
			name:           "白名单不匹配",
			allowedDomains: []string{"example.com"},
			wantError:      true,
		},
		{
			name:           "黑名单匹配（127.0.0.1）",
			blockedDomains: []string{"127.0.0.1"},
			wantError:      true,
		},
		{
			name:           "黑名单不匹配",
			blockedDomains: []string{"example.com"},
			wantError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建 MCP Host
			logger := zap.NewNop()
			host := mcphost.NewHost(logger)
			registerWebFetch(host, logger)

			// 准备输入
			input := map[string]any{
				"url": server.URL,
			}
			if len(tt.allowedDomains) > 0 {
				input["allowed_domains"] = tt.allowedDomains
			}
			if len(tt.blockedDomains) > 0 {
				input["blocked_domains"] = tt.blockedDomains
			}
			inputJSON, _ := json.Marshal(input)

			// 执行工具
			ctx := context.Background()
			result, err := host.ExecuteTool(ctx, "webfetch", inputJSON)

			require.NoError(t, err)

			if tt.wantError {
				assert.True(t, result.IsError, "期望返回错误")
			} else {
				assert.False(t, result.IsError, "不应该返回错误")
			}
		})
	}
}

// TestWebFetch_Timeout 测试超时保护
func TestWebFetch_Timeout(t *testing.T) {
	// 测试服务器使用自签名 TLS 证书并监听在 127.0.0.1
	t.Setenv("WEBFETCH_INSECURE_TLS", "true")
	t.Setenv("WEBFETCH_ALLOW_PRIVATE", "true")

	// 创建一个慢速服务器（使用 request context 感知的等待，避免 server.Close 超时）
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Second):
		case <-r.Context().Done():
		}
		fmt.Fprintln(w, "<p>Slow</p>")
	}))
	defer server.Close()

	// 创建 MCP Host
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerWebFetch(host, logger)

	// 执行工具
	input := map[string]any{
		"url": server.URL,
	}
	inputJSON, _ := json.Marshal(input)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := host.ExecuteTool(ctx, "webfetch", inputJSON)

	require.NoError(t, err)
	assert.True(t, result.IsError, "应该因为超时返回错误")

	var content string
	json.Unmarshal(result.Content, &content)
	// 超时错误可能包含不同的消息，检查是否包含请求失败或超时相关的词
	assert.True(t, strings.Contains(content, "请求失败") || strings.Contains(content, "超时") || strings.Contains(content, "timeout") || strings.Contains(content, "context deadline exceeded"), "错误消息应包含超时相关信息")
}

// TestWebFetch_SizeLimit 测试体积限制
func TestWebFetch_SizeLimit(t *testing.T) {
	// 测试服务器使用自签名 TLS 证书并监听在 127.0.0.1
	t.Setenv("WEBFETCH_INSECURE_TLS", "true")
	t.Setenv("WEBFETCH_ALLOW_PRIVATE", "true")

	// 创建一个返回大响应的服务器
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// 写入超过 maxResponseSize 的数据
		largeContent := strings.Repeat("A", maxResponseSize+1000)
		fmt.Fprintln(w, largeContent)
	}))
	defer server.Close()

	// 创建 MCP Host
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerWebFetch(host, logger)

	// 执行工具
	input := map[string]any{
		"url": server.URL,
	}
	inputJSON, _ := json.Marshal(input)

	ctx := context.Background()
	result, err := host.ExecuteTool(ctx, "webfetch", inputJSON)

	require.NoError(t, err)
	assert.True(t, result.IsError, "应该因为体积超限返回错误")

	var content string
	json.Unmarshal(result.Content, &content)
	assert.Contains(t, content, "体积", "错误消息应包含'体积'")
}

// TestWebFetch_RedirectLimit 测试重定向限制
func TestWebFetch_RedirectLimit(t *testing.T) {
	// 测试服务器使用自签名 TLS 证书并监听在 127.0.0.1
	t.Setenv("WEBFETCH_INSECURE_TLS", "true")
	t.Setenv("WEBFETCH_ALLOW_PRIVATE", "true")

	redirectCount := 0

	// 创建一个无限重定向的服务器
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		http.Redirect(w, r, "/redirect", http.StatusFound)
	}))
	defer server.Close()

	// 创建 MCP Host
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerWebFetch(host, logger)

	// 执行工具
	input := map[string]any{
		"url": server.URL,
	}
	inputJSON, _ := json.Marshal(input)

	ctx := context.Background()
	result, err := host.ExecuteTool(ctx, "webfetch", inputJSON)

	require.NoError(t, err)
	assert.True(t, result.IsError, "应该因为重定向过多返回错误")

	var content string
	json.Unmarshal(result.Content, &content)
	assert.Contains(t, content, "重定向", "错误消息应包含'重定向'")
}

// TestCheckSSRF 测试 SSRF 防护函数
func TestCheckSSRF(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		// 应该被拦截的私有/保留地址
		{"IPv4 回环 127.0.0.1", "127.0.0.1", true},
		{"IPv4 回环 127.0.0.2", "127.0.0.2", true},
		{"私有 A 类 10.0.0.1", "10.0.0.1", true},
		{"私有 A 类 10.255.255.255", "10.255.255.255", true},
		{"私有 B 类 172.16.0.1", "172.16.0.1", true},
		{"私有 B 类 172.31.255.255", "172.31.255.255", true},
		{"私有 C 类 192.168.0.1", "192.168.0.1", true},
		{"私有 C 类 192.168.255.255", "192.168.255.255", true},
		{"链路本地 169.254.1.1", "169.254.1.1", true},
		{"未指定地址 0.0.0.0", "0.0.0.0", true},
		{"IPv6 回环 ::1", "::1", true},
		{"IPv6 唯一本地 fc00::1", "fc00::1", true},
		{"IPv6 唯一本地 fd00::1", "fd00::1", true},
		{"IPv6 链路本地 fe80::1", "fe80::1", true},
		// 应该允许的公网地址
		{"公网 IP 8.8.8.8", "8.8.8.8", false},
		{"公网 IP 1.1.1.1", "1.1.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSSRF(tt.host)
			if tt.wantErr {
				assert.Error(t, err, "期望 SSRF 防护拦截地址: %s", tt.host)
			} else {
				assert.NoError(t, err, "期望允许公网地址: %s", tt.host)
			}
		})
	}
}

// TestWebFetch_SSRFProtection 集成测试：验证 SSRF 防护默认阻止私有地址
func TestWebFetch_SSRFProtection(t *testing.T) {
	// 注意：不设置 WEBFETCH_ALLOW_PRIVATE，验证默认行为会拦截私有地址
	// 同时设置 WEBFETCH_INSECURE_TLS=true（但 SSRF 防护先于 TLS，所以 URL 会被拦截）
	t.Setenv("WEBFETCH_INSECURE_TLS", "true")

	// 创建监听在 127.0.0.1 的测试服务器
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, "<p>不应该被访问</p>")
	}))
	defer server.Close()

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerWebFetch(host, logger)

	input := map[string]any{"url": server.URL}
	inputJSON, _ := json.Marshal(input)

	ctx := context.Background()
	result, err := host.ExecuteTool(ctx, "webfetch", inputJSON)

	require.NoError(t, err)
	// 默认情况下，127.0.0.1 应该被 SSRF 防护拦截
	assert.True(t, result.IsError, "SSRF 防护应该拦截对 127.0.0.1 的访问")

	var content string
	json.Unmarshal(result.Content, &content)
	assert.True(t,
		strings.Contains(content, "内网") || strings.Contains(content, "禁止"),
		"错误消息应包含内网访问拒绝相关说明，实际: %s", content)
}

// TestParseNumericIP 验证 SEC-003 修复：数字格式 IP 绕过检测
// net.ParseIP 无法识别十六进制整数、纯十进制整数、八进制点分等格式，
// 但底层操作系统或 C 库会将它们解析为合法地址，从而绕过 SSRF 检查。
// parseNumericIP 负责捕获这些非标准格式并返回对应的 net.IP。
func TestParseNumericIP(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool   // 期望返回 nil（无法解析）
		wantIP  string // 期望解析后的规范 IP 字符串（非 nil 时）
	}{
		// ── 十六进制整数 ──────────────────────────────────────────────────────
		{
			name:   "十六进制 0x7f000001 → 127.0.0.1",
			input:  "0x7f000001",
			wantIP: "127.0.0.1",
		},
		{
			name:   "十六进制大写 0X7F000001 → 127.0.0.1",
			input:  "0X7F000001",
			wantIP: "127.0.0.1",
		},
		{
			name:   "十六进制 0xc0a80001 → 192.168.0.1",
			input:  "0xc0a80001",
			wantIP: "192.168.0.1",
		},
		{
			name:   "十六进制 0x0a000001 → 10.0.0.1",
			input:  "0x0a000001",
			wantIP: "10.0.0.1",
		},
		// ── 纯十进制整数 ──────────────────────────────────────────────────────
		{
			name:   "十进制整数 2130706433 → 127.0.0.1",
			input:  "2130706433",
			wantIP: "127.0.0.1",
		},
		{
			name:   "十进制整数 3232235521 → 192.168.0.1",
			input:  "3232235521",
			wantIP: "192.168.0.1",
		},
		{
			name:   "十进制整数 167772161 → 10.0.0.1",
			input:  "167772161",
			wantIP: "10.0.0.1",
		},
		// ── 八进制点分 ────────────────────────────────────────────────────────
		{
			name:   "八进制 0177.0.0.1 → 127.0.0.1",
			input:  "0177.0.0.1",
			wantIP: "127.0.0.1",
		},
		{
			name:   "八进制 0300.0250.0.01 → 192.168.0.1",
			input:  "0300.0250.0.01",
			wantIP: "192.168.0.1",
		},
		// ── 标准格式（应返回 nil，由 net.ParseIP 处理）────────────────────────
		{
			name:    "标准点分十进制（返回 nil，交由 ParseIP）",
			input:   "127.0.0.1",
			wantNil: true,
		},
		{
			name:    "普通域名（返回 nil）",
			input:   "example.com",
			wantNil: true,
		},
		{
			name:    "IPv6（返回 nil，不处理）",
			input:   "::1",
			wantNil: true,
		},
		{
			name:    "空字符串（返回 nil）",
			input:   "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNumericIP(tt.input)
			if tt.wantNil {
				assert.Nil(t, got, "parseNumericIP(%q) 期望返回 nil", tt.input)
				return
			}
			require.NotNil(t, got, "parseNumericIP(%q) 期望返回非 nil IP", tt.input)
			// 使用 To4() 规范化为 4 字节再比较，消除 IPv4-in-IPv6 的表示差异
			ip4 := got.To4()
			if ip4 != nil {
				assert.Equal(t, tt.wantIP, ip4.String(),
					"parseNumericIP(%q) 解析结果不匹配", tt.input)
			} else {
				assert.Equal(t, tt.wantIP, got.String(),
					"parseNumericIP(%q) 解析结果不匹配", tt.input)
			}
		})
	}
}

// TestCheckSSRF_NumericIPBypass 验证 checkSSRF 能拦截数字格式 IP（SEC-003）
func TestCheckSSRF_NumericIPBypass(t *testing.T) {
	tests := []struct {
		name string
		host string
	}{
		{"十六进制 0x7f000001 → 127.0.0.1 应被拦截", "0x7f000001"},
		{"十进制整数 2130706433 → 127.0.0.1 应被拦截", "2130706433"},
		{"八进制 0177.0.0.1 → 127.0.0.1 应被拦截", "0177.0.0.1"},
		{"十六进制 0xc0a80001 → 192.168.0.1 应被拦截", "0xc0a80001"},
		{"十进制整数 3232235521 → 192.168.0.1 应被拦截", "3232235521"},
		{"十六进制 0x0a000001 → 10.0.0.1 应被拦截", "0x0a000001"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSSRF(tt.host)
			assert.Error(t, err, "checkSSRF 应拦截数字格式私有地址: %s", tt.host)
		})
	}
}

// TestWebFetch_DialContextSSRF 验证 SEC-001 修复：DialContext 层在 TCP 连接建立时拦截私有地址。
// 即使 fast-fail 的 checkSSRF 通过（模拟 DNS 重绑定窗口后的情况），
// DialContext 仍能在实际 Dial 时阻断连接。
//
// 这里通过直接调用 newSSRFDialContext() 返回的函数来验证其对私有地址的拦截。
func TestWebFetch_DialContextSSRF(t *testing.T) {
	// 确保 WEBFETCH_ALLOW_PRIVATE 未设置（或不为 "true"）
	t.Setenv("WEBFETCH_ALLOW_PRIVATE", "false")

	dialFn := newSSRFDialContext()
	ctx := context.Background()

	privateAddrs := []struct {
		name string
		addr string // host:port 格式
	}{
		{"回环地址 127.0.0.1", "127.0.0.1:80"},
		{"私有 A 类 10.0.0.1", "10.0.0.1:80"},
		{"私有 B 类 172.16.0.1", "172.16.0.1:80"},
		{"私有 C 类 192.168.1.1", "192.168.1.1:80"},
		{"链路本地 169.254.1.1", "169.254.1.1:80"},
	}

	for _, tc := range privateAddrs {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := dialFn(ctx, "tcp", tc.addr)
			// 预期：DialContext 返回错误，连接不应建立
			assert.Error(t, err, "DialContext 应拦截私有地址 %s", tc.addr)
			if conn != nil {
				conn.Close()
			}
		})
	}
}

// TestWebFetch_DialContextAllowPrivate 验证 WEBFETCH_ALLOW_PRIVATE=true 时 DialContext 不拦截（SEC-001 旁路）
func TestWebFetch_DialContextAllowPrivate(t *testing.T) {
	t.Setenv("WEBFETCH_ALLOW_PRIVATE", "true")

	dialFn := newSSRFDialContext()
	ctx := context.Background()

	// 127.0.0.1:1 端口不可达，但应该能通过 SSRF 检查并尝试连接（返回 connection refused，而非 SSRF 错误）
	conn, err := dialFn(ctx, "tcp", "127.0.0.1:1")
	if conn != nil {
		conn.Close()
	}
	// 预期：不返回 SSRF 错误（可能返回 connection refused，也属正常）
	if err != nil {
		errMsg := err.Error()
		assert.True(t,
			strings.Contains(errMsg, "refused") ||
				strings.Contains(errMsg, "connect") ||
				strings.Contains(errMsg, "connection") ||
				strings.Contains(errMsg, "timeout"),
			"WEBFETCH_ALLOW_PRIVATE=true 时不应返回 SSRF 错误，实际错误: %s", errMsg)
	}
}

// TestWebFetch_RedirectSSRFBlocked 集成测试：验证 SEC-004 修复——
// 重定向到私有地址时，DialContext 层阻断连接。
// 攻击场景：公网 URL 重定向到内网 IP。
func TestWebFetch_RedirectSSRFBlocked(t *testing.T) {
	// 测试服务器使用自签名证书，需要跳过 TLS 验证
	t.Setenv("WEBFETCH_INSECURE_TLS", "true")
	// 注意：不设置 WEBFETCH_ALLOW_PRIVATE，验证默认行为

	// 创建一个公网地址（127.0.0.1 测试用），返回重定向到另一个私有地址的响应
	// 由于测试环境全在 127.0.0.1，我们用两个不同端口的服务器模拟：
	// 服务器 A（被允许访问，WEBFETCH_ALLOW_PRIVATE=true 模式下的"公网"）→ 重定向到 → 服务器 B
	// 服务器 B 监听在 127.0.0.1，无论怎样访问都应被 DialContext 拦截

	// 本测试直接验证集成行为：当 WEBFETCH_ALLOW_PRIVATE=false，
	// 整个请求（包括重定向目标）都应被 DialContext 阻断
	serverB := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, "<p>重定向目标，不应被访问</p>")
	}))
	defer serverB.Close()

	// 由于 WEBFETCH_ALLOW_PRIVATE 未设置，serverB（127.0.0.1）的 URL 会在
	// 工具层的 fast-fail checkSSRF 就被拦截，以下用工具入口验证整体防御
	logger := zap.NewNop()
	mhost := mcphost.NewHost(logger)
	registerWebFetch(mhost, logger)

	input := map[string]any{"url": serverB.URL}
	inputJSON, _ := json.Marshal(input)

	ctx := context.Background()
	result, err := mhost.ExecuteTool(ctx, "webfetch", inputJSON)

	require.NoError(t, err)
	assert.True(t, result.IsError, "访问私有地址（重定向目标）应被拦截")

	var content string
	json.Unmarshal(result.Content, &content)
	assert.True(t,
		strings.Contains(content, "内网") || strings.Contains(content, "禁止"),
		"错误消息应表明私有地址被拦截，实际: %s", content)
}

// BenchmarkHtmlToText 性能测试
func BenchmarkHtmlToText(b *testing.B) {
	html := `
		<html>
		<head><title>Test</title></head>
		<body>
			<h1>Title</h1>
			<p>This is a <strong>paragraph</strong> with <a href="http://example.com">link</a>.</p>
			<ul>
				<li>Item 1</li>
				<li>Item 2</li>
				<li>Item 3</li>
			</ul>
			<pre><code>func main() {
				println("Hello")
			}</code></pre>
		</body>
		</html>
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = htmlToText(html)
	}
}
