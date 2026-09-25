// Package telegram implements Telegram's OIDC authorization-code flow with PKCE.
// Hosts own credentials, browser callback association and engine persistence.
package telegram

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/domain/gateway"
)

const (
	Issuer                                = "https://oauth.telegram.org"
	Provider            entity.ProviderID = "telegram"
	defaultLifetime                       = 5 * time.Minute
	defaultTimeout                        = 10 * time.Second
	maxResponseBytes                      = 64 << 10
	randomBytes                           = 32
	encodedRandomLength                   = 43
	maxCodeBytes                          = 8192
)

// Config contains host-owned credentials, never policy or callback input.
// Zero durations select a five-minute challenge and ten-second HTTP timeout.
// RedirectURL must be an HTTPS URL registered with BotFather.
type Config struct {
	ClientID          string
	ClientSecret      string
	RedirectURL       string
	ChallengeLifetime time.Duration
	Timeout           time.Duration
}

func (c *Config) Verify() error {
	redirect, err := url.Parse(c.RedirectURL)
	if err != nil || redirect.Scheme != "https" || redirect.Hostname() == "" ||
		redirect.User != nil || redirect.Fragment != "" || redirect.Opaque != "" ||
		strings.TrimSpace(c.ClientID) == "" || strings.ContainsAny(c.ClientID, ":\r\n") ||
		strings.TrimSpace(c.ClientSecret) == "" || c.Timeout < 0 || c.ChallengeLifetime < 0 {
		return errors.Join(
			&proterror.InvalidArgument{},
			errors.New("invalid Telegram driver configuration"),
		)
	}
	return nil
}

// Method is safe for concurrent use after construction. Its zero value is unusable.
type Method struct {
	config   Config
	client   *resty.Client
	tokenURL string
	keysURL  string
}

var _ gateway.CallbackMethod = (*Method)(nil)

// New validates and copies configuration. It does not contact Telegram.
func New(config Config) (*Method, error) {
	if err := config.Verify(); err != nil {
		return nil, err
	}
	if config.Timeout == 0 {
		config.Timeout = defaultTimeout
	}
	if config.ChallengeLifetime == 0 {
		config.ChallengeLifetime = defaultLifetime
	}
	return &Method{
		config: config,
		client: resty.New().SetTimeout(config.Timeout).SetResponseBodyLimit(maxResponseBytes).
			SetRedirectPolicy(resty.NoRedirectPolicy()).SetRetryCount(0).SetDebug(false),
		tokenURL: Issuer + "/token",
		keysURL:  Issuer + "/.well-known/jwks.json",
	}, nil
}

// challengeState is private engine-persisted data, never a transport response.
type challengeState struct {
	State       string          `json:"state"`
	Verifier    string          `json:"verifier"`
	TenantID    entity.TenantID `json:"tenant_id"`
	ClientID    string          `json:"client_id"`
	RedirectURL string          `json:"redirect_url"`
}
