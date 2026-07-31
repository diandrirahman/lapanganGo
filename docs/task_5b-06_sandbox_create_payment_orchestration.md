# Task 5B-06 — Sandbox Create-payment Orchestration

- Runtime mode: Xendit Test Mode only
- `PLATFORM_MONETIZATION_ENABLED`: `false`
- Provider call: asynchronous worker only; no provider call is made by the
  customer HTTP request
- Status: **READY FOR 5B-07**

## Scope

The customer payment boundary now accepts only `BCA_VA`, `QRIS`, or `CARD`,
copies the amount from the immutable simulation fee snapshot, and atomically
creates a local `payment_attempt`, provider outbox command, and payment audit.
The transaction is owned by the orchestration service; the attempt repository
also remains available through its legacy self-contained wrapper.

Migration `027_payment_create_contracts` stores the exact requested expiry and
normalized success/cancel URLs in a one-to-one immutable row. This keeps the
outbox payload at its frozen four-field shape while giving the future provider
worker a durable source for every hashed create-payment input. The mutable
`payment_attempts.expires_at` remains the normalized local/provider result and
is no longer reused as the original requested expiry during replay.
The down migration refuses while any immutable contract exists. A refused
`golang-migrate` rollback preserves the table and leaves metadata at
`26|dirty`; after verifying the version-27 table and rows remain intact,
recovery is `migrate force 27`.
Migration `028_payment_create_command_contract_guard` makes the database reject
any `PAYMENT_CREATE` outbox command unless the matching immutable contract
already exists with the same request hash. It upgrades the payment-attempt
immutability trigger to `ENABLE ALWAYS`, so replica-role sessions cannot change
server-owned attempt identity or money fields. It also installs always-enabled
guards that prevent a sandbox-backed booking from entering the legacy
confirmation/payment-proof/owner-cash flow and prevent sandbox creation when
legacy payment or owner-income facts already exist. These competing database
writes share the same booking-scoped advisory boundary, including direct SQL
and replica-role sessions. A local cancellation before
provider dispatch changes the attempt from `CREATED` to `CANCELLED`, records
an immutable `payment_create_cancellations` tombstone and payment audit in the
same transaction, and makes the preserved outbox command permanently
ineligible for claim. An always-enabled deferred constraint rejects commit if
that transition has a create command but lacks the matching cancelled booking,
valid tombstone, or exact transition audit; partial direct SQL and replica-role
updates therefore cannot strand a pending command. An always-enabled command
lifecycle guard also rejects
direct and replica-role attempts to lease a command after its tombstone has
committed. "Before provider dispatch" is enforced as command state
`PENDING` with `attempt_count=0`; a leased or retryable command is an uncertain
provider interaction and cannot be cancelled or tombstoned locally.
Migration 028 also reserves `PAYMENT_INQUIRY` with the frozen
`payment:inquiry:{payment_attempt_id}` key and the same allowlisted immutable
attempt payload. Inquiry commands for `PENDING` attempts remain claimable after
booking expiry/cancellation so a late capture cannot be hidden. This is storage
and claim eligibility only: Task 5B-06 adds no inquiry worker and performs no
provider call. Its down migration refuses while payment attempts or provider
commands exist; a refused rollback preserves the guard and leaves
metadata at `27|dirty`, recoverable with `migrate force 28` after verification.
The Phase 5 migration sequence is re-frozen accordingly: webhook inbox moves
to migration 029 and refunds/costs move to migration 030. Those downstream
tasks must use the revised numbers and may not reuse 027 or 028.

Routes:

- `POST /bookings/:id/payment-attempts`
- `GET /payment-attempts/:id`
- `GET /payment-attempts/resolve/:reference`
- frontend `/payments/return/:reference/{success|cancel}`

The POST request requires an `Idempotency-Key` header and a strict JSON body
containing only `requested_method`. Amount, currency, provider identifiers,
checkout URL, and paid authority are server-owned values.
Exactly one canonical `Idempotency-Key` field is accepted; duplicate header
fields and proxy-combined comma values are rejected before orchestration.

## Idempotency and transaction rules

- Client key is scoped to the booking and is converted into a deterministic,
  opaque local reference; the raw key is never persisted or logged.
- Customer and booking UUIDs are parsed and normalized to canonical lowercase
  form before local-reference derivation, request hashing, or locking. The
  database advisory-lock key also casts the booking identifier through `uuid`,
  so equivalent upper/lowercase inputs share one idempotency and lock
  namespace.
- Canonical request hash is SHA-256 over the server-owned create contract.
- The canonical JSON keys are lexicographically ordered and bind amount,
  booking, normalized return URLs, capture/integration mode, currency, expiry,
  local reference, provider/environment, and requested method.
- Attempt expiry is deterministic and never later than the locked
  `bookings.expires_at`; an already-expired booking is rejected.
- Provider command key is frozen as
  `payment:create:{booking_id}:{attempt_no}`.
- Same key and equivalent payload replays the original attempt and command.
- Replay recovery looks up the original opaque local reference before checking
  mutable booking eligibility. It loads the original requested expiry and
  return URLs from `payment_create_contracts`, never from current runtime
  configuration or mutable provider-result fields. The same request therefore
  still replays after the booking expires, becomes terminal, the provider
  normalizes expiry, or `PAYMENT_RETURN_ORIGIN` changes; a different payload
  remains an idempotency conflict.
- Same key with a different method/hash returns an idempotency conflict.
- A second active `CREATED`/`PENDING` attempt for the same booking is rejected.
- A booking-scoped transaction advisory lock serializes sandbox creation and
  local pre-dispatch cancellation. Row locks then follow payment attempt,
  booking, outbox command, and audit order.
- Local pre-dispatch cancellation writes the attempt `CANCELLED` transition,
  immutable tombstone, and audit before changing the booking to `CANCELLED`,
  all in one transaction. The database rejects direct or replica-role booking
  cancellation while a create attempt is still `CREATED`, or while a
  pre-dispatch cancelled attempt lacks its tombstone. A deferred commit
  invariant also rejects the inverse partial write: a `CREATED -> CANCELLED`
  attempt transition cannot commit unless its booking cancellation, valid
  tombstone, and exact state-transition audit commit together.
- The immutable cancellation trigger acquires the canonical booking-flow
  advisory lock and then locks the command/attempt rows before validating
  `PENDING` plus `attempt_count=0`. A concurrent worker lease therefore wins
  cleanly and makes the tombstone fail instead of leaving a cancellation fact
  attached to a dispatched command.
- Capture recognition acquires the same booking-flow advisory boundary before
  locking the attempt. A verified capture from `PENDING` is classified as a
  late capture when the related booking is already `CANCELLED` or locally
  expired, and atomically writes the reconciliation-exception audit without
  reopening fulfillment.
- A failure before commit rolls back all domain, outbox, and audit rows.

## Security and sandbox boundary

- Customer ownership is checked at the database boundary for both create and
  status/reference reads. Unknown and foreign references have the same generic
  not-found response.
- Flag-off requests are rejected with a generic service-unavailable response
  and create a sanitized audit record in a separate transaction.
- The status response exposes only normalized local state, expiry, and a
  validated Xendit Test Mode checkout URL when a future worker has populated
  one while the attempt is `PENDING`, the booking remains `PENDING_PAYMENT`,
  and both booking and attempt remain unexpired according to the database
  clock. Checkout URLs are omitted for locally ineligible bookings, `CREATED`,
  and every terminal attempt state. The accepted hosted-checkout origins are
  `checkout-staging.xendit.co` and `dev.xen.to`; production Xendit checkout
  origins and arbitrary HTTPS hosts are rejected.
- `PAYMENT_RETURN_ORIGIN` is required whenever
  `PAYMENT_CREATE_ENABLED=true`. The backend normalizes that HTTPS origin into
  success and cancellation return paths containing the stable opaque local
  attempt reference before hashing. Startup accepts the same ASCII DNS/IPv4
  hostname and port range (`1..65535`) enforced by migration 027; IPv6,
  malformed ports, and non-ASCII/underscore hostnames fail closed before any
  request is served. No booking ID, customer ID, or provider credential appears
  in those URLs.
- The create endpoint limits the JSON body to 4 KiB before decoding and returns
  a generic HTTP 413 for oversized input; orchestration is never invoked. Its
  token-level strict decoder accepts exactly one `requested_method` field and
  rejects unknown fields, duplicate keys, non-object input, malformed data,
  and trailing JSON.
- The authenticated frontend return route resolves the opaque reference and
  polls normalized local state. The browser `success`/`cancel` path is display
  context only and is never accepted as paid authority. Non-terminal polling
  uses capped exponential backoff and stops at the earlier of the attempt
  expiry or a five-minute verification window, after which the customer is
  directed to the booking detail for authoritative state. If authentication is
  missing or expires while polling, only the canonical internal return path is
  kept temporarily in `sessionStorage`; a successful CUSTOMER login consumes
  it once. External URLs, arbitrary application routes, invalid references,
  unsupported outcomes, and non-customer roles cannot use this redirect.
- The platform audit service is mandatory. Create-payment fails closed before
  any database write when audit recording is unavailable.
- No raw provider response, secret, token, PII, card data, amount override,
  booking `PAID`, journal, payout, settlement, transfer, or production fund
  flow is added.
- Legacy `/pay`, `/payment-proof`, owner verification, mark-paid, and direct
  legacy owner-income writes fail closed once a sandbox payment attempt exists.

## Verification evidence

Verification completed:

```powershell
cd apps/api
go test ./internal/payments
go test ./internal/paymentoutbox
go test ./internal/audit
go test ./cmd/api
go test ./...
go vet ./...

cd ../web
npm test
npm run lint
npm run build
```

The targeted packages, full backend suite, and `go vet ./...` passed. The
opt-in disposable PostgreSQL payment suite also passed with
`TEST_ROLLBACK_HARDENING_DISPOSABLE=1`, including atomic counts, same-key
replay, different-payload conflict, ownership denial, flag-off audit, and
active-attempt protection. Post-review regression evidence additionally proves
that expired bookings are rejected, near-expiry attempts are bounded to the
booking expiry, an overlong expiry is rejected again inside the locked
repository transaction, and an injected late audit failure rolls back the
attempt, provider command, and both audit writes before a clean retry.
The final review regression additionally proves immutable replay after provider
expiry normalization and return-origin drift, authenticated opaque-reference
resolution, concurrent same-key replay, concurrent different-payload conflict,
different-key active-attempt exclusion without deadlock, exactly one
attempt/create-contract/outbox command, atomic audit counts, and frontend
return-page non-authority behavior.
The sandbox/legacy isolation regression additionally proves sequential and
concurrent mutual exclusion, zero owner-income rows, atomic booking/attempt
cancellation, one immutable cancellation tombstone and audit, and that the
preserved command cannot be claimed. Claim and cancellation acquire the same
booking advisory boundary and revalidate after lock acquisition; their
concurrency regression proves that exactly one can win. A state matrix proves
checkout URLs are returned only for `PENDING` attempts.
Additional recovery regressions prove that an inquiry command remains
claimable for a `PENDING` attempt after booking cancellation, a command that
was previously leased cannot receive a pre-dispatch tombstone, an expiry sweep
continues with later candidates after such a conflict, and legacy owner-income
isolation is returned as a booking conflict instead of an internal error.
Post-review regression coverage also proves startup/database return-origin
parity and fail-closed, customer-only recovery of a canonical payment return
route after authentication loss without creating an open redirect.
The final hardening regression additionally rejects malformed DNS labels,
non-canonical/invalid IPv4 addresses, invalid ports, and oversized create
payloads at both application and database boundaries.
Post-review hardening additionally proves canonical UUID replay and shared
create/cancel locking across uppercase/lowercase inputs, rejects direct and
replica-role cancellation that could orphan a create command, preserves the
atomic cancellation path, and rejects authenticated frontend return routes
whose outcome is not exactly `success` or `cancel` without calling the
resolver API.
The latest regression also proves that a `PENDING` capture after booking
cancellation or unswept local expiry is recorded as a late capture with one
reconciliation exception, that a direct tombstone blocks behind the canonical
booking lock and is rejected after a concurrent worker lease, and that
duplicate `requested_method` JSON keys fail before orchestration. Final
hardening additionally proves direct and replica-role lease attempts fail after
a tombstone commits, checkout URLs disappear when the booking or attempt is no
longer payable, and ambiguous `Idempotency-Key` headers fail before
orchestration. The latest three-finding regression proves that attempt
immutability remains active under replica role, partial direct/replica
`CREATED -> CANCELLED` commits are rejected while the complete atomic
cancellation transaction succeeds, and frontend return polling terminates on
attempt expiry or its bounded verification window.

Xendit's Test Mode Payment Session documentation identifies
`https://checkout-staging.xendit.co/sessions/...` and
`https://dev.xen.to/...` as hosted-checkout examples:
<https://docs.xendit.co/apidocs/get-session>.

Docker API/web rebuild and restart succeeded. Migration state is `28|false`;
the immutable create-contract table and always-enabled command guard are
present. `GET /health` and the frontend return-route fallback returned HTTP
200, the unauthenticated resolver returned HTTP 401, and payment/monetization
flags remain disabled.

Task 5B-07 remains responsible for inquiry and timeout recovery. This task
does not infer failure from a provider timeout and does not create a second
external payment.
