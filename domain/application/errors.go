package application

import "errors"

// ErrAccountNotFound means verified sign-in proof has no registered account.
// Hosts may offer explicit sign-up; unrelated storage misses do not carry it.
var ErrAccountNotFound = errors.New("account not found for verified identity")
