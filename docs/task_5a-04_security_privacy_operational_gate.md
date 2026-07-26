# Task 5A-04 -- Security, Privacy, Operational Gate

- Status: **READY FOR 5A-05 -- SANDBOX/SHADOW DESIGN ONLY**
- Sub-status: **RUNTIME PAYMENT SESSION WEBHOOK VERIFICATION DEFERRED WITH HARD STOP**
- Date: 2026-07-23
- Last reviewed: 2026-07-24
- Provider: Xendit Test Mode
- Scope: Phase 5 sandbox/shadow design only
- Runtime guard: `PLATFORM_MONETIZATION_ENABLED=false`
- Related documents: `task_5a-01_provider_capability_evidence.md`, `task_5a-02_xendit_fund_flow_accounting_adr.md`, `task_5a-03_payment_refund_state_machine.md`

## 1. Gate purpose and boundary

Task 5A-04 prevents a payment adapter from being implemented with an assumed
webhook security contract or unsafe operational defaults. It freezes secret
handling, verification order, replay and duplicate handling, payload limits,
redaction, audit fields, rotation, and incident response before Task 5A-05.

This is a documentation and evidence task. It does **not** create an API key,
webhook endpoint, adapter, migration, payment/refund command, provider call,
ledger entry, payout, settlement, or production financial fact. No real
secret, raw webhook body, PAN, CVV, bank credential, or customer fixture is
stored in this repository.

The demo remains isolated to Xendit Test Mode and virtual funds. xenPlatform,
split payment, transfer, payout, Live Mode, and production monetization remain
disabled. The existing read-only controls in
`docs/platform_finance_incident_runbook.md` continue to apply.

### Project-owner deferral decision

On 2026-07-24, the project owner directed that runtime proof of the Payment
Session webhook header may be deferred until the hardened endpoint exists.
This is permitted only because the work remains an internal Test Mode demo and
the webhook processor is disabled. The decision moves evidence collection; it
does not treat the provider contract as proven.

The provisional design assumption is `x-callback-token`, based on Xendit's
general webhook documentation, the Test Mode callback-token dashboard, and the
product-specific Payment Session URL controls. There is no claim that Payment
Session uses HMAC, canonical signed bytes, a signed timestamp, or a unique
event header. Any differing test result returns this contract to
`PHASE 5 NO-GO`.

## 2. Evidence register

The following evidence was checked against Xendit's official documentation on
2026-07-23 and reviewed again on 2026-07-24. A provider capability is not
treated as proof of the exact
contract for another Xendit product.

| Evidence | Confirmed fact | Security implication |
|---|---|---|
| [Handling webhooks](https://docs.xendit.co/v1/docs/handling-webhooks) | Webhooks are asynchronous, can be duplicated, should be authenticated with the `x-callback-token` header, handled server-side, and acknowledged quickly. Xendit may retry a failed delivery. | Verification must happen at the server ingress before any business mutation; inbox processing must be idempotent and asynchronous. |
| [Payment webhook notification](https://docs.xendit.co/apidocs/payment-webhook-notification) | The payment webhook page documents `x-callback-token` and payment/capture identifiers for the payment webhook product. | This is usable only if the final integration includes that documented payment webhook product. It is not silently copied to Payment Session. |
| [Refund webhook notification](https://docs.xendit.co/apidocs/refund-webhook-notification) | The refund webhook page documents `refund.succeeded` / `refund.failed` and an `x-callback-token` header. | Refund authentication uses the documented token contract and constant-time comparison; the event must still be deduplicated and matched to the original full-refund request. |
| [Payment Session webhook notification](https://docs.xendit.co/apidocs/webhook-notification-sent-defined-webhook-url-updates-payment-session) | The page documents `payment_session.completed` and `payment_session.expired` bodies and labels endpoint security as `HTTP` / `basic`, but does not specify a callback-token header, signature algorithm, canonical bytes, timestamp header, or event-id contract. | **Blocker:** the selected hosted Payment Session webhook cannot be accepted as authenticated until Xendit confirms the exact wire contract. |
| [Payment Session overview](https://docs.xendit.co/v1/docs/payment-sessions-overview) | A session is asynchronous and emits completion/expiry webhooks. | Browser return URLs are navigation only; the server must use an authenticated provider event or inquiry. |
| [Create a Payment Session](https://docs.xendit.co/apidocs/create-session) | The request supports `capture_method`, `allow_save_payment_method`, customer data, metadata, and HTTPS return URLs. | The demo must freeze automatic capture, disable saved payment methods, minimize outbound customer data, and use configured allowlisted return URLs. |
| [Webhook behavior](https://docs.xendit.co/apidocs/webhook-behavior) | Xendit can retry webhook delivery until a successful 2xx response (the published behavior describes up to six retries). | The endpoint must durably record an inbox item and return 2xx quickly; retries must be safe. |
| [Get Payment Request](https://docs.xendit.co/apidocs/get-payment-request) | The server-side inquiry surface documents payment statuses and is versioned with `api-version: 2024-11-11`. | An authenticated inquiry can resolve `PENDING` only when the ID, amount, currency, and merchant account match the local attempt. |
| [Payment status callback (Bill Payments)](https://docs.xendit.co/apidocs/payment-status-callback-webhook) | A separate Bill Payments product documents `X-Callback-Signature`, `X-Callback-Timestamp`, and HMAC-SHA256. | This is a different product surface. Its algorithm and canonical bytes **must not** be assumed for Payment Session. |

## 3. Selected demo surface

The candidate sandbox surface inherited from 5A-01 is:

1. Xendit Test Mode and IDR only.
2. Hosted Payment Session with `session_type=PAY`,
   `mode=PAYMENT_LINK`, `capture_method=AUTOMATIC`,
   `allow_save_payment_method=DISABLED`, and `country=ID`.
3. LapangGo does not request payment-method saving and does not store a
   `payment_token_id`.
4. Initial channels BCA Virtual Account, QRIS, and cards.
5. `payment_session.completed` and `payment_session.expired` as candidate
   session events; `payment.capture` remains conditional on the final selected
   event surface.
6. `refund.succeeded` and `refund.failed` for the one permitted full refund.
7. Server-side status inquiry as a secondary authority.
8. Full refund only; no partial refund, payout, transfer, split, or settlement
   execution.

The final endpoint/version and whether a separate `payment.capture` webhook is
used are still a Task 5A-05 contract decision. The implementation must not
subscribe to a broader webhook family merely because the account exposes it.

Even after the Payment Session webhook contract is authenticated,
`payment_session.completed` is not blindly mapped to local `CAPTURED`.
The event must contain the expected `payment_id`/`payment_request_id`, use the
frozen automatic-capture flow, and match the provider inquiry/payment fact,
amount, currency, merchant account, and immutable local attempt. Task 5A-05
must freeze the exact event-to-state mapping.

## 4. Security contract freeze

### 4.1 Non-negotiable processing order

For every inbound provider request, the future adapter must perform this order:

1. Apply route, header, content-type, and raw-body size limits.
2. Read the raw body once into bounded memory; do not log it.
3. Authenticate the sender using the provider-confirmed contract.
4. Apply timestamp/replay checks and an idempotency decision.
5. Parse the allowlisted schema and validate provider account, event type,
   amount, currency, reference, and provider IDs against the immutable local
   attempt.
6. Persist a sanitized inbox fact atomically, then acknowledge quickly.
7. Process the state transition through the 5A-03 state machine. A failed
   authentication or validation never advances a booking or payment state.

Browser success, cancel, or failure redirects are never payment authority.
Dashboard screenshots and operator claims are investigation evidence only.

### 4.2 Exact authentication contract matrix

| Candidate surface | Header/type | Canonical bytes and algorithm | Timestamp/replay | Gate |
|---|---|---|---|---|
| Hosted Payment Session (`payment_session.*`) | Product page does not specify it; provisional sandbox assumption is `x-callback-token` based on the general webhook contract and Test Mode dashboard. | Constant-time token comparison is the provisional design. No HMAC or canonical signed-body algorithm is claimed. | No signed timestamp or unique event header is proven. Use deterministic object/event identity and body hash; runtime proof is deferred. | **Design may proceed to 5A-05. Processor/state mutation remains blocked until the Phase 5C controlled delivery test passes.** |
| Xendit payment webhook (`payment.capture`, if explicitly selected) | Official page documents `x-callback-token`. | Token comparison; no HMAC/canonical-body rule is documented on that page. | Provider event/ID and duplicate rules must be confirmed for the selected version. | Conditional: usable only after 5A-05 freezes this exact product and test evidence. |
| Xendit refund webhook (`refund.succeeded`, `refund.failed`) | Official page documents `x-callback-token`. | Compare the exact raw token header to the backend-held refund webhook token using constant-time comparison; no HMAC contract is inferred. | No signed timestamp is documented. Deduplicate by the provider refund ID, original payment/request ID, event type/status, and authenticated body hash; conflicting facts are quarantined. | Conditional pass for sandbox fixtures; 5C-00 must prove success, failure, duplicate, tampered token, and conflict cases. |
| Xendit Bill Payments callback | `X-Callback-Signature` and `X-Callback-Timestamp`. | HMAC-SHA256 contract documented for Bill Payments. | Product-specific timestamp rules apply. | **Not interchangeable with Payment Session.** |

The following values remain **runtime-unproven** for the selected Payment
Session surface: actual header delivery, absence/presence of other
authentication headers, signed timestamp semantics, unique event header, and
provider replay behavior. The design freezes callback-token comparison and
deterministic local replay controls provisionally, but must not invent HMAC,
canonical signed bytes, or timestamp tolerance.

After provider confirmation, the implementation contract must state one and
only one of these forms:

- **Token form:** compare the exact raw header value with the backend-held
  webhook token using constant-time comparison. Whitespace, case, alternate
  headers, or body-derived substitutes are not accepted.
- **HMAC form:** compute the provider-specified HMAC over the exact raw request
  bytes and the provider-specified timestamp/canonical string, then compare
  the encoded signature using constant-time comparison. The algorithm,
  delimiter, encoding, and timestamp tolerance must come from the provider
  contract and test vectors; they must not be inferred from another Xendit
  product.

In both forms, authentication happens before JSON parsing, state lookup, or
outbound provider calls. Unknown or unauthenticated events are quarantined and
audited without revealing their body.

For the documented refund token form, absence, duplication, alternate header
names, whitespace-normalized substitutes, or a mismatch must fail closed. The
request body cannot be used to derive or replace the token. Because the
documented refund webhook does not provide a signed timestamp contract, local
replay resistance is based on the immutable refund/payment identifiers,
monotonic refund state, authenticated body hash, and bounded operational
retention; it must not invent an HMAC or timestamp header.

### 4.3 Replay and duplicate controls

The following local controls are frozen even while the provider-specific wire
contract is blocked:

- Prefer a provider event ID when the selected contract supplies one.
- For a repeated authenticated event, the same provider event ID is a no-op.
- If the Payment Session contract supplies no event ID, an exact duplicate is
  recognized only by a hash of the authenticated raw body plus the provider
  object identifiers. A different body for the same object is a conflict and
  is quarantined; it is not treated as a new success.
- A repeated event cannot downgrade a terminal state. Out-of-order delivery
  follows the 5A-03 monotonic transition rules.
- A late capture after `FAILED`, `EXPIRED`, or `CANCELLED` is a reconciliation
  exception. It never reopens a booking or silently fulfills it.
- Under the provisional token contract, signed timestamp tolerance is
  `NOT_APPLICABLE` because no signed timestamp header is claimed. The body
  `created` value is semantic data only: more than five minutes in the future
  is quarantined for clock/contract review, while an old event is handled by
  deterministic deduplication and the monotonic state machine because Xendit
  supports delayed retries/manual resend. It is never authentication proof.
- Provider status inquiry uses the same local attempt, idempotency boundary,
  amount, currency, merchant account, and provider identifier. A timeout
  remains `PENDING`.

### 4.4 Ingress limits and resilience defaults

These are local defensive limits for the future adapter and are not claims
about Xendit's limits:

| Control | Frozen default |
|---|---|
| Raw request body | 256 KiB maximum; reject before unbounded buffering |
| JSON nesting | 16 levels maximum |
| JSON object/array members | 128 members per object/array maximum |
| Metadata retained | 8 KiB after allowlisting and redaction |
| Ingress deadline | 5 seconds; no synchronous inquiry, refund, or booking mutation |
| Acknowledgement | Durable sanitized inbox write first, then fast 2xx; processing is asynchronous |
| Route limiter | Dedicated webhook limiter: 120 requests/minute with burst 30 per route; return generic `429` on excess |
| Outbound calls | Explicit timeout, bounded retry, same idempotency key; never retry an uncertain payment by creating a new attempt |
| Content types | Only the provider-confirmed JSON content type; no form or multipart fallback |

The limit values must be covered by negative tests in 5B. They may be changed
only by a reviewed contract update, not by an emergency workaround in a
handler.

The current API installs a general limiter globally at 100 requests/minute.
The future provider webhook route must not accidentally inherit that
customer-facing limiter before the dedicated webhook limiter. It must be
registered in an explicitly isolated router group or equivalent middleware
order, while still retaining the dedicated limit above. Source IP is only an
auxiliary rate-limit signal and is never a replacement for provider
authentication.

### 4.5 Generic HTTP response matrix

Responses must not reveal whether an identifier, booking, amount, or secret
matched. The response body is a stable generic category plus correlation ID.

| Condition | HTTP result | Provider/local behavior |
|---|---:|---|
| Authenticated event durably inserted | `200` or `204` | Acknowledge quickly; process asynchronously. |
| Authenticated exact duplicate already durable | Same success code as first acceptance | Durable no-op; never create another capture/refund. |
| Authenticated but unsupported event/schema version | `200` or `204` after durable sanitized quarantine | No business mutation; alert for contract review without forcing repeated provider delivery. |
| Missing/invalid authentication | Generic `401` | No inbox/business mutation; audit sanitized failure and allow provider retry behavior to remain observable. |
| Malformed JSON, unsupported content type, or limit violation other than rate | Generic `400`, `415`, or `413` as applicable | No business mutation; do not echo parser/provider details. |
| Dedicated webhook rate limit exceeded | Generic `429` | No mutation; record only aggregate/sanitized metrics. |
| Database/inbox unavailable before durable acceptance | Generic `503` | Xendit may retry; never return 2xx before durability. |
| Business processor fails after durable acceptance | Success response already returned | Inbox remains retryable locally; do not ask Xendit to redeliver a fact already stored. |

Task 5C-03 must test every row. Since Xendit retries any non-2xx delivery, the
endpoint, alerting, and dashboard must tolerate repeated generic failures
without exposing secrets or creating duplicate state transitions.

## 5. Secret and credential management

- The Xendit Test Mode secret key remains backend-only in `apps/api/.env`
  (local, untracked) or an equivalent secret manager. It must never appear in
  `VITE_*`, frontend bundles, source, documentation, screenshots, tickets,
  test output, or chat.
- The webhook token, if confirmed for the selected product, is a separate
  backend secret. It is not the API secret, a public key, a booking ID, or a
  value derived from the request body.
- The current key permission posture is least privilege for the demo:
  Money-In Read/Write only; Money-Out, Balance, Report, xenPlatform, and
  identity permissions remain None.
- Fixtures use placeholders such as `<provider-payment-id>` and fixed hashes;
  they contain no real credential or raw sensitive payload.
- Secret values are never printed during startup, health checks, migrations,
  errors, audit events, or reconciliation. Error messages expose only a
  stable category and correlation ID.

### 5.1 Rotation and suspected exposure runbook

1. Set `PLATFORM_MONETIZATION_ENABLED=false` and stop any provider command or
   demo route that could create a new attempt. Do not attempt an emergency
   payout or refund from a leaked credential.
2. Preserve only sanitized evidence: time window, environment, key label,
   correlation IDs, audit categories, and hashes. Never copy the secret or
   request body into the incident record.
3. Create a new least-privilege Test Mode secret/token in the provider
   dashboard or secret manager. If provider dual-key overlap is supported,
   deploy the new value, verify a synthetic placeholder request, then revoke
   the old value. If overlap is not supported, keep the feature disabled for
   the cutover and verify health after rotation.
4. Search repository, CI logs, container logs, screenshots, and tickets for
   exposure. Remove or restrict access to any artifact containing a secret;
   do not rewrite history as a substitute for revocation.
5. Reconcile the affected virtual payment facts and webhook inbox by provider
   IDs and sanitized hashes. Keep uncertain attempts `PENDING`; do not infer
   capture from a redirect.
6. Record rotation actor/role, key label (not value), time, reason, old-key
   revocation result, verification result, and follow-up actions. Security
   approval is required before any demo re-enable; Live re-enable remains
   prohibited.

## 6. Privacy and redaction gate

### 6.1 Outbound Payment Session minimization

The Test Mode Create Session command is frozen to the minimum provider fields:

- `session_type=PAY`, `mode=PAYMENT_LINK`,
  `capture_method=AUTOMATIC`, `allow_save_payment_method=DISABLED`,
  `country=ID`, `currency=IDR`, and the exact integer booking amount;
- an opaque payment-attempt reference, not a name, email, phone number,
  sequential booking ID, venue name, or other customer/owner identifier;
- the explicit BCA VA, QRIS, and card channel allowlist frozen later in 5A-05;
- fixed non-sensitive description text only; omit `items` and `metadata` unless
  5A-05 proves an allowlisted business need;
- no `customer` or `customer_id` in the demo request while the selected
  `PAY` + `PAYMENT_LINK` + disabled-save contract permits omission.

If a selected channel later requires customer data, the implementation must
stop and return to this privacy gate. It cannot silently add name, email,
mobile number, date of birth, nationality, address, or identity fields.
Synthetic Test Mode data may be used only in provider-owned test tooling and
must not resemble a real person.

Success and cancel return URLs are exact backend configuration values from an
HTTPS origin allowlist. Scheme, host, port, and path are not accepted from the
browser or booking metadata. Query strings/fragments contain no PII, secret,
booking ID, payment token, or provider credential; the server resolves state
from its own authenticated session and opaque local attempt reference. Card,
CVV, bank credential, and checkout collection remain on Xendit's hosted page.

### 6.2 Inbound data minimization

The future inbox stores only the minimum normalized facts required for
idempotency, reconciliation, and the 5A-03 state transition:

- local payment-attempt ID and opaque booking correlation;
- provider event/object IDs and event type;
- integer amount in IDR and currency;
- provider status and server-observed timestamps;
- authenticated raw-body hash for duplicate/conflict detection;
- redacted metadata needed to correlate the demo.

The system must not persist or log PAN, CVV, full bank-account credentials,
payment tokens, API keys, webhook tokens, authorization headers, full URLs with
secrets, or a full raw webhook body. Customer PII is not needed for this demo;
if a provider payload contains it, the adapter must drop it before the inbox
and audit layers.

A non-null `payment_token_id` is unexpected because saved payment methods are
disabled. It must be redacted, quarantined as a provider/configuration
exception, and must not be persisted as a reusable credential.

### 6.3 Logs, audit, access, and retention

- Logs contain event category, local/provider IDs (redacted or hashed where
  practical), state, reason code, latency, and correlation ID only.
- Audit actions are allowlisted: `webhook_received`,
  `webhook_auth_passed`, `webhook_auth_failed`, `webhook_replay`,
  `webhook_duplicate`, `webhook_conflict`, `payment_state_transition`,
  `refund_state_transition`, `provider_command_rejected`, `secret_rotated`,
  `kill_switch_enabled`, and `reconciliation_exception`.
- Audit rows never contain a secret, authorization header, raw body, PAN/CVV,
  or unredacted customer data. Access is server-side and role-limited.
- Raw webhook bodies are not retained. Normalized virtual facts follow the
  existing audit/reconciliation retention policy. Exact Live retention,
  deletion, data-subject handling, and cross-border processing require a
  Privacy/Legal decision before production.
- Support and demo screens show a Test Mode banner and redacted IDs. They do
  not expose the provider dashboard, credentials, or virtual balance as cash.

## 7. Threat model

| Threat | Control frozen by this gate | Residual risk/owner |
|---|---|---|
| Spoofed webhook | Provider-confirmed header/token or signature; constant-time compare; verify before parse/mutation | Payment Session wire contract is unresolved; Security/provider owner must close it. |
| Replay of an old success | Timestamp rule after provider confirmation; event/object dedup; exact duplicate hash; monotonic state machine | No numeric tolerance or event-ID semantics are documented for Payment Session. |
| Duplicate or out-of-order delivery | Durable inbox, idempotency, no-op duplicate, 5A-03 transition matrix | Provider event identity and retry behavior must be tested. |
| Amount/currency/reference tampering | Match immutable local snapshot, IDR, booking reference, provider account, and provider IDs before transition | Adapter tests and contract fixtures are still pending. |
| Browser callback forgery | Redirects are navigation only; state changes require verified event/inquiry | None for design; implementation must preserve this rule. |
| Open redirect or return-URL data leak | Backend-configured HTTPS origin/path allowlist; no browser-supplied target, PII, secret, or provider ID in URL | Deployment-specific HTTPS callback origin must be frozen in 5A-05. |
| Unnecessary outbound PII | Omit customer object, items, and metadata; use opaque reference and fixed description | Any future channel-specific PII requirement must reopen the Privacy gate. |
| Unexpected saved payment token | `allow_save_payment_method=DISABLED`; quarantine and redact any non-null token ID | Provider contract fixtures must prove the disabled-save behavior. |
| Secret/token leakage | Backend-only secret, no raw logging/fixtures, rotation and kill switch | Operational secret-manager access and Live procedures remain open. |
| PII or card-data leakage | Allowlist, redaction, no raw body retention, restricted audit | Live privacy retention/legal decision remains open. |
| Provider outage or network timeout | Keep `PENDING`, bounded retry/inquiry, no second payment attempt, incident runbook | Manual investigation may be needed for long-lived pending attempts. |
| Oversized/malicious payload | Bounded body, depth/member/metadata limits, route limiter, fast rejection | Limits need negative tests in 5B. |
| Unauthorized Live/xenPlatform activity | Runtime flag false, least-privilege key, no Money-Out/xenPlatform permissions, stop conditions | Human approval and deployment controls remain required for Live. |
| Settlement/reconciliation discrepancy | No Phase 5 settlement/payout; sanitized facts and existing read-only reconciliation runbook | LIVE fund-flow/accounting approval is still deferred. |

## 8. Incident operations

### 8.1 Authentication failures or replay spike

1. Keep monetization disabled and stop state-changing processing.
2. Apply the generic response matrix: invalid authentication returns generic
   `401`; temporary inbox/database failure returns generic `503`; neither path
   marks a booking paid.
3. Record counts, time window, endpoint, reason category, and sanitized
   hashes. Never record the body or secret.
4. Rate-limit the route, verify provider configuration and the frozen contract,
   and escalate to Security/provider support if the spike persists.
5. Reconcile all affected attempts by authenticated inquiry. Close only after
   a clean, explainable result and an audit record.

### 8.2 Payment uncertain, provider unavailable, or webhook delayed

1. Keep the attempt `PENDING`; do not create a second provider payment.
2. Retry the same inquiry/command only with the same idempotency boundary and
   bounded backoff.
3. Use the provider dashboard only as investigation evidence. A screenshot
   cannot advance the local state.
4. Escalate stale attempts and document the eventual verified result or
   reconciliation exception.

### 8.3 Amount, currency, merchant, or booking mismatch

Quarantine the event, emit `webhook_conflict`, do not fulfill the booking, and
open a reconciliation incident. Never repair the original fact with direct SQL
or by editing the inbound payload.

### 8.4 PII or credential exposure

Enable the kill switch, restrict log/ticket access, rotate/revoke the exposed
credential, preserve sanitized evidence, remove copies from operational
surfaces, and involve Security/Privacy/Legal according to the incident
severity. Do not paste the exposed value into the incident record while
investigating it.

### 8.5 Provider/settlement discrepancy

Freeze any release, payout, settlement, or Live decision. Use the existing
read-only reconciliation runbook, preserve evidence hashes, and require an
explainable Rp0 result before any future gate can resume. Phase 5 itself has
no payout or settlement action to roll back.

## 9. TOS, privacy, refund, and KYC readiness

| Area | Sandbox decision | Live prerequisite |
|---|---|---|
| Customer disclosure | Demo UI must say Xendit Test Mode, virtual funds, no real charge, and no owner payout. | Approved customer terms and payment disclosure. |
| Privacy | Minimize both outbound Session fields and inbound virtual payment facts; no saved payment method, card data storage, or raw body retention. | Privacy notice, retention/deletion, access, and provider data-processing review. |
| Refund | Full refund only; refund approval is not refund success; provider terminal fact is required. | Approved refund/chargeback policy, support SLA, and accounting/tax treatment. |
| KYC/merchant identity | No sub-account, owner KYC, payout destination, or xenPlatform operation in the demo. | Provider onboarding, legal entity, venue/owner KYC, and permitted marketplace model. |
| Approvals | Finance, Legal, Security, Product, and provider contract approvals may remain deferred for internal Test Mode design as previously recorded. | Named approvers, role, date, and evidence are mandatory before Live or any real-fund flow. |

## 10. Evidence and exit criteria

Evidence recorded for this task:

- official Xendit documentation links and product-surface separation above;
- secret/credential policy and no-secret fixture rule;
- refund webhook token contract and refund replay/conflict boundary;
- outbound Session minimization, disabled saved-payment methods, and HTTPS
  return-URL allowlist;
- bounded ingress, verification order, replay/duplicate, redaction, audit,
  rotation, dedicated rate limiter, generic response matrix, threat model, and
  incident runbook;
- existing repository flags and read-only finance runbook remain consistent
  with sandbox-only operation.

Task 5A-04 permits 5A-05 design under these deferred hard stops:

1. 5A-05 must label the Payment Session callback-token contract
   `PROVISIONAL_TEST_MODE` and keep the webhook processor default-off.
2. Phase 5C must first create redacted local fixtures for the provisional
   token contract. No real secret or customer data may enter a fixture.
3. After the hardened ingress exists, controlled Dashboard `Test and Save`
   deliveries for `payment_session.completed` and
   `payment_session.expired` must prove the header presence and constant-time
   match without logging its value or the raw body.
4. Until that delivery test passes, Payment Session webhooks may be received
   only by a non-mutating diagnostic ingress. They cannot set `CAPTURED`,
   `EXPIRED`, booking `PAID`, refund state, or any finance fact. Authenticated
   provider inquiry remains the only server-side recovery authority.
5. Missing/different authentication, unexpected timestamp/signature behavior,
   or unstable event identity stops Phase 5C and returns to this gate.
6. Finance, Legal, Security, Product, and provider approvals remain required
   for Live; the demo deferral does not waive them.

### Gate verdict

**READY FOR 5A-05 -- SANDBOX/SHADOW DESIGN ONLY.** Security, privacy, and
operational controls are frozen provisionally. Runtime Payment Session webhook
verification is explicitly deferred to the controlled Phase 5C test and is a
hard stop before any webhook processor or state mutation can be enabled.

This decision does not authorize an endpoint, provider call, payment/refund
flow, Live Mode, real funds, xenPlatform, payout, settlement, or actual
journal. Task 5A-05 must issue a separate human
`GO FOR SANDBOX/SHADOW ONLY` verdict before Phase 5B begins.
