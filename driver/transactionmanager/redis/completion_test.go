package redis_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/not-for-prod/proterror/proterror"

	"github.com/jackc/pgx/v5/pgxpool"

	auth "github.com/not-for-prod/authorization"
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
	completionpg "github.com/not-for-prod/authorization/driver/completion/pgx"
	eventpg "github.com/not-for-prod/authorization/driver/event/pgx"
	identitypg "github.com/not-for-prod/authorization/driver/identity/pgx"
	sessionredis "github.com/not-for-prod/authorization/driver/session/redis"
	transactionredis "github.com/not-for-prod/authorization/driver/transaction/redis"
	managerpg "github.com/not-for-prod/authorization/driver/transactionmanager/pgx"
	managerredis "github.com/not-for-prod/authorization/driver/transactionmanager/redis"
	userpg "github.com/not-for-prod/authorization/driver/user/pgx"
	"github.com/not-for-prod/authorization/internal/testutil/pgfixture"
)

type proof struct{}

func (proof) AuthenticationInput() {}

type verifiedMethod struct {
	starts  atomic.Int32
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
	subject string
}

func (*verifiedMethod) Name() auth.MethodID { return "test" }
func (*verifiedMethod) Available(context.Context, auth.Transaction, auth.MethodSelection) (bool, error) {
	return true, nil
}

func (m *verifiedMethod) Start(
	_ context.Context,
	tx auth.Transaction,
	selection auth.MethodSelection,
	_ auth.MethodInput,
) (auth.StartResult, error) {
	m.starts.Add(1)
	if m.entered != nil {
		m.entered <- struct{}{}
		<-m.release
	}
	return auth.StartResult{
		Challenge: &auth.Challenge{
			ID:            auth.ChallengeID(rand.Text()),
			TransactionID: tx.ID,
			Selection:     selection,
			State:         []byte("private"),
			Status:        entity.ChallengePending,
			CreatedAt:     time.Now().UTC(),
			ExpiresAt:     tx.ExpiresAt,
		},
	}, nil
}

func (m *verifiedMethod) Verify(
	_ context.Context,
	_ auth.Transaction,
	s auth.MethodSelection,
	_ *auth.Challenge,
	_ auth.MethodInput,
) (auth.VerificationResult, error) {
	m.calls.Add(1)
	return auth.VerificationResult{
		Identity: &auth.VerifiedIdentity{Kind: "email", Issuer: "test", Subject: m.subject},
		Evidence: auth.Evidence{
			Requirement: s.Requirement,
			Method:      s.Method,
			Provider:    s.Provider,
			VerifiedAt:  time.Now().UTC(),
		},
	}, nil
}

func (m *verifiedMethod) Callback(
	ctx context.Context,
	tx auth.Transaction,
	s auth.MethodSelection,
	c auth.Challenge,
	input auth.MethodInput,
) (auth.VerificationResult, error) {
	return m.Verify(ctx, tx, s, &c, input)
}

type checkedSessions struct {
	gateway.SessionManager
	pool  *pgxpool.Pool
	fail  atomic.Bool
	token string
	mu    sync.Mutex
}

func (s *checkedSessions) Create(ctx context.Context, req gateway.CreateSessionRequest) (entity.IssuedSession, error) {
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM auth_completion WHERE tenant_id=$1 AND id=$2", string(req.Principal.TenantID), string(req.Key)).
		Scan(&count); err != nil {
		return entity.IssuedSession{}, err
	}
	if count != 1 {
		return entity.IssuedSession{}, errors.New("session called before receipt commit")
	}
	result, err := s.SessionManager.Create(ctx, req)
	if err != nil {
		return result, err
	}
	s.mu.Lock()
	s.token = result.Token
	s.mu.Unlock()
	if s.fail.Swap(false) {
		return entity.IssuedSession{}, errors.New("lost session acknowledgement")
	}
	return result, nil
}

type failingEvents struct {
	repository.EventPublisher
	fail atomic.Bool
}

func (e *failingEvents) Publish(ctx context.Context, events []entity.TransactionEvent) error {
	if err := e.EventPublisher.Publish(ctx, events); err != nil {
		return err
	}
	if e.fail.Swap(false) {
		return errors.New("fail after outbox append")
	}
	return nil
}

type uncertainManager struct {
	repository.TransactionManager
	fail atomic.Bool
}

func (m *uncertainManager) Do(ctx context.Context, op func(context.Context) error) error {
	if err := m.TransactionManager.Do(ctx, op); err != nil {
		return err
	}
	if m.fail.Swap(false) {
		return errors.New("lost PostgreSQL commit acknowledgement")
	}
	return nil
}

type callerKey struct{}
type deleteUserKey struct{}
type completionFixture struct {
	clock    *atomic.Int64
	engine   auth.Authentication
	pool     *pgxpool.Pool
	tenant   auth.TenantID
	method   *verifiedMethod
	sessions *checkedSessions
	events   *failingEvents
	manager  *uncertainManager
	progress repository.Transaction
	receipts *completionpg.Repository
	ctx      context.Context
}

func completionSetup(t *testing.T) completionFixture {
	t.Helper()
	client := redisClient(t)
	pool := pgfixture.New(t)
	if _, err := pool.Exec(t.Context(), "DROP TABLE auth_transaction"); err != nil {
		t.Fatal(err)
	}
	pg, err := managerpg.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	rm, err := managerredis.New(client)
	if err != nil {
		t.Fatal(err)
	}
	events, err := eventpg.New(t.Context(), pg)
	if err != nil {
		t.Fatal(err)
	}
	tenant := auth.TenantID(rand.Text())
	sessions, err := sessionredis.NewSessionManager(client, tenant, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	checked := &checkedSessions{SessionManager: sessions, pool: pool}
	method := &verifiedMethod{subject: rand.Text() + "@example.test"}
	failEvents := &failingEvents{EventPublisher: events}
	uncertain := &uncertainManager{TransactionManager: pg}
	receipts := completionpg.New(pg)
	clock := &atomic.Int64{}
	engine, err := auth.New(auth.Dependencies{
		Transactions: transactionredis.New(rm), TransactionManager: rm,
		Completions: receipts, CompletionManager: uncertain,
		Users: userpg.New(pg), Identities: identitypg.New(pg), Events: failEvents,
		Access: func(ctx context.Context, _ auth.TenantID) (auth.Caller, error) {
			owner, _ := ctx.Value(callerKey{}).(string)
			id, _ := ctx.Value(deleteUserKey{}).(*auth.UserID)
			return auth.Caller{Key: owner, UserID: id}, nil
		},
		Eligible: func(_ context.Context, _ auth.TenantID, user auth.User) error {
			if user.Status != auth.UserStatusActive {
				return &proterror.PermissionDenied{}
			}
			return nil
		},
	}, auth.WithClock(func() time.Time {
		if n := clock.Load(); n != 0 {
			return time.Unix(0, n).UTC()
		}
		return time.Now().UTC()
	}), auth.WithLimits(auth.TransactionSettings{Lifetime: time.Minute, OperationTimeout: 10 * time.Second, MaxAttempts: 5, MaxChallenges: 5, MaxChallengeBytes: 128, MaxPolicyNodes: 8}),
		auth.WithTenant(
			tenant,
			auth.WithSessionManager(checked),
			auth.WithIntent(
				auth.IntentDelete,
				auth.WithPolicy(auth.Any("primary", auth.Method(method))),
				auth.WithAuthenticatedUserRequired(),
			),
			auth.WithIntent(
				auth.IntentSignUp,
				auth.WithPolicy(auth.Any("primary", auth.Method(method))),
				auth.WithSessionIssuance(),
			),
			auth.WithIntent(
				auth.IntentSignIn,
				auth.WithPolicy(auth.Any("primary", auth.Method(method))),
				auth.WithSessionIssuance(),
			),
		))
	if err != nil {
		t.Fatal(err)
	}
	return completionFixture{
		clock:    clock,
		engine:   engine,
		pool:     pool,
		tenant:   tenant,
		method:   method,
		sessions: checked,
		events:   failEvents,
		manager:  uncertain,
		progress: transactionredis.New(rm),
		receipts: receipts,
		ctx:      context.WithValue(t.Context(), callerKey{}, "owner"),
	}
}
func (f completionFixture) start(t *testing.T, intent auth.Intent) auth.VerifyMethodRequest {
	t.Helper()
	created, err := f.engine.CreateTransaction(f.ctx, auth.CreateTransactionRequest{TenantID: f.tenant, Intent: intent})
	if err != nil {
		t.Fatal(err)
	}
	started, err := f.engine.StartMethod(
		f.ctx,
		auth.StartMethodRequest{
			TenantID:      f.tenant,
			TransactionID: created.Transaction.ID,
			Selection:     auth.MethodSelection{Requirement: "primary", Method: "test"},
			Input:         proof{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return auth.VerifyMethodRequest{
		TenantID:      f.tenant,
		TransactionID: created.Transaction.ID,
		AttemptID:     started.AttemptID,
		ChallengeID:   &started.Challenge.ID,
		Input:         proof{},
	}
}
func (f completionFixture) counts(t *testing.T, accounts, completions int) {
	t.Helper()
	for table, want := range map[string]int{"auth_user": accounts, "auth_identity": accounts, "auth_completion": completions, "event_log": completions} {
		var got int
		if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count=%d want=%d", table, got, want)
		}
	}
}
func TestCompletionRecoversAfterSessionFailure(t *testing.T) {
	f := completionSetup(t)
	req := f.start(t, auth.IntentSignUp)
	f.sessions.fail.Store(true)
	if _, err := f.engine.VerifyMethod(f.ctx, req); err == nil {
		t.Fatal("session failure hidden")
	}
	f.counts(t, 1, 1)
	originalToken := f.sessions.token
	// Delete only this test's progress by its exact hashed key, emulating loss.
	client := redisClient(t)
	sum := sha256.Sum256([]byte(string(req.TenantID) + "\x00" + string(req.TransactionID)))
	if err := client.Del(f.ctx, "authorization:transaction:v1:{"+hex.EncodeToString(sum[:])+"}").Err(); err != nil {
		t.Fatal(err)
	}
	recovered, err := f.engine.VerifyMethod(f.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Result.Session.Token != originalToken || f.method.calls.Load() != 1 {
		t.Fatal("recovery repeated proof or session")
	}
	progress, err := f.engine.GetTransaction(
		f.ctx,
		auth.GetTransactionRequest{TenantID: f.tenant, TransactionID: req.TransactionID},
	)
	if err != nil || progress.Transaction.Status != entity.TransactionCompleted {
		t.Fatalf("receipt progress: %v", err)
	}
	wrong := req
	wrong.AttemptID = "wrong"
	if _, err = f.engine.VerifyMethod(f.ctx, wrong); err == nil {
		t.Fatal("wrong attempt recovered")
	}
	if _, err = f.engine.VerifyMethod(context.WithValue(f.ctx, callerKey{}, "other"), req); err == nil {
		t.Fatal("wrong caller recovered")
	}
	wrong = req
	challenge := auth.ChallengeID("wrong")
	wrong.ChallengeID = &challenge
	if _, err = f.engine.VerifyMethod(f.ctx, wrong); err == nil {
		t.Fatal("wrong challenge recovered")
	}
	signin := f.start(t, auth.IntentSignIn)
	result, err := f.engine.VerifyMethod(f.ctx, signin)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Principal.UserID != recovered.Result.Principal.UserID {
		t.Fatal("signin changed user")
	}
	f.counts(t, 1, 2)
}
func TestFrozenProofConcurrentRecoveryAndRollback(t *testing.T) {
	for _, failure := range []string{"outbox rollback", "lost commit"} {
		t.Run(failure, func(t *testing.T) {
			f := completionSetup(t)
			req := f.start(t, auth.IntentSignUp)
			if failure == "outbox rollback" {
				f.events.fail.Store(true)
			} else {
				f.manager.fail.Store(true)
			}
			if _, err := f.engine.VerifyMethod(f.ctx, req); err == nil {
				t.Fatal("injected failure hidden")
			}
			if failure == "outbox rollback" {
				f.counts(t, 0, 0)
			} else {
				f.counts(t, 1, 1)
			}
			var wg sync.WaitGroup
			results := make(chan error, 8)
			for range 8 {
				wg.Add(1)
				go func() { defer wg.Done(); _, err := f.engine.VerifyMethod(f.ctx, req); results <- err }()
			}
			wg.Wait()
			close(results)
			// Concurrent marker CAS may return Aborted; the committed receipt remains recoverable.
			for err := range results {
				if err != nil {
					var aborted *proterror.Aborted
					if !errors.As(err, &aborted) {
						t.Fatal(err)
					}
				}
			}
			if _, err := f.engine.VerifyMethod(f.ctx, req); err != nil {
				t.Fatal(err)
			}
			f.counts(t, 1, 1)
			if f.method.calls.Load() != 1 {
				t.Fatal("recovery called provider again")
			}
		})
	}
}
func TestCallbackCompletionRecovery(t *testing.T) {
	f := completionSetup(t)
	req := f.start(t, auth.IntentSignUp)
	callback := auth.CompleteCallbackRequest{
		TenantID:      req.TenantID,
		TransactionID: req.TransactionID,
		AttemptID:     req.AttemptID,
		ChallengeID:   *req.ChallengeID,
		Input:         proof{},
	}
	f.sessions.fail.Store(true)
	if _, err := f.engine.CompleteCallback(f.ctx, callback); err == nil {
		t.Fatal("session failure hidden")
	}
	result, err := f.engine.CompleteCallback(f.ctx, callback)
	if err != nil || result.Result == nil {
		t.Fatalf("recovery failed: %v", err)
	}
	if f.method.calls.Load() != 1 {
		t.Fatal("callback replayed provider")
	}
	if _, err = f.engine.VerifyMethod(f.ctx, req); err == nil {
		t.Fatal("verify recovered callback receipt")
	}
	f.counts(t, 1, 1)
}

func TestRecoveryDeadlineAndCompetingSignup(t *testing.T) {
	f := completionSetup(t)
	req := f.start(t, auth.IntentSignUp)
	if _, err := f.engine.VerifyMethod(f.ctx, req); err != nil {
		t.Fatal(err)
	}
	duplicate := f.start(t, auth.IntentSignUp)
	if _, err := f.engine.VerifyMethod(f.ctx, duplicate); !errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("competing signup: %v", err)
	}
	f.counts(t, 1, 1)
	receipt, err := f.receipts.Get(f.ctx, f.tenant, req.TransactionID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Store(receipt.ExpiresAt.UnixNano())
	if _, err = f.engine.VerifyMethod(f.ctx, req); !errors.Is(err, entity.ErrTransactionExpired) {
		t.Fatalf("expired recovery: %v", err)
	}
	f.counts(t, 1, 1)
}

func TestConcurrentStartDispatchesOnce(t *testing.T) {
	f := completionSetup(t)
	f.method.entered = make(chan struct{}, 2)
	f.method.release = make(chan struct{})
	created, err := f.engine.CreateTransaction(
		f.ctx,
		auth.CreateTransactionRequest{TenantID: f.tenant, Intent: auth.IntentSignUp},
	)
	if err != nil {
		t.Fatal(err)
	}
	req := auth.StartMethodRequest{
		TenantID:      f.tenant,
		TransactionID: created.Transaction.ID,
		Selection:     auth.MethodSelection{Requirement: "primary", Method: "test"},
		Input:         proof{},
	}
	first := make(chan error, 1)
	go func() { _, startErr := f.engine.StartMethod(f.ctx, req); first <- startErr }()
	select {
	case <-f.method.entered:
	case <-time.After(5 * time.Second):
		close(f.method.release)
		t.Fatal("first start did not dispatch")
	}
	second := make(chan error, 1)
	go func() { _, startErr := f.engine.StartMethod(f.ctx, req); second <- startErr }()
	select {
	case secondErr := <-second:
		close(f.method.release)
		if secondErr == nil {
			t.Fatal("concurrent start accepted")
		}
	case <-f.method.entered:
		close(f.method.release)
		<-first
		<-second
		t.Fatal("concurrent request dispatched provider")
	case <-time.After(5 * time.Second):
		close(f.method.release)
		t.Fatal("concurrent start did not return")
	}
	if err = <-first; err != nil {
		t.Fatal(err)
	}
	if f.method.starts.Load() != 1 {
		t.Fatal("provider dispatched more than once")
	}
}

func TestConcurrentSignupsKeepOneIdentity(t *testing.T) {
	f := completionSetup(t)
	first := f.start(t, auth.IntentSignUp)
	second := f.start(t, auth.IntentSignUp)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, req := range []auth.VerifyMethodRequest{first, second} {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.engine.VerifyMethod(f.ctx, req); results <- err }()
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, repository.ErrAlreadyExists) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	f.counts(t, 1, 1)
}
