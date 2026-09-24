package inmem

import (
	"context"

	"authorization/domain/entity"
	"authorization/driver/internal/memory"
)

func (r *Repository) Delete(ctx context.Context, ID entity.IdentityID) error {
	data, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return err
	}
	defer release()

	row, exists := data.Identities[ID]
	if !exists {
		return memory.Missing()
	}

	if !data.LockedIdentities[ID] {
		return memory.Invalid("delete requires root lock")
	}

	delete(data.Identities, ID)
	delete(data.Subjects, memory.SubjectKey{Issuer: row.Issuer, Subject: row.Subject})
	delete(data.LockedIdentities, ID)

	return nil
}
