---
name: gateway-layer
description: Design, implement, review, or refactor Firestarter gateway contracts and adapters under `internal/domain/gateway` and `internal/infrastructure/gateway`. Use when integrating a provider from documentation, including typed config, Resty-based HTTP clients or provider SDKs, authentication or signing, transport DTOs, conversion boundaries, provider-aware errors, and focused gateway tests.
---

# Gateway Layer

## Purpose

Use this skill as the single source of truth for gateway contracts and external API implementations. It covers both design questions and implementation tasks such as `implement gateway @internal/infrastructure/gateway/... <docs link>`.

Gateways are infrastructure adapters behind domain-facing interfaces. Choose an HTTP client or provider SDK deliberately from the provider contract, existing dependencies, and testability.

## Ownership

- `internal/domain/gateway` owns interfaces and domain-facing request/response contracts used by application services.
- `internal/infrastructure/gateway/<provider>-gateway` owns provider clients, HTTP or SDK wiring, provider DTOs, auth/signing, provider error handling, and transport mapping.
- `config` owns provider runtime configuration such as base URL, timeout, credentials, network IDs, and debug flags; Fx passes the typed config to the gateway constructor.
- `internal/pkg/convert` owns reusable transport-neutral mapping only when conversion is shared outside one gateway method/package.

Do not put provider clients, Resty clients, JSON transport structs, provider auth fields, HTTP status codes, SDK response types, or provider-shaped responses into domain gateway contracts.

## Package Shape

Gateway implementation packages usually contain:

- `implementation.go` for shared client wiring, config usage, auth/signing hooks, and interface assertion
- one public method per file, named after the method in snake_case
- private request/response transport models colocated with the method that owns them
- focused tests for request construction, response mapping, auth helpers, and error handling

Prefer provider-local packages over one generic client package containing unrelated integrations.

## Domain Contracts

Domain gateway contracts should be narrower than provider responses. Include only fields the application/domain currently needs.

Avoid:

- JSON tags in domain gateway DTOs
- Resty, HTTP, SDK, or provider client types in domain contracts
- provider-specific auth fields in application-facing requests
- copying entire provider responses upward to avoid mapping

Use typed enums for provider options with a documented closed set, and convert them to provider strings inside the gateway.

## Config

Before adding gateway wiring, inspect the existing config shape. Add only fields required by the provider docs or existing package conventions.

Typical fields:

- `BaseURL string`
- `Timeout time.Duration`
- `Debug bool`
- API key, bearer token, secret, private key, or network identifiers when required

Keep config parsing and validation in config loading. Gateway methods should consume typed, trusted config values.

## HTTP Client Wiring

Use Resty for every direct third-party HTTP integration. Do not introduce or extend raw `net/http` provider
transports. If the target gateway still uses `http.Client`, migrate the affected provider gateway to one shared
Resty client as part of the change instead of copying the legacy pattern.

Use this selection order:

1. use an official SDK when it materially reduces protocol or signing risk and its dependency cost is justified
2. otherwise use Resty for the provider's HTTP API
3. reuse an established Resty client in the target provider package rather than creating another client per method

Add the repository's established Resty module when it is missing, then run `go mod tidy` and review `go.mod` and
`go.sum`. For non-HTTP gateways, keep the same domain/infrastructure boundary rules.

Typical constructor:

```go
type Implementation struct {
    client *resty.Client
}

func NewImplementation(cfg config.Provider) *Implementation {
    return &Implementation{
        client: resty.New().
            SetBaseURL(cfg.BaseURL).
            SetTimeout(cfg.Timeout).
            SetResponseBodyLimit(defaultMaxResponseBytes),
    }
}
```

Prefer injecting a configured Resty client when the repository already has a shared provider. Do not read a config
singleton from every method or constructor. If provider docs require auth or signing, centralize it in Resty request
middleware, hooks, or a gateway-local request builder; do not duplicate signing logic in each method.

Use `SetContext(ctx)`, typed `SetResult`/`SetError` mapping where practical, a bounded response-body limit, and a
gateway-local auth hook. Keep debug logging disabled when it could expose authorization headers, signed requests,
or payload secrets.

## Transport Models

Keep endpoint constants, request payload structs, response payload structs, and mapping helpers in the method file that uses them unless multiple methods genuinely share the same transport contract.

Use private transport DTOs with exact provider JSON field names. Do not reuse domain DTOs as raw HTTP payloads, and do not add provider tags to domain models.

Preferred helper names:

- `new<Method>Request(...)`
- `(r <method>Response) toDomain()`
- `(r <method>Response) toDTO()` when the domain gateway contract returns a DTO

Use `internal/pkg/convert` only for reusable, provider-neutral mapping.

## Implementation Workflow

When implementing a gateway from docs:

1. Inspect the target domain gateway interface and infrastructure package.
2. Inspect config and adjacent gateways for local style.
3. Inspect the provider docs before editing.
4. Identify endpoint path, method, params, body, status codes, response shape, and auth/signing requirements.
5. Add or adjust config fields only when required.
6. Wire the provider client once in shared gateway code. Inject typed config and centralize auth/signing.
7. Add private transport request/response models near each method.
8. Implement the provider call through Resty with `SetContext(ctx)`.
9. Map provider responses into domain-facing DTOs/entities.
10. Preserve local tracing and error conventions.
11. Add focused tests that do not require live provider access unless the repo already has explicit env-gated integration tests.

If docs are ambiguous, inspect adjacent code first, choose the safest endpoint only when defensible, and state the ambiguity.

## Method Shape

Typical HTTP method boundary:

```go
func (i *Implementation) GetSomething(ctx context.Context, req gateway.GetSomethingRequest) (*gateway.GetSomethingResponse, error) {
    ctx, span := prospan.Start(ctx)
    defer span.End()

    var body getSomethingResponse
    resp, err := i.client.R().
        SetContext(ctx).
        SetQueryParams(newGetSomethingQuery(req)).
        SetResult(&body).
        Get("/something")
    if err != nil {
        return nil, span.Err(errors.Join(&proterror.Unavailable{}, err))
    }

    if err = classifyProviderStatus(resp.StatusCode()); err != nil {
        return nil, span.Err(err)
    }

    return body.toDomain(), nil
}
```

Treat every documented 2xx status deliberately. Map authentication, authorization, invalid-request, rate-limit, and provider 5xx responses to the narrowest project error classification. Never place an unbounded raw provider body into an error or span; decode a bounded error DTO and retain only safe diagnostic fields.

Retry only documented transient failures and only when the operation is idempotent or carries a provider-supported idempotency key. Do not add automatic retries to side-effecting requests by default.

After an ambiguous side-effecting create, recover by a stable provider idempotency key or request identity before issuing another create. If the provider offers lookup by a caller-supplied name, persist and reuse that name. Never make “provider ID missing” itself the condition for repeating a paid request.

Gateway recovery does not replace an application-level dispatch claim: two workers can both observe “not found” and race into the create call. The aggregate must durably elect one dispatcher before the recovery-then-create sequence.

Bind lookup results to all immutable request facts available at the boundary: provider request identity, wallet, network, resource or asset type, requested quantity, and relevant period. Treat mismatches and multiple matches as failed preconditions, not successful reconciliation.

When an operator supplies provider-verified completion facts for a security or financial reconciliation, require the bounded evidence digest mandated by the domain contract. Authentication proves who submitted the result; it does not replace evidence binding.

Provider-declared success is insufficient when the business outcome is independently observable. For delegated chain resources, query the chain and expose the available energy or bandwidth; the application may finalize only after the required quantity is visible on-chain.

A provider `PENDING` or 5xx response to a paid create is not retryable merely because it is non-terminal. If the provider exposes no lookup or idempotency key for that created object, return an ambiguous disposition and prohibit automatic POST retries. Reserve retryable status for pre-dispatch failures or safe reads such as GET polling by a persisted provider ID.

Represent definitive provider rejection as a typed domain-facing disposition distinct from transport ambiguity. The application layer must be able to terminally fail the aggregate instead of endlessly retrying the same rejected request.

## Testing

Require deterministic tests that validate:

- request DTO mapping
- response-to-domain mapping
- status/error handling
- auth/signing helper behavior
- endpoint/path/query/body construction through `httptest` when needed
- every supported success status and representative 4xx, rate-limit, and 5xx mappings
- response size or malformed-body handling when the gateway decodes arbitrary provider data

Do not add tests that call real external endpoints by default. Use live calls only when the repository already has env-gated integration tests or the user explicitly asks for them.

## Boundary Smells

Pause and re-check the design if gateway code:

- leaks provider JSON DTOs into `internal/domain/gateway`
- makes application services depend on concrete gateway packages
- hardcodes config values in methods
- concatenates URLs unsafely instead of resolving paths and query parameters with URL helpers
- invents auth behavior not supported by docs
- mirrors provider responses into domain contracts without a current business need
