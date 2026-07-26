# Task 5B-01 -- Payment Attempts Migration

- Status: **READY FOR 5B-02**
- Date: 2026-07-25
- Provider boundary: Xendit Test Mode schema only; no provider call
- Runtime guard: `PLATFORM_MONETIZATION_ENABLED=false`
- Current development migration state: `25|f`

## Scope completed

Migration `025_payment_attempts` adds only the canonical payment-attempt and
immutable capture-fact persistence foundation:

- `payment_attempts` records a server-owned collection attempt for one booking
  and its immutable fee snapshot;
- `payment_capture_facts` records one verified, immutable capture proof for an
  attempt; and
- the paired down migration refuses to remove either table when either contains
  a row.

No endpoint, handler, service, repository, DTO, Xendit adapter, secret,
provider request, checkout redirect, webhook, outbox, refund, cost fact,
booking `PAID` transition, legacy owner-finance transaction, journal,
settlement, payout, xenPlatform, or Live Mode capability was introduced.

## Enforced migration invariants

| Concern | Constraint or guard |
|---|---|
| Fee source | `payment_attempts.booking_id` has `RESTRICT` foreign keys to both `bookings` and `booking_fee_snapshots`; an attempt cannot exist without a snapshot. |
| Money | IDR only and positive `BIGINT` rupiah amount. |
| Provider boundary | `XENDIT` and `TEST` only; allowed methods are BCA VA, QRIS, and card; integration/capture modes are hosted payment link and automatic capture. |
| Attempt identity | Unique `(booking_id, attempt_no)` and unique `(provider, provider_environment, local_reference)`. |
| Provider identity | Non-null session, payment-request, and payment IDs are provider-scoped unique. |
| One capture per booking | Partial unique index permits at most one `CAPTURED` attempt per booking. The rule is not relaxed by a future refund. |
| Capture consistency | Capture state and `captured_at` must appear together; once set, `captured_at` cannot change. |
| Capture fact | One fact per attempt, provider payment identity unique, amount/currency/provider must exactly match the captured attempt. |
| Fact immutability | Any UPDATE or DELETE of a capture fact is rejected by trigger. |
| Rollback | Empty schema may roll back to 024. Any attempt or fact makes rollback fail without dropping data. |

## Verification recorded

1. Runtime preflight passed before implementation: PostgreSQL healthy,
   migration `24|f`, and all required foundation tables present.
2. Disposable PostgreSQL integration tests passed for fresh migration,
   `024 -> 025` upgrade, empty down migration, down refusal with an attempt,
   snapshot FK, IDR/positive-money constraints, duplicate attempt denial,
   capture-once denial, capture matching, and capture-fact immutability.
3. Existing rollback pre-fact test passed after its baseline was advanced from
   migration 024 to 025.
4. `go test ./...` from `apps/api` passed.
5. The local development API was rebuilt and started migration 025
   successfully. PostgreSQL reports `25|f`, both payment tables exist, and
   `http://localhost:8080/health` returned HTTP 200.

## Handoff

Task 5B-02 may now introduce strict, provider-neutral DTO and money parsing
only. It must not create provider calls or mutate booking/payment state. The
schema does not itself activate payment behavior: there are no runtime routes
or adapters in this task, and all Phase 5 feature flags remain off.
