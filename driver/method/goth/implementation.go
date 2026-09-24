// Package goth implements Google OAuth using instance-local Goth providers.
package goth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/markbates/goth/providers/google"
	"github.com/not-for-prod/proterror/proterror"

	"authorization/domain/entity"
	"authorization/domain/gateway"
)

const (
	Google                   entity.ProviderID = "google"
	GoogleIssuer                               = "https://accounts.google.com"
	defaultChallengeLifetime                   = 5 * time.Minute
	defaultTimeout                             = 10 * time.Second
)

var _ gateway.CallbackMethod = (*Method)(nil)

// Config belongs to one tenant. Zero durations select a five-minute challenge
// lifetime and a ten-second provider timeout. Now defaults to time.Now.
// Credentials and CallbackURL must remain stable
// while attempts are pending. HTTPClient's transport must be concurrency-safe,
// must not retry token exchanges, and must not log credentials or response bodies.
type Config struct {
	TenantID          entity.TenantID
	ClientID          string
	ClientSecret      string
	CallbackURL       string
	ChallengeLifetime time.Duration
	Timeout           time.Duration
	HTTPClient        *http.Client
	Now               func() time.Time
}

// Verify validates host configuration without retaining supplied values in errors.
func (c *Config) Verify() error {
	if c == nil {
		return errors.Join(
			&proterror.InvalidArgument{},
			errors.New("missing Google OAuth configuration"),
		)
	}
	redirect, err := url.Parse(c.CallbackURL)
	if err != nil || redirect.Hostname() == "" || redirect.User != nil || redirect.Fragment != "" || redirect.Opaque != "" ||
		(redirect.Scheme != "https" && (redirect.Scheme != "http" || (redirect.Hostname() != "localhost" && redirect.Hostname() != "127.0.0.1" && redirect.Hostname() != "::1"))) ||
		strings.TrimSpace(string(c.TenantID)) == "" ||
		strings.TrimSpace(c.ClientID) == "" ||
		strings.TrimSpace(c.ClientSecret) == "" ||
		c.ChallengeLifetime < 0 ||
		c.Timeout < 0 {
		return errors.Join(
			&proterror.InvalidArgument{},
			errors.New("invalid Google OAuth configuration"),
		)
	}
	return nil
}

// Method is safe for concurrent use after construction. Its zero value is unusable.
type Method struct {
	config Config
	client http.Client
	now    func() time.Time
}

// New constructs a Google-only OAuth method without network calls or global registration.
func New(cfg Config) (*Method, error) {
	if err := cfg.Verify(); err != nil {
		return nil, err
	}
	if cfg.ChallengeLifetime == 0 {
		cfg.ChallengeLifetime = defaultChallengeLifetime
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	client := http.Client{}
	if cfg.HTTPClient != nil {
		client = *cfg.HTTPClient
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}
	// Never forward provider credentials across redirects or use shared cookies.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client.Jar = nil
	return &Method{config: cfg, client: client, now: now}, nil
}

// Goth has no context parameter. Each operation gets its own provider and a
// transport bound to the caller's context, with no shared client mutation.
func (m *Method) provider(ctx context.Context) *google.Provider {
	provider := google.New(
		m.config.ClientID,
		m.config.ClientSecret,
		m.config.CallbackURL,
		"openid",
		"email",
		"profile",
	)
	provider.SetAccessType("online")
	client := m.client
	client.Transport = &googleTransport{ctx: ctx, base: client.Transport}
	provider.HTTPClient = &client
	return provider
}
