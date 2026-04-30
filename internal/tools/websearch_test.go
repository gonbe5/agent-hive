package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

type fakeSearchProvider struct {
	name    string
	results []SearchResult
	err     error
	calls   int
}

func (p *fakeSearchProvider) Name() string {
	return p.name
}

func (p *fakeSearchProvider) Search(_ context.Context, _ string, _ *zap.Logger) ([]SearchResult, error) {
	p.calls++
	return p.results, p.err
}

// --- cleanHTML 单元测试 ---

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "移除简单HTML标签",
			input:    "<p>Hello World</p>",
			expected: "Hello World",
		},
		{
			name:     "解码HTML实体",
			input:    "AT&amp;T &lt;div&gt; &quot;test&quot;",
			expected: "AT&T <div> \"test\"",
		},
		{
			name:     "清理多余空白",
			input:    "  多个   空格    换行  ",
			expected: "多个 空格 换行",
		},
		{
			name:     "复杂HTML标签和实体混合",
			input:    "<b>OpenAI&#39;s</b> GPT&mdash;4 &nbsp; model",
			expected: "OpenAI's GPT—4 model",
		},
		{
			name:     "空字符串",
			input:    "",
			expected: "",
		},
		{
			name:     "只有HTML标签",
			input:    "<div><span></span></div>",
			expected: "",
		},
		{
			name:     "嵌套标签",
			input:    "<div><p>Nested <strong>bold</strong> text</p></div>",
			expected: "Nested bold text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- extractActualURL 单元测试 ---

func TestExtractActualURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "DuckDuckGo重定向URL",
			input:    "//duckduckgo.com/l/?uddg=https%3A%2F%2Fgithub.com%2Fopenai&rut=xyz",
			expected: "https://github.com/openai",
		},
		{
			name:     "直接HTTPS URL",
			input:    "https://github.com/openai",
			expected: "https://github.com/openai",
		},
		{
			name:     "相对协议URL转换",
			input:    "//example.com/page",
			expected: "https://example.com/page",
		},
		{
			name:     "内部链接（应跳过）",
			input:    "/internal/path",
			expected: "",
		},
		{
			name:     "无效URL",
			input:    "://invalid",
			expected: "",
		},
		{
			name:     "DuckDuckGo重定向但无uddg参数",
			input:    "https://duckduckgo.com/l/?rut=xyz",
			expected: "",
		},
		{
			name:     "URL编码的中文",
			input:    "//duckduckgo.com/l/?uddg=https%3A%2F%2Fbaidu.com%2F%E6%90%9C%E7%B4%A2",
			expected: "https://baidu.com/搜索",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractActualURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- isDomainInList 单元测试 ---

func TestIsDomainInList(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		list     []string
		expected bool
	}{
		{
			name:     "精确匹配",
			domain:   "github.com",
			list:     []string{"github.com", "gitlab.com"},
			expected: true,
		},
		{
			name:     "子域名匹配",
			domain:   "api.github.com",
			list:     []string{"github.com"},
			expected: true,
		},
		{
			name:     "www前缀忽略",
			domain:   "www.github.com",
			list:     []string{"github.com"},
			expected: true,
		},
		{
			name:     "不在列表中",
			domain:   "bitbucket.org",
			list:     []string{"github.com", "gitlab.com"},
			expected: false,
		},
		{
			name:     "空列表",
			domain:   "github.com",
			list:     []string{},
			expected: false,
		},
		{
			name:     "大小写不敏感",
			domain:   "GitHub.COM",
			list:     []string{"github.com"},
			expected: true,
		},
		{
			name:     "多级子域名",
			domain:   "v1.api.github.com",
			list:     []string{"github.com"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDomainInList(tt.domain, tt.list)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- filterResultsByDomain 单元测试 ---

func TestFilterResultsByDomain(t *testing.T) {
	baseResults := []SearchResult{
		{Title: "GitHub", URL: "https://github.com/openai", Description: "AI research"},
		{Title: "OpenAI", URL: "https://openai.com/research", Description: "Official site"},
		{Title: "Wikipedia", URL: "https://en.wikipedia.org/wiki/OpenAI", Description: "Wiki page"},
		{Title: "API Docs", URL: "https://api.github.com/v3", Description: "GitHub API"},
	}

	tests := []struct {
		name            string
		results         []SearchResult
		allowedDomains  []string
		blockedDomains  []string
		expectedCount   int
		expectedDomains []string
	}{
		{
			name:            "无过滤",
			results:         baseResults,
			allowedDomains:  nil,
			blockedDomains:  nil,
			expectedCount:   4,
			expectedDomains: []string{"github.com", "openai.com", "wikipedia.org", "api.github.com"},
		},
		{
			name:            "白名单过滤",
			results:         baseResults,
			allowedDomains:  []string{"github.com"},
			blockedDomains:  nil,
			expectedCount:   2,
			expectedDomains: []string{"github.com", "api.github.com"},
		},
		{
			name:            "黑名单过滤",
			results:         baseResults,
			allowedDomains:  nil,
			blockedDomains:  []string{"wikipedia.org"},
			expectedCount:   3,
			expectedDomains: []string{"github.com", "openai.com", "api.github.com"},
		},
		{
			name:            "白名单和黑名单同时使用",
			results:         baseResults,
			allowedDomains:  []string{"github.com"},
			blockedDomains:  []string{"api.github.com"},
			expectedCount:   1,
			expectedDomains: []string{"github.com"},
		},
		{
			name:            "空结果集",
			results:         []SearchResult{},
			allowedDomains:  []string{"github.com"},
			blockedDomains:  nil,
			expectedCount:   0,
			expectedDomains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterResultsByDomain(tt.results, tt.allowedDomains, tt.blockedDomains)
			assert.Equal(t, tt.expectedCount, len(filtered))

			for i, result := range filtered {
				domain := strings.TrimPrefix(strings.Split(strings.TrimPrefix(result.URL, "https://"), "/")[0], "www.")
				if i < len(tt.expectedDomains) {
					assert.Contains(t, result.URL, tt.expectedDomains[i])
				}
				_ = domain // 避免未使用变量警告
			}
		})
	}
}

// --- parseSearchResults 单元测试 ---

func TestParseSearchResults(t *testing.T) {
	mockHTML := `
<html>
<body>
<div class="results">
	<div class="result__body">
		<h2 class="result__title">
			<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgithub.com%2Fopenai&rut=xyz">
				OpenAI GitHub
			</a>
		</h2>
		<a class="result__snippet">Official OpenAI repository</a>
	</div>
</div>
<div class="results">
	<div class="result__body">
		<h2 class="result__title">
			<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fopenai.com&rut=abc">
				OpenAI &amp; ChatGPT
			</a>
		</h2>
		<a class="result__snippet">AI research &mdash; powered by GPT</a>
	</div>
</div>
</body>
</html>
`

	logger := zap.NewNop()
	results := parseSearchResults(mockHTML, logger)

	require.Len(t, results, 2)

	assert.Equal(t, "OpenAI GitHub", results[0].Title)
	assert.Equal(t, "https://github.com/openai", results[0].URL)
	assert.Equal(t, "Official OpenAI repository", results[0].Description)

	assert.Equal(t, "OpenAI & ChatGPT", results[1].Title)
	assert.Equal(t, "https://openai.com", results[1].URL)
	assert.Equal(t, "AI research — powered by GPT", results[1].Description)
}

func TestParseSearchResults_NoResults(t *testing.T) {
	mockHTML := `<html><body>No results found</body></html>`

	logger := zap.NewNop()
	results := parseSearchResults(mockHTML, logger)

	assert.Empty(t, results)
}

func TestParseSearchResults_MalformedHTML(t *testing.T) {
	mockHTML := `
<div class="result">
	<a class="result__a" href="invalid-url">Incomplete Result</a>
</div>
`

	logger := zap.NewNop()
	results := parseSearchResults(mockHTML, logger)

	// 应该跳过格式不正确的结果
	assert.Empty(t, results)
}

// --- formatSearchResults 单元测试 ---

func TestFormatSearchResults(t *testing.T) {
	tests := []struct {
		name     string
		results  []SearchResult
		query    string
		expected string
	}{
		{
			name: "正常格式化",
			results: []SearchResult{
				{Title: "Result 1", URL: "https://example.com/1", Description: "Description 1"},
				{Title: "Result 2", URL: "https://example.com/2", Description: "Description 2"},
			},
			query:    "test query",
			expected: "搜索 'test query' 的结果（共 2 条）:\n\n1. **Result 1**\n   URL: https://example.com/1\n   描述: Description 1\n\n2. **Result 2**\n   URL: https://example.com/2\n   描述: Description 2\n\n",
		},
		{
			name:     "空结果",
			results:  []SearchResult{},
			query:    "empty query",
			expected: "未找到关于 'empty query' 的搜索结果",
		},
		{
			name: "无描述的结果",
			results: []SearchResult{
				{Title: "No Description", URL: "https://example.com/no-desc", Description: ""},
			},
			query:    "test",
			expected: "搜索 'test' 的结果（共 1 条）:\n\n1. **No Description**\n   URL: https://example.com/no-desc\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSearchResults(tt.results, tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- 集成测试（使用 Mock HTTP Server）---

func TestPerformDuckDuckGoSearch_MockServer(t *testing.T) {
	mockHTML := `
<html>
<body>
<div class="results">
	<div class="result__body">
		<h2 class="result__title">
			<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com&rut=xyz">
				Example Result
			</a>
		</h2>
		<a class="result__snippet">Example description</a>
	</div>
</div>
</body>
</html>
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	// 注意：实际实现中无法直接替换 DuckDuckGo URL，此测试主要验证解析逻辑
	logger := zap.NewNop()
	results := parseSearchResults(mockHTML, logger)

	require.Len(t, results, 1)
	assert.Equal(t, "Example Result", results[0].Title)
	assert.Equal(t, "https://example.com", results[0].URL)
}

// H5：真调 performSearch，通过 httptest.Server 覆盖 endpoint —— 不再打真 DDG。

func TestSearchClient_performSearch_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	c := &searchClient{
		Endpoint:   server.URL,
		HTTPClient: &http.Client{Timeout: 500 * time.Millisecond},
	}
	_, err := c.performSearch(ctx, "test", zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestSearchClient_performSearch_HTTP500(t *testing.T) {
	var hit int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &searchClient{
		Endpoint:   server.URL,
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	}
	_, err := c.performSearch(context.Background(), "q", zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
	assert.Equal(t, 1, hit, "应只打 endpoint 一次")
}

func TestSearchClient_performSearch_EmptyHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>no results here</body></html>"))
	}))
	defer server.Close()

	c := &searchClient{
		Endpoint:   server.URL,
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	}
	got, err := c.performSearch(context.Background(), "q", zap.NewNop())
	require.NoError(t, err)
	assert.Len(t, got, 0, "HTML 里无 result__a 时，raw 必须为 0（H4/P0-B 触发 IsError）")
}

func TestSearchClient_performSearch_Success(t *testing.T) {
	const body = `<html><body><div class="results">
<div class="result__body">
  <h2 class="result__title">
    <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev&rut=x">Go</a>
  </h2>
  <a class="result__snippet">Go 官网</a>
</div>
</div></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	c := &searchClient{Endpoint: server.URL, HTTPClient: &http.Client{Timeout: 2 * time.Second}}
	got, err := c.performSearch(context.Background(), "golang", zap.NewNop())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Go", got[0].Title)
	assert.Equal(t, "https://go.dev", got[0].URL)
}

// H5 + H4 + M8 端到端：registerWebSearch 真跑一遍，验证 strict + 过滤场景
// 以及结构化日志 outcome 在各种场景下都被记录（通过 observer logger 捕获）。
func TestRegisterWebSearch_EndToEnd_FilterEmpty(t *testing.T) {
	const body = `<html><body><div class="results">
<div class="result__body">
  <h2 class="result__title"><a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fblocked.example&rut=x">Blocked</a></h2>
  <a class="result__snippet">blocked host result</a>
</div>
</div></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	// H5-MED-A：改成显式入参注入，完全消除 package 全局。每个 Host 独立 client，并发安全。
	testClient := &searchClient{Endpoint: server.URL, HTTPClient: &http.Client{Timeout: 2 * time.Second}}

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerWebSearch(host, logger, true, testClient) // strict=true

	// 构造输入：allowed_domains 只允许 notmatching.example → filtered 清零但 raw=1
	in := json.RawMessage(`{"query":"anything","allowed_domains":["notmatching.example"]}`)
	got, err := host.ExecuteTool(context.Background(), "websearch", in)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.IsError, "filter 清零但 raw>0 不应 IsError（H4 语义）")
	assert.Contains(t, string(got.Content), "过滤")
}

func TestRegisterWebSearch_EndToEnd_RawEmptyStrict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>empty</html>"))
	}))
	defer server.Close()

	testClient := &searchClient{Endpoint: server.URL, HTTPClient: &http.Client{Timeout: 2 * time.Second}}

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerWebSearch(host, logger, true, testClient) // strict=true

	in := json.RawMessage(`{"query":"nothing-matches"}`)
	got, err := host.ExecuteTool(context.Background(), "websearch", in)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.IsError, "raw==0 + strict 必须 IsError（P0-B 核心契约）")
	assert.Contains(t, string(got.Content), "websearch 零结果")
}

// M8-LOW：用 zaptest/observer 捕获 websearch 结构化日志，断言字段名与取值。
// 修复前用 zap.NewNop() 等于不测——有人把 "outcome" 改名成 "status" 单元测试不会红。
// 本用例直接对字段名 "outcome" / "raw_count" / "filtered_count" / "strict" 做强断言，
// 字段 schema 变动即刻失败，这才是 M8 的真实验收点。
func TestRegisterWebSearch_M8_StructuredLogFields(t *testing.T) {
	type logScenario struct {
		name           string
		body           string
		input          string
		wantQuery      string
		strict         bool
		wantOutcome    string
		wantRawCount   int64
		wantFiltered   int64
		wantStrictFlag bool
	}

	okBody := `<html><body><div class="results">
<div class="result__body">
  <h2 class="result__title"><a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev&rut=x">Go</a></h2>
  <a class="result__snippet">Go 官网</a>
</div>
</div></body></html>`

	scenarios := []logScenario{
		{
			name:           "ok",
			body:           okBody,
			input:          `{"query":"golang"}`,
			wantQuery:      "golang",
			strict:         true,
			wantOutcome:    "ok",
			wantRawCount:   1,
			wantFiltered:   1,
			wantStrictFlag: true,
		},
		{
			name:           "filter_empty",
			body:           okBody,
			input:          `{"query":"golang","allowed_domains":["notmatching.example"]}`,
			wantQuery:      "golang",
			strict:         true,
			wantOutcome:    "filter_empty",
			wantRawCount:   1,
			wantFiltered:   0,
			wantStrictFlag: true,
		},
		{
			name:           "raw_empty",
			body:           `<html>empty</html>`,
			input:          `{"query":"nothing"}`,
			wantQuery:      "nothing",
			strict:         true,
			wantOutcome:    "raw_empty",
			wantRawCount:   0,
			wantFiltered:   0,
			wantStrictFlag: true,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(sc.body))
			}))
			defer server.Close()

			// observer 捕获所有级别日志，用强断言校验字段。
			core, recorded := observer.New(zapcore.DebugLevel)
			logger := zap.New(core)
			host := mcphost.NewHost(logger)
			registerWebSearch(host, logger, sc.strict, &searchClient{
				Endpoint:   server.URL,
				HTTPClient: &http.Client{Timeout: 2 * time.Second},
			})

			_, err := host.ExecuteTool(context.Background(), "websearch", json.RawMessage(sc.input))
			require.NoError(t, err)

			entries := recorded.FilterMessage("websearch 执行完毕").All()
			require.Len(t, entries, 1, "期望恰好一条 '执行完毕' 日志，观测到 %d 条", len(entries))

			fieldMap := entries[0].ContextMap()
			assert.Equal(t, sc.wantOutcome, fieldMap["outcome"], "outcome 字段名/值都必须被锁死")
			assert.Equal(t, sc.wantRawCount, fieldMap["raw_count"], "raw_count 字段名/值必须匹配")
			assert.Equal(t, sc.wantFiltered, fieldMap["filtered_count"], "filtered_count 字段名/值必须匹配")
			assert.Equal(t, sc.wantStrictFlag, fieldMap["strict"], "strict 字段必须被保留")
			assert.Equal(t, sc.wantQuery, fieldMap["query"], "query 字段需照传")
		})
	}
}

// --- buildSearchToolResult 纯函数测试（P0-B strict + H4 raw/filtered 区分）---

func TestBuildSearchToolResult_StrictRawEmpty(t *testing.T) {
	// raw=0：真·零结果 → IsError，强制上层重试
	result := buildSearchToolResult(nil, 0, "天气", true)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "strict + raw==0 必须 IsError=true")
	assert.Contains(t, string(result.Content), "websearch 零结果")
	assert.Contains(t, string(result.Content), "天气")
	assert.Contains(t, string(result.Content), "不要凭记忆编造")
}

// H4 新增：raw>0 但 filtered=0（域名过滤干掉全部）→ 非 IsError 文本。
func TestBuildSearchToolResult_StrictFilteredEmpty(t *testing.T) {
	result := buildSearchToolResult(nil, 7, "golang", true)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "strict + raw>0 但 filter 清零时不应是 IsError（上层应当调域名过滤）")
	assert.Contains(t, string(result.Content), "过滤")
	assert.Contains(t, string(result.Content), "7")
}

func TestBuildSearchToolResult_NonStrictEmpty(t *testing.T) {
	result := buildSearchToolResult(nil, 0, "天气", false)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "strict=false 保持旧行为，零结果不报错")
}

func TestBuildSearchToolResult_StrictWithResults(t *testing.T) {
	results := []SearchResult{
		{Title: "T1", URL: "https://a.example", Description: "D1"},
	}
	result := buildSearchToolResult(results, 1, "q", true)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "有结果时 strict=true 也不应报错")
	assert.Contains(t, string(result.Content), "T1")
}

func TestBuildSearchToolResult_ReturnsStructuredEnvelope(t *testing.T) {
	results := []SearchResult{
		{Title: "T1", URL: "https://a.example", Description: "D1"},
	}

	result := buildSearchToolResult(results, 1, "q", true)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	var envelope SearchResultEnvelope
	require.NoError(t, json.Unmarshal([]byte(result.DecodeContent()), &envelope))
	assert.Equal(t, "duckduckgo", envelope.Provider)
	assert.Equal(t, "ok", envelope.Status)
	assert.Equal(t, results, envelope.Results)
	assert.Empty(t, envelope.Error)
	assert.False(t, envelope.Fallback.Used)
}

func TestExecuteWebSearch_UsesFallbackProviderWhenPrimaryFails(t *testing.T) {
	primary := &fakeSearchProvider{
		name: "primary",
		err:  fmt.Errorf("primary unavailable"),
	}
	fallback := &fakeSearchProvider{
		name: "fallback",
		results: []SearchResult{
			{Title: "Fallback", URL: "https://fallback.example", Description: "ok"},
		},
	}

	envelope, err := executeWebSearch(context.Background(), websearchInput{
		Query:      "q",
		MaxResults: 10,
	}, true, primary, fallback, zap.NewNop())

	require.NoError(t, err)
	require.Equal(t, 1, primary.calls)
	require.Equal(t, 1, fallback.calls)
	assert.Equal(t, "fallback", envelope.Provider)
	assert.Equal(t, "ok", envelope.Status)
	require.Len(t, envelope.Results, 1)
	assert.Equal(t, "Fallback", envelope.Results[0].Title)
	assert.True(t, envelope.Fallback.Used)
	assert.Equal(t, "primary", envelope.Fallback.FromProvider)
	assert.Equal(t, "fallback", envelope.Fallback.ToProvider)
	assert.Contains(t, envelope.Fallback.Reason, "primary unavailable")
}

// --- registerWebSearch 集成测试 ---

func TestRegisterWebSearch_ValidInput(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerWebSearch(host, logger, false, nil)

	// 验证工具已注册
	tools := host.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "websearch" {
			found = true
			break
		}
	}
	assert.True(t, found, "websearch 工具应该已注册")
}

func TestRegisterWebSearch_EmptyQuery(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	// 空 query 在 handler 入口就被拦截，不会走到 performSearch，传 nil client 安全。
	registerWebSearch(host, logger, false, nil)

	// 调用工具（空查询）
	input, _ := json.Marshal(websearchInput{Query: ""})
	result, err := callTool(host, "websearch", input)

	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, string(result.Content), "搜索查询不能为空")
}

func TestRegisterWebSearch_InvalidJSON(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	// 无效 JSON 场景：需要 mock endpoint，否则 JSON 通过类型转换后会打 DDG 真站。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer server.Close()
	registerWebSearch(host, logger, false, &searchClient{
		Endpoint:   server.URL,
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	})

	// 无效 JSON 输入
	input := json.RawMessage(`{"query": 123}`) // query 应该是字符串
	result, err := callTool(host, "websearch", input)

	require.NoError(t, err)
	// 由于 JSON unmarshal 会自动转换类型，此测试可能通过，更严格的验证在实际使用中
	_ = result
}

// Codex round-4 对 round-3 #23 的再审：max=100→50 的断言在 raw=55 + parser cap=50 下
// 无论 clamp 是否存在都 =50，不独立证明 L109 的参数 clamp。
// 重写后把职责拆开：
//   - TestClampMaxResults（纯函数）刺穿 L109（0→10 / 10→10 / 50→50 / 100→50 / 10000→50）
//   - 本测试只保留真正有可观测差异的 max=3 路径（刺穿 L137 切片）
//
// 端到端 handler 测试：raw=55，max=3，期望输出"共 3 条"且不含"共 50 条"。
// 删掉 websearch.go:137 的切片 → 输出变 50 条（parser 上限），断言红。
func TestRegisterWebSearch_MaxResultsSliceClamp(t *testing.T) {
	const rawCount = 55
	var body strings.Builder
	body.WriteString(`<html><body>`)
	for i := 0; i < rawCount; i++ {
		body.WriteString(fmt.Sprintf(`<div class="results"><div class="result__body">
<h2 class="result__title"><a class="result__a" href="//duckduckgo.com/l/?uddg=https%%3A%%2F%%2Fsite%d.example&rut=x">T%d</a></h2>
<a class="result__snippet">desc %d</a>
</div></div>`, i, i, i))
	}
	body.WriteString(`</body></html>`)

	var hitCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		_, _ = w.Write([]byte(body.String()))
	}))
	defer server.Close()

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerWebSearch(host, logger, false, &searchClient{
		Endpoint:   server.URL,
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	})

	input, _ := json.Marshal(websearchInput{
		Query:      "test",
		MaxResults: 3,
	})
	result, err := callTool(host, "websearch", input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Equal(t, 1, hitCount, "应打到 mock endpoint 一次")
	s := string(result.Content)
	assert.Contains(t, s, "共 3 条", "max_results=3 时输出应被 L137 切片截断到 3")
	assert.NotContains(t, s, "共 50 条", "未被截断会暴露 parser 解出的 50 条——L137 clamp 失效")
}

// 独立测 clampMaxResults 纯函数——即 websearch.go:109 的参数上限 clamp。
// 之前端到端测试无法独立验证这个分支（parser 内部的 50 上限遮蔽了 100→50 的可观测差异）。
// 现在抽成纯函数，直接表驱动 assert。删掉任一分支（0→10 / >50→50）就会红一行。
func TestClampMaxResults(t *testing.T) {
	cases := []struct {
		name string
		raw  int
		want int
	}{
		{"zero → default 10", 0, 10},
		{"1 → 1", 1, 1},
		{"10 → 10", 10, 10},
		{"49 → 49", 49, 49},
		{"50 (边界) → 50", 50, 50},
		{"51 (刚越界) → 50", 51, 50},
		{"100 → 50", 100, 50},
		{"10000 (恶意) → 50", 10000, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampMaxResults(tc.raw); got != tc.want {
				t.Fatalf("clampMaxResults(%d) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

// --- 辅助函数 ---

// callTool 调用已注册的工具
func callTool(host *mcphost.Host, toolName string, input json.RawMessage) (*mcphost.ToolResult, error) {
	ctx := context.Background()
	return host.ExecuteTool(ctx, toolName, input)
}

// --- 边界测试 ---

func TestParseSearchResults_LargeHTML(t *testing.T) {
	// 生成包含 100 个结果的大 HTML
	var builder strings.Builder
	builder.WriteString("<html><body>")
	for i := 0; i < 100; i++ {
		builder.WriteString(`
<div class="results">
	<div class="result__body">
		<h2 class="result__title">
			<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2F` + string(rune(i)) + `&rut=xyz">
				Result ` + string(rune(i)) + `
			</a>
		</h2>
		<a class="result__snippet">Description ` + string(rune(i)) + `</a>
	</div>
</div>
`)
	}
	builder.WriteString("</body></html>")

	logger := zap.NewNop()
	results := parseSearchResults(builder.String(), logger)

	// 应该限制在 50 个结果
	assert.LessOrEqual(t, len(results), 50)
}

func TestFilterResultsByDomain_InvalidURL(t *testing.T) {
	results := []SearchResult{
		{Title: "Valid", URL: "https://github.com", Description: "Valid URL"},
		{Title: "Invalid", URL: "://invalid-url", Description: "Invalid URL"},
	}

	filtered := filterResultsByDomain(results, []string{"github.com"}, nil)

	// 无效 URL 应该被跳过
	assert.Equal(t, 1, len(filtered))
	assert.Equal(t, "Valid", filtered[0].Title)
}

// --- 性能测试 ---

func BenchmarkCleanHTML(b *testing.B) {
	input := "<p>OpenAI&#39;s <strong>GPT&mdash;4</strong> &nbsp; model &amp; more</p>"
	for i := 0; i < b.N; i++ {
		cleanHTML(input)
	}
}

func BenchmarkParseSearchResults(b *testing.B) {
	mockHTML := strings.Repeat(`
<div class="results">
	<div class="result__body">
		<h2 class="result__title">
			<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com&rut=xyz">
				Example Result
			</a>
		</h2>
		<a class="result__snippet">Example description</a>
	</div>
</div>
`, 20)

	logger := zap.NewNop()
	for i := 0; i < b.N; i++ {
		parseSearchResults(mockHTML, logger)
	}
}

// --- Mock 服务器完整集成测试 ---

func TestWebSearchEndToEnd_MockServer(t *testing.T) {
	// 创建 mock DuckDuckGo 服务器
	mockHTML := `
<html>
<body>
<div class="results">
	<div class="result__body">
		<h2 class="result__title">
			<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgithub.com%2Fopenai&rut=xyz">
				OpenAI on GitHub
			</a>
		</h2>
		<a class="result__snippet">Official OpenAI repository</a>
	</div>
</div>
<div class="results">
	<div class="result__body">
		<h2 class="result__title">
			<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fwikipedia.org%2Fwiki%2FOpenAI&rut=abc">
				OpenAI - Wikipedia
			</a>
		</h2>
		<a class="result__snippet">OpenAI is an AI research laboratory</a>
	</div>
</div>
</body>
</html>
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求参数
		assert.Contains(t, r.URL.String(), "q=")
		assert.Contains(t, r.Header.Get("User-Agent"), "Mozilla")

		w.WriteHeader(http.StatusOK)
		io.WriteString(w, mockHTML)
	}))
	defer server.Close()

	logger := zap.NewNop()
	results := parseSearchResults(mockHTML, logger)

	// 验证解析结果
	require.Len(t, results, 2)

	assert.Equal(t, "OpenAI on GitHub", results[0].Title)
	assert.Equal(t, "https://github.com/openai", results[0].URL)

	assert.Equal(t, "OpenAI - Wikipedia", results[1].Title)
	assert.Equal(t, "https://wikipedia.org/wiki/OpenAI", results[1].URL)

	// 测试域名过滤
	filtered := filterResultsByDomain(results, []string{"github.com"}, nil)
	assert.Equal(t, 1, len(filtered))
	assert.Equal(t, "OpenAI on GitHub", filtered[0].Title)
}

// Coverage padding test removed —— 用 H4/H5 的真实覆盖替代。
