# Task 5B-07 — Sandbox Inquiry/Timeout Recovery: Implementation Plan

Status dokumen:
**IMPLEMENTATION EXECUTED — REVISED, CONTRACT-SAFE CORE; XENDIT ADAPTER TETAP BLOCKED**

Revision date: 2026-07-31

Target keluaran yang diizinkan oleh kontrak saat ini:
**5B-07 PROVIDER-NEUTRAL CORE READY — XENDIT SESSION ADAPTER BLOCKED**

Status **READY FOR 5B-08** tidak boleh diberikan sebelum kontrak customer
Payment Session diamendemen di 5A-04/5A-05 dan adapter Xendit Test Mode
melewati review ulang.

Runtime yang diizinkan selama plan revisi ini:
**semua payment flags tetap `false`; provider hanya `FakeAdapter` di automated
test terisolasi**

Konfigurasi yang tidak boleh berubah:

- `PLATFORM_MONETIZATION_ENABLED=false`;
- xenPlatform, split payment, transfer, payout, settlement, dan Money Out tetap
  tidak aktif;
- webhook ingress/processor, refund, shadow reconciliation, serta isolated test
  ledger tetap di luar scope 5B-07;
- secret hanya dibaca backend dari `apps/api/.env`, tidak pernah ditulis ke
  source, log, fixture, frontend, `VITE_*`, dokumentasi, atau Git.

## 0. Konflik kontrak yang sudah terkonfirmasi

[Dokumentasi resmi Xendit `POST /sessions`](https://docs.xendit.co/apidocs/create-session),
diperbarui 28 Juli 2026, menyatakan Session membutuhkan `customer` atau
`customer_id`. Dokumentasi tersebut juga mendefinisikan error
`MISSING_CUSTOMER` apabila keduanya tidak disediakan.

Kontrak 5A-05 yang sudah dibekukan menyatakan:

- adapter harus menghilangkan customer PII;
- implementasi harus berhenti jika Xendit membutuhkan customer field;
- customer PII, sub-account, xenPlatform, split rule, dan saved payment method
  tidak boleh ditambahkan diam-diam.

Keputusan plan revisi:

1. Kontrak 5A-05 dipertahankan; plan ini tidak mengamendemen security/privacy
   contract.
2. Jangan membuat `POST /sessions` HTTP adapter yang mengirim payload tanpa
   customer karena payload itu diketahui tidak valid.
3. Jangan menambahkan `customer`, `customer_id`, email, nomor telepon, nama,
   atau synthetic PII hanya agar request provider diterima.
4. Implementasikan dan buktikan recovery core dengan provider-neutral
   `FakeAdapter` pada test process terisolasi.
5. Jangan wire `FakeAdapter` ke runtime API/Docker.
6. Runtime harus fail closed apabila payment create/inquiry mencoba
   diaktifkan sebelum adapter contract dinyatakan siap.
7. Real Xendit adapter, checkout redirect, dan manual provider proof menjadi
   pekerjaan bersyarat setelah amendment 5A-04/5A-05.

## 1. Tujuan

Membangun dan membuktikan core worker/recovery provider-neutral untuk command
yang sudah dibuat oleh Task 5B-06, tanpa melakukan provider HTTP call dan
tanpa melanggar kontrak customer/PII.

Implementasi harus membuktikan bahwa:

1. HTTP customer hanya membuat/replay local attempt dan outbox command; HTTP
   customer tidak memanggil provider secara synchronous.
2. Worker core dapat menjalankan `PAYMENT_CREATE` melalui injected
   provider-neutral adapter menggunakan deterministic key yang sama.
3. Timeout tidak pernah dianggap gagal. Attempt harus tetap `PENDING`.
4. Create yang timeout dipulihkan dengan mengulang request create yang sama,
   dengan provider idempotency key yang sama, bukan membuat pembayaran baru.
5. Setelah identitas provider diketahui, satu command
   `PAYMENT_INQUIRY` dengan key
   `payment:inquiry:{payment_attempt_id}` melakukan status inquiry terhadap
   payment yang sama.
6. Pada automated test terisolasi, hanya normalized inquiry result dari
   trusted injected adapter boundary yang cocok dengan attempt lokal yang
   boleh menghasilkan `CAPTURED`, `FAILED`, `EXPIRED`, atau `CANCELLED`.
7. Hasil inquiry `PENDING` atau timeout tetap `PENDING` dan dijadwalkan ulang
   dengan command serta key yang sama.
8. State terminal tidak dapat diturunkan atau dibuka kembali. Pengecualian
   satu-satunya adalah late capture yang sudah dibekukan dalam state machine:
   terminal failure/expiry/cancellation dapat menjadi `CAPTURED` setelah bukti
   capture valid, tetapi booking tidak dibuka kembali.
9. Mutasi attempt/capture fact, lifecycle command, dan audit hasil provider
   commit atau rollback sebagai satu transaksi lokal.
10. Browser redirect/return hanya untuk navigasi. Path `success` atau `cancel`
    tidak mempunyai otoritas pembayaran.
11. Runtime API tidak menjalankan worker/provider adapter sampai amendment
    kontrak customer selesai.

## 2. Bukan tujuan task ini

Jangan mengerjakan hal berikut dalam 5B-07:

- webhook endpoint, webhook inbox, atau webhook processor;
- refund atau provider cost;
- booking projection menjadi `PAID`;
- owner income, owner cashbook, payout, transfer, atau settlement;
- actual/production journal;
- Live Mode atau xenPlatform;
- real Xendit HTTP request;
- real atau synthetic customer creation di Xendit;
- `customer`, `customer_id`, email, nomor telepon, atau nama dalam payload
  provider;
- menyimpan raw provider body, header, error text, credential, customer PII,
  data rekening, PAN, CVV, atau token kartu;
- mengubah migration lama yang sudah pernah dijalankan;
- menggunakan migration `029`, karena nomor itu sudah dibekukan untuk
  `payment_webhook_inbox` pada Phase 5C;
- polling provider langsung dari browser;
- menerima status pembayaran dari query string, success URL, cancel URL,
  localStorage, atau request body customer;
- customer method selector yang mengarahkan ke hosted checkout sebelum adapter
  contract disetujui.

## 3. Prasyarat sebelum programmer mulai

Programmer harus berhenti dan melapor apabila salah satu prasyarat berikut
tidak terpenuhi.

### 3.1 Repository

- Branch adalah `master`.
- Perubahan 5B-06 sudah direview dan di-commit terlebih dahulu.
- `git status --short` bersih sebelum implementasi 5B-07.
- Migration PostgreSQL menunjukkan `28|false`.
- Migration 025 sampai 028 tersedia dan tidak dimodifikasi ulang.
- Test backend dan frontend dari 5B-06 masih lulus.

Catatan untuk kondisi repository saat plan ini dibuat: perubahan 5B-06 masih
ada di working tree. Jangan mencampur commit plan/5B-07 dengan perubahan
tersebut sebelum baseline 5B-06 dipastikan.

### 3.2 Provider

- Dashboard boleh tetap berada di Xendit Test Mode, tetapi plan revisi tidak
  memanggilnya.
- Jangan membaca atau memasukkan Test secret saat menjalankan slice
  provider-neutral.
- Jangan mengaktifkan `PAYMENT_SANDBOX_ENABLED`, `PAYMENT_CREATE_ENABLED`, atau
  `PAYMENT_INQUIRY_ENABLED` pada API/Docker.
- Gunakan `payments.FakeAdapter` dengan scripted normalized result hanya pada
  unit/integration test.
- Fixture hanya merepresentasikan provider-neutral response; fixture tidak
  boleh diklaim sebagai bukti exact Xendit wire contract.
- Exact `POST /sessions`, `GET /sessions/{id}`, dan
  `GET /v3/payment_requests/{id}` tetap dicatat sebagai calon surface, bukan
  endpoint yang diimplementasikan oleh plan revisi.
- Amendment berikutnya harus menetapkan apakah LapangGo:
  - membuat/stores Xendit `customer_id`;
  - mengirim customer object per payment;
  - memakai identifier pseudonymous;
  - atau memilih provider/surface lain yang tidak membutuhkan customer field.
- Amendment harus menetapkan lawful purpose, data minimization, consent/notice,
  retention, deletion, redaction, access control, incident handling, dan
  mapping user deletion sebelum adapter nyata boleh dibuat.

### 3.3 Runtime flags

Selama coding, migration, unit/integration test, dan Docker verification, semua
flag tetap `false`. Test behavior menginjeksi options/dependency secara
langsung dan tidak mengubah process environment:

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
PAYMENT_PROVIDER=XENDIT
PAYMENT_PROVIDER_MODE=TEST
```

## 4. Baseline yang sudah tersedia

Jangan membuat ulang fondasi berikut:

- `payments.PaymentAdapter` dan `FakeAdapter`;
- normalized payment statuses dan normalized adapter errors;
- `payment_attempts` dan immutable `payment_capture_facts`;
- `payment_provider_commands`;
- command types `PAYMENT_CREATE` dan `PAYMENT_INQUIRY`;
- deterministic create/inquiry keys;
- allowlisted outbox payload;
- lease token, retry, terminal, dan succeeded lifecycle;
- create-payment local orchestrator;
- immutable create contract berisi expiry dan return URL;
- customer status endpoint;
- opaque return-reference resolver;
- frontend return page yang hanya membaca local status;
- late-capture guard;
- sandbox/legacy isolation guard.

Kekurangan yang harus ditutup oleh 5B-07:

- belum ada worker yang claim dan menjalankan command;
- belum ada penyimpanan provider create result secara atomik;
- belum ada enqueue dan pemrosesan inquiry result secara atomik;
- lifecycle command saat ini selesai dalam transaksi sendiri dan perlu
  primitive `Tx` agar domain result + command result + audit tidak terpisah;
- audit allowlist saat ini hanya menerima
  `PAYMENT_COMMAND_ENQUEUED` untuk `PAYMENT_CREATE`, belum
  `PAYMENT_INQUIRY`.

Kekurangan yang **tidak** ditutup plan revisi:

- Xendit Test Mode HTTP adapter;
- customer/customer-ID privacy contract;
- runtime provider worker activation;
- customer hosted-checkout method selector;
- real checkout URL;
- real Test Mode success/failure simulation.

Kekurangan tersebut menjadi blocker eksplisit untuk `READY FOR 5B-08`, bukan
alasan untuk mengurangi validation atau mengirim payload provider yang tidak
sesuai.

## 5. Keputusan desain yang wajib diikuti

### 5.1 Adapter call selalu di luar database transaction

Urutan wajib untuk core worker. Pada plan revisi, adapter yang dipanggil hanya
`FakeAdapter` di test process:

1. claim command dengan lease;
2. load immutable provider input;
3. panggil injected adapter tanpa transaction terbuka;
4. mulai transaction hasil;
5. lock booking-flow, attempt, lalu command;
6. validasi lease dan current state;
7. tulis domain result/capture fact, lifecycle command, dan audit;
8. commit.

Jangan menahan transaction atau row lock selama adapter call. Ketika real
adapter akhirnya diizinkan, aturan yang sama berlaku untuk HTTP request.

### 5.2 Create-timeout recovery

Apabila scripted `CreatePayment` mengembalikan normalized timeout:

- `CREATED` berubah menjadi `PENDING`;
- create command menjadi `RETRYABLE` dengan `RETRYABLE_TIMEOUT`;
- retry menggunakan request hash dan provider idempotency key yang sama;
- jangan membuat payment attempt baru;
- jangan menaikkan `attempt_no`;
- jangan membuat `PAYMENT_CREATE` command baru;
- jangan membuat inquiry yang tidak mempunyai provider session/request
  identity.

Setelah retry create yang sama pada test mengembalikan simulated provider
identity:

- simpan identity dan safe Test checkout URL;
- tandai create command `SUCCEEDED`;
- enqueue tepat satu `PAYMENT_INQUIRY` di transaksi hasil yang sama.

### 5.3 Inquiry recovery

Inquiry core hanya boleh memakai provider identity milik attempt yang sama.
Pada plan revisi seluruh identity bersifat sintetis dan hanya berada pada
database test disposable:

- gunakan Session inquiry apabila baru ada `provider_session_id`;
- jika Session `COMPLETED` memberikan payment request identity, lanjutkan ke
  Payment Request inquiry;
- Session `COMPLETED` sendiri tidak membuktikan capture;
- gunakan Payment Request inquiry untuk membuktikan
  `SUCCEEDED`/capture;
- jangan memanggil create endpoint dari handler customer atau inquiry worker.

### 5.4 Satu command inquiry, dijadwalkan ulang

Database sudah membatasi satu `PAYMENT_INQUIRY` per payment attempt. Karena itu:

- `PENDING`, timeout, rate-limit, dan transient provider error menjadwalkan
  ulang row yang sama;
- jangan insert inquiry kedua;
- jangan mengubah idempotency key;
- jangan mengubah request hash atau payload;
- jangan menggunakan terminal state hanya karena retry sudah banyak;
- gunakan capped exponential backoff; attempt lama tetap `PENDING` dan
  menghasilkan signal stale/unresolved untuk operasi.

Schema lifecycle saat ini mensyaratkan normalized retry error pada state
`RETRYABLE`. Untuk hasil provider yang masih pending, gunakan code
`RETRYABLE_PROVIDER` pada command yang sama, tetapi jangan mengubah local
payment state. Jangan menambah error code baru atau mengubah migration lama
hanya untuk membedakan hasil pending.

### 5.5 Hasil provider sukses tidak sama dengan command `SUCCEEDED`

Arti lifecycle command:

- `SUCCEEDED`: provider operation dan seluruh local result sudah commit;
- `TERMINAL`: command tidak dapat dijalankan karena contract/config/security
  error;
- local payment `FAILED`, `EXPIRED`, atau `CANCELLED` tetap dapat dihasilkan
  oleh command yang `SUCCEEDED`, karena provider inquiry berhasil dan
  mengembalikan status terminal yang valid.

Jangan menandai command `TERMINAL_PROVIDER` hanya karena payment result adalah
`FAILED`.

### 5.6 Provider mismatch tidak boleh mengubah attempt

Untuk reference, amount, currency, atau provider identity mismatch:

- local state tidak berubah;
- tidak ada capture fact;
- tidak ada checkout URL baru;
- leased command menjadi `TERMINAL` menggunakan normalized mismatch code;
- tulis sanitized reconciliation/security audit;
- jangan menyimpan raw body atau raw error.

### 5.7 Browser bukan authority

- Frontend boleh redirect hanya ke `checkout_url` yang dikembalikan status API.
- Backend tetap memfilter URL ke hostname Test Mode yang sudah diizinkan.
- Browser return `success` dan `cancel` hanya memilih teks/status tampilan.
- Return page selalu membaca `GET /payment-attempts/...`.
- Frontend tidak pernah mengirim state `CAPTURED`, `FAILED`, `EXPIRED`, atau
  `CANCELLED` ke backend.

## 6. Target arsitektur yang diizinkan

```text
Customer POST create attempt
        |
        v
payment_attempt + PAYMENT_CREATE command + audit (atomic)
        |
        v
Payment worker claims command
        |
        +--> injected FakeAdapter call (test only, no DB transaction)
        |
        v
Apply create result + command lifecycle + inquiry enqueue + audit (atomic)
        |
        v
PAYMENT_INQUIRY command, same attempt/key
        |
        +--> authenticated Xendit status call (no DB transaction)
        |
        v
Validate identity + amount + currency
        |
        +--> PENDING: reschedule same command, state remains PENDING
        +--> CAPTURED: immutable capture fact, attempt CAPTURED
        +--> FAILED/EXPIRED/CANCELLED: guarded terminal transition
        +--> mismatch: reject result, command terminal, attempt unchanged
```

Runtime production-like/Docker:

```text
payment flags=false
        |
        v
no provider adapter + no payment worker
        |
        v
attempted activation => startup/dependency setup fails closed
```

Diagram ini sengaja tidak memuat `POST /sessions`. Real Xendit wire flow baru
boleh ditambahkan setelah amendment kontrak customer disetujui.

## 7. Rencana implementasi per slice

Setiap slice harus selesai dan lulus targeted test sebelum lanjut ke slice
berikutnya.

### Slice 0 — Preflight dan test inventory

Tidak ada source edit pada slice ini.

Langkah:

1. Jalankan `git status --short`.
2. Pastikan baseline 5B-06 sudah menjadi commit terpisah.
3. Catat migration version/dirty.
4. Jalankan:

```powershell
cd apps/api
go test ./internal/payments
go test ./internal/paymentoutbox
go test ./internal/audit
go test ./cmd/api

cd ../web
npm test
npm run build
```

5. Buat daftar test 5B-07 sebelum coding:
   create timeout, create replay recovery, inquiry pending, capture, failure,
   expiry, cancellation, mismatch, duplicate, late capture, stale lease,
   rollback, flag-off, dan browser non-authority.

Acceptance:

- baseline hijau;
- tidak ada secret di output;
- tidak ada residue dari test;
- scope file 5B-07 disepakati.

### Slice 1 — Lengkapi provider-neutral inquiry evidence

File utama:

- `apps/api/internal/payments/adapter.go`;
- `apps/api/internal/payments/adapter_test.go`.

Perubahan:

1. Tambahkan `ProviderSessionID` ke `GetPaymentStatusRequest`, karena Session
   inquiry memerlukan ID tersebut.
2. Tambahkan evidence hash yang aman ke `PaymentStatusResponse`, misalnya
   `PayloadHash`, berupa SHA-256 lowercase dari response body yang telah
   diterima adapter.
3. Jangan menambahkan raw body, raw header, merchant secret, checkout URL,
   customer data, atau provider SDK type ke DTO provider-neutral.
4. Pertahankan interface enam operasi yang sudah dibekukan.
5. Perbarui fake adapter tests untuk memastikan seluruh field inquiry
   didelegasikan persis dan tidak berubah.

Validation:

- provider session/request/payment ID panjang 1–191 byte jika terisi;
- minimal satu session/request/payment identity harus tersedia sebelum GET;
- payload hash tepat 64 lowercase hexadecimal;
- `CapturedAt` wajib hanya untuk `CAPTURED`;
- amount positif dan currency `IDR` untuk authoritative Payment Request result.

Acceptance:

- adapter DTO cukup untuk Session lalu Payment Request inquiry;
- tidak ada provider-specific DTO bocor keluar adapter;
- seluruh existing adapter test tetap lulus.

### Slice 2 — Freeze provider blocker dan scripted FakeAdapter scenarios

Jangan membuat package `internal/providers/xendit` pada plan revisi ini.
Jangan membuat `http.Client`, Basic Auth request, `/sessions` payload, atau
provider fixture yang diklaim sebagai exact wire response.

File:

- `apps/api/internal/payments/adapter_test.go`;
- file test helper baru di `apps/api/internal/paymentworker`, jika diperlukan;
- dokumen evidence 5B-07.

Buat reusable scripted scenarios menggunakan existing
`payments.NewFakeAdapter`:

1. create returns `PENDING` dengan synthetic Session ID dan Test checkout URL;
2. create returns `RETRYABLE_TIMEOUT`, kemudian replay create dengan exact
   request/key yang sama returns `PENDING`;
3. inquiry returns `PENDING`;
4. inquiry timeout kemudian `CAPTURED`;
5. inquiry timeout kemudian `FAILED`;
6. inquiry returns `EXPIRED`;
7. inquiry returns `CANCELLED`;
8. inquiry returns amount mismatch;
9. inquiry returns currency mismatch;
10. inquiry returns provider request/payment ID mismatch;
11. inquiry returns malformed response sekali dan dua kali;
12. inquiry returns late capture after local cancellation/expiry.

Aturan synthetic evidence:

- semua provider IDs harus palsu dan bounded;
- payload hash berupa SHA-256 atas canonical synthetic evidence, bukan klaim
  hash raw Xendit body;
- checkout URL hanya hostname Test Mode allowlist yang sudah ada;
- tidak ada customer/customer ID, secret, callback token, Business ID, email,
  nomor telepon, nama, PAN, CVV, atau bank credential;
- fake script harus mencatat received provider-neutral request sehingga test
  dapat membuktikan retry menggunakan attempt, hash, dan key yang sama;
- fake script tidak boleh melakukan network, filesystem write, goroutine
  tersembunyi, atau database mutation.

Tambahkan test contract yang memastikan:

- `PaymentAdapter` tetap provider-neutral;
- `FakeAdapter` menerima `ProviderSessionID` untuk inquiry;
- normalized result tidak mempunyai field customer/PII/raw body/raw header;
- arbitrary provider error text dinormalisasi dan tidak bocor;
- source scan tidak menemukan package Xendit HTTP adapter baru;
- source scan tidak menemukan string `customer_id` atau customer object pada
  payment provider request DTO.

Acceptance:

- seluruh recovery scenario dapat disuntikkan tanpa provider network;
- tidak ada credential yang dibutuhkan;
- tidak ada klaim bahwa Xendit wire integration sudah selesai;
- blocker customer contract tercatat dan tetap fail closed.

### Slice 3 — Transaction-aware outbox finalization

File:

- `apps/api/internal/paymentoutbox/repository.go`;
- `apps/api/internal/paymentoutbox/repository_test.go` atau test existing;
- `apps/api/internal/payments/outbox_repository_integration_test.go`.

Perubahan:

1. Ekstrak transaction-aware primitive:
   - `MarkRetryableTx`;
   - `MarkSucceededTx`;
   - `MarkTerminalTx`.
2. Wrapper existing tanpa suffix `Tx` tetap membuka/commit transaction sendiri
   agar caller lama tidak rusak.
3. Primitive `Tx` tidak boleh begin, commit, atau rollback.
4. Semua finalizer tetap memerlukan exact command ID, lease owner, lease token,
   dan lease belum expired.
5. Primary key, aggregate, command type, key, hash, payload, attempt count, dan
   provider reference lama tetap immutable sesuai guard.
6. Jangan melemahkan migration trigger untuk mempermudah test.

Test:

- Tx caller rollback mengembalikan lifecycle command seperti semula;
- stale/expired/wrong lease token ditolak;
- duplicate finalization menjadi lease conflict/no-op aman;
- wrapper lama tetap bekerja;
- primary key tidak dapat berubah;
- direct SQL dan replica-role guard lama tetap lulus.

Acceptance:

- service hasil provider dapat menyelesaikan command pada transaction yang sama
  dengan domain result;
- tidak ada perubahan schema.

### Slice 4 — Repository untuk normalized create result

File:

- `apps/api/internal/payments/repository.go`;
- file test repository unit/integration yang relevan.

Buat tipe input terpisah, contoh:

```go
type ApplyCreateProviderResultParams struct {
    CommandID             string
    LeaseOwner            string
    LeaseToken            string
    AttemptID             string
    ProviderSessionID     string
    ProviderPaymentReqID  *string
    ProviderPaymentID     *string
    ProviderStatusCode    string
    CheckoutURL           *string
    ProviderExpiresAt     time.Time
    Status                PaymentStatus
    AmountRupiah          int64
    Currency              Currency
    ProviderReference     string // sudah berupa digest untuk outbox
    ObservedAt            time.Time
}
```

Nama final dapat mengikuti style project, tetapi jangan memakai raw response.

Transaction flow:

1. Begin transaction di service/repository orchestration.
2. Resolve booking ID dari attempt.
3. Ambil `paymentflow.LockBooking`.
4. Lock attempt.
5. Lock leased command dan validasi:
   - type `PAYMENT_CREATE`;
   - aggregate/attempt sama;
   - key/hash/payload cocok;
   - lease owner/token cocok dan belum expired.
6. Revalidate provider/environment `XENDIT`/`TEST`, amount, currency, method,
   integration/capture mode, dan immutable create contract.
7. Provider fields hanya boleh:
   - diisi dari `NULL` ke exact validated value; atau
   - replay dengan value identik.
8. Provider identity yang sudah berbeda menghasilkan mismatch.
9. Checkout URL hanya disimpan jika safe Test URL.
10. Provider expiry tidak boleh memperpanjang immutable requested expiry atau
    booking expiry; gunakan nilai paling awal.
11. `CREATED -> PENDING` untuk active/uncertain result.
12. Create Session `COMPLETED` tetap `PENDING`.
13. Create result tidak boleh langsung menulis capture fact.
14. Mark create command `SUCCEEDED`.
15. Jika attempt `PENDING` dan provider identity sudah tersedia, enqueue satu
    `PAYMENT_INQUIRY`.
16. Tulis audit sanitized.
17. Commit.

Scripted create timeout/error flow:

- timeout/transient/rate limit:
  - `CREATED -> PENDING` jika masih `CREATED`;
  - state `PENDING` tetap `PENDING` pada retry;
  - mark create command retryable dengan code yang sama;
  - jangan menghapus provider fields yang mungkin sudah ada;
  - jangan enqueue inquiry tanpa identity;
  - commit state/lifecycle/audit secara atomik.
- terminal config/contract error:
  - attempt tetap unresolved jika provider outcome mungkin terjadi;
  - command terminal;
  - tulis operational/security audit;
  - jangan mengarang `FAILED`.

Test:

- successful create menyimpan safe URL dan enqueue tepat satu inquiry;
- same result replay tidak mengganti field;
- different provider identity ditolak;
- production checkout URL ditolak;
- provider expiry tidak memperpanjang local expiry;
- timeout menghasilkan satu attempt dan satu create command;
- retry timeout memakai key/hash yang sama;
- injected audit/DB failure rollback attempt, command, inquiry, dan audit.

### Slice 5 — Provider-neutral inquiry result processor

File baru yang disarankan:

- `apps/api/internal/paymentworker/processor.go`;
- `apps/api/internal/paymentworker/processor_test.go`;
- integration test di package `payments`/`paymentworker`.

Dependency processor:

- database pool;
- payment repository;
- outbox repository;
- platform audit service;
- injected `payments.PaymentAdapter` (`FakeAdapter` pada seluruh test plan
  revisi);
- clock yang dapat diinjeksi untuk test;
- retry policy yang dapat diinjeksi.

Sebelum adapter call:

1. Decode allowlisted command payload secara strict.
2. Pastikan command adalah `PAYMENT_INQUIRY`.
3. Load attempt dan provider identity.
4. Jika attempt sudah terminal, jangan adapter call; selesaikan stale command
   sebagai no-op dengan transaction/lease guard.
5. Jika attempt `PENDING` tetapi provider identity belum ada, reschedule dan
   biarkan create recovery berjalan.
6. Bentuk `GetPaymentStatusRequest` dari server-owned facts.

Setelah adapter call, validasi berurutan:

1. attempt ID/aggregate/key/hash/payload/lease;
2. provider dan environment;
3. payment request ID;
4. payment ID;
5. amount;
6. currency;
7. captured timestamp dan evidence hash untuk capture.

Jangan mengubah urutan lock yang sudah dipakai create/cancel/capture:
booking-flow advisory lock, attempt row, command row, lalu audit.

Result matrix:

| Inquiry result | Attempt sebelum | Efek lokal | Command |
|---|---|---|---|
| `PENDING` | `PENDING` | tidak ada state change | retryable, capped backoff |
| timeout/transient | `PENDING` | tidak ada state change | retryable, same key |
| `CAPTURED` valid | `PENDING` | capture fact + `CAPTURED` | succeeded |
| `CAPTURED` valid | `FAILED`/`EXPIRED`/`CANCELLED` | late capture fact + `CAPTURED`; booking tidak dibuka | succeeded |
| `CAPTURED` identical | `CAPTURED` | no-op | succeeded/no-op |
| `FAILED` valid | `PENDING` | `FAILED` | succeeded |
| `EXPIRED` valid | `PENDING` | `EXPIRED` | succeeded |
| `CANCELLED` valid | `PENDING` | `CANCELLED` | succeeded |
| pending/failure setelah `CAPTURED` | `CAPTURED` | no-op, tidak downgrade | succeeded/no-op |
| terminal non-capture setelah terminal | terminal | no-op, tidak reopen | succeeded/no-op |
| reference mismatch | apa pun | tidak berubah | terminal mismatch |
| amount mismatch | apa pun | tidak berubah | terminal mismatch |
| currency mismatch | apa pun | tidak berubah | terminal mismatch |
| malformed sekali | `PENDING` | tidak berubah | retryable |
| malformed kedua | `PENDING` | tidak berubah | terminal incident |

Capture pada disposable integration test harus menggunakan existing immutable
capture path dengan:

- `Authority=AUTHENTICATED_INQUIRY`;
- `SourceReference=payment:inquiry:{attempt_id}`;
- `PayloadHash` dari adapter evidence hash;
- exact provider effective `CapturedAt`;
- server `ObservedAt`;
- exact amount/currency/provider IDs.

Penggunaan `Authority=AUTHENTICATED_INQUIRY` di sini hanya membuktikan domain
contract pada disposable test database. Runtime tidak boleh menghasilkan
authority tersebut sampai real authenticated adapter disetujui dan di-wire.

Refactor `RecordCapture` menjadi wrapper atas `RecordCaptureTx` agar capture
fact, command success, dan audit berada dalam transaction yang sama. Lakukan
hal serupa untuk terminal transition jika existing `TransitionState` tidak
bisa mengikuti transaction caller.

Test wajib:

- timeout kemudian success;
- timeout kemudian pending;
- timeout kemudian failure;
- pending berulang memakai row/key yang sama;
- repeated successful inquiry no-op;
- failure/expiry/cancel setelah capture tidak downgrade;
- session completed tanpa Payment Request success tetap pending;
- late capture setelah booking cancellation dan local expiry;
- amount, currency, provider request ID, dan payment ID mismatch;
- missing captured time/hash ditolak;
- duplicate capture fact identik;
- concurrent inquiry workers menghasilkan satu capture fact;
- stale lease processor tidak dapat commit;
- injected failure rollback seluruh local effect;
- tidak ada booking `PAID`, owner income, atau journal row.

### Slice 6 — Worker core dan retry policy

File:

- `apps/api/internal/paymentworker/worker.go`;
- `apps/api/internal/paymentworker/policy.go`;
- test masing-masing.

Worker rules:

1. Worker ID berbentuk `worker:{uuid}`.
2. Poll satu command per iterasi menggunakan existing `ClaimNext`.
3. `ErrNoCommandAvailable` menunggu poll interval atau context cancellation.
4. Context cancellation menghentikan ticker, HTTP call, dan goroutine.
5. Lease duration harus lebih panjang dari provider request timeout plus
   margin.
6. Adapter call menggunakan child context dengan timeout eksplisit.
7. Jangan menjalankan dua adapter calls untuk command yang sama dari satu
   worker.
8. Expired lease boleh direclaim; result dari lease lama tidak boleh commit.
9. Jangan log secret, raw provider errors, raw body, checkout URL, atau payload.
10. Log hanya command type, safe command ID/attempt ID, normalized error code,
    attempt count, dan correlation ID yang diizinkan.

Retry policy:

- timeout/transient: exponential backoff;
- rate-limit: gunakan bounded `RetryAfter`;
- pending status: inquiry backoff;
- semua delay mempunyai minimum dan maksimum;
- tambahkan deterministic jitter source yang dapat diinjeksi pada test;
- database menghitung `available_at` dari transaction time;
- stale attempt menghasilkan metric/audit operational, bukan state `FAILED`;
- timeout/pending tidak membuat key atau command baru.

Contoh nilai awal untuk constructor default, bukan environment contract baru:

- adapter call timeout: 10 detik;
- lease duration: 30 detik;
- empty queue poll: 1 detik;
- retry awal: 2 detik;
- max retry delay: 60 detik.

Nilai dapat disesuaikan setelah test, tetapi harus diinjeksi melalui
`WorkerOptions` agar unit test tidak sleep nyata.

Test:

- empty queue tidak busy-loop;
- cancellation berhenti cepat;
- backoff capped;
- Retry-After negatif/terlalu besar dinormalisasi;
- lease expiry/restart aman;
- worker panic/error pada satu command tidak mematikan loop;
- flag off berarti zero claim dan zero adapter call.

### Slice 7 — Runtime non-wiring dan fail-closed activation guard

File:

- `apps/api/cmd/api/router.go`;
- `apps/api/cmd/api/router_test.go`/startup tests;
- `apps/api/internal/config/payment_config_test.go`;
- `apps/api/.env.example`;
- Docker Compose hanya untuk memverifikasi semua flag tetap false.

Aturan:

1. Jangan import atau membangun Xendit adapter di `router.go`.
2. Jangan wire `FakeAdapter` ke router, API runtime, Docker, atau background
   goroutine.
3. Worker core hanya dibuat langsung oleh unit/integration test melalui
   dependency injection.
4. Tambahkan explicit runtime activation guard. Jika
   `PAYMENT_CREATE_ENABLED=true` atau `PAYMENT_INQUIRY_ENABLED=true` pada
   runtime dengan `startWorkers=true`, dependency setup harus gagal dengan
   safe internal error karena provider adapter contract belum siap.
5. Error startup tidak boleh menyebut secret, customer data, raw provider
   error, atau isi environment.
6. `startWorkers=false` hanya boleh melewati guard pada unit/integration test
   yang membangun dependency secara terisolasi; test harus menginjeksi
   orchestrator/worker options secara langsung.
7. Semua payment flags tetap default false di `.env.example` dan Docker.
8. Jangan menambahkan readiness flag environment baru. Kesiapan adapter harus
   berasal dari amendment kontrak dan implementasi code yang direview, bukan
   boolean yang dapat diaktifkan operator.
9. Pertahankan webhook/refund/ledger/monetization flags false.
10. Existing payment attempts/commands tidak dihapus saat runtime disabled.

Acceptance:

- activation create/inquiry gagal sebelum HTTP server menerima request;
- flags false tetap mengizinkan startup normal;
- zero provider adapter construction dan zero payment worker goroutine;
- config redaction tidak pernah menampilkan secret;
- semua flags default `false`.

### Slice 8 — Frontend non-authority regression only

Jangan menambahkan customer method selector atau real checkout redirect pada
plan revisi. UI tersebut bergantung pada Session adapter yang masih diblokir.

File:

- `apps/web/src/types/payment.ts`;
- `apps/web/src/pages/PaymentReturnPage.tsx`;
- `apps/web/src/__tests__/paymentReturnPage.test.tsx`;
- `apps/web/src/lib/authReturn.ts`.

Implementasi:

1. Pertahankan existing return page sebagai local-status reader.
2. Path `success` dan `cancel` hanya memengaruhi copy tampilan.
3. Return page tidak boleh memanggil payment mutation.
4. Return page tidak boleh mengubah booking/payment state di memory seolah-olah
   authoritative.
5. Polling tetap bounded oleh attempt expiry atau lima menit.
6. Authentication return tetap canonical, internal, customer-only, dan
   one-time.
7. Jangan menambahkan `VITE_*` payment/customer/provider configuration.
8. Jangan mengubah legacy manual payment UI sebagai bagian plan revisi.

Frontend tests:

- polling berhenti saat terminal, expiry, atau bounded window;
- success/cancel return path tidak mengirim mutation;
- status `CAPTURED` hanya berasal dari GET response;
- auth return tetap canonical dan customer-only;
- unsupported outcome ditolak sebelum resolver dipanggil;
- tidak ada customer method selector atau external redirect baru.

### Slice 9 — Audit, privacy, dan operational evidence

File:

- `apps/api/internal/audit/platform_dto.go`;
- `apps/api/internal/audit/payment_contract_test.go`;
- payment worker tests.

Audit changes:

1. Izinkan `PAYMENT_COMMAND_ENQUEUED` untuk `PAYMENT_CREATE` dan
   `PAYMENT_INQUIRY`.
2. Gunakan existing canonical `payment_state_transition` untuk state change.
3. Gunakan `reconciliation_exception` untuk late capture/mismatch dengan
   normalized reason allowlist.
4. Provider/worker actor:
   - `actor_user_id=NULL`;
   - `actor_role=SYSTEM`.
5. Correlation ID menggunakan local safe reference atau deterministic command
   key, bukan provider raw payload.
6. Metadata hanya scalar allowlisted. Jangan menyimpan URL, raw provider ID
   jika tidak diperlukan, response, customer data, atau error text.

Operational checks:

- command backlog;
- leased command yang expired;
- retry count tinggi;
- stale `PENDING`;
- authentication failure;
- mismatch;
- late capture.

Jika project belum mempunyai metrics sink yang sesuai, sediakan structured
safe log/testable observer interface. Jangan membuat admin finance endpoint
baru pada task ini.

### Slice 10 — Integration, concurrency, dan rollback verification

Gunakan disposable PostgreSQL test harness yang sudah dipakai payment
integration tests.

Skenario minimum:

1. scripted create timeout lalu replay melalui `FakeAdapter` dengan key sama
   mendapatkan synthetic Session identity yang sama;
2. setelah identity tersimpan, tepat satu inquiry command dibuat;
3. inquiry timeout lalu success;
4. inquiry timeout lalu pending;
5. inquiry timeout lalu failed;
6. repeated inquiry tidak membuat command/fact/audit duplikat;
7. mismatch tidak mengubah attempt;
8. out-of-order failure setelah capture tidak downgrade;
9. late capture setelah booking cancellation/expiry mencatat capture dan
   reconciliation exception, tanpa membuka booking;
10. dua worker concurrent menghasilkan satu final effect;
11. lease lama gagal commit setelah lease direclaim;
12. adapter call sukses tetapi transaction lokal gagal: retry dengan key sama
    memulihkan normalized result tanpa local attempt/command kedua;
13. audit failure rollback domain dan command lifecycle;
14. sandbox attempt tetap menghalangi legacy payment/owner income;
15. jumlah actual journal, payout, transfer, owner-income rows tetap nol;
16. flags off menghasilkan zero adapter call;
17. secret/customer/PII fixture scan menghasilkan nol temuan;
18. runtime activation guard menolak create/inquiry sebelum adapter nyata
    dibuat.

Database assertions harus menyebut exact count, bukan hanya “tidak error”:

- satu payment attempt;
- satu create contract;
- satu create command;
- maksimal satu inquiry command;
- maksimal satu capture fact;
- satu exact terminal state;
- audit count sesuai event yang benar;
- nol legacy cash/journal/payout facts.

### Slice 11 — Full regression dan Docker fail-closed proof

Automated checks:

```powershell
cd apps/api
go test ./internal/paymentworker
go test ./internal/payments
go test ./internal/paymentoutbox
go test ./internal/audit
go test ./internal/config
go test ./cmd/api
go test ./...
go vet ./...

cd ../web
npm test
npm run lint
npm run build
```

Docker/manual proof plan revisi dilakukan setelah automated checks hijau:

1. Rebuild API/web.
2. Pastikan migration `28|false`.
3. Pastikan semua payment flags false.
4. Jangan memasukkan atau membaca Test secret untuk proof ini.
5. Pastikan monetization, webhook, refund, journal, payout, dan xenPlatform
   tetap false/off.
6. Buktikan API dan web sehat dengan zero provider call.
7. Jalankan startup test terisolasi yang mengaktifkan create/inquiry dan
   buktikan dependency setup gagal sebelum listener start.
8. Tunjukkan return URL success/cancel tidak mengubah status.
9. Jalankan credential/customer/PII scan dan residue check.

Jangan memasukkan screenshot yang memperlihatkan secret, webhook token,
Business ID, customer PII, atau raw Authorization header.

Hasil Slice 11 adalah technical-core evidence. Jangan menjalankan Dashboard
Test payment, jangan mengklaim Xendit sandbox end-to-end, dan jangan memberi
status `READY FOR 5B-08`.

## 8. Daftar file yang diperkirakan berubah

Daftar ini adalah panduan, bukan izin mengubah file yang tidak relevan:

```text
apps/api/internal/payments/adapter.go
apps/api/internal/payments/adapter_test.go
apps/api/internal/payments/repository.go
apps/api/internal/payments/repository_integration_test.go
apps/api/internal/payments/outbox_repository_integration_test.go
apps/api/internal/paymentoutbox/repository.go
apps/api/internal/paymentoutbox/*_test.go
apps/api/internal/paymentworker/worker.go
apps/api/internal/paymentworker/policy.go
apps/api/internal/paymentworker/processor.go
apps/api/internal/paymentworker/*_test.go
apps/api/internal/audit/platform_dto.go
apps/api/internal/audit/payment_contract_test.go
apps/api/internal/config/payment_config_test.go
apps/api/cmd/api/router.go
apps/api/cmd/api/*_test.go
apps/api/.env.example
apps/web/src/types/payment.ts
apps/web/src/pages/PaymentReturnPage.tsx
apps/web/src/__tests__/paymentReturnPage.test.tsx
apps/web/src/lib/authReturn.ts
docs/task_5b-07_sandbox_inquiry_timeout_recovery.md
```

File/package yang dilarang dibuat oleh plan revisi:

```text
apps/api/internal/providers/xendit/*
apps/api/internal/providers/*http*
apps/web payment method selector/checkout redirect baru
provider wire fixtures
```

Expected schema change: **tidak ada**. Jangan membuat migration baru kecuali
review membuktikan invariant tidak dapat dipenuhi dengan schema 025–028.
Migration `029` tetap milik 5C-01.

## 9. Acceptance criteria final

Plan revisi hanya boleh diberi status
`5B-07 PROVIDER-NEUTRAL CORE READY — XENDIT SESSION ADAPTER BLOCKED` apabila:

- [ ] Adapter call hanya dijalankan asynchronous worker core pada test.
- [ ] Seluruh worker test memakai injected `FakeAdapter`.
- [ ] Tidak ada package Xendit HTTP adapter.
- [ ] Tidak ada provider/customer wire DTO.
- [ ] Runtime/Docker tidak membangun adapter atau menjalankan payment worker.
- [ ] Runtime activation create/inquiry gagal sebelum listener start.
- [ ] Create timeout tetap `PENDING`.
- [ ] Create timeout retry memakai deterministic adapter key yang sama.
- [ ] Tidak ada local attempt/create command kedua karena timeout.
- [ ] Satu deterministic inquiry command menyelesaikan attempt yang sama.
- [ ] Inquiry pending/timeout memakai command/key yang sama.
- [ ] Simulated authenticated inquiry pada disposable test membuat satu
      immutable capture fact.
- [ ] Inquiry failure/expiry/cancel menggunakan guarded transition.
- [ ] Terminal state tidak dapat downgrade.
- [ ] Late capture tidak membuka booking.
- [ ] Amount/currency/reference/provider mismatch ditolak tanpa state mutation.
- [ ] Adapter call berada di luar DB transaction.
- [ ] Domain result, command lifecycle, capture fact, dan audit atomik.
- [ ] Stale lease tidak dapat commit.
- [ ] Repeated inquiry adalah no-op aman.
- [ ] Browser redirect/return tidak authoritative.
- [ ] Synthetic checkout URL pada tests hanya menggunakan Test Mode allowlist.
- [ ] Secret/raw payload/PII tidak ada di DB, log, fixture, frontend, atau Git.
- [ ] Tidak ada webhook/refund/actual journal/owner cash/payout/settlement.
- [ ] Semua flags tetap `false` selama Docker proof.
- [ ] Targeted tests, full Go tests, vet, frontend test/lint/build lulus.
- [ ] Migration tetap `28|false`.
- [ ] Git diff hanya berisi scope 5B-07 dan dokumentasinya.

Status `READY FOR 5B-08` baru boleh diberikan setelah pekerjaan bersyarat
berikut selesai:

- [ ] 5A-04 mengizinkan dan mengatur customer/customer-ID data flow.
- [ ] 5A-05 membekukan exact updated Session request/response contract.
- [ ] Legal/Security/Product decision atau demo-only exception dicatat dengan
      nama, role, dan tanggal sesuai gate yang berlaku.
- [ ] Xendit Test adapter dibuat tanpa menebak field.
- [ ] Adapter membuktikan Test Mode, authentication, response limit,
      redirect protection, redaction, dan mismatch validation.
- [ ] Runtime worker di-wire secara fail closed.
- [ ] BCA/QRIS/card hosted checkout Test Mode terbukti.
- [ ] Timeout lalu real provider inquiry pending/success/failure terbukti.
- [ ] Seluruh core regression diulang setelah adapter nyata ditambahkan.

## 10. Stop conditions

Konflik `customer`/`customer_id` sudah diputuskan untuk plan revisi: skip real
adapter, pertahankan runtime disabled, lalu lanjutkan provider-neutral core.
Model AI tidak perlu berhenti atau menanyakan konflik yang sama lagi selama
tidak mencoba real provider integration.

Programmer atau model AI harus berhenti dan meminta review senior apabila core
implementation:

- mencoba membuat atau memanggil Xendit HTTP adapter;
- menambahkan customer/customer ID/PII untuk melewati blocker;
- mengaktifkan payment flags atau payment worker di runtime;
- membutuhkan secret, provider network, sub-account, atau xenPlatform;
- tidak dapat memulihkan simulated create timeout dengan key yang sama;
- Test/Live Mode tidak dapat dibedakan secara fail-closed;
- scripted capture tidak menyediakan payment ID, amount, currency, atau
  effective captured time;
- atomic result application membutuhkan melemahkan trigger/constraint;
- implementasi memerlukan migration `029`;
- worker core hanya dapat bekerja dengan menyimpan raw payload/secret;
- ada kebutuhan menandai booking `PAID`;
- test menemukan duplicate external create, duplicate capture, downgrade
  terminal, mismatch diterima, atau actual finance write;
- working tree berisi perubahan lain yang akan tertimpa.

## 11. Urutan commit yang disarankan

Jangan membuat satu commit besar. Setelah setiap commit, jalankan targeted
test yang relevan.

1. `test/payments: freeze 5B-07 inquiry recovery cases`
2. `test/payments: add contract-safe fake adapter scenarios`
3. `refactor/paymentoutbox: add transaction-aware lease finalizers`
4. `feat/payments: apply create and inquiry results atomically`
5. `feat/payments: add provider-neutral command worker core`
6. `fix/payments: fail closed while provider customer contract is blocked`
7. `test/web: preserve payment return non-authority`
8. `test/payments: add concurrency rollback and security regressions`
9. `docs/payments: record blocked adapter and technical-core evidence`

Setiap commit dilarang memasukkan `.env`, secret, build artifact, test database,
atau unrelated cleanup.

## 12. Instruksi singkat untuk junior/model AI

Gunakan urutan kerja berikut pada setiap slice:

1. baca file existing yang akan disentuh;
2. tulis failing test untuk satu behavior;
3. buat perubahan terkecil agar test lulus;
4. jalankan targeted test;
5. inspect diff;
6. pastikan adapter call tidak berada di transaction;
7. pastikan timeout tidak menjadi failure;
8. pastikan key/hash/attempt tidak berubah;
9. pastikan tidak ada secret/raw payload/PII;
10. baru lanjut ke slice berikutnya.

Jika ragu mengenai status provider, pilih hasil yang lebih konservatif:
pertahankan `PENDING`, jadwalkan inquiry dengan key yang sama, dan eskalasi
operasional. Jangan pernah mengarang capture atau failure.

Jangan membuat real provider adapter, jangan mengaktifkan runtime, dan jangan
memberi verdict `READY FOR 5B-08`. Output plan revisi berhenti pada:

```text
5B-07 PROVIDER-NEUTRAL CORE READY — XENDIT SESSION ADAPTER BLOCKED
```
