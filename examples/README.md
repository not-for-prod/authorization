# Auth lab

Run `go run ./examples`, then open **http://localhost:8080/**. The page is embedded
in the Go binary: no Node installation, package manager, or frontend build is needed.
Swagger remains available at `/docs/`.

If the port is occupied, use `AUTH_EXAMPLE_PORT=8081 go run ./examples` and open
http://localhost:8081/. Clay binds the chosen port on all interfaces; this is a local
development example, not a production authentication server.

## Try it

- **Sign in:** create a sign-in transaction, start `login + password`, then enter
  the seeded login `alice` and password `test-password`.
- **Failures:** try a wrong password or an unknown login. The response panel
  displays the API's error.
- **Inspect:** refresh progress, expand the policy, or inspect the principal and
  session. Tokens are only shown in the expandable session result.

The host uses `driver/method/logpass` with bcrypt. It creates Alice's user and
identity at startup, storing only a salted password hash. `logpass.New(identityRepo)`
uses the default local issuer and bcrypt verifier and generates a separate dummy
hash for unknown logins. Both hashes use the same bcrypt cost.
Only sign-in is configured: this driver does not implement password enrollment,
so sign-up, update and delete are unavailable. Use only test data.

The page retains attempt IDs and uses the caller cookie automatically. Reloading
clears UI history; restarting resets in-memory state and seeds Alice again.
No credentials are stored in browser storage.

## Files

- `main.go`: dependencies, policies, seeded user, static page and HTTP/gRPC wiring.
- `web/`: plain HTML, CSS and JavaScript.
- `method.go`: HTTP input decoding into `logpass` inputs.

Tenant is `main`. HTTP bodies use protobuf intent enum `INTENT_SIGN_IN`. The UI exercises the existing `/v1/tenants/main/auth/...` routes;
no API contract changes are required. For gRPC, obtain the continuation cookie
through HTTP and send it as `cookie` metadata.
