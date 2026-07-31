# Task 5B-07 — Review Findings Remediation Implementation Plan

Status:
**IMPLEMENTATION EXECUTED — DISPOSABLE POSTGRESQL PROOF PASS**

Tanggal: 2026-07-31

Target setelah seluruh slice dan verification lulus:

**5B-07 PROVIDER-NEUTRAL CORE REMEDIATED — XENDIT SESSION ADAPTER BLOCKED**

Status `READY FOR 5B-08` tetap dilarang. Real Xendit adapter masih menunggu
amendment kontrak customer Payment Session pada 5A-04/5A-05.

## 1. Tujuan

Memperbaiki seluruh temuan review implementasi 5B-07 tanpa:

- membuat HTTP adapter Xendit;
- memanggil provider network;
- menambahkan `customer`, `customer_id`, atau customer PII;
- mengaktifkan payment worker pada runtime;
- mengaktifkan payment, webhook, refund, monetization, payout, settlement,
  transfer, atau xenPlatform;
- membuat migration baru;
- mengubah booking menjadi `PAID`;
- menyimpan raw provider response, header, error text, atau credential.

Plan ini harus cukup eksplisit agar junior programmer atau model AI yang lebih
rendah dapat mengerjakannya per slice tanpa menebak kontrak.

## 2. Temuan yang wajib ditutup

| ID | Prioritas | Temuan | Exit criteria |
|---|---|---|---|
| F-01 | P1 | Session → Payment Request identity ditemukan tetapi tidak dipersist | Identity baru terikat ke exact Session dan tersimpan atomik sebelum Payment Request inquiry |
| F-02 | P1 | Processor tidak mempunyai integration/concurrency/rollback tests | Mandatory matrix berjalan pada disposable PostgreSQL dan exact counts terbukti |
| F-03 | P2 | Error membaca attempt untuk reconciliation audit diabaikan | Semua audit lookup/write error menggagalkan transaction command |
| F-04 | P2 | Panic processor/adapter mematikan worker loop | Panic satu command direkam secara aman dan worker dapat memproses command berikutnya |
| F-05 | P2 | Worker owner/timing/retry contract belum aman | Owner unik, lease margin tervalidasi, option tidak terpakai dihapus, jitter deterministik tersedia |
| F-06 | P3 | Strict JSON decoder tidak membedakan EOF dari trailing syntax error | Hanya EOF setelah object pertama yang diterima |

## 3. Invariant yang tidak boleh berubah

### 3.1 Runtime dan provider

Seluruh flag berikut tetap `false`:

```text
PLATFORM_MONETIZATION_ENABLED=false
PAYMENT_SANDBOX_ENABLED=false
PAYMENT_CREATE_ENABLED=false
PAYMENT_INQUIRY_ENABLED=false
PAYMENT_WEBHOOK_INGRESS_ENABLED=false
PAYMENT_WEBHOOK_PROCESSOR_ENABLED=false
PAYMENT_REFUND_ENABLED=false
PAYMENT_SHADOW_RECONCILIATION_ENABLED=false
PAYMENT_ISOLATED_TEST_LEDGER_ENABLED=false
```

Aturan:

1. `FakeAdapter` hanya boleh dibuat langsung oleh unit/integration test.
2. `router.go` tidak boleh membangun `FakeAdapter`, real adapter, atau payment
   worker.
3. Startup guard yang menolak create/inquiry ketika `startWorkers=true` harus
   tetap ada.
4. Tidak boleh membaca `XENDIT_SECRET_KEY` untuk implementasi atau test.
5. Tidak boleh membuat package `internal/providers/xendit`.

### 3.2 Database

1. Migration tetap versi 28 dan `dirty=false`.
2. Jangan membuat migration 029; nomor tersebut tetap milik Phase 5C.
3. Gunakan kolom identity yang sudah ada:
   - `provider_session_id`;
   - `provider_payment_request_id`;
   - `provider_payment_id`;
   - `provider_status_code`.
4. Jangan melemahkan trigger atau unique constraint.
5. Semua result application tetap mengikuti lock order:
   booking-flow advisory lock → attempt row → command lease row → audit.

### 3.3 Payment authority

1. Browser return tidak authoritative.
2. Session completion bukan bukti capture.
3. `CAPTURED` hanya boleh melalui `RecordCaptureTx`.
4. Payment Request identity harus terikat ke Session identity milik attempt
   yang sama sebelum hasil Payment Request boleh authoritative.
5. Timeout, pending, panic, internal error, atau audit error tidak boleh
   mengarang `FAILED`.

## 4. Keputusan kontrak identity yang dibekukan oleh plan ini

Implementasi tidak boleh memilih aturan lain tanpa review senior.

### 4.1 Inquiry scope provider-neutral

Tambahkan tipe provider-neutral:

```go
type PaymentInquiryScope string

const (
    PaymentInquiryScopeCheckoutSession PaymentInquiryScope = "CHECKOUT_SESSION"
    PaymentInquiryScopePayment         PaymentInquiryScope = "PAYMENT"
)
```

Ini bukan DTO wire Xendit. Adapter masa depan bertanggung jawab mengubah
provider response menjadi scope provider-neutral tersebut.

Tambahkan field berikut ke `PaymentStatusResponse`:

```go
type PaymentStatusResponse struct {
    Scope                PaymentInquiryScope
    ProviderSessionID    string
    ProviderPaymentReqID string
    ProviderPaymentID    string
    StatusCode           string
    Status               PaymentStatus
    AmountRupiah         int64
    Currency             Currency
    CapturedAt           *time.Time
    PayloadHash          string
    ReasonCode           string
}
```

Jangan menambahkan raw body, header, URL, customer data, atau SDK type.

### 4.2 Rules untuk `CHECKOUT_SESSION`

Response Session hanya valid jika:

1. attempt mempunyai `provider_session_id`;
2. `response.ProviderSessionID` exact sama;
3. scope adalah `CHECKOUT_SESSION`;
4. `ProviderPaymentID` kosong;
5. status yang diperbolehkan:
   - `PENDING`;
   - `EXPIRED`;
   - `CANCELLED`;
6. `CAPTURED` atau `FAILED` dari Session scope ditolak sebagai
   `MALFORMED_RESPONSE`;
7. jika `ProviderPaymentReqID` pertama kali ditemukan:
   - tetap local `PENDING`;
   - persist identity tersebut;
   - reschedule command inquiry yang sama;
   - jangan menulis capture fact;
   - jangan membuat command inquiry kedua.

Session `PENDING` tanpa Payment Request identity hanya mereschedule row yang
sama.

### 4.3 Rules untuk `PAYMENT`

Response Payment hanya valid jika:

1. attempt sudah mempunyai persisted `provider_payment_request_id`;
2. response `ProviderPaymentReqID` exact sama;
3. scope adalah `PAYMENT`;
4. jika attempt sudah mempunyai `provider_payment_id`, response harus exact
   sama;
5. `PENDING`, `FAILED`, `EXPIRED`, `CANCELLED`, dan `CAPTURED` diperbolehkan;
6. `CAPTURED` wajib mempunyai:
   - exact Payment Request ID;
   - non-empty Payment ID;
   - exact amount;
   - currency `IDR`;
   - effective `CapturedAt`;
   - lowercase SHA-256 evidence hash.

Payment ID baru boleh diisi dari `NULL` ke validated value atau replay dengan
value identik. Nilai berbeda adalah `REFERENCE_MISMATCH`.

### 4.4 Authority matrix

| Scope | Status | Efek |
|---|---|---|
| Session | Pending, tanpa request ID | Retry command yang sama |
| Session | Pending, request ID baru | Persist request ID + retry atomik |
| Session | Expired/Cancelled | Guarded terminal transition |
| Session | Captured/Failed | Malformed; jangan mutasi attempt |
| Payment | Pending | Optional identity bind + retry |
| Payment | Captured | `RecordCaptureTx` + command succeeded |
| Payment | Failed/Expired/Cancelled | Guarded transition + command succeeded |
| Apa pun | Identity mismatch | Command terminal + reconciliation audit |

## 5. Target arsitektur setelah remediasi

```text
Leased PAYMENT_INQUIRY
        |
        v
Load server-owned attempt + command facts
        |
        v
FakeAdapter.GetPaymentStatus (test only, outside transaction)
        |
        +--> CHECKOUT_SESSION/PENDING/no request ID
        |        -> same command RETRYABLE
        |
        +--> CHECKOUT_SESSION/PENDING/new request ID
        |        -> bind request ID + same command RETRYABLE (one transaction)
        |
        +--> PAYMENT/PENDING
        |        -> optional identity bind + same command RETRYABLE
        |
        +--> PAYMENT/CAPTURED
        |        -> capture fact + attempt CAPTURED + command SUCCEEDED
        |
        +--> PAYMENT/FAILED|EXPIRED|CANCELLED
        |        -> guarded transition + command SUCCEEDED
        |
        +--> mismatch/malformed
                 -> attempt unchanged + terminal/retry lifecycle + audit
```

## 6. Urutan implementasi wajib

Kerjakan slice secara berurutan. Jangan menggabungkan seluruh perubahan sebelum
targeted test slice sebelumnya lulus.

### Slice 0 — Baseline dan scope protection

Mode: read-only.

Langkah:

1. Jalankan `git status --short`.
2. Catat bahwa working tree berisi perubahan 5B-06/5B-07 yang belum di-commit.
3. Jangan mereset atau menghapus perubahan tersebut.
4. Pastikan tidak ada package Xendit HTTP adapter.
5. Jalankan:

```powershell
cd apps/api
go test ./internal/paymentworker ./internal/paymentoutbox ./internal/payments ./internal/audit ./cmd/api
```

6. Catat coverage awal:

```powershell
go test -cover ./internal/paymentworker
```

Baseline yang diketahui saat review: coverage `paymentworker` sekitar 15,2%.

Acceptance:

- baseline test hijau;
- tidak ada secret di output;
- daftar file scope disepakati;
- tidak ada source edit pada slice ini.

### Slice 1 — Perbaiki strict command decoding (F-06)

File:

- `apps/api/internal/paymentworker/processor.go`;
- `apps/api/internal/paymentworker/processor_test.go`.

Implementasi:

1. Import `io`.
2. Setelah decode object pertama, lakukan decode kedua.
3. Hanya `io.EOF` yang berarti payload selesai.
4. Jika decode kedua mengembalikan `nil`, berarti ada JSON value kedua:
   tolak.
5. Jika decode kedua mengembalikan error selain `io.EOF`, berarti ada trailing
   malformed bytes: tolak.

Pseudocode:

```go
var trailing any
err := decoder.Decode(&trailing)
switch {
case errors.Is(err, io.EOF):
    // valid end
case err == nil:
    return empty, ErrMalformedCommand
default:
    return empty, ErrMalformedCommand
}
```

Test cases:

- valid object;
- valid object dengan trailing whitespace;
- unknown field;
- second JSON object;
- second scalar;
- trailing `{`;
- trailing `x`;
- empty body;
- top-level array;
- invalid amount/currency/method.

Acceptance:

- hanya satu complete JSON object yang diterima;
- test tidak bergantung database;
- tidak ada perubahan outbox schema.

### Slice 2 — Freeze inquiry response identity contract (F-01)

File:

- `apps/api/internal/payments/adapter.go`;
- `apps/api/internal/payments/adapter_test.go`;
- `apps/api/internal/paymentworker/processor_test.go`.

Implementasi:

1. Tambahkan `PaymentInquiryScope`.
2. Tambahkan `Scope`, `ProviderSessionID`, dan `StatusCode` pada
   `PaymentStatusResponse`.
3. Pertahankan enam method `PaymentAdapter`; jangan menambah operasi baru.
4. Perbarui `FakeAdapter` tests agar exact request dan response identity dapat
   diobservasi.
5. Buat pure validation function yang mengembalikan normalized decision, bukan
   boolean yang ambigu.

Tipe keputusan yang disarankan:

```go
type inquiryDecisionKind string

const (
    inquiryRetry                 inquiryDecisionKind = "RETRY"
    inquiryBindIdentityAndRetry  inquiryDecisionKind = "BIND_IDENTITY_AND_RETRY"
    inquiryCapture               inquiryDecisionKind = "CAPTURE"
    inquiryTerminalPayment       inquiryDecisionKind = "TERMINAL_PAYMENT"
    inquiryRejectMismatch        inquiryDecisionKind = "REJECT_MISMATCH"
    inquiryRejectMalformed       inquiryDecisionKind = "REJECT_MALFORMED"
)

type inquiryDecision struct {
    Kind   inquiryDecisionKind
    Reason string
}
```

Validation order harus eksplisit:

1. scope valid;
2. local attempt provider/environment;
3. Session ID;
4. Payment Request ID;
5. Payment ID;
6. allowed status untuk scope;
7. amount;
8. currency;
9. captured timestamp;
10. evidence hash.

Test matrix minimum:

- exact Session pending;
- Session pending menemukan request ID;
- wrong Session ID;
- Session capture ditolak;
- Payment response sebelum request ID dipersist ditolak;
- exact Payment pending;
- exact Payment captured;
- wrong request ID;
- wrong payment ID;
- amount/currency mismatch;
- missing captured time;
- invalid/uppercase evidence hash;
- unknown scope/status.

Acceptance:

- capture tidak mungkin berasal dari Session scope;
- request ID baru hanya diterima jika Session ID exact;
- provider-specific SDK/wire DTO tidak muncul.

### Slice 3 — Repository untuk atomic inquiry identity binding (F-01)

File:

- `apps/api/internal/payments/repository.go`;
- `apps/api/internal/payments/repository_integration_test.go`.

Tambahkan input normalized:

```go
type ApplyInquiryIdentityParams struct {
    AttemptID            string
    Provider             Provider
    ProviderEnvironment  ProviderEnvironment
    Scope                PaymentInquiryScope
    ProviderSessionID    *string
    ProviderPaymentReqID *string
    ProviderPaymentID    *string
    ProviderStatusCode   string
}
```

Tambahkan:

```go
func (r *Repository) ApplyInquiryIdentityTx(
    ctx context.Context,
    tx pgx.Tx,
    params ApplyInquiryIdentityParams,
) (PaymentAttempt, bool, error)
```

Arti bool: `true` hanya jika identity baru benar-benar diisi. Replay identik
mengembalikan `false`.

Transaction rules:

1. `tx` wajib non-nil.
2. Validate UUID, provider `XENDIT`, environment `TEST`, scope, bounded IDs,
   dan safe status code sebelum query.
3. Resolve booking ID.
4. Ambil booking-flow advisory lock.
5. Lock attempt `FOR UPDATE`.
6. Attempt harus `PENDING`.
7. Untuk Session scope:
   - current Session ID wajib ada dan exact;
   - request ID boleh `NULL -> value` atau exact replay;
   - Payment ID wajib tidak berubah.
8. Untuk Payment scope:
   - current request ID wajib ada dan exact;
   - Payment ID boleh `NULL -> value` atau exact replay.
9. Nilai berbeda mengembalikan `ErrCaptureConflict` atau error identity
   khusus yang dipetakan processor menjadi `REFERENCE_MISMATCH`.
10. Update hanya allowlisted identity dan `provider_status_code`.
11. Jangan mengubah state, checkout URL, amount, currency, expiry, captured_at,
    atau booking.
12. Method tidak begin/commit/rollback.

Integration tests:

- Session exact mengisi request ID;
- same identity replay;
- different Session ID rollback;
- different request ID rollback;
- Payment scope sebelum request ID ditolak;
- Payment scope mengisi Payment ID;
- terminal attempt tidak dimutasi;
- transaction caller rollback mengembalikan seluruh identity seperti semula;
- direct unsafe status code ditolak.

Acceptance:

- identity bind bersifat `NULL -> exact` atau exact replay;
- tidak ada schema change;
- seluruh test repository existing tetap lulus.

### Slice 4 — Implement Session → Payment handoff di processor (F-01)

File:

- `apps/api/internal/paymentworker/processor.go`;
- `apps/api/internal/paymentworker/processor_test.go`;
- `apps/api/internal/paymentworker/processor_integration_test.go`.

Flow setelah adapter response:

1. Jalankan pure inquiry decision.
2. Untuk `RETRY`:
   - mark command yang sama `RETRYABLE`;
   - state attempt tetap `PENDING`.
3. Untuk `BIND_IDENTITY_AND_RETRY`:
   - begin transaction;
   - `ApplyInquiryIdentityTx`;
   - `MarkRetryableTx` pada command ID/owner/token yang sama;
   - commit;
   - jangan enqueue command baru.
4. Untuk `CAPTURE`:
   - pastikan scope `PAYMENT`;
   - `RecordCaptureTx`;
   - `MarkSucceededTx`;
   - commit.
5. Untuk terminal payment:
   - guarded `TransitionStateTx`;
   - `MarkSucceededTx`;
   - commit.
6. Untuk mismatch:
   - attempt tidak berubah;
   - command terminal;
   - reconciliation audit atomik.
7. Untuk malformed:
   - gunakan existing two-strike lifecycle;
   - audit hanya saat menjadi terminal.

Pseudocode bind:

```go
tx, err := db.Begin(ctx)
if err != nil { return err }
defer tx.Rollback(ctx)

_, _, err = attempts.ApplyInquiryIdentityTx(ctx, tx, params)
if err != nil { return mapIdentityError(...) }

_, err = outbox.MarkRetryableTx(
    ctx, tx, command.ID, owner, token,
    "RETRYABLE_PROVIDER",
    retryPolicy.Delay(command.IdempotencyKey, command.AttemptCount, 0),
)
if err != nil { return err }

return tx.Commit(ctx)
```

Test assertions:

- first Session inquiry receives only Session ID;
- Session result discovers request ID;
- database stores request ID;
- command ID/key/hash/payload tidak berubah;
- next adapter request contains persisted request ID;
- capture sebelum handoff ditolak;
- capture setelah handoff diterima;
- jumlah command inquiry tetap satu.

Acceptance:

- handoff berjalan dua tahap dan atomik;
- tidak ada unanchored capture;
- timeout/pending tidak membuat attempt/command baru.

### Slice 5 — Audit failure harus rollback command lifecycle (F-03)

File:

- `apps/api/internal/paymentworker/processor.go`;
- `apps/api/internal/paymentworker/processor_integration_test.go`.

Perubahan:

1. `recordReconciliationTx` harus menolak command tanpa
   `PaymentAttemptID`.
2. Error `GetAttemptTx` wajib dikembalikan.
3. Error `audit.Record` wajib dikembalikan.
4. Jangan commit command terminal jika audit gagal.
5. Jangan log raw DB/audit error.

Target shape:

```go
func (p *Processor) recordReconciliationTx(...) error {
    if command.PaymentAttemptID == nil {
        return ErrMalformedCommand
    }
    attempt, err := p.attempts.GetAttemptTx(ctx, tx, *command.PaymentAttemptID)
    if err != nil {
        return err
    }
    return p.audit.Record(ctx, tx, sanitizedParams(attempt, reason))
}
```

Test dengan failing audit service:

1. lease inquiry command;
2. adapter menghasilkan mismatch;
3. audit service mengembalikan sentinel error;
4. `Process` mengembalikan sentinel;
5. command tetap `LEASED` dalam database karena transaction rollback;
6. attempt/identity/state tidak berubah;
7. audit count nol;
8. setelah lease expired, command dapat direclaim.

Acceptance:

- audit dan lifecycle command tidak dapat terpisah;
- error tidak disembunyikan.

### Slice 6 — Worker dependency seam dan panic isolation (F-04)

File:

- `apps/api/internal/paymentworker/worker.go`;
- `apps/api/internal/paymentworker/worker_test.go`.

Tambahkan interface kecil:

```go
type CommandClaimer interface {
    ClaimNextForTypes(
        context.Context,
        string,
        time.Duration,
        []paymentoutbox.CommandType,
    ) (paymentoutbox.Command, error)
}

type CommandProcessor interface {
    Process(context.Context, paymentoutbox.Command) error
    CallTimeout() time.Duration
}
```

`paymentoutbox.Repository` dan `Processor` harus memenuhi interface tanpa
adapter wrapper baru.

Tambahkan safe observer:

```go
type WorkerEvent struct {
    Code           string
    CommandID      string
    CommandType    paymentoutbox.CommandType
    PaymentAttemptID string
    AttemptCount   int
}

type WorkerObserver interface {
    Record(WorkerEvent)
}
```

Allowed event code:

- `CLAIM_FAILED`;
- `COMMAND_FAILED`;
- `COMMAND_PANIC_RECOVERED`;
- `LEASE_CONFLICT`;
- `WORKER_STOPPED`.

Aturan:

1. Jangan menyimpan atau log `error.Error()` dari adapter.
2. Jangan log panic value atau stack yang mungkin memuat payload.
3. Panic boundary berada per command, bukan mengelilingi seluruh process.
4. Command yang panic tetap `LEASED` sampai lease expiry dan kemudian dapat
   direclaim.
5. Worker loop tetap hidup.

Test tanpa database:

- empty queue tidak busy-loop;
- cancellation berhenti;
- claim error menghasilkan safe event;
- processor error tidak mematikan worker;
- processor panic pada command pertama, command kedua tetap diproses;
- observer event tidak mengandung URL, secret, payload, atau raw error;
- lease conflict bukan fatal.

Acceptance:

- worker logic dapat diuji dengan fake claimer/processor;
- tidak ada panic yang keluar dari `processOne`;
- tidak ada raw error logging.

### Slice 7 — Worker identity, timing, dan retry policy (F-05)

File:

- `apps/api/internal/paymentworker/worker.go`;
- `apps/api/internal/paymentworker/worker_test.go`;
- `apps/api/internal/paymentworker/policy.go`;
- `apps/api/internal/paymentworker/policy_test.go`;
- `apps/api/internal/paymentworker/processor.go`.

#### 7.1 Worker owner

1. Jika owner kosong, generate `worker:{uuid.NewString()}`.
2. Jika owner diberikan, validate exact `worker:{canonical-uuid}`.
3. Jangan memakai static UUID.
4. Test dua worker default menghasilkan owner berbeda.

#### 7.2 Lease margin

Tambahkan:

```go
type WorkerOptions struct {
    CommandTypes []paymentoutbox.CommandType
    Owner        string
    Lease        time.Duration
    LeaseMargin  time.Duration
    Poll         time.Duration
    Observer     WorkerObserver
}
```

Hapus `RetryPolicy` dari `WorkerOptions`; retry dimiliki `ProcessorOptions`.

Default:

- adapter timeout: 10 detik;
- lease margin: 5 detik;
- lease: 30 detik;
- poll: 1 detik.

Constructor harus menolak:

```text
lease < processor.CallTimeout() + leaseMargin
```

Gunakan safe sentinel error seperti `ErrInvalidWorkerTiming`.

#### 7.3 Deterministic jitter

Refactor retry policy:

```go
type JitterFunc func(
    key string,
    attemptCount int,
    base time.Duration,
) time.Duration

type RetryPolicy struct {
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Jitter       JitterFunc
}
```

Tambahkan method:

```go
func (p RetryPolicy) Delay(
    key string,
    attemptCount int,
    retryAfter time.Duration,
) time.Duration
```

Rules:

1. base delay exponential dan capped;
2. negative Retry-After diabaikan;
3. Retry-After di atas max dicap;
4. final delay tidak boleh lebih kecil dari bounded Retry-After;
5. default jitter deterministik berdasarkan SHA-256 dari key + attempt count;
6. jitter hanya menambah 0–10% agar tidak menjalankan retry sebelum
   Retry-After;
7. hasil akhir tetap <= max;
8. unit test dapat inject identity jitter tanpa random/sleep.

Perbarui semua call processor agar mengirim:

```go
command.IdempotencyKey
command.AttemptCount
normalized.RetryAfter
```

Test:

- same key/attempt menghasilkan delay sama;
- different keys menghasilkan deterministic spread;
- cap tidak dilewati;
- Retry-After minimum dihormati;
- negative/oversized Retry-After aman;
- tidak ada real sleep.

Acceptance:

- tidak ada option mati;
- owner unik;
- lease selalu cukup panjang;
- retry tetap deterministic untuk test.

### Slice 8 — Database-backed processor matrix (F-02)

File baru:

- `apps/api/internal/paymentworker/processor_integration_test.go`.

Gunakan pola disposable PostgreSQL yang sudah ada pada
`payments/repository_integration_test.go`.

Environment opt-in:

```text
TEST_ROLLBACK_HARDENING_DISPOSABLE=1
ROLLBACK_HARDENING_TEST_DATABASE_URL=<admin DSN disposable>
```

Jangan memakai database development aktif. Test harus:

1. membuat database unik;
2. menjalankan migration sampai 28;
3. seed user/owner/venue/court/booking/snapshot minimal;
4. cleanup connection;
5. terminate backend target database;
6. drop database pada `t.Cleanup`.

Gunakan `payments.NewFakeAdapter` dengan closure scripted. Jangan network.

#### Scenario A — Create timeout lalu exact replay

1. Seed satu attempt + one `PAYMENT_CREATE`.
2. Claim command.
3. First adapter call mengembalikan `RETRYABLE_TIMEOUT`.
4. Assert:
   - attempt `PENDING`;
   - create command `RETRYABLE`;
   - attempt count satu;
   - create command count satu;
   - inquiry count nol.
5. Claim row yang sama.
6. Assert received create request:
   - AttemptID sama;
   - idempotency key sama;
   - request hash sama;
   - amount/currency/method sama.
7. Return pending Session result.
8. Assert:
   - create command `SUCCEEDED`;
   - Session ID tersimpan;
   - tepat satu inquiry command.

#### Scenario B — Session handoff lalu capture

1. Claim inquiry.
2. Session response mengembalikan exact Session ID dan new request ID.
3. Assert:
   - attempt tetap `PENDING`;
   - request ID tersimpan;
   - inquiry row/key/hash/payload tetap sama;
   - command `RETRYABLE`.
4. Claim inquiry yang sama.
5. Assert adapter request sekarang membawa request ID.
6. Return Payment scope `CAPTURED`.
7. Assert:
   - attempt `CAPTURED`;
   - satu capture fact;
   - command `SUCCEEDED`;
   - exact captured time/hash;
   - tidak ada command baru.

#### Scenario C — Pending, timeout, failure

Subtests terpisah:

- Session pending tanpa request ID;
- inquiry timeout;
- Payment pending;
- Payment failed;
- Payment expired;
- Payment cancelled.

Assert same command/key untuk retry dan exact terminal state untuk successful
terminal inquiry.

#### Scenario D — Mismatch/malformed

Subtests:

- wrong Session ID;
- wrong request ID;
- wrong Payment ID;
- amount mismatch;
- currency mismatch;
- missing capture time/hash;
- malformed pertama;
- malformed kedua.

Assert:

- attempt tidak berubah;
- capture count nol;
- mismatch command terminal;
- malformed pertama retryable;
- malformed kedua terminal;
- sanitized audit exact count.

#### Scenario E — Audit rollback

Gunakan failing audit service sesuai Slice 5. Exact database assertions wajib.

#### Scenario F — Stale lease dan concurrency

1. Worker A claim.
2. Fake adapter A block pada channel.
3. Expire lease dengan database clock/test setup yang aman.
4. Worker B reclaim row yang sama.
5. Worker B commit valid result.
6. Release worker A.
7. Worker A mendapat `ErrLeaseConflict`.
8. Assert satu final effect dan maksimal satu capture fact.

Jangan mengubah lease token langsung kecuali test existing menunjukkan operasi
itu diizinkan oleh lifecycle guard. Prefer menunggu database-controlled lease
yang sangat pendek tetapi tetap memenuhi test processor timeout/margin yang
diinjeksi.

#### Scenario G — Late capture

1. Claim inquiry ketika attempt masih `PENDING`.
2. Block fake adapter.
3. Cancel booking atau expire local booking dalam transaction lain.
4. Bila perlu transition attempt ke `CANCELLED`/`EXPIRED`.
5. Release valid Payment capture response.
6. Assert:
   - capture fact satu;
   - attempt `CAPTURED`;
   - booking tetap cancelled/expired, tidak `PAID`;
   - reconciliation audit `LATE_CAPTURE`;
   - owner income/journal/payout rows nol.

#### Scenario H — Duplicate/out-of-order

- identical capture replay tidak membuat fact kedua;
- failure setelah captured tidak downgrade;
- terminal stale command selesai no-op;
- Payment ID berbeda ditolak.

Exact count assertions:

```text
payment_attempts = 1
payment_create_contracts = 1
PAYMENT_CREATE commands = 1
PAYMENT_INQUIRY commands <= 1
payment_capture_facts <= 1
legacy booking payments = 0
actual journal rows = 0
payout/transfer/owner income rows = 0
```

Acceptance:

- seluruh mandatory matrix plan lama tercakup;
- test tidak melakukan network;
- test residue nol;
- coverage `paymentworker` meningkat material; target minimum 75% untuk
  statement package, atau setiap branch risiko tinggi memiliki test eksplisit
  jika target tidak tercapai.

### Slice 9 — Full regression dan evidence update

Backend:

```powershell
cd apps/api
go test ./internal/paymentworker
go test ./internal/paymentoutbox
go test ./internal/payments
go test ./internal/audit
go test ./cmd/api
go test ./...
go vet ./...
```

Disposable integration:

```powershell
$env:TEST_ROLLBACK_HARDENING_DISPOSABLE='1'
$env:ROLLBACK_HARDENING_TEST_DATABASE_URL='<admin disposable DSN>'
go test ./internal/paymentworker -run 'TestProcessor' -count=1
```

Frontend regression:

```powershell
cd ../web
npm.cmd run lint
npm.cmd run build
npx.cmd vitest run --environment jsdom src/__tests__/paymentReturnPage.test.tsx
```

Docker proof hanya jika Docker daemon tersedia:

1. seluruh payment flag tetap false;
2. API/web startup sehat;
3. zero provider call;
4. activation guard test tetap menolak create/inquiry worker.

Security scans:

```powershell
rg -n "internal/providers/xendit|customer_id|Authorization|Basic " apps/api/internal/paymentworker apps/api/internal/payments/adapter.go
rg -n "VITE_.*PAYMENT|VITE_.*XENDIT" apps/web
```

Interpretasi:

- dokumentasi boleh menyebut `customer_id` sebagai blocker;
- source provider request DTO tidak boleh mempunyai field tersebut;
- jangan mencetak `.env` atau secret untuk membuktikan scan.

Update:

- `docs/task_5b-07_sandbox_inquiry_timeout_recovery.md`;
- jangan mengubah verdict menjadi `READY FOR 5B-08`;
- catat exact test commands, counts, migration version, dan Docker limitation.

## 7. Definition of Done remediasi

Semua checkbox wajib:

- [ ] F-01 Session identity exact dan Payment Request handoff atomik.
- [ ] Capture hanya dari Payment scope dengan persisted request ID.
- [ ] Session scope tidak dapat menghasilkan capture.
- [ ] Pending/timeout memakai inquiry row dan key yang sama.
- [ ] F-02 processor integration matrix lulus pada disposable PostgreSQL.
- [ ] Concurrency menghasilkan satu final effect.
- [ ] Stale lease tidak dapat commit.
- [ ] Audit failure rollback seluruh local effect.
- [ ] F-03 audit lookup error tidak disembunyikan.
- [ ] F-04 panic satu command tidak mematikan worker.
- [ ] Panic/error observer tidak membocorkan raw value.
- [ ] F-05 worker owner unik dan tervalidasi.
- [ ] Lease lebih panjang dari adapter timeout + margin.
- [ ] Retry jitter deterministic dan capped.
- [ ] Tidak ada unused worker option.
- [ ] F-06 decoder hanya menerima EOF setelah object.
- [ ] Late capture tidak membuka booking.
- [ ] Terminal state tidak downgrade.
- [ ] Exact counts menunjukkan tidak ada duplicate attempt/command/capture.
- [ ] Tidak ada actual finance, payout, transfer, atau owner income write.
- [ ] Tidak ada migration baru.
- [ ] Semua runtime payment flags tetap false.
- [ ] Tidak ada real provider adapter/network/secret/PII.
- [ ] Targeted tests, full Go tests, vet, frontend lint/build/return test lulus.
- [ ] Evidence document diperbarui secara faktual.

## 8. Stop conditions

Junior/model AI harus berhenti dan meminta review senior jika:

1. identity handoff membutuhkan `customer` atau `customer_id`;
2. implementasi mulai membuat HTTP request Xendit;
3. diperlukan migration baru atau perubahan migration 025–028;
4. trigger/constraint harus dilemahkan;
5. capture hanya bisa diproses tanpa persisted Payment Request identity;
6. atomic bind + command retry tidak dapat dilakukan dalam satu transaction;
7. test membutuhkan real secret/provider network;
8. payment worker harus di-wire ke runtime agar test lulus;
9. booking harus diubah menjadi `PAID`;
10. raw provider payload/error harus disimpan atau dilog;
11. unrelated dirty worktree harus ditimpa;
12. disposable database tidak dapat dijamin aman untuk dibuat/dihapus.

## 9. Suggested commit sequence

Jangan commit sebelum test slice terkait lulus.

1. `test/payments: freeze 5B-07 remediation contracts`
2. `fix/payments: require strict command payload EOF`
3. `fix/payments: bind session and payment inquiry identities`
4. `fix/payments: rollback terminal lifecycle when audit fails`
5. `fix/payments: isolate payment worker command panics`
6. `fix/payments: enforce worker identity lease and jitter policy`
7. `test/payments: prove 5B-07 processor recovery atomically`
8. `docs/payments: record 5B-07 remediation evidence`

Jangan memasukkan `.env`, Go cache, frontend `dist`, test database, screenshot
credential, atau unrelated cleanup.

## 10. Instruksi ringkas untuk junior/model AI

Untuk setiap slice:

1. baca file existing dan test terdekat;
2. tulis failing test terlebih dahulu;
3. implementasikan perubahan terkecil;
4. jalankan targeted test;
5. inspect diff;
6. pastikan adapter call tetap di luar DB transaction;
7. pastikan identity baru hanya `NULL -> exact` atau exact replay;
8. pastikan leased command difinalisasi di transaction domain yang sama;
9. pastikan error audit menggagalkan commit;
10. pastikan tidak ada raw provider/PII/secret;
11. lanjut ke slice berikutnya hanya setelah acceptance slice hijau.

Jika hasil provider ambigu, pertahankan local `PENDING`, reschedule command yang
sama, dan catat signal operasional yang aman. Jangan mengarang capture atau
failure.

## 11. Hasil eksekusi plan

Slice 1—8 telah diimplementasikan. Processor integration matrix dijalankan
pada PostgreSQL disposable melalui
`TEST_ROLLBACK_HARDENING_DISPOSABLE=1` dan lulus. Matrix tersebut mencakup
create/timeout exact replay, Session→Payment handoff, Session dan Payment
terminal outcomes, terminal Payment-ID binding, malformed two-strike,
mismatch, audit rollback, stale lease, late capture, duplicate/out-of-order,
serta terminal race saat adapter call.

Review lanjutan juga menutup drift retry-delay/lease-owner dan cleanup database
disposable. Guard migration 028 yang masih menjadi bagian working tree task
dikoreksi agar syarat attempt `PENDING` berlaku saat inquiry command dibuat,
sementara lifecycle legal command yang sudah leased tetap dapat diselesaikan
setelah terminal race. Tidak ada migration baru atau nomor 029.

Koreksi 028 ini merupakan tindak lanjut eksplisit atas review terminal-race,
setelah stop condition plan awal ditulis. Migration 028 belum menjadi baseline
Git yang dirilis; fresh disposable migration dan schema Docker lokal versi 28
keduanya telah diselaraskan ke definisi guard yang sama. Constraint identity,
payload, lease, dan larangan insert inquiry untuk attempt terminal tetap aktif.

Verification akhir:

```text
go test ./internal/paymentworker -run 'TestProcessor' -count=1
go test ./internal/database -run 'TestPaymentCreateCommandContractGuardMigration' -count=1
go test ./...
go test ./internal/paymentworker ./internal/paymentoutbox -run 'TestRetryPolicy|TestNewWorker' -count=20
```

Database runtime menunjukkan `28|false`, definisi guard lokal memuat kondisi
`TG_OP = 'INSERT'`, dan tidak ada database `lapangango_worker_*` tersisa.
Runtime payment tetap disabled; tidak ada provider network, credential, PII,
payout, settlement, atau finance write.
