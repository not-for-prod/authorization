package inmem

import (
	"context"

	"authorization/domain/entity"
	"authorization/domain/repository"
	"authorization/driver/internal/memory"
)

func (r *Repository) GetBySubject(ctx context.Context, Issuer, Subject string, opts ...repository.SelectOption) (*entity.Identity, error) {
	data, release, err := r.manager.Access(ctx, false, opts...)
	if err != nil {
		return nil, err
	}
	defer release()

	ID, exists := data.Subjects[memory.SubjectKey{Issuer: Issuer, Subject: Subject}]
	if !exists {
		return nil, memory.Missing()
	}

	if len(opts) > 0 {
		data.LockedIdentities[ID] = true
	}

	row := data.Identities[ID]

	return &row, nil
}
