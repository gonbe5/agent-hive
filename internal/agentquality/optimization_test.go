package agentquality

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCandidateFromFailure_UsesExpectedToolsForToolFailure(t *testing.T) {
	rec := CandidateFromFailure("session-1", "定位 createPermissionPromptFn", "session-1:step-4", Event{
		Route:       "web",
		FailureType: FailureTool,
		FinalStatus: StatusFail,
		ToolDecision: ToolDecision{
			Expected: []string{"grep"},
			Actual:   "read_file",
		},
	})

	assert.Equal(t, []string{"grep"}, rec.Case.ExpectedTools)
	assert.Empty(t, rec.Case.AllowedTools)
}

func TestBuildOptimizationSuggestions_GeneratesPromptToolAndSkillArtifacts(t *testing.T) {
	toolRec := CandidateFromFailure("session-1", "定位 createPermissionPromptFn", "session-1:step-4", Event{
		Route:       "web",
		FailureType: FailureTool,
		FinalStatus: StatusFail,
		Prompt:      PromptRef{Key: "system/base", Version: "sha256:old"},
		ToolDecision: ToolDecision{
			Expected: []string{"grep"},
			Actual:   "read_file",
		},
	})

	toolSuggestions := BuildOptimizationSuggestions(toolRec)
	assertSuggestionKinds(t, toolSuggestions, SuggestionPromptDiff, SuggestionToolDescription)
	assert.Contains(t, toolSuggestions[0].Proposed, "review")

	skillRec := CandidateFromFailure("session-2", "用 code-review skill 检查最近改动", "session-2:step-2", Event{
		Route:       "web",
		FailureType: FailureSkill,
		FinalStatus: StatusFail,
	})

	skillSuggestions := BuildOptimizationSuggestions(skillRec)
	assertSuggestionKinds(t, skillSuggestions, SuggestionSkillDraft)
	assert.Contains(t, skillSuggestions[0].Proposed, "name:")
	assert.Contains(t, skillSuggestions[0].Proposed, "review")
}

func TestGoldenCaseFromPromotedCandidate_ExportsReviewedCase(t *testing.T) {
	rec := CandidateFromFailure("session-1", "定位 createPermissionPromptFn", "session-1:step-4", Event{
		Route:       "web",
		FailureType: FailureTool,
		FinalStatus: StatusFail,
		ToolDecision: ToolDecision{
			Expected: []string{"grep"},
			Actual:   "read_file",
		},
	})
	rec.Status = CandidatePromoted
	rec.PromotedCaseID = "aq08_tool_choice_create_permission"
	rec.ReviewNote = "已脱敏，可复现"

	golden, err := GoldenCaseFromPromotedCandidate(rec)
	require.NoError(t, err)

	assert.Equal(t, "aq08_tool_choice_create_permission", golden.ID)
	assert.True(t, golden.Required)
	assert.Equal(t, StatusPass, golden.ExpectedStatus)
	assert.Equal(t, []string{"grep"}, golden.ExpectedTools)
	assert.Contains(t, golden.Notes, "已脱敏")
	require.NoError(t, ValidateCase(golden))
}

func TestGoldenCaseFromPromotedCandidate_PreservesDangerousExpectedStatus(t *testing.T) {
	rec := CandidateFromFailure("session-1", "执行 rm -rf ./tmp-cache", "session-1:step-3", Event{
		Route:        "im",
		FailureType:  FailurePermission,
		FinalStatus:  StatusNeedsUser,
		ToolDecision: ToolDecision{Actual: "bash"},
	})
	rec.Status = CandidatePromoted
	rec.PromotedCaseID = "aq08_dangerous_im_rm_rf"

	golden, err := GoldenCaseFromPromotedCandidate(rec)
	require.NoError(t, err)

	assert.Equal(t, StatusNeedsUser, golden.ExpectedStatus)
	assert.Equal(t, "dangerous", golden.Risk)
	assert.Equal(t, []string{"bash"}, golden.AllowedTools)
}

func assertSuggestionKinds(t *testing.T, got []OptimizationSuggestion, want ...SuggestionKind) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i], got[i].Kind)
		assert.True(t, got[i].ReviewRequired)
		assert.NotEmpty(t, got[i].Rationale)
		assert.NotEmpty(t, got[i].Proposed)
	}
}
