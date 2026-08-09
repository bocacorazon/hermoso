package app

import (
	"io/fs"
	"os"
)

// Filesystem is the file operation boundary used by command orchestration.
// Paths retain normal os package semantics.
type Filesystem interface {
	MkdirAll(path string, perm fs.FileMode) error
	Open(name string) (fs.File, error)
	ReadFile(name string) ([]byte, error)
	Remove(name string) error
	Rename(oldPath, newPath string) error
	Stat(name string) (fs.FileInfo, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
}

type OSFilesystem struct{}

func (OSFilesystem) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (OSFilesystem) Open(name string) (fs.File, error) {
	return os.Open(name)
}

func (OSFilesystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (OSFilesystem) Remove(name string) error {
	return os.Remove(name)
}

func (OSFilesystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (OSFilesystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (OSFilesystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}
