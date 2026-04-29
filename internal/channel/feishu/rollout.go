package feishu

type DeterministicRollout struct{}

func (DeterministicRollout) Allow(mode GovernanceRolloutMode) bool {
	return mode != RolloutModeDeny
}
