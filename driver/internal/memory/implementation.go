// Package memory implements process-local storage for development and embedded tests.
package memory

import (
	"errors"
	"sync"

	"github.com/not-for-prod/proterror/proterror"

	"authorization/domain/entity"
	"authorization/domain/repository"
)

type contextKey struct{}
type TransactionKey struct {
	Tenant entity.TenantID
	ID     entity.TransactionID
}
type SubjectKey struct{ Issuer, Subject string }
type State struct {
	Events             []entity.TransactionEvent
	Transactions       map[TransactionKey]entity.Transaction
	Users              map[entity.UserID]entity.User
	Identities         map[entity.IdentityID]entity.Identity
	Subjects           map[SubjectKey]entity.IdentityID
	LockedTransactions map[TransactionKey]bool
	LockedUsers        map[entity.UserID]bool
	LockedIdentities   map[entity.IdentityID]bool
}
type scope struct {
	mu     sync.Mutex
	owner  *Store
	active bool
	data   *State
}

// Store owns the shared in-memory state and transaction boundary. Use NewStore.
// Data is lost on restart. Transactions serialize across the entire store.
type Store struct {
	gate chan struct{}
	data *State
}

func NewStore() *Store {
	s := &Store{gate: make(chan struct{}, 1), data: &State{
		Transactions: make(map[TransactionKey]entity.Transaction), Users: make(map[entity.UserID]entity.User),
		Identities: make(map[entity.IdentityID]entity.Identity), Subjects: make(map[SubjectKey]entity.IdentityID),
	}}

	return s
}

func Invalid(message string) error {
	return errors.Join(&proterror.FailedPrecondition{}, errors.New(message))
}
func Missing() error   { return errors.Join(&proterror.NotFound{}, repository.ErrNotFound) }
func Duplicate() error { return errors.Join(&proterror.AlreadyExists{}, repository.ErrAlreadyExists) }
