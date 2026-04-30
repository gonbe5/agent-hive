package eval

import (
	"context"
	"testing"
)

func TestRunCasesSummarizesFixtures(t *testing.T) {
	summary, err := RunCases(context.Background(), "testdata")
	if err != nil {
		t.Fatalf("RunCases returned error: %v", err)
	}
	if summary.Total != 3 {
		t.Fatalf("Total = %d, want 3", summary.Total)
	}
	if summary.RequiredTotal != 3 {
		t.Fatalf("RequiredTotal = %d, want 3", summary.RequiredTotal)
	}
	if summary.Passed != 3 || summary.RequiredPassed != 3 {
		t.Fatalf("summary = %+v, want all cases passed", summary)
	}
	if len(summary.Results) != 3 {
		t.Fatalf("len(Results) = %d, want 3", len(summary.Results))
	}
}
