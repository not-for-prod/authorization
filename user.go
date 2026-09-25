package authorization

import "github.com/not-for-prod/authorization/domain/entity"

// UserID exposes the entity contract for host implementations.
type UserID = entity.UserID

// UserStatus exposes the entity contract for host implementations.
type UserStatus = entity.UserStatus

// UserStatusActive is assigned to newly registered users.
const UserStatusActive = entity.UserStatusActive

// User exposes the entity contract for host implementations.
type User = entity.User

// IdentityID exposes the entity contract for host implementations.
type IdentityID = entity.IdentityID

// IdentityKind exposes the entity contract for host implementations.
type IdentityKind = entity.IdentityKind

// Identity exposes the entity contract for host implementations.
type Identity = entity.Identity
