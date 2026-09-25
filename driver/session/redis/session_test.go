package redis

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/not-for-prod/proterror/proterror"
	goredis "github.com/redis/go-redis/v9"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

func setup(t *testing.T, lifetime time.Duration) (*SessionManager, gateway.CreateSessionRequest) {
	t.Helper()
	addr := os.Getenv("AUTHORIZATION_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("AUTHORIZATION_TEST_REDIS_ADDR is required for Redis integration tests")
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr, ContextTimeoutEnabled: true})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	tenant := entity.TenantID(rand.Text())
	s, err := NewSessionManager(client, tenant, lifetime)
	if err != nil {
		t.Fatal(err)
	}
	// Clean only keys belonging to this test's random tenant, never flush the database.
	t.Cleanup(func() {
		ctx := context.Background()
		var cursor uint64
		for {
			keys, next, err := client.Scan(ctx, cursor, s.prefix+"*", 100).Result()
			if err != nil {
				t.Error(err)
				return
			}
			if len(keys) > 0 {
				if err = client.Del(ctx, keys...).Err(); err != nil {
					t.Error(err)
				}
			}
			cursor = next
			if cursor == 0 {
				return
			}
		}
	})
	return s, gateway.CreateSessionRequest{
		Key:          "tx",
		Principal:    entity.Principal{TenantID: tenant, UserID: "user", AuthenticatedAt: time.Now().UTC()},
		RecoverUntil: time.Now().Add(time.Minute),
	}
}
func TestConcurrentIssuanceAndRecovery(t *testing.T) {
	s, req := setup(t, time.Hour)
	ctx := context.Background()
	const workers = 16
	results := make(chan entity.IssuedSession, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() { defer wg.Done(); issued, err := s.Create(ctx, req); results <- issued; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var original entity.IssuedSession
	for result := range results {
		if original.Token == "" {
			original = result
		}
		if !reflect.DeepEqual(original, result) {
			t.Fatal("concurrent issuance returned different credentials")
		}
	}
	other, err := NewSessionManager(s.client, s.tenant, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := other.Create(ctx, req)
	if err != nil || !reflect.DeepEqual(original, recovered) {
		t.Fatalf("recovery: %v", err)
	}
	session, err := other.Validate(ctx, original.Token)
	if err != nil || !reflect.DeepEqual(session, original.Session) {
		t.Fatalf("validate: %v", err)
	}
	sameInstant := req
	sameInstant.Principal.AuthenticatedAt = sameInstant.Principal.AuthenticatedAt.In(time.FixedZone("other", 3*60*60))
	if again, err := other.Create(ctx, sameInstant); err != nil || again.Token != original.Token {
		t.Fatalf("time-zone changed issuance identity: %v", err)
	}
	changed := req
	changed.Principal.UserID = "other"
	if _, err = other.Create(ctx, changed); err == nil {
		t.Fatal("different principal accepted")
	}
	changed = req
	changed.RecoverUntil = changed.RecoverUntil.Add(time.Minute)
	if again, err := other.Create(ctx, changed); err != nil || !reflect.DeepEqual(original, again) {
		t.Fatalf("retry changed the original issuance: %v", err)
	}
	if err = other.Revoke(ctx, original.Session.ID); err != nil {
		t.Fatal(err)
	}
	if err = other.Revoke(ctx, original.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = other.Validate(ctx, original.Token); err == nil {
		t.Fatal("revoked credential validated")
	}
	recovered, err = other.Create(ctx, req)
	if err != nil || !reflect.DeepEqual(original, recovered) {
		t.Fatalf("revoked issuance recovery: %v", err)
	}
	if _, err = other.Validate(ctx, recovered.Token); err == nil {
		t.Fatal("recovery reactivated revoked session")
	}
	if err = other.Revoke(ctx, "unknown"); err != nil {
		t.Fatal(err)
	}
}
func TestExpiryIsolationAndFailures(t *testing.T) {
	s, req := setup(t, time.Hour)
	ctx := context.Background()
	issued, err := s.Create(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := NewSessionManager(s.client, "different-tenant", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = foreign.Validate(ctx, issued.Token); err == nil {
		t.Fatal("tenant isolation failed")
	}
	if err = foreign.Revoke(ctx, issued.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Validate(ctx, issued.Token); err != nil {
		t.Fatal("foreign revocation changed original")
	}
	for _, token := range []string{"", "invalid", string(issued.Session.ID) + "." + rand.Text()} {
		if _, err = s.Validate(ctx, token); err == nil {
			t.Fatal("invalid credential validated")
		}
	}
	// Set the expiry in the past without sleeping. The server-side check must reject it while issuance recovery remains retained.
	if err = s.client.HSet(ctx, s.prefix+"session:"+digest(string(issued.Session.ID)), "expires", time.Now().Add(-time.Minute).UnixMilli()).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Validate(ctx, issued.Token); err == nil {
		t.Fatal("expired credential validated")
	}
	if again, err := s.Create(ctx, req); err != nil || again.Token != issued.Token {
		t.Fatalf("expired issuance recovery: %v", err)
	}
	ttl, err := s.client.PTTL(ctx, s.prefix+"issuance:"+digest(string(req.Key))).Result()
	if err != nil || ttl < 59*time.Minute {
		t.Fatalf("issuance not retained for session lifetime: %v %v", ttl, err)
	}
	req.RecoverUntil = time.Now().Add(-time.Second)
	if _, err = s.Create(ctx, req); err == nil {
		t.Fatal("expired recovery accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.Create(cancelled, req); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = s.Validate(cancelled, issued.Token); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = s.Revoke(cancelled, issued.Session.ID); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	dead := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: time.Millisecond})
	defer func() {
		if err := dead.Close(); err != nil {
			t.Error(err)
		}
	}()
	unavailable, err := NewSessionManager(dead, s.tenant, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, err = unavailable.Validate(ctx, issued.Token)
	var outage *proterror.Unavailable
	if !errors.As(err, &outage) {
		t.Fatalf("unclassified outage: %v", err)
	}
}
func TestRecoveryRetentionOutlivesSession(t *testing.T) {
	s, req := setup(t, time.Second)
	ctx := context.Background()
	issued, err := s.Create(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{s.prefix + "issuance:" + digest(string(req.Key)), s.prefix + "session:" + digest(string(issued.Session.ID))} {
		ttl, err := s.client.PTTL(ctx, key).Result()
		if err != nil || ttl < 50*time.Second {
			t.Fatalf("recovery retention too short: %v %v", ttl, err)
		}
	}
}
func TestConstructorValidation(t *testing.T) {
	var nilClient *goredis.Client
	if _, err := NewSessionManager(nilClient, "tenant", time.Hour); err == nil {
		t.Fatal("nil client accepted")
	}
	client := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})
	defer func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := NewSessionManager(client, "", time.Hour); err == nil {
		t.Fatal("empty tenant accepted")
	}
	if _, err := NewSessionManager(client, "tenant", 0); err == nil {
		t.Fatal("zero lifetime accepted")
	}
}
