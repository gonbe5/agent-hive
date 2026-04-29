package eval

import (
	"context"
	"strings"
	"testing"

	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// noopRunner 是仅用于 validate 正向测试的 Runner 占位——
// 真 behavior gate test 需要 TG2+ 的真实实现注入,不归本包职责。
type noopRunner struct{}

func (noopRunner) ResolveContinuation(context.Context, Case) (specdriven.Decision, error) {
	return specdriven.Decision{Kind: specdriven.DecisionNew}, nil
}
func (noopRunner) Plan(context.Context, Case) (*specdriven.Plan, error) {
	return &specdriven.Plan{}, nil
}
func (noopRunner) ExecuteFallback(context.Context, Case) error { return nil }

// TestEvalFixtures 是 schema gate——总是运行,不依赖 Runner。
// 职责:
// 1) 扫描 testdata/*.json,严格解码 + EOF 检查
// 2) 断言 fm01~fm08 必选集完整且每个 Required:true
// 3) 对每条 fixture 做静态 schema 校验（decision 合法、输入非空等）
//
// 这层 gate 在 TG2+ behavior gate wiring 前就能独立守护 fixture 质量：
// 任何 PR 改坏 fixture schema、漏掉 fm01~fm08、或写错 required 标记,
// 当前 target 都会立即红。这是 spec-eval-harness scenario
// "Fixture decoding is strict" + "Required fixture set covers FM-1 to FM-8" 的兑现。
func TestEvalFixtures(t *testing.T) {
	cases, err := LoadAll("testdata")
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no fixtures found under testdata/")
	}
	if err := RequiredSetComplete(cases); err != nil {
		t.Fatalf("required-set check failed: %v", err)
	}

	var required, optional int
	for _, lc := range cases {
		t.Run(lc.Base(), func(t *testing.T) {
			if err := ValidateSchema(lc.Case); err != nil {
				t.Fatalf("%s schema invalid: %v", lc.Base(), err)
			}
		})
		if lc.Case.Required {
			required++
		} else {
			optional++
		}
	}
	t.Logf("schema gate: required=%d optional=%d total=%d", required, optional, len(cases))
}

// TestHarnessFailClosed 验证 P0-1/P0-6 修复:Runner 未注入时 validate 必须报错。
// 这是防"假门"回潮的自测锚点——如果未来有人偷偷放宽 nil 检查,
// 这个 test 会在关键词层面捕获（"nil" + "fail-closed" 必须出现在错误消息里）。
func TestHarnessFailClosed(t *testing.T) {
	h := Harness{} // Runner 故意 nil
	err := h.validate()
	if err == nil {
		t.Fatal("Harness.validate with nil Runner did not return error — fail-closed contract broken")
	}
	msg := err.Error()
	for _, needle := range []string{"Runner", "nil", "fail-closed"} {
		if !strings.Contains(msg, needle) {
			t.Fatalf("fail-closed error missing keyword %q: got %q", needle, msg)
		}
	}
}

// TestHarnessValidateAcceptsRunner 正向用例:注入 noopRunner 后 validate 放行。
func TestHarnessValidateAcceptsRunner(t *testing.T) {
	h := Harness{Runner: noopRunner{}}
	if err := h.validate(); err != nil {
		t.Fatalf("validate with non-nil Runner returned error: %v", err)
	}
}
