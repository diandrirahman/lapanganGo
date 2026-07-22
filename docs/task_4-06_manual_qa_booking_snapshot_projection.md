# Task 4-06 — Manual QA Booking/Snapshot/Projection Evidence

## Verdict

`READY FOR 4-07`

All AF-P0-01..07 scenarios passed on a disposable PostgreSQL/API/web stack. No production or LIVE environment was used. No required P0 failure, duplicate fact, unexplained amount, or snapshot mutation was observed.

## Environment and isolation

- Compose project: `lapangango-task406` (disposable only).
- PostgreSQL: `127.0.0.1:15432`; API: `127.0.0.1:18080`; web: `http://localhost:13000`.
- Migration state before/after QA: `schema_migrations = version 24, dirty false`.
- Feature flags: `PLATFORM_MONETIZATION_ENABLED=false`, admin diagnostics enabled only inside the disposable stack.
- No production DSN, provider secret, or LIVE account was used or stored in this report.
- User-owned dirty files were not modified: `MabarSection.tsx`, `VenueSection.tsx`.

## Scenario matrix

| ID | Scenario and evidence | Result |
|---|---|---|
| AF-P0-01 | `TestOnlineBookingIntegration` passed normal 0/500/700 bps cases. The real UI customer flow created booking `0f8b05bf-5bf2-4bf9-bd53-a8aa409b5bd4` for `2026-07-22 17:00–18:00`, total `Rp100.000`; payment-proof UI reached `Menunggu Verifikasi`. | PASS |
| AF-P0-02 | API booking `1d482ebb-abd0-44e6-9ec3-45241c9ea8ce` with `QA406PROMO`: original `Rp200.000`, discount `Rp100.000`, final/total `Rp100.000`. DB snapshot: basis `100000`, commission `700` bps = `Rp7.000`, final `100000`, reason `PROMO:QA406PROMO`. | PASS |
| AF-P0-03 | Real owner API offline booking `09a3dc4e-948f-4966-8b7d-e21d889b9e0f`: status `PAID`, total `Rp150.000`. DB snapshot: channel `OWNER_WALK_IN`, commission bps `0`, commission `Rp0`, owner net `Rp150.000`, one owner-finance transaction. `TestOfflineBookingIntegration` also passed discount/markup/access and rollback cases. | PASS |
| AF-P0-04 | `TestBookingRetryConcurrencyRollbackMatrix` passed concurrent online exact-one booking/snapshot, concurrent offline one ledger, sequential retries, timeout-after-commit retries, and resolver/snapshot/commit orphan guards. | PASS |
| AF-P0-05 | The UI booking was submitted and verified through owner API, then fully refunded through owner API. DB: booking status `CANCELLED`, principal `Rp100.000`, snapshot commission basis `Rp100.000` at `700` bps = `Rp7.000`; owner transactions exactly `INCOME:100000`, `EXPENSE:100000`, net `Rp0`. Admin summary for `2026-07-22` reported gross `Rp100.000`, refund principal `Rp100.000`, net GMV `Rp0`, projected commission `Rp0`, refunded booking count `1`, snapshot projection count `1`. The clean exact-refund fixture also passed. | PASS |
| AF-P0-06 | `TestClassifyProjectionSourceAndRupiahMatrix` passed policy 0/500/700 bps (`Rp0`, `Rp10.000`, `Rp14.000` on a `Rp200.000` basis), promo final-basis (`Rp5.000` on `Rp100.000`, `Rp7.500` on `Rp150.000`), and historical legacy no-commission cases. `TestProjectionReadModel_HistoricalSnapshotMixedAndRefund` and the full boundary suite passed. | PASS |
| AF-P0-07 | `TestBookingFeeSnapshotRepository_E_TermChangeImmutable` passed with a `Rp100.000` booking snapshot at `700` bps / `Rp7.000`; changing the commercial term to `800` bps left the stored snapshot at `700` bps. | PASS |

## UI evidence

The browser run used the real login form, not localStorage token injection:

1. Opened `/login`.
2. Filled `Alamat Email` and `Kata Sandi` and clicked `Masuk Sekarang`.
3. Navigated through the observed venue link to `Demo GBK Alpha Field`.
4. Selected `Court Demo 1`, slot `17:00`, and continued checkout.
5. Confirmed total `Rp100.000` and submitted reference `TASK406-UI-TRANSFER`.
6. Verified visible status `Menunggu Verifikasi` and the booking ID above.

## Executed gates and raw logs

All commands used the disposable DB URL and returned exit code `0`:

```text
TEST_INTEGRATION=1 TEST_DATABASE_URL=<disposable-dsn> bookings-qa.test.exe -test.count=1 -test.run=^TestOnlineBookingIntegration$ -test.v
TEST_INTEGRATION=1 TEST_DATABASE_URL=<disposable-dsn> bookings-qa.test.exe -test.count=1 -test.run=^TestOfflineBookingIntegration$ -test.v
TEST_BOOKING_MATRIX_DISPOSABLE=1 BOOKING_MATRIX_TEST_DATABASE_URL=<disposable-dsn> bookings-qa.test.exe -test.count=1 -test.run=^TestBookingRetryConcurrencyRollbackMatrix$ -test.v
TEST_INTEGRATION=1 TEST_DATABASE_URL=<disposable-dsn> platformfinance-qa.test.exe -test.count=1 -test.run=^(TestProjectionReadModel_HistoricalSnapshotMixedAndRefund|TestClassifyProjectionSourceAndRupiahMatrix|TestBookingFeeSnapshotRepository_E_TermChangeImmutable)$ -test.v
TEST_INTEGRATION=1 TEST_DATABASE_URL=<disposable-dsn> platformfinance-qa.test.exe -test.count=1 -test.run=^TestReconciliationBoundarySuite$ -test.v
```

Raw sanitized logs are retained outside the repository at:

`D:\project\lapangGo_task406_evidence_20260722\`

Files: `AF-P0-01-online.log`, `AF-P0-03-offline.log`, `AF-P0-04-retry.log`, `AF-P0-06-07-projection-snapshot.log`, and `AF-P0-06-07-boundary.log`.

SHA-256: `AF-P0-01-online.log=6866B81D829B8192F0BDD86C2588680FCD93B6B879197B6CD3E2AE9748487C6A`; `AF-P0-03-offline.log=DA37C7E78BD05C6D7FF9CBB93A44701DD0C6B990C8BC7E4C3CCBFE3E00CC8597`; `AF-P0-04-retry.log=D6C0EC158209A1AA7B1AA7F3896843DDFC288ACCD3732C95946BD5642949BDD3`; `AF-P0-06-07-projection-snapshot.log=5F1C0E7B9E8A750941D1A3105F92E9FBD8F1D721431480546D42E4550650D15F`; `AF-P0-06-07-boundary.log=C6B01D62F5126ECA358996504E8FA2272B5F92086E58906BB88BE7BF3AD2A527`.

## Cleanup

The disposable Compose project was stopped with:

```powershell
docker compose -p lapangango-task406 -f docker-compose.yml -f docker-compose.task-4-05.yml down -v --remove-orphans --rmi local
```

Post-cleanup verification returned zero `lapangango_task406_*` containers, volumes, networks, and listeners on ports `15432/16379/18080/13000`. The persistent local stack was not stopped.
