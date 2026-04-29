package assistantcap

import (
	"testing"
)

func TestGrantPass(t *testing.T) {
	const passValue = 0
	cap, ok := GrantPass(0, passValue)
	if !ok {
		t.Fatal("GrantPass should issue when action == passValue")
	}
	if cap == nil {
		t.Fatal("GrantPass returned ok=true but cap is nil")
	}

	cap2, ok2 := GrantPass(1, passValue)
	if ok2 {
		t.Fatal("GrantPass must NOT issue when action != passValue")
	}
	if cap2 != nil {
		t.Fatal("GrantPass returned ok=false but cap is not nil")
	}
}

func TestGrantStream(t *testing.T) {
	cap, ok := GrantStream("auto", "required")
	if !ok {
		t.Fatal("GrantStream should issue when toolChoice != required")
	}
	if cap == nil {
		t.Fatal("GrantStream returned ok=true but cap is nil")
	}

	cap2, ok2 := GrantStream("required", "required")
	if ok2 {
		t.Fatal("GrantStream must NOT issue when toolChoice == required")
	}
	if cap2 != nil {
		t.Fatal("GrantStream returned ok=false but cap is not nil")
	}
}

// TestGrantedSentinelStable 锁定 granted 单例每次返回同一个值，
// 防止有人改成每次堆分配新实例（既浪费 GC，也使 == 比较失效）。
func TestGrantedSentinelStable(t *testing.T) {
	c1, _ := GrantPass(0, 0)
	c2, _ := GrantPass(0, 0)
	if c1 != c2 {
		t.Fatal("granted sentinel must be a stable single value")
	}
}

// TestCapabilityIsInterface 锁定 Capability 是 interface 类型 —— 这是
// cross-package compile-time unforgeability 的关键。若有人改成 struct，
// 跨包就能用 `pkg.Capability{}` 伪造，整个结构性锁失效。
//
// 间接验证：把一个 nil 接口值通过函数返回后赋给 Capability 不应 panic，
// 且 GrantPass(nonpass) 必须返回 nil 接口；二者形态都依赖 interface 语义。
func TestCapabilityIsInterface(t *testing.T) {
	c, ok := GrantPass(1, 0) // not pass
	if ok {
		t.Fatal("expected GrantPass to return ok=false for non-pass action")
	}
	// 若 Capability 是 struct，下面这行会报 "cannot use untyped nil"，编译失败
	if c != nil {
		t.Fatal("non-pass GrantPass must return nil Capability")
	}
}
