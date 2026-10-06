package inmem

import (
	"context"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/driver/internal/memory"
)

func (r *Repository) Delete(ctx context.Context, id entity.UserID) error {
	data, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return err
	}
	defer release()
	if _, exists := data.Users[id]; !exists {
		return memory.Missing()
	}
	if !data.LockedUsers[id] {
		return memory.Invalid("delete requires root lock")
	}
	for key, identity := range data.Identities {
		if identity.UserID == id {
			delete(data.Identities, key)
			delete(data.Subjects, memory.SubjectKey{Issuer: identity.Issuer, Subject: identity.Subject})
			delete(data.LockedIdentities, key)
		}
	}
	delete(data.Users, id)
	delete(data.LockedUsers, id)
	return nil
}
