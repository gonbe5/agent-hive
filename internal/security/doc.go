// Package security 实现了 agents-hive 的安全执行机制。
//
// 提供基于规则的命令执行策略控制（allow/ask/deny），防止恶意或危险命令执行。
// 支持通配符模式匹配、环境变量校验和命令参数安全解析。
//
// SafeExecutor 是核心类型，通过 ExecRule 列表匹配命令并决定执行策略。
package security
