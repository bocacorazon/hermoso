package workflow

import (
	"testing"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func TestIsAmendable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		phase  domain.Phase
		status domain.RunStatus
		design bool
		verif  bool
		want   bool
	}{
		{"blocked verification with contract", domain.PhaseVerification, domain.StatusBlocked, true, true, true},
		{"awaiting verification", domain.PhaseConstruction, domain.StatusAwaitingVerification, true, true, true},
		{"blocked verification no contract", domain.PhaseVerification, domain.StatusBlocked, true, false, false},
		{"blocked verification no design", domain.PhaseVerification, domain.StatusBlocked, false, false, false},
		{"design phase", domain.PhaseDesign, domain.StatusAwaitingApproval, true, true, false},
		{"in progress", domain.PhaseConstruction, domain.StatusInProgress, true, true, false},
		{"awaiting judgment", domain.PhaseVerification, domain.StatusAwaitingJudgment, true, true, false},
		{"awaiting release", domain.PhaseVerification, domain.StatusAwaitingRelease, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := domain.Run{Phase: tt.phase, Status: tt.status}
			if tt.design {
				run.Design = &domain.DesignState{}
				if tt.verif {
					run.Design.Verification = &domain.FeatureVerificationContract{}
				}
			}
			if got := isAmendable(run); got != tt.want {
				t.Errorf("isAmendable() = %v, want %v", got, tt.want)
			}
		})
	}
}
