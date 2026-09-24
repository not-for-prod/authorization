# Constraints
## Layer ownership
Concrete Goth integration under driver/method/goth. Application owns claim transactions and proof acceptance. Domain retains transitions and identities.
## API and protobuf constraints
No contract changes or generated output required. Host owns route and callback input decoding.
## Domain constraints
Reuse bounded transaction-owned challenges and existing claim/rejection/abandonment behavior. No new aggregates.
## Application service constraints
Claim and publish atomically before I/O. Use the original context outside storage transaction and reload locked state to commit results. Never repeat an uncertain provider exchange. Keep sign-up/sign-in finalization semantics.
## Repository constraints
Existing TransactionManager, Transaction and EventPublisher ports suffice. Claim commits survive later failures.
## Gateway constraints
Use requested Goth SDK with instance-local Google providers, fixed HTTPS endpoints, bounded context-aware HTTP execution and no global registry/cookie store. No automatic exchange retries.
## Migration constraints
None.
## Error handling constraints
Classify local failures with proterror. Preserve ErrProofRejected for definitive proof rejection. Replace unsafe provider diagnostics with sanitized errors, including error chains.
## Tracing and observability constraints
Use prospan without request attributes containing secrets. Do not record state, codes, tokens or upstream bodies.
## Generation constraints
No generated source changes.
