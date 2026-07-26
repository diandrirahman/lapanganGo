# Task 5B-02 -- Strict Payment DTO and Money Validation

- Status: **READY FOR 5B-03**
- Date: 2026-07-26
- Runtime guard: `PLATFORM_MONETIZATION_ENABLED=false`
- Scope: pure internal validation only

## Completed boundary

`apps/api/internal/payments` now provides a provider-neutral, dependency-free
DTO validation boundary. It imports no database, HTTP, booking, provider SDK,
or configuration package and cannot make a provider call or mutate a booking.

The only exported error for invalid input is `ErrInvalidPaymentInput` with the
generic message `invalid payment input`. Validators never include a rejected
amount, reference, provider body, token, or other caller-controlled value in
the error.

## Frozen validation rules

| Input | Accepted value |
|---|---|
| Provider / environment | `XENDIT` / `TEST` only |
| Requested method | `BCA_VA`, `QRIS`, `CARD` |
| Integration / capture mode | `PAYMENT_LINK` / `AUTOMATIC` |
| Attempt state | `CREATED`, `PENDING`, `CAPTURED`, `FAILED`, `EXPIRED`, `CANCELLED` |
| Currency | `IDR` only |
| Amount | Positive, canonical ASCII base-10 string that parses to `int64` |
| Local reference | Lowercase opaque ASCII reference, 1--64 bytes, using only letters, digits, `.`, `_`, `:`, `-` |
| Request hash | Exactly 64 lowercase hexadecimal characters |

The parser rejects empty input, zero, leading zeroes, signs, whitespace,
fractions, scientific notation, separators, non-ASCII digits, and values above
`int64` maximum. Input is not trimmed or case-normalized.

`AmountRupiah` in this DTO is not authority for a customer create-attempt API.
Task 5B-03 must read the authoritative amount from
`booking_fee_snapshots.customer_charge_amount_rupiah` and only use this parser
for trusted normalized/provider fact data where that contract applies.

## Explicit exclusions

This task adds no route, handler, service orchestration, repository/query,
database write, provider client, Xendit request, checkout URL, webhook, refund,
outbox, journal, booking state transition, payout, settlement, xenPlatform, or
Live Mode setting.

State values are validated as allowlisted strings only. State transition,
locking, idempotency replay, and capture-once behavior remain the responsibility
of Task 5B-03 and migration 025.

## Verification recorded

- `go test ./internal/payments` passed.
- `go test ./...` from `apps/api` passed.
- Unit tests cover valid values, IDR-only/currency failure, all allowed payment
  methods and states, zero/negative/fraction/scientific/separator/whitespace/
  Unicode/overflow money failures, invalid provider/environment/mode/capture
  method/state/reference/hash, and generic-error behavior.

## Handoff

Task 5B-03 may build the payment repository and guarded state transitions over
migration 025. It must preserve the DTO rules above, keep provider calls out,
and source attempt money from the immutable booking fee snapshot.
