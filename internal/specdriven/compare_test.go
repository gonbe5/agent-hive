package specdriven

import "testing"

// TestCompareTaskKey 覆盖 CompareTaskKey 的核心约束:
//  1. 数字段按数值比较 ("1.10" > "1.9" 而非字符串 lex 的 <)——防 JSON
//     反序列化把 1.10 / 1.1 坍塌成同一 float64 所带来的歧义
//  2. 前缀更短的 key 小于更长但前缀相同的 key ("1" < "1.1")
//  3. 相等 key 返回 0
//
// 这是 add-spec-driven-cognition Phase 1 设计里"task_key 必须是 string"
// 的底层保护——否则 "1.10" 和 "1.1" 在 JSON 层就已经没救了。
func TestCompareTaskKey(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.1", "1.2", -1},
		{"1.2", "1.1", 1},
		{"1.1", "1.1", 0},
		{"1.10", "1.9", 1},  // 关键反坑: 数值 10 > 9
		{"1.9", "1.10", -1}, // 对称
		{"2.1", "1.99", 1},  // 顶层段优先
		{"1", "1.1", -1},    // 前缀短者更小
		{"1.1", "1", 1},
		{"1.2.3", "1.2.3", 0},
		{"", "", 0},
		{"", "1", -1},
		{"1", "", 1},
	}
	for _, c := range cases {
		if got := CompareTaskKey(c.a, c.b); got != c.want {
			t.Errorf("CompareTaskKey(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
