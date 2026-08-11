package app

import (
	"bytes"
	"context"
	"testing"

	"github.com/bocacorazon/hermoso/internal/testutil"
)

func TestVerificationRemediateUsageError(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	deps := testDependencies(stdout, stderr)
	if code := Run(context.Background(), []string{"init", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("init: code=%d output=%s", code, stdout.String())
	}
	stdout.Reset()
	// "remediate" is a valid subcommand but needs full context + path.
	// Missing path should produce a state error, not usage error.
	// But a bogus subcommand should still produce usage error.
	if code := Run(context.Background(), []string{"verification", "bogus", "p", "f", "r", repo.Root, "--json"}, deps); code != ExitUsage {
		t.Fatalf("expected ExitUsage for bogus verification subcommand, got %d", code)
	}
}

func TestVerificationRemediateAcceptsValidSubcommand(t *testing.T) {
	t.Parallel()
	repo := testutil.NewRepository(t)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	deps := testDependencies(stdout, stderr)
	if code := Run(context.Background(), []string{"init", repo.Root, "--json"}, deps); code != ExitOK {
		t.Fatalf("init: code=%d output=%s", code, stdout.String())
	}
	stdout.Reset()
	// "remediate" should NOT be rejected as a usage error (it should fail
	// with a state error instead, since there's no run in awaiting_remediation).
	// We just verify it's accepted as a valid subcommand.
	code := Run(context.Background(), []string{"verification", "remediate", "p", "f", "r", repo.Root, "spec.json", "--json"}, deps)
	if code == ExitUsage {
		t.Fatalf("remediate should not be a usage error, got ExitUsage: %s", stderr.String())
	}
}
