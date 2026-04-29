package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// AgentDefinition 描述一个通过 .md 文件定义的自定义 Agent
type AgentDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Mode        string   `yaml:"mode"`            // "primary", "subagent", "all"
	Model       string   `yaml:"model,omitempty"` // 可选模型覆盖
	Temperature float64  `yaml:"temperature,omitempty"`
	MaxSteps    int      `yaml:"max_steps,omitempty"`
	Tools       []string `yaml:"tools,omitempty"` // 允许的工具列表
	Prompt      string   `yaml:"-"`               // Markdown body 作为系统提示词（不从 YAML 解析）
}

// Validate 校验 AgentDefinition 字段的合法性
func (d *AgentDefinition) Validate() error {
	if d.Name == "" {
		return errs.New(errs.CodeInvalidInput, "agent 定义缺少 name 字段")
	}
	switch d.Mode {
	case "", "primary", "subagent", "all":
		// 合法值
	default:
		return errs.New(errs.CodeInvalidInput, fmt.Sprintf("agent %q 的 mode 值无效: %q（允许: primary, subagent, all）", d.Name, d.Mode))
	}
	if d.Temperature < 0 || d.Temperature > 2 {
		if d.Temperature != 0 { // 0 表示未设置
			return errs.New(errs.CodeInvalidInput, fmt.Sprintf("agent %q 的 temperature 值超出范围 [0, 2]: %f", d.Name, d.Temperature))
		}
	}
	if d.MaxSteps < 0 {
		return errs.New(errs.CodeInvalidInput, fmt.Sprintf("agent %q 的 max_steps 不能为负数", d.Name))
	}
	return nil
}

// LoadAgentDefinitions 扫描指定目录下的 .md 文件，解析为 AgentDefinition 列表。
//
// 每个 .md 文件的格式为：
//
//	---
//	name: my-agent
//	description: 自定义 Agent
//	mode: subagent
//	---
//	这里是系统提示词内容...
func LoadAgentDefinitions(dir string, logger *zap.Logger) ([]AgentDefinition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("自定义 Agent 目录不存在，跳过加载", zap.String("dir", dir))
			return nil, nil
		}
		return nil, errs.Wrap(errs.CodeInternal, "读取 Agent 定义目录失败", err)
	}

	var defs []AgentDefinition
	seen := make(map[string]string) // name -> 文件路径，用于检测重复

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		def, err := parseAgentFile(filePath)
		if err != nil {
			logger.Warn("解析 Agent 定义文件失败，跳过",
				zap.String("file", filePath),
				zap.Error(err),
			)
			continue
		}

		if err := def.Validate(); err != nil {
			logger.Warn("Agent 定义校验失败，跳过",
				zap.String("file", filePath),
				zap.Error(err),
			)
			continue
		}

		// 检测重复名称
		if prevFile, exists := seen[def.Name]; exists {
			logger.Warn("Agent 名称重复，后者覆盖前者",
				zap.String("name", def.Name),
				zap.String("prev_file", prevFile),
				zap.String("curr_file", filePath),
			)
		}
		seen[def.Name] = filePath

		defs = append(defs, *def)
		logger.Info("加载自定义 Agent 定义",
			zap.String("name", def.Name),
			zap.String("mode", def.Mode),
			zap.String("file", filePath),
		)
	}

	return defs, nil
}

// parseAgentFile 解析单个 Agent .md 文件，提取 YAML frontmatter 和 Markdown body
func parseAgentFile(path string) (*AgentDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "读取 Agent 文件失败", err)
	}

	frontmatter, body, err := splitFrontmatter(string(data))
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidInput, fmt.Sprintf("解析文件 %s 的 frontmatter 失败", filepath.Base(path)), err)
	}

	var def AgentDefinition
	if err := yaml.Unmarshal([]byte(frontmatter), &def); err != nil {
		return nil, errs.Wrap(errs.CodeInvalidInput, fmt.Sprintf("解析文件 %s 的 YAML 失败", filepath.Base(path)), err)
	}

	def.Prompt = strings.TrimSpace(body)

	// 如果没有设置 mode，默认为 "subagent"
	if def.Mode == "" {
		def.Mode = "subagent"
	}

	return &def, nil
}

// splitFrontmatter 从 Markdown 文本中分离 YAML frontmatter 和正文。
//
// 格式要求：文件以 "---" 开头，以下一个 "---" 结束 frontmatter 区域。
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	scanner := bufio.NewScanner(strings.NewReader(content))

	// 第一行必须是 "---"
	if !scanner.Scan() {
		return "", "", fmt.Errorf("文件为空")
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return "", "", fmt.Errorf("文件缺少 YAML frontmatter 开始标记 (---)")
	}

	// 读取 frontmatter 直到遇到结束的 "---"
	var fmLines []string
	foundEnd := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			foundEnd = true
			break
		}
		fmLines = append(fmLines, line)
	}

	if !foundEnd {
		return "", "", fmt.Errorf("文件缺少 YAML frontmatter 结束标记 (---)")
	}

	// 剩余部分作为 body
	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", "", fmt.Errorf("读取文件内容失败: %w", err)
	}

	return strings.Join(fmLines, "\n"), strings.Join(bodyLines, "\n"), nil
}
