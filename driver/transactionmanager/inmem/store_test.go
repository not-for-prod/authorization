package inmem_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/not-for-prod/authorization"
	identityinmem "github.com/not-for-prod/authorization/driver/identity/inmem"
	transactioninmem "github.com/not-for-prod/authorization/driver/transaction/inmem"
	managerinmem "github.com/not-for-prod/authorization/driver/transactionmanager/inmem"
	userinmem "github.com/not-for-prod/authorization/driver/user/inmem"
)

func TestCommitRollbackAndIdentityUniqueness(t *testing.T) {
	ctx := context.Background()
	store := newStore()
	user := authorization.User{ID: "alice", Status: "active", CreatedAt: time.Now()}
	identity := authorization.Identity{ID: "local-alice", UserID: user.ID, Issuer: "local", Subject: "alice"}
	aborted := errors.New("abort")

	var retained context.Context

	create := func(ctx context.Context) error {
		retained = ctx
		if err := store.Users.Create(ctx, user); err != nil {
			return err
		}

		return store.Identities.Create(ctx, identity)
	}
	if err := store.Do(ctx, func(ctx context.Context) error {
		if err := create(ctx); err != nil {
			return err
		}
		return aborted
	}); !errors.Is(err, aborted) {
		t.Fatal(err)
	}

	if _, err := store.Users.Get(ctx, user.ID); !errors.Is(err, authorization.ErrNotFound) {
		t.Fatal(err)
	}

	if _, err := store.Identities.GetBySubject(ctx, "local", "alice"); !errors.Is(err, authorization.ErrNotFound) {
		t.Fatal(err)
	}

	if err := store.Do(ctx, create); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Users.Get(retained, user.ID); err == nil {
		t.Fatal("escaped context accepted")
	}

	if err := store.Users.Create(ctx, user); err == nil {
		t.Fatal("mutation outside transaction")
	}

	if err := store.Do(ctx, func(ctx context.Context) error {
		other := identity
		other.ID = "another"
		return store.Identities.Create(ctx, other)
	}); !errors.Is(err, authorization.ErrAlreadyExists) {
		t.Fatal(err)
	}

	if err := store.Do(ctx, func(ctx context.Context) error { return store.Users.Update(ctx, user) }); err == nil {
		t.Fatal("unlocked update accepted")
	}

	if err := store.Do(ctx, func(ctx context.Context) error {
		row, err := store.Users.Get(ctx, user.ID, authorization.ForUpdate())
		if err != nil {
			return err
		}
		row.Status = "blocked"
		if err := store.Users.Update(ctx, *row); err != nil {
			return err
		}
		if _, err := store.Identities.GetBySubject(ctx, "local", "alice", authorization.ForUpdate()); err != nil {
			return err
		}
		if err := store.Identities.Delete(ctx, identity.ID); err != nil {
			return err
		}
		return aborted
	}); !errors.Is(err, aborted) {
		t.Fatal(err)
	}

	row, err := store.Users.Get(ctx, user.ID)
	if err != nil || row.Status != "active" {
		t.Fatalf("rollback: %+v %v", row, err)
	}

	row.Status = "changed outside storage"

	row, err = store.Users.Get(ctx, user.ID)
	if err != nil || row.Status != "active" {
		t.Fatalf("snapshot: %+v %v", row, err)
	}

	if _, err := store.Identities.GetBySubject(ctx, "local", "alice"); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentIdentityCreation(t *testing.T) {
	store := newStore()
	results := make(chan error, 16)

	var wg sync.WaitGroup
	for n := range 16 {
		wg.Go(func() {
			results <- store.Do(context.Background(), func(ctx context.Context) error {
				return store.Identities.Create(ctx, authorization.Identity{ID: authorization.IdentityID(fmt.Sprint(n)), Issuer: "issuer", Subject: "subject"})
			})
		})
	}

	wg.Wait()
	close(results)

	successes := 0

	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, authorization.ErrAlreadyExists) {
			t.Fatal(err)
		}
	}

	if successes != 1 {
		t.Fatalf("got %d identities", successes)
	}
}

func TestTransactionSnapshotsAndContextBoundaries(t *testing.T) {
	store := newStore()
	other := newStore()
	ctx := context.Background()
	userID := authorization.UserID("alice")
	row := authorization.Transaction{ID: "tx", TenantID: "tenant", OwnerKey: "owner", UserID: &userID}

	if err := store.Do(ctx, func(ctx context.Context) error { return store.Transactions.Create(ctx, row) }); err != nil {
		t.Fatal(err)
	}

	userID = "mutated"

	saved, err := store.Transactions.Get(ctx, "tenant", "tx", "owner")
	if err != nil || *saved.UserID != "alice" {
		t.Fatalf("snapshot: %+v %v", saved, err)
	}

	if _, err := store.Transactions.Get(ctx, "other", "tx", "owner"); !errors.Is(err, authorization.ErrNotFound) {
		t.Fatal(err)
	}

	for _, owner := range []string{"other-owner", ""} {
		if _, err := store.Transactions.Get(ctx, "tenant", "tx", owner); !errors.Is(err, authorization.ErrNotFound) {
			t.Fatalf("owner %q: expected not found, got %v", owner, err)
		}
		if err := store.Do(ctx, func(ctx context.Context) error {
			if _, err := store.Transactions.Get(ctx, "tenant", "tx", owner, authorization.ForUpdate()); !errors.Is(err, authorization.ErrNotFound) {
				t.Fatalf("locked owner %q: expected not found, got %v", owner, err)
			}
			if err := store.Transactions.Update(ctx, *saved); err == nil {
				t.Fatal("owner mismatch granted an update lock")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := store.Transactions.Get(ctx, "tenant", "tx", "owner", authorization.ForUpdate()); err == nil {
		t.Fatal("lock outside transaction")
	}

	if _, err := store.Transactions.Get(ctx, "tenant", "tx", "owner", authorization.SelectOption(99)); err == nil {
		t.Fatal("unsupported option")
	}

	if err := store.Do(ctx, func(ctx context.Context) error {
		if _, err := other.Users.Get(ctx, "alice"); err == nil {
			return errors.New("foreign context accepted")
		}
		if err := store.Do(ctx, func(context.Context) error { return nil }); err == nil {
			return errors.New("nested scope accepted")
		}
		tx, err := store.Transactions.Get(ctx, "tenant", "tx", "owner", authorization.ForUpdate())
		if err != nil {
			return err
		}
		tx.Intent = "changed"
		if err := store.Transactions.Update(ctx, *tx); err == nil {
			return errors.New("immutable intent changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCancellationAndPanicRollback(t *testing.T) {
	store := newStore()
	ctx := context.Background()
	canceled, cancel := context.WithCancel(ctx)

	err := store.Do(canceled, func(ctx context.Context) error {
		if err := store.Users.Create(ctx, authorization.User{ID: "cancel"}); err != nil {
			return err
		}

		cancel()

		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()

		_ = store.Do(ctx, func(ctx context.Context) error {
			if err := store.Users.Create(ctx, authorization.User{ID: "panic"}); err != nil {
				t.Fatal(err)
			}

			panic("abort")
		})
	}()

	for _, id := range []authorization.UserID{"cancel", "panic"} {
		if _, err := store.Users.Get(ctx, id); !errors.Is(err, authorization.ErrNotFound) {
			t.Fatal(err)
		}
	}

	if err := store.Do(ctx, func(context.Context) error {
		_, err := store.Users.Get(canceled, "anything")
		if !errors.Is(err, context.Canceled) {
			return fmt.Errorf("canceled read: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

type testStore struct {
	*managerinmem.Manager
	Transactions authorization.TransactionRepository
	Users        authorization.UserRepository
	Identities   authorization.IdentityRepository
}

func newStore() *testStore {
	manager := managerinmem.New()
	return &testStore{Manager: manager, Transactions: transactioninmem.New(manager), Users: userinmem.New(manager), Identities: identityinmem.New(manager)}
}
