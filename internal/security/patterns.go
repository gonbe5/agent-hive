package security

import "regexp"

// MatchPattern 使用正则表达式匹配命令字符串
// 例如: "^ls\\b" 匹配 "ls -la" 但不匹配 "lsblk"
//
//	"^git\\s+(status|log|diff)" 匹配 "git status"
//	"rm\\s+-rf" 匹配 "rm -rf /tmp"
//
// 如果 pattern 不是合法正则，返回 false。
func MatchPattern(pattern, command string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(command)
}
