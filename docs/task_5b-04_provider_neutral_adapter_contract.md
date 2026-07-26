# Task 5B-04 -- Provider-neutral Adapter Contract

- Status: **READY FOR 5B-05**
- Date: 2026-07-26
- Runtime mode: Xendit Test Mode only
- Runtime flags: all payment capabilities default `false`
- Scope: contract, normalized DTO/error taxonomy, fake adapter, and fail-closed configuration

## Contract boundary

`apps/api/internal/payments/adapter.go` defines `PaymentAdapter` with six
operations:

- `CreatePayment`;
- `GetPaymentStatus`;
- `VerifyWebhook`;
- `ParseWebhook`;
- `RequestRefund`; and
- `GetRefundStatus`.

The interface accepts and returns LapangGo DTOs only. No service is allowed to
import Xendit SDK types or provider-specific response objects. The adapter
boundary also does not write payment facts, booking status, audit rows, or
financial journals; those mutations remain owned by the repository/service
layers introduced by their own tasks.

## Normalized result and error contract

Payment statuses are limited to `PENDING`, `CAPTURED`, `FAILED`, `EXPIRED`,
and `CANCELLED`. Refund statuses are `PROCESSING`, `SUCCEEDED`, and `FAILED`.

Adapter errors use a fixed code taxonomy:

`RETRYABLE_TIMEOUT`, `RETRYABLE_PROVIDER`, `RATE_LIMITED`,
`AUTHENTICATION_FAILED`, `INVALID_REQUEST`, `IDEMPOTENCY_CONFLICT`,
`REFERENCE_MISMATCH`, `AMOUNT_MISMATCH`, `CURRENCY_MISMATCH`,
`TERMINAL_PROVIDER`, and `MALFORMED_RESPONSE`.

`NormalizeAdapterError` maps context timeout/cancellation and unknown errors to
safe internal categories. Raw provider text, headers, secrets, and payloads are
never copied into the normalized error. Arbitrary error-code values are reduced
to `MALFORMED_RESPONSE`, so provider text cannot bypass the fixed taxonomy.

The normalized webhook event supports both payment and refund facts. It carries
session, payment-request, payment, and refund identifiers plus one normalized
state capable of representing the frozen payment and refund lifecycles. Refund
commands carry both the stable idempotency key and canonical request hash.

## Fake adapter

`NewFakeAdapter` implements every interface operation using scripted callbacks.
It performs no HTTP, SDK, database, webhook, booking, refund, payout, transfer,
settlement, or journal operation. It is intended for service contract tests and
can simulate success, pending, terminal, timeout, rate-limit, and mismatch
results by returning normalized DTOs/errors.

## Safe configuration

`internal/config` now parses the frozen Phase 5 configuration names. Defaults
are fail-closed:

- all payment capability flags are `false`;
- provider is exactly `XENDIT`;
- provider mode is exactly `TEST`; and
- webhook contract version is `DISABLED`.

Validation rejects capability flags without the sandbox master flag, enabled
commands without the backend-only `XENDIT_SECRET_KEY`, webhook ingress without
`XENDIT_WEBHOOK_TOKEN`, an unverified webhook processor, non-Test provider mode,
and non-HTTPS return origins. Secret/token fields are never part of adapter DTOs
or the redacted adapter configuration summary. Refund enablement remains blocked
until the payment-facts/outbox prerequisites exist, and the isolated test ledger
flag is restricted to isolated test construction.

Startup scans the complete process environment and fails closed when a
`VITE_*` provider secret/token/API-key variable is present, including an
explicitly present empty variable. Public frontend configuration that is not a
secret remains allowed. `PLATFORM_MONETIZATION_ENABLED` remains `false` and no
Live Mode/xenPlatform/Money-Out configuration is enabled.

## Verification

- `go test ./internal/payments` passed;
- `go test ./internal/config` passed;
- `go test ./...` passed;
- `go vet ./...` passed;
- fake adapter delegation and normalized error mapping tests passed;
- payment/refund webhook event and refund request-hash tests passed;
- arbitrary adapter error codes are reduced to a safe taxonomy value;
- frontend `VITE_*` provider secret/token startup rejection tests passed;
- safe-config dependency and redaction tests passed; and
- source scan confirmed no Xendit SDK, HTTP provider call, webhook route,
  outbox worker, booking `PAID`, actual journal, payout, transfer, or settlement
  was added by this task.

## Handoff

Task 5B-05 may add the transactional outbox foundation. It must call this
provider-neutral interface through an explicit worker boundary, preserve the
normalized error/idempotency contract, and keep provider calls outside local
database transactions.
