package feishu

import "testing"

func TestNormalizeCommand_StripsZeroWidthAndCasing(t *testing.T) {
	got := NormalizeCommand("/ReSeT\u200b ")
	if got != "/reset" {
		t.Fatalf("want /reset, got %q", got)
	}
}

func TestParseCommand_WhitelistOnly(t *testing.T) {
	cmd, ok := ParseCommand("/status")
	if !ok || cmd == nil {
		t.Fatal("expected /status to be recognized")
	}
	if cmd.Name != "status" {
		t.Fatalf("want status, got %q", cmd.Name)
	}
	if cmd.Arg != "" {
		t.Fatalf("want empty arg, got %q", cmd.Arg)
	}

	cmd, ok = ParseCommand("/mute 10m")
	if !ok || cmd == nil {
		t.Fatal("expected /mute 10m to be recognized")
	}
	if cmd.Name != "mute" {
		t.Fatalf("want mute, got %q", cmd.Name)
	}
	if cmd.Arg != "10m" {
		t.Fatalf("want arg 10m, got %q", cmd.Arg)
	}

	cmd, ok = ParseCommand("/model gpt-5.2")
	if !ok || cmd == nil {
		t.Fatal("expected /model gpt-5.2 to be recognized")
	}
	if cmd.Name != "model" {
		t.Fatalf("want model, got %q", cmd.Name)
	}
	if cmd.Arg != "gpt-5.2" {
		t.Fatalf("want arg gpt-5.2, got %q", cmd.Arg)
	}

	cmd, ok = ParseCommand("/debug on")
	if !ok || cmd == nil {
		t.Fatal("expected /debug on to be recognized")
	}
	if cmd.Name != "debug" {
		t.Fatalf("want debug, got %q", cmd.Name)
	}
	if cmd.Arg != "on" {
		t.Fatalf("want arg on, got %q", cmd.Arg)
	}

	cmd, ok = ParseCommand("/audit last 5")
	if !ok || cmd == nil {
		t.Fatal("expected /audit last 5 to be recognized")
	}
	if cmd.Name != "audit" {
		t.Fatalf("want audit, got %q", cmd.Name)
	}
	if cmd.Arg != "last" {
		t.Fatalf("want arg last, got %q", cmd.Arg)
	}
	if len(cmd.Args) != 2 || cmd.Args[0] != "last" || cmd.Args[1] != "5" {
		t.Fatalf("want args [last 5], got %#v", cmd.Args)
	}

	if _, ok := ParseCommand("/unknown"); ok {
		t.Fatal("unexpected unknown command acceptance")
	}
}
