// Package search 提供可插拔的代码搜索引擎接口和实现。
package search

import "context"

// GlobEngine 文件模式匹配引擎接口。
type GlobEngine interface {
	Glob(ctx context.Context, pattern string, root string) ([]string, error)
}

// GrepEngine 内容搜索引擎接口。
type GrepEngine interface {
	Grep(ctx context.Context, req GrepRequest) (*GrepResult, error)
}

// GrepRequest 搜索请求参数。
type GrepRequest struct {
	Pattern    string // 正则表达式搜索模式
	Path       string // 搜索文件或目录
	GlobFilter string // 文件过滤模式（如 *.go）
	TypeFilter string // 按文件类型过滤（如 go、ts、py）
	Context    int    // 前后上下文行数（-C）
	Before     int    // 匹配前上下文行数（-B）
	After      int    // 匹配后上下文行数（-A）
	MaxResults int    // 每个文件的最大匹配数（0 表示不限制）
	Multiline  bool   // 跨行匹配
}

// GrepResult 搜索结果。
type GrepResult struct {
	Matches []GrepMatch
	Total   int
}

// GrepMatch 单条匹配结果。
type GrepMatch struct {
	File    string // 文件路径
	Line    int    // 行号
	Content string // 匹配内容（含上下文）
}
