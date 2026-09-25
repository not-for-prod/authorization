// Package logpass verifies existing local identities using host-provisioned password hashes.
package logpass

import (
	"context"
	"crypto/rand"
	"errors"
	"reflect"

	"github.com/not-for-prod/proterror/proterror"
	"golang.org/x/crypto/bcrypt"

	"github.com/not-for-prod/authorization/domain/gateway"
	"github.com/not-for-prod/authorization/domain/repository"
)

const (
	// MaxLoginBytes and MaxPasswordBytes bound untrusted credential input.
	MaxLoginBytes    = 1024
	MaxPasswordBytes = 1024
	// MaxHashBytes bounds the stored encoding, not its algorithm's work factor.
	MaxHashBytes = 4096
)

// VerifyPassword checks an encoded hash against a password. Implementations must
// validate algorithm/cost parameters before work, be safe for concurrent calls,
// honor context cancellation where possible, and never log their arguments.
// Return false,nil for a password mismatch, and an error for an invalid hash or
// operational failure. The driver discards error details to protect credentials.
type VerifyPassword func(ctx context.Context, encodedHash, password string) (bool, error)

// Config selects the identity namespace and the host's password hash verifier.
// Use a repository/issuer scoped to the intended tenant. No login normalization
// is performed. DummyHash must be a valid hash with the same algorithm and cost
// as enrolled credentials; its comparison result can never authenticate a user.
// Empty fields select defaults: issuer local, bcrypt at DefaultCost, and a generated
// dummy hash. A custom VerifyPassword requires an explicit compatible DummyHash.
type Config struct {
	Issuer         string
	DummyHash      string
	VerifyPassword VerifyPassword
}

// Method is a SIGN_IN-only password driver. Construct it with New.
type Method struct {
	identities repository.Identity
	config     Config
}

var _ gateway.Method = (*Method)(nil)

// New configures a driver, generating a dummy bcrypt hash when omitted.
// Without options, identities must use issuer "local" and bcrypt.DefaultCost hashes.
// The host remains responsible for provisioning identities and password hashes.
func New(identities repository.Identity, opts ...Option) (*Method, error) {
	config := Config{}
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("nil logpass option"))
		}
		opt.apply(&config)
	}

	missingRepository := identities == nil
	if !missingRepository {
		value := reflect.ValueOf(identities)
		switch value.Kind() {
		case reflect.Chan,
			reflect.Func,
			reflect.Interface,
			reflect.Map,
			reflect.Pointer,
			reflect.Slice:
			missingRepository = value.IsNil()
		}
	}
	if missingRepository || len(config.DummyHash) > MaxHashBytes ||
		(config.VerifyPassword != nil && config.DummyHash == "") {
		return nil, errors.Join(
			&proterror.InvalidArgument{},
			errors.New("invalid logpass configuration"),
		)
	}

	if config.Issuer == "" {
		config.Issuer = "local"
	}
	if config.VerifyPassword == nil {
		config.VerifyPassword = func(ctx context.Context, encodedHash, password string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			hash := []byte(encodedHash)
			cost, err := bcrypt.Cost(hash)
			if err != nil {
				return false, err
			}
			if cost != bcrypt.DefaultCost {
				return false, errors.New("unsupported bcrypt cost")
			}
			if len(password) > 72 {
				return false, nil
			}
			err = bcrypt.CompareHashAndPassword(hash, []byte(password))
			if ctxErr := ctx.Err(); ctxErr != nil {
				return false, ctxErr
			}
			if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
				return false, nil
			}
			return err == nil, err
		}
		if config.DummyHash == "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(rand.Text()), bcrypt.DefaultCost)
			if err != nil {
				return nil, errors.Join(&proterror.Internal{}, errors.New("dummy password hash generation failed"))
			}
			config.DummyHash = string(hash)
		} else if _, err := config.VerifyPassword(context.Background(), config.DummyHash, ""); err != nil {
			return nil, errors.Join(&proterror.InvalidArgument{}, errors.New("invalid bcrypt dummy hash"))
		}
	}

	return &Method{identities: identities, config: config}, nil
}
