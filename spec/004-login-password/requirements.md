# Requirements
## Problem
The identity repository has no password credential, and the only password driver uses fixed demo credentials.
## Goals
Verify an existing identity's login/password through the configured identity database.
## Non-goals
Enrollment, reset, credential rotation, new SQL adapter, engine lifecycle changes, hash algorithms, new transports and signup.
## Functional requirements
Start accepts StartInput without a challenge. Verify accepts VerifyInput with Login and Password. Method name is password, provider is empty, intent is sign_in. Lookup the exact login under the configured issuer. Compare the stored hash through a host callback. Return the stored UserID and matching timestamped evidence only on success. Reject a conflicting bound UserID.
Missing identities, absent hashes and mismatches return ErrProofRejected. A dummy hash incurs verification work on missing identities or absent hashes and can never authenticate. Repository/verifier failures return sanitized operational errors.
## Data and contract requirements
Identity owns optional PasswordHash as an immutable string. Existing Create/Get/GetBySubject preserve it. The public alias exposes the same field. No interface method or generation changes.
Config requires issuer, dummy encoded hash and a context-aware VerifyPassword callback returning (match, error). The callback must be concurrency-safe and validate algorithm parameters before performing bounded hash work. It returns false,nil only for a valid hash with a wrong password. No algorithm is selected by the engine.
## Validation requirements
Constructor rejects missing dependencies/config. Login and password each accept 1..1024 bytes. Encoded hashes accept 1..4096 bytes. Invalid types including typed nil, nonempty provider, empty requirement, wrong intent/method and unexpected challenge are definitive rejections. Stored nil rows, mismatched lookup keys and missing UserID are operational failures. Context cancellation prevents successful proof.
## Compatibility requirements
Existing identities without hashes continue working with other methods. No normalization or implicit merging. Hosts scope repository/issuer per tenant. The engine remains responsible for eligibility and attempts.
## Open questions
None blocking. Algorithm and cost configuration belong to the host, as approved.
