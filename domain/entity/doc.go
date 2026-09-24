// Package entity defines the authentication domain's model types.
//
// Authentication transactions validate construction and guard claims, challenge
// consumption, subject binding, expiry, and completion. User lifecycle and
// identity-linking behavior remain outside the implemented scope. Policy
// evaluation lives in domain/service; serialization and provider-specific
// details belong in adapters.
package entity
