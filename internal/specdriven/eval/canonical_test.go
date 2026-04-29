package eval

import "testing"

// TestCanonicalJSON_IntegerFidelity 锁 Codex R5-1 红线:canonicalJSON 必须
// 通过 json.Decoder.UseNumber() 保留整数字面量,不允许 Marshal→Unmarshal→Marshal
// 链路把 int 坍塌成 float64 丢精度。这一道 gate 是 Sprint 2.1 反例化 fixture
// 能真红的前提——否则 Args 里的整型字段被悄悄改写,反例被糊掉。
//
// 三条 subtest 对应 Sprint 1.1 DONE 断言:
//   - IntStaysInt: 42 不变成 42.0（小整数 regression 护栏）
//   - LargeInt:    9007199254740993 = 2^53+1,pre-fix 会坍塌成 9007199254740992
//   - NestedMap:   嵌套结构递归保真 + key 排序稳定
func TestCanonicalJSON_IntegerFidelity(t *testing.T) {
	t.Run("IntStaysInt", func(t *testing.T) {
		input := map[string]any{"n": 42}
		got, err := canonicalJSON(input)
		if err != nil {
			t.Fatalf("canonicalJSON: %v", err)
		}
		want := `{"n":42}`
		if string(got) != want {
			t.Fatalf("IntStaysInt: want=%s got=%s", want, got)
		}
	})

	t.Run("LargeInt", func(t *testing.T) {
		// 9007199254740993 = 2^53 + 1。float64 mantissa 只有 52 位,
		// 该值无法精确表示,会被坍塌成 9007199254740992。UseNumber 路径
		// 保留 json.Number("9007199254740993") 字符串,Marshal 回去仍是原值。
		input := map[string]any{"n": int64(9007199254740993)}
		got, err := canonicalJSON(input)
		if err != nil {
			t.Fatalf("canonicalJSON: %v", err)
		}
		want := `{"n":9007199254740993}`
		if string(got) != want {
			t.Fatalf("LargeInt precision lost: want=%s got=%s", want, got)
		}
	})

	t.Run("NestedMap", func(t *testing.T) {
		// 嵌套整数也必须保真;同时验 Go json.Marshal 对 map key 按字母序稳定输出。
		input := map[string]any{"a": map[string]any{"c": 2, "b": 1}}
		got, err := canonicalJSON(input)
		if err != nil {
			t.Fatalf("canonicalJSON: %v", err)
		}
		want := `{"a":{"b":1,"c":2}}`
		if string(got) != want {
			t.Fatalf("NestedMap: want=%s got=%s", want, got)
		}
	})
}
