package feishu

import "testing"

// TestInferReceiveIDType 蓝军点:
//
// 把 case "ou_" 改成 "ou_disabled" → ou_xxx 子用例必红(本来识别 open_id 的回 fallback chat_id)。
// 把 default 改成返 "open_id" → oc_xxx 子用例必红(group chat_id 路径被错走 open_id)。
// 把 TrimPrefix 删了 → p2p:ou_xxx 子用例必红(前缀残留)。
func TestInferReceiveIDType(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantType       string
		wantNormalized string
	}{
		{"group chat", "oc_abc123", "chat_id", "oc_abc123"},
		{"open id", "ou_user1", "open_id", "ou_user1"},
		{"user id intl", "on_intl_user", "user_id", "on_intl_user"},
		{"email", "alice@example.com", "email", "alice@example.com"},
		{"p2p prefix剥掉", "p2p:ou_user1", "open_id", "ou_user1"},
		{"empty fallback chat_id", "", "chat_id", ""},
		{"unknown prefix fallback", "weird_xxx", "chat_id", "weird_xxx"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idType, normalized := inferReceiveIDType(tc.input)
			if idType != tc.wantType {
				t.Fatalf("input=%q: type=%q want %q", tc.input, idType, tc.wantType)
			}
			if normalized != tc.wantNormalized {
				t.Fatalf("input=%q: normalized=%q want %q", tc.input, normalized, tc.wantNormalized)
			}
		})
	}
}
