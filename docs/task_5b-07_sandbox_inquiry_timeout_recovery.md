# Task 5B-07 — Sandbox Inquiry/Timeout Recovery Evidence

Tanggal implementasi: 2026-07-31

## Verdict

**5B-07 PROVIDER-NEUTRAL CORE REMEDIATED — XENDIT SESSION ADAPTER BLOCKED**

Task ini hanya mengimplementasikan core recovery yang menerima
`payments.PaymentAdapter` melalui dependency injection. Tidak ada HTTP call ke
Xendit, tidak ada `customer`/`customer_id`, tidak ada credential, dan tidak ada
FakeAdapter yang di-wire ke API atau Docker runtime.

Disposable PostgreSQL/concurrency proof telah lulus. `READY FOR 5B-08` belum
diberikan karena kontrak customer Payment Session dan exact wire contract
Xendit masih harus diamendemen dan direview di 5A-04/5A-05.

## Implementasi

- DTO inquiry memiliki `ProviderSessionID` pada request dan `PayloadHash`
  sebagai evidence hash ter-normalisasi, bukan raw response.
- Outbox memiliki `ClaimNextForTypes` dan finalizer `Tx` untuk
  `RETRYABLE`, `SUCCEEDED`, dan `TERMINAL`. Finalizer `Tx` wajib menerima
  transaction dari caller dan tidak pernah membuka/commit transaction sendiri.
- Repository payment memiliki `ApplyCreateProviderResultTx`, `GetAttemptTx`,
  `TransitionStateTx`, dan `RecordCaptureTx`.
- Worker provider-neutral menjalankan create/inquiry asynchronous dengan
  lease, timeout adapter yang dapat diinjeksi, capped exponential backoff,
  deterministic create/inquiry key, strict payload decoding, dan normalized
  error taxonomy.
- Create timeout mengubah `CREATED` menjadi `PENDING`, menjadwalkan ulang row
  command yang sama, dan tidak membuat attempt atau create command kedua.
- Create result pending menyimpan identity provider yang tervalidasi dan
  enqueue maksimal satu `PAYMENT_INQUIRY` deterministic.
- Inquiry pending/timeout tetap `PENDING`; capture hanya diterima jika
  identity, amount, currency, captured time, dan SHA-256 evidence hash valid.
- Failure/expiry/cancellation memakai guarded transition; late capture tetap
  melewati immutable capture path dan tidak membuka booking.
- Mismatch dan malformed response tidak mengubah attempt. Malformed response
  menggunakan guard dua-strike outbox yang sudah ada.
- Startup guard menolak runtime `startWorkers=true` apabila payment create atau
  inquiry diaktifkan sebelum provider adapter contract siap. Flags default tetap
  `false`.
- Frontend return authority tidak diperluas dan tidak ada method selector atau
  external checkout redirect baru.

## File 5B-07

- `apps/api/internal/paymentworker/policy.go`
- `apps/api/internal/paymentworker/worker.go`
- `apps/api/internal/paymentworker/processor.go`
- `apps/api/internal/paymentworker/policy_test.go`
- `apps/api/internal/paymentworker/processor_test.go`
- `apps/api/internal/paymentworker/worker_test.go`
- `apps/api/internal/paymentworker/processor_integration_test.go`
- `apps/api/internal/payments/adapter.go` dan `adapter_test.go`
- `apps/api/internal/payments/repository.go` dan identity integration test
- perubahan transaction/normalized result pada `apps/api/internal/paymentoutbox`
  dan `apps/api/internal/payments`
- fail-closed guard pada `apps/api/cmd/api/router.go`

Perubahan 5B-06 yang sudah ada di working tree tetap dipisahkan secara logis;
tidak ada migration baru dan migration 029 tidak digunakan.

## Verification

## Remediation execution (review findings F-01—F-06)

- F-01: `PaymentInquiryScope` membedakan `CHECKOUT_SESSION` dan `PAYMENT`;
  Request ID baru hanya di-bind jika Session ID exact, lalu binding dan retry
  command yang sama commit dalam satu transaction. Capture tetap hanya berasal
  dari scope `PAYMENT` dengan Request ID tersimpan.
- F-03: reconciliation audit sekarang fail-closed; attempt lookup atau audit
  write error membatalkan transaction command.
- F-04/F-05: worker memakai `CommandClaimer`/`CommandProcessor` seam, owner
  UUID unik, lease margin, safe observer, panic isolation, dan retry jitter
  deterministik 0—10% yang capped.
- F-06: strict decoder hanya menerima `io.EOF` setelah object pertama; trailing
  syntax error ditolak.
- F-02: disposable PostgreSQL processor matrix ditambahkan pada
  `internal/paymentworker/processor_integration_test.go`, mencakup create
  timeout exact replay, inquiry uniqueness, Session→Payment handoff, audit
  rollback, stale lease, terminal identity, mismatch/malformed two-strike,
  late capture, duplicate/out-of-order, dan terminal race.
- Review lanjutan: retry delay selalu dinormalisasi ke microsecond tanpa
  melewati cap; worker menggunakan validator lease-owner yang sama dengan
  outbox; terminal Payment ID/status disimpan `NULL -> exact`; late create dan
  inquiry terminal race menyelesaikan leased command sebagai atomic no-op.
- Review P1 terbaru: worker lease dan seluruh retry-policy range divalidasi
  dengan validator duration outbox yang sama (`1us..24h` untuk lease,
  `0..24h` untuk retry, keduanya microsecond-aligned). Respons `PENDING`
  tetap boleh menghilangkan money fields, tetapi amount/currency yang hadir
  wajib exact atau menjadi reconciliation mismatch.
- Review P1 timing/identity: adapter timeout dibatasi oleh maksimum lease
  outbox dan validasi worker memakai subtraction-safe comparison sehingga
  `time.Duration` tidak dapat overflow. Terminal inquiry membandingkan seluruh
  identity provider yang sudah terikat sebelum no-op; mismatch menjadi command
  terminal dan reconciliation exception.
- Mandatory concurrency/recovery matrix sekarang menjalankan dua processor
  yang overlap setelah lease reclaim dan membuktikan tepat satu capture fact.
  Test terpisah memaksa adapter capture sukses diikuti kegagalan transaction
  lokal, lalu exact retry dengan command/idempotency key yang sama memulihkan
  satu attempt, satu command, dan satu capture fact.
- Cleanup drop/terminate database didaftarkan segera setelah
  `CREATE DATABASE`, sebelum driver, migration, atau pool dibuat.
- Session `EXPIRED`/`CANCELLED` yang pertama kali membawa Payment Request ID
  sekarang mengikat identity secara atomik, mempertahankan attempt `PENDING`,
  dan menjadwalkan ulang inquiry yang sama untuk Payment scope.
- Expired lease create/inquiry untuk attempt terminal dapat direclaim melalui
  compare-and-swap yang sama. Processor mengenali terminal state sebelum
  adapter call dan hanya menjalankan local no-op finalizer.
- Create response melewati satu classifier untuk status, money, dan provider
  identity. Invalid output dan repository `ErrInvalidCapture` menggunakan
  malformed two-strike; strike kedua menghasilkan command `TERMINAL` dengan
  `last_error_code=MALFORMED_RESPONSE`.
- Read repository sebelum provider call sekarang membedakan fakta lokal yang
  benar-benar tidak ditemukan dari gangguan database sementara.
  `ErrAttemptNotFound` diselesaikan atomik sebagai command terminal dengan
  audit `PAYMENT_COMMAND_INVARIANT_VIOLATION` yang tidak membaca ulang attempt
  yang hilang. Error infrastruktur dikembalikan tanpa mengubah lease sehingga
  command dapat direclaim.
- Provider Session, Payment Request, dan Payment ID memakai boundary bersama:
  UTF-8 valid, printable, tanpa whitespace/control character, dan maksimum 191
  byte. Guard yang sama dipakai create classifier, inquiry decision, create
  result repository, inquiry identity repository, dan capture repository.
  Identity malformed tetap mengikuti aturan two-strike dan tidak dipersist.
- Terminal local no-op tidak bergantung pada format raw provider identity.
  Identity yang valid untuk storage tetapi tidak kompatibel dengan outbox
  memakai digest `local:<attempt_id>` yang deterministik, menyelesaikan command
  `SUCCEEDED`, dan mencatat reconciliation `PROVIDER_CONTRACT_BLOCKED` dalam
  transaksi yang sama.
- Constructor processor menolak nil biasa maupun typed-nil untuk repository,
  adapter, dan audit service sebelum worker dapat dijalankan.

Targeted checks yang lulus:

```text
go test ./internal/paymentworker
go test ./internal/paymentoutbox
go test ./internal/payments
go test ./internal/audit
go test ./cmd/api
go test ./internal/paymentworker -run 'TestProcessor' -count=1
go test ./internal/database -run 'TestPaymentCreateCommandContractGuardMigration' -count=1
go test ./internal/paymentworker ./internal/paymentoutbox -run 'TestRetryPolicy|TestNewWorker' -count=20
```

Coverage `internal/paymentworker` dan repository integration suite harus
dijalankan bersama disposable PostgreSQL. Prasyaratnya adalah PostgreSQL lokal
khusus development/test sedang aktif, DSN menunjuk ke server tersebut, dan role
pada DSN memiliki izin membuat serta menghapus database disposable. Jangan
pernah arahkan command ini ke staging atau production. Dari PowerShell yang
bersih, jalankan command lengkap berikut di `apps/api`:

```powershell
$env:REQUIRE_PAYMENT_REPOSITORY_DISPOSABLE = '1'
$env:REQUIRE_PAYMENT_WORKER_DISPOSABLE = '1'
$env:TEST_ROLLBACK_HARDENING_DISPOSABLE = '1'
$env:ROLLBACK_HARDENING_TEST_DATABASE_URL = 'postgres://lapangango_user:lapangango_password@localhost:5432/lapangango_db?sslmode=disable'
try {
    1..2 | ForEach-Object {
        go test -v ./internal/payments -count=1
        if ($LASTEXITCODE -ne 0) {
            throw 'Disposable payment repository integration suite failed or was not enabled.'
        }
        go test -v ./internal/paymentworker -count=1
        if ($LASTEXITCODE -ne 0) {
            throw 'Disposable payment worker integration suite failed or was not enabled.'
        }
    }
}
finally {
    Remove-Item Env:REQUIRE_PAYMENT_REPOSITORY_DISPOSABLE -ErrorAction SilentlyContinue
    Remove-Item Env:REQUIRE_PAYMENT_WORKER_DISPOSABLE -ErrorAction SilentlyContinue
    Remove-Item Env:TEST_ROLLBACK_HARDENING_DISPOSABLE -ErrorAction SilentlyContinue
    Remove-Item Env:ROLLBACK_HARDENING_TEST_DATABASE_URL -ErrorAction SilentlyContinue
}
```

`TestPaymentRepositoryDisposableEvidenceGate` dan
`TestPaymentWorkerDisposableEvidenceGate` masing-masing mencetak marker
`PAYMENT_REPOSITORY_DISPOSABLE_SUITE_ENABLED` dan
`PAYMENT_WORKER_DISPOSABLE_SUITE_ENABLED` pada output verbose. Karena kedua
`REQUIRE_*_DISPOSABLE=1`, command gagal apabila prerequisite hilang; repository
atau worker integration suite tidak boleh diam-diam di-skip. Dengan environment
tersebut, coverage
`internal/paymentworker` yang tercatat adalah **79.3%**, melewati gate minimum
dokumentasi **75%**. Seluruh disposable `internal/payments` dan
`internal/paymentworker` suite lulus dua kali dengan `-count=1`; database residue
`lapangango_payment_*`/`lapangango_worker_*` setelah run adalah `0`.

Regresi tambahan yang lulus: `go test ./...`, `go vet ./...`, `npm.cmd run
build`, `npm.cmd run lint`, dan Vitest khusus
`src/__tests__/paymentReturnPage.test.tsx` (7 test). Suite frontend gabungan
berhenti pada script responsive karena proses Vite test harness tidak exit
sendiri; ini adalah masalah cleanup harness, bukan kegagalan assertion payment
return.

Test mencakup strict allowlisted payload, inquiry identity delegation, capture
evidence validation, retry backoff cap, Retry-After bounding, command-type
allowlist, nil-Tx rejection, worker panic/owner/timing guard, identity
decision matrix, dan startup activation guard.

## Root-cause stabilization

Stabilisasi 2026-07-31 menambahkan satu pre-provider decision boundary untuk
create, inquiry, local retry/recovery/no-op/terminal, dan lease rejection.
Create tidak lagi dipanggil ketika provider identity sudah diketahui; recovery
lokal memastikan attempt `PENDING`, satu inquiry deterministic, audit, dan
finalisasi create dalam satu transaksi. Inquiry tidak dipanggil untuk identity
kosong, credential-like, atau hierarchy Payment ID tanpa Payment Request.
Snapshot lease yang sudah kedaluwarsa ditolak sebelum adapter call.

Provider identity storage dan outbox digest sekarang memakai policy yang sama:
bounded printable UTF-8, tanpa whitespace/control character, URL, secret/key/
authorization/card/account prefix, atau numeric account-like suffix. Audit
validation dan sanitizer juga memakai satu action-specific reason validator,
sehingga reason milik action lain tidak dapat jatuh melalui enum global.

Root-cause map, ledger 38 finding, state-transition matrix, invariant, crash
points, command evidence, dan backlog tersedia pada
`docs/task_5b-07_root_cause_stabilization.md`.

Disposable-PostgreSQL concurrency/rollback proof lulus dengan migration
`28|false`. Schema Docker lokal yang telah berada di versi 28 diselaraskan ke
definisi guard migration yang sama tanpa reset data. Seluruh database
disposable `lapangango_worker_*` berhasil dibersihkan (residue `0`). Docker
Desktop tersedia dan digunakan hanya untuk PostgreSQL lokal pada proof ini;
payment flags tetap false dan tidak ada provider network call.

## Blocker yang tetap berlaku

1. Xendit Payment Session membutuhkan `customer` atau `customer_id`; kontrak
   lokal saat ini melarang pengiriman customer PII.
2. Real Xendit adapter, checkout redirect, Test Mode payment simulation,
   webhook, refund, payout, settlement, dan ledger tetap di luar task ini.
3. Sebelum 5B-08, perlu amendment customer data-flow, exact Session contract,
   lawful purpose/retention/deletion, security review, dan provider Test Mode
   evidence.
