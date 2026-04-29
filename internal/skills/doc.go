// Package skills 实现了 agents-hive 的技能（Skill）系统。
//
// Skill 是可复用的 Markdown 指令包，通过 YAML frontmatter 定义元数据，
// 支持模板变量、动态上下文（!`command`）、脚本执行和生命周期 hooks。
//
// Registry 管理所有已注册的 skills，Finder 负责自动发现，
// ToolBridge 桥接 skill 与 MCP 工具调用。
package skills
