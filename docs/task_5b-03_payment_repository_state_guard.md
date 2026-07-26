# Task 5B-03 -- Payment Repository and State Guard

- Status: **READY FOR 5B-04**
- Date: 2026-07-26
- Runtime guard: `PLATFORM_MONETIZATION_ENABLED=false`
- Provider boundary: no provider call; Xendit Test Mode values only

## Completed scope

`apps/api/internal/payments/repository.go` now provides a PostgreSQL repository
over migration 025 for:

- create/replay of a local payment attempt;
- lookup by attempt ID and booking;
- next attempt number lookup;
- compare-and-set state transitions; and
- one-transaction verified capture plus immutable capture fact.

The repository is provider-neutral at the HTTP/SDK boundary. It uses only the
frozen provider/environment/method values and does not import an Xendit SDK,
call a provider, change booking fulfillment, write a legacy owner cashbook,
write a platform journal, or dispatch a refund/payout/settlement.

## State guard

Normal transitions are limited to:

```text
CREATED  -> PENDING | CANCELLED
PENDING  -> FAILED | EXPIRED | CANCELLED
```

`CAPTURED` is reachable only through `RecordCapture`, which locks the attempt,
validates the exact provider/environment/IDR/amount/reference facts, updates
the attempt, inserts one immutable capture fact, and writes sanitized audit
metadata in the same transaction.

Verified late captures from `FAILED`, `EXPIRED`, or `CANCELLED` are accepted as
`LateCapture=true` and additionally record a `reconciliation_exception` audit
row. They do not reopen booking fulfillment. Direct capture transitions,
terminal-state downgrades, stale compare-and-set updates, and captures from
`CREATED` are denied.

## Idempotency and concurrency

- Creation serializes the deterministic provider/environment/local-reference
  namespace with a transaction-scoped PostgreSQL advisory lock.
- A same reference and same request hash replays the original attempt.
- Replay lookup occurs before the new-attempt expiry guard, so the original
  result remains replayable after its `expires_at` has passed.
- A same reference with a different request hash is
  `ErrIdempotencyConflict`.
- Booking row locking serializes next-attempt allocation.
- Capture locks the canonical attempt with `FOR UPDATE`.
- Repeating the same capture fact is a successful duplicate no-op. A later
  webhook and inquiry observation for the same immutable provider payment fact
  are also duplicate no-ops even though their observation time, authority,
  source reference, and payload hash differ.
- A second captured attempt for the same booking is rejected by the migration
  partial unique index and mapped to `ErrAlreadyCaptured`.
- Timestamp inputs are canonicalized to PostgreSQL microsecond precision before
  comparison, so a retry cannot create a false conflict from nanoseconds.

## Transaction boundary

Create, state transition, and capture operations use database transactions and
defer rollback. Capture commits the state update, immutable fact, and audit
together; any mismatch or write error leaves the attempt and fact unchanged.
Payment audit actions use the central platform-audit validator, metadata
allowlist, sanitizer, and admin filter contract.
Provider network calls are not made while locks are held because no provider
adapter exists in this task.

## Verification recorded

- `go test ./internal/payments` passed.
- Disposable PostgreSQL repository integration tests passed for create/replay,
  replay after expiry, same-key conflict, snapshot absence, CAS stale writer,
  normal capture, exact duplicate capture, inquiry/webhook cross-authority
  duplicate capture, late capture, amount mismatch rollback, second booking
  capture rejection, and concurrent create/capture.
- Platform payment-audit validation, sanitization, and admin action/entity
  filtering tests passed.
- State transition table tests passed.
- `go test ./...` from `apps/api` passed.
- `go vet ./internal/payments` passed.
- Boundary scan found no Xendit SDK, HTTP provider call, booking `PAID`, legacy
  owner-finance, or platform-journal implementation in the payment package.

## Handoff

Task 5B-04 may define the provider-neutral adapter interface and normalized
errors. It must continue to use this repository as the only state/fact mutation
boundary and must not bypass its row locks, compare-and-set guards, immutable
capture fact, or idempotency rules.
