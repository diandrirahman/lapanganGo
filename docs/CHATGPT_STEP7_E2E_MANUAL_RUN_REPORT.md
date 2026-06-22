# Laporan Eksekusi Step 7: Manual E2E Booking Flow Smoke Test

Proses pengujian fungsionalitas ujung-ke-ujung (End-to-End) sistem pesanan LapangGo secara keseluruhan telah sukses dieksekusi secara aktual menembus *Local Database* dan *Local API Daemon* menggunakan skenario yang dipandu dari dokumen `STEP6_E2E_BOOKING_FLOW_QA.md`.

## 1. Environment & Setup Singkat
- **Database**: `lapangango_postgres` Docker container.
- **Data Prerequisite**: Migrasi eksisting berhasil ditambah skrip `step6_e2e_seed.sql` (*Idempotent Seed*) yang memasukkan entitas Owner, Customer, Profile, Venue, Court, dan Jam Operasional.
- **API Engine**: Dijalankan lokal (`go run ./cmd/api` di Port `8080`).

## 2. Bug Fix Nyata (Source Code Go yang Diubah)
Sesuai prosedur pelolosan QA, satu blokir nyata (*blocker bug*) diidentifikasi di tahap **Step 6 - Create Booking** (awalnya memicu pesan ralat palsu `"booking time is outside court operating hours"`).
- **Penyebab**: Perbedaan cara perlakuan atribut *time* basis data (*PostgreSQL pgx* mengikatkan tahun absolut 2000) dan *golang time.Parse* (mengikat tahun `0000`) menyebabkan validasi komparasi `startParsed.Before(*oh.OpenTime)` senantiasa dinilai kedaluwarsa meski jamnya tepat.
- **Solusi**: Di dalam `apps/api/internal/bookings/service.go`, kami menormalisasi tanggal absolut tersebut menjadi format konstan (`time.Date(0, 1, 1, hour, min, 0, 0, time.UTC)`) sebelum mengkomparasinya. Bug terselesaikan dengan mulus dan `go test ./...` lulus sepenuhnya.

## 3. Eksekusi Endpoint & Actual Result
Seluruh langkah pengujian dibuktikan secara riil di _Localhost API_ dengan hasil aktual yang selaras absolut dengan ekspektasi DTO:

| No | Segmen Tes | Endpoint Dijalankan | Expected Result | Actual Result | Status |
|----|------------|---------------------|-----------------|---------------|--------|
| 1. | **Auth** | `POST /auth/login` | Return JSON Token | Berhasil, Token JWT tersimpan. | **PASS** |
| 2. | **Auth** | `POST /auth/login` | Return JSON Token | Berhasil, Token JWT tersimpan. | **PASS** |
| 3. | **Happy Path** | `GET /courts/:id/availability` | Slot (10-11) is `AVAILABLE` | Slot 10:00 - 11:00 berstatus `"AVAILABLE"` | **PASS** |
| 4. | **Happy Path** | `POST /bookings` | HTTP 201, `PENDING_PAYMENT` | HTTP 201, Status berubah jadi `"PENDING_PAYMENT"` | **PASS** |
| 5. | **Happy Path** | `GET /courts/:id/availability` | Slot (10-11) is `BOOKED` | Slot 10:00 - 11:00 kini terdeteksi `"BOOKED"` | **PASS** |
| 6. | **Happy Path** | `POST /bookings/:id/pay` | HTTP 200, `CONFIRMED` | HTTP 200, Status berubah jadi `"CONFIRMED"` | **PASS** |
| 7. | **Failsafe** | `PATCH /bookings/:id/cancel` | HTTP 409 Conflict | `{"message":"booking cannot be cancelled in current status"}` | **PASS** |
| 8. | **Failsafe** | `POST /bookings/:id/pay` | HTTP 409 Conflict | `{"message":"booking already confirmed"}` | **PASS** |
| 9. | **Owner View** | `GET /owner/venues/:id/bookings` | Memuat data `CONFIRMED` | Booking muncul secara eksplisit dengan status `"CONFIRMED"` | **PASS** |
| 10.| **Cancel** | `POST /bookings` (Slot 2) | HTTP 201, `PENDING_PAYMENT` | HTTP 201, Dibuat di Slot (13-14) dengan status valid | **PASS** |
| 11.| **Cancel** | `PATCH /bookings/:id/cancel`| HTTP 200, `CANCELLED` | HTTP 200, Status berubah menjadi `"CANCELLED"` | **PASS** |
| 12.| **Cancel** | `GET /courts/:id/availability` | Slot 2 is `AVAILABLE` | Slot (13-14) secara dinamis pulih jadi `"AVAILABLE"` | **PASS** |

## 4. Hasil `go test ./...`
Penyesuaian normalisasi validasi pada `service.go` terbukti tidak memutus sirkulasi unit tes mana pun (*Backward Compatible* dan Valid).

Pada **Step 7B**, kami juga telah menginjeksi pengujian regresi (*Regression Tests*) secara spesifik di `apps/api/internal/bookings/service_test.go` untuk menangkal bug ini di masa depan:
1. **`TestCreateBooking_Success_PgxBaseYear2000`**: Mereproduksi secara harafiah kelakuan PostgreSQL/pgx yang menyisipkan *base year 2000* untuk tipe kolom `time` (sedangkan komparator awal memakai *base year 0000*). Tes ini membuktikan pemesanan pada jam 10:00-11:00 akan lolos dari jerat *False Positive* galat operasional `ErrOutsideOpHours`.
2. **`TestCreateBooking_Success_BoundaryOperatingHours`**: Memvalidasi ujung tapal batas pemesanan (kasus jam persis *open time* 08:00 dan jam jelang *close time* 22:00) yang dieksekusi dengan *mock* tahun 2000 agar *failsafe* normalisasi senantiasa tertinjau.

**Kenapa test ini penting?**
Bug validasi waktu operasional berpotensi melumpuhkan arus kas seluruh penyewa jika server atau *driver* basis data berganti. Penulisan *test suite* regresif memastikan siapapun yang merefaktor kode di masa mendatang tidak akan menghapus logika normalisasi dasar (*base date normalization*) tanpa memicu kegagalan uji.

Hasil *go test* termutakhir:
```text
ok      lapangango-api/internal/bookings        2.511s
ok      lapangango-api/internal/auth    (cached)
ok      lapangango-api/internal/availability    (cached)
ok      lapangango-api/internal/blockedslots    (cached)
ok      lapangango-api/internal/courts  (cached)
ok      lapangango-api/internal/schedules       (cached)
ok      lapangango-api/internal/venues  (cached)
```

## 5. Ringkasan Akhir & Risiko Tersisa
- **Status E2E Manual**: Secara konsisten tetap 100% **PASS** mengadopsi pencapaian dari Step 7.
- **Risiko Tersisa**: Kestabilan tingkat lanjut perlu diawasi khususnya menyangkut komparasi *timezone local vs UTC* di atas server produksi Linux kelak, mengingat simulasi lokal sejauh ini menggunakan parameter standar *Asia/Jakarta*.
- **Kesimpulan**: Alur Pemesanan (*Booking Flow*) lapangGo secara definitif beroperasi utuh dan stabil, lengkap dengan mekanisme penangkisan galat berlebih, sinkronisasi *availability*, eksekusi *dummy payment*, hingga perisai uji regresi *bug pgx*. Produk *backend MVP* siap diserahkan menuju tahapan desain *UI/Frontend*!
