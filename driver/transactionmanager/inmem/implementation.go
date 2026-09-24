// Package inmem coordinates process-local storage transactions for development and tests.
package inmem

import "authorization/driver/internal/memory"

// Manager coordinates repositories constructed with the same manager. Transactions
// serialize across its storage; errors, cancellation and panics discard writes.
// Construct with New. All records are lost when this instance is discarded.
type Manager struct{ *memory.Store }

func New() *Manager { return &Manager{Store: memory.NewStore()} }
