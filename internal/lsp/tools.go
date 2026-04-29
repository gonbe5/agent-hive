package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// LSPToolInput LSP 工具通用输入参数
type LSPToolInput struct {
	FilePath string `json:"file_path"` // 文件路径（绝对路径）
	Line     int    `json:"line"`      // 行号（从 1 开始）
	Column   int    `json:"column"`    // 列号（从 1 开始）
}

// RenameInput 重命名工具输入
type RenameInput struct {
	LSPToolInput
	NewName string `json:"new_name"` // 新名称
}

// WorkspaceSymbolInput 工作区符号搜索输入
type WorkspaceSymbolInput struct {
	Query string `json:"query"` // 搜索关键词
}

// RegisterTools 注册所有 LSP 工具到 MCP host
func RegisterTools(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	// 1. lsp_goto_definition - 跳转到定义
	registerGotoDefinition(host, manager, logger)

	// 2. lsp_find_references - 查找所有引用
	registerFindReferences(host, manager, logger)

	// 3. lsp_hover - 获取悬停信息
	registerHover(host, manager, logger)

	// 4. lsp_rename - 重命名符号
	registerRename(host, manager, logger)

	// 5. lsp_code_action - 代码操作建议
	registerCodeAction(host, manager, logger)

	// 6. lsp_formatting - 格式化文档
	registerFormatting(host, manager, logger)

	// 7. lsp_document_symbol - 文档符号列表（大纲）
	registerDocumentSymbol(host, manager, logger)

	// 8. lsp_workspace_symbol - 工作区符号搜索
	registerWorkspaceSymbol(host, manager, logger)

	// 9. lsp_completion - 代码补全
	registerCompletion(host, manager, logger)

	logger.Info("LSP 工具已注册", zap.Int("count", 9))
}

// 1. lsp_goto_definition
func registerGotoDefinition(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "文件的绝对路径"},
			"line":      map[string]any{"type": "integer", "description": "行号（从 1 开始）"},
			"column":    map[string]any{"type": "integer", "description": "列号（从 1 开始）"},
		},
		"required": []string{"file_path", "line", "column"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "lsp_goto_definition",
			Description: "跳转到符号定义位置",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params LSPToolInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			server, err := manager.GetServerForFile(ctx, params.FilePath)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			uri := pathToURI(params.FilePath)
			lspParams := TextDocumentPositionParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Position:     Position{Line: params.Line - 1, Character: params.Column - 1},
			}

			var locations []Location
			if err := server.Call(ctx, "textDocument/definition", lspParams, &locations); err != nil {
				return errorResult("跳转到定义失败: " + err.Error()), nil
			}

			if len(locations) == 0 {
				return textResult("未找到定义"), nil
			}

			// 格式化输出
			var result strings.Builder
			for i, loc := range locations {
				if i > 0 {
					result.WriteString("\n")
				}
				result.WriteString(formatLocation(loc))
			}

			return textResult(result.String()), nil
		},
	)
}

// 2. lsp_find_references
func registerFindReferences(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "文件的绝对路径"},
			"line":      map[string]any{"type": "integer", "description": "行号（从 1 开始）"},
			"column":    map[string]any{"type": "integer", "description": "列号（从 1 开始）"},
		},
		"required": []string{"file_path", "line", "column"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "lsp_find_references",
			Description: "查找符号的所有引用位置",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params LSPToolInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			server, err := manager.GetServerForFile(ctx, params.FilePath)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			uri := pathToURI(params.FilePath)
			lspParams := ReferenceParams{
				TextDocumentPositionParams: TextDocumentPositionParams{
					TextDocument: TextDocumentIdentifier{URI: uri},
					Position:     Position{Line: params.Line - 1, Character: params.Column - 1},
				},
				Context: ReferenceContext{IncludeDeclaration: true},
			}

			var locations []Location
			if err := server.Call(ctx, "textDocument/references", lspParams, &locations); err != nil {
				return errorResult("查找引用失败: " + err.Error()), nil
			}

			if len(locations) == 0 {
				return textResult("未找到引用"), nil
			}

			// 格式化输出
			var result strings.Builder
			result.WriteString(fmt.Sprintf("找到 %d 个引用:\n", len(locations)))
			for i, loc := range locations {
				result.WriteString(fmt.Sprintf("%d. %s\n", i+1, formatLocation(loc)))
			}

			return textResult(result.String()), nil
		},
	)
}

// 3. lsp_hover
func registerHover(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "文件的绝对路径"},
			"line":      map[string]any{"type": "integer", "description": "行号（从 1 开始）"},
			"column":    map[string]any{"type": "integer", "description": "列号（从 1 开始）"},
		},
		"required": []string{"file_path", "line", "column"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "lsp_hover",
			Description: "获取符号的悬停信息（文档、类型签名）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params LSPToolInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			server, err := manager.GetServerForFile(ctx, params.FilePath)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			uri := pathToURI(params.FilePath)
			lspParams := TextDocumentPositionParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Position:     Position{Line: params.Line - 1, Character: params.Column - 1},
			}

			var hover Hover
			if err := server.Call(ctx, "textDocument/hover", lspParams, &hover); err != nil {
				return errorResult("获取悬停信息失败: " + err.Error()), nil
			}

			// 解析 contents（可能是 string、MarkedString 或数组）
			content := extractHoverContent(hover.Contents)
			if content == "" {
				return textResult("无悬停信息"), nil
			}

			return textResult(content), nil
		},
	)
}

// 4. lsp_rename
func registerRename(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "文件的绝对路径"},
			"line":      map[string]any{"type": "integer", "description": "行号（从 1 开始）"},
			"column":    map[string]any{"type": "integer", "description": "列号（从 1 开始）"},
			"new_name":  map[string]any{"type": "string", "description": "新的符号名称"},
		},
		"required": []string{"file_path", "line", "column", "new_name"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "lsp_rename",
			Description: "重命名符号（生成所有需要修改的位置）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params RenameInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			server, err := manager.GetServerForFile(ctx, params.FilePath)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			uri := pathToURI(params.FilePath)
			lspParams := RenameParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Position:     Position{Line: params.Line - 1, Character: params.Column - 1},
				NewName:      params.NewName,
			}

			var edit WorkspaceEdit
			if err := server.Call(ctx, "textDocument/rename", lspParams, &edit); err != nil {
				return errorResult("重命名失败: " + err.Error()), nil
			}

			// 格式化输出
			result := formatWorkspaceEdit(edit)
			if result == "" {
				return textResult("未生成任何编辑"), nil
			}

			return textResult(result), nil
		},
	)
}

// 5. lsp_code_action
func registerCodeAction(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "文件的绝对路径"},
			"line":      map[string]any{"type": "integer", "description": "行号（从 1 开始）"},
			"column":    map[string]any{"type": "integer", "description": "列号（从 1 开始）"},
		},
		"required": []string{"file_path", "line", "column"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "lsp_code_action",
			Description: "获取代码操作建议（如快速修复、重构）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params LSPToolInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			server, err := manager.GetServerForFile(ctx, params.FilePath)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			uri := pathToURI(params.FilePath)
			pos := Position{Line: params.Line - 1, Character: params.Column - 1}
			lspParams := CodeActionParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Range:        Range{Start: pos, End: pos},
				Context:      CodeActionContext{Diagnostics: []Diagnostic{}},
			}

			var actions []CodeAction
			if err := server.Call(ctx, "textDocument/codeAction", lspParams, &actions); err != nil {
				return errorResult("获取代码操作失败: " + err.Error()), nil
			}

			if len(actions) == 0 {
				return textResult("无可用代码操作"), nil
			}

			// 格式化输出
			var result strings.Builder
			result.WriteString(fmt.Sprintf("可用代码操作 (%d):\n", len(actions)))
			for i, action := range actions {
				result.WriteString(fmt.Sprintf("%d. %s", i+1, action.Title))
				if action.Kind != "" {
					result.WriteString(fmt.Sprintf(" [%s]", action.Kind))
				}
				result.WriteString("\n")
			}

			return textResult(result.String()), nil
		},
	)
}

// 6. lsp_formatting
func registerFormatting(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "文件的绝对路径"},
		},
		"required": []string{"file_path"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "lsp_formatting",
			Description: "格式化整个文档",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params struct {
				FilePath string `json:"file_path"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			server, err := manager.GetServerForFile(ctx, params.FilePath)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			uri := pathToURI(params.FilePath)
			lspParams := DocumentFormattingParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Options: FormattingOptions{
					TabSize:      4,
					InsertSpaces: false, // 使用 tab
				},
			}

			var edits []TextEdit
			if err := server.Call(ctx, "textDocument/formatting", lspParams, &edits); err != nil {
				return errorResult("格式化失败: " + err.Error()), nil
			}

			if len(edits) == 0 {
				return textResult("文档已格式化（无修改）"), nil
			}

			// 格式化输出
			var result strings.Builder
			result.WriteString(fmt.Sprintf("格式化建议 (%d 处修改):\n", len(edits)))
			for i, edit := range edits[:min(len(edits), 10)] { // 最多显示 10 处
				result.WriteString(fmt.Sprintf("%d. 行 %d-%d: 替换为 %q\n",
					i+1, edit.Range.Start.Line+1, edit.Range.End.Line+1, edit.NewText))
			}
			if len(edits) > 10 {
				result.WriteString(fmt.Sprintf("...还有 %d 处修改\n", len(edits)-10))
			}

			return textResult(result.String()), nil
		},
	)
}

// 7. lsp_document_symbol
func registerDocumentSymbol(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "文件的绝对路径"},
		},
		"required": []string{"file_path"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "lsp_document_symbol",
			Description: "获取文档符号列表（函数、类、变量等大纲）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params struct {
				FilePath string `json:"file_path"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			server, err := manager.GetServerForFile(ctx, params.FilePath)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			uri := pathToURI(params.FilePath)
			lspParams := DocumentSymbolParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
			}

			var symbols []DocumentSymbol
			if err := server.Call(ctx, "textDocument/documentSymbol", lspParams, &symbols); err != nil {
				return errorResult("获取文档符号失败: " + err.Error()), nil
			}

			if len(symbols) == 0 {
				return textResult("未找到符号"), nil
			}

			// 格式化输出（树形结构）
			var result strings.Builder
			result.WriteString(fmt.Sprintf("文档符号 (%d):\n", len(symbols)))
			for _, sym := range symbols {
				formatDocumentSymbol(&result, sym, 0)
			}

			return textResult(result.String()), nil
		},
	)
}

// 8. lsp_workspace_symbol
func registerWorkspaceSymbol(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "符号搜索关键词"},
		},
		"required": []string{"query"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "lsp_workspace_symbol",
			Description: "在整个工作区搜索符号",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params WorkspaceSymbolInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			// 使用 Go 语言服务器（默认）
			server, err := manager.GetServer(ctx, "go")
			if err != nil {
				return errorResult("获取语言服务器失败: " + err.Error()), nil
			}

			lspParams := WorkspaceSymbolParams{
				Query: params.Query,
			}

			var symbols []SymbolInformation
			if err := server.Call(ctx, "workspace/symbol", lspParams, &symbols); err != nil {
				return errorResult("搜索符号失败: " + err.Error()), nil
			}

			if len(symbols) == 0 {
				return textResult("未找到符号"), nil
			}

			// 格式化输出
			var result strings.Builder
			result.WriteString(fmt.Sprintf("找到 %d 个符号:\n", len(symbols)))
			for i, sym := range symbols[:min(len(symbols), 50)] { // 最多显示 50 个
				result.WriteString(fmt.Sprintf("%d. %s [%s] - %s\n",
					i+1, sym.Name, symbolKindName(sym.Kind), formatLocation(sym.Location)))
			}
			if len(symbols) > 50 {
				result.WriteString(fmt.Sprintf("...还有 %d 个结果\n", len(symbols)-50))
			}

			return textResult(result.String()), nil
		},
	)
}

// 9. lsp_completion
func registerCompletion(host *mcphost.Host, manager *ServerManager, logger *zap.Logger) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "文件的绝对路径"},
			"line":      map[string]any{"type": "integer", "description": "行号（从 1 开始）"},
			"column":    map[string]any{"type": "integer", "description": "列号（从 1 开始）"},
		},
		"required": []string{"file_path", "line", "column"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "lsp_completion",
			Description: "获取代码补全建议",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params LSPToolInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			server, err := manager.GetServerForFile(ctx, params.FilePath)
			if err != nil {
				return errorResult(err.Error()), nil
			}

			uri := pathToURI(params.FilePath)
			lspParams := TextDocumentPositionParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Position:     Position{Line: params.Line - 1, Character: params.Column - 1},
			}

			var completions CompletionList
			if err := server.Call(ctx, "textDocument/completion", lspParams, &completions); err != nil {
				return errorResult("获取补全建议失败: " + err.Error()), nil
			}

			if len(completions.Items) == 0 {
				return textResult("无补全建议"), nil
			}

			// 格式化输出
			var result strings.Builder
			result.WriteString(fmt.Sprintf("补全建议 (%d):\n", len(completions.Items)))
			for i, item := range completions.Items[:min(len(completions.Items), 20)] {
				result.WriteString(fmt.Sprintf("%d. %s", i+1, item.Label))
				if item.Detail != "" {
					result.WriteString(fmt.Sprintf(" - %s", item.Detail))
				}
				result.WriteString("\n")
			}
			if len(completions.Items) > 20 {
				result.WriteString(fmt.Sprintf("...还有 %d 个建议\n", len(completions.Items)-20))
			}

			return textResult(result.String()), nil
		},
	)
}

// --- 辅助函数 ---

func textResult(text string) *mcphost.ToolResult {
	return &mcphost.ToolResult{Content: jsonText(text)}
}

func errorResult(msg string) *mcphost.ToolResult {
	return &mcphost.ToolResult{Content: jsonText(msg), IsError: true}
}

func jsonText(text string) json.RawMessage {
	data, _ := json.Marshal(text)
	return data
}

func formatLocation(loc Location) string {
	// 将 URI 转换回路径
	path := strings.TrimPrefix(loc.URI, "file://")
	return fmt.Sprintf("%s:%d:%d",
		filepath.Base(path),
		loc.Range.Start.Line+1,
		loc.Range.Start.Character+1)
}

func formatWorkspaceEdit(edit WorkspaceEdit) string {
	if len(edit.Changes) == 0 {
		return ""
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("重命名将修改 %d 个文件:\n", len(edit.Changes)))

	i := 1
	for uri, edits := range edit.Changes {
		path := strings.TrimPrefix(uri, "file://")
		result.WriteString(fmt.Sprintf("%d. %s (%d 处修改)\n",
			i, filepath.Base(path), len(edits)))
		i++
	}

	return result.String()
}

func formatDocumentSymbol(sb *strings.Builder, sym DocumentSymbol, indent int) {
	prefix := strings.Repeat("  ", indent)
	sb.WriteString(fmt.Sprintf("%s%s [%s] 行 %d\n",
		prefix, sym.Name, symbolKindName(sym.Kind), sym.Range.Start.Line+1))

	for _, child := range sym.Children {
		formatDocumentSymbol(sb, child, indent+1)
	}
}

func extractHoverContent(contents json.RawMessage) string {
	// 尝试解析为 string
	var str string
	if err := json.Unmarshal(contents, &str); err == nil {
		return str
	}

	// 尝试解析为 MarkedString
	var marked MarkedString
	if err := json.Unmarshal(contents, &marked); err == nil {
		if marked.Language != "" {
			return fmt.Sprintf("```%s\n%s\n```", marked.Language, marked.Value)
		}
		return marked.Value
	}

	// 尝试解析为数组
	var arr []json.RawMessage
	if err := json.Unmarshal(contents, &arr); err == nil && len(arr) > 0 {
		var parts []string
		for _, item := range arr {
			if err := json.Unmarshal(item, &str); err == nil {
				parts = append(parts, str)
			}
		}
		return strings.Join(parts, "\n")
	}

	return string(contents)
}

func symbolKindName(kind SymbolKind) string {
	switch kind {
	case SymbolKindFile:
		return "File"
	case SymbolKindModule:
		return "Module"
	case SymbolKindNamespace:
		return "Namespace"
	case SymbolKindPackage:
		return "Package"
	case SymbolKindClass:
		return "Class"
	case SymbolKindMethod:
		return "Method"
	case SymbolKindProperty:
		return "Property"
	case SymbolKindField:
		return "Field"
	case SymbolKindConstructor:
		return "Constructor"
	case SymbolKindEnum:
		return "Enum"
	case SymbolKindInterface:
		return "Interface"
	case SymbolKindFunction:
		return "Function"
	case SymbolKindVariable:
		return "Variable"
	case SymbolKindConstant:
		return "Constant"
	default:
		return fmt.Sprintf("Kind%d", kind)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

