// Package state owns durable, clone-local Hermoso state.
package state

import "errors"

var (
	ErrNotRepository     = errors.New("not a Git repository")
	ErrNotInitialized    = errors.New("Hermoso is not initialized")
	ErrIncompatibleState = errors.New("incompatible Hermoso state")
	ErrCorruptState      = errors.New("corrupt Hermoso state")
	ErrBindingConflict   = errors.New("conflicting Hermes task binding")
	ErrUnsafeStatePath   = errors.New("unsafe Hermoso state path")
)
