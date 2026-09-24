package inmem

import (
	"context"

	"authorization/domain/entity"
	"authorization/driver/internal/memory"
)

func (r *Repository) Create(ctx context.Context, row entity.Identity) error {
	data, release, err := r.manager.Access(ctx, true)
	if err != nil {
		return err
	}
	defer release()

	key := row.ID
	if _, exists := data.Identities[key]; exists {
		return memory.Duplicate()
	}

	Subject := memory.SubjectKey{Issuer: row.Issuer, Subject: row.Subject}
	if _, exists := data.Subjects[Subject]; exists {
		return memory.Duplicate()
	}

	data.Subjects[Subject] = row.ID
	data.Identities[key] = row

	return nil
}
