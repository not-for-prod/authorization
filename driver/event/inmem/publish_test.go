package inmem_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	eventinmem "github.com/not-for-prod/authorization/driver/event/inmem"
	transactioninmem "github.com/not-for-prod/authorization/driver/transaction/inmem"
	managerinmem "github.com/not-for-prod/authorization/driver/transactionmanager/inmem"
)

func TestAtomicOutbox(t *testing.T) {
	ctx := t.Context()
	manager := managerinmem.New()
	rows := transactioninmem.New(manager)
	publisher := eventinmem.New(manager)

	tx, err := entity.NewTransaction("tx", "owner", entity.Policy{TenantID: "tenant", Intent: "sign_in", Root: entity.AnyOfRequirement{ID: "primary", Children: []entity.Requirement{entity.MethodRequirement{Method: "password"}}}}, nil, entity.TransactionSettings{Lifetime: time.Minute, OperationTimeout: time.Second, MaxAttempts: 4, MaxChallenges: 2, MaxChallengeBytes: 100, MaxPolicyNodes: 10}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Restore a finalizing root to exercise publication of a real completion event.
	tx.Status = entity.TransactionFinalizing
	tx.Principal = &entity.Principal{TenantID: tx.TenantID, UserID: "alice"}
	if err := tx.Complete(time.Now()); err != nil {
		t.Fatal(err)
	}

	failure := errors.New("rollback")
	if err := manager.Do(ctx, func(ctx context.Context) error {
		if err := rows.Create(ctx, *tx); err != nil {
			return err
		}
		if err := publisher.Publish(ctx, tx.Events()); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}

	if _, err := rows.Get(ctx, "tenant", "tx", "owner"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("state escaped rollback", err)
	}

	events, err := publisher.List(ctx)
	if err != nil || len(events) != 0 {
		t.Fatal("event escaped rollback", err)
	}

	if err := publisher.Publish(ctx, tx.Events()); err == nil {
		t.Fatal("publication without transaction")
	}

	if err := manager.Do(ctx, func(ctx context.Context) error {
		if err := rows.Create(ctx, *tx); err != nil {
			return err
		}
		if err := publisher.Publish(ctx, tx.Events()); err != nil {
			return err
		}
		return publisher.Publish(ctx, tx.Events())
	}); err != nil {
		t.Fatal(err)
	}

	events, err = publisher.List(ctx)
	if err != nil || !reflect.DeepEqual(events, tx.Events()) {
		t.Fatal("missing or duplicate event", err)
	}

	events[0].After.Intent = "mutated"

	again, err := publisher.List(ctx)
	if err != nil || again[0].After.Intent != "sign_in" {
		t.Fatal("snapshot alias", err)
	}

	if err := manager.Do(ctx, func(ctx context.Context) error { return publisher.Publish(ctx, events) }); err == nil {
		t.Fatal("conflicting event accepted")
	}

	stored, err := rows.Get(ctx, "tenant", "tx", "owner")
	if err != nil || len(stored.Events()) != 0 {
		t.Fatal("rehydration emitted events", err)
	}
}
