package logpass

// Option configures an optional password-driver setting. Options apply in order.
// Config also implements Option for existing constructor calls.
type Option interface {
	apply(*Config)
}

type optionFunc func(*Config)

func (f optionFunc) apply(config *Config) { f(config) }

// apply overlays nonzero fields; defaults are resolved after all options.
func (c Config) apply(config *Config) {
	if c.Issuer != "" {
		config.Issuer = c.Issuer
	}
	if c.DummyHash != "" {
		config.DummyHash = c.DummyHash
	}
	if c.VerifyPassword != nil {
		config.VerifyPassword = c.VerifyPassword
	}
}

// WithIssuer selects the identity namespace. Empty selects "local".
func WithIssuer(issuer string) Option {
	return optionFunc(func(config *Config) { config.Issuer = issuer })
}

// WithDummyHash replaces the generated hash. It must match the verifier and
// enrolled credentials' algorithm and cost.
func WithDummyHash(hash string) Option {
	return optionFunc(func(config *Config) { config.DummyHash = hash })
}

// WithVerifyPassword replaces bcrypt verification. A custom verifier requires
// a compatible dummy hash supplied by WithDummyHash or Config.
func WithVerifyPassword(verify VerifyPassword) Option {
	return optionFunc(func(config *Config) { config.VerifyPassword = verify })
}
