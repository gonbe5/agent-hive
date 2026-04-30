package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// websearchInput 定义 websearch 工具的输入参数
type websearchInput struct {
	Query          string   `json:"query"`                     // 搜索查询
	AllowedDomains []string `json:"allowed_domains,omitempty"` // 允许的域名白名单
	BlockedDomains []string `json:"blocked_domains,omitempty"` // 禁止的域名黑名单
	MaxResults     int      `json:"max_results,omitempty"`     // 最大结果数（默认10，最多50）
}

// SearchResult 表示单个搜索结果
type SearchResult struct {
	Title       string `json:"title"`       // 结果标题
	URL         string `json:"url"`         // 结果URL
	Description string `json:"description"` // 结果描述
}

// SearchResultEnvelope 是 websearch 的结构化输出。
// Content 仍作为 JSON 字符串返回，保持旧调用方按文本读取的兼容性。
type SearchResultEnvelope struct {
	Provider string                 `json:"provider"`            // 实际提供结果的 provider
	Status   string                 `json:"status"`              // ok / no_results / filtered_empty / error
	Results  []SearchResult         `json:"results"`             // 过滤和截断后的结果
	Error    string                 `json:"error,omitempty"`     // 错误或零结果原因
	Fallback SearchFallbackEnvelope `json:"fallback"`            // fallback 执行信息
	Text     string                 `json:"text,omitempty"`      // 兼容旧文本消费方的人类可读输出
	RawCount int                    `json:"raw_count,omitempty"` // provider 原始结果数，用于观测和排障
}

// SearchFallbackEnvelope 记录 provider fallback 是否发生以及原因。
type SearchFallbackEnvelope struct {
	Used         bool   `json:"used"`
	FromProvider string `json:"from_provider,omitempty"`
	ToProvider   string `json:"to_provider,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type searchProvider interface {
	Name() string
	Search(ctx context.Context, query string, logger *zap.Logger) ([]SearchResult, error)
}

// searchClient 封装 DuckDuckGo HTTP 调用，用于在测试中替换 endpoint / transport。
// 生产路径通过 defaultSearchClient() 构造；测试路径用 httptest.Server 注入。
type searchClient struct {
	Endpoint   string
	HTTPClient *http.Client
	NameValue  string
}

// maxResultsHardCap 是 websearch 参数的硬上限。
// 与 parseSearchResults 内部的 50 条 break 上限保持一致，防止客户端传 10000 这种
// 导致下游 O(n) 遍历退化。
const maxResultsHardCap = 50

// clampMaxResults 实现 websearch 的 MaxResults 归一化：
//   - 0 (未传或显式 0) → 10（文档默认值，跟前端 UI 一致）
//   - >50 → 50（硬上限）
//   - 其它 → 原样
//
// 拆为纯函数是为了可独立单测 L112 的参数上限逻辑。之前这段内联在 handler 里，
// 测试不得不叠加 rawCount 去触发；但 parseSearchResults 自身有 50 条 break，
// 导致 MaxResults=100 vs MaxResults=50 在下游切片点无可观测差异，clamp 分支成了
// 测试空白。现在直接 clampMaxResults(100) 即可 assert 返回 50。
func clampMaxResults(raw int) int {
	if raw == 0 {
		return 10
	}
	if raw > maxResultsHardCap {
		return maxResultsHardCap
	}
	return raw
}

func defaultSearchClient() *searchClient {
	return &searchClient{
		Endpoint:   "https://html.duckduckgo.com/html/",
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		NameValue:  "duckduckgo",
	}
}

func (c *searchClient) Name() string {
	if c != nil && c.NameValue != "" {
		return c.NameValue
	}
	return "duckduckgo"
}

func (c *searchClient) Search(ctx context.Context, query string, logger *zap.Logger) ([]SearchResult, error) {
	return c.performSearch(ctx, query, logger)
}

// registerWebSearch 注册 websearch 工具。
// strictMode：P0-B 质量闸门。true 时，零结果走 IsError=true，强制上层重试而非生成幻觉；
// false 时保持旧行为（textResult("未找到 ...")）。
// client：H5-MED-A。旧实现用 package 全局 currentSearchClient 注入测试 endpoint，
// 多个 Host 并发注册会互相污染、产生数据 race。改成显式入参：
//   - 生产路径传 nil → 闭包捕获 defaultSearchClient()，每个 Host 独立实例。
//   - 测试路径传 httptest.Server 背后的 searchClient，随 Host 生命周期独立。
//
// 闭包捕获后执行时直接走 capturedClient，不再读全局。
func registerWebSearch(host *mcphost.Host, logger *zap.Logger, strictMode bool, client *searchClient) {
	capturedClient := client
	if capturedClient == nil {
		capturedClient = defaultSearchClient()
	}
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "搜索查询字符串",
			},
			"allowed_domains": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "只返回这些域名的结果（可选）",
			},
			"blocked_domains": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "排除这些域名的结果（可选）",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "最大结果数量（默认10，最多50）",
				"minimum":     1,
				"maximum":     50,
			},
		},
		"required": []string{"query"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "websearch",
			Description: "使用 DuckDuckGo 搜索网络内容，支持域名过滤",
			InputSchema: schema,
			Core:        true,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params websearchInput
			if err := json.Unmarshal(input, &params); err != nil {
				logger.Error("websearch 输入解析失败", zap.Error(err))
				return errorResult("输入无效: " + err.Error()), nil
			}

			// 验证查询字符串
			if strings.TrimSpace(params.Query) == "" {
				return errorResult("搜索查询不能为空"), nil
			}

			// 设置默认值 + 上限 clamp（纯函数，便于独立单测）
			params.MaxResults = clampMaxResults(params.MaxResults)

			// 创建带超时的 context（统一为 30s，与 HTTP 客户端超时一致）
			// 超时硬编码为 30s，后续可从配置项读取
			searchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			envelope, err := executeWebSearch(searchCtx, params, strictMode, capturedClient, nil, logger)
			if err != nil {
				// M8：输出结构化日志，outcome=http_error 便于告警过滤
				logger.Error("websearch 搜索失败",
					zap.Error(err),
					zap.String("query", params.Query),
					zap.Bool("strict", strictMode),
					zap.String("outcome", "http_error"))
				return errorResult("搜索失败: " + err.Error()), nil
			}

			// M8：统一结构化日志，outcome 枚举便于可观测性后端聚合。
			outcome := envelope.Status
			switch outcome {
			case "no_results":
				outcome = "raw_empty"
			case "filtered_empty":
				outcome = "filter_empty"
			}
			logLevel := logger.Info
			if outcome != "ok" {
				logLevel = logger.Warn
			}
			logLevel("websearch 执行完毕",
				zap.String("query", params.Query),
				zap.Int("raw_count", envelope.RawCount),
				zap.Int("filtered_count", len(envelope.Results)),
				zap.Bool("strict", strictMode),
				zap.String("outcome", outcome))

			return buildSearchToolResultFromEnvelope(envelope, strictMode), nil
		},
	)
}

func executeWebSearch(ctx context.Context, params websearchInput, strict bool, primary searchProvider, fallback searchProvider, logger *zap.Logger) (SearchResultEnvelope, error) {
	if primary == nil {
		primary = defaultSearchClient()
	}
	rawResults, providerName, fallbackInfo, err := searchWithFallback(ctx, params.Query, primary, fallback, logger)
	if err != nil {
		return SearchResultEnvelope{
			Provider: providerName,
			Status:   "error",
			Error:    err.Error(),
			Fallback: fallbackInfo,
			Text:     "搜索失败: " + err.Error(),
		}, err
	}

	rawCount := len(rawResults)
	filteredResults := filterResultsByDomain(rawResults, params.AllowedDomains, params.BlockedDomains)
	if len(filteredResults) > params.MaxResults {
		filteredResults = filteredResults[:params.MaxResults]
	}

	envelope := makeSearchResultEnvelope(providerName, filteredResults, rawCount, params.Query, strict)
	envelope.Fallback = fallbackInfo
	return envelope, nil
}

func searchWithFallback(ctx context.Context, query string, primary searchProvider, fallback searchProvider, logger *zap.Logger) ([]SearchResult, string, SearchFallbackEnvelope, error) {
	providerName := primary.Name()
	results, err := primary.Search(ctx, query, logger)
	if err == nil && len(results) > 0 {
		return results, providerName, SearchFallbackEnvelope{}, nil
	}

	if fallback == nil {
		return results, providerName, SearchFallbackEnvelope{}, err
	}

	reason := "primary returned no results"
	if err != nil {
		reason = err.Error()
	}
	fallbackInfo := SearchFallbackEnvelope{
		Used:         true,
		FromProvider: providerName,
		ToProvider:   fallback.Name(),
		Reason:       reason,
	}
	fallbackResults, fallbackErr := fallback.Search(ctx, query, logger)
	if fallbackErr != nil {
		if err != nil {
			return nil, providerName, fallbackInfo, fmt.Errorf("%s fallback failed: %w", err.Error(), fallbackErr)
		}
		return results, providerName, fallbackInfo, fallbackErr
	}
	if len(fallbackResults) == 0 {
		return results, providerName, fallbackInfo, err
	}
	return fallbackResults, fallback.Name(), fallbackInfo, nil
}

// buildSearchToolResult P0-B + H4：根据 raw/filtered 区分返回值。
// strict=true 且 raw==0：DDG 真的一条都没返回（反爬 / HTML 变更 / 空 query）→ IsError，强制重试。
// strict=true 且 raw>0 但 filtered==0：搜到了但被域名过滤干掉 → 非 IsError 文本，告诉上层是过滤问题。
// strict=false：一律 textResult（保持旧行为）。
func buildSearchToolResult(filtered []SearchResult, rawCount int, query string, strict bool) *mcphost.ToolResult {
	return buildSearchToolResultFromEnvelope(makeSearchResultEnvelope("duckduckgo", filtered, rawCount, query, strict), strict)
}

func makeSearchResultEnvelope(provider string, filtered []SearchResult, rawCount int, query string, strict bool) SearchResultEnvelope {
	envelope := SearchResultEnvelope{
		Provider: provider,
		Status:   "ok",
		Results:  filtered,
		Text:     formatSearchResults(filtered, query),
		RawCount: rawCount,
	}
	if strict && rawCount == 0 {
		envelope.Status = "no_results"
		envelope.Error = fmt.Sprintf(
			"websearch 零结果（query=%q）：DuckDuckGo HTTP 200 但解析后结果为 0。"+
				"可能原因：查询过窄 / DDG 反爬 / HTML 结构变更。请调整查询词后重试，不要凭记忆编造。",
			query,
		)
		envelope.Text = envelope.Error
		return envelope
	}
	if strict && len(filtered) == 0 {
		envelope.Status = "filtered_empty"
		envelope.Text = fmt.Sprintf(
			"搜索 '%s' 返回了 %d 条结果，但都被 allowed_domains / blocked_domains 过滤掉。"+
				"这是过滤配置问题而非搜索失败，请调整域名过滤后重试。",
			query, rawCount,
		)
	}
	return envelope
}

func buildSearchToolResultFromEnvelope(envelope SearchResultEnvelope, strict bool) *mcphost.ToolResult {
	data, _ := json.Marshal(envelope)
	result := textResult(string(data))
	result.IsError = strict && envelope.Status == "no_results"
	if envelope.Status == "error" {
		result.IsError = true
	}
	return result
}

// performSearch 在可注入的 endpoint 上执行 DuckDuckGo 风格搜索。
// H5：endpoint + http.Client 都从 receiver 读取，让测试可以用 httptest.Server 替换。
func (c *searchClient) performSearch(ctx context.Context, query string, logger *zap.Logger) ([]SearchResult, error) {
	if c.HTTPClient == nil {
		return nil, errs.New(errs.CodeMCPConnFailed, "searchClient 未设置 HTTPClient")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://html.duckduckgo.com/html/"
	}
	searchURL := endpoint
	if strings.Contains(endpoint, "?") {
		searchURL = endpoint + "&q=" + url.QueryEscape(query)
	} else {
		searchURL = endpoint + "?q=" + url.QueryEscape(query)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, errs.Wrap(errs.CodeMCPConnFailed, "创建请求失败", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeMCPConnFailed, "HTTP 请求失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errs.New(errs.CodeMCPConnFailed, fmt.Sprintf("搜索服务返回错误: HTTP %d", resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, errs.Wrap(errs.CodeMCPConnFailed, "读取响应失败", err)
	}

	return parseSearchResults(string(body), logger), nil
}

// parseSearchResults 从 DuckDuckGo HTML 页面解析搜索结果
func parseSearchResults(htmlContent string, logger *zap.Logger) []SearchResult {
	var results []SearchResult

	// DuckDuckGo HTML 搜索结果格式：
	// <div class="result__body">
	//   <h2 class="result__title">
	//     <a class="result__a" href="...">Title</a>
	//   </h2>
	//   <a class="result__snippet">Description</a>
	// </div>

	// 提取所有结果块
	resultBlockRegex := regexp.MustCompile(`(?s)<div[^>]*class="[^"]*result[^"]*"[^>]*>.*?</div>\s*</div>`)
	blocks := resultBlockRegex.FindAllString(htmlContent, -1)

	for _, block := range blocks {
		// 提取标题和URL（使用 (?s) 允许换行符）
		titleRegex := regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
		titleMatches := titleRegex.FindStringSubmatch(block)
		if len(titleMatches) < 3 {
			continue
		}

		rawURL := titleMatches[1]
		title := cleanHTML(titleMatches[2])

		// 提取描述（使用 (?s) 允许换行符）
		descRegex := regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
		descMatches := descRegex.FindStringSubmatch(block)
		description := ""
		if len(descMatches) >= 2 {
			description = cleanHTML(descMatches[1])
		}

		// 处理 DuckDuckGo 的重定向 URL
		// 格式: //duckduckgo.com/l/?uddg=<encoded_url>&rut=...
		actualURL := extractActualURL(rawURL)
		if actualURL == "" {
			logger.Debug("无法提取实际 URL", zap.String("rawURL", rawURL))
			continue
		}

		results = append(results, SearchResult{
			Title:       title,
			URL:         actualURL,
			Description: description,
		})

		// 限制解析数量（性能优化）
		if len(results) >= 50 {
			break
		}
	}

	return results
}

// extractActualURL 从 DuckDuckGo 重定向 URL 提取实际 URL
func extractActualURL(rawURL string) string {
	// 处理相对 URL
	if strings.HasPrefix(rawURL, "//") {
		rawURL = "https:" + rawURL
	} else if strings.HasPrefix(rawURL, "/") {
		return "" // 跳过内部链接
	}

	// 解析 URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	// 如果是 DuckDuckGo 重定向链接，提取实际 URL
	if strings.Contains(parsedURL.Host, "duckduckgo.com") {
		uddg := parsedURL.Query().Get("uddg")
		if uddg != "" {
			decoded, err := url.QueryUnescape(uddg)
			if err == nil {
				return decoded
			}
		}
		return ""
	}

	return rawURL
}

// cleanHTML 清理 HTML 标签和实体
func cleanHTML(text string) string {
	// 移除 HTML 标签
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	text = tagRegex.ReplaceAllString(text, "")

	// 解码常见 HTML 实体
	replacements := map[string]string{
		"&amp;":   "&",
		"&lt;":    "<",
		"&gt;":    ">",
		"&quot;":  "\"",
		"&#39;":   "'",
		"&nbsp;":  " ",
		"&mdash;": "—",
		"&ndash;": "–",
	}
	for entity, char := range replacements {
		text = strings.ReplaceAll(text, entity, char)
	}

	// 清理多余空白
	text = strings.TrimSpace(text)
	spaceRegex := regexp.MustCompile(`\s+`)
	text = spaceRegex.ReplaceAllString(text, " ")

	return text
}

// filterResultsByDomain 根据域名白名单/黑名单过滤结果
func filterResultsByDomain(results []SearchResult, allowedDomains, blockedDomains []string) []SearchResult {
	if len(allowedDomains) == 0 && len(blockedDomains) == 0 {
		return results
	}

	var filtered []SearchResult
	for _, result := range results {
		parsedURL, err := url.Parse(result.URL)
		if err != nil {
			continue
		}

		domain := strings.ToLower(parsedURL.Host)
		// 移除 www. 前缀
		domain = strings.TrimPrefix(domain, "www.")

		// 检查黑名单
		if len(blockedDomains) > 0 && isDomainInList(domain, blockedDomains) {
			continue
		}

		// 检查白名单
		if len(allowedDomains) > 0 && !isDomainInList(domain, allowedDomains) {
			continue
		}

		filtered = append(filtered, result)
	}

	return filtered
}

// isDomainInList 检查域名是否在列表中（支持子域名匹配）
func isDomainInList(domain string, list []string) bool {
	// 标准化输入域名
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "www.")

	for _, d := range list {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(d, "www.")

		// 精确匹配
		if domain == d {
			return true
		}

		// 子域名匹配（例如：github.com 匹配 api.github.com）
		if strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}

// formatSearchResults 格式化搜索结果为文本输出
func formatSearchResults(results []SearchResult, query string) string {
	if len(results) == 0 {
		return fmt.Sprintf("未找到关于 '%s' 的搜索结果", query)
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("搜索 '%s' 的结果（共 %d 条）:\n\n", query, len(results)))

	for i, result := range results {
		output.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, result.Title))
		output.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
		if result.Description != "" {
			output.WriteString(fmt.Sprintf("   描述: %s\n", result.Description))
		}
		output.WriteString("\n")
	}

	return output.String()
}
