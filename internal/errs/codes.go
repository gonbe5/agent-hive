package errs

// agents-hive 系统的错误代码
const (
	// 通用错误 (1xxx)
	CodeUnknown            = 1000
	CodeInternal           = 1001
	CodeInvalidInput       = 1002
	CodeTimeout            = 1003
	CodeCanceled           = 1004
	CodeInvalidArgument    = 1005
	CodeNotFound           = 1006
	CodeUnavailable        = 1007
	CodeResourceExhausted  = 1008
	CodeFailedPrecondition = 1009

	// Agent 错误 (2xxx)
	CodeAgentNotFound    = 2000
	CodeAgentUnavailable = 2001
	CodeAgentTimeout     = 2002
	CodeAgentPanic       = 2003
	CodeAgentLoopFailed  = 2004

	// 技能错误 (3xxx)
	CodeSkillNotFound     = 3000
	CodeSkillLoadFailed   = 3001
	CodeSkillExecFailed   = 3002
	CodeSkillInvalidName  = 3003
	CodeSkillScriptFailed = 3004
	CodeSkillHookFailed   = 3005
	CodeSkillForkFailed   = 3006
	CodeSkillToolBlocked  = 3007
	CodePermissionDenied  = 3008 // 工具权限被拒绝
	CodeExecutionFailed   = 3009 // 命令执行失败
	CodeSkillAmbiguous    = 3010 // 多 marketplace 同名 skill 冲突

	// 计划错误 (4xxx)
	CodePlanGenFailed       = 4000
	CodePlanInvalid         = 4001
	CodePlanExecFailed      = 4002
	CodeReplanLimitExceeded = 4003

	// MCP 错误 (5xxx)
	CodeMCPConnFailed        = 5000
	CodeMCPToolNotFound      = 5001
	CodeMCPToolExecFailed    = 5002
	CodeMCPTransportFailed   = 5003 // 传输层连接/发送失败
	CodeMCPTransportClosed   = 5004 // 传输层已关闭
	CodeMCPSSEParseFailed    = 5005 // SSE 事件解析失败
	CodeMCPResponseInvalid   = 5006 // 服务端响应格式无效
	CodeMCPResourceNotFound  = 5007 // MCP 资源未找到
	CodeMCPPromptNotFound    = 5008 // MCP 提示未找到
	CodeMCPOAuthFailed       = 5009 // OAuth 认证失败
	CodeMCPOAuthTokenExpired = 5010 // OAuth token 已过期

	// API 错误 (6xxx)
	CodeTaskNotFound   = 6000
	CodeBadRequest     = 6001
	CodeInvalidRequest = 6002

	// HITL 错误 (7xxx)
	CodeInputTimeout    = 7000 // 等待用户输入超时
	CodeInputInvalid    = 7001 // 输入响应格式无效
	CodeTaskPaused      = 7002 // 暂停的任务拒绝操作
	CodeTaskCanceled    = 7003 // 任务被用户取消
	CodeInputNotPending = 7004 // 回复了不存在的输入请求

	// Channel 错误 (8xxx)
	CodeChannelSendFailed         = 8000 // 消息发送失败
	CodeChannelWebhookInvalid     = 8001 // Webhook 请求无效
	CodeChannelPlatformNotFound   = 8002 // 平台未注册
	CodeChannelBindingNotFound    = 8003 // 绑定不存在
	CodeWeChatLoginFailed         = 8010 // 微信登录失败
	CodeWeChatProtocolError       = 8011 // 微信协议通信错误
	CodeWeChatNotLoggedIn         = 8012 // 微信未登录
	CodeWeChatPadProAPIError      = 8013 // WeChatPadPro API 调用失败
	CodeWeChatPadProConnectFailed = 8014 // 无法连接到 WeChatPadPro 服务
	CodeWeChatPadProInvalidResp   = 8015 // WeChatPadPro API 返回无效响应

	// Security 错误 (9xxx)
	CodeExecDenied          = 9000 // 命令被安全策略拒绝
	CodeExecApprovalTimeout = 9001 // 命令审批超时
	CodeEnvTampered         = 9002 // 环境变量被篡改
	CodeASTParseFailed      = 9003 // AST 解析失败

	// 控制平面错误 (10xxx) - 原 ACP 控制平面
	CodeCPSessionLimitReached = 10000 // 并发会话数达到上限
	CodeCPRateLimited         = 10001 // 速率限制
	CodeCPBindingNotFound     = 10002 // 绑定不存在

	// Plugin 错误 (11xxx)
	CodePluginNotFound   = 11000 // 插件未找到
	CodePluginLoadFailed = 11001 // 插件加载失败
	CodePluginExecFailed = 11002 // 插件执行失败
	CodePluginCrashed    = 11003 // 插件崩溃

	// Store 错误 (12xxx)
	CodeStoreReadFailed  = 12000 // 存储读取失败
	CodeStoreWriteFailed = 12001 // 存储写入失败
	CodeStoreParseFailed = 12002 // 存储解析失败
	CodeStoreError       = 12003 // 存储通用错误

	// Config 错误 (13xxx)
	CodeConfigInvalid = 13000 // 配置无效

	// LLM 错误 (14xxx)
	CodeLLMError           = 14000 // LLM 调用错误
	CodeModelFetchFailed   = 14001 // 模型元数据获取失败
	CodeLLMResponseInvalid = 14002 // LLM 响应格式无效

	// Watcher 错误 (15xxx)
	CodeWatcherInitFailed  = 15000 // 监听器初始化失败
	CodeWatcherStartFailed = 15001 // 监听器启动失败
	CodeWatcherAddFailed   = 15002 // 添加监听路径失败
	CodeWatcherParseFailed = 15003 // gitignore 解析失败

	// ACPServer 错误 (16xxx) - ACP 协议服务器错误
	CodeACPServerInitFailed   = 16000 // ACP 服务器初始化失败
	CodeACPServerConnFailed   = 16001 // ACP 连接失败
	CodeACPServerSessionLimit = 16002 // ACP 会话数达到上限
	CodeACPServerAuthFailed   = 16003 // ACP 认证失败
	CodeACPServerInvalidReq   = 16004 // ACP 请求无效

	// ACP Client 错误 (17xxx) - 远程 ACP Agent 客户端错误
	CodeACPClientConnFailed   = 17000 // 远程 ACP Agent 连接失败
	CodeACPClientPromptFailed = 17001 // 远程 ACP Agent 调用失败
	CodeACPClientTimeout      = 17002 // 远程 ACP Agent 超时

	// Memory 错误 (18xxx) - 记忆系统错误
	CodeMemoryNotFound     = 18000 // 记忆未找到
	CodeMemoryWriteFailed  = 18001 // 记忆写入失败
	CodeMemoryReadFailed   = 18002 // 记忆读取失败
	CodeMemorySearchFailed = 18003 // 记忆搜索失败

	// 认证/配额错误 (19xxx)
	CodeQuotaExceeded = 19000 // 用户配额超限
)
