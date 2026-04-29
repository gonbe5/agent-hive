package imctx_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/chef-guo/agents-hive/internal/imctx"
)

func TestBuildSessionID_Format(t *testing.T) {
	got, err := imctx.BuildSessionID(imctx.PlatformFeishu, "tk-abc", "oc-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "im-feishu-tk_abc-oc_xyz"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if !strings.HasPrefix(got, imctx.SessionIDPrefix+"-") {
		t.Fatalf("session_id %q must start with prefix %q", got, imctx.SessionIDPrefix)
	}
}

func TestBuildSessionID_RejectsEmpty(t *testing.T) {
	cases := []struct {
		name              string
		platform          imctx.Platform
		tenantKey, chatID string
	}{
		{"empty platform", "", "tk", "chat"},
		{"empty tenant", imctx.PlatformFeishu, "", "chat"},
		{"empty chat", imctx.PlatformFeishu, "tk", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := imctx.BuildSessionID(c.platform, c.tenantKey, c.chatID)
			if !errors.Is(err, imctx.ErrEmptySessionPart) {
				t.Fatalf("want ErrEmptySessionPart, got %v", err)
			}
		})
	}
}

func TestBuildSessionID_DashEscape(t *testing.T) {
	// tenant 含 "-" 时必须转为 "_"，否则 4 段会被解析成 5 段
	got, err := imctx.BuildSessionID(imctx.PlatformFeishu, "a-b", "c-d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "im-feishu-a_b-c_d" {
		t.Fatalf("dash should be escaped to underscore, got %q", got)
	}
	parts := strings.Split(got, "-")
	if len(parts) != 4 {
		t.Fatalf("session_id must split into exactly 4 segments, got %d (%q)", len(parts), got)
	}
}
