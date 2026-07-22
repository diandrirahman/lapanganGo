# v1.7 Platform Finance Incident Runbook

This is a read-only/simulation-era operational runbook. It does not authorize
payment capture, refund dispatch, settlement, payout, LIVE terms, or production
database destruction.

## 1. Safety preflight

Before any diagnostic or recovery action:

1. Confirm the target is disposable/staging or an explicitly approved read-only
   environment. Never paste a production DSN, JWT, password, provider key, or PII
   into a command, ticket, or evidence log.
2. Record application version, migration `version|dirty`, feature flags, date
   range, and operator role. Keep credentials in the secret manager; use
   `<redacted-disposable-dsn>` in shared artifacts.
3. If there is a posted-fact mismatch, duplicate, missing snapshot, audit gap,
   dirty migration, or non-zero unexplained difference, stop the affected
   mutation/release decision first. Do not edit or delete a posted ledger,
   snapshot, audit row, or booking fact.

## 2. Feature-disable procedure

The kill switch is configuration-driven and fail closed:

```text
PLATFORM_MONETIZATION_ENABLED=false
PLATFORM_FINANCE_ADMIN_ENABLED=false
VITE_PLATFORM_FINANCE_ADMIN_ENABLED=false
```

`PLATFORM_MONETIZATION_ENABLED=true` is rejected by startup validation in Phase
4 across all environments. It must not be used as an incident workaround.

When admin diagnostics or OPEX/journal mutations must be isolated:

1. Set backend `PLATFORM_FINANCE_ADMIN_ENABLED=false` and restart the API.
2. Build/redeploy the web bundle with
   `VITE_PLATFORM_FINANCE_ADMIN_ENABLED=false`.
3. Verify the affected backend `/admin/finance/*` API paths return `404`, the
   UI menu is absent, and direct `/admin/finance/*` navigation redirects to
   `/admin/dashboard`.
4. Verify health and unaffected booking/owner flows before deciding whether to
   continue service. The database schema and existing facts must remain intact.

The commercial-terms SuperAdmin simulation/read/preview surface is separate;
the Platform Finance admin flag does not claim to disable it. LIVE activation is
still rejected.

## 3. Read-only reconciliation diagnostic

The approved v1.7 diagnostic is the CLI; no reconciliation HTTP endpoint is
required by this phase. Run it from `apps/api` with explicit Jakarta dates:

```powershell
Set-Location apps/api
$env:RECONCILIATION_DATABASE_URL = '<redacted-disposable-dsn>'
go run ./cmd/reconcile-platform-finance `
  --start-date=2030-01-01 `
  --end-date=2030-01-31 `
  1>reconciliation.stdout.json 2>reconciliation.stderr.log
Remove-Item Env:RECONCILIATION_DATABASE_URL -ErrorAction SilentlyContinue
```

Contract:

- both dates are mandatory `YYYY-MM-DD` values;
- range is Jakarta calendar inclusive and UTC half-open internally;
- maximum range is 366 days;
- stdout is deterministic versioned JSON (`version=1`) without raw reasons,
  credentials, or PII;
- exit `0` means `clean=true`; exit `1` means exception, blocked integrity,
  setup, argument, or serialization failure;
- stderr contains only sanitized categories such as `invalid_arguments`,
  `setup_failed`, `reconciliation_failed`, or `serialization_failed`.

Never treat a `BLOCKED` check or an empty result caused by an error as clean.
Record the exact `bucket_date`, check code, metric, expected/actual count, and
expected/actual rupiah from the JSON. Do not copy raw SQL error text into a
release artifact.

## 4. Anomaly response

For any reconciliation exception:

1. Freeze the affected release/report/payout decision and preserve the sanitized
   CLI JSON plus migration/flag metadata.
2. Classify the exception: source/ledger mismatch, missing or duplicate event,
   missing snapshot, offline non-zero commission, refund mismatch, OPEX
   post/reversal mismatch, rollup mismatch, or actual-metric availability fault.
3. Finance/Ops verifies the business source and expected rupiah. Platform
   Engineering verifies immutable snapshot, journal, ledger, audit, and
   idempotency links. Security handles auth/audit/secret exposure. The Release
   Owner controls GO/NO-GO.
4. Use the official idempotent reversal/adjustment flow if a correction is
   approved. Never make a compensating change by direct SQL or mutate the
   original posted fact.
5. Re-run the narrow regression and the relevant reconciliation range. A
   difference must be explained and Rp0 before the release decision resumes.
6. Record root cause, impacted bucket(s), evidence hashes, corrective action,
   and regression reference before closing the incident.

## 5. Rollback and migration safety

Before the first business fact, a disposable database may test the reviewed
down-migration path after recording a schema fingerprint. After facts exist:

- do not run `migrate down`, `DROP TABLE`, `TRUNCATE`, or `Force()` on shared or
  production databases;
- disable the feature and roll back application code if needed;
- preserve facts and roll the schema forward with an approved migration;
- let the guarded down migrations refuse when facts, cutover, snapshots,
  journals/ledger, expenses/idempotency, audit rows, or a missing, additional,
  or mutated frozen seed are present. The sole pristine migration-019 seed is
  allowed only in the reviewed disposable/pre-fact down path;
- use backups/restore only through the approved database recovery procedure.

After recovery, verify migration `version|dirty`, route/flag behavior, ledger
balance, audit atomicity, snapshot immutability, and reconciliation before any
re-enable decision.

## 6. Incident record template

```text
Incident ID: <ticket-id>
Detected at (Asia/Jakarta): <timestamp>
Release/application commit: <commit>
Environment: <disposable|staging; never paste DSN>
Flags: monetization=<false>; admin=<false|true>; web=<false|true>
Migration: <version>|<dirty>
Range: <start_date> .. <end_date>
Check/metric/bucket_date: <code> / <metric> / <YYYY-MM-DD>
Expected vs actual: <counts and IDR strings>
Immediate containment: <feature-disable/freeze>
Root cause: <pending until verified>
Official correction/reversal: <reference>
Regression evidence: <command + sanitized log hash>
Release decision: <NO-GO until explained and rechecked>
```

## 7. Stop conditions

Stop and escalate when any of these occurs: non-zero unexplained reconciliation,
`BLOCKED` check, dirty migration, duplicate or unbalanced posted journal,
missing post-cutover snapshot, failed audit atomicity, unauthorized LIVE/provider
activity, or any credential/PII leak. No limitation in this runbook waives a
P0/P1 money, auth, integrity, or security failure.
