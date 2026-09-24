// Package repository defines storage ports for host-supplied adapters.
//
// Repositories persist already-decided domain state; they do not authenticate,
// evaluate policies, or invent lifecycle transitions. Returned values must be
// independent snapshots, including nested slices and pointers. Create must not
// overwrite existing records, and Update must not silently create missing ones.
// Adapters must preserve the errors below for errors.Is, even when also applying
// the host's infrastructure error classification.
package repository

import "errors"

var (
	// ErrNotFound means no record exists for the complete requested identity.
	ErrNotFound = errors.New("authentication record not found")
	// ErrAlreadyExists means an identity or uniqueness key is already occupied.
	ErrAlreadyExists = errors.New("authentication record already exists")
)

// SelectOption describes a storage read option understood by an adapter.
// Unsupported options must be rejected rather than silently ignored.
type SelectOption uint8

// SelectForUpdate requires an exclusive root lock held through the enclosing
// transaction. The adapter must reject it outside a TransactionManager.Do scope.
const SelectForUpdate SelectOption = 1

// ForUpdate requests a locked read before changing existing state.
func ForUpdate() SelectOption { return SelectForUpdate }
