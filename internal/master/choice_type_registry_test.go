package master

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
)

// resetChoiceTypeRegistryForTest 清空 registry 并让调用方自行注册，避免 init()
// 内置 3 值污染单测断言。测试结束后通过 t.Cleanup 恢复。
func resetChoiceTypeRegistryForTest(t *testing.T) {
	t.Helper()
	choiceTypeMu.Lock()
	backup := make(map[string]ChoiceTypeSpec, len(choiceTypeRegistry))
	for k, v := range choiceTypeRegistry {
		backup[k] = v
	}
	choiceTypeRegistry = map[string]ChoiceTypeSpec{}
	choiceTypeMu.Unlock()
	t.Cleanup(func() {
		choiceTypeMu.Lock()
		choiceTypeRegistry = backup
		choiceTypeMu.Unlock()
	})
}

func TestBuiltinChoiceTypes_RegisteredAtInit(t *testing.T) {
	for _, name := range []string{
		"account_selector",
		"ambiguity_clarification",
		"confirmation_before_irreversible_business_action",
	} {
		if !IsRegisteredChoiceType(name) {
			t.Errorf("built-in %q MUST be registered at init()", name)
		}
	}
	if IsRegisteredChoiceType("skill_install_confirmation") {
		t.Error("skill_install_confirmation MUST NOT be registered by master init (owned by hive-skill-on-demand)")
	}
}

func TestRegisterChoiceType_Idempotent(t *testing.T) {
	resetChoiceTypeRegistryForTest(t)
	spec := ChoiceTypeSpec{Name: "test_idem", Description: "x"}
	if err := RegisterChoiceType(spec); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := RegisterChoiceType(spec); err != nil {
		t.Fatalf("second identical register must be idempotent, got: %v", err)
	}
	if got := ListChoiceTypes(); len(got) != 1 {
		t.Errorf("want 1 entry after idempotent register, got %d", len(got))
	}
}

func TestRegisterChoiceType_Conflict(t *testing.T) {
	resetChoiceTypeRegistryForTest(t)
	if err := RegisterChoiceType(ChoiceTypeSpec{Name: "test_conf", Description: "A"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := RegisterChoiceType(ChoiceTypeSpec{Name: "test_conf", Description: "B"})
	if !errors.Is(err, ErrChoiceTypeAlreadyRegisteredDifferent) {
		t.Fatalf("want ErrChoiceTypeAlreadyRegisteredDifferent, got %v", err)
	}
}

// TestRegisterAndList_PayloadHintDeepCopy 证明 PayloadHint 在 Register 与 List
// 两处都被深拷贝，外部修改不会污染 registry（review findings #1 闭环）。
func TestRegisterAndList_PayloadHintDeepCopy(t *testing.T) {
	resetChoiceTypeRegistryForTest(t)
	callerHint := map[string]string{"k": "v"}
	if err := RegisterChoiceType(ChoiceTypeSpec{Name: "with_hint", PayloadHint: callerHint}); err != nil {
		t.Fatal(err)
	}
	// 外部 mutate 原始 map，registry 必须纹丝不动
	callerHint["k"] = "tampered"
	callerHint["new"] = "injected"

	got := ListChoiceTypes()
	var found *ChoiceTypeSpec
	for i := range got {
		if got[i].Name == "with_hint" {
			found = &got[i]
			break
		}
	}
	if found == nil {
		t.Fatal("spec not found")
	}
	if found.PayloadHint["k"] != "v" || len(found.PayloadHint) != 1 {
		t.Errorf("registry polluted by external map mutation: %+v", found.PayloadHint)
	}

	// 再 mutate 返回值，再 List 一次，registry 仍不动
	found.PayloadHint["k"] = "again_tampered"
	again := ListChoiceTypes()
	for _, s := range again {
		if s.Name == "with_hint" && s.PayloadHint["k"] != "v" {
			t.Errorf("registry polluted by List-return mutation: %+v", s.PayloadHint)
		}
	}
}

func TestRegisterChoiceType_InvalidName(t *testing.T) {
	resetChoiceTypeRegistryForTest(t)
	cases := []string{"MyType", "my-type", "", "123abc", "_leading_underscore", "UPPER"}
	for _, name := range cases {
		err := RegisterChoiceType(ChoiceTypeSpec{Name: name})
		if !errors.Is(err, ErrChoiceTypeNameInvalid) {
			t.Errorf("name=%q want ErrChoiceTypeNameInvalid, got %v", name, err)
		}
	}
}

func TestListChoiceTypes_Sorted(t *testing.T) {
	resetChoiceTypeRegistryForTest(t)
	for _, n := range []string{"zebra", "alpha", "mango"} {
		if err := RegisterChoiceType(ChoiceTypeSpec{Name: n, Description: n}); err != nil {
			t.Fatal(err)
		}
	}
	got := ListChoiceTypes()
	names := make([]string, len(got))
	for i, s := range got {
		names[i] = s.Name
	}
	want := []string{"alpha", "mango", "zebra"}
	if !sort.StringsAreSorted(names) {
		t.Errorf("ListChoiceTypes must return alphabetical, got %v", names)
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("want %v, got %v", want, names)
	}
}

func TestChoiceTypeRegistry_Concurrent(t *testing.T) {
	resetChoiceTypeRegistryForTest(t)
	const goroutines = 100
	const perG = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				name := fmt.Sprintf("c_%d_%d", g, i)
				if err := RegisterChoiceType(ChoiceTypeSpec{Name: name}); err != nil {
					t.Errorf("register %q: %v", name, err)
				}
			}
		}(g)
	}
	wg.Wait()
	got := ListChoiceTypes()
	if len(got) != goroutines*perG {
		t.Errorf("want %d entries, got %d", goroutines*perG, len(got))
	}
}
