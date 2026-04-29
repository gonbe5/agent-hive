package imctx_test

import (
	"testing"

	"github.com/chef-guo/agents-hive/internal/imctx"
)

func TestSafeSenderID_EmptyReturnsEmpty(t *testing.T) {
	if got := imctx.SafeSenderID(""); got != "" {
		t.Fatalf("空输入必须返回空，得到 %q", got)
	}
}

func TestSafeSenderID_NotPlaintext(t *testing.T) {
	const raw = "ou_open_real_user"
	out := imctx.SafeSenderID(raw)
	if out == raw {
		t.Fatal("禁止把 raw open_id 原样返回")
	}
}

func TestSafeSenderID_Length8Hex(t *testing.T) {
	out := imctx.SafeSenderID("ou_open_real_user")
	if len(out) != 8 {
		t.Fatalf("sha256[:4] = 8 hex chars，得到 %d (%q)", len(out), out)
	}
	for _, r := range out {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			t.Fatalf("输出必须是 lowercase hex，含非法字符 %q (%q)", r, out)
		}
	}
}

func TestSafeSenderID_Deterministic(t *testing.T) {
	a := imctx.SafeSenderID("ou_open_real_user")
	b := imctx.SafeSenderID("ou_open_real_user")
	if a != b {
		t.Fatalf("相同输入必须确定性，得到 %q vs %q", a, b)
	}
}

func TestSafeSenderID_DifferentInputsDiverge(t *testing.T) {
	a := imctx.SafeSenderID("ou_open_real_user")
	b := imctx.SafeSenderID("ou_other_user")
	if a == b {
		t.Fatalf("不同输入必须产生不同 hash（碰撞风险），得到相同的 %q", a)
	}
}
