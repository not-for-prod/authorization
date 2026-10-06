package redis_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/not-for-prod/proterror/proterror"
	goredis "github.com/redis/go-redis/v9"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/repository"
	manager "github.com/not-for-prod/authorization/driver/transactionmanager/redis"
)

func redisClient(t *testing.T) *goredis.Client {
	t.Helper()
	address := os.Getenv("AUTHORIZATION_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("AUTHORIZATION_TEST_REDIS_ADDR is required")
	}
	client := goredis.NewClient(&goredis.Options{Addr: address, ContextTimeoutEnabled: true})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	return client
}
func TestAtomicProgressScopes(t *testing.T) {
	client := redisClient(t)
	m, err := manager.New(client)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	value, err := entity.NewTransaction(
		entity.TransactionID(rand.Text()),
		"owner",
		entity.Policy{
			TenantID: entity.TenantID(rand.Text()),
			Intent:   entity.IntentSignIn,
			Root:     entity.MethodRequirement{Method: "password"},
		},
		nil,
		entity.TransactionSettings{
			Lifetime:          time.Minute,
			OperationTimeout:  time.Second,
			MaxAttempts:       5,
			MaxChallenges:     5,
			MaxChallengeBytes: 128,
			MaxPolicyNodes:    8,
		},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	var escaped context.Context
	if err = m.Do(
		t.Context(),
		func(ctx context.Context) error { escaped = ctx; return m.Create(ctx, *value) },
	); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Get(escaped, value.TenantID, value.ID, "owner"); err == nil {
		t.Fatal("escaped scope accepted")
	}
	if _, err = m.Get(t.Context(), value.TenantID, value.ID, ""); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("owner wildcard", err)
	}
	if err = m.Update(t.Context(), *value); err == nil {
		t.Fatal("unscoped write accepted")
	}
	rejected := errors.New("rollback")
	if err = m.Do(t.Context(), func(ctx context.Context) error {
		tx, e := m.Get(ctx, value.TenantID, value.ID, "owner", repository.ForUpdate())
		if e != nil {
			return e
		}
		tx.Attempts = 4
		if e = m.Update(ctx, *tx); e != nil {
			return e
		}
		return rejected
	}); !errors.Is(err, rejected) {
		t.Fatal(err)
	}
	read, err := m.Get(t.Context(), value.TenantID, value.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if read.Attempts != 0 {
		t.Fatal("callback error committed")
	}
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- m.Do(t.Context(), func(ctx context.Context) error {
				tx, e := m.Get(ctx, value.TenantID, value.ID, "owner", repository.ForUpdate())
				if e != nil {
					return e
				}
				ready <- struct{}{}
				<-release
				tx.Attempts++
				return m.Update(ctx, *tx)
			})
		}()
	}
	<-ready
	<-ready
	close(release)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			var aborted *proterror.Aborted
			if errors.As(e, &aborted) {
				conflict++
			} else {
				t.Fatal(e)
			}
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	read, err = m.Get(t.Context(), value.TenantID, value.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if read.Attempts != 1 || !read.ExpiresAt.Equal(value.ExpiresAt) {
		t.Fatal("lost update or extended deadline")
	}
	other, err := manager.New(client)
	if err != nil {
		t.Fatal(err)
	}
	if err = other.Do(t.Context(), func(ctx context.Context) error {
		_, e := m.Get(ctx, value.TenantID, value.ID, "owner", repository.ForUpdate())
		return e
	}); err == nil {
		t.Fatal("foreign scope accepted")
	}
	if err = m.Do(
		t.Context(),
		func(ctx context.Context) error { return m.Do(ctx, func(context.Context) error { return nil }) },
	); err == nil {
		t.Fatal("nested scope accepted")
	}
	canceled, cancel := context.WithCancel(t.Context())
	if err = m.Do(canceled, func(ctx context.Context) error {
		tx, e := m.Get(ctx, value.TenantID, value.ID, "owner", repository.ForUpdate())
		if e != nil {
			return e
		}
		tx.Attempts = 3
		if e = m.Update(ctx, *tx); e != nil {
			return e
		}
		cancel()
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation committed", err)
	}
	cancel()
	read, err = m.Get(t.Context(), value.TenantID, value.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if read.Attempts != 1 {
		t.Fatal("canceled write persisted")
	}
	if err = m.Do(t.Context(), func(ctx context.Context) error {
		tx, e := m.Get(ctx, value.TenantID, value.ID, "owner", repository.ForUpdate())
		if e != nil {
			return e
		}
		tx.ExpiresAt = tx.ExpiresAt.Add(time.Hour)
		return m.Update(ctx, *tx)
	}); err == nil {
		t.Fatal("expiry extended")
	}

	expired := value.Copy()
	expired.ID = entity.TransactionID(rand.Text())
	expired.ExpiresAt = now.Add(-time.Second)
	if err = m.Do(
		t.Context(),
		func(ctx context.Context) error { return m.Create(ctx, expired) },
	); !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatal("expired write accepted", err)
	}
}

func TestProgressExpiresAtOriginalDeadline(t *testing.T) {
	client := redisClient(t)
	m, err := manager.New(client)
	if err != nil {
		t.Fatal(err)
	}
	now, err := client.Time(t.Context()).Result()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := entity.NewTransaction(
		entity.TransactionID(rand.Text()),
		"owner",
		entity.Policy{
			TenantID: entity.TenantID(rand.Text()),
			Intent:   entity.IntentSignIn,
			Root:     entity.MethodRequirement{Method: "password"},
		},
		nil,
		entity.TransactionSettings{
			Lifetime:          time.Second,
			OperationTimeout:  time.Second,
			MaxAttempts:       2,
			MaxChallenges:     2,
			MaxChallengeBytes: 128,
			MaxPolicyNodes:    4,
		},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Do(t.Context(), func(ctx context.Context) error { return m.Create(ctx, *tx) }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		serverNow, timeErr := client.Time(t.Context()).Result()
		if timeErr != nil {
			t.Fatal(timeErr)
		}
		if !serverNow.Before(tx.ExpiresAt) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Redis clock did not reach expiry")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err = m.Get(t.Context(), tx.TenantID, tx.ID, "owner"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("expired progress retained", err)
	}
}
