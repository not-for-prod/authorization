package logpass_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	auth "authorization"
	"authorization/domain/repository"
	"authorization/driver/method/logpass"
	repositorymock "authorization/generated/mocks/repository"
)

func TestDefaultPasswordVerification(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, hash, password string
		missing, success     bool
	}{
		{name: "correct", hash: string(hash), password: "correct-password", success: true},
		{name: "wrong", hash: string(hash), password: "wrong-password"},
		{name: "unknown", password: "correct-password", missing: true},
		{name: "hashless", password: "correct-password"},
		{name: "malformed", hash: "invalid", password: "correct-password"},
		{name: "expensive hash", hash: strings.Replace(string(hash), "$10$", "$31$", 1), password: "correct-password"},
		{name: "oversized password", hash: string(hash), password: strings.Repeat("x", 73)},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &repositorymock.IdentityMock{GetBySubjectFunc: func(_ context.Context, issuer, subject string, _ ...repository.SelectOption) (*auth.Identity, error) {
				if issuer != "local" || subject != "alice" {
					t.Fatal("unexpected default identity lookup")
				}
				if test.missing {
					return nil, repository.ErrNotFound
				}
				return &auth.Identity{UserID: "alice-id", Issuer: issuer, Subject: subject, PasswordHash: test.hash}, nil
			}}
			method, err := logpass.New(repo)
			if err != nil {
				t.Fatal(err)
			}
			result, err := method.Verify(t.Context(), auth.Transaction{Intent: "sign_in"}, auth.MethodSelection{Method: "password", Requirement: "primary"}, nil, logpass.VerifyInput{Login: "alice", Password: test.password})
			if test.success {
				if err != nil || result.UserID == nil || *result.UserID != "alice-id" {
					t.Fatalf("default verification failed: %v", err)
				}
			} else if err == nil || result.UserID != nil {
				t.Fatal("invalid credentials accepted")
			}
		})
	}
}

func TestOptionalConfiguration(t *testing.T) {
	repo := &repositorymock.IdentityMock{GetBySubjectFunc: func(_ context.Context, issuer, _ string, _ ...repository.SelectOption) (*auth.Identity, error) {
		if issuer != "custom" {
			t.Fatal("issuer override ignored")
		}
		return nil, repository.ErrNotFound
	}}
	called := false
	verify := func(_ context.Context, hash, _ string) (bool, error) {
		called = true
		if hash != "custom-dummy" {
			t.Fatal("dummy override ignored")
		}
		return true, nil
	}
	method, err := logpass.New(repo, logpass.WithIssuer("custom"), logpass.WithVerifyPassword(verify), logpass.WithDummyHash("custom-dummy"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := method.Verify(t.Context(), auth.Transaction{Intent: "sign_in"}, auth.MethodSelection{Method: "password", Requirement: "primary"}, nil, logpass.VerifyInput{Login: "unknown", Password: "anything"})
	if !called || !errors.Is(err, auth.ErrProofRejected) || result.UserID != nil {
		t.Fatal("dummy match must never authenticate")
	}
	if _, err := logpass.New(repo, logpass.Config{}); err != nil {
		t.Fatal(err)
	}
	if _, err := logpass.New(repo, nil); err == nil {
		t.Fatal("nil option accepted")
	}
	if _, err := logpass.New(repo, logpass.WithVerifyPassword(verify)); err == nil {
		t.Fatal("custom verifier without matching dummy accepted")
	}
}
