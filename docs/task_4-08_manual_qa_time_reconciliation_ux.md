# Task 4-08 — Manual QA Time/Reconciliation/UX Evidence

## Verdict

`READY FOR 4-09`

AF-P0-11, AF-P0-12, and AF-P1-02 were executed against disposable fixtures and
the disposable Compose stack. No P0/P1 failure was waived. The persistent local
Compose stack and user-owned files were not modified.

## Scope and environment

- Disposable Compose project: `lapangango-task408`.
- PostgreSQL/API/web ports: `15432`, `18080`, and `13000`; Redis `16379`.
- Migration state before cleanup: `schema_migrations=24|false`.
- Feature flags in the disposable stack: `PLATFORM_MONETIZATION_ENABLED=false` and
  `PLATFORM_FINANCE_ADMIN_ENABLED=true`.
- Database facts observed before cleanup: `bookings=66`; all finance fact tables
  used by the reconciliation checks were `0`. Boundary fixtures were created in
  rolled-back transactions/temp tables, so they did not alter those facts.
- No LIVE DSN, provider credential, JWT, password, or PII was stored in evidence.

## Scenario matrix

| ID | Executed scenario and exact evidence | Result |
|---|---|---|
| AF-P0-11 | Jakarta half-open boundary: `16:59:59Z` remains in the prior Jakarta date, `17:00:00Z` enters the next date; exact end sentinel is excluded. Date parser accepts the inclusive 366-day range and rejects one day beyond it. Repository max-range test also asserts one production breakdown query and exact Jakarta buckets for both endpoints. | PASS |
| AF-P0-12 | Reconciliation repository production-path tests passed: clean read-only snapshot, non-paid missing snapshot dated bucket, independent offsetting/fractional source-ledger faults, one-query 366-day breakdown, and full repository fault path. | PASS |
| AF-P0-12 | `TestReconciliationBoundarySuite` passed all 16 subtests, including clean 0/5/7/legacy commission matrix, exact refund reversal, OPEX post/void across a Jakarta month boundary, missing/duplicate/orphan/mismatch faults, Jakarta midnight, half-open exclusion, and rollback isolation. Clean fixtures assert all eight checks PASS and `Clean=true`; fault fixtures assert exact exception buckets/deltas. | PASS |
| AF-P0-12 | CLI clean/fault integration passed on a clone of the disposable database. Clean exits `0`; injected missing-snapshot fault exits `1`, reports `EXCEPTIONS` with the exact Jakarta `bucket_date`, emits no raw database error, and preserves before/after booking/snapshot counts. | PASS |
| AF-P0-12 | Direct read-only CLI run for `2030-01-10` emitted JSON `version=1`, `timezone=Asia/Jakarta`, `status=CLEAN`, `clean=true`, eight checks, and zero count/rupiah differences; stderr was empty. | PASS |
| AF-P1-02 | Full frontend gate passed: real login-form interaction in the browser harness, desktop and mobile viewports, finance summary → expenses → journals → posted/reversal journal focus, no horizontal overflow, no page/console errors, and Vite cleanup. | PASS |
| AF-P1-02 | Vitest state matrix passed 39/39 across five files: loading/empty/error, stale indicator, failed-filter clearing, rapid-filter newest-response-wins, URL Back/Forward, owner/venue option retry, Jakarta timestamp rendering, mobile reversal links, and feature-flag route behavior. | PASS |

The browser harness uses deterministic intercepted auth/API fixtures to make the
workflow repeatable; the disposable API/DB health probes were run separately and
returned HTTP 200 for `/health`, `/db-health`, `/venues`, and web `/login`.

The broad unit log contains 18 PASS and 6 opt-in integration SKIP entries. Those
six database branches are not counted as evidence from the unit run; every one
was rerun with an explicit disposable DSN in `AF-P0-11-12-reconciliation-rerun.log`
and passed. The boundary and CLI logs contain no skipped required scenario.

## Commands executed

Backend reconciliation and boundary gates (DSN redacted in this report):

```text
TEST_RECONCILIATION_INTEGRATION=1 TEST_DATABASE_URL=<disposable-dsn> \
  go test -count=1 ./internal/platformfinance -run '^(TestParseAndValidateDatesEnforcesInclusive366DayLimit|TestReconciliationRepositoryCleanReadOnlySnapshot|TestReconciliationMissingSnapshotPreflightIncludesNonPaidPostCutover|TestReconciliationSourceLedgerPreflightRejectsOffsettingAndFractionalRows|TestReconciliationMaximumRangeBreakdownUsesOneQueryAndJakartaDates|TestReconciliationFullRepositoryPathRejectsOffsettingAndFractionalSource)$' -v

TEST_INTEGRATION=1 TEST_DATABASE_URL=<disposable-dsn> \
  go test -count=1 ./internal/platformfinance -run '^TestReconciliationBoundarySuite$' -v

TEST_INTEGRATION=1 RECONCILIATION_CLI_TEST_DATABASE_URL=<disposable-dsn> \
  go test -count=1 ./cmd/reconcile-platform-finance -run '^TestCLIReconciliation(Clean|Fault)Integration$' -v
```

Frontend gates:

```text
npm test
npm run build
npm run lint
```

The CLI clone test temporarily stopped only the disposable API container to
release the template database connection, then started it again and verified it
healthy before cleanup.

The first CLI rerun was intentionally rejected by PostgreSQL with
`source database ... is being accessed by other users` because the disposable
API still held a connection. That harness-only attempt is superseded by the
serialized `AF-P0-12-cli-rerun2.log` run after stopping the disposable API;
clean and fault cases then both passed. No application failure was waived.

## Evidence artifacts and SHA-256

Raw sanitized artifacts are retained outside the repository at:

`D:\project\lapangGo_task408_evidence_20260722\`

Authoritative rerun artifacts:

```text
AF-P0-11-12-backend-unit.log=F2F08FD81193C7A641D0672CF977ADABE9D607DDAF73112E72C37BC124339521
AF-P0-11-12-reconciliation-rerun.log=0EFA98E04FCFC75E1AD3745F59C49D9D2A692E73C086538C36CA3D19496350AE
AF-P0-12-boundary-suite-rerun.log=AF7DBE4ED79D873B58BCCF65AAC1F934C245EF79BC2871156CFF2BC091431A1A
AF-P0-12-cli-rerun2.log=21FFF24A1CD42A153525FCFDDB0EC73B7E1DD9D594E8FBCE394E7F270338798E
AF-P0-12-cli-direct-rerun.stdout.json=A3EB65688360BE4FC45091D38E48FEA147D8B2FA9E3F5CD459B8E0076DE266BE
AF-P0-12-cli-direct-rerun.stderr.log=E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855
AF-P1-02-frontend-suite-rerun.log=DEDE9C369A7908DB1F0AF7A66BA6B8B76B0225C2CF8F7DA975E79CAB078D6FD3
AF-P1-02-web-build-rerun.log=0ED71D1D35F218465FB3B6FAA562CE2A19D5D27A872E17E4E2811C5C9E756032
AF-P1-02-web-lint-rerun.log=FE51A8BF405D1F819090EF559AAAA19BFD7AD3B1783B603D8E8DECDA8F24143A
AF-P0-11-12-environment-rerun.log=B4F55F2E010B5FB4ED8887093C5E2C96C1274C2C957D50D74A5138C4ADCCC34A
cleanup-rerun.log=9EF5A8D1CECE786444A5F646EA0697A571384ED40B67340437642831A1F91FAF
```

A credential scan over all files in the evidence directory returned zero hits
for DSNs, passwords, JWT/Bearer tokens, or provider secrets.

## Cleanup and residue proof

The disposable project was removed with:

```text
docker compose -p lapangango-task408 -f docker-compose.yml -f docker-compose.task-4-05.yml down -v --remove-orphans --rmi local
```

Post-cleanup verification recorded `containers=0`, `volumes=0`,
`networks=0`, and zero listeners on ports `15432`, `16379`, `18080`, `13000`,
`11025`, `18025`, and `4173`. The persistent local Compose project was not
stopped or repaired.
