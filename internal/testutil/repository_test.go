package testutil

import "testing"

func TestNewRepositoryCreatesCleanMainBranch(t *testing.T) {
	t.Parallel()

	repo := NewRepository(t)

	if got := repo.Git("branch", "--show-current"); got != "main" {
		t.Errorf("branch = %q, want main", got)
	}
	if got := repo.Git("status", "--porcelain"); got != "" {
		t.Errorf("repository is dirty:\n%s", got)
	}
	if got := repo.Git("rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("commit count = %q, want 1", got)
	}
}
