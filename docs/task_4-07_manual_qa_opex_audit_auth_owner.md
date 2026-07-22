# Task 4-07 — Manual QA OPEX/Audit/Auth/Owner Evidence

## Verdict

`READY FOR 4-08`

AF-P0-08, AF-P0-09, AF-P0-10, and AF-P1-01 passed on an isolated disposable stack. No production/LIVE environment or provider credential was used.

## Environment and isolation

- Compose project: `lapangango-task407` (disposable only).
- PostgreSQL/API/web ports: `15432`, `18080`, `13000`; Redis `16379`.
- Migration state: `schema_migrations = version 24, dirty false`.
- `PLATFORM_MONETIZATION_ENABLED=false`; Platform Finance admin UI/API was enabled only in this disposable project.
- User-owned files `MabarSection.tsx` and `VenueSection.tsx` were not modified.

## Scenario matrix

| ID | Evidence | Result |
|---|---|---|
| AF-P0-08 | Real SUPER_ADMIN browser workflow created `TASK407-OPEX-001` for `Rp125.000`, then moved `DRAFT → APPROVED → POSTED → VOID`. UI showed the posted journal and exact void reversal links. DB expense `ce4f1f1b-bd18-4aa5-852f-2405e4c78cb9` is `VOID`; posted journal `a6f8d226-9617-4fe9-82c7-27b1f5b45446`; reversal `7260a082-d8df-4a89-83e3-c6651780e49d`. Each journal has two ledger entries with debit `Rp125.000` equal to credit `Rp125.000`. Admin summary after reversal reports platform OPEX `Rp0`, projected operating result `Rp0`, and OPEX availability `AVAILABLE`. | PASS |
| AF-P0-09 | `TestAuditedJournalReversalCommitsJournalAndOneAuditMarker`, `TestAuditedJournalReversalAuditFailureRollsBackJournal`, and `TestPlatformAuditRollbackRemovesJournalAndAuditTogether` passed on disposable ledger databases. Injected audit failure left zero reversal journals, zero reversal entries, and zero audit markers. The real OPEX row has exactly one each of `PLATFORM_EXPENSE_CREATED`, `PLATFORM_EXPENSE_APPROVED`, `PLATFORM_EXPENSE_POSTED`, `PLATFORM_EXPENSE_VOIDED`, and `PLATFORM_FINANCE_JOURNAL_REVERSED`. | PASS |
| AF-P0-10 | Real API matrix: `CUSTOMER`, `OWNER`, and `STAFF` received HTTP `403` for both `/admin/finance/summary` and `/admin/finance/breakdown`; active `SUPER_ADMIN` received `200` for both. `/admin/audit-logs` returned the same `403/200` role boundary. Production-chain unit tests also passed for summary/breakdown and all expense mutation routes. | PASS |
| AF-P1-01 | Real OWNER browser reached `/owner/dashboard`, `/owner/bookings`, and `/owner/finance`; visible dashboard metrics, booking list, finance summary, venue selector, and transaction tabs rendered. Owner API returned `200` for metrics, bookings, finance summary/transactions, and all four analytics endpoints. Frontend owner/platform contract tests passed (`32/32`); `npm run build` and `npm run lint` passed. | PASS |

## AF-P0-08 exact assertions

The disposable integration gate passed these cases:

- Draft create idempotency and conflict detection.
- Cancel/approve idempotency.
- Post and void exact reversal, balanced entries, and OPEX summary `125000 → -125000 → 0`.
- Timeout-after-commit replay returns the same transition and does not duplicate journals.
- Concurrent same-key post/void produces a single transition.

The targeted frontend suite passed 32 tests, including double-click protection, timeout replay, and response-parse ambiguity for expense mutations.

## Evidence files

Sanitized raw logs are retained outside the repository at:

`D:\project\lapangGo_task407_evidence_20260722\`

Files:

- `AF-P0-08-expense.log`
- `AF-P0-09-audit.log`
- `AF-P0-10-auth-contract.log`
- `AF-P0-10-audit-auth.log`
- `AF-P0-10-real-api-auth.json`
- `AF-P0-10-real-admin-auth.json`
- `AF-P1-01-owner-api.json`
- `AF-P0-08-P1-01-frontend-contract.log`
- `AF-P1-01-web-build.log`
- `AF-P1-01-web-lint.log`

The logs contain no password, JWT, DSN, provider secret, or PII output.

SHA-256: `AF-P0-08-expense.log=2018E859DBA9C34FF9CCBEB1F2407C7E6E3F1C332748FDC0136E07E8381CAC13`; `AF-P0-09-audit.log=C51D788F0AD610D5A4A5BDD8A5F9746E5C4790F09160F69E8B935EEFFC1DDE59`; `AF-P0-10-auth-contract.log=522EA6FAE9CE52417579C0AC256EC6FC99BA00ED2772DE08E81BD0FF93B0E44D`; `AF-P0-10-audit-auth.log=7FB77E8F935368167372252A0A760BDE09D67C1D10A099559F8EC49CD2C17965`; `AF-P0-10-real-api-auth.json=B46F46898ACD706F210DB070FCD4030B7144584A1F9FED46C5C7FF9AF38A7D0C`; `AF-P0-10-real-admin-auth.json=EE87B97ECCA7598DE69B8C1B2B9D013CD9F4E012777E4BD3D2685A2FBA6DEBFA`; `AF-P1-01-owner-api.json=4CBD1E02D12D0D498FCC90C0AF5148044D620D05762B0AEB932EB0AEEBED3CD1`; `AF-P0-08-P1-01-frontend-contract.log=E2CBEC47E71961F7881783C433367EF16F66E8F58CCD94E1454E3CAC4550EF18`; `AF-P1-01-web-build.log=E7E44EA6DEC8BDA132DA671502D83C94E2EA02ECEE0F1E544CA26D058D47A3DC`; `AF-P1-01-web-lint.log=FE51A8BF405D1F819090EF559AAAA19BFD7AD3B1783B603D8E8DECDA8F24143A`.

## Cleanup

The disposable project was stopped with:

```powershell
docker compose -p lapangango-task407 -f docker-compose.yml -f docker-compose.task-4-05.yml down -v --remove-orphans --rmi local
```

Post-cleanup verification returned zero `lapangango_task407_*` containers, volumes, networks, and listeners on ports `15432/16379/18080/13000`. The persistent local stack is out of scope and was not stopped.
