package workflow

import (
	"testing"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func TestAggregateVerdictWithPendingReturnsPending(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{Status: domain.JudgmentPass},
		{Status: domain.JudgmentPending},
	}
	if v := aggregateVerdict(outcomes); v != domain.VerificationPending {
		t.Fatalf("verdict with pending outcome should be pending, got %s", v)
	}
}

func TestAggregateVerdictBlockedOverridesPending(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{Status: domain.JudgmentPending},
		{Status: domain.JudgmentBlocked},
	}
	if v := aggregateVerdict(outcomes); v != domain.VerificationBlocked {
		t.Fatalf("blocked should override pending, got %s", v)
	}
}

func TestAggregateVerdictPendingOverridesFail(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{Status: domain.JudgmentFail},
		{Status: domain.JudgmentPending},
	}
	if v := aggregateVerdict(outcomes); v != domain.VerificationPending {
		t.Fatalf("pending should override fail, got %s", v)
	}
}

func TestAggregateVerdictAllPassStillPass(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{Status: domain.JudgmentPass},
		{Status: domain.JudgmentPass},
	}
	if v := aggregateVerdict(outcomes); v != domain.VerificationPass {
		t.Fatalf("all pass should be pass, got %s", v)
	}
}

func TestAggregateCoveragePropagatesPending(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{
			Status:        domain.JudgmentPending,
			RequirementIDs: []string{"req-1"},
		},
	}
	result := aggregateCoverage(outcomes, func(o domain.JudgmentOutcome) []string {
		return o.RequirementIDs
	}, nil, "requirement")
	if len(result) != 1 || result[0].Status != string(domain.JudgmentPending) {
		t.Fatalf("coverage should propagate pending, got %+v", result)
	}
}

func TestAggregateCoverageBlockedOverridesPending(t *testing.T) {
	outcomes := []domain.JudgmentOutcome{
		{
			Status:        domain.JudgmentPending,
			RequirementIDs: []string{"req-1"},
		},
		{
			Status:        domain.JudgmentBlocked,
			RequirementIDs: []string{"req-1"},
		},
	}
	result := aggregateCoverage(outcomes, func(o domain.JudgmentOutcome) []string {
		return o.RequirementIDs
	}, nil, "requirement")
	if len(result) != 1 || result[0].Status != string(domain.JudgmentBlocked) {
		t.Fatalf("blocked should override pending in coverage, got %+v", result)
	}
}
