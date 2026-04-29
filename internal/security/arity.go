package security

import (
	"fmt"
	"strings"
)

// CommandArity 命令分类信息
type CommandArity struct {
	Prefix      string // 命令前缀
	Category    string // 分类：package_manager, file_system, network, vcs, build, editor, dangerous
	Risk        string // 风险等级：low, medium, high
	Description string // 中文描述
	ReadOnly    bool   // 是否为只读操作
}

// categoryLabels 分类中文标签
var categoryLabels = map[string]string{
	"package_manager": "包管理器",
	"file_system":     "文件系统",
	"network":         "网络",
	"vcs":             "版本控制",
	"build":           "构建",
	"editor":          "编辑器",
	"dangerous":       "危险操作",
}

// riskLabels 风险等级中文标签
var riskLabels = map[string]string{
	"low":    "低风险",
	"medium": "中风险",
	"high":   "高风险",
}

// arityTable 命令分类表
var arityTable = map[string]CommandArity{
	// 包管理器
	"npm install":     {Prefix: "npm install", Category: "package_manager", Risk: "low", Description: "安装 npm 依赖", ReadOnly: false},
	"npm run":         {Prefix: "npm run", Category: "package_manager", Risk: "low", Description: "运行 npm 脚本", ReadOnly: false},
	"npm test":        {Prefix: "npm test", Category: "package_manager", Risk: "low", Description: "运行 npm 测试", ReadOnly: true},
	"npm ls":          {Prefix: "npm ls", Category: "package_manager", Risk: "low", Description: "查看 npm 依赖", ReadOnly: true},
	"npm":             {Prefix: "npm", Category: "package_manager", Risk: "low", Description: "npm 包管理", ReadOnly: false},
	"yarn":            {Prefix: "yarn", Category: "package_manager", Risk: "low", Description: "yarn 包管理", ReadOnly: false},
	"pnpm":            {Prefix: "pnpm", Category: "package_manager", Risk: "low", Description: "pnpm 包管理", ReadOnly: false},
	"pip install":     {Prefix: "pip install", Category: "package_manager", Risk: "low", Description: "安装 Python 依赖", ReadOnly: false},
	"pip":             {Prefix: "pip", Category: "package_manager", Risk: "low", Description: "pip 包管理", ReadOnly: false},
	"go get":          {Prefix: "go get", Category: "package_manager", Risk: "low", Description: "获取 Go 依赖", ReadOnly: false},
	"go mod":          {Prefix: "go mod", Category: "package_manager", Risk: "low", Description: "Go 模块管理", ReadOnly: false},
	"cargo":           {Prefix: "cargo", Category: "package_manager", Risk: "low", Description: "Rust 包管理", ReadOnly: false},
	"brew":            {Prefix: "brew", Category: "package_manager", Risk: "medium", Description: "Homebrew 包管理", ReadOnly: false},
	"apt":             {Prefix: "apt", Category: "package_manager", Risk: "medium", Description: "APT 包管理", ReadOnly: false},
	"apt-get":         {Prefix: "apt-get", Category: "package_manager", Risk: "medium", Description: "APT 包管理", ReadOnly: false},
	"gem":             {Prefix: "gem", Category: "package_manager", Risk: "low", Description: "Ruby Gem 包管理", ReadOnly: false},
	"composer":        {Prefix: "composer", Category: "package_manager", Risk: "low", Description: "PHP Composer 包管理", ReadOnly: false},

	// 文件系统 - 只读
	"ls":              {Prefix: "ls", Category: "file_system", Risk: "low", Description: "列出目录内容", ReadOnly: true},
	"cat":             {Prefix: "cat", Category: "file_system", Risk: "low", Description: "查看文件内容", ReadOnly: true},
	"head":            {Prefix: "head", Category: "file_system", Risk: "low", Description: "查看文件头部", ReadOnly: true},
	"tail":            {Prefix: "tail", Category: "file_system", Risk: "low", Description: "查看文件尾部", ReadOnly: true},
	"find":            {Prefix: "find", Category: "file_system", Risk: "low", Description: "搜索文件", ReadOnly: true},
	"wc":              {Prefix: "wc", Category: "file_system", Risk: "low", Description: "统计字数/行数", ReadOnly: true},
	"du":              {Prefix: "du", Category: "file_system", Risk: "low", Description: "统计磁盘用量", ReadOnly: true},
	"df":              {Prefix: "df", Category: "file_system", Risk: "low", Description: "查看磁盘空间", ReadOnly: true},
	"file":            {Prefix: "file", Category: "file_system", Risk: "low", Description: "查看文件类型", ReadOnly: true},
	"stat":            {Prefix: "stat", Category: "file_system", Risk: "low", Description: "查看文件状态", ReadOnly: true},
	"tree":            {Prefix: "tree", Category: "file_system", Risk: "low", Description: "树形显示目录", ReadOnly: true},

	// 文件系统 - 写入
	"mkdir":           {Prefix: "mkdir", Category: "file_system", Risk: "low", Description: "创建目录", ReadOnly: false},
	"cp":              {Prefix: "cp", Category: "file_system", Risk: "medium", Description: "复制文件", ReadOnly: false},
	"mv":              {Prefix: "mv", Category: "file_system", Risk: "medium", Description: "移动/重命名文件", ReadOnly: false},
	"rm":              {Prefix: "rm", Category: "file_system", Risk: "high", Description: "删除文件", ReadOnly: false},
	"chmod":           {Prefix: "chmod", Category: "file_system", Risk: "medium", Description: "修改文件权限", ReadOnly: false},
	"chown":           {Prefix: "chown", Category: "file_system", Risk: "medium", Description: "修改文件所有者", ReadOnly: false},
	"touch":           {Prefix: "touch", Category: "file_system", Risk: "low", Description: "创建/更新文件时间戳", ReadOnly: false},
	"ln":              {Prefix: "ln", Category: "file_system", Risk: "low", Description: "创建链接", ReadOnly: false},

	// 网络
	"curl":            {Prefix: "curl", Category: "network", Risk: "medium", Description: "HTTP 请求", ReadOnly: false},
	"wget":            {Prefix: "wget", Category: "network", Risk: "medium", Description: "下载文件", ReadOnly: false},
	"ssh":             {Prefix: "ssh", Category: "network", Risk: "high", Description: "SSH 远程连接", ReadOnly: false},
	"scp":             {Prefix: "scp", Category: "network", Risk: "high", Description: "SCP 远程拷贝", ReadOnly: false},
	"ping":            {Prefix: "ping", Category: "network", Risk: "low", Description: "网络连通性测试", ReadOnly: true},
	"nc":              {Prefix: "nc", Category: "network", Risk: "high", Description: "netcat 网络工具", ReadOnly: false},
	"dig":             {Prefix: "dig", Category: "network", Risk: "low", Description: "DNS 查询", ReadOnly: true},
	"nslookup":        {Prefix: "nslookup", Category: "network", Risk: "low", Description: "DNS 查询", ReadOnly: true},

	// 版本控制
	"git status":      {Prefix: "git status", Category: "vcs", Risk: "low", Description: "查看 Git 状态", ReadOnly: true},
	"git log":         {Prefix: "git log", Category: "vcs", Risk: "low", Description: "查看 Git 日志", ReadOnly: true},
	"git diff":        {Prefix: "git diff", Category: "vcs", Risk: "low", Description: "查看 Git 差异", ReadOnly: true},
	"git branch":      {Prefix: "git branch", Category: "vcs", Risk: "low", Description: "管理 Git 分支", ReadOnly: false},
	"git add":         {Prefix: "git add", Category: "vcs", Risk: "low", Description: "暂存文件", ReadOnly: false},
	"git commit":      {Prefix: "git commit", Category: "vcs", Risk: "low", Description: "提交变更", ReadOnly: false},
	"git push":        {Prefix: "git push", Category: "vcs", Risk: "medium", Description: "推送到远程", ReadOnly: false},
	"git pull":        {Prefix: "git pull", Category: "vcs", Risk: "medium", Description: "从远程拉取", ReadOnly: false},
	"git checkout":    {Prefix: "git checkout", Category: "vcs", Risk: "medium", Description: "切换分支", ReadOnly: false},
	"git merge":       {Prefix: "git merge", Category: "vcs", Risk: "medium", Description: "合并分支", ReadOnly: false},
	"git rebase":      {Prefix: "git rebase", Category: "vcs", Risk: "high", Description: "变基操作", ReadOnly: false},
	"git reset":       {Prefix: "git reset", Category: "vcs", Risk: "high", Description: "重置提交", ReadOnly: false},
	"git stash":       {Prefix: "git stash", Category: "vcs", Risk: "low", Description: "暂存工作区", ReadOnly: false},
	"git clone":       {Prefix: "git clone", Category: "vcs", Risk: "low", Description: "克隆仓库", ReadOnly: false},
	"git":             {Prefix: "git", Category: "vcs", Risk: "low", Description: "Git 版本控制", ReadOnly: false},

	// 构建工具
	"make":            {Prefix: "make", Category: "build", Risk: "low", Description: "Make 构建", ReadOnly: false},
	"cmake":           {Prefix: "cmake", Category: "build", Risk: "low", Description: "CMake 构建", ReadOnly: false},
	"gcc":             {Prefix: "gcc", Category: "build", Risk: "low", Description: "GCC 编译", ReadOnly: false},
	"go build":        {Prefix: "go build", Category: "build", Risk: "low", Description: "Go 编译", ReadOnly: false},
	"go test":         {Prefix: "go test", Category: "build", Risk: "low", Description: "Go 测试", ReadOnly: true},
	"go run":          {Prefix: "go run", Category: "build", Risk: "low", Description: "Go 运行", ReadOnly: false},
	"go vet":          {Prefix: "go vet", Category: "build", Risk: "low", Description: "Go 代码检查", ReadOnly: true},
	"cargo build":     {Prefix: "cargo build", Category: "build", Risk: "low", Description: "Rust 编译", ReadOnly: false},
	"cargo test":      {Prefix: "cargo test", Category: "build", Risk: "low", Description: "Rust 测试", ReadOnly: true},
	"docker build":    {Prefix: "docker build", Category: "build", Risk: "medium", Description: "Docker 构建镜像", ReadOnly: false},
	"docker run":      {Prefix: "docker run", Category: "build", Risk: "medium", Description: "Docker 运行容器", ReadOnly: false},
	"docker":          {Prefix: "docker", Category: "build", Risk: "medium", Description: "Docker 容器管理", ReadOnly: false},
	"python":          {Prefix: "python", Category: "build", Risk: "low", Description: "Python 运行", ReadOnly: false},
	"node":            {Prefix: "node", Category: "build", Risk: "low", Description: "Node.js 运行", ReadOnly: false},

	// 文本处理
	"grep":            {Prefix: "grep", Category: "file_system", Risk: "low", Description: "文本搜索", ReadOnly: true},
	"sed":             {Prefix: "sed", Category: "file_system", Risk: "medium", Description: "流编辑器", ReadOnly: false},
	"awk":             {Prefix: "awk", Category: "file_system", Risk: "low", Description: "文本处理", ReadOnly: true},
	"sort":            {Prefix: "sort", Category: "file_system", Risk: "low", Description: "排序", ReadOnly: true},
	"uniq":            {Prefix: "uniq", Category: "file_system", Risk: "low", Description: "去重", ReadOnly: true},
	"xargs":           {Prefix: "xargs", Category: "file_system", Risk: "medium", Description: "参数传递", ReadOnly: false},

	// 编辑器
	"vim":             {Prefix: "vim", Category: "editor", Risk: "low", Description: "Vim 编辑器", ReadOnly: false},
	"nano":            {Prefix: "nano", Category: "editor", Risk: "low", Description: "Nano 编辑器", ReadOnly: false},
	"code":            {Prefix: "code", Category: "editor", Risk: "low", Description: "VS Code 编辑器", ReadOnly: false},
	"emacs":           {Prefix: "emacs", Category: "editor", Risk: "low", Description: "Emacs 编辑器", ReadOnly: false},

	// 危险操作
	"rm -rf":          {Prefix: "rm -rf", Category: "dangerous", Risk: "high", Description: "递归强制删除", ReadOnly: false},
	"dd":              {Prefix: "dd", Category: "dangerous", Risk: "high", Description: "磁盘数据拷贝", ReadOnly: false},
	"mkfs":            {Prefix: "mkfs", Category: "dangerous", Risk: "high", Description: "格式化文件系统", ReadOnly: false},
	"fdisk":           {Prefix: "fdisk", Category: "dangerous", Risk: "high", Description: "磁盘分区", ReadOnly: false},
	"shutdown":        {Prefix: "shutdown", Category: "dangerous", Risk: "high", Description: "关机", ReadOnly: false},
	"reboot":          {Prefix: "reboot", Category: "dangerous", Risk: "high", Description: "重启", ReadOnly: false},
	"sudo":            {Prefix: "sudo", Category: "dangerous", Risk: "high", Description: "提权执行", ReadOnly: false},
	"su":              {Prefix: "su", Category: "dangerous", Risk: "high", Description: "切换用户", ReadOnly: false},
}

// FormatPermissionPrompt 格式化命令权限提示信息
// 根据命令在 arityTable 中的分类信息，返回友好的描述。
// 例如：
//   - "npm install lodash" → "npm install lodash [包管理器·低风险]"
//   - "git push origin main" → "git push origin main [版本控制·中风险]"
//   - "unknown-cmd arg1" → "unknown-cmd arg1"
func FormatPermissionPrompt(rawCmd string) string {
	rawCmd = strings.TrimSpace(rawCmd)
	if rawCmd == "" {
		return rawCmd
	}

	// 将命令按空格分词
	parts := strings.Fields(rawCmd)
	if len(parts) == 0 {
		return rawCmd
	}

	// 尝试从最长的前缀开始匹配（最多取前 3 个词）
	maxParts := len(parts)
	if maxParts > 3 {
		maxParts = 3
	}

	for i := maxParts; i >= 1; i-- {
		prefix := strings.Join(parts[:i], " ")
		if arity, ok := arityTable[prefix]; ok {
			catLabel := categoryLabels[arity.Category]
			if catLabel == "" {
				catLabel = arity.Category
			}
			riskLabel := riskLabels[arity.Risk]
			if riskLabel == "" {
				riskLabel = arity.Risk
			}

			suffix := catLabel
			if arity.ReadOnly {
				suffix += "·只读"
			} else {
				suffix += "·" + riskLabel
			}

			return fmt.Sprintf("%s [%s]", rawCmd, suffix)
		}
	}

	// 未匹配到任何分类，返回原始命令
	return rawCmd
}
