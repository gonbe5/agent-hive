package feishu

import "testing"

func TestRollout_DenyDropsNormalMessage(t *testing.T) {
	r := DeterministicRollout{}
	if allowed := r.Allow(RolloutModeDeny); allowed {
		t.Fatal("deny mode must drop normal messages")
	}
}
