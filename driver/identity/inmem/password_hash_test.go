package inmem_test

import (
	"context"
	"errors"
	"testing"

	"authorization"
	identityinmem "authorization/driver/identity/inmem"
	managerinmem "authorization/driver/transactionmanager/inmem"
)

func TestPasswordHashPersistence(t *testing.T) {
	ctx := t.Context()
	manager := managerinmem.New()
	identities := identityinmem.New(manager)
	row := authorization.Identity{
		ID: "identity", UserID: "user", Issuer: "local", Subject: "alice", PasswordHash: "encoded-hash",
	}
	abort := errors.New("abort")
	if err := manager.Do(ctx, func(ctx context.Context) error {
		if err := identities.Create(ctx, row); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatal(err)
	}
	if _, err := identities.GetBySubject(ctx, row.Issuer, row.Subject); !errors.Is(
		err,
		authorization.ErrNotFound,
	) {
		t.Fatal("rolled-back identity persisted")
	}
	if err := manager.Do(ctx, func(ctx context.Context) error { return identities.Create(ctx, row) }); err != nil {
		t.Fatal(err)
	}
	row.PasswordHash = "changed-input"
	stored, err := identities.GetBySubject(ctx, row.Issuer, row.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash != "encoded-hash" {
		t.Fatal("stored hash was not preserved")
	}
	stored.PasswordHash = "changed-output"
	again, err := identities.Get(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.PasswordHash != "encoded-hash" {
		t.Fatal("read result mutated stored hash")
	}
}
