# Constraints
## Layer ownership
Identity field in domain/entity/user.go. Driver, config and inputs in driver/method/logpass. Production storage and hashing remain host configured.
## API and protobuf constraints
No protobuf or transport changes. Host decoder maps into StartInput / VerifyInput.
## Domain constraints
One optional encoded hash owned by Identity, no independent credential aggregate or mutation command.
## Application service constraints
Do not change engine behavior. Inspection shows current StartMethod/VerifyMethod run the driver in txManager.Do, unlike the older context's outside-lock description. Pass the supplied context to the repository without starting a nested transaction. Current failures roll back; preserve actual existing recovery behavior.
## Repository constraints
Use GetBySubject with exact configured issuer and login, no locking option. Full-value identity copies already preserve string fields. No separate password repository.
## Gateway constraints
No sessions, state writes, custom hash algorithm, credential logging or plaintext retention. One public method per file. Configured verifier must bound cost before work; driver caps sizes. Check cancellation before/after dependencies.
## Migration constraints
No concrete SQL identity adapter/schema exists to migrate. Hosts must persist the new optional field.
## Error handling constraints
Proof rejection uses gateway.ErrProofRejected. Unknown login is proof rejection by explicit owner-approved anti-enumeration behavior. Replace unsafe dependency errors with fixed Unavailable/Internal diagnostics, never join raw errors. Preserve context cancellation with a safe standard sentinel.
## Tracing and observability constraints
Trace verification without input attributes. Record only sanitized errors. Never include hash or password in engine evidence or events.
## Generation constraints
No generated interfaces or protobuf changed; regeneration is not required.
