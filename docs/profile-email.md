# Profile email

Configure the optional `Dependencies.ProfileEmails` port with
`driver/identity/pgx.ProfileEmails` and enable an authenticated email-only
`IntentUpdate` tenant policy. This port supports the write-side existing-email
check and trusted metadata persistence; it is not a profile query API. Profile
presentation reads and selection belong to the host application, which can read
its identity storage independently. Authorization's public Authentication interface
has no `GetProfileEmail` method. Missing write-side repository support fails explicitly. Apply migration `00005_profile_email.sql`
before starting the updated identity adapter.

For attachment ownership checks, direct email takes precedence over existing
verified OAuth metadata. The host owns display preference and normalization. Metadata does not alter the provider subject, create
an email login identity, or merge users. Google metadata requires a boolean verified
claim; malformed/unverified email is discarded. Existing OAuth identities can be
enriched on subsequent successful signin.

UPDATE requires the original authenticated user on create/start/verify/read and
receipt recovery. The email proof attaches a direct identity under a canonical
user lock. Foreign ownership and replacement fail. Same-user retries are safe.
Completion uses the existing frozen-proof/SQL-receipt protocol, with no new session
or signup provisioning. Later email-code signin resolves the same user.

`integration/profileemail` exercises isolated PostgreSQL storage and Redis durable
completion, including SQL rollback recovery, changed caller, replay, OAuth metadata
and concurrent claims. Set `AUTHORIZATION_TEST_DATABASE_URL` and
`AUTHORIZATION_TEST_REDIS_URL` to run it; it creates/removes an isolated schema and
never flushes Redis. Its method driver is a fixture; it does not send real email.
