package workflow

import (
	"context"
	"testing"

	"github.com/bocacorazon/hermoso/internal/domain"
)

func TestJudgeExecutionRubricReturnsPending(t *testing.T) {
	judgment := domain.VerificationJudgment{
		Oracle: domain.VerificationOracle{Type: "rubric"},
	}
	status, summary := judgeExecution(
		context.Background(),
		judgment,
		"/work",
		[]byte("some output"),
		0,
		nil,
	)
	if status != domain.JudgmentPending {
		t.Fatalf("rubric oracle should return pending, got %s (summary: %s)", status, summary)
	}
	if summary == "" {
		t.Fatal("pending summary should not be empty")
	}
}

func TestJudgeExecutionRubricReturnsPendingEvenOnNonZeroExit(t *testing.T) {
	judgment := domain.VerificationJudgment{
		Oracle: domain.VerificationOracle{Type: "rubric"},
	}
	status, _ := judgeExecution(
		context.Background(),
		judgment,
		"/work",
		[]byte("partial output"),
		1,
		nil,
	)
	if status != domain.JudgmentPending {
		t.Fatalf("rubric oracle should return pending even on non-zero exit, got %s", status)
	}
}

func TestJudgeExecutionGherkinStillUsesExitCode(t *testing.T) {
	judgment := domain.VerificationJudgment{
		Oracle: domain.VerificationOracle{Type: "gherkin"},
	}
	status, _ := judgeExecution(
		context.Background(),
		judgment,
		"/work",
		[]byte("feature output"),
		0,
		nil,
	)
	if status != domain.JudgmentPass {
		t.Fatalf("gherkin oracle with exit 0 should pass, got %s", status)
	}

	status, _ = judgeExecution(
		context.Background(),
		judgment,
		"/work",
		[]byte("feature output"),
		1,
		nil,
	)
	if status != domain.JudgmentFail {
		t.Fatalf("gherkin oracle with exit 1 should fail, got %s", status)
	}
}

func TestJudgeExecutionExitCodeStillWorks(t *testing.T) {
	judgment := domain.VerificationJudgment{
		Oracle: domain.VerificationOracle{Type: "exit_code", Expected: "0"},
	}
	status, _ := judgeExecution(
		context.Background(),
		judgment,
		"/work",
		nil,
		0,
		nil,
	)
	if status != domain.JudgmentPass {
		t.Fatalf("exit_code oracle with exit 0 should pass, got %s", status)
	}
}
