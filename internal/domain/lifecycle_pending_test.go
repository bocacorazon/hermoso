package domain

import (
	"testing"
)

func TestStatusAwaitingJudgmentConstant(t *testing.T) {
	if StatusAwaitingJudgment != RunStatus("awaiting_judgment") {
		t.Fatalf("StatusAwaitingJudgment = %q, want %q", StatusAwaitingJudgment, "awaiting_judgment")
	}
}

func TestStatusAllowedVerificationAwaitingJudgment(t *testing.T) {
	if !statusAllowed(PhaseVerification, StatusAwaitingJudgment) {
		t.Fatal("statusAllowed should accept PhaseVerification/StatusAwaitingJudgment")
	}
}

func TestValidateTransitionInProgressToAwaitingJudgment(t *testing.T) {
	if err := ValidateTransition(PhaseVerification, StatusInProgress, PhaseVerification, StatusAwaitingJudgment); err != nil {
		t.Fatalf("in_progress -> awaiting_judgment should be legal: %v", err)
	}
}

func TestValidateTransitionAwaitingJudgmentToInProgress(t *testing.T) {
	if err := ValidateTransition(PhaseVerification, StatusAwaitingJudgment, PhaseVerification, StatusInProgress); err != nil {
		t.Fatalf("awaiting_judgment -> in_progress should be legal: %v", err)
	}
}

func TestValidateTransitionAwaitingJudgmentToBlocked(t *testing.T) {
	if err := ValidateTransition(PhaseVerification, StatusAwaitingJudgment, PhaseVerification, StatusBlocked); err != nil {
		t.Fatalf("awaiting_judgment -> blocked should be legal: %v", err)
	}
}

func TestValidateTransitionAwaitingJudgmentToAwaitingRelease(t *testing.T) {
	if err := ValidateTransition(PhaseVerification, StatusAwaitingJudgment, PhaseVerification, StatusAwaitingRelease); err != nil {
		t.Fatalf("awaiting_judgment -> awaiting_release should be legal: %v", err)
	}
}

func TestValidateTransitionAwaitingJudgmentToCancelled(t *testing.T) {
	if err := ValidateTransition(PhaseVerification, StatusAwaitingJudgment, PhaseVerification, StatusCancelled); err != nil {
		t.Fatalf("awaiting_judgment -> cancelled should be legal: %v", err)
	}
}

func TestValidateTransitionAwaitingJudgmentFromDesignRejected(t *testing.T) {
	if err := ValidateTransition(PhaseDesign, StatusAwaitingJudgment, PhaseDesign, StatusInProgress); err == nil {
		t.Fatal("awaiting_judgment should not be valid in design phase")
	}
}
