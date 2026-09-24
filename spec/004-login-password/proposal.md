# Login/password driver
Derived from the owner-approved [visual model](../../docs/event-storming/login-password.md), approved 2026-09-24.
Add driver/method/logpass implementing password SIGN_IN using the identity repository and a host-configured password verifier. Add an optional encoded PasswordHash to Identity. Keep credential provisioning host-owned and reuse engine attempts, policy evaluation and sessions.
