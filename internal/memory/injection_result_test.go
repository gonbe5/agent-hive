package memory

import "testing"

func TestInjectionResultMemoryIDs(t *testing.T) {
	result := InjectionResult{
		Memories: []InjectedMemory{
			{ID: 3, Type: MemoryTypeUser},
			{ID: 0, Type: MemoryTypeProject},
			{ID: 7, Type: MemoryTypeFeedback},
		},
	}

	got := result.MemoryIDs()
	if len(got) != 2 || got[0] != 3 || got[1] != 7 {
		t.Fatalf("MemoryIDs() = %#v, want [3 7]", got)
	}
}

func TestInjectionResultSkippedTotalAndHasSignal(t *testing.T) {
	empty := InjectionResult{}
	if empty.SkippedTotal() != 0 {
		t.Fatalf("SkippedTotal() = %d, want 0", empty.SkippedTotal())
	}
	if empty.HasSignal() {
		t.Fatal("empty result should not have signal")
	}

	filtered := InjectionResult{
		SkippedExpired:     1,
		SkippedLowTrust:    2,
		SkippedCrossUser:   3,
		SkippedTokenBudget: 4,
	}
	if filtered.SkippedTotal() != 10 {
		t.Fatalf("SkippedTotal() = %d, want 10", filtered.SkippedTotal())
	}
	if !filtered.HasSignal() {
		t.Fatal("filtered-only result should have signal for quality events")
	}

	injected := InjectionResult{Memories: []InjectedMemory{{ID: 1, Type: MemoryTypeUser}}}
	if !injected.HasSignal() {
		t.Fatal("injected result should have signal")
	}
}
