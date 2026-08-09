package state

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bocacorazon/hermoso/internal/domain"
)

type Repository struct {
	Root        string
	ExcludePath string
	Target      domain.TargetIdentity
}

func DiscoverRepository(ctx context.Context, start string) (Repository, error) {
	if start == "" {
		return Repository{}, fmt.Errorf("%w: repository path must be explicit", ErrNotRepository)
	}
	root, err := gitOutput(ctx, start, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{}, fmt.Errorf("%w from %q: %v", ErrNotRepository, start, err)
	}
	root, err = canonicalPath(root)
	if err != nil {
		return Repository{}, fmt.Errorf("canonicalize Git root: %w", err)
	}
	excludePath, err := gitOutput(ctx, root, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return Repository{}, fmt.Errorf("locate Git exclude file: %w", err)
	}
	excludePath, err = canonicalPathAllowMissing(excludePath)
	if err != nil {
		return Repository{}, fmt.Errorf("canonicalize Git exclude path: %w", err)
	}

	remote, err := gitOutput(ctx, root, "config", "--get", "remote.origin.url")
	if err != nil && !errors.Is(err, errGitNoValue) {
		return Repository{}, fmt.Errorf("read origin remote: %w", err)
	}
	remote = normalizeRemote(root, remote)
	branch, err := gitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		branch = strings.TrimPrefix(branch, "origin/")
	} else {
		branch, err = gitOutput(ctx, root, "branch", "--show-current")
		if err != nil && !errors.Is(err, errGitNoValue) {
			return Repository{}, fmt.Errorf("read default branch: %w", err)
		}

	}

	return Repository{
		Root:        root,
		ExcludePath: excludePath,
		Target: domain.TargetIdentity{
			Repository:    root,
			RemoteURL:     remote,
			DefaultBranch: branch,
		},
	}, nil
}

func normalizeRemote(root, remote string) string {
	if remote == "" || strings.Contains(remote, "://") || strings.Contains(remote, "@") {
		return remote
	}
	path := remote
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return remote
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

var errGitNoValue = errors.New("Git value is not configured")

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	value := strings.TrimSpace(string(output))
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 && value == "" {
			return "", errGitNoValue
		}
		if value != "" {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), value)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return value, nil
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

func canonicalPathAllowMissing(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}
