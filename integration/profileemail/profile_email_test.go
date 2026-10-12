package profileemail_test

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	auth "github.com/not-for-prod/authorization"
	"github.com/not-for-prod/authorization/domain/repository"
	completionpg "github.com/not-for-prod/authorization/driver/completion/pgx"
	eventpg "github.com/not-for-prod/authorization/driver/event/pgx"
	identitypg "github.com/not-for-prod/authorization/driver/identity/pgx"
	transactionredis "github.com/not-for-prod/authorization/driver/transaction/redis"
	managerpg "github.com/not-for-prod/authorization/driver/transactionmanager/pgx"
	managerredis "github.com/not-for-prod/authorization/driver/transactionmanager/redis"
	userpg "github.com/not-for-prod/authorization/driver/user/pgx"
	"github.com/not-for-prod/authorization/internal/testutil/pgfixture"
	"github.com/not-for-prod/proterror/proterror"
	"github.com/redis/go-redis/v9"
)

type callerContextKey struct{}
type failingIdentity struct {
	auth.IdentityRepository
	failNext bool
}

func (r *failingIdentity) Create(ctx context.Context, identity auth.Identity) error {
	if err := r.IdentityRepository.Create(ctx, identity); err != nil {
		return err
	}
	if r.failNext {
		r.failNext = false
		return &proterror.Unavailable{}
	}
	return nil
}

type oauthMethod struct {
	method
	email string
}

func (*oauthMethod) Name() auth.MethodID { return "oauth" }
func (m *oauthMethod) Verify(_ context.Context, _ auth.Transaction, s auth.MethodSelection, _ *auth.Challenge, _ auth.MethodInput) (auth.VerificationResult, error) {
	return auth.VerificationResult{Evidence: auth.Evidence{Requirement: s.Requirement, Method: s.Method, Provider: s.Provider, VerifiedAt: time.Now().UTC()}, Identity: &auth.VerifiedIdentity{Kind: "oauth", Issuer: "https://accounts.google.com", Subject: "stable-google-subject", VerifiedEmail: m.email}}, nil
}

type input struct{ Email string }

func (input) AuthenticationInput() {}

type method struct{}

func (method) Name() auth.MethodID { return "email_code" }
func (method) Available(context.Context, auth.Transaction, auth.MethodSelection) (bool, error) {
	return true, nil
}
func (method) Start(_ context.Context, tx auth.Transaction, s auth.MethodSelection, value auth.MethodInput) (auth.StartResult, error) {
	now := time.Now().UTC()
	return auth.StartResult{Challenge: &auth.Challenge{ID: auth.ChallengeID(rand.Text()), TransactionID: tx.ID, Selection: s, Status: auth.ChallengePending, CreatedAt: now, ExpiresAt: tx.ExpiresAt, State: []byte(value.(input).Email)}}, nil
}
func (method) Verify(_ context.Context, _ auth.Transaction, s auth.MethodSelection, ch *auth.Challenge, _ auth.MethodInput) (auth.VerificationResult, error) {
	return auth.VerificationResult{Evidence: auth.Evidence{Requirement: s.Requirement, Method: s.Method, VerifiedAt: time.Now().UTC()}, Identity: &auth.VerifiedIdentity{Kind: "email", Issuer: "atlas/email", Subject: string(ch.State)}}, nil
}

func TestDurableEmailLinkOwnershipRecoveryAndSelection(t *testing.T) {
	pool := pgfixture.New(t)
	redisURL := os.Getenv("AUTHORIZATION_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("AUTHORIZATION_TEST_REDIS_URL required")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	cache := redis.NewClient(options)
	defer cache.Close()
	ctx := t.Context()
	manager, err := managerpg.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	identities := identitypg.New(manager)
	users := userpg.New(manager)
	a, b := auth.UserID(rand.Text()), auth.UserID(rand.Text())
	for _, id := range []auth.UserID{a, b} {
		if err := manager.Do(ctx, func(ctx context.Context) error {
			if err := users.Create(ctx, auth.User{ID: id, Status: "active", CreatedAt: time.Now().UTC()}); err != nil {
				return err
			}
			return identities.Create(ctx, auth.Identity{ID: auth.IdentityID(rand.Text()), UserID: id, Kind: "oauth", Issuer: "telegram", Subject: string(id), CreatedAt: time.Now().UTC()})
		}); err != nil {
			t.Fatal(err)
		}
	}
	progress, err := managerredis.New(cache)
	if err != nil {
		t.Fatal(err)
	}
	events, err := eventpg.New(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	faults := &failingIdentity{IdentityRepository: identities}
	oauth := &oauthMethod{email: "provider@example.com"}
	caller := auth.Caller{Key: rand.Text(), UserID: &a}
	tenant := auth.TenantID(rand.Text())
	engine, err := auth.New(auth.Dependencies{TransactionManager: progress, CompletionManager: manager, Completions: completionpg.New(manager), Transactions: transactionredis.New(progress), Users: users, Identities: faults, ProfileEmails: identitypg.ProfileEmails{Repository: identities}, Events: events,
		Access: func(ctx context.Context, _ auth.TenantID) (auth.Caller, error) {
			if value, ok := ctx.Value(callerContextKey{}).(auth.Caller); ok {
				return value, nil
			}
			return caller, nil
		}, Eligible: func(_ context.Context, _ auth.TenantID, user auth.User) error {
			if user.Status != "active" {
				return &proterror.Unauthenticated{}
			}
			return nil
		}},
		auth.WithLimits(auth.TransactionSettings{Lifetime: time.Minute, OperationTimeout: 10 * time.Second, MaxAttempts: 5, MaxChallenges: 5, MaxChallengeBytes: 4096, MaxPolicyNodes: 8}),
		auth.WithTenant(tenant,
			auth.WithIntent(auth.IntentUpdate, auth.WithPolicy(auth.Any("primary", auth.Method(method{}))), auth.WithAuthenticatedUserRequired()),
			auth.WithIntent(auth.IntentSignUp, auth.WithPolicy(auth.Any("primary", auth.Method(oauth, auth.WithProviders("google"))))),
			auth.WithIntent(auth.IntentSignIn, auth.WithPolicy(auth.Any("primary", auth.Method(oauth, auth.WithProviders("google")))))))
	if err != nil {
		t.Fatal(err)
	}
	create := func(address string) auth.VerifyMethodRequest {
		t.Helper()
		created, err := engine.CreateTransaction(ctx, auth.CreateTransactionRequest{TenantID: tenant, Intent: auth.IntentUpdate})
		if err != nil {
			t.Fatal(err)
		}
		started, err := engine.StartMethod(ctx, auth.StartMethodRequest{TenantID: tenant, TransactionID: created.Transaction.ID, Selection: auth.MethodSelection{Requirement: "primary", Method: "email_code"}, Input: input{address}})
		if err != nil {
			t.Fatal(err)
		}
		return auth.VerifyMethodRequest{TenantID: tenant, TransactionID: created.Transaction.ID, AttemptID: started.Transaction.PendingAttempt.AttemptID, ChallengeID: &started.Transaction.PendingAttempt.ChallengeID, Input: input{address}}
	}
	req := create("person@example.com")
	caller.UserID = &b
	if _, err := engine.VerifyMethod(ctx, req); err == nil {
		t.Fatal("different session user linked proof")
	}
	caller.UserID = &a
	result, err := engine.VerifyMethod(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || result.Result.Session != nil || result.Result.Principal.UserID != a {
		t.Fatal("link replaced session or user")
	}
	// Retry represents a committed SQL result whose HTTP response was lost.
	replay, err := engine.VerifyMethod(ctx, req)
	if err != nil || replay.Result.Principal.UserID != a {
		t.Fatal("recovery changed user", err)
	}
	email, err := identities.GetAttachedEmail(ctx, a)
	if err != nil || email != "person@example.com" {
		t.Fatal("committed email not readable", email, err)
	}
	telegram, err := identities.GetBySubject(ctx, "telegram", string(a))
	if err != nil || telegram.UserID != a {
		t.Fatal("telegram identity changed", err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM auth_identity WHERE user_id=$1 AND kind='email'", string(a)).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate identity", count, err)
	}
	caller.UserID = &b
	conflict := create("person@example.com")
	if _, err = engine.VerifyMethod(ctx, conflict); err == nil {
		t.Fatal("foreign address was transferred")
	}
	linked, err := identities.GetBySubject(ctx, "atlas/email", "person@example.com")
	if err != nil || linked.UserID != a {
		t.Fatal("ownership changed", err)
	}
	if _, err := identities.GetBySubject(ctx, "atlas/email", "unknown@example.com"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal(err)
	}
	caller.UserID = &a
	replacement := create("replacement@example.com")
	if _, err := engine.VerifyMethod(ctx, replacement); err == nil {
		t.Fatal("established email replaced")
	}
	// SQL failure after inserting the identity rolls back; frozen proof remains retryable.
	caller.UserID = &b
	retry := create("retry@example.com")
	faults.failNext = true
	if _, err := engine.VerifyMethod(ctx, retry); err == nil {
		t.Fatal("write failure reported success")
	}
	if _, err := identities.GetBySubject(ctx, "atlas/email", "retry@example.com"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("failed SQL scope retained identity", err)
	}
	if _, err := engine.VerifyMethod(ctx, retry); err != nil {
		t.Fatal("frozen proof failed to recover", err)
	}
	caller.UserID = nil
	if _, err := engine.VerifyMethod(ctx, retry); err == nil {
		t.Fatal("completion replay allowed without session")
	}
	// OAuth metadata survives frozen Redis proof, signup and subsequent sign-in.
	var oauthUser auth.UserID
	for _, intent := range []auth.Intent{auth.IntentSignUp, auth.IntentSignIn} {
		caller.UserID = nil
		created, err := engine.CreateTransaction(ctx, auth.CreateTransactionRequest{TenantID: tenant, Intent: intent})
		if err != nil {
			t.Fatal(err)
		}
		started, err := engine.StartMethod(ctx, auth.StartMethodRequest{TenantID: tenant, TransactionID: created.Transaction.ID, Selection: auth.MethodSelection{Requirement: "primary", Method: "oauth", Provider: "google"}, Input: input{}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := engine.VerifyMethod(ctx, auth.VerifyMethodRequest{TenantID: tenant, TransactionID: created.Transaction.ID, AttemptID: started.Transaction.PendingAttempt.AttemptID, ChallengeID: &started.Transaction.PendingAttempt.ChallengeID, Input: input{}})
		if err != nil {
			t.Fatal(err)
		}
		if oauthUser != "" && oauthUser != result.Result.Principal.UserID {
			t.Fatal("OAuth metadata changed user")
		}
		oauthUser = result.Result.Principal.UserID
		caller.UserID = &oauthUser
		stored, err := identities.GetBySubject(ctx, "https://accounts.google.com", "stable-google-subject")
		email := ""
		if stored != nil {
			email = stored.VerifiedEmail
		}
		if err != nil || email != oauth.email {
			t.Fatal("OAuth metadata lost", email, err)
		}
		if _, err := identities.GetBySubject(ctx, "atlas/email", oauth.email); !errors.Is(err, repository.ErrNotFound) {
			t.Fatal("OAuth metadata auto-enrolled email login", err)
		}
		oauth.email = "updated-provider@example.com"
	}
	// Two users may prove one address, but only one can own it after commit.
	ids := []auth.UserID{auth.UserID(rand.Text()), auth.UserID(rand.Text())}
	requests := make([]auth.VerifyMethodRequest, 2)
	callers := make([]auth.Caller, 2)
	for j, id := range ids {
		if err := manager.Do(ctx, func(ctx context.Context) error {
			return users.Create(ctx, auth.User{ID: id, Status: "active", CreatedAt: time.Now().UTC()})
		}); err != nil {
			t.Fatal(err)
		}
		caller = auth.Caller{Key: rand.Text(), UserID: &ids[j]}
		callers[j] = caller
		requests[j] = create("race@example.com")
	}
	outcomes := make(chan error, 2)
	var group sync.WaitGroup
	for j := range requests {
		group.Add(1)
		go func(j int) {
			defer group.Done()
			_, err := engine.VerifyMethod(context.WithValue(ctx, callerContextKey{}, callers[j]), requests[j])
			outcomes <- err
		}(j)
	}
	group.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("ownership race did not select exactly one owner", successes)
	}
	winner, err := identities.GetBySubject(ctx, "atlas/email", "race@example.com")
	if err != nil || (winner.UserID != ids[0] && winner.UserID != ids[1]) {
		t.Fatal("wrong concurrent owner", err)
	}

}
