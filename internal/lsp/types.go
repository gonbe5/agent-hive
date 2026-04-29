package lsp

import "encoding/json"

// 通用 LSP 协议类型定义（基于 LSP 3.17 规范）

// Position 表示文本文档中的位置
type Position struct {
	Line      int `json:"line"`      // 从 0 开始
	Character int `json:"character"` // 从 0 开始（UTF-16 编码单元）
}

// Range 表示文本文档中的范围
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location 表示资源中的位置
type Location struct {
	URI   string `json:"uri"`   // 文件 URI（如 file:///path/to/file.go）
	Range Range  `json:"range"`
}

// TextDocumentIdentifier 标识文本文档
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier 包含版本的文档标识
type VersionedTextDocumentIdentifier struct {
	TextDocumentIdentifier
	Version int `json:"version"` // 文档版本号
}

// TextDocumentItem 表示打开的文本文档
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// TextDocumentPositionParams 包含文档和位置
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// DocumentSymbol 表示文档符号（如函数、类、变量）
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolInformation 表示符号信息（workspace symbols）
type SymbolInformation struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	Location      Location   `json:"location"`
	ContainerName string     `json:"containerName,omitempty"`
}

// Hover 表示悬停信息
type Hover struct {
	Contents json.RawMessage `json:"contents"` // 可能是 string、MarkedString 或 MarkedString[]
	Range    *Range          `json:"range,omitempty"`
}

// MarkedString 表示带语言标记的字符串
type MarkedString struct {
	Language string `json:"language"`
	Value    string `json:"value"`
}

// CompletionItem 表示代码补全项
type CompletionItem struct {
	Label         string          `json:"label"`
	Kind          CompletionKind  `json:"kind,omitempty"`
	Detail        string          `json:"detail,omitempty"`
	Documentation json.RawMessage `json:"documentation,omitempty"`
	InsertText    string          `json:"insertText,omitempty"`
	SortText      string          `json:"sortText,omitempty"`
}

// CompletionList 表示补全列表
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// TextEdit 表示文本编辑
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// WorkspaceEdit 表示工作区编辑
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"` // URI -> []TextEdit
}

// CodeAction 表示代码操作
type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	Command     *Command       `json:"command,omitempty"`
}

// Command 表示命令
type Command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

// Diagnostic 表示诊断信息
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"` // 1=Error, 2=Warning, 3=Info, 4=Hint
	Code     string `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// DocumentFormattingParams 文档格式化参数
type DocumentFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Options      FormattingOptions      `json:"options"`
}

// FormattingOptions 格式化选项
type FormattingOptions struct {
	TabSize      int  `json:"tabSize"`
	InsertSpaces bool `json:"insertSpaces"`
}

// RenameParams 重命名参数
type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

// CodeActionParams 代码操作参数
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

// CodeActionContext 代码操作上下文
type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// DocumentSymbolParams 文档符号参数
type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// WorkspaceSymbolParams 工作区符号参数
type WorkspaceSymbolParams struct {
	Query string `json:"query"`
}

// SymbolKind 符号类型
type SymbolKind int

const (
	SymbolKindFile        SymbolKind = 1
	SymbolKindModule      SymbolKind = 2
	SymbolKindNamespace   SymbolKind = 3
	SymbolKindPackage     SymbolKind = 4
	SymbolKindClass       SymbolKind = 5
	SymbolKindMethod      SymbolKind = 6
	SymbolKindProperty    SymbolKind = 7
	SymbolKindField       SymbolKind = 8
	SymbolKindConstructor SymbolKind = 9
	SymbolKindEnum        SymbolKind = 10
	SymbolKindInterface   SymbolKind = 11
	SymbolKindFunction    SymbolKind = 12
	SymbolKindVariable    SymbolKind = 13
	SymbolKindConstant    SymbolKind = 14
	SymbolKindString      SymbolKind = 15
	SymbolKindNumber      SymbolKind = 16
	SymbolKindBoolean     SymbolKind = 17
	SymbolKindArray       SymbolKind = 18
)

// CompletionKind 补全类型
type CompletionKind int

const (
	CompletionKindText        CompletionKind = 1
	CompletionKindMethod      CompletionKind = 2
	CompletionKindFunction    CompletionKind = 3
	CompletionKindConstructor CompletionKind = 4
	CompletionKindField       CompletionKind = 5
	CompletionKindVariable    CompletionKind = 6
	CompletionKindClass       CompletionKind = 7
	CompletionKindInterface   CompletionKind = 8
	CompletionKindModule      CompletionKind = 9
	CompletionKindProperty    CompletionKind = 10
	CompletionKindUnit        CompletionKind = 11
	CompletionKindValue       CompletionKind = 12
	CompletionKindEnum        CompletionKind = 13
	CompletionKindKeyword     CompletionKind = 14
	CompletionKindSnippet     CompletionKind = 15
)

// InitializeParams 初始化参数
type InitializeParams struct {
	ProcessID    *int                `json:"processId"`
	RootURI      string              `json:"rootUri,omitempty"`
	Capabilities ClientCapabilities  `json:"capabilities"`
	InitOptions  interface{}         `json:"initializationOptions,omitempty"`
}

// ClientCapabilities 客户端能力
type ClientCapabilities struct {
	TextDocument *TextDocumentClientCapabilities `json:"textDocument,omitempty"`
	Workspace    *WorkspaceClientCapabilities    `json:"workspace,omitempty"`
}

// TextDocumentClientCapabilities 文本文档能力
type TextDocumentClientCapabilities struct {
	Completion    *CompletionCapability    `json:"completion,omitempty"`
	Hover         *HoverCapability         `json:"hover,omitempty"`
	Definition    *DefinitionCapability    `json:"definition,omitempty"`
	References    *ReferencesCapability    `json:"references,omitempty"`
	Rename        *RenameCapability        `json:"rename,omitempty"`
	Formatting    *FormattingCapability    `json:"formatting,omitempty"`
	CodeAction    *CodeActionCapability    `json:"codeAction,omitempty"`
	DocumentSymbol *DocumentSymbolCapability `json:"documentSymbol,omitempty"`
}

// WorkspaceClientCapabilities 工作区能力
type WorkspaceClientCapabilities struct {
	Symbol *WorkspaceSymbolCapability `json:"symbol,omitempty"`
}

// CompletionCapability 补全能力
type CompletionCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// HoverCapability 悬停能力
type HoverCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// DefinitionCapability 定义跳转能力
type DefinitionCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// ReferencesCapability 引用查找能力
type ReferencesCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// RenameCapability 重命名能力
type RenameCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// FormattingCapability 格式化能力
type FormattingCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// CodeActionCapability 代码操作能力
type CodeActionCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// DocumentSymbolCapability 文档符号能力
type DocumentSymbolCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// WorkspaceSymbolCapability 工作区符号能力
type WorkspaceSymbolCapability struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// InitializeResult 初始化结果
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

// ServerCapabilities 服务器能力
type ServerCapabilities struct {
	TextDocumentSync           interface{}                `json:"textDocumentSync,omitempty"`
	CompletionProvider         *CompletionOptions         `json:"completionProvider,omitempty"`
	HoverProvider              bool                       `json:"hoverProvider,omitempty"`
	DefinitionProvider         bool                       `json:"definitionProvider,omitempty"`
	ReferencesProvider         bool                       `json:"referencesProvider,omitempty"`
	RenameProvider             interface{}                `json:"renameProvider,omitempty"`
	DocumentFormattingProvider bool                       `json:"documentFormattingProvider,omitempty"`
	CodeActionProvider         interface{}                `json:"codeActionProvider,omitempty"`
	DocumentSymbolProvider     bool                       `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider    bool                       `json:"workspaceSymbolProvider,omitempty"`
}

// CompletionOptions 补全选项
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// ReferenceParams 引用查找参数
type ReferenceParams struct {
	TextDocumentPositionParams
	Context ReferenceContext `json:"context"`
}

// ReferenceContext 引用上下文
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// --- 通知类型定义 ---

// PublishDiagnosticsParams textDocument/publishDiagnostics 通知参数
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// MessageType 消息类型
type MessageType int

const (
	MessageTypeError   MessageType = 1
	MessageTypeWarning MessageType = 2
	MessageTypeInfo    MessageType = 3
	MessageTypeLog     MessageType = 4
)

// ShowMessageParams window/showMessage 通知参数
type ShowMessageParams struct {
	Type    MessageType `json:"type"`
	Message string      `json:"message"`
}

// LogMessageParams window/logMessage 通知参数
type LogMessageParams struct {
	Type    MessageType `json:"type"`
	Message string      `json:"message"`
}
