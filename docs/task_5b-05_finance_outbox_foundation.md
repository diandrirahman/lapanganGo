# Task 5B-05 -- Finance Outbox Foundation

- Status: **READY FOR 5B-06**
- Date: 2026-07-29
- Runtime mode: Xendit Test Mode only
- Runtime flags: payment capabilities remain `false`
- Scope: migration 026, transaction-aware outbox enqueue, idempotency, redacted payload validation, and lease lifecycle primitives

## Result

Migration `026_payment_provider_outbox` adds the durable
`payment_provider_commands` outbox. It stores canonical command identity,
request hash, a bounded typed payment payload, retry state, per-claim lease
metadata, a malformed-response-specific counter, normalized error code, and a
sanitized provider reference.

Payment-create commands use a restrictive reference to `payment_attempts`.
Inquiry and refund command values remain reserved in the frozen command
taxonomy, but migration 026 and the repository reject them. Inquiry receives
its own canonical payload and request hash in Task 5B-07; refund commands wait
for the refund aggregate migration. A create hash is never reused as an inquiry
hash.

The down migration refuses to remove the table while command rows exist.
Operators must preflight `count(*) = 0`. A refused `golang-migrate` rollback
preserves the table but leaves migration metadata at `25|dirty`; after verifying
the version-26 schema and rows remain intact, recovery is `migrate force 26`.

## Repository boundary

`apps/api/internal/paymentoutbox` provides:

- `EnqueueTx`, which participates in a caller-owned `pgx.Tx` and never begins,
  commits, or rolls back that transaction;
- same-key/same-hash/same-payload/same-aggregate replay;
- same-key with a different hash, payload, or aggregate identity returns
  `ErrIdempotencyConflict`;
- command-specific deterministic keys derived from immutable booking/attempt
  identity;
- payload and request hash checked against the referenced payment attempt in
  the caller transaction;
- database trigger enforcement of the same canonical identity and one command
  per command-type/aggregate;
- fail-closed rejection of `PAYMENT_INQUIRY` until Task 5B-07 defines its
  distinct canonical payload and hash;
- `ClaimNext` using a database lease and `FOR UPDATE SKIP LOCKED`;
- lease duration accepts only PostgreSQL-representable, microsecond-aligned
  values from one microsecond through 24 hours and computes expiry from the
  database transaction timestamp using integer microseconds;
- immediate commands derive their default `available_at` from PostgreSQL
  `transaction_timestamp()`, avoiding API/database clock-skew claim delays;
- retry completion accepts a microsecond-aligned bounded relative delay from
  zero through 24 hours and computes `available_at` from the same PostgreSQL
  transaction timestamp, so worker clock skew and timestamp precision loss
  cannot advance or postpone retry eligibility;
- a new opaque UUID lease token on every claim;
- lease owners restricted to `worker:<canonical-uuid>` in both Go and SQL;
- retryable, succeeded, and terminal lease completion using the exact current
  owner and token;
- separate retryable and terminal error-code allowlists;
- one bounded retry for `MALFORMED_RESPONSE`, followed by atomic
  terminalization; this budget uses `malformed_response_count` and is
  independent of claim/reclaim `attempt_count`;
- stale-lease protection even when a restarted process reuses the same worker
  owner.

The database independently enforces identity and lifecycle. The command primary
key, canonical identity, and payload cannot be changed. Commands must be
inserted as `PENDING`; only claim, expired-lease reclaim, retry, success, and
terminal transitions are accepted. Attempt counters and timestamps cannot
regress, terminal rows are immutable, and command rows cannot be deleted or
truncated. The truncate guard also blocks a cascading truncate originating from
a referenced payment-attempt table. All three guard triggers are `ENABLE
ALWAYS`, so even a session using PostgreSQL replica mode cannot bypass them.

No worker, provider HTTP call, webhook, refund execution, booking `PAID`,
journal, payout, transfer, settlement, or Live Mode behavior was added.

## Payload and security boundary

The enqueue DTO has exactly four representable fields: canonical payment
attempt UUID, positive `int64` rupiah amount, `IDR`, and the frozen requested
method enum. Arbitrary keys, nested objects, raw provider payloads,
secret/token/password values, card/CVV data, bank-account data, and customer
contact data cannot be represented by the Go contract. The database independently
enforces the exact four-key JSON shape, UUID/amount/currency/method formats,
aggregate identity, normalized error category, and bounded malformed-response
retry.

Provider result references are never stored raw. `DigestProviderReference`
first accepts only a bounded canonical UUID or normalized prefixed provider ID,
rejecting credential-like prefixes and non-identifier shapes, then returns a
`sha256:<64-lowercase-hex>` digest. The database independently enforces that
exact stored representation. Consequently a raw provider ID, formatted PAN,
bank account, credential, URL, or response fragment cannot be persisted in
this outbox column. Lease ownership is not a free-text field and therefore
cannot be used to store credentials, account data, or provider payload
fragments.

## Database privilege boundary

Docker Compose separates schema authority from runtime authority:

- the one-shot `db-role-init` service idempotently provisions
  `lapangango_app`, including on an existing local database volume;
- the one-shot `migrate` service runs migrations as the schema owner and exits
  before the API starts;
- the API connects only as `lapangango_app`, a non-owner role without
  superuser, replication, database/schema creation, `BYPASSRLS`, or outbox
  `TRUNCATE` authority;
- API startup fails closed if its active database role has any prohibited
  authority, can set or alter `session_replication_role`, starts in any mode
  other than `origin`, belongs to another role, or owns the outbox table;
- every new PostgreSQL pool connection independently rejects a replication mode
  other than `origin`, so later connections cannot inherit an unsafe default.

The passwords in `docker-compose.yml` and `db/init` are local-development
credentials only. A deployed environment must inject distinct managed
credentials and retain the same migrator/runtime privilege separation.

## Verification

- `go test -count=1 ./internal/paymentoutbox` passed;
- `go test -count=1 ./internal/database` passed;
- `go test -count=1 ./internal/payments` passed;
- `go test -count=1 ./...` passed;
- `go vet ./...` passed;
- opt-in disposable PostgreSQL migration tests passed for fresh/upgrade/down,
  `RESTRICT`, exact payload guards including disguised PAN/bank data, refund
  and premature inquiry rejection, canonical attempt/key/hash matching,
  aggregate uniqueness, digest-only provider-reference guards including
  formatted PAN/bank/credential bypass cases, lease-owner guards, legal-only
  lifecycle transitions, primary-key immutability during an otherwise legal
  claim transition, terminal immutability, delete, direct-truncate, and
  cascaded-truncate prevention, replica-mode bypass prevention, lease-token
  constraints, error-state constraints, refused-down dirty-state recovery, and
  migration 025 compatibility;
- opt-in outbox repository tests passed for atomic rollback/commit, replay,
  same-hash/different-payload conflict, error-category separation, bounded
  malformed-response retry independent of prior claims, atomic
  terminalization, expired-lease reclaim with the same owner, positive terminal
  completion and replay rejection, database-clock default/retry eligibility,
  exact microsecond lease-duration boundaries, bounded delayed retry, stale
  token rejection, and repeated concurrent claim safety;
- opt-in runtime-role tests passed for accepting the limited application role
  and rejecting schema-owner/superuser, role-membership escalation, delegated
  `SET`/`ALTER SYSTEM` replica-mode authority, an already-active replica-mode
  session, and truncate authority;
- source scan must continue to show no provider SDK/HTTP call or secret in the
  outbox package.

## Handoff

Task 5B-06 may compose payment-attempt creation, immutable monetization
decision, audit, and `EnqueueTx` in one transaction. It must keep provider
calls asynchronous and outside the database transaction. A worker remains a
later task and must use the provider-neutral adapter from Task 5B-04.
