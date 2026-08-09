package testutil

import (
	"os/exec"
	"strings"
	"testing"
)

type Repository struct {
	*Filesystem
}

func NewRepository(t testing.TB) *Repository {
	t.Helper()
	repo := &Repository{Filesystem: NewFilesystem(t)}
	repo.Git("init", "--initial-branch=main")
	repo.Git("config", "user.name", "Hermoso Test")
	repo.Git("config", "user.email", "hermoso@example.invalid")
	repo.Write("README.md", "# Test repository\n")
	repo.Commit("initial commit")
	return repo
}

func (r *Repository) Git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.Root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func (r *Repository) Commit(message string) string {
	r.t.Helper()
	r.Git("add", "--all")
	r.Git("commit", "--quiet", "-m", message)
	return r.Git("rev-parse", "HEAD")
}
