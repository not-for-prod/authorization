# Login/password authentication

`logpass.New(identities, options...)` implements method `password` for `sign_in`.
It looks up the exact login through `IdentityRepository.GetBySubject`, verifies
that identity's `PasswordHash`, and returns its canonical `UserID` with evidence.
The engine handles user eligibility, pending attempts, policy evaluation and
sessions. No login trimming, case folding, or email matching occurs.

## Host wiring

Use the same identity repository that holds your local identities. Configure the
issuer namespace and repository access for the tenant that owns this driver.
The repository contract itself has no tenant argument.

```go
import (
    "authorization"
    "authorization/driver/method/logpass"
)

password, err := logpass.New(identities, logpass.WithIssuer("my-tenant/local"))
if err != nil {
    return err
}

// Include this tenant option in authorization.New with your existing dependencies
// and WithLimits. Add WithSessionIssuance to the intent only when a session
// manager is also configured on the tenant.
tenant := authorization.WithTenant("my-tenant",
    authorization.WithIntent("sign_in",
        authorization.WithPolicy(authorization.Any("primary",
            authorization.Method(password),
        )),
    ),
)
```

The minimal initialization is `logpass.New(identities)`. Defaults are issuer
`local`, bcrypt verification at exactly `bcrypt.DefaultCost` (10), and a randomly
generated dummy hash created once per constructor call. Provision real identities
with the same issuer and bcrypt cost. The default verifier rejects passwords over
72 bytes, rejects other costs before expensive work, and checks cancellation
before and after comparison; bcrypt itself cannot be interrupted mid-comparison.

Use `WithIssuer`, `WithDummyHash`, and `WithVerifyPassword` to override defaults.
Existing `logpass.New(identities, logpass.Config{...})` calls remain supported;
zero fields select defaults. Options apply in order, and Config overlays nonzero
fields. A custom verifier requires an explicit compatible dummy hash: the driver
cannot generate a hash for an arbitrary host-defined algorithm. For example:

```go
password, err := logpass.New(identities,
    logpass.WithIssuer("my-tenant/local"),
    logpass.WithVerifyPassword(verifyPassword),
    logpass.WithDummyHash(dummyEncodedHash),
)
```

A custom `verifyPassword` uses this signature:

```go
func(ctx context.Context, encodedHash, password string) (bool, error)
```

It must use a password-hashing implementation compatible with provisioned hashes,
validate the encoding and bound algorithm costs **before** expensive work, and
be safe for concurrent calls. Return `(false, nil)` for a wrong password;
return an error for corrupt/unsupported hashes or an operational failure.
Honor cancellation where supported. The driver does not silently upgrade stored hashes. A password mismatch error from a hash library
must be mapped to `(false, nil)` by the host callback.

When overriding defaults, supply a valid dummy hash using the same algorithm and
cost as enrolled hashes.
Unknown logins and identities without a hash invoke the verifier using that dummy
hash, but always fail even when it matches. This reduces a hashing-work timing
difference; it does not guarantee identical database lookup timing. Apply host
rate limiting across transactions. Verification cost remains bounded before hashing.

## Provisioning and storage

Store only an encoded salted password hash in `authorization.Identity.PasswordHash`
when creating the identity through your repository and transaction manager:

```go
identity := authorization.Identity{
    ID:           identityID,
    UserID:       userID,
    Kind:         "local",
    Issuer:       "my-tenant/local",
    Subject:      login,
    PasswordHash: encodedHash,
}
err := transactionManager.Do(ctx, func(ctx context.Context) error {
    return identities.Create(ctx, identity)
})
```

The linked user must already exist and be eligible in the tenant. The in-memory
identity adapter preserves the hash without additional wiring. Production
repository adapters must persist and restore this optional field; this package
introduces no SQL adapter or migration. Empty hashes leave existing OAuth/code
identities usable by their other methods, but disallow password authentication.
Enrollment, reset and rotation are not part of this driver.

## Input and failure contract

Start the engine attempt with `logpass.StartInput{}` and retain its returned
attempt ID. Verify that attempt with a **value** of
`logpass.VerifyInput{Login: login, Password: password}`. A host HTTP decoder should
return those driver-owned values. Pointer inputs, including typed nil pointers,
are rejected. Start returns no challenge and no redirect.

Login and password each allow 1–1024 bytes; encoded hashes are capped at 4096
bytes. The host verifier may enforce a tighter password bound for its algorithm.
Provider must be empty, method must be `password`, and requirement must be set.
The driver is unavailable for signup, update and delete.

Missing identities, hashless identities, wrong passwords, invalid inputs and
conflicting bound users return `authorization.ErrProofRejected`. Operational
storage/hash errors are sanitized and return no proof. The engine currently rolls
back a failed verification, allowing a retry under its existing attempt/expiry
rules. No new retry, lockout or revocation guarantee is introduced.

Never serialize whole identity records to public responses or logs. Repository
adapters and verifier callbacks must also avoid logging credentials or hashes;
the driver cannot redact logs produced within host dependencies. Successful proof
contains only UserID and selection evidence, never the password or its hash.
