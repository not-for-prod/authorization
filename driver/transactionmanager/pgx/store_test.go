package pgx_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	trm "github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	events "github.com/not-for-prod/authorization/driver/event/pgx"
	identities "github.com/not-for-prod/authorization/driver/identity/pgx"
	transactions "github.com/not-for-prod/authorization/driver/transaction/pgx"
	manager "github.com/not-for-prod/authorization/driver/transactionmanager/pgx"
	users "github.com/not-for-prod/authorization/driver/user/pgx"
	"github.com/not-for-prod/authorization/internal/testutil/pgfixture"
)

type fixture struct {
	pool         *pgxpool.Pool
	manager      *manager.Manager
	users        *users.Repository
	identities   *identities.Repository
	transactions *transactions.Repository
	events       *events.Publisher
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := pgfixture.New(t)
	m, err := manager.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	e, err := events.New(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{
		pool:         pool,
		manager:      m,
		users:        users.New(m),
		identities:   identities.New(m),
		transactions: transactions.New(m),
		events:       e,
	}
}
func transaction(t *testing.T, id string) entity.Transaction {
	t.Helper()
	tx, err := entity.NewTransaction(
		entity.TransactionID(id),
		"owner",
		entity.Policy{
			TenantID: "tenant",
			Intent:   entity.IntentSignUp,
			Root:     entity.MethodRequirement{Method: "password"},
		},
		nil,
		entity.TransactionSettings{
			Lifetime:          time.Hour,
			OperationTimeout:  time.Minute,
			MaxAttempts:       4,
			MaxChallenges:     4,
			MaxChallengeBytes: 1024,
			MaxPolicyNodes:    10,
		},
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return *tx
}
func TestAtomicStorageAndOutbox(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tx := transaction(t, "tx")
	now := time.Now().UTC().Truncate(time.Microsecond)
	save := func(ctx context.Context) error {
		if err := f.users.Create(ctx, entity.User{ID: "user", Status: "active", CreatedAt: now}); err != nil {
			return err
		}
		if err := f.identities.Create(ctx, entity.Identity{ID: "identity", UserID: "user", Issuer: "local", Subject: "subject", PasswordHash: "encoded-hash", CreatedAt: now}); err != nil {
			return err
		}
		if err := f.transactions.Create(ctx, tx); err != nil {
			return err
		}
		return f.events.Publish(ctx, tx.Events())
	}
	rejected := errors.New("reject commit")
	if err := f.manager.Do(ctx, func(ctx context.Context) error {
		if err := save(ctx); err != nil {
			return err
		}
		return trm.Skippable(rejected)
	}); !errors.Is(err, rejected) {
		t.Fatal(err)
	}
	for _, table := range []string{"auth_user", "auth_identity", "auth_transaction", "event_log"} {
		var count int
		if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s committed rollback", table)
		}
	}
	if err := f.manager.Do(ctx, save); err != nil {
		t.Fatal(err)
	}
	read, err := f.transactions.Get(ctx, "tenant", "tx", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tx.Copy(), read.Copy()) || len(read.Events()) != 0 {
		t.Fatal("snapshot changed during persistence")
	}
	identity, err := f.identities.GetBySubject(ctx, "local", "subject")
	if err != nil || identity.PasswordHash != "encoded-hash" {
		t.Fatalf("identity round trip: %v", err)
	}
	var payload []byte
	if err = f.pool.QueryRow(ctx, "SELECT payload FROM event_log").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event entity.TransactionEvent
	if err = json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(event, tx.Events()[0]) {
		t.Fatal("event envelope lost metadata or snapshots")
	}
	for _, owner := range []string{"wrong", ""} {
		if _, err = f.transactions.Get(ctx, "tenant", "tx", owner); !errors.Is(err, repository.ErrNotFound) {
			t.Fatal(err)
		}
	}
	if _, err = f.transactions.Get(ctx, "wrong", "tx", "owner"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal(err)
	}
	if err = f.manager.Do(ctx, func(ctx context.Context) error { return f.users.Create(ctx, entity.User{ID: "user", CreatedAt: now}) }); !errors.Is(
		err,
		repository.ErrAlreadyExists,
	) {
		t.Fatal(err)
	}
}
func TestScopesAndImmutableFields(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tx := transaction(t, "tx")
	user := entity.User{ID: "u", CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	if err := f.users.Create(ctx, user); err == nil {
		t.Fatal("mutation outside scope succeeded")
	}
	if _, err := f.users.Get(ctx, "u", repository.ForUpdate()); err == nil {
		t.Fatal("lock outside scope succeeded")
	}
	if _, err := f.users.Get(ctx, "u", repository.SelectOption(99)); err == nil {
		t.Fatal("unknown option succeeded")
	}
	var escaped context.Context
	if err := f.manager.Do(ctx, func(ctx context.Context) error {
		escaped = ctx
		if err := f.users.Create(ctx, user); err != nil {
			return err
		}
		return f.transactions.Create(ctx, tx)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.Get(escaped, "u"); err == nil {
		t.Fatal("escaped context succeeded")
	}
	other, err := manager.New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.manager.Do(ctx, func(ctx context.Context) error {
		if err := f.manager.Do(ctx, func(context.Context) error { return nil }); err == nil {
			t.Fatal("nested transaction succeeded")
		}
		if _, err := users.New(other).Get(ctx, "u"); err == nil {
			t.Fatal("foreign context succeeded")
		}
		if err := f.users.Update(ctx, user); err == nil {
			t.Fatal("unlocked update succeeded")
		}
		if _, err := f.users.Get(ctx, "u", repository.ForUpdate()); err != nil {
			return err
		}
		changed := user
		changed.CreatedAt = changed.CreatedAt.Add(time.Second)
		if err := f.users.Update(ctx, changed); err == nil {
			t.Fatal("changed immutable creation time")
		}
		changed = user
		changed.Status = "disabled"
		return f.users.Update(ctx, changed)
	}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*entity.Transaction){func(tx *entity.Transaction) { tx.OwnerKey = "other" }, func(tx *entity.Transaction) { tx.Intent = entity.IntentSignIn }, func(tx *entity.Transaction) { tx.Settings.MaxAttempts++ }, func(tx *entity.Transaction) { tx.ExpiresAt = tx.ExpiresAt.Add(time.Hour) }, func(tx *entity.Transaction) { tx.Policy.Root = entity.MethodRequirement{Method: "other"} }} {
		if err = f.manager.Do(ctx, func(ctx context.Context) error {
			v, err := f.transactions.Get(ctx, "tenant", "tx", "owner", repository.ForUpdate())
			if err != nil {
				return err
			}
			change(v)
			return f.transactions.Update(ctx, *v)
		}); err == nil {
			t.Fatal("immutable transaction mutation accepted")
		}
	}
	if err = f.manager.Do(ctx, func(ctx context.Context) error {
		v, err := f.transactions.Get(ctx, "tenant", "tx", "owner", repository.ForUpdate())
		if err != nil {
			return err
		}
		v.Attempts++
		return f.transactions.Update(ctx, *v)
	}); err != nil {
		t.Fatal(err)
	}
}
func TestLockBlocksCompetingMutation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	user := entity.User{ID: "u", CreatedAt: time.Now().UTC()}
	if err := f.manager.Do(ctx, func(ctx context.Context) error { return f.users.Create(ctx, user) }); err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- f.manager.Do(ctx, func(ctx context.Context) error {
			_, err := f.users.Get(ctx, "u", repository.ForUpdate())
			close(locked)
			if err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	<-locked
	waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	err := f.manager.Do(
		waitCtx,
		func(ctx context.Context) error { _, err := f.users.Get(ctx, "u", repository.ForUpdate()); return err },
	)
	cancel()
	close(release)
	if err == nil {
		t.Error("competing root lock was not blocked")
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = f.manager.Do(ctx, func(ctx context.Context) error { _, err := f.users.Get(ctx, "u", repository.ForUpdate()); return err }); err != nil {
		t.Fatal(err)
	}
}
func TestConcurrentSignUpUniqueSubject(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"a", "b"} {
		tx := transaction(t, id)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- f.manager.Do(ctx, func(ctx context.Context) error {
				if err := f.transactions.Create(ctx, tx); err != nil {
					return err
				}
				if err := f.users.Create(ctx, entity.User{ID: entity.UserID(id), CreatedAt: time.Now().UTC()}); err != nil {
					return err
				}
				if err := f.identities.Create(ctx, entity.Identity{ID: entity.IdentityID(id), UserID: entity.UserID(id), Issuer: "local", Subject: "same", CreatedAt: time.Now().UTC()}); err != nil {
					return err
				}
				return f.events.Publish(ctx, tx.Events())
			})
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, repository.ErrAlreadyExists) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("got %d winners", wins)
	}
	for _, table := range []string{"auth_user", "auth_identity", "auth_transaction", "event_log"} {
		var count int
		if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s retained losing writes: %d", table, count)
		}
	}
}
func TestOutboxFailureAndPanicRollback(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tx := transaction(t, "failed")
	if _, err := f.pool.Exec(ctx, "ALTER TABLE event_log ADD CONSTRAINT reject_insert CHECK (false) NOT VALID"); err != nil {
		t.Fatal(err)
	}
	err := f.manager.Do(ctx, func(ctx context.Context) error {
		if err := f.transactions.Create(ctx, tx); err != nil {
			return err
		}
		return f.events.Publish(ctx, tx.Events())
	})
	if err == nil {
		t.Fatal("outbox failure was ignored")
	}
	if _, err = f.transactions.Get(ctx, "tenant", "failed", "owner"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed")
			}
		}()
		_ = f.manager.Do(ctx, func(ctx context.Context) error {
			if err := f.transactions.Create(ctx, tx); err != nil {
				return err
			}
			panic("abort")
		})
	}()
	if _, err = f.transactions.Get(ctx, "tenant", "failed", "owner"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	err = f.manager.Do(cancelled, func(ctx context.Context) error {
		if err := f.transactions.Create(ctx, tx); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if err == nil {
		t.Fatal("cancellation committed")
	}
	if _, err = f.transactions.Get(ctx, "tenant", "failed", "owner"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal(err)
	}
	var notFound *proterror.NotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("missing classification: %v", err)
	}
}

func TestIdentityDeleteAndMissingWrites(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	identity := entity.Identity{
		ID:        "identity",
		UserID:    "user",
		Kind:      "password",
		Issuer:    "local",
		Subject:   "subject",
		CreatedAt: now,
	}
	if err := f.manager.Do(ctx, func(ctx context.Context) error {
		if err := f.users.Create(ctx, entity.User{ID: "user", CreatedAt: now}); err != nil {
			return err
		}
		return f.identities.Create(ctx, identity)
	}); err != nil {
		t.Fatal(err)
	}
	got, err := f.identities.Get(ctx, identity.ID)
	if err != nil || !reflect.DeepEqual(*got, identity) {
		t.Fatalf("identity Get: %v", err)
	}
	if err = f.manager.Do(ctx, func(ctx context.Context) error { return f.identities.Delete(ctx, identity.ID) }); err == nil {
		t.Fatal("unlocked delete accepted")
	}
	if err = f.manager.Do(ctx, func(ctx context.Context) error {
		if _, err := f.identities.GetBySubject(ctx, "local", "subject", repository.ForUpdate()); err != nil {
			return err
		}
		return f.identities.Delete(ctx, identity.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.identities.Get(ctx, identity.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err = f.identities.GetBySubject(ctx, "local", "subject"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal(err)
	}
	if err = f.manager.Do(ctx, func(ctx context.Context) error { return f.identities.Delete(ctx, identity.ID) }); !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatal(err)
	}
	if err = f.manager.Do(ctx, func(ctx context.Context) error { return f.users.Update(ctx, entity.User{ID: "missing"}) }); !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatal(err)
	}
	if err = f.manager.Do(ctx, func(ctx context.Context) error {
		return f.transactions.Update(ctx, entity.Transaction{TenantID: "tenant", ID: "missing"})
	}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal(err)
	}
	if err = f.events.Publish(ctx, nil); err == nil {
		t.Fatal("publisher accepted missing transaction scope")
	}
}
