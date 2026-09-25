# Layer ownership
Drivers under driver/<dependency>/pgx and driver/session/redis. Shared pgx scope under driver/internal/postgres, persistence converters under internal/pkg/convert, generated rows under generated/xo.
## API and protobuf constraints
No changes.
## Domain constraints
Preserve roots, events and immutable fields. The owner approved synchronous sign-up's narrow multi-root creation exception.
## Application service constraints
No changes. Provider calls remain outside locks.
## Repository constraints
One host-owned pool and transaction manager, Avito default pgx context for Vibox, explicit scope tracking for lock-before-write and stale context rejection. Parameterized queries use generated table/column constants. No callback retries.
## Gateway constraints
Inject a host-configured Redis client. Use atomic Lua and tenant hash tags. Keep issuance replay data until both deadlines pass. Tokens contain random entropy and no principal data. Never trace tokens or payloads.
## Migration constraints
Goose forward/down migrations, canonical Vibox schema with attribution, fixed versioned authentication topic and partition encoding. Isolated local PostgreSQL for generation/testing.
## Error handling constraints
Use proterrors.FromPG at database origin and preserve repository sentinels. Classify Redis and codec errors at origin.
## Tracing and observability constraints
Use prospan on public I/O boundaries without request attributes. Use Vibox's established trace carrier, restricted to trace context.
## Generation constraints
Restore minimal xo templates for pgx-independent rows and constants. Run make migrations-up and make xo. Do not edit generated output by hand.
