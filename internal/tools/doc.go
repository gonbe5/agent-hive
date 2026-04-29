// Package tools 实现了 agents-hive 的内置工具集。
//
// 提供 MCP 工具供 LLM 在执行任务时调用，包括：
// 文件操作、代码搜索、命令执行、补丁应用、网络工具、批量操作、LSP 集成、自定义工具。
//
// 通过 RegisterBuiltinTools 注册所有内置工具到 MCP Host。
// ShellPool 管理持久化 shell 会话，ReadTracker 防止未读文件被覆盖。
package tools
