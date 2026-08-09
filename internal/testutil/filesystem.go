// Package testutil provides Hermoso's shared filesystem and Git test fixtures.
package testutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

type Filesystem struct {
	t    testing.TB
	Root string
}

func NewFilesystem(t testing.TB) *Filesystem {
	t.Helper()
	repositoryRoot := findModuleRoot(t)
	fixturesRoot := filepath.Join(repositoryRoot, ".hermoso-test")
	if err := os.MkdirAll(fixturesRoot, 0o755); err != nil {
		t.Fatalf("create fixture root %q: %v", fixturesRoot, err)
	}
	root, err := os.MkdirTemp(fixturesRoot, "fixture-")
	if err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove fixture directory %q: %v", root, err)
		}
		if err := os.Remove(fixturesRoot); err != nil &&
			!errors.Is(err, os.ErrNotExist) &&
			!errors.Is(err, syscall.ENOTEMPTY) {
			t.Errorf("remove fixture root %q: %v", fixturesRoot, err)
		}
	})
	return &Filesystem{t: t, Root: root}
}

func (f *Filesystem) Path(parts ...string) string {
	f.t.Helper()
	return filepath.Join(append([]string{f.Root}, parts...)...)
}

func (f *Filesystem) Mkdir(path string) string {
	f.t.Helper()
	fullPath := f.Path(path)
	if err := os.MkdirAll(fullPath, 0o755); err != nil {
		f.t.Fatalf("create fixture directory %q: %v", fullPath, err)
	}
	return fullPath
}

func (f *Filesystem) Write(path, content string) string {
	f.t.Helper()
	fullPath := f.Path(path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		f.t.Fatalf("create parent directory for %q: %v", fullPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		f.t.Fatalf("write fixture file %q: %v", fullPath, err)
	}
	return fullPath
}

func findModuleRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("inspect module root candidate %q: %v", dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("find module root from %q", dir)
		}
		dir = parent
	}
}
