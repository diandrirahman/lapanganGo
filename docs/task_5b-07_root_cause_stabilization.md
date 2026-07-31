# Task 5B-07 — Root-cause Stabilization

Tanggal freeze: 2026-07-31

Scope: provider-neutral sandbox inquiry/timeout recovery

Status: `5B-07 PROVIDER-NEUTRAL CORE VERIFIED — XENDIT SESSION ADAPTER BLOCKED`

Dokumen ini adalah ledger tunggal untuk review Task 5B-07. Tujuannya bukan
menambal finding satu per satu, melainkan membuktikan bahwa seluruh keputusan
state machine berasal dari invariant yang sama.

## 1. Acceptance criteria yang dibekukan

| ID | Acceptance criterion |
|---|---|
| AC-01 | Timeout create mempertahankan satu attempt, mengubah `CREATED` ke `PENDING`, dan mengulang create dengan idempotency key yang sama hanya selama provider identity belum diketahui. |
| AC-02 | Provider create hanya boleh dipanggil oleh command `LEASED` dengan lease aktif, attempt non-terminal, identity belum ada, dan fakta command sama dengan fakta attempt. |
| AC-03 | Provider identity yang valid harus dipersist secara exact dan idempotent; `NULL -> exact` diperbolehkan, exact replay no-op, nilai berbeda menghasilkan konflik eksplisit. |
| AC-04 | Provider identity yang sudah ada menghentikan create. Recovery lokal harus memastikan attempt `PENDING`, satu inquiry command/key, audit, dan finalisasi create secara atomik. |
| AC-05 | Inquiry hanya boleh dipanggil oleh command `LEASED` dengan lease aktif, attempt non-terminal, dan identity valid yang memenuhi hierarchy Session atau Payment Request. |
| AC-06 | Inquiry `PENDING`, timeout, dan error transient menjadwalkan ulang command yang sama; tidak membuat attempt atau inquiry command baru. |
| AC-07 | Capture hanya berasal dari authenticated Payment-scope inquiry dengan Payment Request exact, Payment ID valid/exact, amount/currency exact, timestamp, dan evidence hash valid. |
| AC-08 | `FAILED`, `EXPIRED`, dan `CANCELLED` hanya diterapkan melalui transition guard; state terminal tidak boleh downgrade atau reopen. |
| AC-09 | Attempt terminal tidak boleh memanggil create/inquiry. Expired lease boleh direclaim secara atomic, kemudian hanya local no-op finalizer yang boleh dijalankan. |
| AC-10 | Invalid status, invalid identity, invalid capture evidence, dan malformed adapter result mengikuti satu aturan two-strike: strike pertama `RETRYABLE`, strike kedua `TERMINAL`, tanpa mengubah attempt. |
| AC-11 | Domain result, identity persistence, inquiry scheduling, command finalization, dan audit yang berhubungan harus commit/rollback atomik. Stale lease/token tidak boleh commit. |
| AC-12 | Duplicate/out-of-order response dan worker retry harus idempotent: satu capture fact, satu inquiry row/key, dan tidak ada finance residue. |
| AC-13 | Redirect/browser return tetap non-authoritative; tidak boleh memutasi payment state. |
| AC-14 | Tidak ada secret, credential-like identity, raw provider payload/header/error, customer PII, payout, settlement, journal produksi, real adapter, atau runtime activation dalam scope 5B-07. Semua payment/monetization flags tetap `false`. |

## 2. Root-cause map

| Root cause | Gejala yang pernah direview | Keputusan stabilisasi | Acceptance |
|---|---|---|---|
| RC-01 — Provider-call authorization tersebar | Terminal race meninggalkan command `LEASED`/`RETRYABLE`; create tetap dipanggil ketika identity sudah ada; inquiry dapat dipanggil dengan identity tidak lengkap; stale lease masih mencapai adapter | Satu preflight decision menentukan `CALL_CREATE`, `CALL_INQUIRY`, `LOCAL_CREATE_RECOVERY`, `LOCAL_RETRY`, `LOCAL_NOOP`, `LOCAL_TERMINAL`, atau `REJECT_LEASE` sebelum adapter dipanggil | AC-02, AC-04, AC-05, AC-09 |
| RC-02 — Identity policy tidak tunggal | Session-to-Request handoff hilang; terminal identity tidak terikat; identity mismatch tertutup terminal short-circuit; control character dan credential-like identity diterima; Payment ID tanpa Request dianggap inquiry-ready | Satu classifier identity storage/shape/hierarchy dipakai worker, repository, dan outbox digest boundary; exact comparison selalu mendahului terminal no-op | AC-03, AC-04, AC-05, AC-07, AC-14 |
| RC-03 — Result classification tersebar | `ErrInvalidCapture`, invalid status, dan invalid identity tidak konsisten masuk malformed two-strike; PENDING mengabaikan money mismatch | Create dan inquiry response masing-masing memiliki typed decision tunggal; repository error dipetakan melalui classifier yang sama | AC-06, AC-07, AC-10 |
| RC-04 — Lease/retry contract drift | Owner UUID berbeda dengan outbox; lease non-microsecond/>24h diterima; timeout addition overflow; jitter pecahan nanosecond; retry cap >24h; reclaim meragukan atomicity | Worker memakai validator outbox yang sama, arithmetic subtraction-safe, delay dinormalisasi microsecond, dan claim/reclaim tetap CAS di repository | AC-02, AC-09, AC-11 |
| RC-05 — Finalizer membaca snapshot lama | Attempt menjadi terminal selama adapter call tetapi retry/malformed masih meninggalkan residue; transient repository read dianggap terminal; missing-attempt path rollback | Seluruh local finalizer lock state terbaru dalam transaksi dan memilih no-op terminal atau lifecycle result secara atomic | AC-06, AC-09, AC-11 |
| RC-06 — Audit contract dan transaction boundary tidak fail-closed | Error read/audit diabaikan; invariant reason menerima enum action lain; local transaction gagal setelah provider success; panic dapat menghentikan worker | Action-specific audit reason validator tunggal; audit wajib satu transaksi; panic boundary di worker; crash tests membuktikan rollback dan retry exact | AC-11, AC-12, AC-14 |
| RC-07 — Evidence dapat hijau saat integration skip | Mandatory matrix belum lengkap; disposable cleanup terlambat; coverage command tidak mengaktifkan disposable suite; klaim evidence tidak membuktikan no-skip | Setup cleanup didaftarkan segera setelah DB dibuat; evidence command wajib mengaktifkan disposable suite dan test sentinel wajib muncul | AC-11, AC-12 |

## 3. Ledger seluruh temuan review

Klasifikasi memakai kategori yang diminta. `same root cause` selalu menunjuk
root cause di bagian 2; kategori primer tetap dicatat untuk membedakan regresi
dari gap kontrak yang sudah ada.

| ID | Temuan | Klasifikasi | Root cause | Acceptance | Disposisi |
|---|---|---|---|---|---|
| F-01 | Session response tidak mengikat Payment Request identity | pre-existing contract gap; same root cause | RC-02 | AC-03, AC-04 | In scope |
| F-02 | Mandatory processor/concurrency/rollback matrix belum tercakup | pre-existing contract gap; same root cause | RC-07 | AC-11, AC-12 | In scope |
| F-03 | Error repository/audit pada reconciliation diabaikan | regression; same root cause | RC-06 | AC-11 | In scope |
| F-04 | Panic processor dapat menghentikan worker | pre-existing contract gap; same root cause | RC-06 | AC-11 | In scope |
| F-05 | Worker owner/timing/retry policy berbeda dari outbox | pre-existing contract gap; same root cause | RC-04 | AC-02, AC-09 | In scope |
| F-06 | Strict decoder menerima/menolak trailing EOF secara salah | regression | — | AC-11 | Sudah ditutup; regression tetap dipertahankan |
| F-07 | Retry jitter bukan kelipatan microsecond | regression; same root cause | RC-04 | AC-06, AC-11 | In scope |
| F-08 | Terminal race meninggalkan command `LEASED` | regression; same root cause | RC-01, RC-05 | AC-09, AC-11 | In scope |
| F-09 | Terminal provider identity tidak pernah diikat | pre-existing contract gap; same root cause | RC-02 | AC-03, AC-08 | In scope |
| F-10 | Worker menerima UUID owner yang ditolak outbox | regression; same root cause | RC-04 | AC-02 | In scope |
| F-11 | Disposable DB cleanup didaftarkan setelah migration/pool | regression; same root cause | RC-07 | AC-11, AC-12 | In scope |
| F-12 | Retry finalizer memakai state sebelum adapter call | regression; same root cause | RC-05 | AC-09, AC-11 | In scope |
| F-13 | Terminal short-circuit menutupi identity mismatch create | regression; same root cause | RC-02 | AC-03, AC-09 | In scope |
| F-14 | Lease duration non-microsecond/>24h lolos constructor | regression; same root cause | RC-04 | AC-02 | In scope |
| F-15 | Retry `MaxDelay` dapat melampaui 24 jam | regression; same root cause | RC-04 | AC-06, AC-11 | In scope |
| F-16 | PENDING response mengabaikan amount/currency yang hadir | regression; same root cause | RC-03 | AC-07, AC-10 | In scope |
| F-17 | Concurrency dan adapter-success/local-failure belum terbukti | pre-existing contract gap; same root cause | RC-07 | AC-11, AC-12 | In scope |
| F-18 | Timeout+margin `time.Duration` dapat overflow | regression; same root cause | RC-04 | AC-02 | In scope |
| F-19 | Terminal short-circuit menutupi identity mismatch inquiry terbaru | regression; same root cause | RC-02 | AC-03, AC-09 | In scope |
| F-20 | Coverage command membuat disposable suite diam-diam skip | regression; same root cause | RC-07 | AC-11, AC-12 | In scope |
| F-21 | Terminal create response dengan Payment Request baru tidak melanjutkan inquiry | pre-existing contract gap; same root cause | RC-02, RC-03 | AC-03, AC-04 | In scope |
| F-22 | Expired leased command terminal tidak dapat direclaim/finalize lokal | pre-existing contract gap; same root cause | RC-01, RC-04 | AC-09 | In scope |
| F-23 | Invalid capture/status/identity tidak seragam mengikuti malformed two-strike | regression; same root cause | RC-03 | AC-10 | In scope |
| F-24 | Transient attempt read error diterminalkan | regression; same root cause | RC-05 | AC-11 | In scope |
| F-25 | Provider identity dengan control character diterima | regression; same root cause | RC-02 | AC-03, AC-14 | In scope |
| F-26 | Terminal no-op dengan outbox-incompatible identity meninggalkan lease | regression; same root cause | RC-02, RC-05 | AC-09, AC-11 | In scope |
| F-27 | `ErrAttemptNotFound` finalizer membaca ulang row yang hilang lalu rollback | regression; same root cause | RC-05, RC-06 | AC-11 | Defensive in scope |
| F-28 | Typed-nil repository/adapter/audit lolos constructor | pre-existing contract gap | RC-06 | AC-11 | In scope |
| F-29 | Credential-like provider identity dapat dipersist sebelum outbox menolak digest | regression; same root cause | RC-02 | AC-03, AC-14 | In scope |
| F-30 | Audit invariant reason menerima reason milik action lain | regression; same root cause | RC-06 | AC-11, AC-14 | In scope |
| F-31 | Create dipanggil lagi pada attempt aktif yang identity-nya sudah ada | pre-existing contract gap; same root cause | RC-01, RC-02 | AC-02, AC-04 | In scope |
| F-32 | Inquiry dipanggil dengan Payment ID tanpa Payment Request atau identity invalid | pre-existing contract gap; same root cause | RC-01, RC-02 | AC-05 | In scope |
| F-33 | Worker yang memegang snapshot lease kedaluwarsa masih dapat mencapai adapter | pre-existing contract gap; same root cause | RC-01, RC-04 | AC-02, AC-05, AC-09 | In scope |
| F-34 | Orphan command nyata mungkin ada walau FK payment attempt aktif | false positive untuk schema utuh | RC-05 | AC-11 | FK mencegah row; defensive branch/test tetap dipertahankan |
| F-35 | Diperlukan migration/schema baru untuk stabilization | false positive | — | AC-01–AC-14 | Tidak ada invariant yang memerlukan schema baru |
| F-36 | Real Xendit adapter harus ikut distabilkan sekarang | scope creep | — | AC-14 | Backlog B-01 |
| F-37 | Webhook/refund/ledger/payout/settlement harus ikut diuji | scope creep | — | AC-14 | Backlog B-02 |
| F-38 | xenPlatform dan runtime monetization perlu diaktifkan | scope creep | — | AC-14 | Ditolak; backlog phase Live |

## 4. State-transition matrix

`Identity`:

- `NONE`: Session, Payment Request, dan Payment ID semuanya `NULL`.
- `SESSION`: Session valid; Request dan Payment ID `NULL`.
- `REQUEST`: Payment Request valid; Session opsional valid; Payment ID `NULL`.
- `PAYMENT`: Payment Request dan Payment ID valid; Session opsional valid.
- `INVALID`: credential-like/unsafe identity, empty pointer, atau Payment ID tanpa
  Payment Request.

| Command | Command state | Lease | Attempt | Identity | Strike | Provider call | Keputusan lokal / next state |
|---|---|---|---|---|---:|---|---|
| any | bukan `LEASED` | any | any | any | any | tidak | `ErrLeaseConflict`; tidak ada mutation |
| any | `LEASED` | expired | any | any | any | tidak | `ErrLeaseConflict`; hanya claimant baru dengan CAS boleh lanjut |
| create | `LEASED` | active | `CREATED`/`PENDING` | `NONE` | any | create | classify result; timeout menjaga/menjadi `PENDING`; exact retry command sama |
| create | `LEASED` | active | `CREATED`/`PENDING` | `SESSION`/`REQUEST`/`PAYMENT` | any | tidak | atomic local create recovery: attempt `PENDING`, ensure satu inquiry, audit bila baru, create `SUCCEEDED` |
| create | `LEASED` | active | `CREATED`/`PENDING` | `INVALID` | any | tidak | create `TERMINAL`, reconciliation `PROVIDER_CONTRACT_BLOCKED`, attempt tidak berubah |
| create | `LEASED` | active | terminal | any | any | tidak | local no-op `SUCCEEDED`; mismatch existing identity tetap konflik/audit |
| inquiry | `LEASED` | active | `PENDING` | `NONE` | any | tidak | command sama `RETRYABLE`; menunggu identity |
| inquiry | `LEASED` | active | `PENDING` | `SESSION`/`REQUEST`/`PAYMENT` | any | inquiry | Session scope atau Payment scope dipilih dari hierarchy identity |
| inquiry | `LEASED` | active | `PENDING` | `INVALID` | any | tidak | inquiry `TERMINAL`, reconciliation `PROVIDER_CONTRACT_BLOCKED`; attempt tidak berubah |
| inquiry | `LEASED` | active | terminal | any | any | tidak | local no-op `SUCCEEDED`; state attempt immutable |
| any result | `LEASED` | active | non-terminal | any | 0 | sudah selesai | malformed pertama: command sama `RETRYABLE`, strike `1` |
| any result | `LEASED` | active | non-terminal | any | 1+ | sudah selesai | malformed kedua: command `TERMINAL`, strike `2`, reconciliation; attempt tidak berubah |
| any | expired `LEASED` | reclaimed CAS | terminal | any | any | tidak | token/owner baru; local no-op; token lama selalu `ErrLeaseConflict` |

Tidak ada transition Task 5B-07 yang boleh menghasilkan attempt baru, identity
overwrite, state reopen, provider create kedua setelah identity diketahui, inquiry
tanpa identity, finance fact, payout, settlement, atau jurnal produksi.

## 5. Invariant dan regression/crash proof

| Invariant | Regression/crash point yang wajib dibuktikan |
|---|---|
| INV-01 — Provider call memerlukan lease aktif dan owned | Snapshot lease kedaluwarsa tidak memanggil create/inquiry; reclaimed token baru menang, token lama gagal |
| INV-02 — Terminal attempt tidak memanggil provider | Semua terminal state × create/inquiry; expired terminal reclaim menjalankan local finalizer |
| INV-03 — Create hanya ketika identity `NONE` | `CREATED` dan `PENDING` dengan Session/Request/Payment identity menjalankan atomic local recovery |
| INV-04 — Inquiry hanya dengan hierarchy identity valid | `NONE` retry lokal; Payment ID tanpa Request dan unsafe identity terminal/audit tanpa provider |
| INV-05 — Identity write exact/idempotent | `NULL -> exact`, exact replay, different identity conflict, terminal compatible no-op, terminal mismatch conflict |
| INV-06 — Malformed selalu two-strike | Invalid create/inquiry status, identity, capture timestamp/hash/amount/currency, dan mapped repository invalid result |
| INV-07 — Result + command + audit atomic | Fail setelah identity/domain write, fail saat enqueue/audit/command finish, fail commit; rollback tanpa partial residue lalu exact retry |
| INV-08 — Satu inquiry dan satu capture | Duplicate/out-of-order, dua processor concurrent, local retry recovery; exact row/effect count |
| INV-09 — Finalizer lock state terbaru | Attempt menjadi terminal selama timeout/PENDING/malformed; command local no-op, bukan stranded retry/lease |
| INV-10 — Audit action fail-closed | Reconciliation/invariant hanya menerima enum reason sendiri; sanitizer memakai aturan yang sama; secret-like value tidak lolos |
| INV-11 — Evidence tidak boleh skip | Disposable sentinel test benar-benar `RUN`; command gagal bila prerequisite hilang/skip |
| INV-12 — Zero finance residue | Semua success/failure/retry/concurrency/crash scenario menghasilkan nol journal/payout/settlement/provider-cost fact |

Crash points yang menjadi bagian validation:

1. sebelum adapter call setelah claim;
2. sesudah adapter success sebelum transaksi lokal;
3. sesudah identity/domain mutation sebelum enqueue/finalize;
4. sesudah inquiry enqueue sebelum audit;
5. sesudah audit sebelum command finish;
6. sesudah command finish sebelum commit;
7. lease habis sebelum adapter dan selama adapter;
8. processor panic;
9. transient repository read failure;
10. audit failure dan forced transaction rollback.

## 6. Cabang yang sebelumnya belum memiliki keputusan eksplisit

Cabang berikut wajib ditutup oleh typed preflight decision, bukan `if` tersebar:

1. create command + attempt aktif + identity sudah ada;
2. create/inquiry command + snapshot lease sudah kedaluwarsa;
3. inquiry + Payment ID tanpa Payment Request;
4. inquiry + identity credential-like/unsafe yang berasal dari data legacy;
5. `CREATED` + identity sudah ada akibat recovery/legacy;
6. terminal attempt + identity kosong atau tidak kompatibel dengan outbox digest;
7. terminal attempt + identity yang berubah selama adapter call;
8. malformed result ketika attempt menjadi terminal sebelum finalizer lock;
9. transient attempt read vs authoritative not-found;
10. action-specific audit reason yang kebetulan sama dengan enum global.

## 7. Backlog di luar kontrak 5B-07

| ID | Item | Alasan dipindahkan |
|---|---|---|
| B-01 | Real Xendit HTTP/session adapter, `customer`/`customer_id`, SDK/API-version contract | Provider-neutral core tidak boleh menebak kontrak customer/PII atau melakukan network call |
| B-02 | Webhook inbox/outbox, signature verification, duplicate delivery | Scope Phase 5C; berbeda authority dan threat model |
| B-03 | Refund, provider cost facts, settlement, payout, owner transfer | Scope Phase 5D/Live; dilarang oleh sandbox fund-flow contract |
| B-04 | Shadow reconciliation ledger/reporting | Scope Phase 5E |
| B-05 | Runtime worker activation dan production scheduling | Memerlukan technical contract/provider gate; flags harus tetap false |
| B-06 | xenPlatform/sub-account/split payment | Menunggu kontrak marketplace, Legal, Finance, Security, dan Live gate |

## 8. Validation evidence

Regression tests dibuat dan dijalankan merah sebelum implementation berubah:

| Red proof | Hasil sebelum perbaikan |
|---|---|
| `go test -count=1 ./internal/payments ./internal/audit` | FAIL: credential-like identity diterima; invariant reason lintas-action diterima/disanitasi |
| targeted disposable provider-call authorization | FAIL: known identity memanggil create; invalid hierarchy memanggil inquiry; expired lease memanggil provider |
| numeric account-like identity boundary | FAIL: `session-4111111111111111` diterima |

Validation final:

| Gate | Command | Hasil |
|---|---|---|
| Targeted unit/state matrix | `go test -count=1 ./internal/provideridentity ./internal/payments ./internal/paymentoutbox ./internal/audit ./internal/paymentworker` | PASS |
| Targeted disposable regressions | `go test -count=1 -run 'TestProcessor(CreateWithKnownIdentityUsesLocalRecoveryWithoutProviderCall|InquiryWithInvalidStoredIdentityNeverCallsProvider|ExpiredLeaseNeverCallsProvider)$' -v ./internal/paymentworker` | PASS |
| Payment repository + integration, run 1 | `REQUIRE_PAYMENT_REPOSITORY_DISPOSABLE=1 ... go test -count=1 -v ./internal/payments` | PASS, 42.020s |
| Payment repository + integration, run 2 | command yang sama, cache disabled | PASS, 41.581s |
| Full payment worker, final run 1 | `REQUIRE_PAYMENT_WORKER_DISPOSABLE=1 ... go test -count=1 -v ./internal/paymentworker` | PASS, 83.703s |
| Full payment worker, final run 2 | command yang sama, cache disabled | PASS, 83.535s |
| Broad backend | `go test -count=1 ./...` | PASS |
| Static analysis | `go vet ./...` | PASS |
| Diff hygiene | `git diff --check` | PASS |
| Migration state | `SELECT version, dirty FROM schema_migrations` | `28|false` |
| Disposable residue | query `pg_database` untuk `lapangango_payment_*` dan `lapangango_worker_*` | 0 row |

Bukti no-skip:

- repository run 1 dan 2 mencetak
  `PAYMENT_REPOSITORY_DISPOSABLE_SUITE_ENABLED`;
- worker run 1 dan 2 mencetak
  `PAYMENT_WORKER_DISPOSABLE_SUITE_ENABLED`;
- tidak ada baris `SKIP` pada keempat output verbose;
- kedua gate `REQUIRE_*_DISPOSABLE=1` gagal apabila disposable prerequisite
  tidak tersedia.

## 9. File yang berubah dalam stabilization slice

- `apps/api/internal/paymentworker/decision.go`
- `apps/api/internal/paymentworker/processor.go`
- `apps/api/internal/paymentworker/processor_test.go`
- `apps/api/internal/paymentworker/processor_integration_test.go`
- `apps/api/internal/provideridentity/policy.go`
- `apps/api/internal/payments/provider_identity.go`
- `apps/api/internal/payments/provider_identity_test.go`
- `apps/api/internal/payments/repository_integration_test.go`
- `apps/api/internal/paymentoutbox/model.go`
- `apps/api/internal/audit/platform_dto.go`
- `apps/api/internal/audit/payment_contract_test.go`
- `docs/task_5b-07_sandbox_inquiry_timeout_recovery.md`
- `docs/task_5b-07_root_cause_stabilization.md`

Tidak ada migration, schema, real provider adapter, runtime activation, frontend,
webhook, refund, payout, settlement, atau journal yang ditambahkan.

Boundary working tree pada final verification:

- working tree belum clean karena perubahan Task 5B-06 dan 5B-07 masih
  terakumulasi: `28 modified` dan `34 untracked files`;
- empat file `db/migrations/027_*` dan `db/migrations/028_*` memang terlihat
  pada working tree, tetapi merupakan input Task 5B-06 yang sudah ada sebelum
  stabilization ini; Task 5B-07 tidak mengubah atau menambah migration/schema;
- tidak ada migration `029`;
- perubahan yang dibuat oleh final verification sendiri hanya allowlist
  `.gitignore` dan pembaruan evidence pada dokumen ini;
- daftar file stabilization tetap dibatasi pada daftar di atas. Daftar penuh
  lintas Task 5B-06/5B-07 harus diperiksa dengan
  `git status --short --untracked-files=all` sebelum commit.

## 10. Temuan yang ditutup

- F-01—F-33: ditutup oleh decision boundary, shared identity policy,
  action-specific audit contract, transactional local finalizer, atomic claim/
  reclaim, malformed classifier/two-strike, dan regression matrix.
- F-34: orphan attempt adalah false positive pada schema utuh karena FK, tetapi
  defensive invariant finalizer tetap diuji.
- F-35: kebutuhan schema baru adalah false positive; seluruh invariant terpenuhi
  tanpa migration.

Temuan yang dipindahkan ke backlog:

- F-36/B-01 real Xendit adapter dan customer contract;
- F-37/B-02—B-04 webhook, refund/provider cost, finance, dan shadow
  reconciliation;
- F-38/B-05—B-06 runtime activation, Live Mode, dan xenPlatform.

Alasannya sama: seluruh item tersebut mempunyai authority, privacy, fund-flow,
atau operational gate berbeda dan AC-14 secara eksplisit melarangnya dalam
provider-neutral 5B-07.

Traceability implementation dan regression:

| ID | Implementation proof | Regression proof |
|---|---|---|
| F-01 | Session-to-Request handoff diputuskan oleh typed inquiry decision dan dipersist melalui repository identity guard | `TestProcessorSessionHandoffPersistsIdentityAndCapturesOnPaymentScope`; `TestInquiryDecisionFreezesSessionToPaymentHandoff` |
| F-02 | Processor matrix memakai disposable PostgreSQL dan transaction boundary nyata | `TestProcessorInquiryOutcomeMatrix`; `TestProcessorMismatchAndMalformedMatrix`; `TestTwoConcurrentProcessorsCommitExactlyOneCaptureEffect` |
| F-03 | Semua reconciliation/audit write dipropagasikan dalam transaksi finalizer | `TestProcessorAuditFailureRollsBackTerminalCommand`; `TestProcessorPreservesLeaseOnTransientRepositoryReadFailure` |
| F-04 | Worker membungkus processor dengan panic recovery per command | `TestWorkerRecoversProcessorPanicAndProcessesNextCommand` |
| F-05 | Worker dan processor memakai validator duration/owner/retry outbox yang sama | `TestNewWorkerGeneratesUniqueOwnerAndValidatesLeaseMargin`; `TestNewProcessorRejectsOutboxIncompatibleRetryPolicy` |
| F-06 | Decoder memakai strict single-document boundary | `TestDecodePayloadRejectsUnknownOrTrailingFields` |
| F-07 | Retry policy membulatkan hasil final ke microsecond dengan cap dan `Retry-After` guard | `TestRetryPolicyAlwaysProducesOutboxCompatibleMicroseconds`; `TestRetryPolicyBoundsRetryAfter` |
| F-08 | Terminal race diselesaikan oleh latest-state local finalizer | `TestProcessorTerminalRaceFinalizesLeasedCommand` |
| F-09 | Identity terminal inquiry melewati exact identity persistence sebelum guarded terminal transition | `TestProcessorInquiryOutcomeMatrix`; `TestRepositoryInquiryIdentityBindingIsExactAndAtomic` |
| F-10 | Lease owner worker divalidasi dengan kontrak outbox | `TestNewWorkerGeneratesUniqueOwnerAndValidatesLeaseMargin`; `TestValidateLeaseAndErrorInputs` |
| F-11 | Cleanup didaftarkan segera setelah `CREATE DATABASE` | `TestPaymentDisposableSetupRegistersCleanupBeforeFallibleInitialization` |
| F-12 | Retry/malformed finalizer mengunci ulang attempt terbaru | `TestProcessorTerminalRaceFinalizesRetryOutcomesAsNoop` |
| F-13 | Existing identity dibandingkan sebelum terminal no-op pada create | `TestApplyCreateProviderResultTerminalNoopRejectsIdentityMismatch` |
| F-14 | Lease duration harus microsecond-aligned dan berada dalam batas outbox | `TestNewWorkerGeneratesUniqueOwnerAndValidatesLeaseMargin`; `TestValidateLeaseAndErrorInputs` |
| F-15 | Seluruh retry range ditolak constructor bila melampaui batas outbox | `TestNewProcessorRejectsOutboxIncompatibleRetryPolicy`; `TestRetryPolicyCapsExponentialDelay` |
| F-16 | Nilai money opsional boleh kosong, tetapi nilai yang hadir harus exact | `TestValidateInquiryResponseAllowsPendingWithoutAuthoritativeAmount`; `TestProcessorMismatchAndMalformedMatrix` |
| F-17 | Claim/concurrency dan provider-success/local-failure memakai exact retry recovery | `TestTwoConcurrentProcessorsCommitExactlyOneCaptureEffect`; `TestProcessorExactRetryRecoversAfterLocalTransactionFailure` |
| F-18 | Timeout/margin dibatasi dan dibandingkan subtraction-safe | `TestNewWorkerGeneratesUniqueOwnerAndValidatesLeaseMargin` |
| F-19 | Existing identity dibandingkan sebelum terminal no-op pada inquiry | `TestApplyInquiryIdentityTerminalNoopRejectsBoundMismatch` |
| F-20 | Required disposable flags dan sentinel membuat skip menjadi failure | `TestPaymentRepositoryDisposableEvidenceGate`; `TestPaymentWorkerDisposableEvidenceGate` |
| F-21 | Session terminal-looking yang membawa Request baru mengikat identity lalu retry pada Payment scope | subtest `session expired/cancelled with newly discovered request id` dalam `TestProcessorInquiryOutcomeMatrix` |
| F-22 | Claim repository mereclaim expired terminal lease secara CAS; processor hanya local no-op | `TestProcessorReclaimsExpiredTerminalLeaseWithoutProviderCall`; `TestProcessorStaleLeaseCannotCommitAfterReclaim` |
| F-23 | Create/inquiry classifier dan mapped invalid repository result memakai malformed two-strike tunggal | `TestProcessorCreateMalformedResultUsesTwoStrikeGuard`; `TestProcessorMismatchAndMalformedMatrix` |
| F-24 | Transient read error dikembalikan tanpa finalisasi command | `TestProcessorPreservesLeaseOnTransientRepositoryReadFailure` |
| F-25 | Shared provider identity policy menolak control/whitespace/unsafe bytes sebelum persistence | `TestRepositoryProviderIdentityGuardsRejectControlCharacters`; `TestInquiryDecisionClassifiesInvalidProviderIdentityAsMalformed` |
| F-26 | Terminal no-op memiliki deterministic local digest fallback dan reconciliation audit atomik | `TestProcessorTerminalNoopFallsBackFromOutboxIncompatibleIdentity` |
| F-27 | Authoritative missing attempt memakai invariant finalizer yang tidak membaca ulang row | `TestProcessorFinalizesMissingAttemptInvariantWithoutAttemptRead` |
| F-28 | Constructor memakai nil/typed-nil dependency guard | `TestNewProcessorRejectsTypedNilDependencies` |
| F-29 | Shared identity policy menolak credential/account-like identity sebelum repository/outbox mutation | `TestValidProviderIdentity`; `TestCreateClassificationRejectsInvalidOptionalProviderIdentity` |
| F-30 | Audit reason divalidasi per action dan sanitizer memakai validator yang sama | `TestPaymentAuditContract`; `TestSanitizePaymentInvariantReasonIsActionSpecific` |
| F-31 | Create dengan identity diketahui diarahkan ke atomic local recovery tanpa adapter | `TestProcessorCreateWithKnownIdentityUsesLocalRecoveryWithoutProviderCall`; `TestProcessorKnownIdentityRecoveryRollsBackAtomicallyOnAuditFailure` |
| F-32 | Identity hierarchy invalid diarahkan ke local terminal/audit tanpa inquiry | `TestProcessorInquiryWithInvalidStoredIdentityNeverCallsProvider`; `TestTerminalAttemptDecisionNeverCallsProvider` |
| F-33 | Lease diperiksa saat decision dan tepat sebelum provider call | `TestProcessorExpiredLeaseNeverCallsProvider`; `TestProcessorStaleLeaseCannotCommitAfterReclaim` |

## 11. Verdict

Seluruh gate root-cause stabilization lulus, bukan hanya targeted tests:

`5B-07 PROVIDER-NEUTRAL CORE VERIFIED — XENDIT SESSION ADAPTER BLOCKED`

Status `READY FOR 5B-08` tetap tidak diberikan. Backlog B-01/customer data-flow
dan exact Xendit Session contract harus diselesaikan lebih dahulu.

## 12. Final limited verification

Final verification terbatas dijalankan pada 2026-07-31 tanpa memperluas scope
ke real Xendit adapter atau improvement umum.

| Gate | Run 1 | Run 2 | No-skip proof |
|---|---|---|---|
| Full `internal/payments` + disposable repository | PASS, 41.710s | PASS, 54.936s | Marker `PAYMENT_REPOSITORY_DISPOSABLE_SUITE_ENABLED` hadir dan tidak ada `--- SKIP:` |
| Full `internal/paymentworker` + disposable worker | PASS, 79.159s | PASS, 94.495s | Marker `PAYMENT_WORKER_DISPOSABLE_SUITE_ENABLED` hadir dan tidak ada `--- SKIP:` |

Kedua run memakai proses PowerShell baru, `-count=1`,
`TEST_ROLLBACK_HARDENING_DISPOSABLE=1`, DSN PostgreSQL disposable, serta
`REQUIRE_PAYMENT_REPOSITORY_DISPOSABLE=1` dan
`REQUIRE_PAYMENT_WORKER_DISPOSABLE=1`. Verifier menggagalkan gate bila proses
test gagal, marker wajib hilang, atau output memuat satu baris `--- SKIP:`.

Klasifikasi final:

- `CONTRACT REGRESSION`: tidak ditemukan.
- `TEST/EVIDENCE DEFECT`: laporan ini sebelumnya tidak di-allowlist oleh
  `.gitignore`; diperbaiki sehingga ledger, matrix, invariant, backlog, dan
  evidence sekarang tercatat oleh Git.
- `XENDIT ADAPTER BLOCKER`: F-36/B-01 tetap di backlog.
- `OUT OF SCOPE IMPROVEMENT`: F-37/F-38 tetap di backlog B-02—B-06.
