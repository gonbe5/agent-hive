package memory

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

// 编译期接口合规检查
var _ MemoryExtractor = (*Extractor)(nil)

// Extractor 从压缩摘要中自动提取记忆
type Extractor struct {
	store  MemoryStore
	logger *zap.Logger
}

// NewExtractor 创建记忆提取器
func NewExtractor(store MemoryStore, logger *zap.Logger) *Extractor {
	return &Extractor{
		store:  store,
		logger: logger,
	}
}

// ExtractFromSummary 从压缩摘要文本中提取并保存记忆
// summaryText 是 compaction 生成的 LLM 摘要
// sessionID 是来源会话
// userID 是记忆归属用户
func (e *Extractor) ExtractFromSummary(ctx context.Context, summaryText string, sessionID string, userID string) error {
	if summaryText == "" {
		return nil
	}

	facts := e.parseFacts(summaryText)
	if len(facts) == 0 {
		e.logger.Debug("摘要中未提取到记忆", zap.String("session_id", sessionID))
		return nil
	}

	autoTags := []string{"auto-extracted", "compaction"}
	saved := 0

	for _, fact := range facts {
		// 检查是否已存在相似记忆（去重，限定在同一用户范围内）
		if e.isDuplicate(ctx, fact.content, userID) {
			e.logger.Debug("跳过重复记忆", zap.String("content", fact.content))
			continue
		}

		record := &MemoryRecord{
			Type:      fact.memType,
			Content:   fact.content,
			Tags:      autoTags,
			SessionID: sessionID,
			UserID:    userID,
		}

		if _, err := e.store.Save(ctx, record); err != nil {
			e.logger.Warn("保存提取的记忆失败",
				zap.String("content", fact.content),
				zap.Error(err),
			)
			continue
		}
		saved++
	}

	e.logger.Info("从摘要中提取记忆完成",
		zap.Int("extracted", len(facts)),
		zap.Int("saved", saved),
		zap.String("session_id", sessionID),
	)
	return nil
}

// extractedFact 提取出的事实
type extractedFact struct {
	content string
	memType MemoryType
}

// 目标/决策相关关键词
var projectKeywords = []string{
	"目标", "决策", "计划", "架构", "设计", "方案", "策略",
	"实现", "完成", "修复", "重构", "优化", "部署",
	"goal", "decision", "plan", "architecture", "design",
}

// 用户偏好相关关键词
var userKeywords = []string{
	"偏好", "喜欢", "习惯", "风格", "用户",
	"prefer", "like", "style", "user",
}

// 文件/文档引用相关关键词
var referenceKeywords = []string{
	"文件", "路径", "文档", "链接", "配置",
	"file", "path", "doc", "config", ".go", ".ts", ".json", ".yaml", ".yml",
}

// parseFacts 从摘要文本中解析事实条目
func (e *Extractor) parseFacts(text string) []extractedFact {
	var facts []extractedFact

	lines := strings.Split(text, "\n")
	currentSection := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// 检测章节标题
		if strings.HasPrefix(trimmed, "#") || strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "：") {
			currentSection = strings.ToLower(trimmed)
			continue
		}

		// 提取要点行（以 - 或 * 或数字序号开头）
		content := extractBulletContent(trimmed)
		if content == "" {
			continue
		}

		// 过滤太短的内容
		if len(content) < 5 {
			continue
		}

		memType := classifyFact(content, currentSection)
		facts = append(facts, extractedFact{
			content: content,
			memType: memType,
		})
	}

	return facts
}

// extractBulletContent 提取要点行的内容
// 支持格式：- 内容、* 内容、1. 内容、1) 内容
func extractBulletContent(line string) string {
	// Markdown 无序列表
	if strings.HasPrefix(line, "- ") {
		return strings.TrimSpace(line[2:])
	}
	if strings.HasPrefix(line, "* ") {
		return strings.TrimSpace(line[2:])
	}

	// 有序列表：1. 内容 或 1) 内容
	for i, ch := range line {
		if ch >= '0' && ch <= '9' {
			continue
		}
		if i > 0 && (ch == '.' || ch == ')') && i+1 < len(line) && line[i+1] == ' ' {
			return strings.TrimSpace(line[i+2:])
		}
		break
	}

	return ""
}

// classifyFact 基于内容和所在章节分类事实类型
func classifyFact(content string, section string) MemoryType {
	lower := strings.ToLower(content)
	sectionLower := strings.ToLower(section)

	// 优先检查文件引用
	for _, kw := range referenceKeywords {
		if strings.Contains(lower, kw) || strings.Contains(sectionLower, kw) {
			return MemoryTypeReference
		}
	}

	// 检查用户偏好
	for _, kw := range userKeywords {
		if strings.Contains(lower, kw) || strings.Contains(sectionLower, kw) {
			return MemoryTypeUser
		}
	}

	// 检查项目目标/决策
	for _, kw := range projectKeywords {
		if strings.Contains(lower, kw) || strings.Contains(sectionLower, kw) {
			return MemoryTypeProject
		}
	}

	// 默认归类为项目记忆
	return MemoryTypeProject
}

// isDuplicate 检查是否已存在相似内容的记忆（限定在同一用户范围内）
func (e *Extractor) isDuplicate(ctx context.Context, content string, userID string) bool {
	result, err := e.store.Search(ctx, SearchOptions{
		Query:  content,
		Limit:  1,
		UserID: userID,
	})
	if err != nil {
		return false
	}

	if result == nil || len(result.Memories) == 0 {
		return false
	}

	// 使用简单的内容相似度检查：完全匹配或子串包含
	existing := result.Memories[0].Content
	if existing == content {
		return true
	}
	if strings.Contains(existing, content) || strings.Contains(content, existing) {
		return true
	}

	return false
}
