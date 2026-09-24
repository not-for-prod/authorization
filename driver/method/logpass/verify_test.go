package logpass_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/not-for-prod/proterror/proterror"

	auth "authorization"
	"authorization/domain/repository"
	"authorization/driver/method/logpass"
	repositorymock "authorization/generated/mocks/repository"
)

func TestVerify(t *testing.T) {
	row := auth.Identity{
		ID:           "id",
		UserID:       "alice-id",
		Issuer:       "local",
		Subject:      " Alice ",
		PasswordHash: "stored-hash",
	}
	bound, other := row.UserID, auth.UserID("other")
	cases := []struct {
		name         string
		row          *auth.Identity
		lookupErr    error
		verifyErr    error
		matched      bool
		bound        *auth.UserID
		wantHash     string
		wantUser     bool
		wantRejected bool
	}{
		{name: "success", row: &row, matched: true, wantHash: row.PasswordHash, wantUser: true},
		{
			name:     "bound user",
			row:      &row,
			matched:  true,
			bound:    &bound,
			wantHash: row.PasswordHash,
			wantUser: true,
		},
		{name: "wrong password", row: &row, wantHash: row.PasswordHash, wantRejected: true},
		{
			name:         "different user",
			row:          &row,
			matched:      true,
			bound:        &other,
			wantHash:     row.PasswordHash,
			wantRejected: true,
		},
		{
			name:         "unknown login",
			lookupErr:    errors.Join(&proterror.NotFound{}, auth.ErrNotFound),
			wantHash:     "dummy-hash",
			wantRejected: true,
		},
		{
			name:         "dummy matches",
			lookupErr:    auth.ErrNotFound,
			matched:      true,
			wantHash:     "dummy-hash",
			wantRejected: true,
		},
		{
			name: "no hash",
			row: &auth.Identity{
				UserID:  row.UserID,
				Issuer:  row.Issuer,
				Subject: row.Subject,
			},
			matched:      true,
			wantHash:     "dummy-hash",
			wantRejected: true,
		},
		{name: "database error", lookupErr: errors.New("secret password stored-hash")},
		{
			name:      "corrupt hash",
			row:       &row,
			verifyErr: errors.New("secret password stored-hash"),
			wantHash:  row.PasswordHash,
		},
		{name: "nil row"},
		{name: "missing user", row: &auth.Identity{Issuer: row.Issuer, Subject: row.Subject}},
		{
			name: "wrong issuer",
			row:  &auth.Identity{UserID: row.UserID, Issuer: "other", Subject: row.Subject},
		},
		{
			name: "wrong login",
			row:  &auth.Identity{UserID: row.UserID, Issuer: row.Issuer, Subject: "alice"},
		},
		{
			name: "oversized hash",
			row: &auth.Identity{
				UserID:       row.UserID,
				Issuer:       row.Issuer,
				Subject:      row.Subject,
				PasswordHash: strings.Repeat("x", logpass.MaxHashBytes+1),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &repositorymock.IdentityMock{
				GetBySubjectFunc: func(_ context.Context, issuer, subject string, opts ...repository.SelectOption) (*auth.Identity, error) {
					if issuer != row.Issuer || subject != row.Subject || len(opts) != 0 {
						t.Fatal("lookup changed issuer/login or requested a lock")
					}
					return tc.row, tc.lookupErr
				},
			}
			calls := 0
			driver, err := logpass.New(repo, logpass.Config{
				Issuer: row.Issuer, DummyHash: "dummy-hash",
				VerifyPassword: func(_ context.Context, hash, password string) (bool, error) {
					calls++
					if hash != tc.wantHash || password != " secret " {
						t.Fatal("incorrect hash selection or changed password")
					}
					return tc.matched, tc.verifyErr
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			selection := auth.MethodSelection{Requirement: "primary", Method: "password"}
			before := time.Now()
			result, err := driver.Verify(
				t.Context(),
				auth.Transaction{Intent: "sign_in", UserID: tc.bound},
				selection,
				nil,
				logpass.VerifyInput{Login: row.Subject, Password: " secret "},
			)
			if tc.wantUser {
				if err != nil || result.UserID == nil || *result.UserID != row.UserID || result.Identity != nil ||
					result.Evidence.Requirement != selection.Requirement || result.Evidence.Method != selection.Method ||
					result.Evidence.Provider != "" ||
					result.Evidence.VerifiedAt.Before(before) ||
					result.Evidence.VerifiedAt.After(time.Now()) {
					t.Fatalf("invalid successful proof: %v", err)
				}
			} else {
				if err == nil || errors.Is(err, auth.ErrProofRejected) != tc.wantRejected || result != (auth.VerificationResult{}) {
					t.Fatalf("unexpected failure classification/result: %v", err)
				}
				if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "stored-hash") {
					t.Fatal("dependency error exposed credentials")
				}
			}
			wantCalls := 0
			if tc.wantHash != "" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("verifier calls: got %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestInputAndSelectionBoundaries(t *testing.T) {
	driver, err := logpass.New(&repositorymock.IdentityMock{}, logpass.Config{
		Issuer: "local", DummyHash: "dummy", VerifyPassword: func(context.Context, string, string) (bool, error) {
			t.Fatal("invalid input reached verifier")
			return false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := auth.MethodSelection{Method: "password", Requirement: "primary"}
	tx := auth.Transaction{Intent: "sign_in"}
	var typedNil *logpass.VerifyInput
	for _, input := range []auth.MethodInput{
		nil, typedNil, logpass.StartInput{}, logpass.VerifyInput{},
		logpass.VerifyInput{Login: "alice"}, logpass.VerifyInput{Password: "password"},
		logpass.VerifyInput{Login: strings.Repeat("x", logpass.MaxLoginBytes+1), Password: "password"},
		logpass.VerifyInput{Login: "alice", Password: strings.Repeat("x", logpass.MaxPasswordBytes+1)},
	} {
		if result, err := driver.Verify(t.Context(), tx, selection, nil, input); !errors.Is(
			err,
			auth.ErrProofRejected,
		) ||
			result != (auth.VerificationResult{}) {
			t.Fatalf("invalid input accepted: %v", err)
		}
	}
	valid := logpass.VerifyInput{Login: "alice", Password: "password"}
	for _, badSelection := range []auth.MethodSelection{
		{Method: "password"}, {Requirement: "primary", Method: "other"},
		{Requirement: "primary", Method: "password", Provider: "other"},
	} {
		if available, err := driver.Available(t.Context(), tx, badSelection); err != nil ||
			available {
			t.Fatal("invalid selection available")
		}
		if _, err := driver.Start(t.Context(), tx, badSelection, logpass.StartInput{}); !errors.Is(
			err,
			auth.ErrProofRejected,
		) {
			t.Fatal(err)
		}
		if _, err := driver.Verify(t.Context(), tx, badSelection, nil, valid); !errors.Is(
			err,
			auth.ErrProofRejected,
		) {
			t.Fatal(err)
		}
	}
	for _, intent := range []auth.Intent{"sign_up", "update", "delete", ""} {
		unsupported := auth.Transaction{Intent: intent}
		if available, err := driver.Available(t.Context(), unsupported, selection); err != nil ||
			available {
			t.Fatal("unsupported intent available")
		}
		if _, err := driver.Start(t.Context(), unsupported, selection, logpass.StartInput{}); !errors.Is(
			err,
			auth.ErrProofRejected,
		) {
			t.Fatal(err)
		}
		if _, err := driver.Verify(t.Context(), unsupported, selection, nil, valid); !errors.Is(
			err,
			auth.ErrProofRejected,
		) {
			t.Fatal(err)
		}
	}
	if _, err := driver.Verify(t.Context(), tx, selection, &auth.Challenge{}, valid); !errors.Is(
		err,
		auth.ErrProofRejected,
	) {
		t.Fatal(err)
	}
	for _, input := range []auth.MethodInput{nil, (*logpass.StartInput)(nil), valid} {
		if _, err := driver.Start(t.Context(), tx, selection, input); !errors.Is(
			err,
			auth.ErrProofRejected,
		) {
			t.Fatal(err)
		}
	}
	if result, err := driver.Start(t.Context(), tx, selection, logpass.StartInput{}); err != nil ||
		result != (auth.StartResult{}) {
		t.Fatal(err)
	}
}

func TestCancellation(t *testing.T) {
	for _, stage := range []string{"before", "lookup", "verify"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			repo := &repositorymock.IdentityMock{
				GetBySubjectFunc: func(ctx context.Context, issuer, subject string, _ ...repository.SelectOption) (*auth.Identity, error) {
					if stage == "before" {
						t.Fatal("canceled request reached repository")
					}
					if stage == "lookup" {
						cancel()
					}
					return &auth.Identity{
						UserID:       "alice",
						Issuer:       issuer,
						Subject:      subject,
						PasswordHash: "hash",
					}, nil
				},
			}
			driver, err := logpass.New(
				repo,
				logpass.Config{
					Issuer:    "local",
					DummyHash: "dummy",
					VerifyPassword: func(ctx context.Context, _, _ string) (bool, error) {
						if stage != "verify" {
							t.Fatal("canceled request reached verifier")
						}
						cancel()
						return true, nil
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "before" {
				cancel()
			}
			result, err := driver.Verify(
				ctx,
				auth.Transaction{Intent: "sign_in"},
				auth.MethodSelection{Requirement: "primary", Method: "password"},
				nil,
				logpass.VerifyInput{Login: "alice", Password: "password"},
			)
			if !errors.Is(err, context.Canceled) || result != (auth.VerificationResult{}) {
				t.Fatal("canceled verification produced proof")
			}
		})
	}
}

func TestConstructor(t *testing.T) {
	valid := logpass.Config{
		Issuer:         "local",
		DummyHash:      "dummy",
		VerifyPassword: func(context.Context, string, string) (bool, error) { return false, nil },
	}
	for _, config := range []logpass.Config{
		{Issuer: valid.Issuer, DummyHash: valid.DummyHash},
		{Issuer: valid.Issuer, VerifyPassword: valid.VerifyPassword},
		{Issuer: valid.Issuer, DummyHash: strings.Repeat("x", logpass.MaxHashBytes+1), VerifyPassword: valid.VerifyPassword},
	} {
		method, err := logpass.New(&repositorymock.IdentityMock{}, config)
		var invalid *proterror.InvalidArgument
		if method != nil || !errors.As(err, &invalid) {
			t.Fatal("invalid config accepted")
		}
	}
	for _, repo := range []repository.Identity{nil, (*repositorymock.IdentityMock)(nil)} {
		if method, err := logpass.New(repo, valid); method != nil || err == nil {
			t.Fatal("nil repository accepted")
		}
	}
}

func TestMaximumCredentialSizes(t *testing.T) {
	login := strings.Repeat("l", logpass.MaxLoginBytes)
	password := strings.Repeat("p", logpass.MaxPasswordBytes)
	hash := strings.Repeat("h", logpass.MaxHashBytes)
	repo := &repositorymock.IdentityMock{
		GetBySubjectFunc: func(_ context.Context, issuer, subject string, _ ...repository.SelectOption) (*auth.Identity, error) {
			if subject != login {
				t.Fatal("login truncated")
			}
			return &auth.Identity{
				Issuer:       issuer,
				Subject:      subject,
				UserID:       "user",
				PasswordHash: hash,
			}, nil
		},
	}
	driver, err := logpass.New(repo, logpass.Config{
		Issuer: "local", DummyHash: hash,
		VerifyPassword: func(_ context.Context, gotHash, gotPassword string) (bool, error) {
			if gotHash != hash || gotPassword != password {
				t.Fatal("credential truncated")
			}
			return true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := driver.Verify(t.Context(), auth.Transaction{Intent: "sign_in"},
		auth.MethodSelection{Requirement: "primary", Method: "password"}, nil,
		logpass.VerifyInput{Login: login, Password: password})
	if err != nil || result.UserID == nil || *result.UserID != "user" {
		t.Fatalf("maximum sizes rejected: %v", err)
	}
}
