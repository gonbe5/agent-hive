package master

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/airouter"
	"github.com/chef-guo/agents-hive/internal/compaction"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/fileconv"
	"github.com/chef-guo/agents-hive/internal/i18n"
	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/mcphost"
	"github.com/chef-guo/agents-hive/internal/plugin"
)

// buildUserContent 构建用户消息内容（支持附件）
func (m *Master) buildUserContent(ctx context.Context, text string, attachments []FileAttachment) llm.Content {
	if len(attachments) == 0 {
		return llm.NewTextContent(text)
	}

	parts := []llm.ContentPart{llm.TextPart(text)}
	for _, a := range attachments {
		m.logger.Info("[DEBUG-UPLOAD] fileconv.Convert 开始",
			zap.String("filename", a.Filename),
			zap.String("mime_type", a.MimeType),
			zap.Int("data_len", len(a.Data)),
		)
		result, err := fileconv.Convert(ctx, a.Filename, a.MimeType, a.Data, m.transcribeAudio)
		if err != nil {
			m.logger.Error("[DEBUG-UPLOAD] fileconv.Convert 失败",
				zap.String("filename", a.Filename),
				zap.Error(err),
			)
			parts = append(parts, llm.TextPart(fmt.Sprintf("[文件处理失败: %s] %s", a.Filename, err.Error())))
			continue
		}
		m.logger.Info("[DEBUG-UPLOAD] fileconv.Convert 成功",
			zap.String("filename", a.Filename),
			zap.String("result_type", result.Type),
			zap.Int("text_len", len(result.Text)),
		)
		switch result.Type {
		case "text":
			parts = append(parts, llm.TextPart(result.Text))
		case "image":
			parts = append(parts, llm.ImageBase64Part(a.MimeType, a.Data))
		case "file":
			parts = append(parts, llm.ContentPart{Type: llm.ContentFile, FileData: a.Data, Filename: a.Filename})
		}
	}
	m.logger.Info("[DEBUG-UPLOAD] 最终 parts 构成",
		zap.Int("total_parts", len(parts)),
	)
	for i, p := range parts {
		m.logger.Info("[DEBUG-UPLOAD] part",
			zap.Int("index", i),
			zap.String("type", string(p.Type)),
			zap.Int("text_len", len(p.Text)),
			zap.Int("image_url_len", len(p.ImageURL)),
			zap.Int("file_data_len", len(p.FileData)),
		)
	}
	return llm.NewMultiContent(parts...)
}

// transcribeAudio 使用 OpenAI Whisper API 转录音频
func (m *Master) transcribeAudio(ctx context.Context, audioData []byte, filename string) (string, error) {
	if m.config.APIKey == "" {
		return "", errs.New(errs.CodeInvalidInput, "需要 API Key 才能使用音频转录功能")
	}
	// 创建用于 Whisper 的 openai client
	opts := []option.RequestOption{option.WithAPIKey(m.config.APIKey)}
	if m.config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(m.config.BaseURL))
	}
	client := openai.NewClient(opts...)
	resp, err := client.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		Model: "whisper-1",
		File:  openai.File(bytes.NewReader(audioData), filename, "audio/mpeg"),
	})
	if err != nil {
		return "", errs.Wrap(errs.CodeInternal, "音频转录失败", err)
	}
	return resp.Text, nil
}

// buildSystemPromptHardcoded 使用硬编码默认值构建 system prompt（promptLoader 为 nil 时的 fallback）
func (m *Master) buildSystemPromptHardcoded() string {
	var b strings.Builder
	b.WriteString("## 身份定义\n\n")
	b.WriteString("你是 Hive，一个具备工具调用能力的 AI 助手。你直接完成用户任务，不需要委派给其他系统。\n")
	b.WriteString("你的核心能力：代码开发、文件操作、系统运维、信息检索、项目管理。\n")
	b.WriteString("你的工作方式：分析任务 → 选择工具 → 执行 → 验证结果 → 回复用户。\n\n")
	b.WriteString("## 任务执行策略\n\n")
	b.WriteString("你拥有所有工具，可以直接完成绝大多数任务。优先直接使用工具，不要委派。\n\n")
	b.WriteString("### 何时直接执行（默认）\n")
	b.WriteString("- 单步或少量步骤的任务：直接调用 read_file、edit、bash、grep 等工具\n")
	b.WriteString("- 需要会话上下文的任务：你拥有完整的对话历史\n")
	b.WriteString("- 需要多轮交互的任务：你可以在 ReAct 循环中持续迭代\n\n")
	b.WriteString("### 工具选择指南\n")
	b.WriteString("- 代码相关（读/写/编辑/搜索代码）→ read_file, edit, grep, glob\n")
	b.WriteString("- 系统操作（执行命令、查看日志、管理进程）→ bash\n")
	b.WriteString("- 信息获取（搜索网页、获取文档）→ webfetch, websearch。聚焦可执行结论，不要堆砌原始数据。\n")
	b.WriteString("- 技能调用（匹配已注册技能的专业场景）→ skill\n")
	b.WriteString("- 复杂并行任务（多个独立子任务）→ parallel_dispatch 或 spawn_agent\n\n")
	b.WriteString("## 迭代执行\n\n")
	b.WriteString("工具调用循环：调用工具 → 观察结果 → 决定下一步。每轮最多调用 5 个工具，避免过度并行。\n\n")
	b.WriteString("## 不确定时的处理\n\n")
	b.WriteString("任务意图不明确时，使用 question 工具确认意图，而不是猜测执行。\n\n")
	b.WriteString("## 代码编辑规范\n\n")
	b.WriteString("- 修改代码前先读取文件了解上下文\n")
	b.WriteString("- 优先使用 edit/multiedit 做精确修改，避免整文件重写\n")
	b.WriteString("- 修改后验证编译或测试通过\n\n")
	b.WriteString("## 运维安全规范\n\n")
	b.WriteString("- 执行破坏性操作前务必确认\n")
	b.WriteString("- 优先使用只读命令了解当前状态\n\n")
	b.WriteString("## 回复规范\n\n")
	b.WriteString("- 直接回答问题，不要解释推理过程（除非用户要求）\n")
	b.WriteString("- 执行操作后，简要报告结果，不要复述操作步骤\n\n")
	b.WriteString("## Artifact 输出规范\n\n")
	b.WriteString("当用户请求你生成以下类型的内容时，必须用 <artifact> 标签包裹，不要直接铺在回复里：\n")
	b.WriteString("- 完整文档（方案、报告、大纲、故事、剧本等，超过 300 字的结构化内容）\n")
	b.WriteString("- 完整代码文件（超过 20 行的独立可运行代码）\n")
	b.WriteString("- HTML 页面\n")
	b.WriteString("- PPT/幻灯片大纲（结构化多页内容）\n\n")
	b.WriteString("标签格式：<artifact type=\"markdown\" title=\"文档标题\">内容</artifact>\n")
	b.WriteString("代码文件：<artifact type=\"code\" language=\"python\" title=\"脚本名称\">内容</artifact>\n")
	b.WriteString("type 可选值：markdown | html | code | ppt\n")
	b.WriteString("title 不超过 30 字，不要包含引号字符。\n\n")
	b.WriteString("对话解释、简短回答、步骤说明、代码修改说明等不需要标签，直接回复即可。\n")
	b.WriteString("当用户请求修改现有文件时（工具调用路径），说明改了什么，不要用 artifact 标签包裹。\n")
	b.WriteString("不要在解释性文字中直接输出 artifact 标签示例，用文字描述代替。\n")
	b.WriteString("artifact 标签必须独立成段（前后各一个空行），不要嵌入列表、表格或代码块内部。\n\n")
	b.WriteString("## 防幻觉规范\n\n")
	b.WriteString("- 引用外部信息时标注来源（URL 或文件路径）\n")
	b.WriteString("- 对不确定的信息标记不确定性，不要伪造数据\n\n")
	b.WriteString("## 研究彻底性\n\n")
	b.WriteString("- 调研任务需多角度发现，不要只看第一个结果\n\n")
	b.WriteString("## spawn_agent 使用规范\n\n")
	b.WriteString("spawn_agent 是同步的：调用后等待子 Agent 完成才返回结果。最多同时 3 个子代理。\n\n")
	b.WriteString("## explore Agent 使用规范\n\n")
	b.WriteString("代码库探索任务通过 task 工具委派给 explore Agent，不要自己逐文件读取。\n\n")
	return b.String()
}

type systemPromptBuild struct {
	Content string
	Metas   []i18n.PromptMeta
}

func (b systemPromptBuild) Versions() []string {
	out := make([]string, 0, len(b.Metas))
	for _, meta := range b.Metas {
		out = append(out, meta.Key+"@"+meta.Source+"@"+meta.Hash)
	}
	return out
}

func (m *Master) buildSystemPrompt(tools []mcphost.ToolDefinition) string {
	return m.buildSystemPromptWithMeta(tools).Content
}

func (m *Master) buildSystemPromptWithMeta(tools []mcphost.ToolDefinition) systemPromptBuild {
	var b strings.Builder
	var metas []i18n.PromptMeta

	// nil 防御：promptLoader 未注入时使用硬编码默认值
	if m.promptLoader == nil {
		hardcoded := m.buildSystemPromptHardcoded()
		b.WriteString(hardcoded)
		metas = append(metas, i18n.PromptMeta{
			Key:    "system/hardcoded",
			Source: "hardcoded",
			Hash:   i18n.HashPromptForQuality(hardcoded),
		})
	} else {
		// 三层优先级加载核心 prompt 段落
		writePrompt := func(key, fallback string) {
			v := m.promptLoader.LoadWithMetaOrDefault(key, fallback)
			b.WriteString(v.Content)
			metas = append(metas, v.Meta)
		}
		writePrompt("system/base", "## 身份定义\n\n你是 Hive，一个具备工具调用能力的 AI 助手。\n\n")
		writePrompt("system/execution", "")
		writePrompt("system/business", "") // 域 F：业务场景识别段（可选）
		writePrompt("system/code_editing", "")
		writePrompt("system/safety", "")
		writePrompt("system/reply", "")
		b.WriteString("\n")
	}

	b.WriteString(m.buildToolPrompt(tools))
	return systemPromptBuild{Content: b.String(), Metas: metas}
}

func (m *Master) buildToolPrompt(tools []mcphost.ToolDefinition) string {
	var b strings.Builder

	// 可用工具提示（仅列出核心工具，减少 system prompt 体积）
	if len(tools) > 0 {
		b.WriteString("## 可用工具\n")
		b.WriteString("你可以使用以下核心工具完成任务：\n")
		for _, t := range tools {
			if t.Core {
				b.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Name, t.Description))
			}
		}
		b.WriteString("\n更多扩展工具默认不进入候选集。需要不常用、外部 MCP 或自定义能力时，先调用 **tool_search** 按需发现；发现后的工具会在后续轮次进入可调用列表。\n\n")

		// 外部 MCP 工具提示（按服务端分组，帮助 LLM 了解可用的外部集成）
		externalTools := make(map[string][]mcphost.ToolDefinition)
		for _, t := range tools {
			if parts := strings.SplitN(t.Name, "__", 2); len(parts) == 2 {
				externalTools[parts[0]] = append(externalTools[parts[0]], t)
			}
		}
		if len(externalTools) > 0 {
			b.WriteString("## 外部集成工具\n")
			b.WriteString("以下是通过 MCP 协议集成的外部工具，可直接调用：\n")
			for server, serverTools := range externalTools {
				b.WriteString(fmt.Sprintf("### %s\n", server))
				for _, st := range serverTools {
					b.WriteString(fmt.Sprintf("- **%s**: %s\n", st.Name, st.Description))
				}
			}
			b.WriteString("\n")

			// 外部 MCP 工具专属规范（从 PromptLoader 动态加载）
			for mcpName := range externalTools {
				if m.promptLoader != nil {
					if toolPrompt := m.promptLoader.Load(fmt.Sprintf("tools/%s", mcpName)); toolPrompt != "" {
						b.WriteString(toolPrompt)
						b.WriteString("\n")
					}
				} else if mcpName == "wenyan" {
					// 硬编码 wenyan fallback
					b.WriteString("#### wenyan 发布规范\n")
					b.WriteString("调用 wenyan__publish_article 时，文章内容（content 参数）必须包含 YAML frontmatter，其中 title 字段为必填。\n\n")
				}
			}
		}
	}

	// 外部操作引导
	b.WriteString("## 外部操作\n\n")
	b.WriteString("直接使用 bash 和 webfetch 工具完成外部操作。\n")
	b.WriteString("已配置的外部资源连接信息见下方。\n\n")

	// spawn_agent 使用规范（从 PromptLoader 加载）
	if m.promptLoader != nil {
		if spawnPrompt := m.promptLoader.Load("tools/spawn_agent"); spawnPrompt != "" {
			b.WriteString(spawnPrompt)
			b.WriteString("\n")
		}
	} else {
		b.WriteString("## spawn_agent 使用规范\n\n")
		b.WriteString("spawn_agent 是同步的：调用后等待子 Agent 完成才返回结果。最多同时 3 个子代理。\n\n")
	}

	// 动态工具创建引导（从 PromptLoader 加载）
	if m.promptLoader != nil {
		if dynPrompt := m.promptLoader.Load("tools/dynamic_tools"); dynPrompt != "" {
			b.WriteString(dynPrompt)
			b.WriteString("\n")
		}
	} else {
		b.WriteString("## 动态工具创建\n")
		b.WriteString("需要重复调用某个外部 API 或命令时，使用 create_tool 创建专用工具。\n\n")
	}

	// 注入已配置的外部资源
	if resources := m.getExternalResources(); len(resources) > 0 {
		b.WriteString("### 已配置的外部资源\n")
		b.WriteString("以下外部资源已由用户预配置，创建专用 Agent 时请直接使用对应的连接信息：\n")
		for _, res := range resources {
			if !res.Enabled {
				continue
			}
			b.WriteString(fmt.Sprintf("- **%s** [%s][%s]: %s\n", res.Name, res.Type, res.Environment, res.Description))
			if res.Connection != "" {
				b.WriteString(fmt.Sprintf("  连接: `%s`\n", res.Connection))
			}
			if res.Endpoint != "" {
				b.WriteString(fmt.Sprintf("  端点: `%s`\n", res.Endpoint))
			}
			if res.ReadOnly {
				b.WriteString("  （只读模式）\n")
			}
		}
		b.WriteString("\n")
	}

	// Skills 提示
	if m.skillReg != nil {
		if available := m.skillReg.ListForModel(); len(available) > 0 {
			b.WriteString("## 可用技能（Skills）\n\n")
			b.WriteString("使用 skill 工具调用以下技能。根据描述判断何时使用：\n\n")
			for _, sm := range available {
				b.WriteString(fmt.Sprintf("- **%s**: %s", sm.Name, sm.Description))
				if sm.ArgumentHint != "" {
					b.WriteString(fmt.Sprintf(" | 参数: `%s`", sm.ArgumentHint))
				}
				if sm.Domain != "" {
					b.WriteString(fmt.Sprintf(" | 领域: %s", sm.Domain))
				}
				if len(sm.TriggerKeywords) > 0 {
					b.WriteString(fmt.Sprintf(" | 触发词: %s", strings.Join(sm.TriggerKeywords, ", ")))
				}
				if sm.Context == "fork" {
					b.WriteString(" | [隔离执行]")
				}
				if sm.Model != "" {
					b.WriteString(fmt.Sprintf(" | 模型: %s", sm.Model))
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	// 追加用户自定义指令（从 .claw/AGENTS.md 或 CLAUDE.md 加载）
	if m.promptCtx.Instructions() != "" {
		b.WriteString("## 项目指令\n")
		b.WriteString(m.promptCtx.Instructions())
		b.WriteString("\n\n")
	}

	// 注入动态上下文（模型感知格式：Git 状态、日期、OS 信息）
	if m.promptCtx.PromptManager() != nil {
		contextBlock := i18n.NewPromptBuilder(m.promptCtx.PromptManager(), m.logger).
			WithProvider(m.promptCtx.ProviderKey()).
			WithGitStatus().
			WithCurrentDate().
			WithOSInfo().
			BuildContext()
		if contextBlock != "" {
			b.WriteString(contextBlock)
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

// buildCompactionPipeline 根据配置构建可插拔压缩管线（P2-2）
func (m *Master) buildCompactionPipeline() *compaction.Pipeline {
	cfg := m.config.ContextCompression

	// 统一构建 TokenCounter（tiktoken 模式下所有 compactor 共享同一实例）
	var tc *llm.TokenCounter
	if cfg.UseTiktoken {
		var err error
		tc, err = llm.NewTokenCounter()
		if err != nil {
			m.logger.Warn("tiktoken 初始化失败，降级到启发式估算", zap.Error(err))
		}
	}

	// 注册所有可用的 Compactor
	registry := map[string]compaction.Compactor{}

	// tool_budget: 工具输出截断
	registry["tool_budget"] = &compaction.ToolResultBudgetCompactor{
		ProtectedTurns:  PruneProtectedTurns,
		OutputThreshold: cfg.ToolOutputMaxTokens,
		ContextBudget:   cfg.ToolOutputMaxTokens * 2,
	}

	// session_memory: 提取会话记忆
	registry["session_memory"] = &compaction.SessionMemoryCompactor{
		MaxExtractMessages: 20,
	}

	// history_snip: 首尾保留，中间压缩
	registry["history_snip"] = &compaction.HistorySnipCompactor{
		KeepFirst: true,
		KeepLast:  4,
	}

	// llm_summary: LLM 智能摘要
	registry["llm_summary"] = m.buildLLMSummaryCompactor(cfg, tc)

	// truncate: 简单截断（兜底）
	registry["truncate"] = &compaction.TruncateCompactor{
		UseTiktoken:  cfg.UseTiktoken,
		TokenCounter: tc,
	}

	// 使用配置的阶段列表；空列表则用默认值
	stages := cfg.PipelineStages
	if len(stages) == 0 {
		stages = config.DefaultCompactionPipelineStages
	}

	pipeline, skipped := compaction.NewPipeline(registry, stages)
	if len(skipped) > 0 {
		m.logger.Warn("压缩管线跳过未知阶段",
			zap.Strings("skipped_stages", skipped),
			zap.Strings("configured_stages", stages))
	}
	if pipeline == nil || len(pipeline.StageNames()) == 0 {
		m.logger.Warn("压缩管线为空，禁用压缩",
			zap.Strings("configured_stages", cfg.PipelineStages))
	}
	return pipeline
}

// buildLLMSummaryCompactor 构建 LLM 摘要压缩器
func (m *Master) buildLLMSummaryCompactor(cfg config.CompactionConfig, tc *llm.TokenCounter) *compaction.LLMSummaryCompactor {
	var llmClient *llm.Client
	if m.router != nil {
		llmClient = m.router.GetLLMClient(airouter.TaskSummary)
	} else {
		provDef := llm.LookupProvider(m.config.Provider)
		provDef.APIFormat = m.config.APIFormat
		llmClient = m.llmPool.Get(llm.ClientConfig{
			Model:           m.config.Model,
			Provider:        provDef,
			BaseURL:         m.config.BaseURL,
			APIKey:          m.config.APIKey,
			DisableJSONMode: m.config.DisableJSONMode,
			ReasoningEffort: m.config.ReasoningEffort,
			StorePrivacy:    m.config.StorePrivacy,
		})
	}
	return &compaction.LLMSummaryCompactor{
		LLMClient:    llmClient,
		TokenCounter: tc,
		UseTiktoken:  cfg.UseTiktoken,
		Timeout:      cfg.CompactTimeout,
		Logger:       m.logger,
	}
}

// prepareMessagesWithCompression 准备发送给 LLM 的消息（应用智能压缩）
// P2-2: 优先使用可插拔压缩管线，向后兼容旧策略
//
// harden-spec-driven-phase2 task 3.8：signature 增加 session 参数——pipeline 结束后调
// PreserveSpecStateOnCompact 注入 [SPEC-STATE] pin，保证 spec-driven 会话的 change_id/
// current_task_key/revision 不因为 truncate/summary 丢失 LLM 感知。session 可为 nil（测试路径），
// 此时 preservation 是 no-op——与原 messages-only 语义等价。
func (m *Master) prepareMessagesWithCompression(ctx context.Context, session *SessionState, messages []llm.MessageWithTools) []llm.MessageWithTools {
	cfg := m.config.ContextCompression
	if !cfg.Enabled {
		return messages
	}

	// 计算 token 数（用于懒惰模式和日志）
	var totalTokens int
	if cfg.UseTiktoken {
		tc, err := llm.NewTokenCounter()
		if err != nil {
			m.logger.Warn("tiktoken 初始化失败，降级到启发式估算", zap.Error(err))
			totalTokens = llm.EstimateMessagesTokens(messages)
		} else {
			totalTokens = tc.CountMessages(messages)
		}
	} else {
		totalTokens = llm.EstimateMessagesTokens(messages)
	}

	// tool_budget 裁剪：无论是否 LazyMode 都先执行，防止大型工具输出绕过压缩
	// （LazyMode 早返回会跳过整个 pipeline，但 tool output 过大仍需裁剪）
	toolBudgetCompactor := &compaction.ToolResultBudgetCompactor{
		ProtectedTurns:  PruneProtectedTurns,
		OutputThreshold: cfg.ToolOutputMaxTokens,
		ContextBudget:   cfg.ToolOutputMaxTokens * 2,
	}
	budget0 := cfg.MaxTokens - cfg.ReserveTokens
	if budget0 <= 0 {
		budget0 = cfg.MaxTokens
	}
	if trimmed, err := toolBudgetCompactor.Compact(ctx, messages, budget0); err == nil {
		messages = trimmed
	}

	// 懒惰模式：未超阈值则跳过（tool_budget 已在上方执行）
	if cfg.LazyMode && totalTokens <= cfg.LazyThreshold {
		m.compactionTracker.RecordSkipped()
		return messages
	}

	// 触发 PreCompact hook
	if m.pluginMgr != nil {
		hookCtx, cancel := context.WithTimeout(ctx, plugin.HookCallTimeout)
		_ = m.pluginMgr.TriggerPreCompact(hookCtx, &plugin.CompactInput{
			MessageCount: len(messages),
		})
		cancel()
	}

	// 使用可插拔压缩管线（每次调用时构建，确保 LLM client 始终最新）
	pipeline := m.buildCompactionPipeline()

	budget := cfg.MaxTokens - cfg.ReserveTokens
	if budget <= 0 {
		budget = cfg.MaxTokens
	}

	originalCount := len(messages)
	startTime := time.Now()

	result, err := pipeline.Compact(ctx, messages, budget)
	elapsed := time.Since(startTime)

	if err != nil {
		m.logger.Warn("压缩管线执行失败，回退到原始消息", zap.Error(err))
		result = messages
	}

	remainingCount := len(result)

	// 更新统计
	if remainingCount < originalCount {
		m.compactionTracker.RecordTrigger(elapsed)
		avgDelay := m.GetCompactionStats().AverageDelay
		m.logger.Info("上下文已压缩",
			zap.Int("original", originalCount),
			zap.Int("remaining", remainingCount),
			zap.Strings("stages", pipeline.StageNames()),
			zap.Int("original_tokens", totalTokens),
			zap.Duration("elapsed", elapsed),
			zap.Duration("avg_delay", avgDelay),
		)
	}

	// 触发 PostCompact hook
	if m.pluginMgr != nil {
		hookCtx, cancel := context.WithTimeout(ctx, plugin.HookCallTimeout)
		_ = m.pluginMgr.TriggerPostCompact(hookCtx, &plugin.CompactInput{
			MessageCount: remainingCount,
		})
		cancel()
	}

	// harden-spec-driven-phase2 task 3.8：pipeline 结束后注入 [SPEC-STATE] pin。
	// 顺序必须在 PostCompact hook 之后——hook 看到的是 pipeline 真实产物（pin 不算压缩
	// 对象），插件计数不会把 pin 当成"新增一条被压缩的消息"。
	result = PreserveSpecStateOnCompact(session, result)

	return result
}
