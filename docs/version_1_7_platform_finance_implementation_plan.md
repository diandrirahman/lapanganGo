# LapangGo Platform Finance & Admin Analytics Implementation Plan

Status dokumen: **approved business direction; Phase 0–2A completed, Phase 2B–7 planning only**

Target awal: **v1.7 Platform Finance Foundation**

Mode monetisasi default: **`SIMULATION`**

Timezone bisnis: **`Asia/Jakarta`**

Mata uang MVP: **`IDR`**

Dokumen ini adalah source of truth untuk pengerjaan bertahap oleh Antigravity. Dokumen ini tidak memberi izin untuk langsung mengaktifkan komisi nyata, menahan dana customer, atau melakukan payout otomatis. Setiap phase harus dikerjakan, diuji, dan direview secara terpisah.

Execution breakdown setelah Phase 2A:

- `docs/LapangGo_Phase_2B_to_Phase_7_Antigravity_Task_Cards.md`

Cara memakai dokumen:

1. Phase 0–2A sudah selesai dan tidak perlu diulang.
2. Mulai dari **Task 2B-00** pada task-card document di atas.
3. Berikan Antigravity hanya satu task card per percakapan.
4. Minta evidence acceptance criteria dan verification, lalu lakukan review manusia.
5. Catat keputusan GO/NO-GO; jangan lanjut otomatis.
6. Phase 7 tidak boleh mengubah production tanpa persetujuan eksplisit manusia.

---

## 1. Outcome yang Ingin Dicapai

LapangGo akan diposisikan sebagai **marketplace sekaligus sistem operasional venue olahraga**.

Fondasi bisnis yang disepakati:

- Komisi standar LapangGo adalah **7%** dari booking online yang berhasil dibayar.
- Venue awal dapat memperoleh fase promosi **0%**, kemudian fase perkenalan **5%**, lalu tarif standar **7%**.
- Tarif harus configurable dan versioned; tidak boleh di-hard-code di frontend atau tersebar di banyak service.
- Booking owner offline/walk-in tidak dikenakan komisi.
- Komisi dihitung dari harga final setelah promo, bukan harga sebelum diskon.
- Payment processing fee ditanggung dari margin LapangGo pada model awal agar checkout customer sederhana.
- Payout owner direncanakan secara batch/mingguan setelah payment gateway dan settlement tersedia.
- LapangGo menanggung processing fee, non-refundable refund fee, dan satu scheduled weekly payout fee. Customer menerima approved full refund; owner tidak mendapat potongan provider fee tambahan di luar commission snapshot.
- On-demand/instant payout bukan scope model awal.
- Subscription Pro, promoted listing, event, dan monetisasi lain bukan scope fondasi v1.7.

Hasil teknis minimum v1.7:

1. Admin mempunyai halaman **Keuangan Platform**.
2. Admin dapat membedakan GMV, uang owner, proyeksi komisi, pendapatan aktual, dan pengeluaran platform.
3. Sistem menyimpan commercial term dan snapshot komisi per booking secara immutable.
4. Sistem memiliki ledger platform terpisah dari `owner_finance_transactions`.
5. Pengeluaran operasional platform dapat dicatat dan dikoreksi melalui reversal, bukan edit/delete.
6. Seluruh fondasi berjalan dalam mode `SIMULATION`; tidak ada pemotongan komisi nyata pada v1.7.

---

## 2. Current State dan Masalah yang Harus Dijaga

Kondisi repository saat plan dibuat:

- Dashboard superadmin hanya menampilkan total users, owners, venues, dan bookings.
- `owner_finance_transactions` adalah ledger milik owner, bukan ledger LapangGo.
- Booking online yang diverifikasi/ditandai lunas membuat ledger `INCOME / BOOKING` sebesar nilai penuh booking untuk owner.
- Refund membuat ledger owner `EXPENSE / REFUND` dan mempertahankan income booking awal.
- Pembayaran masih manual/direct-to-owner; belum ada payment gateway, webhook, settlement, atau payout.
- Booking offline dapat dikenali melalui `offline_booking_customers` dan juga membuat ledger `INCOME / BOOKING`.
- Status `CONFIRMED` tidak boleh otomatis dianggap sebagai pembayaran aktual karena terdapat flow legacy/dummy.
- Role admin yang tersedia adalah `SUPER_ADMIN`.
- Nomor migration tertinggi saat dokumen dibuat adalah `018`; Antigravity wajib memeriksa ulang sebelum memilih nomor migration berikutnya.

Konsekuensi penting:

- Menjumlahkan seluruh `owner_finance_transactions` lalu menamainya “Pendapatan LapangGo” adalah salah.
- Owner manual income, owner payroll, maintenance, dan biaya operasional venue tidak boleh masuk P&L platform.
- Booking offline tidak boleh masuk basis komisi.
- GMV harus berasal dari booking income yang benar-benar direalisasikan, bukan hanya status booking.
- Sampai uang ditagih/dipotong secara nyata, angka komisi harus disebut **proyeksi**, bukan pendapatan kas.

---

## 3. Accounting Boundary dan Istilah Resmi

Gunakan istilah berikut secara konsisten di database, API, UI, dokumentasi, dan laporan QA.

| Istilah | Definisi |
|---|---|
| Online GMV Bruto | Total ledger booking yang sudah direalisasikan untuk booking online pada periode pengakuan. |
| Captured GMV | Dana customer yang dikonfirmasi captured oleh provider; baru tersedia setelah gateway. |
| Completed GMV | Nilai booking captured yang layanannya sudah `COMPLETED`. |
| Refund Principal | Nilai booking online yang dikembalikan kepada customer. |
| Online GMV Neto | Online GMV Bruto dikurangi Refund Principal. |
| Proyeksi Komisi | Simulasi komisi berdasarkan scenario rate/snapshot simulation; bukan uang yang sudah diterima LapangGo. |
| Komisi Captured/Deferred | Bagian komisi dari pembayaran gateway yang sudah captured tetapi belum menjadi revenue karena booking belum selesai. |
| Pendapatan Komisi Aktual | Komisi mode `LIVE` yang baru diakui saat booking `COMPLETED`. Selalu tidak tersedia/nol di v1.7. |
| Payment Processing Expense | Biaya aktual dari provider pembayaran, bukan estimasi buatan. |
| Platform OPEX | Pengeluaran operasional LapangGo yang dicatat admin; tidak termasuk pengeluaran owner. |
| Promo Subsidy Attribution | Tag analitik atas payment/payout cost atau diskon platform cohort 0%; bukan expense kedua. Spend diturunkan dari ledger/cost items yang sama. |
| Platform Revenue | Earned commission + earned service fee - commission/service-fee reversal/contra. Tidak dikurangi processing cost. |
| Transaction Contribution | Platform Revenue - processing fee - provider tax - refund fee - payout fee - chargeback loss - actual platform-funded discount. |
| Operating Result | Transaction Contribution - Platform OPEX. Gunakan istilah ini, bukan “laba bersih”, karena belum memasukkan pajak penghasilan dan seluruh akun akuntansi. |
| Gross Take Rate | Platform Revenue dibagi Online GMV Neto. Null bila penyebut <=0. |
| Net Take Rate | Transaction Contribution dibagi Online GMV Neto. Null bila penyebut <=0. |
| Owner Payable | Kewajiban dana kepada owner setelah komisi/refund; hanya boleh muncul setelah platform mengendalikan aliran dana dan settlement tersedia. |
| Provider Settlement | Perpindahan dana dari PSP ke rekening LapangGo; berbeda dari payout owner. |
| Owner Payout | Pelunasan liability kepada owner; bukan pendapatan owner kedua dan bukan expense platform. |

Basis waktu setelah gateway:

- Captured GMV dan deferred commission: `captured_at`.
- Recognized commission revenue: `journal.effective_at` = waktu layanan selesai (`booking_date + end_time` Asia/Jakarta dikonversi UTC). `posted_at` hanya waktu worker memproses dan dipakai untuk observability.
- Refund: `payment_refunds.succeeded_at`.
- Platform OPEX: `occurred_at`/journal effective time.
- Provider settlement: provider settlement time.
- Owner payout: `owner_payouts.paid_at`.

### Formula v1.7 mode SIMULATION

```text
online_gmv_gross       = SUM(online realized booking income)
refund_principal       = SUM(online refund expense)
online_gmv_net         = online_gmv_gross - refund_principal
projected_commission   = SUM(scenario/snapshot commission for paid online bookings)
                         - SUM(the same original projected commission reversed by full refund)
projected_owner_net_after_hypothetical_commission = online_gmv_net - projected_commission
projected_take_rate_bps = projected_commission / online_gmv_net * 10,000
                          (null jika online_gmv_net <= 0)
actual_commission      = UNAVAILABLE before LIVE ledger recognition
payment_processing_fee = UNAVAILABLE before sourced from a real provider
platform_opex          = SUM(posted platform operating expense entries)
projected_operating_result_before_transaction_costs = projected_commission - platform_opex
platform_revenue       = UNAVAILABLE in SIMULATION
transaction_contribution = UNAVAILABLE in SIMULATION
operating_result       = UNAVAILABLE in SIMULATION
```

Proyeksi tidak boleh dimasukkan ke `platform_revenue`, `transaction_contribution`, atau `operating_result` aktual. `projected_operating_result_before_transaction_costs` wajib diberi caveat bahwa gateway, provider tax, payout, refund, chargeback, dan subsidi belum dikurangkan.

---

## 4. Product Rules yang Dikunci

### 4.1 Komisi

- Tarif disimpan dalam basis points: `0% = 0`, `5% = 500`, `7% = 700`.
- Batas validasi awal: `0 <= commission_bps <= 3000`.
- Default global adalah `700` dalam mode `SIMULATION`.
- Owner-specific term mengalahkan default global.
- Term dipilih saat booking dibuat dan disimpan sebagai snapshot.
- Perubahan tarif tidak boleh mengubah booking lama.
- Basis komisi adalah `bookings.final_price` jika tersedia; fallback ke `bookings.total_price` untuk data legacy.
- Perhitungan dibulatkan half-up ke rupiah penuh dan hasil snapshot digunakan untuk seluruh reversal selanjutnya.
- Jangan menghitung ulang komisi refund memakai tarif terbaru.
- Semua nominal domain finance baru menggunakan integer rupiah (`BIGINT` di PostgreSQL, `int64` di Go), bukan `float64`; API men-serialize nominal sebagai integer-rupiah string.
- Sebelum cutover, audit wajib memastikan harga existing tidak mempunyai pecahan rupiah. Jika ada, hentikan dan ambil keputusan eksplisit; jangan silently round.
- Booking historis sebelum cutover tidak boleh menjadi tagihan, piutang, owner payable, atau pendapatan aktual LapangGo.
- Historical scenario 7% boleh ditampilkan sebagai simulasi terpisah, tetapi billable snapshot-nya harus `LEGACY_NO_COMMISSION` dengan rate 0.

Jadwal launch partner yang direkomendasikan:

- Trial: `[first_gateway_capture_at, first_gateway_capture_at + 90 hari)` = 0%.
- Introductory: `[first_gateway_capture_at + 90 hari, first_gateway_capture_at + 180 hari)` = 5%.
- Standard: `[first_gateway_capture_at + 180 hari, infinity)` = 7%.
- Hanya owner yang ditandai `launch_partner`; owner non-promo menggunakan term kontraktualnya.
- Aktivasi tanggal awal harus concurrency-safe jika dua payment pertama datang bersamaan.
- Owner diberi pemberitahuan perubahan tarif minimal H-14 dan H-3.
- Promo 0% membuat LapangGo menanggung payment fee; cohort harus mempunyai budget/cap dan dashboard subsidy sebelum live.
- Tarif yang sudah dijanjikan tidak boleh dipercepat atau diubah retroaktif.

### 4.2 Booking yang Dikenakan Komisi

| Kondisi | Komisi |
|---|---:|
| Booking online baru setelah cutover pada term 7% | 7% |
| Booking online baru setelah cutover pada introductory term 5% | 5% |
| Booking online baru setelah cutover dalam trial 0% | 0% |
| Booking historical sebelum cutover | 0% billable; scenario 7% boleh ditampilkan terpisah |
| Booking offline/walk-in owner | 0%, selalu exempt |
| Owner manual finance income | Bukan GMV, 0% |
| Booking belum dibayar | Belum diakui |
| Booking dibatalkan sebelum dibayar | 0% |
| Booking full refund | Balikkan persis komisi snapshot |
| Mabar | Komisi hanya sekali pada source booking; pembayaran informal peserta bukan GMV tambahan |

Partial refund tidak tersedia saat plan dibuat. Jangan menambahkan partial refund diam-diam dalam phase ini.

Deferred commission LIVE hanya boleh terbentuk saat capture jika seluruh kondisi berikut benar:

```text
booking_fee_snapshot.booking_channel = MARKETPLACE_ONLINE
booking_fee_snapshot.finance_mode = LIVE
captured_payment_attempt.payment_rail = GATEWAY
captured_payment_attempt.collection_mode = PLATFORM_COLLECTED
captured_payment_attempt.monetization_decision = LIVE
payment_status = CAPTURED
booking dibuat setelah live cutover owner terkait
```

Payment order hanya boleh memperoleh `monetization_decision=LIVE` ketika kill switch, acceptance, KYC, destination, dan provider gates lulus saat order dibuat. Webhook tidak re-check current switch untuk mengubah hak in-flight. Jika satu snapshot condition tidak terpenuhi, hasilnya hanya scenario/simulation atau non-commissionable; fail closed. `actual_commission_revenue` memerlukan semua kondisi tersebut **ditambah** booking `COMPLETED` dan unique balanced journal `booking.completed:<id>`. Capture/completion tidak boleh resolve commercial term terbaru—selalu gunakan immutable booking fee snapshot yang direferensikan payment attempt.

### 4.3 Mode Operasi

Gunakan tiga mode:

- `OFF`: finance projection dan monetisasi dimatikan.
- `SIMULATION`: snapshot dan proyeksi aktif, tetapi tidak ada tagihan/potongan komisi aktual.
- `LIVE`: komisi aktual aktif hanya untuk owner yang commercial term-nya live dan hanya setelah seluruh go-live gate terpenuhi.

Guardrail ganda:

1. DB commercial term menentukan `SIMULATION` atau `LIVE` per owner.
2. Environment kill switch `PLATFORM_MONETIZATION_ENABLED` default `false`.

Service harus menolak mode `LIVE` jika kill switch bukan `true`, payment provider belum configured, atau collection method bukan `DEDUCT_FROM_PAYOUT`.

Kill-switch semantics setelah gateway:

- Diperiksa saat membuat payment order; keputusan eligibility/mode distamp immutable pada payment attempt.
- Saat dimatikan, hentikan payment order baru dan aktivasi owner baru.
- Verified webhook/inquiry, refund, settlement, dan payout obligation untuk transaksi in-flight/existing tetap harus diproses sampai aman.
- Kill switch tidak boleh mengubah rate/mode snapshot payment order yang sudah diterbitkan.
- In-flight capture yang tidak dapat dipenuhi harus masuk hold + idempotent refund runbook, bukan diam-diam menjadi tanpa komisi.

### 4.4 Immutability

- System-generated financial entry tidak boleh di-edit atau di-delete.
- Manual platform OPEX juga tidak boleh di-edit/delete setelah diposting.
- Koreksi dilakukan dengan entry reversal yang menyimpan referensi entry asal dan alasan.
- Commercial term yang sudah dipakai tidak diubah; buat versi baru dan tutup periode versi lama.
- Snapshot booking tidak diubah setelah dibuat.
- Semua amount/rate/status/side/account/currency/version/timestamp kritis pada schema finance baru wajib `NOT NULL`; `CHECK` saja tidak menolak NULL di PostgreSQL.

---

## 5. Release dan Phase Gate

Catatan schema: blok schema dalam dokumen ini adalah logical specification, bukan SQL siap salin. Migration final wajib memakai syntax PostgreSQL valid, named constraints, explicit `NOT NULL`, dan tests fresh/upgrade/down.

| Release | Phase | Outcome | Monetisasi |
|---|---|---|---|
| v1.7.0 | 0–4 | Admin analytics, terms, snapshots, platform OPEX, hardening | Simulation only |
| v1.8.0 | 5 | Payment gateway + verified webhook + reconciliation shadow mode | Tetap simulation |
| v1.9.0 | 6 | Payable/settlement/payout sandbox + manual approval | Simulation; tanpa dana customer production |
| Setelah Phase 6 GO | 7 | First limited LIVE capture; rate kalender 0% → 5% → 7%, perluasan cohort berdasarkan KPI | Live sangat terbatas |

Aturan phase gate:

- Antigravity mengerjakan hanya satu subphase per task.
- Tidak boleh lanjut hanya karena build lulus; acceptance criteria dan manual QA phase sebelumnya harus diperiksa.
- Setiap schema change wajib mempunyai `*.up.sql` dan `*.down.sql`.
- Semua perubahan finance/refund/payment wajib memiliki automated test atau manual verification path yang eksplisit.
- Mode `LIVE` tidak boleh diaktifkan selama Phase 0–5.
- Jika ditemukan perbedaan angka antara owner ledger, booking, dan admin analytics, hentikan phase berikutnya dan lakukan rekonsiliasi.

---

## 6. Phase 0 — Baseline, Decision Freeze, dan Data Audit

### Tujuan

Memastikan implementasi dimulai dari data dan kontrak yang dipahami, bukan asumsi.

### Langkah

1. Baca `AGENTS.md`, `.agents/agent_workflow.md`, `.agents/definition_of_done.md`, dan dokumen ini sampai selesai.
2. Jalankan `git status --short`; jangan menyentuh perubahan yang tidak terkait.
3. Catat migration terakhir dan jangan memakai nomor yang sudah ada.
4. Jalankan baseline:

   ```powershell
   Set-Location apps/api
   go test ./...

   Set-Location ../web
   npm.cmd run lint
   npm.cmd run build
   ```

5. Audit data read-only pada database QA/demo:
   - jumlah booking per status;
   - jumlah booking yang memiliki `INCOME / BOOKING` lebih dari satu;
   - booking offline versus online;
   - booking `PAID`/`COMPLETED` tanpa income ledger;
   - income ledger yang booking-nya tidak ada;
   - refund ledger tanpa income booking;
   - perbedaan `bookings.total_price` versus ledger booking amount;
   - jumlah booking dengan harga mempunyai pecahan rupiah (`total_price <> trunc(total_price)` dan field harga terkait);
   - nilai negatif/nol dan timestamp yang tidak masuk akal.
   - kedua jalur refund existing (`bookings` direct cancel-refund dan approval di package `refunds`) serta event/ledger yang masing-masing hasilkan.
6. Simpan query audit dalam test/helper atau dokumen QA hanya bila user meminta laporan. Jangan memperbaiki data produksi pada phase ini.
7. Konfirmasi keputusan tetap:
   - default 7%;
   - trial 0%;
   - introductory 5%;
   - online only;
   - final price after promo;
   - payment fee absorbed by platform;
   - simulation first.

### Acceptance Criteria

- Baseline test/build tercatat.
- Anomali data dan besar dampaknya diketahui.
- Tidak ada file aplikasi atau schema yang berubah.
- Tidak ada backfill atau mutasi data.

### Stop Condition

Jangan lanjut jika booking ledger memiliki duplikasi yang melanggar unique invariant, banyak booking paid tanpa ledger, atau baseline test utama gagal tanpa penjelasan.

---

## 7. Phase 1A — Read-only Admin Finance API (Simulation)

### Tujuan

Memberikan kontrak API stabil untuk GMV dan proyeksi komisi tanpa migration dan tanpa mengubah booking/payment flow.

### Modul yang Direkomendasikan

Buat modul terpisah agar `internal/admin` tidak menjadi terlalu besar:

```text
apps/api/internal/platformfinance/
  dto.go
  repository.go
  service.go
  handler.go
  service_test.go
  handler_test.go
  repository_test.go        # integration jika DATABASE_URL test tersedia
```

Wiring:

- `apps/api/cmd/api/main.go`
- Gunakan auth middleware yang sama dengan admin.
- Semua route wajib `requireActiveUser` dan `RequireRole("SUPER_ADMIN")`.
- Tambahkan filter optional `owner_profile_id` pada existing `GET /admin/venues` (`admin.VenueQuery`, service/repository, tests) agar pilihan venue tidak memuat seluruh dataset.
- Existing `GET /admin/owners?search=&page=&limit=` menjadi sumber owner options; frontend menggunakan server-side debounced search/pagination.

### Endpoint Stabil

```http
GET /admin/finance/summary
GET /admin/finance/breakdown?dimension=owner|venue&page=1&limit=20
```

Query umum:

```text
start_date=YYYY-MM-DD
end_date=YYYY-MM-DD
owner_profile_id=<uuid>       optional
venue_id=<uuid>       optional
granularity=auto|day|week|month
```

Rules:

- Jika kedua tanggal kosong, default periode adalah month-to-date: tanggal 1 sampai hari ini dalam `Asia/Jakarta`.
- Jika hanya satu tanggal diisi, return 400; jangan menebak boundary lainnya.
- Batas maksimum range: 366 hari.
- `start_date > end_date` menghasilkan 400.
- `venue_id` harus konsisten dengan `owner_profile_id` jika keduanya dikirim.
- `granularity=auto`: `day` untuk range <=31 hari, `week` untuk <=180 hari, dan `month` untuk range lebih panjang. Week dimulai Senin WIB.
- Trend wajib mengembalikan bucket kontinu; periode tanpa event tetap hadir dengan nilai available 0 dan unavailable null.
- Grouping hari/bulan memakai timezone Jakarta, bukan UTC implicit.
- Interpretasikan date sebagai interval half-open `[start 00:00 WIB, end+1 day 00:00 WIB)` lalu konversi ke UTC untuk kolom `TIMESTAMPTZ`; jangan memakai `23:59:59`.
- Empty result mengembalikan array kosong dan `"0"` untuk metrik uang yang sumber datanya tersedia. Metrik yang belum mempunyai source of truth mengembalikan `null` plus status `data_availability`, bukan Rp0 palsu.

### Response Summary

Semua nilai uang pada API finance baru dikirim sebagai **integer-rupiah string** (contoh `"250000"`). DB/domain Go tetap `BIGINT`/`int64`; rate/count tetap JSON number. Jangan memakai binary floating point untuk kalkulasi.

```json
{
  "mode": "SIMULATION",
  "currency": "IDR",
  "timezone": "Asia/Jakarta",
  "period": {
    "start_date": "2026-07-01",
    "end_date": "2026-07-11"
  },
  "generated_at": "2026-07-11T13:00:00Z",
  "as_of": "2026-07-11T13:00:00Z",
  "granularity": "day",
  "default_commission_bps": 700,
  "metric_source_version": "legacy-owner-ledger-v1",
  "projection_basis": "HISTORICAL_SCENARIO",
  "metrics": {
    "online_gmv_gross": "1000000",
    "legacy_manual_realized_gmv": "1000000",
    "gateway_captured_gmv": null,
    "refund_principal": "200000",
    "online_gmv_net": "800000",
    "projected_commission": "56000",
    "actual_commission_revenue": null,
    "payment_processing_expense": null,
    "platform_operating_expense": null,
    "projected_operating_result_before_transaction_costs": null,
    "platform_revenue": null,
    "transaction_contribution": null,
    "operating_result": null,
    "projected_take_rate_bps": 700,
    "gross_take_rate_bps": null,
    "net_take_rate_bps": null,
    "projected_owner_net_after_hypothetical_commission": "744000",
    "realized_online_booking_count": 8,
    "refunded_booking_count": 1
  },
  "data_availability": {
    "platform_operating_expense": "PENDING_PHASE_3B",
    "actual_platform_revenue": "UNAVAILABLE_UNTIL_LIVE",
    "payment_processing_expense": "UNAVAILABLE_UNTIL_GATEWAY",
    "owner_payable": "UNAVAILABLE_UNTIL_PLATFORM_COLLECTED"
  },
  "data_quality": {
    "paid_without_ledger_count": 0,
    "ledger_without_booking_count": 0,
    "legacy_scenario_count": 8,
    "snapshot_projection_count": 0,
    "non_billable_projection_amount": "56000"
  },
  "trend": [
    {
      "period_start": "2026-07-01",
      "period_end": "2026-07-01",
      "online_gmv_gross": "200000",
      "refund_principal": "0",
      "online_gmv_net": "200000",
      "projected_commission": "14000",
      "platform_operating_expense": null
    }
  ],
  "top_owner_breakdown": [
    {
      "owner_profile_id": "uuid",
      "business_name": "Arena Contoh",
      "realized_online_booking_count": 3,
      "online_gmv_net": "500000",
      "projected_commission": "35000"
    }
  ],
  "top_venue_breakdown": [
    {
      "venue_id": "uuid",
      "venue_name": "Arena Contoh Sudirman",
      "owner_profile_id": "uuid",
      "realized_online_booking_count": 3,
      "online_gmv_net": "500000",
      "projected_commission": "35000"
    }
  ],
  "caveats": [
    "Proyeksi komisi bukan pendapatan aktual dan belum ditagihkan kepada owner."
  ]
}
```

`projection_basis` bernilai:

- `HISTORICAL_SCENARIO`: seluruh proyeksi berasal dari scenario 7% non-billable;
- `BOOKING_SNAPSHOT`: seluruh proyeksi berasal dari immutable snapshots;
- `MIXED`: kedua basis ada pada periode yang sama.

`as_of` adalah database snapshot time yang mengikat metrics/trend/top breakdown; `generated_at` adalah waktu response selesai dibentuk.

`GET /admin/finance/breakdown` adalah tabel lengkap paginated. Response mengikuti pola admin existing dan setiap item memakai fields breakdown di atas. Sort default: `online_gmv_net DESC`, lalu entity ID `ASC` sebagai tie-breaker. Endpoint ini mempunyai `generated_at` sendiri dan tidak dijamin satu DB snapshot dengan summary; UI tidak boleh mencampurnya ke kartu/chart summary tanpa label waktu.
Top-10 breakdown dalam summary memakai sort/tie-breaker yang sama.

### Sumber Data Phase 1

Online realized booking:

- Sumber: `owner_finance_transactions` dengan `type='INCOME'` dan `source='BOOKING'`.
- Join ke booking, court, venue, owner profile.
- Exclude booking yang mempunyai row di `offline_booking_customers`.
- Untuk automated booking/refund ledger Phase 1, gunakan `created_at` yang dikonversi ke Jakarta sebagai event time; jangan mengandalkan `transaction_date=CURRENT_DATE` yang dapat mengikuti timezone database. Jangan gunakan `booking_date` atau status saja.
- Unique partial index booking ledger harus mencegah double count, tetapi query tetap defensif.

Refund:

- Sumber: `owner_finance_transactions` dengan `type='EXPENSE'` dan `source='REFUND'`.
- Hanya refund untuk booking online.
- Reversal proyeksi memakai komisi yang dihitung dari booking income terkait.

Jangan ikutkan:

- booking offline;
- manual owner income/expense;
- payroll/maintenance owner;
- booking hanya `CONFIRMED` tanpa booking income ledger;
- booking pending/waiting verification;
- mabar contribution informal.

Source cutover setelah gateway:

- Report memisahkan `legacy_manual_realized_gmv` dan `gateway_captured_gmv` lalu menjumlahkan union mutually exclusive.
- Jika canonical captured payment fact ada, booking tidak boleh sekaligus dihitung dari legacy owner auto-income.
- Gateway capture tidak memanggil legacy owner-cash insertion path.
- Response mengubah `metric_source_version` secara versioned dan tetap memberi source breakdown agar tren tidak diam-diam berganti basis.

### Service Rules

- Repository hanya melakukan SQL/data mapping.
- Service memvalidasi periode, mode, formula, dan empty response.
- Handler hanya binding, auth response, dan error mapping.
- Jangan menggunakan `float64` untuk formula baru. Convert nilai existing yang sudah dipastikan rupiah bulat ke `int64` dan fail closed bila ditemukan fraction/overflow.
- Proyeksi Phase 1 menggunakan scenario rate 700 bps dan harus ditandai `HISTORICAL_SCENARIO`, bukan billable snapshot.
- Metrics, trend, data quality, dan top-10 breakdown dalam response summary harus berasal dari satu SQL CTE atau read-only repeatable-read transaction agar tidak berbeda karena concurrent booking/refund.
- Endpoint breakdown lengkap berdiri sendiri dan harus paginated untuk dataset besar.
- Sort list harus deterministic, misalnya `occurred_at DESC, created_at DESC, id DESC` ketika event table tersedia.

### Automated Tests

Minimal table tests:

1. Online paid booking dihitung sebagai GMV.
2. Offline booking tidak dihitung.
3. Owner manual income tidak dihitung.
4. `CONFIRMED` tanpa ledger tidak dihitung.
5. Full refund mengurangi net GMV dan membalik proyeksi komisi.
6. Filter owner dan venue bekerja.
7. Batas tanggal inklusif dan konsisten Jakarta.
8. Invalid range menghasilkan 400.
9. Empty data menghasilkan struktur lengkap.
10. Route tanpa token, non-superadmin, dan suspended admin ditolak.
11. Metrik tanpa source of truth bernilai `null` + `data_availability`, tidak dipalsukan sebagai nol.
12. Granularity auto/day/week/month, bucket kosong, dan Monday-week boundary benar.
13. Metrics, trend, dan top breakdown dalam summary cocok pada satu `as_of` snapshot.
14. Paginated breakdown deterministic dan owner/venue mismatch ditolak.
15. Setelah Phase 2B: projection basis `HISTORICAL_SCENARIO`, `BOOKING_SNAPSHOT`, dan `MIXED` serta count/amount-nya benar.

### Verification

```powershell
Set-Location apps/api
go test ./internal/platformfinance
go test ./internal/admin ./internal/middleware
go test ./...
```

### Acceptance Criteria

- API membedakan GMV dan platform revenue.
- Semua response menyatakan `SIMULATION`.
- Tidak ada mutation endpoint.
- Tidak ada schema/data change.
- Angka fixture manual cocok dengan formula hingga rupiah terakhir.

---

## 8. Phase 1B — Admin Finance UI (Simulation)

### Tujuan

Membuat admin memahami bisnis platform tanpa memberi kesan bahwa proyeksi adalah kas aktual.

### Target Files

```text
apps/web/src/pages/admin/AdminFinancePage.tsx
apps/web/src/components/admin/AdminLayout.tsx
apps/web/src/components/admin/finance/*
apps/web/src/types/adminFinance.ts
apps/web/src/lib/api/adminFinance.ts
apps/web/src/App.tsx
```

### Route dan Navigasi

- Route: `/admin/finance`
- Sidebar label: `Keuangan Platform`
- Icon yang membedakan dari owner finance.
- Tetap dilindungi `SuperAdminRoute`.

### Struktur Halaman

1. Header `Keuangan Platform`.
2. Badge besar `MODE SIMULASI`.
3. Banner permanen:

   > Komisi di halaman ini masih berupa proyeksi. LapangGo belum memotong atau menerima komisi dari owner.

4. Filter periode, owner, dan venue.
5. Metric cards:
   - Online GMV Bruto;
   - Refund;
   - Online GMV Neto;
   - Proyeksi Komisi;
   - Pendapatan Aktual;
   - Biaya Payment Gateway;
   - OPEX Platform;
   - Proyeksi Operating Result sebelum seluruh transaction costs;
   - Operating Result (`Belum tersedia`).
6. Trend GMV dan proyeksi komisi.
7. Breakdown per owner/venue.
8. Data quality warning bila backend mengirim anomaly count.
9. Card `Cara LapangGo Menghasilkan Uang` yang menjelaskan 0% → 5% → 7%, online-only, dan offline exempt.

### UX Rules

- Jangan gunakan label “Laba Bersih” pada v1.7.
- Nilai aktual yang belum memiliki sumber kebenaran harus tampil `Belum tersedia`, bukan Rp0.
- Proyeksi dan aktual harus mempunyai warna/badge berbeda.
- Tooltip wajib menjelaskan setiap formula.
- Loading, empty, error, stale-data, dan retry state harus eksplisit.
- Saat request refresh gagal tetapi data lama ada, pertahankan data lama dan tampilkan warning.
- Metrics/trend/top breakdown memakai satu composite summary response sehingga tidak bercampur periode/as-of. Full paginated breakdown boleh gagal independen dan menampilkan per-table error tanpa mengganti summary.
- Filter mobile tidak boleh memaksa horizontal overflow.
- Integer-rupiah string harus diformat exact melalui helper/BigInt terpusat; jangan menghitung formula keuangan di komponen.
- Jangan menghitung ulang metrik utama di frontend.
- Sinkronkan filter dengan URL search params agar report dapat direload/share secara internal.
- Abort request lama ketika filter berubah cepat; response lama tidak boleh menimpa filter terbaru.
- Parse `YYYY-MM-DD` sebagai local calendar date, bukan `new Date('YYYY-MM-DD')` yang berpotensi bergeser UTC.
- Negative net GMV/projection akibat refund lintas periode harus ditampilkan apa adanya dengan penjelasan; jangan di-clamp ke nol.
- Chart harus didampingi tabel/angka exact untuk auditability.

### Frontend Verification

```powershell
Set-Location apps/web
npm.cmd run lint
npm.cmd run build
```

Manual QA minimum:

1. Superadmin dapat membuka halaman.
2. Customer/owner/staff tidak dapat membuka halaman.
3. Banner simulasi selalu terlihat.
4. Empty, loading, error, retry, dan stale state benar.
5. Filter tanggal/owner/venue mengubah data.
6. Mobile 360px dan desktop tidak overflow.
7. Angka UI sama persis dengan response API.
8. Owner search dan venue options memakai pagination/server-side filter; memilih owner membatasi venue dan tidak memuat seluruh dataset.
9. Rapid filter changes membatalkan request lama; panel dari periode berbeda tidak pernah tercampur.

### Acceptance Criteria

- Admin dapat memahami GMV vs revenue aktual dalam sekali lihat.
- Tidak ada UI untuk mengaktifkan komisi live.
- Tidak ada tombol mutation finance.
- Build dan lint lulus.

---

## 9. Phase 2A — Platform Audit Foundation dan Versioned Commercial Terms

### Tujuan

Menyediakan audit platform ownerless yang durable, menghilangkan hard-coded 7%, dan menyediakan histori tarif 0% → 5% → 7%.

### Migration

Nama contoh bila `018` masih terakhir:

```text
db/migrations/019_platform_audit_and_commercial_terms.up.sql
db/migrations/019_platform_audit_and_commercial_terms.down.sql
```

Antigravity wajib memilih nomor sequential berikutnya saat implementasi.

### Platform Audit Foundation

`owner_audit_logs.owner_profile_id` saat ini `NOT NULL`, dan audit service existing bersifat best-effort. Jangan memakai owner dummy untuk event platform.

Buat `platform_audit_logs` append-only dengan actor, action, entity, optional owner/venue reference, correlation ID, allowlisted metadata, IP, user agent, dan timestamp. Mutation commercial term harus menulis domain record + audit dalam transaction yang sama. Admin audit page/API menambahkan `scope=OWNER|PLATFORM|ALL`; history owner existing tidak diubah.

Platform audit table tidak boleh mempunyai update/delete endpoint dan financial mutation tidak boleh memakai fire-and-forget audit.

### Tabel `platform_commercial_terms`

Kolom minimum:

```text
id UUID PK
owner_profile_id UUID NULL FK owner_profiles
label VARCHAR(120) NOT NULL
phase VARCHAR NOT NULL CHECK TRIAL|INTRODUCTORY|STANDARD|CUSTOM
finance_mode VARCHAR NOT NULL CHECK SIMULATION|LIVE
collection_method VARCHAR NOT NULL CHECK NONE|DEDUCT_FROM_PAYOUT
commission_bps INTEGER NOT NULL CHECK 0..3000
valid_from TIMESTAMPTZ NOT NULL
valid_until TIMESTAMPTZ NULL
supersedes_id UUID NULL FK self
created_by_user_id UUID NULL FK users
created_at TIMESTAMPTZ NOT NULL
```

Semantik:

- `owner_profile_id IS NULL` berarti default platform.
- Satu owner dapat memiliki banyak versi historis tetapi tidak boleh memiliki rentang waktu yang overlap.
- Rate, mode, collection method, `valid_from`, dan `valid_until` yang sudah terisi bersifat immutable. Satu-satunya mutation yang diizinkan adalah `valid_until: NULL → new.valid_from` pada transaction supersession, future-effective, dan diaudit; snapshot existing tetap tidak berubah.
- Global default seed: `STANDARD`, `SIMULATION`, `NONE`, `700 bps`. Seed simulation tidak boleh dipakai untuk billing historical.
- Tidak ada seed `LIVE`.

Indexes/invariants:

- index owner + valid time;
- gunakan normalized `scope_key` (`GLOBAL` atau owner UUID) agar NULL global tidak lolos constraint;
- gunakan half-open `tstzrange(valid_from, COALESCE(valid_until,'infinity'), '[)')` dan DB exclusion constraint `scope_key =` + range overlap `&&` (misalnya dengan `btree_gist`);
- migration boleh `CREATE EXTENSION IF NOT EXISTS btree_gist`, tetapi down migration tidak menghapus shared extension;
- partial unique open-ended boleh menjadi tambahan, bukan satu-satunya proteksi;
- service memakai advisory/scope transaction lock saat create agar concurrent “first term” juga aman;
- `valid_until > valid_from` bila tidak null;
- Pada v1.7–v1.9, `finance_mode='LIVE'` hanya valid dengan `collection_method='DEDUCT_FROM_PAYOUT'` dan semua LIVE gates. `INVOICE` sengaja tidak ada sampai receivable/invoice/aging/collection/write-off mempunyai plan terpisah.

### Backend Contract

Endpoint admin:

```http
GET  /admin/commercial-terms?owner_profile_id=&status=&page=&limit=
POST /admin/commercial-terms/preview
POST /admin/commercial-terms
```

Setiap `POST` mutation finance/commercial term wajib `Idempotency-Key`; replay semantics mengikuti Section 11/12. GET tidak memerlukan key.

Tidak ada PATCH/DELETE pada phase ini.

Create term harus:

1. validate actor `SUPER_ADMIN`;
2. validate bps dan time window;
3. lock active term untuk scope;
4. menutup term lama jika `valid_from` menggantikannya;
5. membuat versi baru;
6. menulis `platform_audit_logs` dengan before/after dalam transaction yang sama;
7. menolak `LIVE` karena v1.7 simulation-only.

Preview harus menampilkan contoh booking Rp100.000, Rp200.000, dan Rp500.000 serta owner net, tanpa menyimpan perubahan.

### Audit Actions

Tambahkan konstanta eksplisit:

```text
PLATFORM_COMMERCIAL_TERM_CREATED
PLATFORM_COMMERCIAL_TERM_SUPERSEDED
PLATFORM_COMMERCIAL_TERM_LIVE_REJECTED
```

Entity type:

```text
PLATFORM_COMMERCIAL_TERM
```

Pastikan admin audit filter/whitelist dan `scope` menerima entity baru tanpa membuka owner data lintas scope.

### Tests

- global fallback;
- owner override;
- exact boundary `valid_from`/`valid_until`;
- overlapping term ditolak;
- historical term tidak berubah;
- invalid bps ditolak;
- LIVE ditolak saat kill switch off;
- concurrent create tidak membuat dua active term;
- Idempotency-Key same payload replays, different payload conflicts;
- non-superadmin ditolak;
- audit event tercatat.

### Acceptance Criteria

- 7% berasal dari satu source of truth.
- Histori 0/5/7 dapat direpresentasikan tanpa update row lama.
- Tidak ada term LIVE.
- Migration up/down teruji pada database kosong dan database berisi data.

---

## 10. Phase 2B — Immutable Booking Fee Snapshot (2B1–2B3)

### Tujuan

Memastikan tarif dan nominal komisi sebuah booking tidak berubah ketika commercial term berubah.

### Phase 2B1 — Schema, Term Resolver, dan Integer Calculator

#### Migration

Nama contoh:

```text
db/migrations/020_booking_fee_snapshots.up.sql
db/migrations/020_booking_fee_snapshots.down.sql
```

#### Tabel `booking_fee_snapshots`

Kolom minimum:

```text
booking_id UUID PK FK bookings ON DELETE RESTRICT
owner_profile_id UUID NOT NULL FK owner_profiles ON DELETE RESTRICT
venue_id UUID NOT NULL FK venues ON DELETE RESTRICT
commercial_term_id UUID NULL FK platform_commercial_terms ON DELETE RESTRICT
terms_source VARCHAR NOT NULL CHECK POLICY|LEGACY_NO_COMMISSION
booking_channel VARCHAR NOT NULL CHECK MARKETPLACE_ONLINE|OWNER_WALK_IN
finance_mode VARCHAR NOT NULL CHECK SIMULATION|LIVE
currency CHAR(3) NOT NULL DEFAULT IDR CHECK currency='IDR'
currency_exponent SMALLINT NOT NULL DEFAULT 0 CHECK currency_exponent=0
original_price_rupiah BIGINT NOT NULL CHECK >= 0
owner_price_adjustment_rupiah BIGINT NOT NULL
price_adjustment_reason TEXT NULL
final_booking_price_rupiah BIGINT NOT NULL CHECK >= 0
customer_service_fee_rupiah BIGINT NOT NULL DEFAULT 0 CHECK = 0
customer_charge_amount_rupiah BIGINT NOT NULL CHECK >= 0
commission_basis_amount_rupiah BIGINT NOT NULL CHECK >= 0
commission_bps INTEGER NOT NULL CHECK 0..3000
commission_amount_rupiah BIGINT NOT NULL CHECK >= 0
owner_net_amount_rupiah BIGINT NOT NULL CHECK >= 0
calculation_version VARCHAR(30) NOT NULL
created_at TIMESTAMPTZ NOT NULL
```

Invariants:

- Satu booking tepat satu snapshot.
- `OWNER_WALK_IN` selalu `commission_bps=0` dan `commission_amount_rupiah=0`.
- `LEGACY_NO_COMMISSION` selalu 0% untuk kewajiban/billing, walaupun API scenario analytics dapat menghitung simulasi 7% terpisah.
- `final_booking_price = original_price + owner_price_adjustment`; adjustment negatif untuk promo/diskon dan positif untuk offline markup. Simpan alasan override existing bila ada.
- DB memaksa `customer_service_fee_rupiah=0` pada model awal; migration service-fee kelak wajib sekaligus memperbarui capture/completion/refund journal templates.
- `customer_charge = final_booking_price + customer_service_fee`.
- `commission_basis = final_booking_price`.
- `commission_amount_rupiah` memakai pembulatan half-up ke rupiah penuh.
- `owner_net_amount_rupiah = basis - commission_amount`; UI/API menyebutnya projected/hypothetical pada simulation dan actual owner share hanya pada LIVE captured flow.
- Snapshot tidak mempunyai `updated_at` dan tidak menyediakan update repository method.
- Snapshot simulation tidak boleh digunakan untuk membuat actual revenue entry.

Helper kalkulasi tunggal harus menggunakan quotient/remainder integer dan memeriksa overflow sebelum `basis * bps`; untuk nilai positif, remainder `>= 5000` dibulatkan naik. Jangan tersebar sebagai rumus manual di repository/handler/frontend.

#### Immutable snapshot cutover record

Migration/deployment membuat satu audited `platform_finance_cutovers` record dengan `snapshot_cutover_at`, `calculation_version`, creator/release reference, dan `created_at`. Timestamp tidak boleh diedit setelah snapshot writer aktif.

Deployment order wajib:

```text
schema expand tanpa active cutover
→ deploy snapshot writer dalam pre-cutover SIMULATION dan verifikasi write-path
→ brief booking-create maintenance window
→ buat immutable cutover record + aktifkan snapshot-required atomically
→ resume booking create dan verifikasi semua booking >= cutover punya snapshot
→ backfill hanya missing booking created_at < cutover sebagai LEGACY_NO_COMMISSION
```

### Phase 2B2 — Transactional Write-path Integration

Integrasikan pembuatan snapshot secara transactional pada seluruh create booking path:

- booking online normal;
- booking online dengan promo;
- owner offline booking;
- path lain yang benar-benar membuat row booking.

Urutan:

1. Hitung harga final booking di backend.
2. Tentukan channel.
3. Resolve commercial term pada waktu booking dibuat.
4. Hitung snapshot dengan integer-safe arithmetic; gunakan helper tunggal yang menghindari overflow dan menerapkan half-up.
5. Insert booking dan snapshot dalam satu DB transaction.
6. Jika snapshot gagal, rollback booking.

Jangan percaya nilai komisi, rate, owner net, atau channel yang dikirim frontend.

### Phase 2B3 — Safe Legacy Backfill

Jangan memasukkan backfill besar langsung ke migration schema.

Buat command idempotent:

```text
apps/api/cmd/backfill-platform-finance/main.go
```

Mode wajib:

```text
--dry-run
--batch-size
--after-booking-id atau cursor setara
--apply
--cutover-at=<exact timestamp matching stored cutover record>
```

Backfill rules:

- booking dengan `offline_booking_customers` → `OWNER_WALK_IN`, rate 0;
- hanya booking `created_at < snapshot_cutover_at` → `terms_source=LEGACY_NO_COMMISSION`, rate 0, tidak billable, tidak membuat payable;
- booking historical online tetap dapat diberi channel `MARKETPLACE_ONLINE` untuk scenario analytics tanpa mengubah rate billing snapshot;
- basis memakai final price/total price yang tersimpan;
- tandai `calculation_version='legacy-backfill-v1'`;
- rerun aman melalui `ON CONFLICT DO NOTHING`;
- output hanya count/ringkasan; jangan log data personal.
- booking `created_at >= cutover_at` tanpa snapshot adalah P0 reconciliation exception; quarantine/repair melalui write-path-aware command, jangan dibebaskan sebagai legacy 0%.

Sebelum `--apply`:

- database backup tersedia;
- dry-run count direview;
- anomaly Phase 0 sudah dipahami;
- jalankan pada QA/staging lebih dahulu.

### Tests

- online 7%, intro 5%, trial 0%;
- offline selalu 0% walau global 7%;
- promo memakai final price;
- offline discount dan markup valid memakai signed adjustment/reason;
- rounding nominal kecil/batas .5;
- perubahan term tidak mengubah snapshot lama;
- concurrent/retry create tidak menduplikasi snapshot;
- booking rollback bila snapshot gagal;
- backfill dry-run tidak menulis;
- backfill apply idempotent.
- backfill hanya pre-cutover; missing post-cutover snapshot menjadi P0 dan tidak ditandai legacy.

Subphase gates:

- **2B1 GO:** migration up/down, term resolver, integer rounding/overflow, and immutable model tests pass; booking write paths belum berubah.
- **2B2 GO:** seluruh create-booking paths menulis snapshot atomically, retry/concurrency/rollback tests pass, dan owner/customer regression pass.
- **2B3 GO:** dry-run count direview, apply pada disposable/staging DB idempotent, reconciliation satu snapshot per booking lulus, dan historical rows tetap 0% billable.

Antigravity wajib berhenti pada setiap gate; jangan mengerjakan 2B1–2B3 dalam satu task.

### Acceptance Criteria

- Semua booking baru mempunyai snapshot.
- Data legacy dapat dibackfill secara aman dan idempotent.
- Admin Phase 1 memakai snapshot simulation untuk booking baru. Untuk historical row, API menghitung scenario projection terpisah dan menandainya non-billable.
- Mode tetap simulation.

---

## 10C. Phase 2C — Read-only Commercial Terms dan Platform Audit UI

### Tujuan

Membuktikan commercial terms dan audit platform dapat dilihat/diperiksa admin tanpa menambah mutation UI berisiko pada v1.7.

### Scope

- Tambahkan tab/read-only panel commercial terms: current, scheduled, historical, rate, mode, effective window, dan source/default.
- Tambahkan `scope=OWNER|PLATFORM|ALL` pada Admin Audit API/UI serta entity/action filters finance baru.
- Platform audit query harus paginated, deterministic, dan tidak mencampur owner scope secara salah.
- Term create/supersede tetap API-only pada v1.7 dan digunakan melalui test/admin operational procedure yang diaudit; mutation UI memerlukan phase terpisah sebelum pilot.
- LIVE option tidak tersedia.

### Acceptance Criteria

- Admin dapat membaca platform audit event yang dibuat Phase 2A.
- Owner/customer/staff tidak dapat membaca audit/terms admin.
- Historical term tidak tampak editable.
- Empty/loading/error/pagination/filter states benar.
- Frontend lint/build dan backend authorization/filter tests lulus.

---

## 11. Phase 3A — Minimal Double-entry Platform Ledger dan Platform Audit

### Tujuan

Membuat fondasi akuntansi LapangGo yang terpisah dari cashbook owner, append-only, balance, dan siap menerima payment/settlement tanpa migrasi arti data di kemudian hari. Pada v1.7 belum ada journal actual commission; journal awal hanya digunakan untuk Platform OPEX yang benar-benar diposting.

### Keputusan Arsitektur

Gunakan **double-entry ledger terbatas dengan chart of accounts tetap**, bukan tabel income/expense bersaldo tunggal dan bukan software akuntansi general-purpose.

Alasannya:

- payment capture menciptakan asset/clearing, owner payable, dan deferred commission sekaligus;
- payout adalah pengurangan liability, bukan operating expense;
- komisi baru menjadi revenue saat booking `COMPLETED`;
- refund sebelum/akhir payout memerlukan reversal yang dapat diseimbangkan;
- setiap journal dapat direkonsiliasi dan diuji `total debit = total credit`.

### Migration

Nama contoh:

```text
db/migrations/021_platform_double_entry_ledger.up.sql
db/migrations/021_platform_double_entry_ledger.down.sql
```

### Tabel `platform_journals`

```text
id UUID PK
event_key VARCHAR(191) NOT NULL UNIQUE
event_type VARCHAR(80) NOT NULL
booking_id UUID NULL FK bookings ON DELETE RESTRICT
owner_profile_id UUID NULL FK owner_profiles ON DELETE RESTRICT
venue_id UUID NULL FK venues ON DELETE RESTRICT
currency CHAR(3) NOT NULL DEFAULT IDR CHECK currency='IDR'
effective_at TIMESTAMPTZ NOT NULL
posted_at TIMESTAMPTZ NOT NULL
reverses_journal_id UUID NULL UNIQUE FK self ON DELETE RESTRICT
created_by_user_id UUID NULL FK users ON DELETE SET NULL
description TEXT NULL
metadata JSONB NOT NULL DEFAULT '{}'
created_at TIMESTAMPTZ NOT NULL
```

### Tabel `platform_ledger_entries`

```text
id UUID PK
journal_id UUID NOT NULL FK platform_journals ON DELETE RESTRICT
account_code VARCHAR(80) NOT NULL FK platform_accounts(code) ON DELETE RESTRICT
owner_profile_id UUID NULL FK owner_profiles ON DELETE RESTRICT
side VARCHAR NOT NULL CHECK DEBIT|CREDIT
amount_rupiah BIGINT NOT NULL CHECK > 0
created_at TIMESTAMPTZ NOT NULL
```

`platform_accounts` adalah catalog migration-owned/immutable berisi `code`, `account_type`, dan normal side. Chart of accounts awal:

```text
BANK_CASH
PSP_CLEARING
FUNDING_CLEARING
ACCOUNTS_PAYABLE
OWNER_RECEIVABLE
OWNER_PAYABLE
CUSTOMER_REFUND_PAYABLE
REFUND_CLEARING
PAYOUT_CLEARING
UNEARNED_COMMISSION
UNEARNED_SERVICE_FEE
COMMISSION_REVENUE
SERVICE_FEE_REVENUE
COMMISSION_REFUND
PAYMENT_PROCESSING_EXPENSE
REFUND_FEE_EXPENSE
PAYOUT_FEE_EXPENSE
CHARGEBACK_LOSS
OPEX_INFRASTRUCTURE
OPEX_MARKETING
OPEX_CUSTOMER_SUPPORT
OPEX_SALARY_CONTRACTOR
OPEX_LEGAL_COMPLIANCE
OPEX_PAYMENT_OPERATIONS
OPEX_OFFICE_ADMIN
OPEX_OTHER
```

Rules:

- Satu journal mempunyai minimal dua entries.
- Total debit harus sama persis dengan total credit sebelum commit.
- Gunakan database transaction dan deferred constraint trigger atau posting function yang fail closed.
- Journal dan entries `POSTED` tidak dapat di-update/delete.
- Koreksi membuat reversal journal yang membalik persis seluruh entries asal.
- Satu journal hanya dapat direverse sekali.
- `event_key` menjadi idempotency boundary, misalnya `booking.completed:<booking-id>`.
- `ON DELETE CASCADE` dilarang untuk journal/entries/referensi finansial.
- Entry `OWNER_PAYABLE`/`OWNER_RECEIVABLE` wajib mempunyai `owner_profile_id` melalui DB constraint/trigger; account lain mengikuti source-specific rule.
- Metadata menggunakan allowlist dan tidak berisi secret/provider payload/PII tidak perlu.

### Platform Audit yang Durable

Gunakan `platform_audit_logs` yang sudah dibuat pada Phase 2A. Jika Phase 2A belum menyediakannya, Phase 3A tidak boleh dimulai. Struktur minimum:

```text
id UUID PK
actor_user_id UUID NULL FK users ON DELETE SET NULL
actor_role VARCHAR NOT NULL
action VARCHAR NOT NULL
entity_type VARCHAR NOT NULL
entity_id UUID NULL
correlation_id VARCHAR NULL
metadata JSONB NOT NULL DEFAULT '{}'
ip_address TEXT NULL
user_agent TEXT NULL
created_at TIMESTAMPTZ NOT NULL
```

Mutation finance harus menulis journal/domain record dan platform audit dalam DB transaction yang sama. Jangan memakai fire-and-forget audit untuk aksi finansial kritis. Admin Audit API/UI dapat menambahkan filter `scope=OWNER|PLATFORM|ALL` dan melakukan union/read terpisah tanpa mengubah owner audit history.

Audit actions minimum:

```text
PLATFORM_EXPENSE_CREATED
PLATFORM_EXPENSE_POSTED
PLATFORM_FINANCE_JOURNAL_REVERSED
PLATFORM_FINANCE_LIVE_WRITE_REJECTED
PLATFORM_COMMERCIAL_TERM_CREATED
PLATFORM_COMMERCIAL_TERM_SUPERSEDED
```

### Repository/Service Methods

Gunakan intent-specific methods:

```go
PostJournal(...)
ReverseJournal(...)
ListJournals(...)
GetSummary(...)
```

Phase 3A hanya membangun primitives generic yang tervalidasi; jangan membuat payment/commission/refund/payout posting methods sebelum provider fund-flow dan phase-specific contract disetujui. `PostOperatingExpense` dibuat pada Phase 3B. Domain-specific posting methods dibuat bersama Phase 5/6 dan menggunakan primitives ini.

### Idempotency

- Mutation admin wajib menerima `Idempotency-Key`.
- Simpan request hash bersama key atau pada tabel idempotency request.
- Key sama + payload sama mengembalikan hasil sebelumnya.
- Key sama + payload berbeda menghasilkan 409.
- Provider event memakai external event ID sebagai sumber event key.
- Jangan membuat random event key baru pada retry.

### Tests

- journal balance dan unbalanced rejection;
- amount nol/negatif/overflow ditolak;
- unknown account ditolak;
- event key idempotent;
- key sama payload berbeda ditolak;
- reversal membalik exact entries dan double reversal ditolak;
- journal/entry immutable;
- concurrent retry hanya membuat satu journal;
- transaction rollback tidak meninggalkan partial journal/audit;
- LIVE write ditolak ketika guard belum terpenuhi;
- platform audit ditulis atomic dan metadata aman.

### Acceptance Criteria

- Owner cashbook dan platform ledger terpisah.
- Setiap posted journal balance exact dengan selisih Rp0; selisih Rp1 tetap gagal.
- Platform audit ownerless bersifat durable.
- Tidak ada actual commission/payment/payout journal di v1.7.
- Migration up/down teruji pada database disposable.

---

## 12. Phase 3B — Platform Operating Expense

### Tujuan

Memberikan admin kemampuan mencatat biaya riil LapangGo tanpa mencampurnya dengan biaya operasional venue.

### Phase 3B1 — Expense Schema, Backend Workflow, Journal, dan Reporting

#### Kategori Awal

Gunakan enum/service whitelist, bukan string bebas untuk kategori utama:

```text
INFRASTRUCTURE
MARKETING
CUSTOMER_SUPPORT
SALARY_CONTRACTOR
LEGAL_COMPLIANCE
PAYMENT_OPERATIONS
OFFICE_ADMIN
OTHER
```

Jika `OTHER`, description wajib dan harus cukup menjelaskan transaksi.

Setelah gateway tersedia, processing/refund/payout fee wajib berasal dari provider cost items. Jangan menginputnya lagi sebagai manual OPEX karena akan double count. `PAYMENT_OPERATIONS` hanya untuk biaya operasional non-provider yang memiliki bukti/referensi.

#### Tabel `platform_expenses`

Tambahkan pada migration Phase 3B (contoh `022_platform_expenses.{up,down}.sql`; cek nomor terbaru):

```text
id UUID PK
category VARCHAR NOT NULL
vendor VARCHAR NULL
amount_rupiah BIGINT NOT NULL CHECK > 0
currency CHAR(3) NOT NULL DEFAULT IDR CHECK currency='IDR'
occurred_at TIMESTAMPTZ NOT NULL
payment_account VARCHAR NOT NULL CHECK FUNDING_CLEARING|ACCOUNTS_PAYABLE
external_reference VARCHAR NULL
description TEXT NOT NULL
status VARCHAR NOT NULL CHECK DRAFT|APPROVED|POSTED|VOID|CANCELLED
posted_journal_id UUID NULL UNIQUE FK platform_journals ON DELETE RESTRICT
void_journal_id UUID NULL UNIQUE FK platform_journals ON DELETE RESTRICT
created_by_user_id UUID NOT NULL FK users ON DELETE RESTRICT
approved_by_user_id UUID NULL FK users ON DELETE SET NULL
posted_by_user_id UUID NULL FK users ON DELETE SET NULL
voided_by_user_id UUID NULL FK users ON DELETE SET NULL
cancelled_by_user_id UUID NULL FK users ON DELETE SET NULL
cancel_reason/void_reason TEXT NULL
created_at TIMESTAMPTZ NOT NULL; approved_at/posted_at/voided_at/cancelled_at nullable by state
```

Attachment/reference bukti dapat ditambahkan setelah storage policy tersedia; jangan menyimpan file sensitif tanpa access control dan retention policy.

#### Endpoint

Semua mutation di bawah wajib header `Idempotency-Key`. Key yang sama disimpan sepanjang retry satu user action.

```http
GET  /admin/finance/journals?start_date=&end_date=&event_type=&account_code=&page=&limit=
GET  /admin/finance/expenses?status=&category=&page=&limit=
POST /admin/finance/expenses
POST /admin/finance/expenses/:id/cancel
POST /admin/finance/expenses/:id/approve
POST /admin/finance/expenses/:id/post
POST /admin/finance/expenses/:id/void
```

Create expense body:

```json
{
  "amount_rupiah": "250000",
  "currency": "IDR",
  "occurred_at": "2026-07-11T10:00:00+07:00",
  "category": "INFRASTRUCTURE",
  "payment_account": "FUNDING_CLEARING",
  "vendor": "Cloud Vendor",
  "external_reference": "INV-2026-07",
  "description": "Hosting staging dan production bulan Juli"
}
```

Create hanya membuat status `DRAFT` dan belum memengaruhi P&L. Approve mengunci isi bisnis expense. Post hanya menerima `APPROVED` dan membuat journal:

```text
Dr OPEX_<CATEGORY>  amount
Cr FUNDING_CLEARING atau ACCOUNTS_PAYABLE  amount
```

Void body untuk expense yang sudah posted:

```json
{
  "reason": "Nominal invoice salah; akan diposting ulang dengan nilai yang benar"
}
```

#### Validation

- Superadmin active only.
- Amount request integer-rupiah string (`^[0-9]+$`), diparse checked ke int64, lebih dari nol, dan maksimum wajar yang didokumentasikan.
- Currency hanya IDR untuk MVP.
- Future `occurred_at` ditolak kecuali aturan accrual future ditambahkan secara eksplisit.
- Description trimmed, memiliki batas panjang, dan wajib untuk `OTHER`.
- State transition matrix bersifat deny-by-default:

  ```text
  DRAFT    -> APPROVED | CANCELLED
  APPROVED -> POSTED
  POSTED   -> VOID
  CANCELLED dan VOID terminal
  ```

- Cancel DRAFT membutuhkan reason + audit dan tidak membuat journal. APPROVED tidak boleh diedit/cancel; POSTED hanya dapat di-void dengan reversal journal.
- Tidak ada PATCH/DELETE untuk POSTED expense atau journal.
- Void/reversal reason wajib.
- `payment_account` hanya `FUNDING_CLEARING` atau `ACCOUNTS_PAYABLE` pada v1.7. Jangan mengklaim saldo bank aktual sebelum opening balance/import dan bank reconciliation tersedia.
- External reference harus unik dalam scope vendor bila diisi untuk mencegah duplicate invoice.
- Handler tidak menerima journal side/account/status/created_by dari client; server menetapkannya.
- Bila maker-checker admin belum tersedia, approve/post membutuhkan explicit confirm dan audit tetapi readiness document harus mencatat residual risk. Nilai material wajib `created_by/approved_by` berbeda sebelum production LIVE.

### Phase 3B2 — Expense UI

Tambahkan tab `Pengeluaran Platform`:

- daftar expense workflow dan journal immutable;
- tombol `Tambah Pengeluaran`;
- modal dengan summary sebelum submit;
- confirm modal terpisah saat `Approve` dan `Post` karena baru saat post P&L berubah;
- tombol `Void dengan Reversal`, bukan edit/hapus, untuk POSTED expense;
- journal reversed tetap tampil dengan badge dan link ke reversal;
- pending submit disabled untuk mencegah double click;
- stale/error/retry state.

UI harus menjelaskan bahwa ini biaya LapangGo, bukan biaya venue.

Frontend membuat key sekali ketika user memulai create/cancel/approve/post/void (misalnya `crypto.randomUUID()`), menyimpannya di state action sampai response terminal, dan memakai key yang sama untuk retry transport/timeout. Double click tidak boleh membuat key kedua. Key baru hanya untuk aksi bisnis baru. Server boleh mengembalikan header `Idempotency-Replayed: true` pada replay.

Reporting time semantics:

- Expense POSTED diakui pada `journal.effective_at = expense.occurred_at`, walaupun `posted_at` lebih lambat.
- VOID reversal diakui pada `journal.effective_at = voided_at`; jangan menulis ulang periode expense asal.
- Backdated expense diperbolehkan dalam range kebijakan dan wajib terlihat sebagai backdated/audited.
- DRAFT, APPROVED, dan CANCELLED tidak masuk summary/trend/OPEX.

Phase 3B juga wajib mengubah summary repository/types/UI secara vertical slice: OPEX posted/reversal masuk metrics dan trend, `data_availability.platform_operating_expense` menjadi `AVAILABLE`, serta card berubah dari `Belum tersedia` menjadi nilai (0 jika tersedia tetapi tidak ada posted expense).

### Tests/QA

1. Create expense menghasilkan DRAFT dan belum masuk summary.
2. Draft dapat cancel tanpa journal; draft harus approved sebelum post; seluruh invalid transition ditolak.
3. Post expense membuat balanced journal dan masuk summary.
4. Same key/same payload dan timeout-after-commit mengembalikan hasil lama tanpa duplikasi; same key/different payload menghasilkan 409.
5. Double click UI tidak menggandakan approve/post.
6. Invalid amount/category/date/account ditolak.
7. Non-admin ditolak.
8. Void membuat exact reversal dan memperbaiki summary pada periode event reversal.
9. Double approve/post/void ditolak.
10. Platform audit menyimpan actor, IP, user agent, reference, correlation ID, dan reason secara atomic.
11. Tidak ada edit/delete action untuk APPROVED/POSTED data di API maupun UI.
12. Backdated posting dan void lintas bulan mengikuti `effective_at` rules.
13. Summary/trend/UI berubah ke OPEX `AVAILABLE`; hanya POSTED net reversal yang terhitung.

### Acceptance Criteria

- Admin dapat melihat platform OPEX aktual.
- API mengganti `data_availability.platform_operating_expense` menjadi `AVAILABLE`; periode tanpa posted expense baru bernilai 0.
- Actual Platform Revenue/Transaction Contribution/Operating Result tetap `Belum tersedia`; hanya projected operating result before transaction costs yang boleh dikurangi OPEX.
- Setiap mutation auditable dan idempotent.

---

## 13. Phase 4 — v1.7 Reconciliation, Hardening, dan Release Gate

### Tujuan

Membuktikan dashboard simulation benar sebelum menyentuh payment gateway.

### Data Reconciliation Endpoint/Check

Internal/admin-only diagnostics:

```http
GET /admin/finance/reconciliation?start_date=YYYY-MM-DD&end_date=YYYY-MM-DD
```

Atau command read-only jika endpoint dianggap terlalu berisiko:

```text
apps/api/cmd/reconcile-platform-finance/main.go --start-date=... --end-date=... --dry-run
```

Range, timezone, half-open boundary, dan maksimum 366 hari sama dengan summary. Implementasi boleh menjalankan check harian lalu mengagregasi range, tetapi response harus menyatakan bucket/date setiap exception.

Checks minimum:

- realized online booking ledger = GMV source rows;
- setiap snapshot online paid mempunyai source booking ledger;
- offline snapshot selalu rate 0;
- full refund mempunyai exact commission projection reversal;
- tidak ada duplicated snapshot/ledger event;
- summary = sum breakdown = sum trend untuk periode sama;
- platform OPEX summary = posted entries - reversal effect;
- semua actual commission/payment/payout metrics `UNAVAILABLE`/`null` di simulation, bukan dipalsukan sebagai kas Rp0.

### Full Verification

```powershell
Set-Location apps/api
go test ./...

Set-Location ../web
npm.cmd run lint
npm.cmd run build
```

Jika perubahan memengaruhi flow end-to-end:

```powershell
Set-Location ../..
./scripts/smoke_test.ps1
```

### Manual QA v1.7

1. `AF-P0-01` Booking online tanpa promo → snapshot dan GMV benar.
2. `AF-P0-02` Booking online dengan promo → basis final price.
3. `AF-P0-03` Booking offline → muncul di owner finance tetapi tidak di online GMV/komisi.
4. `AF-P0-04` Payment verification retry → tidak duplicate.
5. `AF-P0-05` Full refund → owner ledger tetap benar dan projected commission berbalik tepat.
6. `AF-P0-06` Rate 0/5/7 dan historical non-billable fixture → breakdown benar.
7. `AF-P0-07` Perubahan rate → booking lama tidak berubah.
8. `AF-P0-08` Admin OPEX create/retry/reversal → balanced journal dan summary benar.
9. `AF-P0-09` Platform audit memuat mutation secara atomic.
10. `AF-P0-10` Customer/owner/staff tidak dapat mengakses admin finance.
11. `AF-P1-01` Owner dashboard dan finance existing tidak regression.
12. `AF-P0-11` Range/tanggal sekitar pergantian hari Jakarta konsisten.
13. `AF-P0-12` Reconciliation metrics/trend/breakdown/ledger menghasilkan unexplained difference Rp0.
14. `AF-P1-02` Loading/empty/error/stale/rapid-filter/mobile UX lulus.

GO memerlukan seluruh P0 PASS dan tidak ada P1 blocker yang disepakati belum selesai. Setiap FAIL harus menyimpan evidence dan owner perbaikan; jangan menandainya “known limitation” jika menyangkut uang/auth/idempotency/reconciliation.

### Release Criteria v1.7

- Semua automated checks lulus.
- Semua manual QA P0/P1 lulus.
- Rekonsiliasi tidak mempunyai unexplained difference.
- UI selalu menampilkan simulation banner.
- Kill switch default `false` dan startup config membuktikannya.
- Tidak ada provider secret atau production credential.
- Tidak ada actual commission, payout, owner deduction, atau customer service fee.
- Release note menyatakan dashboard adalah proyeksi operasional, bukan laporan pajak.
- `docs/mvp_known_limitations.md` diperbarui: admin analytics tersedia, tetapi payment gateway, actual commission, owner payable, dan payout tetap belum tersedia pada v1.7.

### Rollback v1.7

- UI route dapat dimatikan tanpa mengubah owner flow.
- API simulation bersifat read-only.
- Platform OPEX mutation dapat dihentikan dengan feature flag/routing tanpa menghapus journal.
- Jangan rollback migration dengan drop table jika sudah ada posted journal tanpa backup dan approval eksplisit.
- Dependency order down pada disposable DB: platform expenses → ledger → snapshots → terms/audit. Snapshot down menolak jika backfill/data belum dikosongkan secara eksplisit; ledger down `RAISE EXCEPTION` bila ada posted journal; terms/audit down menolak bila masih direferensikan.
- Uji down pre-cutover sukses dan expected refusal post-first-finance-fact. Setelah first posted fact, rollback production adalah feature disable + app rollback + roll-forward schema, bukan table drop.
- Jika snapshot integration mengganggu booking creation setelah cutover, rollback app hanya sambil menempatkan create-booking dalam maintenance/deny mode sampai snapshot-capable writer pulih; schema expand tetap. Jika race menghasilkan post-cutover missing snapshot, jadikan P0 dan repair memakai original term/channel data—jangan tandai legacy 0%. Tidak ada klaim GO/LIVE selama anomaly tersisa; jangan menghapus booking user.

---

## 14. Phase 5 — Payment Gateway Foundation (v1.8, Tetap Shadow Mode)

### Entry Gate

Phase ini tidak boleh dimulai sebelum v1.7 release gate lulus dan provider dipilih secara resmi.

Keputusan di luar kode yang wajib tersedia:

- badan usaha dan akun merchant;
- provider marketplace/payment yang dipilih;
- metode pembayaran awal;
- aturan MDR/processing fee dan pajak;
- kemampuan split/marketplace payout atau settlement;
- refund API dan batas waktunya;
- webhook signing specification;
- KYC owner dan kebutuhan rekening;
- Terms of Service, Privacy Policy, refund policy, dan persetujuan commercial term.

Refund policy freeze sebelum provider coding:

- Pertahankan v1 eligibility: booking paid/captured, full refund only, request minimal 1 jam sebelum jadwal.
- Owner/system cancellation memberi full refund customer.
- Booking `COMPLETED` tidak memakai ordinary refund; gunakan dispute/support flow.
- Booking cancellation/slot availability state dan money refund state ditampilkan terpisah.
- Setelah approval, money status tetap `PROCESSING` sampai provider `SUCCEEDED`.
- Unresolved request menahan owner payable/payout dan masuk admin escalation.
- SLA pilot yang direkomendasikan: owner merespons maksimal 30 menit dan tidak melewati 30 menit sebelum jadwal; breach memicu escalation, bukan false “refund sukses”. Final SLA harus ditandatangani product/operations sebelum pilot.
- Provider refund cost ditanggung LapangGo pada model awal.

Jangan membangun penyimpanan kartu atau custody/escrow sendiri. Gunakan hosted checkout/tokenization dan produk marketplace provider yang sesuai.

Hard gate provider fund-flow:

- Buat provider-specific ADR yang menjelaskan siapa merchant/seller of record, siapa menguasai dana, kapan owner liability timbul, siapa menanggung refund/chargeback, rekening/subaccount tujuan settlement, dan apakah fee dipotong dari clearing atau ditagih terpisah.
- Dapatkan konfirmasi kontraktual/legal bahwa marketplace/split/payout flow tersebut diizinkan.
- Catat approver name/role/date dan referensi dokumen pada readiness artifact.
- Journal examples di plan ini bersifat conceptual; provider ADR wajib memilih akun clearing/cash/AP yang benar. Tanpa ADR dan signoff tersebut, Phase 5 berstatus NO-GO.

### Schema Minimum

Migration berikutnya membuat:

#### `payment_attempts`

```text
id UUID PK
booking_id UUID NOT NULL FK booking_fee_snapshots(booking_id) ON DELETE RESTRICT
attempt_no INTEGER NOT NULL
provider VARCHAR NOT NULL
provider_payment_id VARCHAR NULL   # required once provider create succeeds; unique when non-null
provider_reference VARCHAR
idempotency_key VARCHAR UNIQUE
payment_method VARCHAR
payment_rail VARCHAR NOT NULL CHECK GATEWAY|MANUAL_DIRECT|CASH|OTHER
collection_mode VARCHAR NOT NULL CHECK MANUAL_DIRECT|PLATFORM_COLLECTED
monetization_decision VARCHAR NOT NULL CHECK SIMULATION|LIVE
decision_at TIMESTAMPTZ NOT NULL
amount_rupiah BIGINT NOT NULL CHECK > 0
currency CHAR(3) NOT NULL CHECK currency='IDR'
status VARCHAR NOT NULL CHECK CREATED|PENDING|CAPTURED|FAILED|EXPIRED|CANCELLED
checkout_url TEXT NULL
expires_at TIMESTAMPTZ NULL
captured_at TIMESTAMPTZ NULL
created_at/updated_at
UNIQUE(provider, provider_payment_id)
UNIQUE(booking_id, attempt_no)
UNIQUE(booking_id) WHERE captured_at IS NOT NULL   # partial unique atas immutable capture fact
```

`captured_at` tidak pernah di-null-kan dan payment attempt tetap menjadi capture fact setelah refund. Refund state hanya berada di `payment_refunds`; test capture → refund → capture kedua harus tetap ditolak.

#### `payment_webhook_events`

```text
id UUID PK
provider VARCHAR
provider_event_id VARCHAR
event_type VARCHAR
signature_verified BOOLEAN
processing_status RECEIVED|PROCESSED|IGNORED|FAILED
payload_hash VARCHAR
payload_redacted JSONB
error_code VARCHAR NULL
received_at/processed_at
UNIQUE(provider, provider_event_id)
```

Jangan simpan raw secret, card PAN, CVV, atau payload sensitif penuh.

Phase 5 migration juga menambah nullable `payment_attempt_id`, `payment_refund_id`, dan `payment_cost_item_id` FKs `ON DELETE RESTRICT` pada `platform_journals`, plus event-type/source checks dan unique source-event invariant. Jangan hanya menaruh reference di metadata.

#### `payment_cost_items`

Biaya actual provider bersifat append-only:

```text
id UUID PK
payment_id UUID UNIQUE FK payment_attempts ON DELETE RESTRICT
cost_type VARCHAR NOT NULL CHECK PROCESSING_FEE|PROVIDER_TAX|REFUND_FEE|ADJUSTMENT
effect VARCHAR NOT NULL CHECK CHARGE|REVERSAL
amount_rupiah BIGINT NOT NULL CHECK > 0
provider_reference VARCHAR
occurred_at TIMESTAMPTZ
UNIQUE(payment_id, cost_type, provider_reference)
```

Estimasi fee tidak boleh masuk actual expense. Jika provider belum mengonfirmasi fee, actual contribution tetap `UNAVAILABLE/UNRECONCILED`.
Amount selalu positif; `effect` menentukan charge atau rebate/reversal dan posting. Jangan memakai signed amount tanpa rule.

#### `payment_refunds`

`booking_refund_requests` tetap workflow approval; `payment_refunds` menjadi fakta money movement.

```text
id UUID PK
refund_request_id UUID UNIQUE FK booking_refund_requests ON DELETE RESTRICT
payment_id UUID FK payment_attempts ON DELETE RESTRICT
provider_refund_id VARCHAR NULL UNIQUE
idempotency_key VARCHAR UNIQUE
amount_rupiah BIGINT NOT NULL CHECK > 0
status VARCHAR NOT NULL CHECK REQUESTED|PROCESSING|SUCCEEDED|FAILED
requested_at/succeeded_at/created_at/updated_at
```

Phase awal tetap full refund. `amount_rupiah` harus sama dengan captured customer amount. Approval owner tidak sama dengan uang sudah kembali.
DB/service wajib memastikan total refund `SUCCEEDED` tidak pernah melebihi captured amount; untuk v1 satu successful full refund harus persis captured amount.

#### `finance_outbox`

Provider API call untuk payment/refund/payout tidak boleh dilakukan sambil menahan DB transaction. Gunakan transactional outbox dengan command idempotency key, attempt count, next retry, terminal status, dan redacted payload.

### Provider Adapter

Interface minimum:

```go
CreatePayment(...)
VerifyWebhook(...)
ParseWebhook(...)
GetPaymentStatus(...)
RequestRefund(...)
```

Business service tidak boleh bergantung langsung pada SDK/provider-specific DTO.

### Payment State Rules

- Browser redirect/callback tidak boleh menandai booking paid.
- Hanya verified webhook atau server-to-server reconciliation yang dapat mengakui payment.
- Webhook processing harus idempotent dan transactional.
- Amount, currency, booking reference, dan current state harus diverifikasi.
- Out-of-order webhook tidak boleh menurunkan state final.
- Duplicate webhook harus menghasilkan no-op sukses.
- Provider timeout tidak berarti payment gagal; status harus direkonsiliasi.
- Existing manual proof flow dipertahankan sebagai fallback terkontrol selama pilot, bukan dicampur tanpa label.
- Payment status harus terpisah dari fulfillment/booking status.
- `bookings.payment_reference` existing tidak boleh dipakai ulang sebagai canonical provider payment ID.
- Captured amount harus sama dengan `booking_fee_snapshots` customer/basis contract yang berlaku.
- Booking historical/manual direct tidak membuat PSP clearing, owner payable, atau actual platform commission.
- Jika verified capture datang setelah booking lokal expired/cancelled, jangan membuka slot kembali atau mengakui revenue otomatis. Catat reconciliation exception, hold dana, dan jalankan refund idempotent sesuai runbook.
- Race capture/refund/completion/payout harus memakai row lock dan deny-by-default state transition.
- Pada LIVE provider flow, normalized capture service secara atomic mengubah payment fact ke `CAPTURED`, booking fulfillment state ke `PAID`, membuat deferred journal/payable/notification yang diwajibkan, dan **tidak** memanggil legacy owner-cash insertion path.

### Revenue Recognition dan Journal Events

Komisi belum menjadi revenue pada payment capture. Journal target ketika LIVE kelak:

Payment captured:

```text
Dr PSP_CLEARING          customer captured amount
Cr OWNER_PAYABLE        owner entitlement
Cr UNEARNED_COMMISSION  commission snapshot
```

Journal builder omits zero-value lines. Pada rate 0%, capture hanya `Dr PSP_CLEARING / Cr OWNER_PAYABLE`; completion mencatat idempotent domain marker tanpa zero-amount revenue journal.

Actual provider fee confirmed:

```text
Dr PAYMENT_PROCESSING_EXPENSE  actual fee
Cr PSP_CLEARING                actual fee
```

Booking completed:

```text
Dr UNEARNED_COMMISSION  exact snapshot commission
Cr COMMISSION_REVENUE   exact snapshot commission
```

Full refund succeeded sebelum completion:

```text
Dr OWNER_PAYABLE        owner entitlement
Dr UNEARNED_COMMISSION  commission
Cr PSP_CLEARING         captured amount
```

Exceptional refund/dispute setelah completion tetapi sebelum payout:

```text
Dr OWNER_PAYABLE       owner share
Dr COMMISSION_REFUND   earned commission contra
Cr PSP/REFUND_CLEARING captured amount
```

Exceptional refund/chargeback setelah payout:

```text
Dr OWNER_RECEIVABLE    owner share to recover
Dr COMMISSION_REFUND   earned commission contra
Cr PSP/REFUND_CLEARING captured amount
```

Future payout offset:

```text
Dr OWNER_PAYABLE
Cr OWNER_RECEIVABLE
```

Refund/completion/payout race memakai row locks dan menentukan deferred-versus-earned/paid path dari existing immutable journals/state dalam satu transaction.

Normal refund flow tidak boleh mengklaim sukses atau membuat final reversal sampai provider mengonfirmasi `SUCCEEDED`. Refund setelah revenue/payout atau chargeback harus memakai dispute/negative owner balance flow Phase 6, bukan mengedit journal lama.

Gateway/refund fee yang tidak dikembalikan provider menjadi platform expense pada model awal dan harus masuk `payment_cost_items`; jangan mengurangi refund customer atau owner entitlement tanpa keputusan bisnis baru.

Scheduler `PAID -> COMPLETED` menjadi dependency recognition. Journal `effective_at` memakai waktu layanan selesai, bukan waktu worker terlambat berjalan; `posted_at` menyimpan processing time. Worker health, idempotent replay, dan unique `booking.completed:<id>` journal wajib ada sebelum LIVE.

### Shadow Mode

Pada v1.8 awal:

- payment hanya diuji di provider sandbox atau melalui redacted production-event replay/read-only comparison; tidak ada real customer funds;
- snapshots dan projected commission dibandingkan dengan provider fee aktual;
- `PLATFORM_MONETIZATION_ENABLED=false`;
- tidak ada commission deduction;
- tidak ada payout otomatis;
- tidak ada production actual commission journal dan UI tetap menampilkan actual revenue sebagai `UNAVAILABLE`;
- payment processing expense boleh masuk hanya pada sandbox/test ledger yang terisolasi, bukan production report.

### Tests

- valid/invalid signature;
- duplicate webhook;
- webhook out of order;
- amount mismatch;
- booking mismatch;
- expired payment kemudian late success;
- provider timeout/retry;
- no card/secret logged;
- gateway flow tidak memanggil legacy `VerifyPayment`/`MarkBookingPaid` untuk mencatat full owner cash income;
- refund retry idempotency;
- kedua jalur refund existing memetakan ke satu normalized payment refund flow tanpa duplicate reversal;
- manual fallback tidak double recognize payment.
- runtime shadow capture/completion hanya menghasilkan provider facts + projected comparison; actual journal write ditolak fail-closed;
- LIVE posting templates (capture, completion, fee, refund) balance dan idempotent hanya pada isolated test ledger dengan explicit test capability;
- refund approval tanpa provider success belum membuat final money reversal;
- refund success pada shadow tetap comparison/fact; isolated template test membuat balanced exact-snapshot reversal;
- scheduler retry tidak duplicate projection/event marker dan tidak membuat actual revenue.

### Exit Gate

- Sandbox E2E lulus untuk setiap metode pembayaran.
- Reconciliation payment provider versus booking 100% untuk dataset pilot.
- Tidak ada duplicate income ledger.
- Failure/retry observability tersedia.
- Incident runbook tersedia.
- Mode tetap simulation sampai Phase 6 selesai.

---

## 15. Phase 6 — Owner Payable, Settlement, dan Payout (v1.9)

### Tujuan

Mencatat kewajiban dana owner dan payout batch secara aman sebelum komisi live.

### Data Model

#### `owner_payables`

Operational control/subledger satu row per eligible booking:

```text
id UUID PK
booking_id UUID UNIQUE FK bookings ON DELETE RESTRICT
owner_profile_id UUID FK owner_profiles ON DELETE RESTRICT
amount_rupiah BIGINT NOT NULL CHECK >= 0
status VARCHAR NOT NULL CHECK PENDING|AVAILABLE|HELD|ALLOCATED|PAID|REVERSED
available_at TIMESTAMPTZ NULL
hold_reason VARCHAR NULL
version INTEGER NOT NULL
created_at/updated_at
```

Transisi:

```text
gateway captured                          -> PENDING
booking COMPLETED + provider settled
  + safety hold lewat + no dispute        -> AVAILABLE
masuk payout batch                        -> ALLOCATED
payout sukses                             -> PAID
payout gagal/unknown                      -> tetap ALLOCATED; retry attempt pada payout aggregate yang sama setelah inquiry/review
refund sebelum payout                     -> REVERSED
```

#### `owner_balance_adjustments` / receivable subledger

Satu positive payable row tidak cukup untuk chargeback/refund setelah payout. Tambahkan append-only subledger:

```text
id UUID PK
owner_profile_id UUID NOT NULL FK owner_profiles ON DELETE RESTRICT
booking_id/payment_id/refund_id/dispute_id UUID references as applicable
effect DEBIT|CREDIT NOT NULL
amount_rupiah BIGINT NOT NULL CHECK > 0
reason VARCHAR NOT NULL
journal_id UUID NOT NULL FK platform_journals ON DELETE RESTRICT
allocation_status OPEN|ALLOCATED|SETTLED
created_at TIMESTAMPTZ NOT NULL
```

Payout available = payable credits - open owner debits. Saldo negatif dibawa ke periode berikutnya dan memblokir payout. Offset future entitlement harus membuat journal `Dr OWNER_PAYABLE / Cr OWNER_RECEIVABLE`, bukan mengedit payout lama.

#### `payment_disputes`

Chargeback/dispute mempunyai objek dan state terpisah dari ordinary refund, dengan provider reference, amount, evidence deadline, status, resolution, journal references, dan audit. Jika provider/payment method mempunyai chargeback exposure tetapi dispute recovery belum tersedia, metode tersebut tidak boleh diaktifkan pada pilot.

#### `owner_payouts`

```text
id UUID PK
owner_profile_id UUID FK owner_profiles
payout_account_version_id UUID FK owner_payout_account_versions ON DELETE RESTRICT
period_start/period_end DATE
gross_collected_rupiah BIGINT NOT NULL
refund_amount_rupiah BIGINT NOT NULL
commission_amount_rupiah BIGINT NOT NULL
payout_fee_rupiah BIGINT NOT NULL
net_payout_rupiah BIGINT NOT NULL
status VARCHAR NOT NULL CHECK DRAFT|READY|APPROVED|PROCESSING|PAID|FAILED|CANCELLED
requested_by_user_id UUID NULL
approved_by_user_id UUID NULL
paid_at TIMESTAMPTZ NULL
created_at/updated_at
```

#### `owner_payout_account_versions`

Simpan provider beneficiary/account token, bank code, masked account number, verified account name, KYC/verification status, effective timestamps, dan actor perubahan. Jangan menyimpan credential bank mentah. Perubahan membuat versi baru, memberi notifikasi, dan menjalani cooldown; payout selalu menunjuk versi rekening yang dipakai agar audit historis tidak berubah.

#### `owner_payout_attempts`

Retry payout append-only; jangan menimpa provider ID/error lama:

```text
id UUID PK
owner_payout_id UUID NOT NULL FK owner_payouts ON DELETE RESTRICT
attempt_no INTEGER NOT NULL
provider VARCHAR NOT NULL
payout_account_version_id UUID NOT NULL FK owner_payout_account_versions ON DELETE RESTRICT
idempotency_key VARCHAR NOT NULL UNIQUE
provider_payout_id VARCHAR NULL UNIQUE
status REQUESTED|PROCESSING|SUCCEEDED|FAILED|UNKNOWN
error_code VARCHAR NULL
requested_at/processed_at TIMESTAMPTZ
UNIQUE(owner_payout_id, attempt_no)
```

Destination account version dibekukan saat payout APPROVED. Failed/unknown attempt tetap tersimpan; `UNKNOWN` harus di-inquiry dan tidak boleh langsung diretry. Setelah dipastikan tidak sukses, aggregate dapat `FAILED → APPROVED(review) → PROCESSING` dengan attempt baru; `PAID` terminal.

#### `owner_payout_items`

```text
payout_id UUID FK owner_payouts ON DELETE RESTRICT
payable_id UUID UNIQUE FK owner_payables ON DELETE RESTRICT
booking_id UUID FK bookings ON DELETE RESTRICT
gross_amount_rupiah BIGINT
refund_amount_rupiah BIGINT
commission_amount_rupiah BIGINT
net_amount_rupiah BIGINT
```

Gunakan row lock dan `FOR UPDATE SKIP LOCKED` saat mengalokasikan payable ke batch.

Payout equations/invariants:

- `owner_payable.amount_rupiah = booking_fee_snapshots.owner_net_amount_rupiah` untuk LIVE captured booking.
- `payout_item.net_amount_rupiah = payable.amount_rupiah` pada full-refund-only flow.
- Gross/refund/commission copies pada item wajib cocok dengan immutable booking snapshot/events.
- `owner_payout.net_payout_rupiah = SUM(owner_payout_items.net_amount_rupiah)`.
- Platform-borne payout fee berada di luar owner net.
- Non-booking adjustment/receivable tidak diselundupkan sebagai booking item; gunakan explicit subledger allocation.
- Outstanding `OWNER_PAYABLE` account pada platform ledger harus sama dengan operational outstanding payables/subledger pada reconciliation.

#### `provider_settlements` dan items

Provider settlement adalah perpindahan PSP → rekening/subaccount sesuai provider fund-flow. Owner payout adalah provider/LapangGo → owner. Keduanya wajib menjadi entitas terpisah:

```text
provider_settlements:
  id, provider, external_settlement_id UNIQUE, currency,
  gross_captured_rupiah, refunds_rupiah, provider_fees_rupiah,
  net_settled_rupiah, settled_at, status, created_at

provider_settlement_items:
  settlement_id, source_type PAYMENT|REFUND|COST,
  source_id, amount_rupiah,
  UNIQUE(source_type, source_id)
```

Conceptual postings (account source disahkan provider ADR):

```text
Provider settlement: Dr BANK_CASH / Cr PSP_CLEARING
Owner payout:        Dr OWNER_PAYABLE / Cr BANK_CASH|PSP_CLEARING|PAYOUT_CLEARING
Payout fee:          Dr PAYOUT_FEE_EXPENSE / Cr provider-specific cash/clearing/AP
```

Jangan hard-code credit account fee sebelum kontrak provider memastikan kapan/cara fee dipotong.

Phase 6 migration menambah `owner_payout_id` dan `provider_settlement_id` FKs `ON DELETE RESTRICT` pada relevant journals, dengan source-specific checks/uniqueness.

### Settlement Rules

- Weekly batch memakai cutoff Senin 00:00:00 `Asia/Jakarta`; eligibility period half-open `[cutoff_sebelumnya, cutoff_saat_ini)`.
- Minimum payout awal Rp100.000; saldo lebih kecil dibawa ke minggu berikutnya.
- Booking eligible hanya jika payment gateway captured dan settled, booking `COMPLETED`, safety hold minimal 24 jam sudah lewat, dan tidak ada refund/dispute aktif.
- Satu payable/booking tidak boleh dibayar dua kali.
- Booking dengan unresolved refund/dispute ditahan.
- Full refund sebelum payout mengeluarkan booking dari payable atau menghasilkan net reversal yang eksplisit.
- Refund setelah payout menjadi negative balance/offset settlement berikutnya; jangan silently mengubah settlement lama.
- Provider payout fee dicatat sebagai platform expense dan tidak mengurangi owner payable pada model awal 7%.
- `net_payout_rupiah` adalah nilai yang diterima/dikirim kepada owner dan harus sama dengan sum payout items; `payout_fee_rupiah` berada di luar nilai tersebut.
- Payout hanya ke rekening owner yang sudah diverifikasi provider/KYC.
- Payout PAID dan provider settlement matched bersifat immutable.
- Failed payout dapat diretry dengan idempotency/reference yang tepat.
- Failed payout tidak melepaskan payable ke batch baru; item tetap allocated dan retry dicatat append-only pada payout aggregate yang sama agar unique payable tidak dilanggar.
- Tiga payout pertama setiap owner melewati manual review.
- Owner suspended tidak kehilangan hak; payable ditahan dan diselesaikan sesuai kontrak.
- Perubahan rekening owner memerlukan re-verification, notifikasi, dan cooldown minimal 48 jam.
- Gunakan kill switch payout terpisah dari payment/refund.

### Owner Finance Semantics sebelum Platform-collected Production Capture

Existing owner dashboard/ledger saat ini menampilkan full booking income. Sebelum platform-collected payment live, kontrak owner-facing wajib diperjelas dan diuji:

- booking gross tetap ditampilkan sebagai pendapatan booking owner;
- platform commission ditampilkan sebagai potongan/expense terpisah;
- hak owner net = gross - commission - refund + exact commission reversal;
- gateway fee tidak dibebankan ke owner pada model awal;
- payout adalah pelunasan payable/cash movement, bukan pendapatan owner kedua dan bukan expense LapangGo;
- full refund membalik gross owner dan exact commission fee sehingga net booking kembali nol;
- halaman settlement/payout terpisah dari owner P&L/cashbook;
- owner dapat drill down dari payout ke setiap booking dan fee snapshot.

Keputusan implementasi: `owner_finance_transactions` tetap menjadi cashbook direct-to-owner/manual. Gateway capture **tidak** memasukkan full amount sebagai owner cash. Owner finance/reporting ditambah source marketplace terpisah yang diturunkan dari immutable payment/snapshot/platform journal/payable facts, lalu union mutually exclusive dengan cashbook legacy. UI membedakan gross marketplace earnings, commission, net owner share, payable, dan payout. Jangan mengaktifkan production capture sebelum query/UI ini selesai dan terbukti tidak double count.

Rekonsiliasi harus membandingkan provider collected balance, platform ledger, owner marketplace report, dan settlement totals.

### Maker-checker

Payout production tidak boleh bergantung pada satu klik satu admin tanpa kontrol tambahan.

Minimum sebelum automatic payout:

- actor pembuat batch dan approver berbeda; atau
- provider dashboard mempunyai approval control yang independen;
- audit log menyimpan kedua actor dan external reference.

Jika role model admin belum mendukung maker-checker, pilot hanya boleh menggunakan payout manual di provider dashboard dan sistem mencatat reference-nya. Jangan mengklaim payout otomatis selesai.

### Exit Gate

- settlement calculation tests mencakup paid, refund-before-payout, refund-after-payout, failed payout, retry, dan adjustment;
- duplicate payout tidak mungkin melalui constraint + idempotency;
- rekonsiliasi harian menghasilkan selisih nol;
- owner dapat melihat breakdown payout miliknya;
- admin dapat melihat payable/outstanding/paid tanpa mencampur revenue;
- legal/operations menyetujui SOP payout.

---

## 16. Phase 7 — Controlled Monetization Go-live

### Owner Commercial Acceptance Evidence

Sebelum owner LIVE, simpan acceptance immutable:

```text
owner_commercial_acceptances
id UUID PK
owner_profile_id UUID NOT NULL FK owner_profiles ON DELETE RESTRICT
commercial_schedule_version VARCHAR NOT NULL
terms_of_service_version VARCHAR NOT NULL
refund_policy_version VARCHAR NOT NULL
payout_policy_version VARCHAR NOT NULL
accepted_by_user_id UUID NOT NULL FK users ON DELETE RESTRICT
accepted_at TIMESTAMPTZ NOT NULL
ip_address TEXT NULL
user_agent TEXT NULL
evidence_hash/reference VARCHAR NOT NULL
UNIQUE(owner_profile_id, commercial_schedule_version)
```

LIVE harus ditolak jika acceptance yang cocok, KYC, dan verified payout destination belum valid. Legal/accounting/privacy signoff juga harus memiliki approver dan tanggal pada readiness artifact.

### Launch Partner Cohort State

Sebelum rollout, tambahkan state yang concurrency-safe untuk jadwal 0% → 5% → 7%:

```text
owner_commercial_cohorts
id UUID PK
owner_profile_id UUID UNIQUE FK owner_profiles ON DELETE RESTRICT
cohort_type LAUNCH_PARTNER|STANDARD|CUSTOM
first_gateway_capture_at TIMESTAMPTZ NULL
trial_ends_at TIMESTAMPTZ NULL
introductory_ends_at TIMESTAMPTZ NULL
subsidy_budget_rupiah BIGINT NULL
status ENROLLED|ACTIVE|PAUSED|COMPLETED
created_at/updated_at
```

Saat owner di-enroll sebagai launch partner, buat term 0% effective segera agar booking pertama sudah snapshot 0%; end date masih terbuka. Verified gateway capture pertama kemudian melakukan compare-and-set/row lock: hanya satu request yang mengisi `first_gateway_capture_at`, menutup term 0% pada hari ke-90, dan menjadwalkan term 5% serta 7% ke depan. Cohort tidak boleh mengubah booking snapshot existing, termasuk booking yang dibuat sebelum first capture.

`subsidy_spent` tidak disimpan sebagai mutable counter; derive/reconcile dari provider cost items dan ledger entries yang diberi cohort attribution. Pada cohort 0%, fee menciptakan funding shortfall karena owner payable = captured amount. Top-up platform ke provider/payout clearing dicatat sebagai asset transfer (`Dr PSP/PAYOUT_CLEARING`, `Cr BANK_CASH`), bukan expense kedua; provider fee tetap satu-satunya expense. Commission 0 tidak membuat zero-amount ledger entries—gunakan idempotent event marker/domain fact.

### Rollout

1. Internal sandbox.
2. Production-data shadow/replay read-only tanpa payment order atau real customer funds.
3. Pilot maksimal 3–5 owner yang menyetujui term 0%.
4. Jalankan tiga payout pertama setiap owner dengan manual review.
5. Tarif owner yang sudah dijanjikan mengikuti kalender half-open kontraktual: `[first_capture, +90 hari)` 0%, `[+90, +180 hari)` 5%, lalu 7%. KPI tidak boleh mempercepat/menunda jadwal ini secara sepihak.
6. Gate minimal 100 captured payment **dan** 30 hari, ditambah rekonsiliasi Rp0 selama 14 hari berturut-turut, digunakan untuk memperluas cohort atau mengaktifkan otomatisasi payout—bukan mengubah rate owner existing.
7. Evaluasi conversion, complaint, refund, dan unit economics per cohort 0/5/7.
8. Perluas owner secara batch hanya bila expansion gate lulus; jangan global switch dan jangan mengubah histori booking.

### Go-live Checklist

- `PLATFORM_MONETIZATION_ENABLED=true` hanya di environment pilot.
- Kill switch terpisah tersedia untuk payment creation, refund dispatch, dan payout.
- Hanya commercial term owner pilot yang `LIVE`.
- Customer checkout dan owner agreement menampilkan fee secara transparan.
- Payment + refund + ledger + settlement + payout E2E lulus.
- Monitoring webhook failure, reconciliation difference, negative owner balance, payout failure tersedia.
- Kill switch diuji.
- On-call owner dan rollback procedure jelas.
- Backup dan restore drill selesai.
- Tidak ada P0/P1 bug terbuka.
- SUPER_ADMIN finance memakai MFA sebelum mengoperasikan money movement production.
- Legal/accounting review menyetujui kontrak marketplace, KYC, pajak/invoice komisi, refund, chargeback, payout, dan perlindungan data; jangan hard-code aturan pajak dari asumsi teknis.
- Unit-economics matrix menghitung break-even per payment method dan minimum final booking price. Method/basket dengan expected processing+payout cost yang membuat contribution cohort 7% negatif dinonaktifkan kecuali ada explicit subsidy approval.

### KPI Go/No-go

Monitor per minggu:

- paid booking count dan GMV;
- payment success rate;
- checkout conversion;
- refund/cancellation rate;
- webhook processing success;
- reconciliation difference;
- payout success dan payout aging;
- effective take rate;
- contribution margin per booking;
- owner complaint/churn;
- customer repeat booking.

Threshold awal untuk pilot (kalibrasi ulang berdasarkan data nyata):

- reconciliation match 100% dan unexplained mismatch Rp0;
- duplicate/unauthorized charge, refund, journal, atau payout = 0;
- payout accuracy 100% dan payout success target minimal 99%;
- payment success di bawah 80% selama dua hari → soft pause/investigasi;
- webhook backlog di atas 15 menit → soft pause;
- refund rate di atas 8%, owner cancellation di atas 5%, atau chargeback di atas 0,5% → review cohort;
- gateway + payout cost melebihi 35% dari komisi 7% → alert unit economics;
- cost tersebut melebihi 50% selama dua minggu → pricing/payment-method review;
- subsidy promo mencapai 80% budget → hentikan onboarding launch partner baru, bukan memotong periode owner existing.
- cohort 0% dipantau dengan budget absolut, cost/GMV, dan cost/booking (bukan cost/commission yang denominator-nya nol); cohort 5% dan 7% dipantau terpisah.

No-go/rollback jika:

- selisih rekonsiliasi tidak dapat dijelaskan;
- duplicate charge, duplicate ledger, atau duplicate payout;
- webhook signature/authorization incident;
- owner net payout salah;
- refund tidak menghasilkan reversal yang tepat;
- payment success/checkout conversion melewati threshold numerik yang disetujui pada readiness artifact sebelum activation;
- complaint owner menunjukkan term tidak dipahami/disetujui.
- booking historical masuk owner payable/payout;
- journal tidak balance walau Rp1;
- refund UI menyatakan sukses sebelum provider confirmation;
- rekening payout berubah tanpa re-verification/cooldown;
- provider contract tidak mendukung marketplace/split settlement yang digunakan;
- kalkulasi money masih melewati `float64`.

Rollback monetisasi berarti mematikan pembuatan payment order baru, membuat future `SIMULATION` term dengan bps kontraktual yang sama (atau menonaktifkan checkout), dan tetap menyelesaikan/refund transaksi in-flight serta kewajiban existing sesuai snapshot. Future 0% hanya boleh melalui kontrak goodwill baru. Jangan mengubah term/snapshot/payment decision lama atau menghapus ledger/settlement historis.

---

## 17. Admin Finance API Contract Lengkap

Kontrak target setelah v1.7:

```http
GET  /admin/finance/summary
GET  /admin/finance/breakdown
GET  /admin/finance/journals
GET  /admin/finance/expenses
POST /admin/finance/expenses
POST /admin/finance/expenses/:id/cancel
POST /admin/finance/expenses/:id/approve
POST /admin/finance/expenses/:id/post
POST /admin/finance/expenses/:id/void
GET  /admin/finance/reconciliation

GET  /admin/commercial-terms
POST /admin/commercial-terms/preview
POST /admin/commercial-terms
```

### Error Semantics

Gunakan response error konsisten dengan project:

- `400`: validation/date/filter/payload invalid;
- `401`: token tidak ada/invalid;
- `403`: role/status tidak diizinkan atau LIVE gate ditolak;
- `404`: referenced owner/venue/expense/journal tidak ada;
- `409`: idempotency conflict, overlapping term, already reversed;
- `500`: generic message ke client, detail aman hanya di server log.

Jangan mengembalikan raw SQL/provider error atau stack trace.

Body error minimum dan stabil:

```json
{
  "message": "Invalid date range",
  "code": "INVALID_DATE_RANGE",
  "field_errors": {
    "end_date": "must be on or after start_date"
  }
}
```

Gunakan 400 untuk request validation dan 409 untuk business/state/idempotency conflict. `code` machine-readable dipakai frontend; `message` tetap aman untuk user.

### Pagination

- Default page 1, limit 20.
- Maksimum limit 100.
- Response mengikuti pola `data`, `total_items`, `total_pages`, `page`, `limit` admin existing.

### Money Contract

- Request/response nominal finance baru memakai integer-rupiah string (`"250000"`) dan dipetakan checked ke Go `int64`; bps/count tetap JSON number.
- Currency selalu eksplisit.
- Currency exponent IDR adalah 0.
- Frontend tidak mengirim desimal, tanda negatif, separator, atau scientific notation untuk positive amount input.
- Service menolak fractional amount, overflow, dan amount di luar limit bisnis.
- TypeScript menggunakan alias `MoneyRupiah = string`; format exact memakai `BigInt`/helper aman. Chart hanya mengonversi nilai yang lolos safe-range check atau memakai normalized value tanpa mengubah angka tabel exact.
- Commission calculation dibulatkan sekali saat snapshot, lalu nominal snapshot digunakan kembali.

---

## 18. Frontend Information Architecture Target

```text
Admin
├── Dashboard
├── Keuangan Platform
│   ├── Ringkasan
│   ├── Pengeluaran Platform
│   ├── Journal
│   └── Rekonsiliasi
├── Commercial Terms       # dapat berupa tab Keuangan pada v1.7
├── Users
├── Owners
├── Venues
└── Audit Logs
```

Commercial term UI pada v1.7 boleh dimulai read-only setelah API create teruji. Jika mutation UI ditambahkan:

- tampilkan current, future, dan historical term;
- preview rupiah sebelum submit;
- confirm modal menjelaskan term tidak mengubah booking lama;
- LIVE option disabled/absent;
- tidak ada edit row lama;
- actor membuat versi baru dengan effective date.

Owner-facing disclosure sebelum pilot live:

- tarif saat ini;
- tanggal mulai/berakhir;
- booking yang dikenakan;
- contoh perhitungan;
- payout schedule;
- refund treatment;
- histori term.

Customer-facing disclosure sebelum pilot live:

- total harga final sebelum bayar;
- biaya tambahan jika kelak ada service fee;
- refund policy;
- jangan menyembunyikan fee di langkah terakhir.

---

## 19. Security, Privacy, dan Audit Checklist

### Authorization

- Semua admin finance endpoint: authenticated + active + `SUPER_ADMIN`.
- Server-side enforcement wajib; hidden menu tidak cukup.
- Owner hanya dapat melihat term dan settlement miliknya ketika endpoint owner ditambahkan.
- Jangan menerima owner/venue access berdasarkan frontend claims.

### Sensitive Data

- Tidak menyimpan PAN, CVV, payment secret, webhook secret, atau bank credential mentah.
- Provider secrets hanya dari environment/secret manager.
- Log menggunakan provider reference yang aman, bukan payload penuh.
- Audit metadata harus allowlist, bukan dump request.
- Deskripsi expense harus dirender escaped; jangan gunakan raw HTML.
- Webhook wajib memverifikasi signature, timestamp tolerance, dan replay prevention sebelum parsing business event.
- Provider/admin endpoints sensitif membutuhkan rate limiting dan correlation ID.
- MFA untuk admin finance menjadi hard gate sebelum money movement production.
- Perubahan rekening, payout, dan adjustment material membutuhkan maker-checker atau independent provider approval.

### Audit Minimum

Catat:

- commercial term created/superseded;
- rejected LIVE attempt;
- platform expense created;
- platform journal/expense posted atau reversed;
- reconciliation run/result summary;
- settlement created/approved/paid/failed;
- payout retry;
- monetization kill-switch-related operational change jika dapat diamati aplikasi.
- payment/refund/payout kill switch activation dan rejected command.

Audit log tidak boleh dapat diedit oleh admin biasa.

### Concurrency dan Idempotency

- Lock active commercial term saat version switch.
- Unique event key untuk setiap finance event.
- Unique provider event ID.
- Unique provider payment ID.
- Payout item/payable constraint mencegah booking dibayar dua kali.
- Gunakan DB transaction untuk payment recognition, owner ledger, platform ledger, dan state change yang harus atomic.

---

## 20. Test Matrix Lintas Modul

| Area | Skenario wajib |
|---|---|
| Pricing | 0%, 5%, 7%, custom, boundary date, rounding |
| Channel | Online dikenakan, offline exempt, manual owner finance excluded |
| Promo | Basis menggunakan final price setelah diskon |
| Payment | Pending, captured, duplicate webhook, out-of-order, amount mismatch, timeout |
| Refund | Full before payout, full after payout, duplicate approval/retry |
| Ledger | Balanced journal, idempotent post, immutable entries, exact reversal, double reversal |
| Settlement | Weekly payout, held dispute, failed payout, retry, negative carry-forward |
| Concurrency | Two first captures, capture vs expiry, refund vs completion, refund vs payout allocation |
| Audit | Atomic platform audit, actor/correlation, no sensitive metadata, no audit on rolled-back mutation |
| Migration | Fresh DB, upgrade current DB, dry-run backfill, idempotent apply, safe down before cutover |
| Filters | Date, owner, venue, timezone, empty period, max range |
| Auth | No token, wrong role, suspended, active superadmin |
| UI | Loading, empty, error, stale, mobile, retry, double submit |
| Regression | Owner finance, owner dashboard, booking creation, offline booking, promo, refund, audit |

### Calculation Fixtures

Gunakan fixture eksplisit dan assert exact amount:

| Booking | Channel | Final price | Rate | Commission | Refund | Net projected commission |
|---|---|---:|---:|---:|---:|---:|
| A | Online | 200.000 | 7% | 14.000 | 0 | 14.000 |
| B | Online | 200.000 | 5% | 10.000 | 0 | 10.000 |
| C | Online | 200.000 | 0% | 0 | 0 | 0 |
| D | Offline | 200.000 | forced 0% | 0 | 0 | 0 |
| E | Online promo | 175.000 | 7% | 12.250 | 0 | 12.250 |
| F | Online refunded | 200.000 | 7% | 14.000 | 200.000 | 0 |

Tambahkan fixture rounding nominal kecil dan nominal maksimum yang diizinkan.

---

## 21. Observability dan Reconciliation Runbook

Metrics/log counters minimum:

- finance summary request error/latency;
- snapshot create failure;
- commercial term resolution fallback;
- duplicate event prevented;
- webhook received/verified/failed/retried;
- payment amount mismatch;
- reconciliation difference count/amount;
- settlement pending age;
- payout failed/retried;
- LIVE write rejected by kill switch.

Alert P0/P1:

- duplicate charge/payout;
- webhook signature verification bypass/failure spike;
- reconciliation difference bukan nol;
- owner payout mismatch;
- finance entry without expected booking/payment reference;
- snapshot missing untuk booking baru.

Runbook incident minimum:

1. Aktifkan kill switch monetisasi.
2. Jangan delete/mutate ledger.
3. Hentikan payout batch yang belum diproses.
4. Ambil snapshot evidence/reference aman.
5. Rekonsiliasi provider, payment attempts, booking ledger, platform ledger, settlement.
6. Buat reversal/adjustment melalui flow resmi jika diperlukan.
7. Dokumentasikan root cause dan test regresi sebelum re-enable.

Reconciliation equations setelah gateway:

```text
PSP opening + captured - refunds - provider fees - provider settlements = PSP closing
Owner payable opening + new entitlements - reversals - payouts = owner payable closing
Commission revenue credits - commission refund/contra - transaction costs = contribution
Setiap platform journal: total debit = total credit
```

Timing difference yang teridentifikasi boleh berstatus pending dengan SLA. Unexplained difference harus Rp0 sebelum payout batch diproses.

---

## 22. Daftar File/Area yang Diperkirakan Terdampak

Backend existing:

```text
apps/api/cmd/api/main.go
apps/api/internal/admin/*
apps/api/internal/audit/*
apps/api/internal/bookings/*
apps/api/internal/refunds/*
apps/api/internal/config/config.go
```

Backend new:

```text
apps/api/internal/platformfinance/*
apps/api/cmd/backfill-platform-finance/main.go
apps/api/cmd/reconcile-platform-finance/main.go
```

Frontend:

```text
apps/web/src/App.tsx
apps/web/src/components/admin/AdminLayout.tsx
apps/web/src/components/admin/finance/*
apps/web/src/pages/admin/AdminFinancePage.tsx
apps/web/src/types/adminFinance.ts
apps/web/src/lib/api/adminFinance.ts
```

Database:

```text
db/migrations/<next>_platform_audit_and_commercial_terms.{up,down}.sql
db/migrations/<next>_booking_fee_snapshots.{up,down}.sql
db/migrations/<next>_platform_double_entry_ledger.{up,down}.sql
db/migrations/<next>_platform_expenses.{up,down}.sql
```

Payment/settlement migrations baru dibuat hanya pada Phase 5/6.

Documentation:

```text
docs/version_1_7_platform_finance_implementation_plan.md
docs/version_1_7_platform_finance_readiness.md       # dibuat saat implementasi selesai
docs/manual_qa_platform_finance_v1_7.md              # dibuat saat QA/release, bukan sekarang
docs/platform_finance_metric_dictionary.md           # optional jika dictionary di dokumen ini terlalu besar
docs/platform_finance_incident_runbook.md             # wajib sebelum LIVE
```

Antigravity harus mempersempit target files per phase dan tidak menyentuh seluruh daftar sekaligus.

---

## 23. Non-scope yang Tidak Boleh Terselip

- Subscription Pro.
- Promoted listing/ads.
- Customer membership.
- Partial refund.
- Multi-currency.
- Tax invoice otomatis.
- Formal general ledger/accounting report.
- Kartu/wallet internal.
- Menyimpan kartu atau credential bank.
- Escrow/custody buatan sendiri.
- Automatic payout sebelum maker-checker dan reconciliation.
- Refactor seluruh owner finance dari `float64` dalam phase yang sama.
- Perubahan besar desain owner dashboard.
- Penghapusan legacy manual payment sebelum gateway pilot stabil.

Setiap item tersebut membutuhkan plan terpisah.

---

## 24. Anti-pattern yang Harus Ditolak Saat Review

- `commission = totalPrice * 0.07` di React.
- Kalkulasi atau penyimpanan money baru menggunakan `float64`.
- Menyebut total booking sebagai pendapatan LapangGo.
- Menjumlahkan manual owner income/expense ke laporan platform.
- Menganggap `CONFIRMED` selalu paid.
- Mengubah snapshot ketika rate berubah.
- Memberi historical booking billable rate/payable/payout secara retroaktif.
- Edit/delete financial transaction.
- Menandai paid dari browser redirect.
- Webhook tanpa signature verification atau idempotency.
- Menggunakan random event key pada setiap retry.
- Menyalakan LIVE dari tombol UI tanpa environment gate.
- Payout dari data agregat tanpa owner payable dan payout items.
- Menganggap payout sebagai platform expense atau owner revenue kedua.
- Logging full webhook payload/secret/bank data.
- Migration schema sekaligus destructive mass backfill.
- Menambahkan payment gateway, payout, dan admin analytics dalam satu giant PR.

---

## 25. Prompt Antigravity per Tahap

Gunakan prompt di bawah satu per satu. Jangan menggabungkan beberapa prompt menjadi satu pekerjaan besar.

> Catatan status: prompt Phase 0–2A dipertahankan sebagai histori/source contract. Untuk eksekusi aktif setelah Phase 2A, gunakan breakdown task-level pada `docs/LapangGo_Phase_2B_to_Phase_7_Antigravity_Task_Cards.md`; prompt subphase di section ini tidak boleh diberikan sebagai satu giant task.

Di Antigravity gunakan `/planning` (bukan `/plan`) untuk breakdown phase, lalu `/build` hanya setelah phase plan disetujui. Gunakan `/test` dan `/review` sebelum meminta GO phase berikutnya.

### Prompt 0 — Baseline dan Audit

```text
Ikuti AGENTS.md, .agents/agent_workflow.md, dan .agents/definition_of_done.md.
Baca seluruh docs/version_1_7_platform_finance_implementation_plan.md.
Kerjakan hanya Phase 0: baseline, decision freeze, dan data audit read-only.
Jangan edit aplikasi, jangan buat migration, jangan backfill, dan jangan mutasi database.
Laporkan baseline test/build, anomali data, query yang digunakan, serta keputusan GO/NO-GO untuk Phase 1A.
```

### Prompt 1A — Backend Simulation API

```text
Ikuti AGENTS.md dan workflow lokal. Baca Phase 1A serta metric dictionary pada
docs/version_1_7_platform_finance_implementation_plan.md.
Kerjakan hanya backend read-only Admin Finance API dalam mode SIMULATION.
Sebelum coding, tulis objective, target files, acceptance criteria, dan test plan singkat.
Jangan buat migration, mutation endpoint, ledger platform, payment gateway, atau LIVE mode.
Pastikan online/offline dan GMV/platform revenue tidak tercampur.
Jalankan targeted tests lalu go test ./... dan berhenti untuk review.
```

### Prompt 1B — Frontend Simulation UI

```text
Ikuti AGENTS.md dan workflow lokal. Pastikan Phase 1A sudah direview dan API contract stabil.
Kerjakan hanya Phase 1B Admin Finance UI mode SIMULATION.
Banner simulasi harus permanen dan proyeksi tidak boleh disebut pendapatan aktual.
Implement loading, empty, error, stale, retry, mobile, filter, dan role guard.
Jangan membuat mutation UI atau tombol LIVE.
Jalankan npm.cmd run lint dan npm.cmd run build. Lampirkan bukti manual QA route guard,
loading, empty, full error, stale refresh, rapid-filter race, owner→venue filter,
mobile, serta equality UI/API. Berhenti dan minta GO sebelum Phase 2A.
```

### Prompt 2A — Platform Audit dan Commercial Terms

```text
Ikuti AGENTS.md dan workflow lokal. Baca Phase 2A secara lengkap.
Kerjakan hanya durable ownerless platform audit + versioned commercial terms dan migration up/down.
Cek migration number terbaru sebelum membuat file.
Default harus 700 bps, SIMULATION, collection NONE. LIVE wajib ditolak.
Tidak boleh ada PATCH/DELETE historical term dan overlapping period harus ditolak secara concurrency-safe.
Gunakan half-open range DB exclusion + scope lock; hanya valid_until NULL→future supersession boleh berubah.
Financial audit tidak boleh fire-and-forget. Tambahkan atomic audit dan focused tests.
Uji migration up/down, go test ./..., lalu berhenti.
```

### Prompt 2B1 — Snapshot Schema, Resolver, Calculator

```text
Ikuti AGENTS.md dan workflow lokal. Pastikan Phase 2A sudah lulus review.
Kerjakan hanya Phase 2B1: migration up/down booking fee snapshot, term resolver,
dan integer-rupiah commission calculator beserta tests rounding/overflow/immutability.
Belum boleh mengubah create-booking paths atau melakukan backfill.
Historical policy tetap non-billable dan LIVE tetap disabled.
Uji migration + targeted/full Go tests, lalu berhenti dan minta GO untuk 2B2.
```

### Prompt 2B2 — Transactional Snapshot Writes

```text
Ikuti AGENTS.md. Kerjakan hanya Phase 2B2 setelah 2B1 GO.
Integrasikan snapshot atomic ke setiap create-booking path: online, promo, dan owner walk-in.
Online memakai resolved term; walk-in selalu 0%; promo memakai final price.
Jangan membuat backfill, charge, ledger actual, gateway, atau LIVE mode.
Uji retry, concurrency, rollback, semua write paths, dan regression booking; berhenti untuk GO 2B3.
```

### Prompt 2B3 — Safe Legacy Backfill

```text
Ikuti AGENTS.md. Kerjakan hanya Phase 2B3 setelah 2B2 GO.
Buat command dry-run/apply batch yang idempotent. Historical booking harus
LEGACY_NO_COMMISSION 0%; scenario 7% hanya analytics non-billable.
Wajibkan --cutover-at cocok immutable record; post-cutover missing snapshot adalah P0.
Jangan mass-backfill di migration. Jalankan dry-run pada disposable/staging database,
reconcile count dan fractional-price anomalies, lalu berhenti sebelum production apply.
```

### Prompt 2C — Read-only Terms dan Platform Audit UI

```text
Ikuti AGENTS.md. Kerjakan hanya Phase 2C setelah 2B gates lulus.
Tambahkan read-only commercial terms UI dan scope OWNER|PLATFORM|ALL pada Admin Audit API/UI.
Tidak ada commercial-term mutation UI dan tidak ada LIVE option.
Uji pagination/filter/auth/loading/empty/error, backend tests, frontend lint/build, lalu berhenti.
```

### Prompt 3A — Platform Ledger

```text
Ikuti AGENTS.md dan workflow lokal. Baca Phase 3A.
Kerjakan hanya minimal double-entry platform journal/ledger foundation dengan migration up/down,
fixed accounts, debit=credit invariant, event-key idempotency, exact reversal, atomic platform audit, dan tests.
Jangan membuat domain-specific payment/commission/refund/payout posting methods, provider, atau actual journal.
Uji concurrency/idempotency/reversal dan go test ./..., lalu berhenti.
```

### Prompt 3B1 — Platform OPEX Backend

```text
Ikuti AGENTS.md. Kerjakan hanya Phase 3B1.
Tambah platform_expenses migration, DRAFT/CANCEL/APPROVE/POST/VOID backend workflow,
Idempotency-Key semantics, balanced/reversal journals, atomic audit, OPEX report-time rules,
dan summary data-availability integration. Tidak ada frontend dalam task ini.
Uji state matrix, retry/timeout-after-commit, journal balance, backdated/void lintas bulan,
auth, summary/trend, migration up/down; jalankan go test ./... lalu berhenti.
```

### Prompt 3B2 — Platform OPEX Frontend

```text
Ikuti AGENTS.md. Kerjakan hanya Phase 3B2 setelah 3B1 GO.
Implement expense list + DRAFT/CANCEL/APPROVE/POST/VOID UI, confirm/reason states,
stable Idempotency-Key per user action, unavailable→AVAILABLE summary card,
loading/empty/error/stale/mobile/double-click behavior. Tidak ada edit/delete POSTED data.
Jalankan lint/build dan manual QA equality API/UI + rapid retry, lalu berhenti.
```

### Prompt 4 — Reconciliation dan Release Readiness

```text
Ikuti AGENTS.md dan definition of done. Kerjakan hanya Phase 4.
Jangan menambah fitur bisnis baru. Bangun reconciliation read-only, lengkapi regression tests,
jalankan full backend/frontend/smoke verification, dan buat readiness + manual QA document.
Pastikan mode tetap SIMULATION dan kill switch false.
Laporkan setiap mismatch; jangan memperbaiki data massal tanpa approval.
```

### Prompt 5A — Provider/Fund-flow Plan Only

```text
Ikuti AGENTS.md. Jangan coding. Baca Phase 5 dan buat provider-specific ADR/sub-plan:
merchant/seller of record, custody, split/settlement, refund/chargeback, fee timing/accounts,
KYC, webhook signing, state machine, schema delta, test matrix, legal/security signoff.
Gunakan sandbox docs/contract. Status NO-GO tanpa named/date signoff. Berhenti untuk approval manusia.
```

### Prompt 5B — Payment Facts dan Sandbox Adapter

```text
Kerjakan hanya setelah 5A disetujui. Implement payment attempts/canonical immutable capture facts,
provider adapter create/inquiry di sandbox, monetization decision snapshot, strict money validation,
dan tests. Belum ada webhook/outbox/refund/actual journal/production funds. Berhenti untuk review.
```

### Prompt 5C — Webhook Inbox dan Outbox

```text
Kerjakan hanya Phase 5C: signature/timestamp/replay verification, redacted append-only webhook inbox,
transactional outbox, idempotent/out-of-order state transitions, late-capture exception, observability,
dan concurrency tests. Browser callback tidak mark paid. Runtime tetap sandbox/shadow. Berhenti.
```

### Prompt 5D — Refund Facts Shadow Flow

```text
Kerjakan hanya Phase 5D: normalized asynchronous full-refund facts untuk kedua legacy approval paths,
PROCESSING sampai provider sandbox SUCCEEDED, cost items, holds/escalation, retry/idempotency,
dan tests. Jangan membuat production actual journal atau payout. Berhenti untuk review.
```

### Prompt 5E — Shadow Reconciliation dan Isolated Journal Templates

```text
Kerjakan hanya Phase 5E: sandbox/provider reconciliation, metric-source union checks,
fail-closed runtime shadow, dan capture/fee/completion/refund posting-template tests pada isolated test ledger.
Tidak ada real customer money atau production actual revenue. Jalankan security/E2E review dan berhenti.
```

### Prompt 6A — Payable/Payout Plan Only

```text
Ikuti AGENTS.md. Jangan coding. Buat Phase 6 provider-specific sub-plan untuk payables,
owner receivables/chargebacks, provider settlements, payout attempts, account-version/KYC,
maker-checker, weekly half-open cutoff, exact journals, reconciliation, UI, dan rollback.
Berhenti untuk finance/security/legal approval manusia.
```

### Prompt 6B — Payable/Settlement Primitives

```text
Setelah 6A GO, kerjakan hanya schema/calculator/state machine owner payables,
balance adjustments/receivables, provider settlements, allocation constraints, dan tests.
Tidak ada provider payout call, UI, atau production LIVE. Berhenti untuk review.
```

### Prompt 6C — Payout Sandbox Execution

```text
Kerjakan hanya payout aggregate/items/append-only attempts, destination version freeze,
maker-checker, provider sandbox idempotency/inquiry/retry, journals, kill switch, dan race tests.
Tetap sandbox/manual approval; jangan mengaktifkan production capture. Berhenti.
```

### Prompt 6D — Owner/Admin UI dan Reconciliation

```text
Kerjakan hanya owner payout breakdown + admin payable/payout/reconciliation UI,
loading/error/exception states, authorization isolation, audit, E2E sandbox reconciliation,
dan readiness evidence. Jangan mengaktifkan LIVE. Berhenti untuk approval manusia.
```

### Prompt 7A — Pilot Readiness/Runbook Only

```text
Phase 7 adalah operasi berisiko, bukan autonomous feature task. Jangan mengubah production state.
Audit owner acceptances, KYC, account verification, provider/legal signoff, MFA/maker-checker,
kill switches, backup/restore, 0/5/7 calendar terms, unit economics, monitoring, rollback,
dan hard-stop criteria. Hasilkan GO/NO-GO evidence untuk persetujuan eksplisit manusia.
```

### Prompt 7B — Monitoring setelah Human Activation

```text
Hanya setelah manusia mengaktifkan pilot secara eksplisit, monitor cohort dan laporkan evidence.
Antigravity tidak boleh secara otonom mengaktifkan owner, mengubah term, mengirim payout,
atau menjalankan rollout 30 hari. Rate mengikuti kalender kontraktual; gate minimal 100 captured
payments DAN 30 hari + 14 hari mismatch Rp0 + tiga payout manual/owner hanya mengontrol
perluasan cohort/otomasi. Pada hard stop, jalankan runbook/kill switch hanya sesuai authority yang diberikan.
```

---

## 26. Definition of Done Keseluruhan

Fondasi admin finance dianggap selesai hanya jika:

- business metric dictionary dipatuhi API dan UI;
- GMV, projected revenue, actual revenue, owner money, dan platform expense terpisah;
- online/offline classification benar;
- historical booking selalu non-billable dan tidak masuk payable/payout;
- seluruh money domain baru memakai integer rupiah/int64, bukan float;
- 0/5/7 terms versioned dan auditable;
- setiap booking baru mempunyai immutable fee snapshot;
- platform journal/ledger double-entry terpisah, balance, immutable, idempotent, dan reversible;
- admin OPEX tidak dapat diedit/delete;
- auth server-side dan audit lengkap;
- backend full tests, frontend lint/build, migration up/down, dan manual QA lulus;
- reconciliation tidak memiliki unexplained difference;
- owner/customer flows lama tidak regression;
- v1.7 tetap simulation-only;
- payment/payout LIVE hanya terjadi setelah Phase 5–7 gates.

Jika salah satu poin finance integrity, authorization, idempotency, refund reversal, atau reconciliation belum terbukti, statusnya **belum done** walaupun UI sudah terlihat selesai.

---

## 27. Final Implementation Order

Urutan yang tidak boleh dilompati:

```text
Phase 0  Baseline & data audit
   ↓
Phase 1A Read-only simulation API
   ↓
Phase 1B Simulation admin UI
   ↓
Phase 2A Platform audit + versioned commercial terms
   ↓
Phase 2B1 Snapshot schema/resolver/calculator
   ↓
Phase 2B2 Transactional snapshot writes
   ↓
Phase 2B3 Safe pre-cutover backfill
   ↓
Phase 2C Read-only terms + platform audit UI
   ↓
Phase 3A Minimal double-entry platform ledger
   ↓
Phase 3B1 Platform OPEX backend/reporting
   ↓
Phase 3B2 Platform OPEX UI
   ↓
Phase 4  Reconciliation & v1.7 release gate
   ↓
Phase 5A–5E Provider plan → sandbox facts/webhook/refund/reconciliation
   ↓
Phase 6A–6D Payable/payout plan → sandbox primitives/execution/UI
   ↓
Phase 7A Human-reviewed readiness
   ↓
Phase 7B Human-activated controlled 0% → 5% → 7% calendar rollout
```

Rekomendasi pekerjaan Antigravity berikutnya adalah **Task 2B-00**, bukan langsung membuat migration snapshot atau mengerjakan seluruh Prompt 2B1 sekaligus.
