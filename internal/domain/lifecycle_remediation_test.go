package domain

import "testing"

func TestStatusAwaitingRemediationAllowed(t *testing.T) {
	t.Parallel()
	if !statusAllowed(PhaseConstruction, StatusAwaitingRemediation) {
		t.Errorf("statusAllowed should permit %s/%s", PhaseConstruction, StatusAwaitingRemediation)
	}
}

func TestAwaitingRemediationTransitions(t *testing.T) {
	t.Parallel()
	src := lifecycleState{PhaseConstruction, StatusAwaitingRemediation}
	destinations := []lifecycleState{
		{PhaseConstruction, StatusPending},
		{PhaseConstruction, StatusBlocked},
		{PhaseConstruction, StatusCancelled},
	}
	for _, dst := range destinations {
		if _, ok := legalTransitions[src][dst]; !ok {
			t.Errorf("missing transition %s/%s → %s/%s", src.phase, src.status, dst.phase, dst.status)
		}
	}
}

func TestAwaitingRemediationFromVerificationFail(t *testing.T) {
	t.Parallel()
	cases := []struct{ from, to lifecycleState }{
		{lifecycleState{PhaseConstruction, StatusAwaitingVerification}, lifecycleState{PhaseConstruction, StatusAwaitingRemediation}},
		{lifecycleState{PhaseVerification, StatusAwaitingJudgment}, lifecycleState{PhaseConstruction, StatusAwaitingRemediation}},
	}
	for _, c := range cases {
		if _, ok := legalTransitions[c.from][c.to]; !ok {
			t.Errorf("missing transition %s/%s → %s/%s", c.from.phase, c.from.status, c.to.phase, c.to.status)
		}
	}
}
