package security

// ParseCommand 将 shell 命令字符串解析为 command + args
// 处理引号、转义等，但不执行 shell 扩展
func ParseCommand(input string) []string {
	var result []string
	var current []byte
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if escaped {
			current = append(current, ch)
			escaped = false
			continue
		}
		switch {
		case ch == '\\' && !inSingle:
			escaped = true
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case ch == ' ' && !inSingle && !inDouble:
			if len(current) > 0 {
				result = append(result, string(current))
				current = current[:0]
			}
		default:
			current = append(current, ch)
		}
	}
	if len(current) > 0 {
		result = append(result, string(current))
	}
	return result
}
