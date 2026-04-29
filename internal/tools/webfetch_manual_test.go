//go:build manual
// +build manual

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// TestWebFetchManual 手动测试 webfetch 功能
// 运行方式: go test -tags=manual -v -run TestWebFetchManual ./internal/tools/
func TestWebFetchManual(t *testing.T) {
	// 创建一个简单的 HTML 服务器
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := `
		<!DOCTYPE html>
		<html>
		<head><title>Test Page</title></head>
		<body>
			<h1>欢迎</h1>
			<p>这是一个<strong>测试</strong>页面。</p>
			<ul>
				<li>项目 1</li>
				<li>项目 2</li>
			</ul>
			<a href="https://example.com">链接</a>
		</body>
		</html>
		`
		fmt.Fprintln(w, html)
	}))
	defer server.Close()

	// 创建 MCP Host
	logger := zap.NewDevelopment()
	host := mcphost.NewHost(logger)
	registerWebFetch(host, logger)

	// 测试 1: 成功获取
	fmt.Println("\n=== 测试 1: 成功获取 HTML ===")
	input := map[string]any{
		"url": server.URL,
	}
	inputJSON, _ := json.Marshal(input)

	ctx := context.Background()
	result, err := host.ExecuteTool(ctx, "webfetch", inputJSON)

	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if result.IsError {
		var errMsg string
		json.Unmarshal(result.Content, &errMsg)
		t.Fatalf("返回错误: %s", errMsg)
	}

	var content string
	json.Unmarshal(result.Content, &content)
	fmt.Println("获取的内容:")
	fmt.Println(content)
	fmt.Println()

	// 验证内容
	if len(content) == 0 {
		t.Fatal("内容为空")
	}

	// 测试 2: HTTP URL 自动升级为 HTTPS
	fmt.Println("\n=== 测试 2: URL 规范化 ===")
	normalized, err := normalizeURL("http://example.com/path")
	if err != nil {
		t.Fatalf("规范化失败: %v", err)
	}
	fmt.Printf("原始: http://example.com/path\n")
	fmt.Printf("规范化: %s\n", normalized)
	if normalized != "https://example.com/path" {
		t.Fatalf("URL 规范化错误: 期望 https://example.com/path, 实际 %s", normalized)
	}
	fmt.Println()

	// 测试 3: HTML 转文本
	fmt.Println("\n=== 测试 3: HTML 转文本 ===")
	htmlInput := "<h1>标题</h1><p>这是<strong>粗体</strong>文本。</p>"
	text, err := htmlToText(htmlInput)
	if err != nil {
		t.Fatalf("HTML 转换失败: %v", err)
	}
	fmt.Printf("HTML: %s\n", htmlInput)
	fmt.Printf("文本: %s\n", text)
	fmt.Println()

	// 测试 4: 域名过滤
	fmt.Println("\n=== 测试 4: 域名黑名单 ===")
	inputBlocked := map[string]any{
		"url":             server.URL,
		"blocked_domains": []string{"127.0.0.1"},
	}
	inputBlockedJSON, _ := json.Marshal(inputBlocked)

	resultBlocked, err := host.ExecuteTool(ctx, "webfetch", inputBlockedJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if !resultBlocked.IsError {
		t.Fatal("应该返回错误（域名在黑名单中）")
	}

	var blockedMsg string
	json.Unmarshal(resultBlocked.Content, &blockedMsg)
	fmt.Printf("黑名单错误: %s\n", blockedMsg)
	fmt.Println()

	fmt.Println("\n=== 所有测试通过 ===")
}
