# Task 2B3-02 — Pre-cutover Idempotent Apply Final Report

## Status

`COMPLETED`

## Baseline

- Branch: `master`
- Baseline commit: `57d4ae46a`
- Highest migration: `021_platform_finance_cutover_guard.up.sql`
- Target fixture: disposable clone of the sanitized synthetic staging database

## Implementation

### Apply safety guards

- `--apply=true` is mutually exclusive with `--dry-run=true`.
- Apply requires `--non-production-confirmed=true`.
- Apply requires a nonnegative `--expected-candidate-count`.
- `BACKFILL_TARGET_ENVIRONMENT` only accepts `development` or `staging`.
- `BACKFILL_EXPECTED_DATABASE_NAME` must exactly match `current_database()`.
- The reviewed cutover timestamp must exactly match the stored cutover.
- Database and PostgreSQL errors are not printed to CLI output.

### Transaction and concurrency contract

- A session advisory lock prevents concurrent backfill runners.
- Every write batch uses `REPEATABLE READ` and a bounded context.
- Booking rows are locked using `FOR SHARE OF b`.
- Offline price rows are locked separately after candidate rows are closed.
- Locked offline prices and reasons are compared with the in-memory candidate.
- Batch counters and cursors advance only after confirmed commit.
- Ambiguous commit outcomes discard the physical connection and return nonzero.
- Advisory unlock failures discard the physical connection and return nonzero.

### Legacy snapshot contract

- Money is parsed from `pgtype.Numeric` without float conversion.
- Fractional, negative, NaN, infinity, and out-of-range values fail closed.
- Positive decimal exponents are evaluated exactly with `big.Int`.
- Online and owner walk-in price-source fallbacks are deterministic.
- Nonzero adjustments require the appropriate promo or offline override reason.
- Backfilled snapshots use:
  - `terms_source=LEGACY_NO_COMMISSION`
  - `finance_mode=SIMULATION`
  - `commercial_term_id=NULL`
  - zero customer service fee and commission
  - `calculation_version=legacy-backfill-v1`

### Repository separation

- Runtime `BookingFeeSnapshotRepository` remains unchanged for booking creation.
- Backfill idempotency uses the separate `LegacyBackfillSnapshotRepository`.
- Exact existing snapshots become idempotent no-ops.
- Mismatched existing snapshots return `ErrBackfillSnapshotConflict`.

## Integration-test isolation

- Every apply test creates a uniquely named disposable PostgreSQL database cloned from the sanitized fixture.
- Tests do not update or delete immutable snapshot rows for cleanup.
- Each clone is forcibly dropped with a bounded cleanup context.
- Cleanup failure fails the test.
- The source fixture remains unchanged after focused and repeated runs.

## Automated matrix

The following tests pass:

- `TestCLIMain`
- `TestApply`
- `TestApply_BatchFailureRollback`
- `TestApply_ConcurrentRunnerRejected`
- `TestApply_ConflictExactAndMismatch`
- `TestApply_UnlockCleanup`
- `TestApply_UnlockFailure`
- `TestApply_CommitOutcomeUnknown`
- `TestApply_NoCollateralMutation`

## Verification evidence

### Apply matrix

```text
go test -count=1 -timeout=300s ./cmd/backfill-platform-finance \
  -run '^(TestCLIMain|TestApply|TestApply_.*)$'

PASS
```

### Apply stability

```text
go test -count=3 -timeout=600s ./cmd/backfill-platform-finance \
  -run '^(TestCLIMain|TestApply|TestApply_.*)$'

PASS
```

### Platform-finance stability

```text
go test -count=3 -timeout=300s ./internal/platformfinance/...

PASS
```

### Full API regression

```text
go test -count=1 -timeout=300s ./...

PASS
```

### Static verification

```text
go vet ./...       PASS
git diff --check   PASS
```

## Reconciliation

The source sanitized fixture remains at its frozen baseline after all tests:

```text
bookings=10
snapshots=3
legacy_snapshots=0
remaining_candidates=7
disposable_test_databases=0
idle_cli_transactions=0
advisory_locks=0
```

Each successful apply clone proves:

```text
processed=7
inserted=7
idempotent_noop=0
commit_outcome=CONFIRMED
```

Its immediate rerun proves:

```text
processed=0
inserted=0
idempotent_noop=0
commit_outcome=CONFIRMED
```

## Skipped or unverified

- No production or representative historical production database was modified.
- Production-data readiness remains gated by a representative staging/production audit.

## Verdict

`READY TO COMMIT`
