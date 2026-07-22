# v1.7 Platform Finance — Release Readiness

## Handoff

Task: `4-09 — v1.7 Release Docs dan Runbook`

Baseline: `master / 532c8a5b7`; working tree also contains pre-existing
user-owned `MabarSection.tsx`, `VenueSection.tsx`, and prior QA evidence docs.

Objective: publish release-facing scope, limitations, metric semantics,
feature-disable behavior, rollback rules, and anomaly ownership for v1.7.
Verdict: `READY FOR 4-10`

This task changes documentation only. It does not enable monetization, alter
financial facts, change migrations, or claim production/LIVE readiness.

## Release position

v1.7 Phase 0–4 is an admin analytics and accounting-foundation release in
`SIMULATION` mode. It is suitable for reviewed internal/demo or disposable QA
use when the required flags and data protections are applied. It is not a
payment, settlement, tax, statutory accounting, or payout release.

The authoritative evidence set is:

- [Task 4-05 automated regression](task_4-05_automated_full_regression_evidence.md)
- [Task 4-06 booking/snapshot/projection QA](task_4-06_manual_qa_booking_snapshot_projection.md)
- [Task 4-07 OPEX/audit/auth/owner QA](task_4-07_manual_qa_opex_audit_auth_owner.md)
- [Task 4-08 time/reconciliation/UX QA](task_4-08_manual_qa_time_reconciliation_ux.md)

Those reports use disposable environments and sanitized external logs. They do
not authorize a production rollout or a LIVE owner cohort.

The older [`mvp_known_limitations.md`](mvp_known_limitations.md) remains a
version-1.2 historical note. For v1.7, the explicit limitations in this report
and the metric dictionary are authoritative; the v1.7 simulation dashboard does
not replace the missing gateway, settlement, payable, payout, or tax/reporting
capabilities.

## Explicitly unavailable in v1.7

The following must be shown as unavailable/null or rejected, never invented as
cash or as a successful provider integration:

- payment gateway, provider capture, webhook verification, and provider settlement;
- actual commission revenue and any LIVE commission write;
- payment-processing/provider fees, provider tax, refund fee, chargeback loss;
- owner payable, settlement liability, and owner payout;
- actual platform revenue, transaction contribution, and operating result;
- tax report, statutory financial statement, GAAP/IFRS result, or net-profit claim.

`projected_commission` and
`projected_operating_result_before_transaction_costs` are scenario/ledger
analytics only. The latter excludes gateway, provider tax, payout, refund,
chargeback, and platform-funded subsidy costs and must not be called net profit.

## Metric and presentation contract

The [v1.7 metric dictionary](platform_finance_metric_dictionary.md) is the
single release-facing vocabulary. In summary:

- currency is integer IDR (`BIGINT`/`int64`, serialized as strings);
- business timezone is `Asia/Jakarta`;
- report dates are inclusive calendar dates converted to a half-open UTC range
  `[start_date 00:00, end_date + 1 day 00:00)`;
- the maximum accepted report range is 366 calendar days;
- unavailable actual metrics use explicit `null`/`UNAVAILABLE_*` availability,
  not `Rp0`;
- historical rows are `LEGACY_NO_COMMISSION` and non-billable;
- offline/walk-in bookings are commission-exempt;
- full refunds reverse the immutable original projection exactly;
- global Platform OPEX is not allocated into owner/venue-scoped OPEX metrics.

## Feature-disable contract

The flags are intentionally separate:

| Flag | Default | Release behavior |
|---|---:|---|
| `PLATFORM_MONETIZATION_ENABLED` | `false` | Strict lowercase parsing. `true` is rejected by `Config.Validate()` in Phase 4 across all environments before database/migration side effects. |
| `PLATFORM_FINANCE_ADMIN_ENABLED` | `false` | Controls Platform Finance admin API/OPEX/journal route registration. `false` omits those routes and returns 404; it does not delete schema or facts. |
| `VITE_PLATFORM_FINANCE_ADMIN_ENABLED` | `false` | Build-time web flag. `false` hides the menu and redirects `/admin/finance/*` to `/admin/dashboard`; a web rebuild/redeploy is required after changing it. |

`PLATFORM_FINANCE_ADMIN_ENABLED=true` is allowed only for an isolated
simulation/QA environment. It does not enable monetization or LIVE writes.
The commercial-terms SuperAdmin simulation/read/preview surface is separate and
is not implied to be disabled by the Platform Finance admin flag; LIVE term
activation remains rejected.

When disabling after facts exist, restart/redeploy the API and rebuild/redeploy
the web bundle as needed. Preserve all snapshots, journals, ledgers, audit rows,
and expense facts.

## Rollback position

Rollback is a controlled schema operation, not an incident data-edit shortcut.
The [incident runbook](platform_finance_incident_runbook.md) is mandatory.

- Destructive down migrations are permitted only on disposable/pre-fact databases
  with an approved backup and migration fingerprint.
- Migrations `019`–`024` refuse to drop objects when business facts, cutover,
  snapshots, journals/ledger entries, expenses/idempotency facts, or a missing,
  additional, or mutated frozen seed are present. Migration `019` permits its
  sole pristine seed only on the reviewed disposable/pre-fact down path.
- After the first business fact exists, disable the feature, stop the affected
  mutation/operator, preserve evidence, roll back application code if needed,
  and roll the schema forward. Do not force a down migration or delete facts.
- Corrections use the official idempotent reversal/adjustment flow and are
  re-reconciled; direct SQL edits to posted ledger/snapshot/audit facts are not
  an approved recovery method.

## Anomaly ownership and release stop conditions

No personal names are invented in this document. Ownership is by role:

| Signal | First owner | Required action | Escalate to |
|---|---|---|---|
| Non-zero reconciliation difference, wrong bucket, OPEX mismatch | Finance/Ops | Freeze affected reporting decision, preserve report and source references, verify Jakarta bucket and expected/actual rupiah | Platform Engineering + Release Owner |
| Missing/duplicate snapshot, booking-source mismatch, ledger imbalance | Platform Engineering | Stop relevant mutation path, inspect immutable facts and idempotency/reversal trail, add regression before recovery | Finance/Ops + Release Owner |
| 401/403/404 drift, audit gap, secret or credential exposure | Security/Platform | Disable affected admin surface, preserve sanitized logs, rotate/revoke exposed credential through approved security process | Release Owner |
| Migration dirty state, failed down refusal, schema drift | Database/Platform | Stop migration activity, record version/dirty state, use disposable reproduction and forward repair | Release Owner |

Any `FAIL`, `BLOCKED`, non-zero unexplained difference, dirty migration,
credential leak, or unauthorized LIVE/provider activity is a release stop. The
release owner records GO/NO-GO; this document does not grant authority to waive
those conditions.

## Task 4-09 result

Readiness, known limitations, simulation/tax-report wording, metric dictionary,
feature-disable behavior, rollback policy, and anomaly ownership are documented
in this file and its linked runbook/dictionary. No secrets or unsupported LIVE
claims are included.

### Finalization record

- Files added: this readiness report, `platform_finance_metric_dictionary.md`,
  and `platform_finance_incident_runbook.md`.
- Supporting `.gitignore` exceptions were added so the three release documents
  can be reviewed and committed intentionally.
- Application behavior, database schema, migration files, and financial facts
  were not changed.
- Verification: scoped secret/credential scan returned zero hits; tracked diff
  whitespace check passed. Runtime regression evidence is referenced from
  Tasks 4-05 through 4-08 and is not rerun by this documentation-only task.
- Skipped/unverified: independent Task 4-10 audit and final Git commit remain
  pending. No production/LIVE execution is claimed.
- Commit: not committed; explicit finalization authorization is still required.

**READY FOR 4-10**
