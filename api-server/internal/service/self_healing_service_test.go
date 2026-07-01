package service

import "testing"

func TestHealingAttemptBoundsUsesTaskMaxRounds(t *testing.T) {
	service := &SelfHealingService{maxRetries: 3}

	current, max := service.healingAttemptBounds(HealingTask{
		AttemptNo: 1,
		MaxRounds: 1,
	})

	if current != 1 || max != 1 {
		t.Fatalf("expected attempt bounds 1/1, got %d/%d", current, max)
	}
}

func TestHealingAttemptBoundsFallsBackToServiceMaxRetries(t *testing.T) {
	service := &SelfHealingService{maxRetries: 3}

	current, max := service.healingAttemptBounds(HealingTask{
		AttemptNo: 0,
		MaxRounds: 0,
	})

	if current != 1 || max != 3 {
		t.Fatalf("expected attempt bounds 1/3, got %d/%d", current, max)
	}
}

func TestHealingAttemptBoundsDetectsExceededTaskRounds(t *testing.T) {
	service := &SelfHealingService{maxRetries: 3}

	current, max := service.healingAttemptBounds(HealingTask{
		AttemptNo: 2,
		MaxRounds: 1,
	})

	if current <= max {
		t.Fatalf("expected current attempt to exceed max rounds, got %d/%d", current, max)
	}
}

func TestEffectiveLLMTimeoutHasBaselineMinimum(t *testing.T) {
	service := &SelfHealingService{llmTimeout: 60}

	if got := service.effectiveLLMTimeoutSeconds(); got != baselineHealingMinLLMTimeoutSeconds {
		t.Fatalf("expected timeout floor %d, got %d", baselineHealingMinLLMTimeoutSeconds, got)
	}
}
