package inmem

import (
	"context"
	"sync"
	"testing"
	"time"

	"authorization"
)

func TestSessionLifecycle(t *testing.T) {
	now := time.Now().UTC()

	manager, err := NewSessionManager("tenant", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	manager.now = func() time.Time { return now }
	ctx := context.Background()
	req := authorization.CreateSessionRequest{Key: "tx", RecoverUntil: now.Add(time.Minute), Principal: authorization.Principal{
		TenantID: "tenant", UserID: "alice", Evidence: []authorization.Evidence{{Method: "password"}},
	}}

	first, err := manager.Create(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	first.Session.Principal.Evidence[0].Method = "mutated"

	retry, err := manager.Create(ctx, req)
	if err != nil || retry.Token != first.Token || retry.Session.Principal.Evidence[0].Method != "password" {
		t.Fatalf("retry: %+v %v", retry.Session, err)
	}

	if _, err := manager.Validate(ctx, retry.Token); err != nil {
		t.Fatal(err)
	}

	other, _ := NewSessionManager("other", time.Hour)
	if _, err := other.Validate(ctx, retry.Token); err == nil {
		t.Fatal("foreign token accepted")
	}

	changed := req

	changed.Principal.UserID = "other"
	if _, err := manager.Create(ctx, changed); err == nil {
		t.Fatal("principal changed for same key")
	}

	if err := manager.Revoke(ctx, retry.Session.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Validate(ctx, retry.Token); err == nil {
		t.Fatal("revoked token accepted")
	}

	recovered, err := manager.Create(ctx, req)
	if err != nil || recovered.Token != retry.Token {
		t.Fatal("revoke must not mint another token", err)
	}

	req.Key = "expiry"

	issued, err := manager.Create(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(time.Hour)

	if _, err := manager.Validate(ctx, issued.Token); err == nil {
		t.Fatal("expired token accepted")
	}

	if _, err := manager.Create(ctx, req); err == nil {
		t.Fatal("expired recovery accepted")
	}
}

func TestConcurrentSessionRecovery(t *testing.T) {
	manager, err := NewSessionManager("tenant", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	req := authorization.CreateSessionRequest{Key: "tx", RecoverUntil: time.Now().Add(time.Minute), Principal: authorization.Principal{TenantID: "tenant", UserID: "alice"}}

	var wg sync.WaitGroup

	results := make(chan authorization.IssuedSession, 16)

	for range 16 {
		wg.Go(func() {
			session, err := manager.Create(context.Background(), req)
			if err != nil {
				t.Error(err)
				return
			}

			results <- session
		})
	}

	wg.Wait()
	close(results)

	token := ""
	for session := range results {
		if token != "" && session.Token != token {
			t.Fatal("different concurrent tokens")
		}

		token = session.Token
	}
}
