# Authentication engine configuration

Import the public `authorization` package and call `New(dependencies, options...)`.
The host supplies storage, access rules, and concrete drivers. Policies declare
which driver satisfies each requirement; there is no separate method list.

```go
import (
    "time"

    auth "authorization"
)

service, err := auth.New(
    auth.Dependencies{
        Events:             eventPublisher,
        Transactions:       transactionRepository,
        TransactionManager: transactionManager,
        Users:              userRepository,
        Identities:         identityRepository,
        Access:             authorizeTenantAndResolveCaller,
        Eligible:           checkUserEligibility,
    },
    auth.WithLimits(auth.TransactionSettings{
        Lifetime:          5 * time.Minute,
        OperationTimeout:  15 * time.Second,
        MaxAttempts:       10,
        MaxChallenges:     4,
        MaxChallengeBytes: 16 * 1024,
        MaxPolicyNodes:    256,
    }),
    auth.WithTenant("acme",
        auth.WithSessionManager(sessionManager),
        auth.WithIntent(auth.IntentSignIn,
            auth.WithPolicy(auth.All(
                auth.Any("primary",
                    auth.Method(passwordDriver),
                    auth.Method(oauthDriver, auth.WithProviders("google", "github")),
                ),
                auth.Any("mfa", auth.Method(totpDriver)),
            )),
            auth.WithSessionIssuance(),
        ),
        auth.WithIntent(auth.IntentUpdate,
            auth.WithPolicy(auth.Any("additional_verification", auth.Method(totpDriver))),
            auth.WithAuthenticatedUserRequired(),
        ),
    ),
)
if err != nil {
    return err
}
```

Dependency variables above belong to the host. A runnable, self-contained test
example is [examples/embedded/host_test.go](../examples/embedded/host_test.go):

```sh
go test -v ./examples/embedded
go test -run TestExternalHost -v .
```

The second command copies the example into an unrelated temporary Go module and
runs it there, proving no host implementation needs an `internal` import. The
current module path is `authorization`; while developing a host in another module,
use `require authorization v0.0.0` and a local `replace authorization => /path/to/authorization`.
No published module address is configured yet.

## Construction and dependency inversion

`Dependencies` contains the required storage ports, event publisher and authorization callbacks.
`Events` must use the same storage transaction as `Transactions`. For local testing,
construct `driver/event/inmem.New(manager)` using the shared in-memory manager.
Public aliases expose all types needed to implement these ports and drivers while
the domain and application implementation remain internal. `AuthenticationMethod`
is the driver interface; `Method(driver, ...)` constructs a policy leaf.

`Option`, `TenantOption`, `IntentOption`, and `MethodOption` are distinct types.
Options cannot be placed at the wrong level. `PolicyNode` is an immutable builder
value produced by `All`, `Any`, and `Method`. A method needs a named enclosing
requirement, normally `Any("primary", Method(driver))`, even for one method.
`All` means all children, not a prescribed execution order. Requirement IDs have
no reserved meaning; existing domain inheritance and uniqueness rules apply.

`New` compiles builder trees into pure domain policies and tenant-specific driver
bindings. No adapter objects enter stored policies or authentication transactions.
Construction calls `Method.Name`, which must be stable and side-effect-free, but
performs no provider or storage I/O. Availability is checked during flows.

Reuse the same driver instance across a tenant's intents. Distinct comparable
values with equal values are also accepted. Different bindings for the same method
ID within one tenant fail construction; different tenants may use different drivers
with the same ID. Prefer pointers for stateful drivers. For a non-comparable value
driver, reuse the same `PolicyNode` instead of independently binding that value twice.

Provider settings and credentials belong in the host's driver constructor.
`WithProviders` only restricts acceptable provider IDs in the policy. The driver
still owns provider configuration and availability.

Configured policies are copied and immutable for the service lifetime. Build a
new service to change them. Each transaction also snapshots its policy and limits,
so existing flows retain their original requirements. The public builder does not
provide YAML loading or dynamic policy-repository injection. The internal policy
port remains available to internal application wiring.

## Defaults and validation

- `WithLimits` is required; every duration and size bound must be positive.
- The default clock is `time.Now`. `WithClock` supplies a concurrency-safe test clock.
- No session manager, JWT implementation, signing keys, or allow-all access rule is installed.
- An intent returns a principal unless it specifies `WithSessionIssuance()`.
- Session issuance requires that tenant's explicit `WithSessionManager(...)`.
- Registering a session manager alone does not enable issuance for any intent.
- `WithAuthenticatedUserRequired()` requires a trusted user at transaction creation.
- Intents are closed lifecycle values: `IntentSignUp`, `IntentSignIn`,
  `IntentUpdate`, and `IntentDelete`. Unknown values are rejected. These values
  select existing-user authentication or initial identity-based sign-up. Reset,
  linking, and deletion workflows remain unimplemented.
- `Limits.IssueSession` is overridden by each intent's completion configuration.

Missing dependencies, typed nil adapters, nil options, duplicate tenants/intents,
multiple policies for an intent, conflicting driver bindings, malformed or oversized
policies, and session issuance without an issuer fail construction with
`proterror.InvalidArgument`. Duplicate scalar service options (`WithLimits`,
`WithClock`) use the last supplied value. Builder errors are returned by `New`.

All configured flows authenticate existing users. Registration and linking are
not implemented. A session manager may use JWT, opaque tokens, or another strategy;
issuance must follow the existing idempotency contract.

## Host access and eligibility

`Access(ctx, tenant)` returns `auth.Caller{Key, UserID}` after the host authorizes
the tenant. `Key` comes from trusted host context, such as a validated continuation
cookie, and must remain stable for the authentication transaction. Resource IDs
and client-chosen strings are not caller credentials. Callback requests must
restore the original caller association safely.

`Caller.UserID` is optional before primary authentication. A supplied
`CreateTransactionRequest.AuthenticatedUserID` must match the trusted caller;
it cannot establish a binding independently. `Eligible(ctx, tenant, user)` owns
user-status and tenant-membership checks before binding and completion.

The [YAML example](examples/auth-engine.yaml) remains a conceptual policy document.
Its future registration/linking examples do not add those operations to the engine.
See [storage obligations](authentication-storage.md),
[driver obligations](authentication-drivers.md), and
[HTTP wiring and testing](authentication-testing.md).
