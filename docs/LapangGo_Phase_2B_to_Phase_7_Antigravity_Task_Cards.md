# LapangGo Phase 2B sampai Phase 7 — Antigravity Task Cards

Status: **execution breakdown approved for planning; implement one task at a time**

Source of truth bisnis dan accounting:

- `docs/version_1_7_platform_finance_implementation_plan.md`

Baseline penyelesaian:

- Phase 0 sampai Phase 2A: **DONE**
- Final Phase 2A commit: `a7d2b0dfeb33c6eac8d3507e301519f316145cd4`
- Next task: **2B-00**
- Total task IDs: **188**, termasuk audit, final gate, conditional blocker fixes, dan human-only operations.

Dokumen ini memecah sisa pekerjaan menjadi unit kecil untuk Antigravity. Dokumen ini tidak mengubah keputusan bisnis pada source plan dan tidak memberi izin kepada AI untuk mengaktifkan production payment, refund dispatch, payout, commercial term LIVE, atau rollout owner.

---

## 1. Cara Menggunakan Task Cards

1. Berikan Antigravity tepat satu task ID per percakapan.
2. Awali dengan: `Ikuti AGENTS.md, .agents/agent_workflow.md, dan .agents/definition_of_done.md.`
3. Untuk implementation task, Antigravity wajib membuat plan kecil dan acceptance criteria sebelum coding.
4. Jangan melanjutkan task berikutnya sebelum reviewer memberi GO.
5. Audit task bersifat read-only. Temuan diperbaiki hanya dalam task `FIX` yang sesuai.
6. Evidence task tidak boleh sekaligus memperbaiki kode.
7. Gunakan database disposable untuk migration, destructive fixture, concurrency, dan backfill tests.
8. Phase 2B–5 tetap `SIMULATION`. Phase 6 tetap sandbox/manual approval. Phase 7B memerlukan aktivasi eksplisit manusia.

Format handoff wajib:

```text
Task:
Baseline branch/commit/status:
Objective:
Files changed:
Behavior implemented:
Invariant proved:
Commands:
Actual result:
Skipped/unverified:
Risks/blockers:
Commit:
READY FOR <next task>
```

Untuk audit:

```text
NO BLOCKER
```

atau:

```text
BLOCKERS FOUND
Severity:
Exact file/evidence:
Actual behavior:
Expected behavior:
Required fix and regression test:
```

---

## 2. Aturan Pemilihan Model

| Jenis pekerjaan | Pelaksana | Reviewer |
|---|---|---|
| Preflight, inventory, dokumentasi, read-only UI | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| API, authorization, filter, UI action/integration | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| Migration, money calculation, ledger, transaction, idempotency, concurrency, refund, payment, payout | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| Independent audit | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| Exact blocker fix | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 finance selalu Codex Sol High |
| Final evidence/gate | Antigravity mengumpulkan evidence | Codex Sol High final gate |

Nama model di atas mengikuti pilihan pada workflow pengguna. Jika nama model berubah di produk, pilih kelas reasoning yang ekuivalen; jangan menurunkan kelas reviewer untuk task uang, auth, concurrency, atau production operations.

---

## 3. Guardrails Global Phase 2B–7

- Money domain baru: PostgreSQL `BIGINT`, Go `int64`, API integer-rupiah string; dilarang `float64`.
- Semua financial facts append-only/immutable; koreksi melalui reversal atau explicit adjustment.
- Tidak ada mutation finansial tanpa stable idempotency boundary dan atomic platform audit.
- Tidak ada raw provider payload, secret, token, PAN, CVV, bank credential, atau PII berlebih di log/audit.
- Tidak ada mass backfill di schema migration.
- Tidak ada PATCH/DELETE untuk posted journal, snapshot, captured fact, settlement, atau paid payout.
- Owner cashbook legacy tidak boleh dicampur dengan platform ledger atau marketplace collected facts.
- Browser callback tidak pernah menjadi bukti payment captured.
- AI tidak boleh mengaktifkan kill switch, owner LIVE, provider production call, payout, atau cohort expansion.

---

# Phase 2B — Immutable Booking Fee Snapshot

## Phase 2B1 — Schema, Resolver, Calculator

### Task 2B-00 — Repository Preflight dan Booking-path Inventory

- Objective: membekukan baseline setelah 2A dan memetakan semua jalur yang benar-benar membuat booking.
- Scope: migration berikutnya, online/promo/offline/mabar paths, transaction owner, sumber final price, owner ledger/refund dependencies, fractional-price audit read-only.
- Verification: `go test -count=1 ./...`, repository search seluruh `INSERT bookings`, dan laporan dirty-tree/dependency map.
- Stop: path create tidak teridentifikasi, fractional-price anomaly belum diputuskan, atau baseline gagal.
- Output: `READY FOR 2B1-01`.

### Task 2B1-01 — Snapshot dan Cutover Migration Only

- Objective: migration up/down `booking_fee_snapshots` dan immutable `platform_finance_cutovers` tanpa writer/cutover aktif.
- Invariants: satu snapshot per booking; FK `RESTRICT`; IDR exponent 0; channel/source/mode allowlist; offline dan legacy selalu 0%; arithmetic consistency; no `updated_at`.
- Verification: fresh up/down/up, upgrade, duplicate/NULL/FK/rate/channel/arithmetic rejection, update/delete denial, dan no active cutover row.
- Do not: resolver, service, booking integration, backfill, atau hard-coded cutover timestamp.
- Output: `READY FOR 2B1-02`.

### Task 2B1-02 — Integer-safe Commission Calculator

- Objective: satu pure Go calculator untuk final price, half-up commission, customer charge, dan owner net.
- Invariants: quotient/remainder integer, overflow/underflow checked, signed adjustment memerlukan reason, bps `0..3000`, customer service fee tetap 0.
- Tests: 0/500/700/custom bps, Rp200.000, promo Rp175.000→Rp12.250, nominal kecil, exact `.5`, maximum safe int64, invalid negative/overflow, offline forced zero.
- Do not: DB, resolver, frontend, atau write path.
- Output: `READY FOR 2B1-03`.

### Task 2B1-03 — Effective Commercial-term Resolver

- Objective: resolve owner-specific term sebelum global default pada timestamp booking dengan interval `[)`.
- Tests: global, owner override, exact boundaries, adjacent terms, scheduled/historical exclusion, missing/duplicate integrity fail-closed.
- Do not: create/supersede terms, silent fallback, snapshot write, atau LIVE.
- Output: `READY FOR 2B1-04`.

### Task 2B1-04 — Append-only Snapshot Repository

- Objective: DTO/model dan transaction-aware insert/read repository menggunakan caller `DBTX`.
- Invariants: no nested transaction, no update/delete method, canonical calculation version, POLICY versus LEGACY explicit.
- Tests: exact insert/read, caller rollback removes snapshot, duplicate denied, term change tidak mengubah snapshot lama, money serialization exact.
- Output: `READY FOR 2B1-05`.

### Task 2B1-05 — Independent 2B1 Contract Audit

- Read-only audit schema, calculator, resolver, repository, immutability, migration down, dan bukti bahwa booking paths belum berubah.
- Verdict hanya `NO BLOCKER` atau `BLOCKERS FOUND`.
- Output: `READY FOR 2B1-07` atau `READY FOR 2B1-06`.

### Task 2B1-06 — Exact 2B1 Blocker Fix (Conditional)

- Jalankan hanya bila 2B1-05 menemukan blocker.
- Perbaiki tepat constraint/calculator/resolver/repository yang disebut dan tambahkan satu regression test per blocker.
- Dilarang mulai booking integration/backfill atau refactor umum.
- Output: `READY FOR 2B1-07`.

### Task 2B1-07 — 2B1 Final Regression dan Evidence

- Repeat migration cycle, rounding/overflow/boundary tests, immutability, rollback, full backend, Git/dependency evidence.
- Pastikan tidak ada active cutover dan tidak ada booking path berubah.
- Final verdict: `2B1 GO — 2B2 MAY BE PLANNED` atau fail/insufficient evidence.

## Phase 2B2 — Transactional Snapshot Writes

### Task 2B2-00 — Write-path Re-preflight

- Reconfirm setiap booking insertion path, exact hook, retry behavior, promo/offline classification, dan failure-injection point.
- Read-only; tidak boleh code, cutover, atau backfill.
- Output: path matrix dan `READY FOR 2B2-01`.

### Task 2B2-01 — Shared Snapshot Transaction Orchestrator

- Compose server-owned channel/price, resolver, calculator, booking insert, dan snapshot insert dalam satu existing transaction.
- Tests: resolver/calculator/snapshot failure menggagalkan booking; no orphan; no nested transaction.
- Belum wiring route mana pun.
- Output: `READY FOR 2B2-02`.

### Task 2B2-02 — Marketplace Online Normal dan Promo Paths

- Wire online normal dan promo; basis promo wajib final price setelah diskon.
- Tests: 0/5/7 terms, exact promo basis, retry, duplicate prevention, rollback, owner/customer regression.
- Dilarang mengubah owner cashbook semantics atau offline path.
- Output: `READY FOR 2B2-03`.

### Task 2B2-03 — Owner Walk-in/Offline Path

- Wire owner offline booking sebagai `OWNER_WALK_IN`, selalu 0% walaupun global 7%.
- Preserve signed discount/markup dan reason dari backend authority.
- Tests: auth/owner access, discount/markup, rollback, owner finance unchanged.
- Output: `READY FOR 2B2-04`.

### Task 2B2-04 — Remaining-path Coverage

- Tutup hanya path booking yang ditemukan di 2B2-00, termasuk source booking mabar bila relevan.
- Prove informal participant payment tidak menjadi GMV/snapshot kedua.
- Repository search harus menunjukkan tidak ada uncovered booking insertion.
- Output: `READY FOR 2B2-05`.

### Task 2B2-05 — Cutover Activation Guard dan Procedure

- Implement immutable activation command/procedure: exact timestamp, calculation version, release reference, maintenance/deny-create window, dan snapshot-required guard.
- Production execution tetap memerlukan manusia; task ini hanya membangun dan menguji mekanisme.
- Missing post-cutover snapshot adalah P0; tidak boleh ditandai legacy.
- Output: `READY FOR 2B2-06`.

### Task 2B2-06 — Retry, Concurrency, Rollback Matrix

- Test concurrent creates, retry, timeout-after-commit, resolver/snapshot/commit failure, and post-cutover boundary.
- Expected: one booking→one snapshot; no orphan; stable replay; no duplicate owner ledger.
- Output: `READY FOR 2B2-07`.

### Task 2B2-07 — Independent 2B2 Audit

- Read-only audit seluruh write paths, server authority, cutover semantics, concurrency, owner/customer regressions, dan larangan backfill.
- Output: `READY FOR 2B2-09` atau `READY FOR 2B2-08`.

### Task 2B2-08 — Exact 2B2 Blocker Fix (Conditional)

- Perbaiki hanya blocker 2B2-07 beserta focused regression.
- Dilarang mengerjakan backfill atau improvement tambahan.
- Output: `READY FOR 2B2-09`.

### Task 2B2-09 — 2B2 Final Regression dan Evidence

- Actual DB evidence untuk seluruh write path, cutover, concurrency, rollback, full backend dan smoke bila E2E berubah.
- Final verdict: `2B2 GO — 2B3 MAY BE PLANNED` atau fail/insufficient evidence.

## Phase 2B3 — Safe Legacy Backfill

### Task 2B3-00 — Legacy Data Preflight

- Read-only counts pre/post-cutover, online/offline classification, price source, anomaly list, cursor plan, backup/staging readiness.
- Stop bila cutover mismatch, fractional/invalid price, atau post-cutover missing snapshot belum diinvestigasi.
- Output: reviewed expected counts dan `READY FOR 2B3-01`.

### Task 2B3-01 — Backfill CLI Dry-run Only

- Implement `--dry-run --batch-size --after-booking-id --cutover-at`; zero writes dan no PII logs.
- Validate exact stored cutover and deterministic cursor/counts.
- Belum boleh mengimplementasikan/apply write.
- Output: `READY FOR 2B3-02`.

### Task 2B3-02 — Idempotent Pre-cutover Apply

- Add explicit `--apply`: only `created_at < cutover`, `LEGACY_NO_COMMISSION`, 0%, `legacy-backfill-v1`, transaction per batch, resumable cursor, conflict no-op.
- Dilarang membuat payable/revenue/journal atau menyentuh post-cutover row.
- Tests: approved dry-run count equals apply, rerun writes zero, interruption/resume safe.
- Output: `READY FOR 2B3-03`.

### Task 2B3-03 — Post-cutover P0 Detector/Quarantine

- Detect booking `>= cutover` tanpa snapshot; fail nonzero dan hasilkan sanitized exception report.
- Repair hanya bila original term/channel facts terbukti; tidak boleh memberi legacy waiver.
- Output: `READY FOR 2B3-04` atau `BLOCKED — manual data decision required`.

### Task 2B3-04 — Disposable/Staging Execution Evidence

- Run backup check→dry-run→review→apply→rerun→reconcile pada disposable/staging DB.
- Expected: historical exact zero, one snapshot per booking, post-cutover gap zero, rerun no-op.
- Tidak boleh production apply.
- Output: `READY FOR 2B3-05`.

### Task 2B3-05 — Admin Finance Projection Migration to Snapshot Sources

- Update `platformfinance` repository/service/API DTO and focused tests so post-cutover booking projections use immutable snapshot amounts rather than recalculating the latest rate.
- Preserve historical analytics as a separate 7% non-billable scenario; `LEGACY_NO_COMMISSION` remains 0% for billing/payable/revenue.
- Return correct `projection_basis=HISTORICAL_SCENARIO|BOOKING_SNAPSHOT|MIXED` plus source count/amount fields across summary, breakdown and trend without double counting.
- Tests: snapshot-only, historical-only, mixed range, 0/5/7/promo/offline/refund, term changes, sum consistency, unavailable actual metrics and integer-rupiah serialization.
- Do not create revenue/payable/journal or change UI wording beyond contract synchronization required by existing API types.
- Output: `READY FOR 2B3-06`.

### Task 2B3-06 — Independent Backfill/Projection Audit

- Read-only audit cursor, cutover boundary, dry-run no-write, apply idempotency, PII safety, post-cutover handling, snapshot projection source, historical non-billable scenario and mixed totals.
- Output: `READY FOR 2B3-08` atau `READY FOR 2B3-07`.

### Task 2B3-07 — Exact Backfill/Projection Blocker Fix (Conditional)

- Fix only 2B3-06 blockers; repeat isolated dry-run/apply/reconcile and affected platformfinance regressions.
- Dilarang production apply atau mengubah business rate.
- Output: `READY FOR 2B3-08`.

### Task 2B3-08 — Phase 2B Final Gate

- Repeat isolated migration/backfill/reconciliation, snapshot/historical/mixed projection evidence, focused/full backend tests, Git scope and no-LIVE evidence.
- Final verdict: `PHASE 2B DONE — 2C MAY BE PLANNED` atau fail/insufficient evidence.

---

# Phase 2C — Read-only Terms dan Platform Audit UI

### Task 2C-00 — Read Contract Freeze

- Map existing audit/terms endpoints, types, navigation, `OWNER|PLATFORM|ALL`, pagination, errors, auth, and UI states.
- Read-only; distinguish PLATFORM audit scope from GLOBAL term scope.
- Output: `READY FOR 2C-01`.

### Task 2C-01 — Platform Audit Read API

- Active `SUPER_ADMIN` only; scope/entity/action filters; deterministic pagination; ownerless events; sanitized metadata.
- Tests: complete auth matrix, scope isolation, invalid filter, stable empty response, limit 20/max 100, deterministic `created_at,id`.
- No audit write-path changes.
- Output: `READY FOR 2C-02`.

### Task 2C-02 — Commercial Terms Read-only UI

- Show current/scheduled/historical terms, rate, mode, effective window, source/default, loading/empty/error/pagination/filter.
- Historical rows must not look editable; no create/edit/delete/LIVE control.
- Verification: lint, build, responsive and route guard.
- Output: `READY FOR 2C-03`.

### Task 2C-03 — Platform Audit UI

- Add OWNER/PLATFORM/ALL and finance entity/action filters with safe metadata rendering.
- Tests/QA: ownerless labels, pagination reset, stale/rapid filter, mobile, no raw HTML/payload.
- Output: `READY FOR 2C-04`.

### Task 2C-04 — Cross-layer Auth dan UX Regression

- Prove anonymous/customer/owner/staff/suspended denied; active superadmin allowed; direct URL denied server-side.
- Verify loading/empty/error/stale/rapid-filter and API/UI equality.
- Output: `READY FOR 2C-05`.

### Task 2C-05 — Independent 2C Audit

- Read-only audit auth, filters, pagination, metadata, no mutation/LIVE, and UI states.
- Output: `READY FOR 2C-07` atau `READY FOR 2C-06`.

### Task 2C-06 — Exact 2C Blocker Fix (Conditional)

- Fix only audit blockers and add focused backend/frontend regression.
- Output: `READY FOR 2C-07`.

### Task 2C-07 — Phase 2C Final Gate

- Full backend, frontend lint/build, optional smoke, browser QA, Git/dependency evidence.
- Final verdict: `PHASE 2C DONE — 3A MAY BE PLANNED` atau fail/insufficient evidence.

---

# Phase 3A — Minimal Double-entry Platform Ledger

### Task 3A-00 — Accounting Contract Freeze dan Preflight

- Freeze chart of accounts, event key, payload hash, reversal/effective-time semantics, metadata allowlist, owner dimension, and migration-down refusal rules.
- Read-only baseline; no journal, OPEX, payment, commission, refund, atau payout implementation.
- Stop bila accounting side/account atau idempotency boundary belum disepakati.
- Output: `READY FOR 3A-01`.

### Task 3A-01 — Ledger Migration Only

- Paired migration untuk immutable `platform_accounts`, `platform_journals`, dan `platform_ledger_entries` serta idempotency storage bila dibutuhkan.
- DB invariants: fixed accounts, `RESTRICT`, ≥2 entries, debit=credit fail-closed, event key unique, exact one reversal, required owner dimension, no update/delete.
- Verification: fresh/upgrade/down, Rp1 imbalance, zero/negative/NULL, unknown account, owner dimension, double reversal, and post-fact down refusal.
- No service/API/domain posting.
- Output: `READY FOR 3A-02`.

### Task 3A-02 — Generic PostJournal Primitive

- Transaction-aware repository/service for balanced generic journal with checked int64 and allowlisted metadata.
- No payment/commission/refund/payout/OPEX-specific helper or route.
- Tests: exact balance, minimum entries, invalid amounts/accounts, atomic rollback, no float, generic errors.
- Output: `READY FOR 3A-03`.

### Task 3A-03 — Journal Idempotency dan Concurrency

- Same event/key + same canonical hash replays; different payload returns conflict; concurrent retry creates one journal.
- Tests include timeout-after-commit, duplicate integrity fail-closed, and no partial entries.
- Never generate a new random event key for retry.
- Output: `READY FOR 3A-04`.

### Task 3A-04 — Exact Reversal Primitive

- Reverse every original entry once with exact side inversion, linked immutable journal and required reason.
- Dilarang edit source, partial reversal, or second reversal.
- Tests: concurrent/double reversal conflict, atomic rollback, effective-time rules, exact balance.
- Output: `READY FOR 3A-05`.

### Task 3A-05 — Read-only Journal Primitives

- Add deterministic internal `ListJournals`/`GetSummary` building blocks with filters, page 20/max 100, stable empty, reversal links, and money strings.
- Do not expose OPEX/payment actual metrics yet.
- Output: `READY FOR 3A-06`.

### Task 3A-06 — Atomic Platform Audit dan LIVE Guard

- Journal/domain mutation and platform audit share one DB transaction.
- Reject secret/nested/unknown metadata and all premature LIVE/domain-specific writes.
- Tests: audit failure rolls journal back, domain rollback audit zero, ownerless event, LIVE creates no journal.
- Output: `READY FOR 3A-07`.

### Task 3A-07 — Immutability dan Down-safety Evidence

- Prove no update/delete paths, pre-fact down succeeds, and post-first-fact down refuses on disposable DB.
- Evidence only; do not destruct shared DB or add features.
- Output: `READY FOR 3A-08`.

### Task 3A-08 — Independent Ledger Audit

- Read-only audit accounting balance, schema, idempotency, reversal, immutability, audit atomicity, LIVE rejection, and absence of domain journals.
- Output: `READY FOR 3A-10` atau `READY FOR 3A-09`.

### Task 3A-09 — Exact Ledger Blocker Fix (Conditional)

- Fix only 3A-08 blockers with focused migration/transaction/concurrency regressions.
- Output: `READY FOR 3A-10`.

### Task 3A-10 — Phase 3A Final Gate

- Repeat migration, balance/reversal/idempotency/concurrency/audit tests and full backend.
- Verify no actual commission/payment/refund/payout journal exists.
- Final verdict: `PHASE 3A DONE — 3B1 MAY BE PLANNED` atau fail/insufficient evidence.

---

# Phase 3B — Platform Operating Expense

## Phase 3B1 — Backend Workflow, Journal, Reporting

### Task 3B1-00 — Expense Contract Freeze

- Freeze category/state matrix, reasonable max amount, backdate policy, vendor+external-reference uniqueness, maker-checker residual risk, effective time, error and idempotency contracts.
- Read-only; stop on unresolved policy.
- Output: `READY FOR 3B1-01`.

### Task 3B1-01 — Expense Migration Only

- Paired `platform_expenses` migration with state/timestamp/FK/unique constraints and safe down behavior.
- Tests: invalid NULL/category/status/account/amount/date, duplicate invoice, FK `RESTRICT`, fresh/upgrade/down.
- No endpoint or journal posting.
- Output: `READY FOR 3B1-02`.

### Task 3B1-02 — List dan Create DRAFT

- Active superadmin list/filter/pagination and idempotent create-DRAFT using integer-rupiah string validation.
- Atomic CREATED audit; no journal/P&L effect; no client-supplied status/actor/account side.
- Tests: auth, invalid input, replay/conflict/timeout, escaped text, summary unchanged.
- Output: `READY FOR 3B1-03`.

### Task 3B1-03 — Cancel DRAFT

- DRAFT→CANCELLED only, reason required, audit/idempotency/concurrency safe, no journal.
- Reject cancel for APPROVED/POSTED/terminal states.
- Output: `READY FOR 3B1-04`.

### Task 3B1-04 — Approve Expense

- DRAFT→APPROVED, lock business fields, explicit confirm, audit and stable idempotency.
- Apply frozen maker-checker rule; no post/edit in this task.
- Output: `READY FOR 3B1-05`.

### Task 3B1-05 — Post Expense Journal

- APPROVED→POSTED atomically with `Dr OPEX_<CATEGORY> / Cr FUNDING_CLEARING|ACCOUNTS_PAYABLE`, linked journal and audit.
- `effective_at=occurred_at`; do not claim reconciled bank balance/provider cost.
- Tests: balance, failure rollback, timeout/concurrent single post, state denial.
- Output: `READY FOR 3B1-06`.

### Task 3B1-06 — Void with Exact Reversal

- POSTED→VOID only via exact reversal journal effective at `voided_at`, reason and audit.
- Cross-month behavior explicit; no edit/delete or rewriting original period.
- Tests: double/concurrent void rejection, exact summary reversal.
- Output: `READY FOR 3B1-07`.

### Task 3B1-07 — Journals Read API

- Active superadmin `GET /admin/finance/journals` with date/event/account filters, deterministic pagination, reversal links, generic errors.
- Jakarta half-open ranges and auth matrix required.
- No mutation/export-sensitive endpoint.
- Output: `READY FOR 3B1-08`.

### Task 3B1-08 — OPEX Reporting Vertical Slice

- Count only POSTED net reversal in summary/breakdown/trend; mark OPEX `AVAILABLE`, returning 0 when available but empty.
- Projected operating result subtracts OPEX; actual platform revenue/contribution/result remain `UNAVAILABLE/null`.
- Tests: backdated/cross-month, sum consistency, money strings, empty periods.
- Output: `READY FOR 3B1-09`.

### Task 3B1-09 — Independent Backend Accounting Audit

- Read-only audit migration, state matrix, idempotency, journal balance, reversal, audit atomicity, reporting, auth, forbidden routes.
- Output: `READY FOR 3B1-11` atau `READY FOR 3B1-10`.

### Task 3B1-10 — Exact Backend Blocker Fix (Conditional)

- Fix only 3B1-09 blockers; rerun focused/full backend and migration tests.
- Output: `READY FOR 3B1-11`.

### Task 3B1-11 — Phase 3B1 Final Gate

- Actual evidence for all states, timeout/retry/concurrency, atomic audit, journals and reporting; full backend.
- Final verdict: `3B1 GO — 3B2 MAY BE PLANNED` atau fail/insufficient evidence.

## Phase 3B2 — Expense Frontend

### Task 3B2-00 — Frontend Contract/Route Preflight

- Map types, API, navigation, components, state-dependent actions and one-idempotency-key-per-user-action lifecycle.
- Read-only; output exact file and QA matrix.
- Output: `READY FOR 3B2-01`.

### Task 3B2-01 — Expense dan Journal Read UI

- List/filter/pagination/status/reversal links with loading/empty/error/mobile states.
- No mutation button in this slice.
- Verification: lint/build and API equality.
- Output: `READY FOR 3B2-02`.

### Task 3B2-02 — Create Expense UI

- Modal, validation, summary confirmation, exact money handling, one UUID retained through transport retry, pending disabled.
- Do not send server-owned fields or generate second key on double-click.
- QA: timeout/replay, XSS-safe rendering, rapid submit.
- Output: `READY FOR 3B2-03`.

### Task 3B2-03 — Cancel dan Approve UX

- Separate confirmation/reason, show only allowed state actions, keep stable key per action.
- QA: stale conflict, retry, role guard, no duplicate.
- Output: `READY FOR 3B2-04`.

### Task 3B2-04 — Post dan Void UX

- Explicit P&L warning before post; `Void dengan Reversal` with required reason and reversal link/badge.
- No edit/delete for approved/posted rows.
- QA: double-click, timeout replay, status refresh.
- Output: `READY FOR 3B2-05`.

### Task 3B2-05 — OPEX Summary dan Trend UI

- Replace unavailable OPEX with exact value/0 and retain simulation caveat for projected result.
- Actual revenue/contribution/operating result remain unavailable.
- Tests/QA across filters and periods; lint/build.
- Output: `READY FOR 3B2-06`.

### Task 3B2-06 — UI Resilience, Auth, Responsive QA

- Loading/empty/error/stale/rapid-filter/mobile/direct-route/role matrix and API/UI equality.
- Evidence task only; no feature additions.
- Output: `READY FOR 3B2-07`.

### Task 3B2-07 — Independent Frontend/Contract Audit

- Read-only audit idempotency lifecycle, state controls, auth, money precision, unavailable metrics, responsive/error states.
- Output: `READY FOR 3B2-09` atau `READY FOR 3B2-08`.

### Task 3B2-08 — Exact Frontend Blocker Fix (Conditional)

- Fix only 3B2-07 blockers; focused UI tests plus lint/build.
- Output: `READY FOR 3B2-09`.

### Task 3B2-09 — Phase 3B Final Gate

- Backend contract regression, lint/build, browser/manual QA, smoke where relevant, Git/dependency evidence.
- Final verdict: `PHASE 3B DONE — PHASE 4 MAY BE PLANNED` atau fail/insufficient evidence.

---

# Phase 4 — v1.7 Reconciliation dan Release Gate

### Task 4-00 — Release Baseline dan Evidence Matrix

- Freeze commit, DB version, feature flags, endpoints, AF-P0/P1 owners, source/formula map, and diagnostics surface decision.
- Recommend read-only CLI first; endpoint only if operational need is approved.
- Read-only baseline backend/frontend; stop on existing unexplained difference.
- Output: `READY FOR 4-01`.

### Task 4-01 — Reconciliation Core Service

- Pure read-only checks for max 366-day Jakarta half-open ranges and dated exception buckets.
- Checks: online ledger↔GMV, paid snapshot↔source, offline 0%, refund reversal, duplicates, summary=breakdown=trend, OPEX posted−reversal, actual metrics unavailable.
- Tests: clean Rp0 difference, exact fault fixtures, empty/boundary/max range.
- Output: `READY FOR 4-02`.

### Task 4-02 — Safe Reconciliation CLI/Diagnostics

- Expose core as deterministic sanitized dry-run CLI with nonzero exception exit; admin endpoint only if 4-00 explicitly approved it.
- Prove zero writes, no PII, repeatable output, invalid range rejection and auth/rate limit if endpoint exists.
- No auto-fix.
- Output: `READY FOR 4-03`.

### Task 4-03 — Reconciliation Fixture dan Boundary Suite

- Actual DB fixtures for 0/5/7/historical, promo/offline/refund, OPEX post/void cross-month, missing/duplicate anomalies and Jakarta boundaries.
- Scoped cleanup/rollback only; injected fault must be detected in correct bucket.
- Output: `READY FOR 4-04`.

### Task 4-04 — Feature-disable, Startup Guard, Rollback Hardening

- Prove monetization default false, LIVE rejected, UI/OPEX route can be disabled without deleting facts, maintenance behavior, down dependency and post-fact refusal.
- No provider secret, schema drop on shared DB, or production flag activation.
- Output: `READY FOR 4-05`.

### Task 4-05 — Automated Full Regression Evidence

- Run full backend, frontend lint/build, migration fresh/upgrade/down safety, focused concurrency/idempotency/audit/reconciliation, and smoke.
- Evidence only; any failure routes to final audit/blocker path rather than being fixed here.
- Output: `READY FOR 4-06` or `BLOCKERS FOUND`.

### Task 4-06 — Manual QA Booking/Snapshot/Projection

- Execute AF-P0-01..07: normal, promo, offline, payment verification retry, full refund, 0/5/7/legacy, term-change immutability.
- Exact rupiah/API/DB/UI evidence; no production or LIVE.
- Output: `READY FOR 4-07`.

### Task 4-07 — Manual QA OPEX/Audit/Auth/Owner Regression

- Execute AF-P0-08..10 and AF-P1-01: expense retry/post/void, atomic audit, deny roles, existing owner dashboard/finance.
- No P0/P1 money/auth failure may be waived as limitation.
- Output: `READY FOR 4-08`.

### Task 4-08 — Manual QA Time/Reconciliation/UX

- Execute AF-P0-11..12 and AF-P1-02: Jakarta boundary, reconciliation Rp0, loading/empty/error/stale/rapid-filter/mobile.
- Output: `READY FOR 4-09`.

### Task 4-09 — v1.7 Release Docs dan Runbook

- Update readiness, known limitations, simulation-not-tax-report wording, metric dictionary, feature-disable, rollback and anomaly ownership.
- State clearly that gateway, actual commission, payable and payout are unavailable.
- No secret or unsupported LIVE claim.
- Output: `READY FOR 4-10`.

### Task 4-10 — Independent v1.7 Release Audit

- Read-only source-plan/DoD/security/accounting/scope audit over all evidence and diff since Phase 2A.
- Verdict only `NO BLOCKER`, `BLOCKERS FOUND`, or `INSUFFICIENT EVIDENCE`.
- Output: `READY FOR 4-12` atau `READY FOR 4-11`.

### Task 4-11 — Exact v1.7 Release Blocker Fix (Conditional)

- Fix only 4-10 blockers; never alter facts merely to force reconciliation to zero.
- Rerun impacted integration and full checks.
- Output: `READY FOR 4-12`.

### Task 4-12 — v1.7 Final Regression dan Release Gate

- Rerun final migration, backend/frontend/smoke, P0/P1, reconciliation Rp0, banners/flags, no-secret, Git/dependency/scope and rollback evidence.
- Do not start Phase 5 in the same task.
- Final verdict exactly one:

```text
FINAL REVIEW PASS — v1.7 PHASE 0–4 DONE — PHASE 5A MAY BE PLANNED
FINAL REVIEW FAIL — v1.7 BLOCKER REMAINS
INSUFFICIENT EVIDENCE
```

---

# Phase 5 — Payment Gateway Foundation (Sandbox/Shadow Only)

Hard prerequisite: Phase 4 final PASS, provider resmi dipilih, dan manusia menyetujui fund-flow/security/legal ADR. Semua runtime tetap sandbox/shadow dengan `PLATFORM_MONETIZATION_ENABLED=false`.

## Phase 5A — Provider dan Fund-flow Gate

### Task 5A-00 — Phase 5 Entry Preflight

- Read-only proof bahwa Phase 4 PASS, P0/P1 selesai, reconciliation Rp0, simulation banner aktif, kill switch false, dan tidak ada actual journal/payable/payout/provider secret.
- Missing evidence apa pun menghasilkan `NO-GO`.
- Output: `READY FOR 5A-01` atau `PHASE 5 NO-GO`.

### Task 5A-01 — Provider Capability Evidence

- Freeze named provider, merchant account, initial methods, marketplace/split/refund/KYC/settlement/signing/API-version/sandbox capabilities and fee/tax evidence.
- No adapter or credential storage.
- Unproven marketplace/refund/signing capability means `NO-GO`.
- Output: `READY FOR 5A-02`.

### Task 5A-02 — Fund-flow dan Accounting ADR

- Decide merchant/seller of record, custody, owner liability, clearing/cash/subaccount, settlement, refund/chargeback and provider fee/tax mapping.
- Include diagrams, conceptual journal mapping, named finance/legal approver role/date.
- No implementation; without contractual/legal signoff remain `NO-GO`.
- Output: `READY FOR 5A-03`.

### Task 5A-03 — Payment/Refund State-machine Freeze

- Define allowed/denied attempt, webhook, timeout, retry, late capture, expiry, duplicate/out-of-order and refund transitions, authority, idempotency and lock order.
- Browser callback cannot mark paid; approval cannot mean refund succeeded; partial refund remains out of scope.
- Output: `READY FOR 5A-04`.

### Task 5A-04 — Security, Privacy, Operational Gate

- Freeze secret management, exact signature bytes/algorithm, constant-time verification, timestamp tolerance, replay defense, limits, redaction, audit, rotation, TOS/privacy/refund/KYC readiness.
- Produce threat model and incident/key-rotation runbook without real secrets/raw sensitive fixtures.
- Output: `READY FOR 5A-05` atau `PHASE 5 NO-GO`.

### Task 5A-05 — Technical Contract Freeze

- Convert approved ADRs into migration order, schema delta, normalized adapter DTO, command/event names, idempotency namespaces, feature flags, metrics, fixtures and rollback plan.
- No source/migration implementation.
- Human verdict required: `GO FOR SANDBOX/SHADOW ONLY`.

## Phase 5B — Payment Facts dan Sandbox Adapter

### Task 5B-00 — Repository/Migration Preflight

- Read-only actual branch/status/migration inventory and snapshot/ledger/audit/idempotency dependency map.
- Stop on missing foundation or dirty overlap.
- Output: `READY FOR 5B-01`.

### Task 5B-01 — Payment Attempts Migration

- Paired migration for canonical attempts and immutable capture facts: snapshot FK, IDR/int64, provider/key/attempt uniqueness, capture-once partial unique, immutable `captured_at`.
- Tests: fresh/upgrade/down, enums/money/FKs, duplicate IDs, second capture remains denied after refund.
- No webhook/outbox/refund/cost/journal/backfill.
- Output: `READY FOR 5B-02`.

### Task 5B-02 — Strict Payment DTO dan Money Validation

- Integer string→checked int64, IDR only, rail/mode/state/reference allowlists and generic errors.
- Tests: zero/negative/fraction/scientific/separator/overflow/max and invalid currency/enum.
- No provider or booking mutation.
- Output: `READY FOR 5B-03`.

### Task 5B-03 — Payment Repository dan State Guard

- Create/get/next attempt, row lock, compare-and-set, capture-once, deterministic replay/conflict.
- Tests: illegal downgrade, concurrent capture, rollback, retry without duplicate fact.
- No provider call, booking PAID, or journal.
- Output: `READY FOR 5B-04`.

### Task 5B-04 — Provider-neutral Adapter Contract

- Interface `CreatePayment`, `GetPaymentStatus`, `VerifyWebhook`, `ParseWebhook`, `RequestRefund` with normalized DTO/error, fake adapter and safe config.
- Business services may not import provider SDK DTO.
- Tests: timeout/retryable/terminal mapping and redacted logs.
- Output: `READY FOR 5B-05`.

### Task 5B-05 — Finance Outbox Foundation

- Paired migration and transaction-aware enqueue primitive for durable provider commands: canonical command type, deterministic idempotency key, redacted payload, attempts, lease/retry/terminal state and `RESTRICT` references.
- Tests: atomic domain+enqueue, duplicate same payload replay, different payload conflict, lease/restart safety and no raw secret/card/bank data.
- No worker/provider call/webhook/refund yet.
- Output: `READY FOR 5B-06`.

### Task 5B-06 — Sandbox Create-payment Orchestration

- Idempotent local attempt and outbox command, immutable monetization decision, deterministic provider key, safe checkout reference and atomic audit.
- Same key replay; different payload conflict; timeout remains uncertain; flag-off audited.
- No synchronous provider call, booking paid, webhook, actual journal, owner cash insertion, or production funds.
- Output: `READY FOR 5B-07`.

### Task 5B-07 — Sandbox Inquiry/Timeout Recovery

- Inquiry command/result resolves uncertain attempts without creating a new external payment or downgrading terminal state.
- Tests: timeout→success/pending/failure, repeated inquiry no-op, mismatch reject, redirect non-authoritative.
- Output: `READY FOR 5B-08`.

### Task 5B-08 — Payment Facts Regression Gate

- Migration, package/full backend, immutable capture, concurrency, config/secret/dependency and no-actual-journal evidence.
- Review-only; blockers use conditional `5B-FIX`, then this gate repeats.
- Verdict: `PHASE 5B PASS — 5C MAY START` or fail.

### Task 5B-FIX — Exact Payment Facts Blocker Fix (Conditional)

- Fix only blockers recorded by 5B-08 with focused migration/state/idempotency regressions.
- No webhook/refund/journal feature may be pulled forward.
- Repeat 5B-08 after review.

## Phase 5C — Webhook Inbox dan Outbox

### Task 5C-00 — Signed Webhook Fixture Freeze

- Obtain versioned redacted valid/invalid signature, canonical bytes, timestamps, replay, duplicate/out-of-order examples and normalized expectations.
- No real secrets or raw sensitive payload.
- Stop if signing bytes/tolerance remain uncertain.

### Task 5C-01 — Webhook Inbox Migration

- Paired migration for append-only `payment_webhook_events`: unique provider event, hash/redacted JSON, verification/processing states and `RESTRICT` references. Reuse the durable outbox created by 5B-05.
- Reject raw-payload columns, random event identities and cascade delete.
- Output: `READY FOR 5C-02`.

### Task 5C-02 — Signature/Timestamp/Replay Verification

- Verify exact raw body before parsing using constant-time comparison, timestamp tolerance, rotation and replay checks.
- Tests: valid/tampered/stale/future/missing/replay/rotation/malformed.
- Verifier must fail closed and never log body/signature/secret.
- Output: `READY FOR 5C-03`.

### Task 5C-03 — Hardened Webhook Ingress

- Route with body/rate limit, correlation ID, generic response, hash/redaction and durable duplicate no-op.
- No direct booking-paid or journal mutation.
- Tests: invalid auth/signature, oversized body, DB failure, duplicate and sensitive-data persistence scan.
- Output: `READY FOR 5C-04`.

### Task 5C-04 — Transactional Outbox Worker

- Atomic enqueue, lease/claim, backoff/jitter, deterministic key, crash/restart recovery and terminal errors; provider call outside transaction.
- Tests: crash before/after call, concurrent workers, lease expiry, kill switch.
- Output: `READY FOR 5C-05`.

### Task 5C-05 — Idempotent Payment Event Processor

- Validate payment/booking/amount/currency/state, lock row, capture once, handle duplicate/out-of-order and atomically mark inbox processing.
- No actual journal/payable/revenue/payout or legacy owner-cash path.
- Tests: pending/captured/failed/expired, mismatch, capture-vs-expiry and rollback.
- Output: `READY FOR 5C-06`.

### Task 5C-06 — Late-capture Exception Flow

- Captured-after-expiry/cancel creates durable exception, hold diagnostic, idempotent refund intent, audit and metric.
- Booking/slot must not reopen/mark paid; no final refund/revenue.
- Output: `READY FOR 5C-07`.

### Task 5C-07 — Concurrency dan Legacy-isolation Tests

- Separate DB connections for duplicate webhook, two captures, capture-vs-expiry, worker race and manual-vs-gateway.
- Repeat stress; expected one fact/event/command and zero legacy owner-income insertion.
- Output: `READY FOR 5C-08`.

### Task 5C-08 — Observability/Security Checkpoint

- Evidence metrics/alerts/log redaction/runbook for received, verified, failed, retried, duplicate, mismatch and late capture.
- Full regression/migration/secret scan; no production credential or LIVE.
- Blockers use conditional `5C-FIX`, then checkpoint repeats.
- Verdict: `PHASE 5C PASS — 5D MAY START` or fail.

### Task 5C-FIX — Exact Webhook/Outbox Blocker Fix (Conditional)

- Fix only blockers recorded by 5C-08 and rerun signature, replay, concurrency, redaction and migration evidence.
- Contract/signing changes return to 5A human-approved specification.
- Repeat 5C-08 after review.

## Phase 5D — Refund dan Provider Cost Facts

### Task 5D-00 — Refund-path Mapping Freeze

- Map both legacy approval paths into one normalized money-flow, with eligibility, SLA, booking-vs-money states, lock order and deterministic key.
- No code, partial refund or ordinary completed-booking refund.
- Stop if any path can directly claim provider refund success.

### Task 5D-01 — Refund/Cost Migration dan Journal References

- Paired migration for `payment_refunds`, `payment_cost_items`, journal source FKs/checks/uniqueness, positive amount and effect enum.
- Tests: fresh/upgrade/down, `RESTRICT`, source combinations, duplicate provider IDs and no signed amount.
- No posting/backfill/payout.
- Output: `READY FOR 5D-02`.

### Task 5D-02 — Normalized Full-refund Request Service

- REQUESTED→PROCESSING only, exact captured amount, eligibility, row lock, replay/conflict, outbox and audit atomically.
- Approval is not SUCCEEDED; no final journal.
- Tests: no capture, wrong amount, completion/time boundary, retry/concurrency/rollback.
- Output: `READY FOR 5D-03`.

### Task 5D-03 — Sandbox Refund Command

- Provider `RequestRefund` through durable worker with deterministic key; timeout remains uncertain and resolved by inquiry.
- No provider call in DB transaction or synchronous success assumption.
- Tests: timeout, duplicate, retry, rejection and restart.
- Output: `READY FOR 5D-04`.

### Task 5D-04 — Refund Result Normalization

- Only verified webhook/inquiry may set SUCCEEDED/FAILED; exact amount/provider ID, no downgrade, successful total cannot exceed capture.
- Capture fact remains immutable; shadow mode makes no production reversal.
- Tests: duplicate/out-of-order/mismatch/two concurrent successes.
- Output: `READY FOR 5D-05`.

### Task 5D-05 — Legacy Refund Bridge

- Thinly connect both legacy paths to one fact while keeping cancellation state separate from money status and stable legacy responses.
- Approval response remains PROCESSING; crossover retry no-op.
- No legacy route deletion or owner-finance refactor.
- Output: `READY FOR 5D-06`.

### Task 5D-06 — Append-only Provider Cost Facts

- Record only provider-confirmed processing/refund/tax/adjustment charge or reversal using positive amounts and unique reference.
- Estimation remains unavailable and may not enter actual report/journal.
- Tests: duplicates, exact reversal, invalid amount/effect/type.
- Output: `READY FOR 5D-07`.

### Task 5D-07 — Holds, SLA, Escalation Diagnostics

- Make unresolved refunds visible and emit hold signal for future payable, pending-age metric and sanitized escalation codes/audit.
- No Phase 6 payable/payout implementation.
- Tests: SLA boundaries, retry/failed/unresolved states, no false customer success.
- Output: `READY FOR 5D-08`.

### Task 5D-08 — Refund/Cost Regression Gate

- Test both paths, retry, refund-vs-completion races, one fact, capture immutability, audit/privacy, migration and full backend.
- No final shadow journal or sensitive leak.
- Blockers use conditional `5D-FIX`, then gate repeats.
- Verdict: `PHASE 5D PASS — 5E MAY START` or fail.

### Task 5D-FIX — Exact Refund/Cost Blocker Fix (Conditional)

- Fix only blockers recorded by 5D-08 with focused refund race, idempotency, cost and privacy regressions.
- Refund policy changes return to the approved Phase 5A contract.
- Repeat 5D-08 after review.

## Phase 5E — Shadow Reconciliation dan Isolated Journal Templates

### Task 5E-00 — Reconciliation Contract/Fixture Freeze

- Freeze PSP equations, source/timing differences, manual-direct separation, pilot dataset and exact 0/5/7/promo/refund/rounding/max fixtures.
- No journal or payout reconciliation implementation.
- Output: `READY FOR 5E-01`.

### Task 5E-01 — Read-only Provider Reconciliation Engine

- Compare provider versus local payment/refund/cost facts by range/timezone/reference/status/amount; identify timing pending vs unexplained.
- Prove zero writes; no raw data exposure or auto-fix.
- Tests: match, mismatch, missing each side, pending timing and unexplained difference.
- Output: `READY FOR 5E-02`.

### Task 5E-02 — Metric-source Union/Duplicate-income Checks

- Prove manual direct, cash/offline and platform-collected sources are mutually exclusive; gateway facts never call legacy owner cash income.
- Actual metrics remain unavailable in shadow.
- Stop if one booking appears in two income sources.
- Output: `READY FOR 5E-03`.

### Task 5E-03 — Runtime Shadow Guard/Test-ledger Capability

- Deny all runtime actual posting; allow journal template only behind an explicit isolated test capability and startup validation.
- Rejected writes are audited/observable; default/forged production config fails closed.
- No UI toggle or production activation.
- Output: `READY FOR 5E-04`.

### Task 5E-04 — Capture Journal Template

- Isolated test ledger only: ADR-approved clearing, owner payable, unearned commission, zero-line omission, source FK and deterministic key.
- Tests: 0/5/7/promo/max, exact balance, replay/conflict/rollback.
- Output: `READY FOR 5E-05`.

### Task 5E-05 — Provider-fee Journal Template

- Isolated confirmed fee charge/reversal using exact source and deterministic key.
- Reject estimates; do not reduce customer/owner amount outside ADR.
- Tests: balance, duplicate, reversal and immutable original.
- Output: `READY FOR 5E-06`.

### Task 5E-06 — Completion Template dan Shadow Scheduler

- Isolated unearned→revenue template at service completion effective time; runtime shadow only creates comparison/marker, never actual revenue.
- 0% omits zero journal; never resolve latest rate.
- Tests: early/late worker, retry, races and exact snapshot.
- Output: `READY FOR 5E-07`.

### Task 5E-07 — Refund/Dispute Journal Templates

- Isolated exact pre-completion reversal, post-completion contra and conceptual post-payout receivable paths based on immutable facts.
- Tests: refund-vs-completion/payout races, replay and no source-journal edit.
- No Phase 6 payout implementation.
- Output: `READY FOR 5E-08`.

### Task 5E-08 — Sandbox E2E dan Observability

- Every pilot payment method: create/capture, timeout/inquiry, duplicate/out-of-order, mismatch, late capture, refund, cost, scheduler and restart.
- Required: provider/local reconciliation 100%, unexplained Rp0, zero duplicate income, stress repeats, safe logs and incident drills.
- Output: `READY FOR 5E-09`.

### Task 5E-09 — Independent Phase 5 Security/Privacy Audit

- Adversarial read-only audit signature/replay, auth/rate limits, logs/secrets, append-only facts, races, isolated capability, flags and fund-flow ADR compliance.
- Findings include severity, exact file/evidence and required regression; do not fix here.
- Output: `READY FOR 5E-10` or conditional `5E-FIX`.

### Task 5E-FIX — Exact Phase 5 Blocker Fix (Conditional)

- Fix only 5E-09 findings with focused security/concurrency/accounting regression.
- Any changed fund-flow/state/security contract returns to human ADR approval.
- Output: `READY FOR 5E-10`.

### Task 5E-10 — Phase 5 Final Shadow Gate

- Fresh/upgrade/down migrations, full tests, every-method sandbox E2E, reconciliation, concurrency, audit, config, runbook, Git/dependencies.
- Must prove runtime actual journal denied, kill switch false, actual revenue unavailable, no production credential/funds/payable/payout.
- Final verdict only:

```text
PHASE 5 SHADOW PASS — READY FOR PHASE 6A PLANNING
PHASE 5 NO-GO — BLOCKER REMAINS
INSUFFICIENT EVIDENCE
```

---

# Phase 6 — Owner Payable, Settlement, Payout (Sandbox Only)

Phase 6 tetap sandbox/shadow dengan monetization false dan tanpa real customer funds atau production payout. Finance, Security, Legal/Privacy, Accounting, Operations and provider approvals are human gates.

## Phase 6A — Provider-specific Payable/Payout Plan

### Task 6A-00 — Phase 5 Exit Preflight

- Read-only proof of Phase 5 final PASS, named provider, every-method sandbox E2E, reconciliation 100%, zero duplicate journal/income, observability/runbook and no P0/P1.
- Any missing evidence means `NO-GO`.
- Output: `READY FOR 6A-01`.

### Task 6A-01 — Provider Payout Fund-flow ADR

- Freeze custody, owner-liability timing, settlement timing, subaccount/PSP destination, fee/tax deduction, refund/chargeback liability, payout/inquiry/idempotency and official account mapping.
- Include capture→settlement→payout→refund examples and contractual references.
- No implementation; ambiguous or unsupported marketplace/payout flow means `NO-GO`.
- Output: `READY FOR 6A-02`.

### Task 6A-02 — Domain Invariants, Journal Map, Race Matrix

- Freeze payable/receivable/settlement/payout states, Monday Jakarta `[)` cutoff, 24-hour hold, Rp100.000 minimum, negative carry, full-refund-only, UNKNOWN retry, lock order and exact journals.
- No schema/endpoint/worker/provider call.
- Stop on any unbalanced journal, dual source of truth or unresolved post-payout refund semantics.
- Output: `READY FOR 6A-03`.

### Task 6A-03 — Security/SOP Package dan Human Plan Gate

- Define KYC, payout destination versions, 48-hour cooldown/notification, maker-checker, first-three review, separate switches, rate limits, audit, incident/rollback/reconciliation owners and on-call.
- Antigravity prepares; Codex reviews; Finance/Security/Legal/Privacy/Accounting/Operations approve with name/role/date/reference.
- **6B cannot start without explicit human GO.**

## Phase 6B — Payable, Receivable, Settlement Primitives

### Task 6B-00 — Repository Preflight dan Schema Freeze

- Read-only payment/refund/journal dependency map, migration number, dirty state, patterns and `go test -count=1 ./...`.
- Output: `READY FOR 6B-01`.

### Task 6B-01 — Payable, Adjustment, Dispute Migration

- Paired migration for `owner_payables`, append-only `owner_balance_adjustments`, and `payment_disputes` if absent.
- Tests: state/amount/version/unique booking, positive adjustments, restrictive FKs, immutability, fresh/upgrade/down.
- No payout tables/API/UI/provider call/backfill production.
- Output: `READY FOR 6B-02`.

### Task 6B-02 — Provider Settlement dan Journal-source Migration

- Separate immutable `provider_settlements/items` from payout with unique external/source facts, IDR and journal source checks.
- Do not hard-code fee credit account beyond approved ADR.
- Tests: duplicate/missing source, settlement/payout separation, down safety.
- Output: `READY FOR 6B-03`.

### Task 6B-03 — Idempotent Owner Payable Creation

- Create exactly one PENDING payable from normalized sandbox LIVE capture under isolated test capability; amount equals snapshot owner net.
- Historical/manual/SIMULATION/offline cannot create payable; mutation/journal/audit atomic; no legacy cash insertion.
- Tests: 0/5/7/custom, duplicate, missing snapshot, rollback.
- Output: `READY FOR 6B-04`.

### Task 6B-04 — Eligibility, Hold, Weekly Cutoff

- PENDING→AVAILABLE only after captured+settled, COMPLETED, ≥24h hold, no refund/dispute, Monday Jakarta half-open cutoff.
- Row lock/version and suspended-owner semantics required.
- Tests exact boundaries and out-of-order events; no payout batch yet.
- Output: `READY FOR 6B-05`.

### Task 6B-05 — Refund, Chargeback, Negative Carry-forward

- Before payout reverse payable; after payout append receivable/debit and future explicit offset journal.
- Never edit old payout, use signed amount, silently net, or add partial refund.
- Tests: before/after payout, duplicate, dispute, negative balance blocks payout, exact journal/no double reversal.
- Output: `READY FOR 6B-06`.

### Task 6B-06 — Provider Settlement Ingestion dan Matching

- Idempotent sandbox import/replay matching PAYMENT/REFUND/COST facts with amount/currency/timing validation and immutable match.
- No raw provider payload or payout creation.
- Tests: duplicate, missing/duplicate source, mismatch, out-of-order and exact settlement equation.
- Output: `READY FOR 6B-07`.

### Task 6B-07 — Payable/Settlement Reconciliation dan Metrics

- Read-only compare operational subledger, platform ledger and provider facts with pending SLA, settlement age and difference metrics.
- Unexplained Rp1 fails; no auto-repair or estimated facts.
- Output: `READY FOR 6B-08`.

### Task 6B-08 — Independent 6B Contract/Security Audit

- Read-only audit migrations, money, states, journals, audit, races, no historical payable and no production write.
- Output: `READY FOR 6B-10` or `READY FOR 6B-09`.

### Task 6B-09 — Exact 6B Blocker Fix (Conditional)

- Fix only 6B-08 blockers and focused regressions; return to ADR if contract changes.
- Output: `READY FOR 6B-10`.

### Task 6B-10 — Phase 6B Final Regression dan Evidence

- Migration, focused/full backend, journal/audit/rollback/concurrency/reconciliation evidence.
- Final verdict: `PHASE 6B PASS — 6C MAY START` or fail/insufficient evidence.

## Phase 6C — Payout Sandbox Execution

### Task 6C-00 — Payout Sandbox Preflight

- Freeze payout/inquiry/idempotency docs, sandbox credential path, outbox, maker-checker and migration number.
- No code/provider request; production-only credentials or non-authoritative inquiry means stop.

### Task 6C-01 — Payout Schema Migration

- Paired migration for payout account versions, aggregates, items, attempts and journal FKs with restrictive/immutable/unique constraints.
- Tests: payable allocated once, stable attempt/provider/idempotency IDs, valid states, PAID immutable, fresh/upgrade/down.
- No service/API/provider call.
- Output: `READY FOR 6C-02`.

### Task 6C-02 — Payout Account Versioning/KYC/Cooldown

- Store provider beneficiary token, masked account, verified name/status, actors/notifications and ≥48h cooldown as new immutable versions.
- Never store raw bank credentials or edit old version.
- Tests: unverified/cooldown block and historical payout remains linked.
- Output: `READY FOR 6C-03`.

### Task 6C-03 — Weekly Batch dan Allocation

- Monday Jakarta `[)`, Rp100.000 minimum, negative/debit offset, `FOR UPDATE SKIP LOCKED`, suspension semantics, exact item/aggregate totals and frozen destination.
- Batch must derive from payable rows, not reporting aggregate.
- Tests concurrent allocators, carry-forward and fee outside owner net.
- Output: `READY FOR 6C-04`.

### Task 6C-04 — Maker-checker Approval

- DRAFT→READY→APPROVED with different maker/checker, first-three review evidence, frozen destination and dual-actor audit.
- Same actor/unauthorized/suspended denied; concurrent approval idempotent.
- No single-admin production payout.
- Output: `READY FOR 6C-05`.

### Task 6C-05 — Sandbox Outbox dan Provider Adapter

- Stable idempotency, provider-neutral sandbox adapter, redacted outbox and external call only after DB commit.
- Tests timeout/retry/no duplicate and explicit sandbox environment assertion.
- No production endpoint/credential/funds.
- Output: `READY FOR 6C-06`.

### Task 6C-06 — UNKNOWN Inquiry dan Append-only Retry

- UNKNOWN requires authoritative inquiry; retry only after confirmed failure using a new append-only attempt on same aggregate; PAID terminal.
- Do not immediately retry timeout or release items to another batch.
- Tests timeout→UNKNOWN→success/failure and duplicate callback.
- Output: `READY FOR 6C-07`.

### Task 6C-07 — Payout Journal dan Provider Fee

- Post ADR-approved `Dr OWNER_PAYABLE / Cr clearing` and separate platform payout-fee expense atomically with status/audit.
- Payout is not platform expense/owner revenue; fee never reduces owner net.
- Tests exact balance, replay and journal-failure rollback.
- Output: `READY FOR 6C-08`.

### Task 6C-08 — Kill Switch, Rate Limit, Audit

- Separate payout switch, rejection audit, correlation, rate limit and allowlisted create/approve/fail/retry/paid events.
- Switch blocks dispatch but permits inquiry/reconciliation; do not combine payment/refund/payout switches.
- Output: `READY FOR 6C-09`.

### Task 6C-09 — Payout Concurrency/Atomicity Matrix

- Separate DB connections: two allocators, refund/dispute-vs-allocation, approval/retry, duplicate callback, journal failure and kill-switch race.
- Expected one effect, no partial audit/journal and no duplicate payout.
- Output: `READY FOR 6C-10`.

### Task 6C-10 — Independent Payout Audit

- Read-only schema/accounting/idempotency/security/provider-boundary audit.
- Output: `READY FOR 6C-12` or `READY FOR 6C-11`.

### Task 6C-11 — Exact Payout Blocker Fix (Conditional)

- Fix only 6C-10 blockers; provider/ADR redesign returns to human approval.
- Output: `READY FOR 6C-12`.

### Task 6C-12 — Phase 6C Final Sandbox Evidence

- Migration, sandbox E2E, UNKNOWN/inquiry/retry, maker-checker, switch, journals, reconciliation and Git scope.
- Final verdict: `PHASE 6C SANDBOX PASS — 6D MAY START`; never claim production-ready.

## Phase 6D — Owner/Admin UI dan Reconciliation

### Task 6D-00 — API/Money Contract Freeze

- Freeze owner/admin endpoints, integer-rupiah strings, states/errors/filters/timezone/auth, page 20/max 100 and booking drill-down.
- No implementation; contract cannot mix revenue, payable and payout.
- Output: `READY FOR 6D-01`.

### Task 6D-01 — Owner Marketplace Finance Read Model

- Mutually exclusive union of legacy direct/manual cashbook and marketplace gross, commission, net, payable and payout facts.
- Gateway capture is not owner cash; payout is not second income; full refund net zero; provider fee not owner expense.
- Tests all channels and no double counting; no broad owner-finance refactor.
- Output: `READY FOR 6D-02`.

### Task 6D-02 — Owner Read-only Payout APIs

- Owner-only list/detail/items/payable state/term context/masked destination with deterministic pagination and empty/error states.
- Complete ownership/auth matrix and no cross-owner/sensitive data.
- No owner mutation.
- Output: `READY FOR 6D-03`.

### Task 6D-03 — Admin Payable/Payout/Reconciliation APIs

- Active superadmin read models and only already-approved sandbox maker-checker actions, with safe filters/errors/idempotency.
- Do not invent transitions in handler or expose generic production dispatch.
- Output: `READY FOR 6D-04`.

### Task 6D-04 — Owner Settlement/Payout UI

- Separate payout from owner P&L/cashbook; show gross/commission/net/payable/payout, booking drill-down and masked destination.
- No React money calculations or major dashboard redesign.
- QA: ownership, exact API values, mobile/loading/empty/error.
- Output: `READY FOR 6D-05`.

### Task 6D-05 — Admin Operational UI

- Show outstanding/held/allocated/paid, UNKNOWN/failure, reconciliation, disabled switches and maker/checker actors.
- No hidden production activation or raw provider error.
- QA: route/server guards, loading/empty/error/retry/double-submit/mobile.
- Output: `READY FOR 6D-06`.

### Task 6D-06 — Sandbox E2E dan Cross-view Consistency

- Capture→completion→settlement→payout plus refund/failure/retry/negative carry must match provider facts, platform ledger, owner report, payout items and admin reconciliation exactly.
- Frontend lint/build and owner isolation required; Rp1 difference is failure.
- Output: `READY FOR 6D-07`.

### Task 6D-07 — Independent Phase 6 Audit

- Read-only full Phase 6 accounting/security/auth/idempotency/UI/SOP audit.
- Output: `READY FOR 6D-09` or `READY FOR 6D-08`.

### Task 6D-08 — Exact Phase 6 Blocker Fix (Conditional)

- Fix only 6D-07 blockers with focused/full regressions.
- Output: `READY FOR 6D-09`.

### Task 6D-09 — Phase 6 Final Regression dan Human Exit Gate

- Full backend, frontend lint/build, migrations, sandbox E2E, duplicate-payout prevention, daily reconciliation Rp0, owner/admin UI and SOP evidence.
- Technical PASS is followed by explicit Legal/Operations/Finance/Security approval.
- **7A cannot start without Phase 6 human exit approval.**

---

# Phase 7A — Pilot Readiness dan Runbook

Phase 7A remains readiness-only. It must not mutate production state.

### Task 7A-00 — Phase 6 Exit Preflight

- Read-only verification of Phase 6 technical evidence, signoffs, SOP, MFA, switches, reconciliation and zero P0/P1.
- Missing/stale artifact means `NO-GO`.
- Output: `READY FOR 7A-01`.

### Task 7A-01 — Immutable Commercial Acceptance

- Migration and service for owner acceptance version evidence: commercial schedule, TOS/refund/payout versions, actor/time/IP/UA/evidence hash/reference.
- No edit/delete, raw document or automatic acceptance.
- Tests: authorized owner, replay/conflict, new version requires new acceptance, restrictive deletion.
- Output: `READY FOR 7A-02`.

### Task 7A-02 — Cohort Schema/Foundation

- Unique owner cohort, state, first capture, trial/intro dates, subsidy budget and audit; service disabled by default.
- No production enrollment/global switch; subsidy spent remains derived, not a mutable counter.
- Tests valid/invalid transitions and concurrency.
- Output: `READY FOR 7A-03`.

### Task 7A-03 — Controlled Enrollment dan Immediate 0% Term

- Under isolated test capability, atomically enroll only approved owner and create immediate 0% term with idempotency/audit.
- Acceptance, KYC and verified destination required; no generic LIVE UI/API or production enrollment.
- Tests replay, partial rollback and old snapshot immutability.
- Output: `READY FOR 7A-04`.

### Task 7A-04 — First-capture CAS dan 0→5→7 Calendar

- Row-lock/CAS first capture exactly once and schedule `[first,+90d)` 0%, `[+90,+180d)` 5%, then 7%, half-open without overlap/gap.
- KPI may not alter dates; old snapshots remain immutable.
- Tests concurrent first captures and exact boundaries.
- Output: `READY FOR 7A-05`.

### Task 7A-05 — Fail-closed LIVE Eligibility Gate

- Require environment flag, exact cohort allowlist, matching acceptance, KYC, verified/cooldown destination and provider readiness for every monetization command.
- Each missing prerequisite returns safe denial and audit; kill switch wins races.
- No generic LIVE toggle; production stays off in this task.
- Output: `READY FOR 7A-06`.

### Task 7A-06 — Owner/Customer Disclosure

- Implement only human-approved legal/product copy showing terms, final amount, payout and refund status using backend amounts.
- No hidden fee or frontend money formula.
- QA mobile/desktop/stale/error/version visibility, then human Legal/Product content approval.
- Output: `READY FOR 7A-07`.

### Task 7A-07 — Operator Security Controls

- Enforce MFA for finance superadmin, maker-checker identities, limits/correlation, separate payment/refund/payout switches and rejected-operation audit.
- No activation.
- Tests MFA absent/expired, role/status matrix and independent switches.
- Output: `READY FOR 7A-08`.

### Task 7A-08 — KPI, Unit Economics, Monitoring Read Model

- Exact cohort/payment-method metrics for success/conversion/refund/cancel/chargeback/reconciliation/payout/take rate/contribution/complaints/churn/subsidy.
- KPI never changes rates automatically; cohort 0 uses cost/GMV and cost/booking, not cost/commission.
- Tests 35%/50% cost alerts, 80% subsidy and negative 7% contribution; no float.
- Output: `READY FOR 7A-09`.

### Task 7A-09 — Alerts, Incident, Backup/Restore Drills

- Prove webhook backlog, mismatch, negative balance, payout failure and duplicate alerts, kill-switch drill, backup/restore and reconciliation after restore with named on-call.
- Operations/Security humans must sign evidence.
- Output: `READY FOR 7A-10`.

### Task 7A-10 — Independent Pilot Readiness Audit

- Read-only acceptance/KYC/provider/legal/accounting/privacy/MFA/maker-checker/switch/backup/calendar/economics/monitoring/rollback audit.
- Output: `READY FOR 7A-12`, `READY FOR 7A-11`, or `INSUFFICIENT EVIDENCE`.

### Task 7A-11 — Exact Readiness Blocker Fix (Conditional)

- Fix only 7A-10 blockers; no production activation.
- Output: `READY FOR 7A-12`.

### Task 7A-12 — Final GO/NO-GO dan Human Activation Approval

- Compile maximum 3–5 owner pilot list, exact acceptance/KYC/destination/contract/threshold/economics/switch/MFA/maker-checker/restore/on-call/rollback evidence.
- AI may not set env, enroll production owner, create production LIVE terms or initiate funds.
- Finance, Legal, Accounting, Privacy/Security, Operations and authorized business approver must sign.
- **7B cannot start without signed human GO.**

---

# Phase 7B — Human-activated Pilot dan Final Monitoring

### Task 7B-00 — Human-only Pilot Activation

- Authorized human operators only: activate exact pilot environment and maximum 3–5 approved owner allowlist/cohorts/terms under dual control and change ticket.
- Antigravity/Codex may only inspect evidence; they must not activate, enroll, change terms or dispatch money.
- Output: signed activation or `NO-GO`.

### Task 7B-01 — Immediate Post-activation Smoke

- Read-only confirm pilot owner allowed, non-pilot denied, correct 0% term, acceptance/audit/alerts and no duplicate journal/payable/order leakage.
- Failures escalate to authorized human runbook.
- Output: `READY FOR 7B-02` or `PILOT HARD STOP`.

### Task 7B-02 — Daily Reconciliation/Hard-stop Monitoring

- Daily PSP/payable equations, journal balance, webhook/payment/refund/payout metrics and signed result; unexplained difference must remain Rp0.
- No silent repair or autonomous kill switch unless explicit authority was given.
- Any hard-stop triggers immediate human escalation.

### Task 7B-03 — First Three Payouts per Owner

- Human maker-checker validates items, destination, KYC/cooldown, net, fees and references and manually reviews first three successful payouts per owner.
- AI cannot approve/dispatch; it may collect and audit evidence.
- Stop on UNKNOWN, mismatch, account change or failed reconciliation.

### Task 7B-04 — Day-90/Day-180 Contract Boundary Monitoring

- Only when real boundary arrives, read-only prove new booking snapshots resolve exact 0→5→7 rate with no overlap/gap and historical snapshots unchanged.
- No early production time simulation or manual rate rewrite.
- Mismatch triggers hard stop for new payment creation.

### Task 7B-05 — Cohort Expansion Eligibility Evidence

- Evaluate only after ≥100 captures **and** ≥30 days, 14 consecutive days unexplained Rp0, and three manual payouts per owner.
- Include KPI, incidents, complaints, chargebacks, conversion, unit economics and subsidy; produce reproducible `ELIGIBLE/NOT ELIGIBLE`.
- No expansion or rate change.

### Task 7B-06 — Human-only Expansion/Automation Decision

- Authorized humans approve exact owner batch/capability/limits/date/rollback only after 7B-05 ELIGIBLE.
- AI may not execute expansion, global switch or automatic payout.
- Output: signed decision or no change.

### Task 7B-07 — Final Controlled Rollout Monitoring dan Handoff

- Final task: post-change smoke, reconciliation, payout accuracy/aging, webhook, take rate, contribution, complaint, subsidy, calendar, incident closure, runbook/on-call handoff and signed report.
- No autonomous global rollout, historical rewrite, early rate change or AI money movement.
- Final verdict exactly one:

```text
CONTROLLED PILOT MONITORING PASS — HUMAN MAY PLAN NEXT EXPANSION
PILOT HARD STOP — AUTHORIZED HUMAN RUNBOOK REQUIRED
INSUFFICIENT OBSERVATION WINDOW/EVIDENCE
```

---

## 4. Conditional Exact-fix Cards

IDs `5B-FIX`, `5C-FIX`, and `5D-FIX` hanya dibuat bila gate fase terkait menghasilkan blocker. Aturan yang sama berlaku untuk setiap task `FIX` eksplisit:

- Root cause dan exact evidence harus berasal dari audit/gate sebelumnya.
- Fix hanya blocker yang tercatat; satu focused regression per blocker.
- Tidak boleh refactor umum, menambah endpoint/model bisnis, atau mengubah ADR diam-diam.
- P0/P1 finance/security/accounting selalu direview Codex Sol High.
- Jika fix memerlukan perubahan fund-flow, legal policy, state machine frozen, atau production authority, task dihentikan dan dikembalikan ke human approval gate.

---

## 5. Urutan Penggunaan Model per Task

### Phase 2B

| Task | Pelaksana | Reviewer |
|---|---|---|
| 2B-00 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 2B1-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B1-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B1-03 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 2B1-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B1-05 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 2B1-06 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 2B1-07 | Antigravity mengumpulkan evidence | Codex Sol High final gate |
| 2B2-00 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 2B2-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B2-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B2-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B2-04 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 2B2-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B2-06 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B2-07 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 2B2-08 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 2B2-09 | Antigravity mengumpulkan evidence | Codex Sol High final gate |
| 2B3-00 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B3-01 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 2B3-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B3-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B3-04 | Antigravity mengumpulkan evidence | Codex Sol High |
| 2B3-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 2B3-06 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 2B3-07 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 2B3-08 | Antigravity mengumpulkan evidence | Codex Sol High final gate |

### Phase 2C

| Task | Pelaksana | Reviewer |
|---|---|---|
| 2C-00 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 2C-01 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 2C-02 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 2C-03 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 2C-04 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 2C-05 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 2C-06 | Antigravity Gemini 3.1 Pro High | Sesuai severity |
| 2C-07 | Antigravity mengumpulkan evidence | Codex Sol High final gate |

### Phase 3A

| Task | Pelaksana | Reviewer |
|---|---|---|
| 3A-00 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3A-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3A-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3A-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3A-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3A-05 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 3A-06 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3A-07 | Antigravity mengumpulkan evidence | Codex Sol High |
| 3A-08 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 3A-09 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 3A-10 | Antigravity mengumpulkan evidence | Codex Sol High final gate |

### Phase 3B

| Task | Pelaksana | Reviewer |
|---|---|---|
| 3B1-00 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3B1-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3B1-02 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 3B1-03 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 3B1-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3B1-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3B1-06 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3B1-07 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 3B1-08 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3B1-09 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 3B1-10 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 3B1-11 | Antigravity mengumpulkan evidence | Codex Sol High final gate |
| 3B2-00 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 3B2-01 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 3B2-02 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 3B2-03 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 3B2-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 3B2-05 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 3B2-06 | Antigravity mengumpulkan evidence | Codex Terra High |
| 3B2-07 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 3B2-08 | Antigravity Gemini 3.1 Pro High | Sesuai severity |
| 3B2-09 | Antigravity mengumpulkan evidence | Codex Sol High final gate |

### Phase 4

| Task | Pelaksana | Reviewer |
|---|---|---|
| 4-00 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 4-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 4-02 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 4-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 4-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 4-05 | Antigravity mengumpulkan evidence | Codex Sol High |
| 4-06 | Antigravity mengumpulkan evidence | Codex Sol High |
| 4-07 | Antigravity mengumpulkan evidence | Codex Sol High |
| 4-08 | Antigravity mengumpulkan evidence | Codex Terra High + Codex Sol High gate |
| 4-09 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 4-10 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 4-11 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 4-12 | Antigravity mengumpulkan evidence | Codex Sol High final gate |

### Phase 5

| Task | Pelaksana | Reviewer |
|---|---|---|
| 5A-00 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 5A-01 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 5A-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5A-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5A-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5A-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High + human approval |
| 5B-00 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 5B-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5B-02 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 5B-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5B-04 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 5B-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5B-06 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5B-07 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5B-08 | Antigravity mengumpulkan evidence | Codex Sol High final gate |
| 5B-FIX | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 5C-00 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 5C-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5C-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5C-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5C-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5C-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5C-06 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5C-07 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5C-08 | Antigravity mengumpulkan evidence | Codex Sol High final gate |
| 5C-FIX | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 5D-00 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5D-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5D-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5D-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5D-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5D-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5D-06 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 5D-07 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 5D-08 | Antigravity mengumpulkan evidence | Codex Sol High final gate |
| 5D-FIX | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 5E-00 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 5E-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5E-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5E-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5E-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5E-05 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 5E-06 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5E-07 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5E-08 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 5E-09 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 5E-FIX | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 5E-10 | Antigravity mengumpulkan evidence | Codex Sol High final gate |

### Phase 6

| Task | Pelaksana | Reviewer |
|---|---|---|
| 6A-00 | Antigravity atau Codex read-only | Codex Sol High |
| 6A-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6A-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6A-03 | Antigravity menyiapkan evidence | Codex Sol High + human approval |
| 6B-00 | Antigravity Gemini 3.1 Pro High | Codex Terra Medium |
| 6B-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6B-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6B-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6B-04 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 6B-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6B-06 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6B-07 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6B-08 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 6B-09 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 6B-10 | Antigravity mengumpulkan evidence | Codex Sol High final gate |
| 6C-00 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6C-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6C-02 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 6C-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6C-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6C-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6C-06 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6C-07 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6C-08 | Antigravity Gemini 3.1 Pro High | Codex Terra High + Codex Sol High fail-closed review |
| 6C-09 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6C-10 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 6C-11 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 6C-12 | Antigravity mengumpulkan evidence | Codex Sol High final gate |
| 6D-00 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 6D-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 6D-02 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 6D-03 | Antigravity Gemini 3.1 Pro High | Codex Terra High; mutation reviewed Codex Sol High |
| 6D-04 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 6D-05 | Antigravity Gemini 3.1 Pro High | Codex Terra High |
| 6D-06 | Antigravity mengumpulkan evidence | Codex Sol High |
| 6D-07 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 6D-08 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 6D-09 | Antigravity mengumpulkan evidence | Codex Sol High + human exit approval |

### Phase 7

| Task | Pelaksana | Reviewer |
|---|---|---|
| 7A-00 | Antigravity atau Codex read-only | Codex Sol High |
| 7A-01 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 7A-02 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 7A-03 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 7A-04 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 7A-05 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 7A-06 | Antigravity Gemini 3.1 Pro High | Codex Terra High + human Legal/Product approval |
| 7A-07 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 7A-08 | Antigravity Gemini 3.1 Pro High | Codex Sol High |
| 7A-09 | Antigravity mengumpulkan evidence | Codex Sol High + human Operations/Security |
| 7A-10 | Antigravity atau Codex read-only | Codex Sol High lebih baik |
| 7A-11 | Antigravity Gemini 3.1 Pro High | Sesuai severity; P0/P1 Codex Sol High |
| 7A-12 | Antigravity menyiapkan GO/NO-GO packet | Codex Sol High + human multisignoff |
| 7B-00 | Human authorized operator only | Codex Sol High read-only post-check |
| 7B-01 | Antigravity read-only monitoring | Codex Sol High |
| 7B-02 | Antigravity read-only monitoring | Codex Sol High; human owns production action |
| 7B-03 | Human maker-checker; Antigravity evidence only | Codex Sol High audit |
| 7B-04 | Antigravity read-only monitoring | Codex Sol High |
| 7B-05 | Antigravity read-only evidence | Codex Sol High |
| 7B-06 | Human authorized decision only | Codex Sol High evidence check |
| 7B-07 | Antigravity read-only evidence | Codex Sol High final audit + human closure |

---

## 6. Urutan Eksekusi yang Tidak Boleh Dilompati

```text
2B-00
  → 2B1-01 → 02 → 03 → 04 → 05 → [06 bila blocker] → 07
  → 2B2-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → [08] → 09
  → 2B3-00 → 01 → 02 → 03 → 04 → 05 → 06 → [07] → 08
  → 2C-00 → 01 → 02 → 03 → 04 → 05 → [06] → 07
  → 3A-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → [09] → 10
  → 3B1-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → 09 → [10] → 11
  → 3B2-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → [08] → 09
  → 4-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → 09 → 10 → [11] → 12
  → HUMAN/RELEASE GO FOR v1.7
  → 5A-00 → 01 → 02 → 03 → 04 → 05 → HUMAN SHADOW GO
  → 5B-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → [5B-FIX → repeat 5B-08]
  → 5C-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → [5C-FIX → repeat 5C-08]
  → 5D-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → [5D-FIX → repeat 5D-08]
  → 5E-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → 09 → [5E-FIX] → 10
  → 6A-00 → 01 → 02 → 03 → HUMAN GO
  → 6B-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → [09] → 10
  → 6C-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → 09 → 10 → [11] → 12
  → 6D-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → [08] → 09 → HUMAN EXIT GO
  → 7A-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07 → 08 → 09 → 10 → [11] → 12
  → HUMAN PILOT ACTIVATION
  → 7B-00 → 01 → 02 → 03 → 04 → 05 → 06 → 07
```

Task dalam kurung siku hanya dijalankan bila audit/gate sebelumnya menemukan blocker. Task UI yang independen secara teknis tetap tidak boleh digabungkan dalam satu commit/session; urutan review tetap serial.

---

## 7. Prompt Template untuk Antigravity

Salin template ini dan ganti `<TASK_ID>` serta `<TASK_TITLE>` dari card yang dipilih:

```text
Ikuti AGENTS.md, .agents/agent_workflow.md, dan .agents/definition_of_done.md.
Baca source plan docs/version_1_7_platform_finance_implementation_plan.md
dan task cards docs/LapangGo_Phase_2B_to_Phase_7_Antigravity_Task_Cards.md.

Kerjakan hanya Task <TASK_ID> — <TASK_TITLE>.
Jangan mengerjakan task berikutnya atau improvement di luar card.

Sebelum coding:
1. Laporkan branch, commit, git status, migration tertinggi, dan baseline test terkait.
2. Tulis objective, target files, acceptance criteria, test plan, serta risiko.
3. Pastikan prerequisite dan GO task sebelumnya terbukti.

Setelah implementasi:
1. Jalankan focused test, migration/integration test pada DB disposable bila relevan,
   kemudian full verification sesuai AGENTS.md.
2. Periksa diff hanya menyentuh scope task.
3. Lampirkan actual sanitized evidence, bukan klaim umum.
4. Berhenti untuk review; jangan lanjut otomatis.

Akhiri dengan READY FOR <NEXT_TASK>, BLOCKED — <reason>, atau verdict gate yang diwajibkan card.
```

Untuk audit/evidence task, tambahkan:

```text
Task ini read-only. Jangan edit atau commit. Jangan memberi nice-to-have sebagai blocker.
Temuan harus mempunyai severity, exact file/evidence, actual behavior, expected behavior,
dan required regression test.
```

Untuk Phase 7B, tambahkan:

```text
AI hanya boleh membaca, memonitor, dan menyusun evidence.
AI tidak berwenang mengaktifkan environment, owner/cohort/LIVE term,
mengirim refund/payout, atau memperluas pilot.
Semua production mutation memerlukan authorized human operator dan change record.
```

---

## 8. Task Berikutnya

Mulai dari:

```text
Task 2B-00 — Repository Preflight dan Booking-path Inventory
```

Jangan langsung membuat migration 020 sebelum hasil 2B-00 direview karena nomor migration, dirty worktree, create-booking paths, dan fractional-price condition harus diperiksa kembali dari repository aktual.
