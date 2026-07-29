# Task 5A-05 -- Payment Technical Contract Freeze

- Status: **FROZEN FOR SANDBOX/SHADOW IMPLEMENTATION**
- Human verdict: **GO FOR SANDBOX/SHADOW ONLY**
- Verdict authority: project-owner direction in the 2026-07-24 working session
- Date: 2026-07-24
- Provider: Xendit Test Mode
- Currency: IDR, exponent 0
- Runtime guard: `PLATFORM_MONETIZATION_ENABLED=false`
- Live verdict: **NO-GO**
- Related documents:
  `task_5a-01_provider_capability_evidence.md`,
  `task_5a-02_xendit_fund_flow_accounting_adr.md`,
  `task_5a-03_payment_refund_state_machine.md`, and
  `task_5a-04_security_privacy_operational_gate.md`

## 1. Purpose and implementation boundary

This document converts the Phase 5A evidence and ADRs into one technical
contract for Phase 5B through Phase 5E. Later tasks may implement this contract
in their assigned slices, but may not silently change states, money rules,
provider authority, schema identities, idempotency boundaries, security
controls, or the sandbox-only fund-flow.

Task 5A-05 creates no source code, migration, endpoint, credential, Xendit
request, webhook URL, refund, journal, payout, settlement, or production
financial fact.

The project owner has authorized only an internal Xendit Test Mode demo.
Finance, Legal, Security, Product, and provider marketplace approvals remain
deferred for Live. This human verdict is not authority to use real funds or
production credentials.

## 2. Frozen invariants

1. `PLATFORM_MONETIZATION_ENABLED=false` throughout Phase 5.
2. Xendit Test Mode and virtual funds only; provider mode `LIVE` is rejected at
   startup.
3. xenPlatform, `for-user-id`, `with-split-rule`, sub-accounts, split,
   transfer, payout, settlement execution, and Money-Out remain prohibited.
4. IDR integer rupiah only. API decimal, fraction, scientific notation,
   separator, negative, zero, and overflow input is rejected.
5. A payment attempt is tied to exactly one immutable
   `booking_fee_snapshots` row. Its amount comes from
   `customer_charge_amount_rupiah`, not from the browser.
6. At most one attempt for a booking may ever become `CAPTURED`, including
   after refund.
7. A browser return URL, dashboard screenshot, operator statement, or request
   timeout cannot mark payment or booking paid.
8. Provider timeout remains `PENDING`; retries reuse the same deterministic
   key and original attempt.
9. Payment, booking, refund, settlement, payout, and service-completion states
   remain separate.
10. Phase 5 permits one full refund equal to the captured principal. Partial or
    second refund is denied.
11. Refund approval creates `REQUESTED`; provider acceptance creates at most
    `PROCESSING`; only verified webhook/inquiry may create `SUCCEEDED`.
12. No gateway path inserts legacy owner income/refund cashbook facts or
    production platform journals, payable, revenue, settlement, or payout.
13. All accepted mutations are atomic with a sanitized audit record and use a
    deterministic idempotency key.
14. Raw webhook bodies, API secrets, callback tokens, authorization headers,
    PAN/CVV, bank credentials, saved payment tokens, and unnecessary PII are
    never persisted or logged.
15. Immutable provider/capture/cost facts are corrected only through explicit
    reversal or exception facts; never UPDATE/DELETE or direct SQL repair.

## 3. Repository and module boundaries

The later implementation must preserve these provider-neutral boundaries:

| Module responsibility | Planned package boundary | May import Xendit DTO/HTTP details? |
|---|---|---:|
| Payment states, money validation, orchestration | `internal/payments` | No |
| Payment repository and row locking | `internal/payments` repository | No |
| Provider-neutral interface and normalized errors | `internal/paymentprovider` | No |
| Xendit Test Mode HTTP adapter | `internal/providers/xendit` | Yes, internally only |
| Durable command outbox/worker | `internal/paymentoutbox` | Worker may call provider-neutral interface only |
| Webhook verification/inbox/processor | `internal/paymentwebhooks` | Verifier/parser implementation may be provider-specific behind interface |
| Normalized refund flow | `internal/paymentrefunds` | No |
| Provider cost facts | `internal/paymentcosts` | No |
| Read-only shadow reconciliation | `internal/paymentreconciliation` | No provider write capability |
| Isolated test journal templates | `internal/paymenttestledger` | No runtime/UI construction path |

Business services must not import a provider SDK model or expose a Xendit
payload to the frontend. Provider error bodies are converted to safe normalized
codes before leaving the adapter.

## 4. Migration order and ownership

The current repository baseline is migration `024`. These numbers are reserved
by this contract, subject to Task 5B-00 proving that they remain unused:

| Reserved migration | Owning task | Schema delta | Down-migration rule |
|---:|---|---|---|
| `025_payment_attempts` | 5B-01 | `payment_attempts`, `payment_capture_facts`, constraints, indexes, immutable capture guard | Refuse down when either table contains facts |
| `026_payment_provider_outbox` | 5B-05 | `payment_provider_commands`, lease/retry indexes and payload guards | Refuse down when command rows exist |
| `027_payment_webhook_inbox` | 5C-01 | `payment_webhook_events`, processing indexes, optional capture source FK | Refuse down when inbox rows or dependent facts exist |
| `028_payment_refunds_and_costs` | 5D-01 | `payment_refunds`, `payment_cost_items`, webhook/refund links and isolated journal source FKs | Refuse down when refund/cost/source-linked journal rows exist |

Every migration requires matching `*.up.sql` and `*.down.sql`. All financial
references use `ON DELETE RESTRICT`; no new payment table may use CASCADE.
Migration tests must cover fresh, `024 -> target`, and reverse order on an
empty disposable database.

Task 5B-00 must stop if:

- migration 024 is not current and clean;
- any reserved number/file/table already exists unexpectedly;
- the working tree contains overlapping migration/payment work;
- `booking_fee_snapshots`, `platform_audit_logs`, or required booking/owner
  references are missing;
- a payment/provider secret is found in source, frontend, fixtures, or Git.

## 5. Frozen schema delta

The following is a logical schema contract. The owning implementation task may
choose constraint/index names consistent with repository conventions, but not
change the fields, identities, relationships, or invariants without returning
to Phase 5A.

### 5.1 `payment_attempts`

Canonical mutable state for one collection attempt:

| Column | Contract |
|---|---|
| `id` | UUID primary key |
| `booking_id` | UUID; `RESTRICT` FK to both `bookings(id)` and `booking_fee_snapshots(booking_id)` |
| `attempt_no` | Positive SMALLINT |
| `provider` | `XENDIT` only in Phase 5 |
| `provider_environment` | `TEST` only |
| `requested_method` | `BCA_VA`, `QRIS`, or `CARD` |
| `integration_mode` | `PAYMENT_LINK` |
| `capture_method` | `AUTOMATIC` |
| `state` | `CREATED`, `PENDING`, `CAPTURED`, `FAILED`, `EXPIRED`, `CANCELLED` |
| `currency` | `IDR` |
| `amount_rupiah` | Positive BIGINT copied by server from immutable snapshot |
| `local_reference` | Unique opaque reference, maximum 64 bytes, no PII |
| `request_hash` | Lowercase SHA-256 hex of canonical create command |
| `provider_session_id` | Nullable, provider-scoped unique |
| `provider_payment_request_id` | Nullable, provider-scoped unique |
| `provider_payment_id` | Nullable, provider-scoped unique |
| `provider_status_code` | Nullable normalized allowlisted code, not raw error text |
| `checkout_url` | Nullable HTTPS Xendit hosted-checkout URL; redacted from logs |
| `expires_at` | Provider/local expiry instant |
| `captured_at` | Nullable; set once on verified capture and thereafter immutable |
| `created_at`, `updated_at` | Server timestamps |

Required constraints/indexes:

- unique `(booking_id, attempt_no)`;
- unique `(provider, provider_environment, local_reference)`;
- partial unique indexes for each non-null provider session/request/payment ID;
- partial unique `(booking_id) WHERE state='CAPTURED'`;
- `state='CAPTURED'` iff `captured_at IS NOT NULL`;
- `expires_at > created_at`;
- immutable `booking_id`, attempt number, provider/environment, method, mode,
  currency, amount, local reference, request hash, and non-null `captured_at`;
- lookup indexes for `(booking_id, created_at DESC)`,
  `(state, updated_at)`, and provider identifiers.

The repository creates this row using `INSERT ... SELECT` from
`booking_fee_snapshots`. It never accepts the amount from a customer request.
The snapshot must have `finance_mode='SIMULATION'` and
`booking_channel='MARKETPLACE_ONLINE'`.

### 5.2 `payment_capture_facts`

Append-only proof of one verified capture:

| Column | Contract |
|---|---|
| `id` | UUID primary key |
| `payment_attempt_id` | Unique `RESTRICT` FK to `payment_attempts` |
| `provider` / `provider_environment` | `XENDIT` / `TEST` |
| `provider_payment_id` | Provider-scoped unique, required |
| `provider_payment_request_id` | Required when provider supplies it |
| `amount_rupiah` / `currency` | Positive BIGINT / `IDR`; exact attempt amount |
| `captured_at` | Provider-confirmed effective time, immutable |
| `observed_at` | Server observation time |
| `authority` | `VERIFIED_WEBHOOK` or `AUTHENTICATED_INQUIRY` |
| `source_reference` | Deterministic sanitized provider/inquiry reference |
| `payload_hash` | Lowercase SHA-256 hex |
| `source_webhook_event_id` | Nullable `RESTRICT` FK added by migration 027 |
| `created_at` | Insert timestamp |

All UPDATE and DELETE operations are rejected. A successful refund never
removes or weakens the capture fact.

### 5.3 `payment_provider_commands`

Durable outbox for provider calls:

| Column | Contract |
|---|---|
| `id` | UUID primary key |
| `command_type` | `PAYMENT_CREATE`, `PAYMENT_INQUIRY`, `REFUND_CREATE`, `REFUND_INQUIRY` |
| `aggregate_type` / `aggregate_id` | `PAYMENT_ATTEMPT` or `PAYMENT_REFUND` plus UUID |
| `idempotency_key` | Deterministic, unique with command type |
| `request_hash` | Lowercase SHA-256 canonical-command hash |
| `redacted_payload` | Allowlisted JSON only; no raw provider payload/secret/PII |
| `state` | `PENDING`, `LEASED`, `RETRYABLE`, `SUCCEEDED`, `TERMINAL` |
| `attempt_count` | Non-negative integer |
| `available_at` | Next eligible attempt time |
| `lease_owner`, `lease_token`, `lease_expires_at` | Nullable bounded worker lease plus opaque per-claim generation token |
| `last_error_code` | Nullable normalized safe code |
| `provider_reference` | Nullable `sha256:<64-lowercase-hex>` digest; raw provider result references are forbidden |
| `created_at`, `updated_at`, `completed_at` | Worker lifecycle timestamps |

Enqueue must be in the same database transaction as the domain state and
audit. Provider HTTP calls always occur outside that transaction. Claiming
uses `FOR UPDATE SKIP LOCKED` or an equivalent safe lease. A crashed/expired
lease is retryable with the same key and request hash.
Retry scheduling passes a bounded relative delay to the repository; PostgreSQL
derives `available_at` from `transaction_timestamp()` so worker/API clock skew
cannot change the intended backoff.
The enqueue boundary reads the referenced immutable payment attempt in the
same transaction, derives the command key, and persists only amount, currency,
method, and request hash that match that attempt. One command of each type is
allowed per aggregate.
Every claim rotates `lease_token`; lease completion requires the exact current
owner and token so a stale execution cannot complete a command reclaimed after
expiry or worker restart.

### 5.4 `payment_webhook_events`

Durable redacted webhook inbox:

| Column | Contract |
|---|---|
| `id` | UUID primary key |
| `provider` / `provider_environment` | `XENDIT` / `TEST` |
| `event_type` | Normalized allowlisted event name |
| `provider_event_key` | Deterministic provider/object/event key; never random |
| `provider_event_id` | Nullable only when Xendit actually supplies one |
| `primary_object_id` | Session, payment, request, or refund ID |
| `raw_body_hash` | Lowercase SHA-256 of bounded exact request body |
| `auth_contract_version` | `XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL` or `XENDIT_CALLBACK_TOKEN_V1_VERIFIED` |
| `verification_state` | `DIAGNOSTIC`, `VERIFIED`, or `QUARANTINED` |
| `processing_state` | `RECEIVED`, `PROCESSING`, `PROCESSED`, `RETRYABLE`, `TERMINAL`, `DUPLICATE` |
| `redacted_payload` | Normalized allowlisted JSON; never raw body |
| `payment_attempt_id` | Nullable `RESTRICT` FK |
| `payment_refund_id` | Nullable `RESTRICT` FK added by migration 028 |
| `correlation_id` | Opaque server correlation value |
| `received_at`, `processed_at`, `created_at`, `updated_at` | Lifecycle timestamps |

Required uniqueness and immutability:

- unique `(provider, provider_environment, provider_event_key)`;
- event identity, raw hash, provider/object IDs, received time, and redacted
  payload are immutable after insertion;
- only verification/processing lifecycle fields may change through guarded
  repository methods;
- no raw-payload column;
- invalid authentication creates only a sanitized audit/metric, not a
  business inbox row.

Provisional deterministic event keys:

| Event family | Key material |
|---|---|
| Payment Session | provider + event type + `payment_session_id` |
| Payment/capture | provider + normalized event + `payment_id` or `payment_request_id` |
| Refund | provider + normalized event + `refund_id` |

A repeat with the same key/hash is a success no-op. Same key with a different
hash is quarantined and cannot mutate state.

### 5.5 `payment_refunds`

One normalized full-refund lifecycle:

| Column | Contract |
|---|---|
| `id` | UUID primary key |
| `payment_attempt_id` | Unique `RESTRICT` FK |
| `payment_capture_fact_id` | Unique `RESTRICT` FK |
| `booking_refund_request_id` | Nullable unique `RESTRICT` FK to customer-request path |
| `legacy_owner_refund_transaction_id` | Nullable unique `RESTRICT` FK to legacy owner-finance path |
| `source_type` | `CUSTOMER_REQUEST`, `OWNER_CANCEL_REFUND`, or `LATE_CAPTURE_INTENT` |
| `state` | `REQUESTED`, `PROCESSING`, `SUCCEEDED`, `FAILED` |
| `amount_rupiah` / `currency` | Exact captured principal / `IDR` |
| `idempotency_key` / `request_hash` | Deterministic full-refund identity and hash |
| `provider_refund_id` | Nullable provider-scoped unique |
| `provider_status_code` | Nullable normalized safe code |
| `requested_at`, `processing_at`, `succeeded_at`, `failed_at` | State timestamps |
| `created_at`, `updated_at` | Server timestamps |

There is one row per captured attempt and one successful total equal to the
capture. `APPROVED` in either legacy flow maps only to refund `REQUESTED`;
legacy response wording must become/retain “processing”, never “refunded”.
The gateway bridge must not insert another legacy owner refund ledger row.

### 5.6 `payment_cost_items`

Append-only provider-confirmed costs:

| Column | Contract |
|---|---|
| `id` | UUID primary key |
| `payment_attempt_id` | `RESTRICT` FK |
| `payment_refund_id` | Nullable `RESTRICT` FK |
| `provider` / `provider_environment` | `XENDIT` / `TEST` |
| `cost_type` | `PROCESSING_FEE`, `REFUND_FEE`, `PROVIDER_TAX`, `ADJUSTMENT` |
| `effect` | `CHARGE` or `REVERSAL` |
| `amount_rupiah` / `currency` | Positive BIGINT / `IDR` |
| `provider_cost_reference` | Deterministic provider-scoped unique reference |
| `reverses_cost_item_id` | Nullable unique `RESTRICT` self-FK |
| `effective_at`, `observed_at`, `created_at` | Provider/server timestamps |
| `payload_hash` | Lowercase SHA-256 hex |

Signed amounts are forbidden. Direction is expressed only by `effect`.
Estimates/public MDR values cannot enter this table. A reversal must reference
one original charge and match its type and exact amount.

### 5.7 Isolated journal source references

Migration 028 may add nullable `RESTRICT` FKs to `platform_journals`:

- `payment_capture_fact_id`;
- `payment_refund_id`;
- `payment_cost_item_id`;
- `payment_template_mode`, whose only Phase 5 value is `ISOLATED_TEST`.

Partial unique indexes prevent more than one non-reversal template journal per
source. Existing journals remain valid with all new columns NULL. Future
payment template event types require exactly one matching source:

- `PAYMENT_CAPTURE_TEST`;
- `PAYMENT_PROVIDER_COST_TEST`;
- `PAYMENT_COMPLETION_TEST`;
- `PAYMENT_REFUND_TEST`;
- `PAYMENT_DISPUTE_TEST`.

No runtime service may construct the isolated test-ledger capability. It is
available only to the explicit test package/CLI introduced by Phase 5E.

## 6. API and endpoint contract

### 6.1 Customer payment API

| Method/path | Contract |
|---|---|
| `POST /bookings/:id/payment-attempts` | Authenticated booking customer requests one allowed method. No amount/provider IDs accepted. Returns `202` with local attempt ID/state. |
| `GET /payment-attempts/:id` | Booking customer reads normalized state, expiry, and checkout URL when available. No raw provider response. |

Create-payment is asynchronous: the first request atomically creates the local
attempt, outbox command, and audit. The frontend polls the GET endpoint until a
Test Mode checkout URL is ready, then redirects. A return from checkout causes
the frontend to poll again; it never submits “paid” authority.

### 6.2 Future webhook routes

| Route | Event family |
|---|---|
| `POST /webhooks/xendit/payment-session` | `payment_session.completed`, `payment_session.expired` |
| `POST /webhooks/xendit/payment` | Selected payment/capture events |
| `POST /webhooks/xendit/refund` | `refund.succeeded`, `refund.failed` |

Payment Session Completed and Expired use the same first route. These routes
have the dedicated limits and generic response matrix from 5A-04 and are not
protected by customer JWT middleware.

The Payment Session route initially runs diagnostic-only with
`PAYMENT_WEBHOOK_PROCESSOR_ENABLED=false`. Dashboard `Test and Save` is allowed
only after the hardened ingress exists. A successful controlled test must prove
callback-token presence/match without recording its value before the processor
can be enabled.

### 6.3 Return URLs

Success/cancel URLs are constructed from a backend HTTPS allowlist and opaque
attempt reference. They contain no amount, booking ID, customer data, provider
credential, or payment token. Return routes display/poll local state only.

## 7. Xendit Test Mode adapter contract

The selected provider surface is:

| Operation | Xendit surface |
|---|---|
| Create hosted checkout | `POST /sessions`, `session_type=PAY`, `mode=PAYMENT_LINK`, `capture_method=AUTOMATIC`, `allow_save_payment_method=DISABLED`, `country=ID`, `currency=IDR` |
| Session inquiry | `GET /sessions/{session_id}` |
| Payment inquiry | `GET /v3/payment_requests/{payment_request_id}`, `api-version: 2024-11-11` |
| Full refund | `POST /refunds` using original payment/request identity and exact captured amount |
| Payment/refund observation | Selected documented webhooks plus authenticated inquiry |

The adapter must omit `for-user-id`, `with-split-rule`, customer PII, items,
metadata, and saved-payment settings. If Xendit requires a customer field or a
different contract for a selected channel, implementation stops and returns to
5A-04/5A-05.

The secret Test API key is used as server-side Basic Auth according to Xendit
API requirements. It is never returned, logged, stored in a command payload,
or exposed as `VITE_*`. The public key is not required for hosted checkout.

### 7.1 Provider-neutral adapter interface

The frozen operations are:

- `CreatePayment`;
- `GetPaymentStatus`;
- `VerifyWebhook`;
- `ParseWebhook`;
- `RequestRefund`;
- `GetRefundStatus`.

Normalized create request:

| Field | Contract |
|---|---|
| `AttemptID`, `LocalReference` | Opaque local IDs |
| `Method` | `BCA_VA`, `QRIS`, `CARD` |
| `AmountRupiah`, `Currency` | Checked int64 / `IDR` |
| `Mode`, `CaptureMethod` | `PAYMENT_LINK` / `AUTOMATIC` |
| `ExpiresAt` | Bounded expiry |
| `SuccessReturnURL`, `CancelReturnURL` | Backend allowlisted HTTPS |
| `IdempotencyKey`, `RequestHash` | Deterministic |

Normalized create result contains only provider session/request/payment IDs,
normalized state, checkout URL, expiry, provider correlation ID, and safe
status code.

Normalized payment/refund event contains only:

- deterministic event key and optional provider event ID;
- event type and primary object ID;
- provider session/request/payment/refund IDs;
- normalized state;
- checked amount/currency;
- provider effective time and server observed time;
- raw-body SHA-256;
- safe reason code.

### 7.2 Status mapping

| Provider fact | Local result |
|---|---|
| Session `ACTIVE` | `PENDING` |
| Session `COMPLETED` | Remain `PENDING` until payment ID/request inquiry proves capture |
| Session `EXPIRED` | `EXPIRED` only if no capture exists |
| Session `CANCELED` | `CANCELLED` only if no capture exists |
| Payment Request `ACCEPTING_PAYMENTS` / `REQUIRES_ACTION` | `PENDING` |
| Payment Request `SUCCEEDED` | `CAPTURED` after identity/amount/currency validation |
| Payment Request `FAILED` | `FAILED` unless capture already exists |
| Payment Request `EXPIRED` | `EXPIRED` unless capture already exists |
| Payment Request `CANCELED` | `CANCELLED` unless capture already exists |
| Refund `PENDING` | `PROCESSING` |
| Refund `SUCCEEDED` | `SUCCEEDED` after exact full-amount validation |
| Refund `FAILED` or provider `CANCELLED` | Local `FAILED` with normalized reason |

### 7.3 Normalized error taxonomy

| Class | Meaning/behavior |
|---|---|
| `RETRYABLE_TIMEOUT` | Outcome uncertain; retain state and same key |
| `RETRYABLE_PROVIDER` | Bounded retry with backoff/jitter |
| `RATE_LIMITED` | Honor bounded retry-after |
| `AUTHENTICATION_FAILED` | Terminal/config incident; disable commands |
| `INVALID_REQUEST` | Terminal local/provider contract error |
| `IDEMPOTENCY_CONFLICT` | Same key/different hash; quarantine |
| `REFERENCE_MISMATCH` | Provider IDs/reference/merchant do not match |
| `AMOUNT_MISMATCH` / `CURRENCY_MISMATCH` | Security/reconciliation exception |
| `TERMINAL_PROVIDER` | Verified terminal failure |
| `MALFORMED_RESPONSE` | Retryable once/bounded, then terminal incident |

Raw provider error text/body is never propagated to users or logs.

## 8. Canonical commands and events

### 8.1 Durable provider commands

| Command type | Aggregate | Required effect |
|---|---|---|
| `PAYMENT_CREATE` | payment attempt | Create/recover the original Test Mode Session |
| `PAYMENT_INQUIRY` | payment attempt | Resolve original uncertain attempt |
| `REFUND_CREATE` | payment refund | Request the one exact full refund |
| `REFUND_INQUIRY` | payment refund | Resolve original uncertain refund |

### 8.2 Normalized domain events/audit actions

Canonical domain event names:

- `PAYMENT_ATTEMPT_CREATED`;
- `PAYMENT_COMMAND_ENQUEUED`;
- `PAYMENT_CHECKOUT_READY`;
- `PAYMENT_PENDING`;
- `PAYMENT_CAPTURED`;
- `PAYMENT_FAILED`;
- `PAYMENT_EXPIRED`;
- `PAYMENT_CANCELLED`;
- `PAYMENT_LATE_CAPTURE_DETECTED`;
- `REFUND_REQUESTED`;
- `REFUND_PROCESSING`;
- `REFUND_SUCCEEDED`;
- `REFUND_FAILED`;
- `PROVIDER_COST_RECORDED`;
- `PROVIDER_COST_REVERSED`.

The audit action allowlist from 5A-04 remains authoritative. Every audit uses a
correlation ID and sanitized metadata. IDs may appear only when needed for
server-side correlation; no secret, raw payload, checkout URL, or PII is
allowed. Provider/worker actions use `actor_user_id=NULL` and
`actor_role=SYSTEM`; customer/owner actions retain the authenticated actor and
authorized role.

## 9. Idempotency and canonical hashing

Frozen namespaces:

| Operation | Key |
|---|---|
| Create payment | `payment:create:{booking_id}:{attempt_no}` |
| Provider inquiry | `payment:inquiry:{payment_attempt_id}` |
| Capture recognition | `payment:capture:{provider}:{provider_payment_id}` |
| Payment webhook inbox | `webhook:{provider}:{provider_event_key}` |
| Full refund | `refund:create:{payment_attempt_id}:full:v1` |
| Refund inquiry | `refund:inquiry:{payment_refund_id}` |
| Refund webhook inbox | `refund-webhook:{provider}:{provider_event_key}` |
| Booking projection | `booking:payment-capture:{payment_attempt_id}` |
| Cost fact | `payment-cost:{provider}:{provider_cost_reference}:{effect}` |
| Test journal | `payment-test-journal:{event_type}:{source_id}` |

Keys use lowercase ASCII, UUID canonical lowercase, no whitespace, PII, token,
URL, or amount. Maximum serialized key length is 191 bytes.

Canonical request hashing is `SHA-256` over UTF-8 canonical JSON with:

- lexicographically sorted object keys;
- no insignificant whitespace;
- integer rupiah serialized as a base-10 string;
- UTC timestamps serialized in RFC3339Nano;
- absent optional fields omitted, not `null`;
- enum values in their frozen uppercase form;
- URL origin/path normalized before hashing;
- hash encoded as 64 lowercase hexadecimal characters.

Same key and same hash replays the original result. Same key with a different
hash is an `IDEMPOTENCY_CONFLICT`; no provider call or state change occurs.

## 10. Feature flags and startup validation

All new flags default to `false`:

| Flag | Capability |
|---|---|
| `PAYMENT_SANDBOX_ENABLED` | Master Xendit Test Mode capability |
| `PAYMENT_CREATE_ENABLED` | Customer create-attempt orchestration |
| `PAYMENT_INQUIRY_ENABLED` | Status inquiry/recovery |
| `PAYMENT_WEBHOOK_INGRESS_ENABLED` | Hardened webhook HTTP ingress |
| `PAYMENT_WEBHOOK_PROCESSOR_ENABLED` | Webhook-driven state mutation |
| `PAYMENT_REFUND_ENABLED` | Normalized full-refund workflow |
| `PAYMENT_SHADOW_RECONCILIATION_ENABLED` | Read-only provider/local comparison |
| `PAYMENT_ISOLATED_TEST_LEDGER_ENABLED` | Test-only journal-template capability |

Non-boolean configuration:

- `PAYMENT_PROVIDER=XENDIT`;
- `PAYMENT_PROVIDER_MODE=TEST`;
- `PAYMENT_WEBHOOK_CONTRACT_VERSION` is `DISABLED`,
  `XENDIT_CALLBACK_TOKEN_V1_PROVISIONAL`, or
  `XENDIT_CALLBACK_TOKEN_V1_VERIFIED`;
- backend-only `XENDIT_SECRET_KEY`;
- backend-only `XENDIT_WEBHOOK_TOKEN`;
- allowlisted HTTPS payment return origin.

Startup fails closed when:

- `PLATFORM_MONETIZATION_ENABLED` is true;
- any payment flag is true while `PAYMENT_SANDBOX_ENABLED` is false;
- provider/mode is not exactly `XENDIT`/`TEST`;
- create/inquiry/refund is enabled without the required backend secret;
- webhook ingress is enabled without the callback token;
- webhook processor is enabled unless ingress is enabled and contract version
  is `XENDIT_CALLBACK_TOKEN_V1_VERIFIED`;
- refund is enabled before payment facts/outbox prerequisites;
- isolated test ledger is requested through normal HTTP/runtime construction;
- any `VITE_*` provider secret/token variable is present.

The safe activation order is:

1. schema/repository with all flags off;
2. sandbox + create orchestration;
3. inquiry recovery;
4. webhook ingress in provisional diagnostic mode;
5. controlled Dashboard delivery proof;
6. verified contract marker and webhook processor;
7. refund workflow;
8. read-only reconciliation;
9. isolated test-ledger tests only.

There is no UI or admin API for these flags.

## 11. Metrics, alerts, and log contract

Prometheus-style metric names are frozen; implementations may add only
low-cardinality labels such as provider, method, command/event type, result,
and normalized reason:

- `lapanggo_payment_attempts_total`;
- `lapanggo_payment_attempt_state_total`;
- `lapanggo_payment_pending_age_seconds`;
- `lapanggo_payment_provider_commands_total`;
- `lapanggo_payment_provider_command_duration_seconds`;
- `lapanggo_payment_provider_command_retries_total`;
- `lapanggo_payment_webhooks_received_total`;
- `lapanggo_payment_webhook_verification_total`;
- `lapanggo_payment_webhook_duplicates_total`;
- `lapanggo_payment_webhook_conflicts_total`;
- `lapanggo_payment_webhook_processing_total`;
- `lapanggo_payment_late_captures_total`;
- `lapanggo_payment_refunds_total`;
- `lapanggo_payment_refund_pending_age_seconds`;
- `lapanggo_payment_provider_cost_items_total`;
- `lapanggo_payment_reconciliation_difference_rupiah`;
- `lapanggo_payment_kill_switch_rejections_total`.

Forbidden metric/log labels include booking/customer/owner/venue IDs, email,
phone, checkout URL, provider object ID, token, raw reason, and payload.
Detailed investigation uses protected audit correlation, not metric labels.

Minimum alerts:

- webhook authentication failures or conflicts above baseline;
- provider authentication failure;
- stale `PENDING` payment/refund;
- outbox retry exhaustion or lease backlog;
- late capture;
- amount/currency/reference mismatch;
- non-zero unexplained reconciliation;
- attempted Live/xenPlatform/Money-Out configuration;
- secret/PII persistence scan failure.

## 12. Fixture freeze

All fixtures are synthetic, versioned, redacted, and stored under the owning
package `testdata` directory. They use impossible/non-customer identities and a
fixed fake token that is never accepted outside tests.

Required fixture groups:

### Payment create/status

- BCA VA, QRIS, and card request;
- valid IDR max-boundary integer;
- zero, negative, fraction, scientific, separator, and int64 overflow;
- provider timeout then pending/success/failure inquiry;
- same-key replay and different-hash conflict;
- mismatched booking, amount, currency, merchant, or provider ID;
- Session completed without verified payment remains pending;
- late capture after expiry/cancellation.

### Webhook

- provisional callback-token valid, missing, wrong, and rotated-token cases;
- completed, expired, captured, failed, refund succeeded, refund failed;
- duplicate and out-of-order delivery;
- same deterministic event key with conflicting hash;
- oversized body, malformed JSON, unsupported type/schema;
- DB failure before durability and processor failure after durability;
- sensitive-data persistence scan;
- controlled Xendit `Test and Save` evidence must later replace/confirm the
  provisional Payment Session header assumptions without storing the token or
  raw body.

### Refund/cost

- approval remains `REQUESTED`;
- one exact full refund before completion;
- timeout/retry with the same key;
- duplicate/out-of-order success/failure;
- partial and second refund rejection;
- capture/refund concurrency;
- provider fee/tax charge and exact reversal;
- estimate rejected from actual cost facts.

### Shadow accounting/reconciliation

- commission 0%, 5%, and 7%;
- promo/price-adjustment snapshot;
- rounding and maximum int64-safe values;
- capture, completion, full refund, provider fee/tax, and reversal;
- manual-direct/offline source isolation;
- matching, missing-local, missing-provider, timing-pending, and unexplained
  differences;
- every isolated journal balances exactly and omits zero lines.

Task 5C-00 freezes the provisional callback-token fixtures and explicitly
records HMAC/canonical signed bytes/signed timestamp tolerance as
`NOT_APPLICABLE` unless new provider evidence says otherwise. Body `created`
time more than five minutes in the future is a semantic quarantine case, not
signature verification; old events rely on deterministic deduplication and
monotonic state.

After 5C-03 creates the hardened diagnostic ingress, controlled Dashboard
delivery is a second hard stop before 5C-05 or
`PAYMENT_WEBHOOK_PROCESSOR_ENABLED`. If the delivery differs from the
provisional fixture, Phase 5C stops and returns to Phase 5A.

## 13. Transaction and lock order

All state-changing flows use this lock order:

1. authenticate/validate request or webhook outside the business transaction;
2. insert/deduplicate idempotency/inbox identity;
3. lock `payment_attempts`;
4. lock related `bookings`;
5. lock related `payment_refunds`, when applicable;
6. insert immutable fact, state transition, outbox command, and audit
   atomically;
7. commit;
8. perform provider call only through the worker outside the transaction.

No flow may acquire these locks in another order. Provider HTTP calls and
notification delivery are never performed while database locks are held.

## 14. Rollback, kill switch, and recovery plan

### 14.1 Operational rollback

1. Set create, processor, refund, reconciliation, and test-ledger flags false.
2. Keep webhook ingress only if needed to durably quarantine/observe Test Mode
   events; otherwise disable it too.
3. Stop workers after releasing/expiring leases; never delete commands.
4. Revoke/rotate a suspected provider key/token and preserve sanitized
   evidence.

Migration `026` may be rolled down only after an explicit
`SELECT count(*) FROM payment_provider_commands` returns zero. If a rollback is
attempted while commands exist, the down migration preserves the table and
fails; `golang-migrate` then records version `25` as dirty. Verify that the
version-26 table and command rows remain intact, then restore migration
metadata with `migrate force 26`. Never force version `25` while the outbox
table exists, and never delete commands to make rollback pass.
5. Keep uncertain attempts/refunds in their current non-terminal state.
6. Reconcile by authenticated inquiry with the original provider IDs and keys.
7. Re-enable one capability at a time in the frozen activation order.

### 14.2 Data rollback

- Never downgrade `CAPTURED`, delete a capture, mark an uncertain command
  failed, edit an immutable snapshot/fact, or repair by direct SQL.
- A bad fact is corrected by a new exception/reversal fact after review.
- Down migrations run only on an empty disposable database and must refuse when
  rows or dependent references exist.
- On a non-empty environment, rollback means disable runtime capability and
  deploy a forward corrective migration.
- Existing booking, legacy owner finance, platform audit, and platform ledger
  tables are not deleted or rewritten.

### 14.3 Provider-call uncertainty

If a worker crashes or times out after a possible provider acceptance:

- preserve the original local attempt/command;
- reuse the same idempotency key and hash;
- inquire before any new create/refund call;
- never create another external payment/refund to “recover”;
- escalate as unresolved when the bounded window expires.

### 14.4 Webhook contract failure

If Dashboard testing shows that Payment Session does not send the provisional
token, adds an undocumented signing/timestamp contract, or lacks stable event
identity:

- leave the processor disabled;
- return 5C to `NO-GO`;
- continue status recovery only through authenticated inquiry;
- amend 5A-04 and this contract with provider evidence;
- do not weaken verification or accept browser authority.

## 15. Phase handoff and human verdict

The prerequisite documents now permit technical implementation planning:

- 5A-01 provider capability: ready;
- 5A-02 sandbox/shadow fund-flow: provisionally accepted, Live approvals
  deferred;
- 5A-03 payment/refund state machine: frozen;
- 5A-04 security/privacy/operations: ready for design with controlled webhook
  runtime verification deferred;
- 5A-05 technical contract: frozen by this document.

The project owner directed progression under the internal Test Mode boundary
and deferred runtime Payment Session delivery proof until the hardened endpoint
exists. Accordingly, the required human verdict is:

**GO FOR SANDBOX/SHADOW ONLY**

This verdict authorizes Task 5B-00 read-only preflight as the next task. It does
not authorize implementation to skip its task gates, webhook processor
activation before proof, Live Mode, production credentials, real funds,
xenPlatform, settlement, payout, owner transfer, production journal, or
production revenue.
