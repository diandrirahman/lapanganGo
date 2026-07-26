# Task 5A-01 — Provider Capability Evidence

Status: **READY FOR 5A-02** (technical capability evidence complete; Phase 5
remains sandbox/shadow-only and the human approval gates are still open)

Evidence date: 2026-07-23
Repository baseline: `master`, clean working tree before this artifact
Provider: **Xendit Test Mode**
Merchant account scope: LapangGo's Xendit Test Mode dashboard account (the
Business ID and all credentials are intentionally not recorded here)

## Decision boundary

This task proves provider capability and freezes the candidate sandbox
surface. It does not create an adapter, migration, webhook endpoint, API key,
payment order, refund, payout, settlement, or journal. A Test Mode balance or
transaction is virtual and must not be treated as a production fund-flow fact.

`PLATFORM_MONETIZATION_ENABLED=false` remains mandatory. xenPlatform,
split-payment, transfer, payout, and Live Mode remain disabled.

## Evidence classes

- **Official**: current Xendit documentation or public pricing page, linked in
  the source register below.
- **Operator-provided**: the dashboard/preflight facts supplied for this
  task. No screenshot, secret, Business ID, or token is copied into the repo.
- **Open**: a capability exists, but an account-specific commercial or legal
  decision is still required in a later Phase 5A task.

## Capability matrix

| Required capability | Result | Evidence and boundary |
|---|---|---|
| Named provider and sandbox | PASS | Operator-provided dashboard is in Xendit Test Mode. Xendit documents Test Mode as using virtual funds and separate from Live Mode. |
| Merchant account | PASS (sandbox scope) | Operator-provided Xendit Test Mode merchant dashboard and `lg-demo-pay` Test API key name. The legal entity, Business ID, and Live merchant onboarding are deliberately not asserted. |
| Initial payment methods | PASS | Operator-provided dashboard shows BCA VA, QRIS, cards, and other Indonesian channels enabled in Test Mode. Xendit documents BCA VA (`BCA_VIRTUAL_ACCOUNT`, IDR, ID), QRIS (`QR`), and Indonesian cards on its channel/pricing pages. |
| Hosted checkout / redirect | PASS | Payment Session `PAYMENT_LINK` redirects to Xendit's hosted checkout. Xendit documents a staging checkout URL for Test Mode. |
| Payment creation and status inquiry | PASS | Xendit Payments API exposes `/v3/payment_requests` and `GET /v3/payment_requests/{id}`. The documented status set includes `ACCEPTING_PAYMENTS`, `REQUIRES_ACTION`, `SUCCEEDED`, `FAILED`, `EXPIRED`, and `CANCELED`. |
| Test success/failure simulation | PASS | Test-only `POST /v3/payment_requests/{payment_request_id}/simulate` returns `PENDING` and reports the final result via webhook. Xendit also publishes card test scenarios. |
| Payment webhook capability | PASS | Xendit documents payment/session webhooks, payment success/pending/failure/expiry events, and a webhook authentication token. A separate Xendit callback product documents HMAC-SHA256 plus timestamp headers. The exact signing contract for the selected Session/Payment Request surface must be frozen in 5A-04. |
| Refund API and refund webhook | PASS | `POST /refunds` accepts a Payment Request ID and returns a pending refund; documented terminal states include `SUCCEEDED`, `FAILED`, `PENDING`, and `CANCELLED`. Xendit documents `refund.succeeded` / `refund.failed` webhooks and warns that provider acceptance does not guarantee when the end-user account reflects the funds. LapangGo remains full-refund-only for Phase 5. |
| Marketplace / split / transfer | PASS as provider capability; **DEFERRED by decision** | xenPlatform documents sub-accounts, split rules, transfers, and payout routing. Test Mode can simulate payments, split, and transfers when xenPlatform is activated. LapangGo must not activate xenPlatform in Phase 5A or the sandbox demo. |
| Settlement | PASS as provider capability; **OPEN for ADR** | Xendit documents channel-specific settlement ranging from instant to several business days and separate settlement reconciliation concerns. Destination account, custody, seller of record, and settlement accounting are not decided here. |
| KYC / sub-account verification | PASS as provider capability; **DEFERRED** | Xendit documents sub-account verification, authorized representative, document upload, and verification status. No LapangGo sub-account or KYC data is created for this demo. |
| Provider fee evidence | PASS for public reference; **OPEN for account contract** | Public standard rates currently show BCA VA: IDR 9,000 method fee + IDR 4,000 Xendit fee; QRIS: 0.70% inclusive of VAT + IDR 4,000 Xendit fee; Indonesia cards: 2.90% + IDR 2,000 + IDR 4,000 Xendit fee. These are not asserted as LapangGo's negotiated/account-specific rate. |
| Tax evidence | PASS for rule; **OPEN for contract** | Xendit states local taxes apply according to the contracting/billing entity and quoted fees may be exclusive of taxes; tax is calculated/deducted during settlement. Finance/legal must confirm the applicable invoice/tax treatment. |
| API version | PASS for the selected test API surface | Xendit's status and simulation references document `api-version: 2024-11-11` for `/v3/payment_requests`. Payment Session is the hosted-checkout surface; 5A-05 must freeze the final endpoint/version combination before implementation. |
| Security/permission posture | PASS for sandbox preparation | Operator-provided Test API key is Money-In Read/Write only; Money-Out, Balance, Report, xenPlatform and identity permissions are None. Secret remains backend-only and is not recorded in this evidence. |

## Frozen sandbox capability decision

For the demo, the provider surface is limited to:

1. Xendit Test Mode.
2. Hosted Payment Session with `mode=PAYMENT_LINK`.
3. Initial methods BCA Virtual Account, QRIS, and cards.
4. IDR only.
5. Test simulation and webhook-driven status observation.
6. Full-refund policy only, after the later approval workflow; no partial
   refund feature is exposed by LapangGo even though the provider API accepts
   an amount.

The following are explicitly outside the demo: xenPlatform activation,
sub-accounts, split rules, transfers, owner payout, live settlement execution,
production credentials, real funds, actual journal, and production revenue.

## Open decisions handed to later tasks

- 5A-02 must decide merchant/seller of record, custody, clearing, settlement,
  owner liability, refund/chargeback responsibility, and fee/tax mapping.
- 5A-03 must freeze the local payment/refund state machine and authority.
- 5A-04 must confirm the exact webhook product, signing/token bytes,
  timestamp/replay rules, and redaction/runbook requirements. Xendit has
  product-specific webhook documentation; these contracts must not be mixed.
- 5A-05 must freeze endpoint/version, normalized DTOs, idempotency namespace,
  feature flags, metrics, fixtures, and rollback plan.
- Human Finance, Legal, Security, and Product approvers must be named and date
  their approvals before any implementation or sandbox/shadow activation gate.

## Source register

All official sources were reviewed on 2026-07-23:

- [Xendit dashboard modes and virtual Test Mode funds](https://docs.xendit.co/v1/docs/your-dashboard)
- [Xendit payment integration checklist](https://docs.xendit.co/docs/payments-integration-setup)
- [Payment Session / hosted checkout API](https://docs.xendit.co/apidocs/create-session)
- [Payments API overview](https://docs.xendit.co/v1/docs/how-payments-api-work)
- [Payment Request status](https://docs.xendit.co/apidocs/get-payment-request)
- [Test-mode simulation](https://docs.xendit.co/apidocs/simulate-payment-test-mode)
- [BCA Virtual Account](https://docs.xendit.co/docs/bca-virtual-account)
- [Available payment channels](https://docs.xendit.co/docs/available-payment-channels)
- [Xendit public pricing](https://www.xendit.co/en/pricing/)
- [Payment webhook notification](https://docs.xendit.co/apidocs/payment-webhook-notification)
- [Payment Session webhook](https://docs.xendit.co/apidocs/webhook-notification-sent-defined-webhook-url-updates-payment-session)
- [Webhook handling and callback token](https://docs.xendit.co/v1/docs/handling-webhooks)
- [Payment/refund webhook events](https://docs.xendit.co/docs/payments-api-webhooks)
- [Refund API](https://docs.xendit.co/apidocs/refund-payment-request)
- [Refund webhook](https://docs.xendit.co/apidocs/refund-webhook-notification)
- [Settlement overview](https://docs.xendit.co/docs/settlements-overview)
- [xenPlatform test capabilities](https://docs.xendit.co/docs/testing-xenplatform-features)
- [xenPlatform split payments](https://docs.xendit.co/docs/split-payments)
- [xenPlatform sub-account/KYC setup](https://docs.xendit.co/docs/xenplatform-global-accounts-setup)

## Verdict

**READY FOR 5A-02**

This verdict means the provider capability evidence is sufficient to begin
the fund-flow/accounting ADR. It does not authorize provider calls, source
implementation, xenPlatform activation, Live Mode, payout, settlement, or
actual finance posting.
