# Problem
The engine has no production Telegram method driver.
# Goals
Authenticate with Telegram's official OIDC code flow.
# Non-goals
Legacy widgets, Mini Apps, linking, messaging, additional permissions, engine lifecycle changes.
# Functional requirements
Start stores a random state and PKCE verifier privately, returning a redirect only after engine persistence. Verify and Callback validate correlation before a single code exchange and verify the resulting ID token. Existing engine replay and uncertain-operation fencing remain authoritative.
# Data and contract requirements
Method oauth, provider telegram; identity kind oauth, issuer https://oauth.telegram.org, subject from sub. Host supplies client ID/secret and fixed callback URL. Typed StartInput and CallbackInput belong to driver. No tokens leave the driver.
# Validation requirements
Reject wrong input types, invalid selections, nil/expired/mismatched challenges, malformed private state, provider denial, invalid code, bad signatures, wrong issuer/audience, missing or expired time claims and empty subjects. Bound response sizes and network time. Do not expose raw errors or credentials. Bind private state to tenant and driver configuration.
# Compatibility requirements
Existing APIs, aggregates, events, migrations and mock drivers remain unchanged.
# Open questions
None. RS256 is Telegram's documented default. Default five-minute challenge lifetime is capped by transaction expiry.
