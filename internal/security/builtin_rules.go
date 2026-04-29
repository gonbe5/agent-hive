package security

// BuiltinDangerousRules 硬编码的危险命令规则，不可通过配置关闭。
// 这些规则在用户配置规则之前匹配，优先级最高。
// 计划 D6 要求：高风险工具护栏必须落在工具层/权限层，不接受靠 prompt 提醒。
var BuiltinDangerousRules = []ExecRule{
	// === PolicyDeny: 绝对禁止，无审批通道 ===
	{Pattern: `^rm\s+-rf\s+/$`, Policy: PolicyDeny, Description: "禁止删除根目录"},
	{Pattern: `^rm\s+-rf\s+/\*`, Policy: PolicyDeny, Description: "禁止删除根目录下所有内容"},
	{Pattern: `^mkfs`, Policy: PolicyDeny, Description: "禁止格式化磁盘"},
	{Pattern: `^dd\s+if=.*\s+of=/dev/`, Policy: PolicyDeny, Description: "禁止直接写磁盘设备"},
	{Pattern: `>\s*/dev/sd`, Policy: PolicyDeny, Description: "禁止重定向到磁盘设备"},
	{Pattern: `>\s*/dev/nvme`, Policy: PolicyDeny, Description: "禁止重定向到 NVMe 设备"},
	{Pattern: `:\(\)\s*\{`, Policy: PolicyDeny, Description: "禁止 fork bomb"},

	// === PolicyAsk: 需要人工审批 ===
	{Pattern: `^rm\s+-rf\s+`, Policy: PolicyAsk, Description: "递归强制删除需审批"},
	{Pattern: `^rm\s+-r\s+`, Policy: PolicyAsk, Description: "递归删除需审批"},
	{Pattern: `(?i)^DROP\s+TABLE\s+`, Policy: PolicyAsk, Description: "删表需审批"},
	{Pattern: `(?i)^DROP\s+DATABASE\s+`, Policy: PolicyAsk, Description: "删库需审批"},
	{Pattern: `(?i)^TRUNCATE\s+`, Policy: PolicyAsk, Description: "清空表需审批"},
	{Pattern: `^git\s+push\s+--force`, Policy: PolicyAsk, Description: "强制推送需审批"},
	{Pattern: `^git\s+push\s+-f\s+`, Policy: PolicyAsk, Description: "强制推送需审批"},
	{Pattern: `^git\s+reset\s+--hard`, Policy: PolicyAsk, Description: "硬重置需审批"},
	{Pattern: `^kubectl\s+delete\s+`, Policy: PolicyAsk, Description: "K8s 删除需审批"},
	{Pattern: `^docker\s+rm\s+-f\s+`, Policy: PolicyAsk, Description: "强制删除容器需审批"},
	{Pattern: `^docker\s+rmi\s+-f\s+`, Policy: PolicyAsk, Description: "强制删除镜像需审批"},
	{Pattern: `^chmod\s+777\s+`, Policy: PolicyAsk, Description: "全开权限需审批"},
}
