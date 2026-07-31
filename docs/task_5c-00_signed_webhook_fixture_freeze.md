# Task 5C-00 -- Signed Webhook Fixture Freeze

## Scope and frozen contract

- Fixture version: `XENDIT_WEBHOOK_FIXTURES_V1`.
- Scope is synthetic Xendit Test Mode data only. `PLATFORM_MONETIZATION_ENABLED=false`; no ingress or processor flag is enabled by this task.
- Authentication contract: `XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL`, exact `x-callback-token` raw header comparison, constant-time comparison at future ingress, and no body-derived substitute.
- `HMAC`, canonical signed bytes, and signed-timestamp tolerance are `NOT_APPLICABLE`. The raw body is nevertheless bounded and hashed before parsing. Body `created` is semantic data only.
- The manifest records symbolic header presence (`PRESENT_CURRENT`, `PRESENT_WRONG`, or `ABSENT`), never a token-like value. Current-token acceptance is frozen; previous/rotated-token overlap is not a fixture because the frozen contract does not authorize it.

## Fixture location and byte rule

Fixtures and their authoritative expectations are in `apps/api/internal/payments/testdata/xendit_webhooks_v1/manifest.json`. Each manifest row freezes its filename, family, normalized type, synthetic headers, lowercase SHA-256, deterministic key, primary object identity, auth/verification/processing result, safe reason, duplicate behavior, audit, metric, and normalized allowlisted payload where parsing is permitted.

All normal bodies are exact UTF-8 file bytes, including their final newline. `oversized_body.spec` is the deterministic notation `repeat-utf8:a:262145`; it expands to exactly 262145 UTF-8 `a` bytes with no newline. Hashes are always over these raw bytes before parsing. The malformed file is intentionally not JSON and is only hashed.

## Deterministic identity and processing rules

- Payment Session: `XENDIT|<event type>|<payment_session_id>`.
- Payment/capture: `XENDIT|<event type>|<payment_id>` (or payment-request ID only when no payment ID exists).
- Refund: `XENDIT|<event type>|<refund_id>`.
- Invalid input uses a fixed `XENDIT|invalid...|...-v1` key only as a fixture correlation identity; it must not produce an inbox row where the frozen ingress contract says `NO_INBOX`.
- A same key and same hash is a no-op. A same key and different hash is `IDEMPOTENCY_CONFLICT` and quarantined. Old and out-of-order events rely on deduplication and monotonic processing, not a timestamp signature. A body `created` more than five minutes ahead is `FUTURE_CREATED_SEMANTIC` quarantine, not authentication failure.
- `payment_session.completed` remains `PENDING` until an authoritative payment request/payment inquiry validates identity, amount, and IDR currency.

## Matrix coverage

The 29 frozen rows cover valid/missing/wrong/current callback-token states; Payment Session completed/expired and its non-capture invariant; capture succeeded/pending/failed/cancelled/expired; amount, currency, and reference mismatch; refund succeeded/failed; duplicate, conflict, out-of-order, old, and future-created delivery; malformed, unsupported type/version, missing primary identity, oversized body, invalid amount, and redacted-sensitive probes.

Each valid-token Payment Session row remains `DIAGNOSTIC` because runtime delivery proof is deferred. Invalid auth and semantic/contract-invalid inputs are `QUARANTINED`; no fixture asserts a `VERIFIED` provider event before the controlled dashboard proof. The `sensitive-redacted` raw probe contains only `<redacted>` sentinel values and its expected normalized payload omits every sensitive field.

## Redaction, audit, and provider assumptions

Fixtures, manifest headers, normalized payloads, audit expectations, and metrics contain no callback token, authorization value, API key, PAN, CVV, credential, saved-payment token, real customer identity, sensitive checkout URL, raw provider error, or raw webhook storage/log evidence. Metrics use only event-category counters; provider/object IDs are not metric labels.

The Payment Session callback-token header and the synthetic Payment/capture schema remain provisional provider assumptions. The documented refund callback-token surface supports the token form, but this task does not turn any fixture into provider-runtime evidence. After 5C-03, controlled Xendit Dashboard delivery must confirm the exact Payment Session wire contract without storing its token or raw body; any different auth/signing/timestamp/event-identity behavior returns Phase 5C to Phase 5A.
