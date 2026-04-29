package skillhitl

import (
	"testing"

	"github.com/chef-guo/agents-hive/internal/master"
)

// TestSkillInstall_ChoiceTypeRegistered is the BLOCKER 2 regression trap.
// If someone deletes or typos the init()-time MustRegisterChoiceType in
// install_hitl.go, this test goes red immediately. Runtime behavior:
// Master.EmitInputRequest hard-fails with ErrUnregisteredChoiceType when a
// skill_install handler tries to emit HITL — so a missing registration is a
// 100%-failure path the moment the feature is used in production.
func TestSkillInstall_ChoiceTypeRegistered(t *testing.T) {
	if !master.IsRegisteredChoiceType(ChoiceTypeSkillInstallConfirmation) {
		t.Fatalf(
			"choice_type %q is NOT registered — skill_install HITL emit would hard-fail at runtime (BLOCKER 2)",
			ChoiceTypeSkillInstallConfirmation,
		)
	}
}

// TestSkillInstall_ChoiceTypeIdempotent — re-registering the same spec is a
// no-op. Guards against a future refactor that might duplicate the register.
func TestSkillInstall_ChoiceTypeIdempotent(t *testing.T) {
	err := master.RegisterChoiceType(master.ChoiceTypeSpec{
		Name:        ChoiceTypeSkillInstallConfirmation,
		Description: "User approves or declines an on-demand skill installation",
		PayloadHint: map[string]string{
			"name":           "skill name being installed",
			"scope":          "personal|public",
			"source":         "marketplace URL",
			"admin_required": "bool — whether scope=public required admin",
		},
	})
	if err != nil {
		t.Fatalf("re-registering identical spec should be idempotent, got: %v", err)
	}
}

// TestSkillInstall_ChoiceTypeRejectsSpecDrift — rogue re-registration with a
// different spec must fail. Regression defense against silent spec drift.
func TestSkillInstall_ChoiceTypeRejectsSpecDrift(t *testing.T) {
	err := master.RegisterChoiceType(master.ChoiceTypeSpec{
		Name:        ChoiceTypeSkillInstallConfirmation,
		Description: "a completely different description — should be rejected",
	})
	if err == nil {
		t.Fatal("expected ErrChoiceTypeAlreadyRegisteredDifferent, got nil")
	}
}
