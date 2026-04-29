package specdriven

import (
	"strconv"
	"strings"
)

// CompareTaskKey 按 dot-separated int 段对比两个 task_key。
// "1.10" > "1.9" > "1.1"。字符串 lex 序会判错（"1.10" < "1.9"），因此
// 任何涉及 task_key 顺序的业务逻辑必须用本函数，而非 strings.Compare。
// 非法输入（非数字段）按段字符串对比降级，不 panic。
func CompareTaskKey(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := min(len(as), len(bs))
	for i := range n {
		ai, aerr := strconv.Atoi(as[i])
		bi, berr := strconv.Atoi(bs[i])
		if aerr == nil && berr == nil {
			if ai < bi {
				return -1
			}
			if ai > bi {
				return 1
			}
			continue
		}
		if as[i] < bs[i] {
			return -1
		}
		if as[i] > bs[i] {
			return 1
		}
	}
	if len(as) < len(bs) {
		return -1
	}
	if len(as) > len(bs) {
		return 1
	}
	return 0
}
