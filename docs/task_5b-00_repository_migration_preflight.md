# Task 5B-00 -- Repository/Migration Preflight

- Status: **READY FOR 5B-01 -- runtime gate passed**
- Date: 2026-07-25
- Scope: read-only repository, schema, configuration, and local-runtime audit
- Intended output when all evidence passes: `READY FOR 5B-01`
- Current output: **READY FOR 5B-01** (gate passed before migration 025 was applied)

## 1. Boundary

This task creates no payment schema, source code, provider adapter, endpoint,
credential, Xendit request, webhook, payout, settlement, owner transfer, or
production financial record. It checks whether the foundation is safe for
Task 5B-01 only.

The Phase 5 contract remains sandbox/shadow-only:

- `PLATFORM_MONETIZATION_ENABLED=false`;
- Xendit Test Mode and virtual funds only;
- no Live Mode, xenPlatform, Money-Out, split payment, payout, settlement, or
  owner transfer.

## 2. Read-only evidence

| Check | Result | Evidence |
|---|---|---|
| Actual branch | PASS | `master` |
| Working-tree overlap | PASS with documentation-only changes | The only changes are `.gitignore` plus the five Task 5A evidence/contract documents. No API, web, migration, compose, or payment implementation file is modified. |
| Migration inventory | PASS (static) | All migrations `001` through `024` have paired `*.up.sql` and `*.down.sql` files. |
| Reserved numbers | PASS (static) | `025`, `026`, `027`, and `028` are absent and available. |
| Existing Phase 5 payment tables | PASS (static) | No `payment_attempts`, `payment_capture_facts`, `payment_provider_commands`, `payment_webhook_events`, `payment_refunds`, or `payment_cost_items` identifier exists in `apps/api` or `db/migrations`. |
| Sandbox monetization guard | PASS | `Config.Validate` rejects `PLATFORM_MONETIZATION_ENABLED=true`; `go test ./internal/config` passed. |
| Tracked credential exposure | PASS | A path-only scan of tracked non-`.env` files found no credential-like value. `apps/api/.env` and `apps/web/.env` are ignored and not tracked. No secret value was read or printed. |
| Docker/PostgreSQL runtime | PASS | PostgreSQL was healthy, `schema_migrations` was `24|f`, and all required foundation tables were present before Task 5B-01 began. |

## 3. Static dependency map

```text
bookings (004, payment reference 006, expiry 007)
  -> booking_fee_snapshots (020; one immutable snapshot per booking)
  -> [025 payment_attempts + payment_capture_facts]
  -> [026 provider-command outbox]
  -> [027 verified webhook inbox]
  -> [028 refund and provider-cost facts]

platform_audit_logs (019) <--- sanitized audit for each accepted mutation
platform_journals / platform_ledger_entries (022) <--- isolated test-only
                                                      source references in 028
platform_expense_idempotency (024) <--- existing pattern only; no reuse
owner_finance_transactions (009) <--- legacy path; gateway must not write it
```

The required foundation exists statically:

- `bookings` provides the booking identity; migrations 006 and 007 add the
  legacy payment reference and booking expiry;
- `booking_fee_snapshots` (020) has the `booking_id` primary key, `SIMULATION`
  finance mode, IDR/integer-rupiah constraints, and the canonical
  `customer_charge_amount_rupiah` input frozen by Task 5A-05;
- `platform_audit_logs` (019) provides an audit sink;
- `platform_journals` and `platform_ledger_entries` (022) are immutable
  ledger structures, but Phase 5B must not write production journal facts;
- `platform_expense_idempotency` (024) demonstrates an existing immutable
  idempotency pattern but is unrelated to payment idempotency; and
- `owner_finance_transactions` (009) is a legacy owner-finance path and must
  remain isolated from gateway capture/refund processing.

## 4. Runtime proof recorded

The following read-only proof was recorded from the repository root before
Task 5B-01 began:

```powershell
docker compose ps
docker compose exec -T postgres psql -U lapangango_user -d lapangango_db -Atc "SELECT version, dirty FROM schema_migrations LIMIT 1"
docker compose exec -T postgres psql -U lapangango_user -d lapangango_db -Atc "SELECT to_regclass('public.bookings'), to_regclass('public.booking_fee_snapshots'), to_regclass('public.platform_audit_logs'), to_regclass('public.platform_journals'), to_regclass('public.platform_ledger_entries'), to_regclass('public.platform_expense_idempotency')"
```

Recorded results:

1. PostgreSQL was healthy.
2. Migration state was `24|f` and was not dirty.
3. Every required `to_regclass` result was non-null.
4. The working tree had no overlapping payment/source/migration edits.

Task 5B-01 has subsequently applied migration 025; the current local
development state is therefore `25|f`. Its separate evidence document records
the implementation and verification.

## 5. Verdict

The repository and static schema foundations were compatible with the frozen
Task 5A-05 contract, and migration 025 was free when the gate ran. The actual
database verification passed at migration 24 clean.

**Verdict: READY FOR 5B-01 -- consumed successfully by Task 5B-01.**
