# Task 4-00 — Release Baseline dan Evidence Matrix

Tanggal evidence: 2026-07-18 (Asia/Jakarta)

## Scope dan keputusan

Task ini membekukan baseline v1.7 sebelum reconciliation core dibangun. Tidak ada backend, frontend, migration, feature flag, atau financial fact yang diubah.

Diagnostics v1.7 diputuskan **read-only CLI first**:

```text
apps/api/cmd/reconcile-platform-finance
```

HTTP endpoint reconciliation belum disetujui dan tidak dibuat pada Task 4-00. Core check akan menjadi reusable pada Task 4-01 dan CLI dieksekusi pada Task 4-02.

## Baseline branch/commit/status

| Item | Evidence aktual |
|---|---|
| Branch | `master` |
| Application commit | `87017f7767ae60576be5d5b923c818256aab0f49` |
| Commit message | `fix(finance): close phase 3b final gate blockers` |
| Upstream divergence | `master` ahead of `origin/master` by 15 commits after the Task 4-00 finalization commit |
| Worktree | Dirty only in `apps/web/src/components/MabarSection.tsx` dan `apps/web/src/components/VenueSection.tsx`; both are user-owned and outside Phase 4 finance scope |
| In-scope Phase 3B files | Clean against `HEAD` |
| Dependency/migration diff | None against `HEAD` |
| Evidence artifact | Tracked by the Task 4-00 finalization commit containing this file |
| `git diff --check` | Pass; only existing LF/CRLF normalization warnings |

Downstream evidence must reference the application commit above or an explicitly newer commit that preserves this baseline. User-owned changes must not be silently included in a release evidence claim.

## Runtime and dependency baseline

| Surface | Evidence |
|---|---|
| Go | `go1.26.4 windows/amd64` |
| Node | `v24.16.0` |
| npm | `11.13.0` |
| Docker services | `api`, `postgres`, `redis`, `web` running |
| Go modules | `go mod verify` → `all modules verified` |
| Frontend dependency tree | `npm ls --depth=0` passed during Phase 3B final gate |

No provider secret, production credential, token, PAN, CVV, or bank credential is included in this matrix.

## Database baseline

Read-only queries against the local PostgreSQL instance returned:

```text
schema_migrations: 24|false
expense_disposable_dbs: 0
invalid_expense_state_rows: 0
incomplete_or_unbalanced_journals: 0
```

Migration inventory at the release baseline ends at:

```text
019_platform_audit_and_commercial_terms
020_booking_fee_snapshots
021_platform_finance_cutover_guard
022_platform_double_entry_ledger
023_platform_ledger_balance_reschedule
024_platform_expenses
```

Task 4-00 performs no migration up/down, backfill, fixture mutation, or cleanup of application facts. Destructive and disposable migration checks are reserved for later Phase 4 tasks.

## Feature flag and safety matrix

| Control | Current evidence | Status/owner |
|---|---|---|
| Commercial term `finance_mode` | DTO/service require `SIMULATION`; service rejects `LIVE` and records `LIVE_NOT_ALLOWED` audit evidence | Frozen; verify again in 4-05/4-07 |
| Frontend finance mode | UI contract uses `SIMULATION` and labels actual metrics unavailable | Frozen; verify in 4-06/4-08 |
| `PLATFORM_MONETIZATION_ENABLED` | No runtime config key found in current branch | Explained gap owned by 4-04; do not invent a value in 4-00 |
| OPEX mutation disable | No independent feature flag/routing guard frozen yet | 4-04 owner |
| Email delivery | `EMAIL_DELIVERY_ENABLED=true` in local Compose; unrelated to finance release claims | Record only; no secret value in evidence |
| LIVE payment/provider activation | No activation permitted by Phase 4 guardrails | Must remain disabled |

The missing monetization flag is not silently treated as `false`; Task 4-04 must prove an explicit default-false startup guard before the v1.7 final gate.

## Endpoint and route inventory

All platform-finance groups use the production middleware chain:

```text
JWT/Auth → RequireActiveUser → RequireRole("SUPER_ADMIN")
```

| Method | Route | Kind | Source |
|---|---|---|---|
| GET | `/admin/finance/summary` | Read-only summary | `internal/platformfinance/handler.go:88-92` |
| GET | `/admin/finance/breakdown` | Read-only scoped breakdown | `internal/platformfinance/handler.go:88-92` |
| GET | `/admin/finance/expenses` | Read-only expense list | `internal/platformfinance/expense_handler.go:264-272` |
| POST | `/admin/finance/expenses` | Create DRAFT expense | `internal/platformfinance/expense_handler.go:264-272` |
| POST | `/admin/finance/expenses/:id/cancel` | Cancel transition | `internal/platformfinance/expense_handler.go:264-272` |
| POST | `/admin/finance/expenses/:id/approve` | Approve transition | `internal/platformfinance/expense_handler.go:264-272` |
| POST | `/admin/finance/expenses/:id/post` | Post and journal mutation | `internal/platformfinance/expense_handler.go:264-272` |
| POST | `/admin/finance/expenses/:id/void` | Exact reversal mutation | `internal/platformfinance/expense_handler.go:264-272` |
| GET | `/admin/finance/journals` | Read-only journal list | `internal/platformfinance/expense_handler.go:264-272` |
| GET | `/admin/commercial-terms` | Simulation term read surface | `internal/commercialterms/handler.go:138-145` |
| POST | `/admin/commercial-terms/preview` | Simulation term preview | `internal/commercialterms/handler.go:138-145` |
| POST | `/admin/commercial-terms` | Simulation term create | `internal/commercialterms/handler.go:138-145` |

No reconciliation HTTP endpoint exists in the baseline route inventory.

Frontend consumers are `/admin/finance` and `/admin/finance/expenses` in `apps/web/src/App.tsx:128-129`. The following dependent surfaces are included because they feed AF-P0/P1 scenarios:

Customer rows use `JWT/Auth → RequireActiveUser → CUSTOMER`; owner rows use `JWT/Auth → RequireActiveUser → owner workspace → RequireOwnerPermission(...)`.

| Method | Route | Kind/auth boundary | Source |
|---|---|---|---|
| GET | `/admin/audit-logs` | Read-only, `JWT → RequireActiveUser → SUPER_ADMIN` | `internal/admin/handler.go:86-96` |
| GET | `/owner/finance/summary` | Read-only, owner workspace + `FINANCE_READ` | `internal/finance/handler.go:213-218` |
| GET | `/owner/finance/transactions` | Read-only, owner workspace + `FINANCE_READ` | `internal/finance/handler.go:213-221` |
| POST | `/owner/finance/transactions` | Owner workspace + `FINANCE_WRITE` | `internal/finance/handler.go:213-221` |
| PATCH | `/owner/finance/transactions/:id` | Owner workspace + `FINANCE_WRITE` | `internal/finance/handler.go:213-221` |
| DELETE | `/owner/finance/transactions/:id` | Owner workspace + `FINANCE_WRITE` | `internal/finance/handler.go:213-221` |
| GET | `/owner/analytics/bookings` | Owner workspace + `ANALYTICS_READ` | `internal/analytics/router.go:14-20` |
| GET | `/owner/analytics/revenue` | Owner workspace + `ANALYTICS_READ` | `internal/analytics/router.go:14-20` |
| GET | `/owner/analytics/status` | Owner workspace + `ANALYTICS_READ` | `internal/analytics/router.go:14-20` |
| GET | `/owner/analytics/expenses` | Owner workspace + `ANALYTICS_READ` | `internal/analytics/router.go:14-20` |
| POST | `/bookings` | Customer booking create, `JWT → RequireActiveUser → CUSTOMER` | `internal/bookings/handler.go:23-29` |
| GET | `/bookings` | Customer booking list, `JWT → RequireActiveUser → CUSTOMER` | `internal/bookings/handler.go:23-29` |
| GET | `/bookings/:id` | Customer booking read, `JWT → RequireActiveUser → CUSTOMER` | `internal/bookings/handler.go:23-29` |
| PATCH | `/bookings/:id/cancel` | Customer booking cancel, `JWT → RequireActiveUser → CUSTOMER` | `internal/bookings/handler.go:23-29` |
| POST | `/bookings/:id/pay` | Customer payment confirmation, `JWT → RequireActiveUser → CUSTOMER` | `internal/bookings/handler.go:23-29` |
| POST | `/bookings/:id/payment-proof` | Customer payment proof, `JWT → RequireActiveUser → CUSTOMER` | `internal/bookings/handler.go:23-29` |
| GET | `/owner/bookings` | Owner booking read + `BOOKINGS_READ` | `internal/bookings/handler.go:33-41` |
| GET | `/owner/venues/:id/bookings` | Owner venue booking read + `BOOKINGS_READ` | `internal/bookings/handler.go:33-41` |
| PATCH | `/owner/bookings/:id/verify-payment` | Owner payment verification + `PAYMENT_VERIFY` | `internal/bookings/handler.go:33-41` |
| PATCH | `/owner/bookings/:id/mark-paid` | Owner payment marking + `PAYMENT_VERIFY` | `internal/bookings/handler.go:33-41` |
| PATCH | `/owner/bookings/:id/complete` | Owner booking completion + `BOOKINGS_WRITE` | `internal/bookings/handler.go:33-41` |
| PATCH | `/owner/bookings/:id/cancel-refund` | Owner full refund mutation + `BOOKINGS_WRITE` | `internal/bookings/handler.go:33-41` |
| POST | `/owner/bookings/offline` | Owner offline booking + `OFFLINE_BOOKINGS_CREATE` | `internal/bookings/handler.go:33-41` |
| GET | `/owner/metrics` | Owner booking metrics + `BOOKINGS_READ` | `internal/bookings/handler.go:33-41` |
| POST | `/bookings/:id/refund-request` | Customer refund request, `JWT → RequireActiveUser → CUSTOMER` | `internal/refunds/handler.go:25-28` |
| GET | `/bookings/:id/refund-request` | Customer refund read, `JWT → RequireActiveUser → CUSTOMER` | `internal/refunds/handler.go:25-28` |
| GET | `/owner/refund-requests` | Owner workspace + `REFUNDS_READ` | `internal/refunds/handler.go:30-32` |
| PATCH | `/owner/refund-requests/:id/approve` | Owner refund approval + `REFUNDS_WRITE` | `internal/refunds/handler.go:30-34` |
| PATCH | `/owner/refund-requests/:id/reject` | Owner refund rejection + `REFUNDS_WRITE` | `internal/refunds/handler.go:30-34` |

## Source and formula map

| Contract | Source | Formula/invariant | Exact evidence |
|---|---|---|---|
| Online GMV | `owner_finance_transactions` `INCOME/BOOKING`; joins `bookings → courts → venues → owner_profiles`; excludes `offline_booking_customers` | Canonical booking predicate, `created_at >= start AND < end`, exact `BIGINT` sum | `internal/platformfinance/projection_query.go:321-375`, `repository.go:285-297` |
| Paid booking source check | `bookings`, `courts`, `venues`, `owner_profiles`, `owner_finance_transactions` | Paid/completed booking must have matching owner/venue ledger income | `internal/platformfinance/repository.go:326-349` |
| Snapshot/projection basis | `booking_fee_snapshots` joined to booking ledger rows | `terms_source`, `booking_channel`, `finance_mode`, snapshot commission/final price and term ID are validated before projection | `internal/platformfinance/projection_query.go:321-375` |
| Offline exclusion | `offline_booking_customers` | Offline facts do not enter online GMV/commission; offline rate is zero | `internal/platformfinance/repository.go:285-320`, `bookings/handler.go:33-41` |
| Refund reversal | `owner_finance_transactions` `EXPENSE/REFUND` joined to original `INCOME/BOOKING` | Full refund amount must equal original amount; commission reversal uses the matched original | `internal/platformfinance/projection_query.go:380-430`, `repository.go:302-321` |
| Duplicate/data quality guards | `owner_finance_transactions`, `bookings` | Duplicate income, fractional amount, orphan refund, refund mismatch, paid-without-ledger and ledger-without-booking fail closed | `internal/platformfinance/repository.go:200-279`, `326-349` |
| Net GMV | Validated gross and refund aggregates | `gross GMV - refund principal` | `internal/platformfinance/service.go:48` |
| Net projected commission | Validated gross and refund commission aggregates | `projected commission gross - exact refunded commission` | `internal/platformfinance/service.go:49` |
| OPEX | `platform_expenses`, posted/reversal `platform_journals`, `platform_ledger_entries` | OPEX debit positive; VOID OPEX credit negative; global filters do not allocate to owner/venue | `internal/platformfinance/projection_query.go:642-675` |
| Projected operating result | Net projected commission and OPEX | `net projected commission - platform OPEX` | `internal/platformfinance/service.go:100` |
| Trend | Income/refund/OPEX daily buckets | Continuous Jakarta calendar buckets; total must reconcile to summary | `internal/platformfinance/service.go:127`, `255-410` |
| Actual metrics | API DTO/service and UI contract | Actual revenue/contribution/result stay unavailable in simulation; never fake cash as Rp0 | `internal/platformfinance/service.go:109`, `apps/web/src/pages/admin/AdminPlatformFinancePage.tsx` |
| Money contract | Ledger migration, Go service, API DTOs | PostgreSQL `BIGINT`, Go `int64`, API integer-rupiah strings; no float arithmetic | `db/migrations/022_platform_double_entry_ledger.up.sql` |
| Time contract | `apps/api/internal/platformfinance/helpers.go:33-68`; `contract_test.go:TestParseAndValidateDatesEnforcesInclusive366DayLimit`; `apps/web/src/lib/platformExpenseForm.ts` | Asia/Jakarta display; start-inclusive/end-exclusive range; current report helper enforces max 366 days and 4-01 must reuse the same contract | Exact helper/contract/timezone tests |
| Scoped OPEX | Repository/service/UI contract tests | Global OPEX is not presented as owner/venue allocated OPEX | `internal/platformfinance/contract_test.go:208-257` |

## AF-P0/P1 evidence ownership matrix

Each row now has an invariant/source, executable evidence pointer, manual owner/task, current status, and blocker reference. `PENDING` means the owning Phase 4 task has not executed yet; it is not treated as PASS.

| ID | Invariant/source | Automated evidence | Manual owner/task | Status | Blocker ref |
|---|---|---|---|---|---|
| AF-P0-01 | Online booking without promo; snapshot and GMV | `apps/api/internal/bookings/service_online_integration_test.go:TestOnlineBookingIntegration`; `apps/api/internal/platformfinance/projection_query_test.go:TestServiceProjectionSourceContract` | Booking/snapshot QA — 4-06 | Integration skipped (`TEST_INTEGRATION`/DSN); manual pending | 4-06 |
| AF-P0-02 | Promo uses final-price basis | `apps/api/internal/platformfinance/projection_query_test.go:TestClassifyProjectionSourceAndRupiahMatrix`; `apps/api/internal/platformfinance/projection_fixture_test.go:TestProjectionReadModel_HistoricalSnapshotMixedAndRefund` | Booking/snapshot QA — 4-06 | Unit contract PASS; DB fixture skipped; manual pending | 4-06 |
| AF-P0-03 | Offline appears in owner finance, not online GMV/commission | `apps/api/internal/bookings/service_offline_integration_test.go:TestOfflineBookingIntegration`; `apps/api/internal/platformfinance/projection_query_test.go:TestClassifyProjectionSourceAndRupiahMatrix` | Booking/snapshot QA — 4-06 | Integration skipped (`TEST_INTEGRATION`/DSN); manual pending | 4-06 |
| AF-P0-04 | Payment verification retry creates no duplicate fact | `apps/api/internal/bookings/service_retry_matrix_integration_test.go:TestBookingRetryConcurrencyRollbackMatrix` | Booking/payment regression — 4-06 | Disposable integration skipped (`TEST_BOOKING_MATRIX_DISPOSABLE`/DSN); manual pending | 4-06 |
| AF-P0-05 | Full refund keeps owner ledger correct and reverses projected commission exactly | `apps/api/internal/bookings/repository.go:708-844`; `apps/api/internal/platformfinance/projection_query.go:380-430`; `service_test.go:TestCancelPaidBookingWithRefund_Success` only covers orchestration status | Ledger/reconciliation QA — 4-03/4-06 | Unit orchestration PASS; DB ledger/projection fixture pending | 4-03 |
| AF-P0-06 | Rate 0/5/7 and historical non-billable data | `apps/api/internal/platformfinance/projection_query_test.go:TestClassifyProjectionSourceAndRupiahMatrix`; `apps/api/internal/platformfinance/post_cutover_detector_test.go:TestClassifyPostCutoverP0Candidate_OnlinePriceRules`; `TestClassifyPostCutoverP0Candidate_OfflinePriceRules`; `apps/api/internal/platformfinance/projection_fixture_test.go:TestProjectionReadModel_HistoricalSnapshotMixedAndRefund` | Projection QA — 4-03/4-06 | Unit contract PASS; DB fixture skipped; manual pending | 4-03 |
| AF-P0-07 | Term change does not mutate old booking | `apps/api/internal/platformfinance/booking_fee_snapshot_repository_test.go:TestBookingFeeSnapshotRepository_E_TermChangeImmutable` | Snapshot immutability — 4-06 | Integration skipped (`TEST_INTEGRATION`/DSN); disposable/manual pending | 4-06 |
| AF-P0-08 | OPEX create/retry/post/void is balanced and exact | `apps/api/internal/platformfinance/expense_service_integration_test.go:TestExpenseServicePostAndVoidAreAtomicExactAndIdempotent`; `apps/api/internal/platformfinance/expense_service_integration_test.go:TestExpenseServicePostAndVoidTimeoutAfterCommitReplay`; `apps/web/src/__tests__/platformExpenseWorkflow.test.tsx` | Platform finance QA — 4-03/4-07 | Frontend contract PASS; DB integration skipped (`TEST_EXPENSE_DISPOSABLE`/DSN); manual pending | 4-07 |
| AF-P0-09 | Journal and audit mutation are atomic | `apps/api/internal/platformfinance/journal_audit_integration_test.go:TestAuditedJournalReversalAuditFailureRollsBackJournal`; `apps/api/internal/audit/platform_contract_test.go:TestPlatformFinanceAuditContract` validates metadata only | Audit/ledger QA — 4-07 | Audit validation PASS; atomic DB integration skipped (`TEST_LEDGER_DISPOSABLE`/DSN); manual pending | 4-07 |
| AF-P0-10 | Customer/owner/staff denied admin finance | `apps/api/internal/platformfinance/handler_integration_test.go:TestHandler_Integration_UsesProductionAuthChain`; `apps/api/internal/platformfinance/expense_handler_auth_test.go:TestExpenseMutationRoutesUseProductionAuthChain`; `apps/api/internal/admin/audit_read_test.go:TestAdminAuditLogsAuthMatrix` | Auth QA — 4-07 | Production auth contract PASS; full matrix/manual pending | 4-07 |
| AF-P1-01 | Existing owner dashboard/finance has no regression | `apps/api/internal/finance/handler.go:213-221`; `apps/api/internal/analytics/router.go:14-20`; no dedicated cross-flow test at baseline | Owner regression QA — 4-07 | Pending dedicated cross-flow evidence | 4-07 |
| AF-P0-11 | Jakarta date boundary is consistent | `apps/api/internal/platformfinance/contract_test.go:TestParseAndValidateDatesEnforcesInclusive366DayLimit`; `apps/api/internal/platformfinance/helpers.go:33-68`; `apps/web/src/__tests__/platformExpenseWorkflow.test.tsx:it("renders accounting timestamps in Jakarta time regardless of browser timezone")` | Time/reconciliation QA — 4-03/4-08 | Date contract PASS; boundary/manual pending | 4-08 |
| AF-P0-12 | Reconciliation summary/breakdown/trend/ledger unexplained difference is Rp0 | No reconciliation core/CLI exists at baseline | Reconciliation QA — 4-01/4-02/4-03/4-08 | Pending by design | 4-01 |
| AF-P1-02 | Loading/empty/error/stale/rapid-filter/mobile | `apps/web/src/__tests__/platformFinanceSummary.test.tsx`; `platformExpenseWorkflow.test.tsx`; `apps/web/scripts/test_platform_finance_responsive.mjs` | Frontend QA — 4-08 | Automated mocked-API/browser PASS; manual pending | 4-08 |

No personal owner names are invented in this matrix. If release governance requires named individuals, the owner field must be completed before the corresponding task is declared ready.

## Baseline automated evidence

Executed from an archive containing exactly application `HEAD` `87017f7`; the dirty user files were not present in this archive. Go ran in a read-only container mount and frontend reused the existing dependency tree through a temporary junction:

```text
go test -count=1 ./...                         PASS
npm test -- --run                              PASS (32 tests)
npm run lint                                   PASS
npm run build                                  PASS
go mod verify                                  PASS
scripts/smoke_test.ps1                         PASS (/health, /db-health, /venues)
npm ls --depth=0                                PASS
```

Browser evidence includes responsive finance summary and expense/journal workflows at 360px and 1440px. The Vite build emits a non-blocking large-chunk warning; the build exits successfully. The temporary exact-HEAD archive was removed after verification.

The broad `go test` PASS includes integration tests that intentionally reported `SKIP` because disposable databases and integration DSNs were not enabled. Those skips are explicit in the AF matrix and are not counted as proof of database-backed P0 behavior. No disposable migration, fixture, backfill, or cleanup was performed in Task 4-00.

## Stop conditions for later Phase 4 tasks

The following are intentionally carried as explicit owners, not hidden limitations:

- Explicit `PLATFORM_MONETIZATION_ENABLED=false` startup/feature guard → 4-04.
- Reconciliation core and exact daily exception buckets → 4-01.
- Sanitized deterministic CLI with nonzero exception exit and zero-write proof → 4-02.
- Actual DB fixtures for clean, boundary, duplicate, missing, refund, and OPEX reversal cases → 4-03.
- Manual AF-P0/P1 evidence → 4-06 through 4-08.

Any unexplained difference in source/formula, auth, money, idempotency, journal balance, or reconciliation is a blocker and must stop the next handoff.

## Finalization gate

The evidence artifact is tracked by the Task 4-00 finalization commit containing this file. The downstream handoff is valid only when `git show HEAD:docs/task_4-00_release_baseline_evidence_matrix.md` succeeds and no Task 4-00 change remains staged. The two unrelated user-owned frontend files remain outside this artifact commit.

## Required handoff

```text
Task: 4-00 — Release Baseline dan Evidence Matrix
Baseline branch/commit/status: master / 87017f7767ae60576be5d5b923c818256aab0f49 / two explained out-of-scope user files remain dirty
Objective: Freeze v1.7 release baseline, source/formula map, AF ownership, and diagnostics decision
Files changed: docs/task_4-00_release_baseline_evidence_matrix.md
Behavior implemented: No runtime behavior; read-only evidence artifact only
Invariant proved: Migration 24 clean, no invalid expense state, no incomplete/unbalanced journal, current automated baseline green
Commands: Full backend, frontend test/lint/build, go mod verify, GET-only smoke, PostgreSQL SELECT checks
Actual result: All commands passed; disposable database count is 0
Skipped/unverified: Disposable DB/integration suites are intentionally pending for 4-03/4-06/4-07; explicit monetization flag and reconciliation CLI are owned by 4-04 and 4-02
Risks/blockers: No Task 4-00 blocker; user-owned unrelated worktree files remain outside the artifact commit; monetization flag and reconciliation core are explicitly assigned to 4-04/4-01
Commit: Task 4-00 finalization commit containing this artifact; resolve exact hash with git rev-parse HEAD
READY FOR 4-01
```
