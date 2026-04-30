// 会话相关
export interface Session {
  id: string;
  name: string;
  message_count: number;
  total_tokens: number;
  last_accessed: string;
  created_at?: string;
  updated_at?: string;
  tags: string[];
  is_active: boolean;
  is_starred?: boolean;
}

export interface SessionDetail extends Session {
  created: string;
  updated: string;
}

export interface CreateSessionRequest {
  name: string;
  tags?: string[];
}

export interface UpdateSessionRequest {
  name?: string;
  tags?: string[];
}

export interface FileAttachment {
  filename: string;
  mime_type: string;
  data: string; // base64
  size: number; // bytes, for display
}

export interface SendMessageRequest {
  content: string;
  attachments?: FileAttachment[];
  reasoning_effort?: string;
}

export interface SendMessageResponse {
  content: string;
  completed: boolean;
}

// 消息相关
export interface ToolCall {
  id: string;
  name: string;
  arguments: string; // JSON 字符串
}

// 工具调用实时状态（由 tool_call WS 事件更新）
export interface ToolCallStatus {
  id: string;
  name: string;
  status: 'running' | 'success' | 'error';
  duration?: number; // 毫秒
  error?: string;
}

// Token 用量
export interface MessageUsage {
  input_tokens: number;
  output_tokens: number;
}

export interface Message {
  role: 'user' | 'assistant' | 'tool';
  content: string;
  reasoning_content?: string; // <think>...</think> 推理内容，可折叠展示
  tool_call_id?: string;
  tool_calls?: ToolCall[];
  timestamp?: string;
  attachments?: FileAttachment[];
  usage?: MessageUsage;       // token 用量（后端支持后填充）
  llm_duration?: number;      // LLM 请求耗时（毫秒）
  is_error?: boolean;         // 错误消息标记
  tool_name?: string;         // 工具名称（tool 消息使用）
}

export interface MessagesListResponse {
  session_id: string;
  messages: Message[];
  total: number;
}

// HITL 相关
export type InputRequestType = 'approval' | 'clarification' | 'confirmation' | 'choice' | 'permission';

export interface InputRequest {
  id: string;
  task_id: string;
  step_id?: string;
  session_id?: string;
  type: InputRequestType;
  prompt: string;
  options?: string[];
  default?: string;
  timeout?: number;
  tool_name?: string;
  data?: Record<string, unknown>;
  created_at: string;
}

export interface InputResponse {
  request_id: string;
  task_id: string;
  value: string;
  action: string; // "approve" | "reject" | "modify" | "proceed"
  remember?: boolean;
}

export interface UserCommand {
  type: 'pause' | 'resume' | 'cancel';
  task_id: string;
  payload?: unknown;
}

// Agent / Skill
export interface AgentInfo {
  id: string;
  name: string;
  description: string;
  skills?: string[];
}

export interface SkillMetadata {
  name: string;
  description: string;
  user_invocable?: boolean;
  argument_hint?: string;
  model?: string;
  context?: string;
}

// Admin Skills（管理后台 overlay 视图）
export interface AdminSkillItem {
  name: string;
  description: string;
  path: string;
  origin: 'fs' | 'db';
  revision: number;
}

export interface AdminSkillDetail extends AdminSkillItem {
  content: string;
}

export type QualityCandidateStatus = 'new' | 'reviewing' | 'approved' | 'rejected' | 'promoted';

export interface QualityCandidateCase {
  id: string;
  name: string;
  route: string;
  input: string;
  expected_tools?: string[];
  allowed_tools?: string[];
  expected_skills?: string[];
  expected_agents?: string[];
  scenario?: string;
  expected_status: string;
  failure_type?: string;
  risk?: string;
  required: boolean;
  notes?: string;
}

export type QualityOptimizationSuggestionKind = 'prompt_diff_suggestion' | 'tool_description_suggestion' | 'skill_draft';

export interface QualityOptimizationSuggestion {
  kind: QualityOptimizationSuggestionKind;
  title: string;
  target?: string;
  rationale: string;
  proposed: string;
  review_required: boolean;
}

export interface QualityCandidateRecord {
  id: string;
  status: QualityCandidateStatus;
  route: string;
  session_id: string;
  replay_ref: string;
  input: string;
  case: QualityCandidateCase;
  failure_type: string;
  risk: string;
  fingerprint: string;
  source_event: Record<string, unknown>;
  review_note?: string;
  created_by?: string;
  reviewed_by?: string;
  promoted_case_id?: string;
  optimization_suggestions?: QualityOptimizationSuggestion[];
  golden_case?: QualityCandidateCase;
  created_at: string;
  updated_at: string;
  reviewed_at?: string;
}

export interface QualityCandidateUpdateRequest {
  status: QualityCandidateStatus;
  review_note?: string;
  promoted_case_id?: string;
}

export interface QualityCandidateCreateRequest {
  session_id: string;
  replay_ref?: string;
  event_index?: number;
  input: string;
  quality_event: unknown;
}

export interface QualityCandidatesResponse {
  candidates: QualityCandidateRecord[];
  total: number;
  page: number;
  size: number;
}

// 健康检查
export interface Health {
  status: string;
  version?: string;
  uptime?: number;
  active_sessions?: number;
}

// WebSocket 消息
export interface WSMessage {
  type: string;
  payload: unknown;
}

// API 错误
export interface ApiError {
  error: string;
  code: number;
}

// 通用列表响应
export interface SessionListResponse {
  sessions: Session[];
}

// 微信配置相关
export interface WeChatProtocolConfig {
  [key: string]: unknown;
}

export interface WeChatProtocolStatus {
  enabled: boolean;
  status: 'not_started' | 'connected' | 'error';
  logged_in: boolean;
  config: WeChatProtocolConfig;
}

export interface WeChatConfigResponse {
  protocols: {
    wechaty: WeChatProtocolStatus;
    wechatpadpro: WeChatProtocolStatus;
  };
}

export interface UpdateWeChatProtocolRequest {
  enabled: boolean;
  config: WeChatProtocolConfig;
}

// Model
export interface ModelInfo {
  name: string;
  model: string;
  provider?: string;
  service_type?: 'llm' | 'image_gen' | 'video_gen' | 'tts' | 'stt' | 'embedding';
  is_active: boolean;
}

// 远程 ACP Agent
export interface RemoteAgentConfig {
  name: string;
  description: string;
  transport: 'stdio' | 'http';
  command?: string;
  args?: string[];
  url?: string;
  headers?: Record<string, string>;
  skills?: string[];
  enabled: boolean;
}

export interface RemoteAgentHealth {
  agent_id: string;
  status: number | string; // 后端 AgentStatus: 0=stopped, 1=running, 2=error
  uptime: number;          // time.Duration 纳秒
}

export interface ExecRule {
  pattern: string;
  policy: 'allow' | 'ask' | 'deny';
  description?: string;
}

// 运行时配置（config.get 返回的脱敏配置）
export interface RuntimeConfig {
  hitl: {
    enabled: boolean;
    permission_rules: PermissionRule[];
  };
  agent: {
    timeout: number;
    shell_timeout: number;
  };
  mcp: {
    timeout: number;
    servers: Record<string, MCPServerConfig>;
  };
  channel: {
    enabled: boolean;
    dingtalk: DingTalkConfig;
    feishu: FeishuConfig;
    wecom: WeComConfig;
  };
  security?: {
    default_policy?: 'allow' | 'ask' | 'deny';
    exec_rules: ExecRule[];
  };
}

export interface PermissionRule {
  tool_name: string;
  action: 'allow' | 'ask' | 'deny';
  pattern?: string;
}

export interface MCPServerConfig {
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  transport?: string; // "stdio" | "sse" | "http"
  url?: string;
  headers?: Record<string, string>;
  timeout?: string;
}

// IM 通道配置
export interface DingTalkConfig {
  enabled: boolean;
  app_key: string;
  app_secret: string;
  token: string;
  aes_key: string;
  agent_id: number;
}

export interface FeishuRendererConfig {
  /** 回滚开关（反向语义）：true = 禁用流式卡片，走 legacy Plugin.Send。默认 false = 启用 EventRenderer。 */
  disabled?: boolean;
  /** 卡片 PATCH 最小间隔（毫秒）。默认 300；<= 0 后端 Normalize 回退到 300。 */
  throttle_ms?: number;
  /** 卡片中展示 "Agent 思考中" 等中间状态文案。默认 false。 */
  show_agent_progress?: boolean;
}

/** 飞书事件入口模式。CEO 决议:webhook XOR longconn,严禁同进程并存。 */
export type FeishuIngressMode = '' | 'webhook' | 'longconn';

export interface FeishuReliabilityConfig {
  /** longconn 入口主开关。Phase 2B+ 推荐用这个,旧的 longconn_enabled 顶层字段会回退读取。 */
  longconn_enabled?: boolean;
  /** longconn 重连后是否补偿断线期间消息(gap fetch),默认 false。 */
  longconn_gap_fetch_enabled?: boolean;
}

export interface FeishuConfig {
  enabled: boolean;
  app_id: string;
  app_secret: string;
  verification_token: string;
  encrypt_key: string;
  /** 收到消息的 ack 表情:"Get"(默认)/ "Typing" / "none"(禁用);飞书 reactions API emoji_type(CamelCase)。
   *  老值 "GET"/"KEYBOARD" 后端 Normalize 会静默迁移到 "Get"/"Typing";其他非法值 warn 回退到 "Get"。 */
  ack_emoji?: string;
  /** EventRenderer(流式卡片)行为参数,未配置等同于"全部默认"。 */
  renderer?: FeishuRendererConfig;
  /** 事件入口模式。空 = 默认走 longconn_enabled 推断,否则默认 webhook。 */
  ingress_mode?: FeishuIngressMode;
  /** webhook URL 声明。dual-ingress fatal guard:webhook_url 非空 + longconn=true → 启动 panic。 */
  webhook_url?: string;
  /** 飞书地区:""/"cn" 默认连 open.feishu.cn;"intl"/"lark"/"international" 切 open.larksuite.com。 */
  region?: string;
  /** 可靠性 / 长连接配置。 */
  reliability?: FeishuReliabilityConfig;
}

export interface WeComConfig {
  enabled: boolean;
  corp_id: string;
  agent_id: number;
  secret: string;
  token: string;
  encoding_aes_key: string;
}

export interface ConfigUpdateRequest {
  hitl?: {
    enabled?: boolean;
    permission_rules?: PermissionRule[];
  };
  agent?: {
    timeout?: string;
    shell_timeout?: string;
  };
  mcp?: {
    timeout?: string;
    servers?: Record<string, MCPServerConfig | null>;
  };
  channel?: {
    enabled?: boolean;
    dingtalk?: DingTalkConfig;
    feishu?: FeishuConfig;
    wecom?: WeComConfig;
  };
  security?: {
    default_policy?: 'allow' | 'ask' | 'deny';
    exec_rules?: ExecRule[];
  };
}

// 外部资源
export interface ExternalResource {
  name: string;
  type: string;
  environment: string;
  description: string;
  connection: string;
  endpoint: string;
  credentials: string;
  read_only: boolean;
  enabled: boolean;
  updated_at: string;
}

// Gateway RPC 响应格式
export interface RPCResponse<T = unknown> {
  id: string;
  result?: T;
  error?: { code: number; message: string };
}

// ── Admin 用户管理 ──────────────────────────────────────────────────────────

export interface AdminUser {
  id: string;
  display_name: string;
  email: string;
  role: 'user' | 'admin';
  status: 'active' | 'disabled';
  auth_provider: string;
  token_quota: number;
  token_used: number;
}

export interface AdminUsersResponse {
  users: AdminUser[];
  total: number;
  page: number;
  size: number;
}

export interface UsageSummary {
  total_cost_usd: number;
  total_tokens: number;
  by_model: Record<string, { tokens: number; cost_usd: number }>;
}

export interface UsageModelCost {
  cost_usd: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  tokens: number;
  request_count?: number;
}

export interface UsageQualityCost {
  by_task_type: Record<string, UsageModelCost>;
  by_quality_case: Record<string, UsageModelCost>;
  by_prompt_version: Record<string, UsageModelCost>;
  by_failure_type?: Record<string, UsageModelCost>;
  by_final_status?: Record<string, UsageModelCost>;
  top_quality_cases: Array<{
    quality_case_id: string;
    tokens: number;
    cost_usd: number;
    request_count: number;
  }>;
}

export interface AdminProvider {
  name: string;
  provider_type: string;
  enabled: boolean;
  config_json: Record<string, unknown>;
}

export interface AdminProvidersResponse {
  providers: AdminProvider[];
}

export interface PromptRecord {
  key: string;
  language: string;
  content: string;
  updated_at: string;
  updated_by: string;
}

export interface PromptSmokeEvalRequest {
  key: string;
  language: string;
  content: string;
}

export interface PromptSmokeEvalResponse {
  ok: boolean;
  checked_cases: number;
  warnings: string[];
}

// LLM Provider 管理
export interface LLMProviderRecord {
  name: string;
  provider_type: string;
  base_url: string;
  api_key: string; // 脱敏后
  is_default: boolean;
  enabled: boolean;
  api_format: string;
  service_type: string;
  config_json: string;
  created_at: string;
  updated_at: string;
}

// LLM Model 管理
export interface LLMModelRecord {
  name: string;
  provider_name: string;
  model: string;
  base_url: string;
  api_key: string;
  is_default: boolean;
  enabled: boolean;
  config_json: string;
  created_at: string;
  updated_at: string;
}
