package app

import (
	"bytes"
	"context"
	"testing"

	"github.com/bocacorazon/hermoso/internal/testutil"
)

func TestVerificationJudgeUsageError(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	deps := testDependencies(stdout, stderr)
	if code := Run(context.Background(), []string{"init", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("init: code=%d output=%s", code, stdout.String())
	}
	stdout.Reset()
	// No subcommand
	if code := Run(context.Background(), []string{"verification", "--json"}, deps); code != ExitUsage {
		t.Fatalf("expected ExitUsage for verification without subcommand, got %d", code)
	}
	// Unknown subcommand
	if code := Run(context.Background(), []string{"verification", "bogus", "proj-1", "feat-1", "run-1", repo.Root, "--json"}, deps); code != ExitUsage {
		t.Fatalf("expected ExitUsage for unknown verification subcommand, got %d", code)
	}
}
