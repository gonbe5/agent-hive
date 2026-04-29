package security

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// ClassifyResult LLM 分类结果
type ClassifyResult struct {
	Safe   bool   // true = 无需 HITL 自动放行；false = 需要人工审批
	Reason string // 判断依据（用于日志）
}

// LLMClassifier 使用 LLM 语义分析工具调用是否需要人工审批。
// 对标 Claude Code 的 classifyYoloAction。
// 分类失败时默认返回 Safe=false（fail closed，安全侧）。
type LLMClassifier struct {
	client *llm.Client
	logger *zap.Logger
}

// NewLLMClassifier 创建 LLMClassifier。client 不得为 nil。
func NewLLMClassifier(client *llm.Client, logger *zap.Logger) *LLMClassifier {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LLMClassifier{client: client, logger: logger}
}

// classifyResponse LLM 返回的 JSON 结构
type classifyResponse struct {
	Safe   bool   `json:"safe"`
	Reason string `json:"reason"`
}

// classifySystemPrompt 是分类器的系统提示，指导 LLM 做出安全判断。
// 设计原则：
//   - 保守：拿不准就返回 safe=false，宁可多一次 HITL 也不要漏掉危险操作
//   - 快速：prompt 尽量短，减少 token 消耗
const classifySystemPrompt = `你是一个 AI 代理的安全分类器，判断一个工具调用是否可以不经过人工审批自动执行。

## 安全（safe=true）的条件
满足以下「所有」条件才能返回 safe=true：
- 操作完全可逆，或者只读
- 影响范围仅限于代理的工作目录/会话内部
- 不涉及系统级修改（不改 /etc、/usr、不修改 PATH、不安装软件）
- 不涉及网络请求（无 curl、wget、fetch 等出站操作）
- 不删除、覆盖或移动文件到工作目录之外
- 不创建或修改可执行文件、脚本、定时任务、服务
- 不涉及凭证、密钥、Token 的读写或传输

## 危险（safe=false）的示例
- rm -rf、删除非临时文件
- 网络请求（curl、wget、nc、ssh、scp 等）
- sudo、su、chmod 777、chown、setuid
- 修改系统文件或 shell 配置
- 写入 ~/.ssh、~/.bashrc、/etc/cron 等
- 任何形式的代码注入或 eval
- 读取敏感文件（/etc/shadow、~/.ssh/id_*、*.pem 等）

## 输出格式
只返回 JSON，不要其他内容：
{"safe": true或false, "reason": "一句话说明理由"}`

// Classify 判断工具调用是否安全（无需人工审批）。
// toolName: 工具名称
// input:    工具原始 JSON 输入参数
// 返回 ClassifyResult；出错时 Safe=false（fail closed）。
func (c *LLMClassifier) Classify(ctx context.Context, toolName string, input json.RawMessage) ClassifyResult {
	// 构造用户消息，描述待分类的工具调用
	inputPreview := string(input)
	if len(inputPreview) > 512 {
		inputPreview = inputPreview[:512] + "...(truncated)"
	}
	userMsg := fmt.Sprintf("工具名: %s\n参数: %s", toolName, inputPreview)

	req := llm.ChatRequest{
		SystemPrompt: classifySystemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: llm.NewTextContent(userMsg)},
		},
		Temperature: 0,    // 确定性输出
		MaxTokens:   128,  // 分类任务不需要长输出
		JSONMode:    true,
	}

	resp, err := c.client.Chat(ctx, req)
	if err != nil {
		c.logger.Warn("LLM 分类器调用失败，默认拒绝",
			zap.String("tool", toolName),
			zap.Error(err),
		)
		return ClassifyResult{Safe: false, Reason: "classifier error: " + err.Error()}
	}

	var result classifyResponse
	if jsonErr := json.Unmarshal([]byte(resp.Content), &result); jsonErr != nil {
		c.logger.Warn("LLM 分类器响应解析失败，默认拒绝",
			zap.String("tool", toolName),
			zap.String("raw", resp.Content),
			zap.Error(jsonErr),
		)
		return ClassifyResult{Safe: false, Reason: "classifier parse error"}
	}

	c.logger.Debug("LLM 分类器结果",
		zap.String("tool", toolName),
		zap.Bool("safe", result.Safe),
		zap.String("reason", result.Reason),
	)

	return ClassifyResult{Safe: result.Safe, Reason: result.Reason}
}
