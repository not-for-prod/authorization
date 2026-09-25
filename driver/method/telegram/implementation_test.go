package telegram

import (
	"errors"
	"testing"
	"time"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

func TestConfigurationAndAvailability(t *testing.T) {
	valid := Config{
		ClientID:     "123456",
		ClientSecret: "secret",
		RedirectURL:  "https://host.example/callback",
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.ClientID = "" }, func(c *Config) { c.ClientID = "a:b" },
		func(c *Config) { c.ClientSecret = "" }, func(c *Config) { c.RedirectURL = "http://host.example/callback" },
		func(c *Config) { c.RedirectURL = "https://user:password@host.example/callback" },
		func(c *Config) { c.RedirectURL = "https://host.example/callback#fragment" },
		func(c *Config) { c.RedirectURL = "%%%" }, func(c *Config) { c.Timeout = -time.Second },
		func(c *Config) { c.ChallengeLifetime = -time.Second },
	} {
		config := valid
		change(&config)
		if _, err := New(config); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	m, err := New(valid)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "oauth" || m.config.Timeout != defaultTimeout {
		t.Fatal("wrong method or default timeout")
	}
	tx := entity.Transaction{
		ID:        "tx",
		TenantID:  "tenant",
		Status:    entity.TransactionActive,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	selection := entity.MethodSelection{Requirement: "primary", Method: "oauth", Provider: Provider}
	if ok, err := m.Available(t.Context(), tx, selection); err != nil || !ok {
		t.Fatal("valid method unavailable")
	}
	tx.ExpiresAt = time.Now().Add(-time.Second)
	if ok, err := m.Available(t.Context(), tx, selection); err != nil || ok {
		t.Fatal("expired method available")
	}
	if _, err := m.Start(
		t.Context(),
		tx,
		selection,
		StartInput{},
	); !errors.Is(
		err,
		gateway.ErrProofRejected,
	) {
		t.Fatal("expired start accepted")
	}
	tx.ExpiresAt = time.Now().Add(time.Hour)
	for _, input := range []gateway.MethodInput{nil, (*StartInput)(nil), CallbackInput{}} {
		if _, err := m.Start(
			t.Context(),
			tx,
			selection,
			input,
		); !errors.Is(
			err,
			gateway.ErrProofRejected,
		) {
			t.Fatal("invalid start input accepted")
		}
	}
}
