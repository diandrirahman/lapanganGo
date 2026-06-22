# AntiGravity Discussion Report: Next Step 6 E2E QA

Sebagai Product-minded Senior Backend Engineer, berikut adalah hasil evaluasi dan rekomendasi saya terkait langkah selanjutnya pasca-penyelesaian MVP Dummy Payment.

## 1. Rekomendasi Next Step Final
**Sangat setuju.** Langkah selanjutnya yang paling strategis dan krusial adalah **Step 6 - End-to-End (E2E) Booking Flow QA / API Smoke Test**. Kita harus menahan diri dari penambahan fitur baru sebelum alur inti (Core Journey) tervalidasi secara utuh.

## 2. Alasan Product/Engineering
- **Product-wise**: MVP kita bersandar pada satu asumsi: *Customer bisa menyewa lapangan dan membayarnya, lalu Owner bisa melihatnya.* Jika ada satu *gap* di alur ini, produk gagal memenuhi janji utamanya.
- **Engineering-wise**: Meskipun *Unit Test* Go kita lolos 100%, uji unit hanya memvalidasi tiap fungsi secara terisolasi dengan _mock database_. Uji _End-to-End_ (Integrasi) membuktikan bahwa database riil PostgreSQL, pembacaan struktur token JWT, dan penggabungan antar-endpoint benar-benar selaras.

## 3. Scope Step 6 yang Disarankan
- Pembuatan dokumentasi **Manual QA Walkthrough** (berupa urutan perintah cURL atau koleksi Postman).
- Pembuatan skrip pemicu data awal (**Seed Script** sederhana menggunakan `seed.go` atau `seed.sql`) khusus untuk menciptakan kondisi prasyarat secara instan tanpa harus memanggil belasan API secara berurutan secara manual setiap kali *test*.
- Eksekusi satu siklus penuh pengujian E2E secara lokal untuk membuktikan poin-poin *Flow* Produk.

## 4. Out of Scope Step 6
- Pembangunan *Automated Integration Test* dalam Go menggunakan `testcontainers` atau semacamnya (Terlalu berat untuk fase iterasi cepat MVP saat ini).
- Pembuatan aplikasi Frontend (Website / Mobile).
- Integrasi Payment Gateway sungguhan (Xendit/Midtrans).

## 5. Data / Test Prerequisite (Prasyarat)
Agar siklus *Smoke Test* bisa dieksekusi, kita membutuhkan pangkalan data dalam *state* berikut:
1. **Customer Account** (termasuk *Bearer JWT Token* valid).
2. **Owner Account** (termasuk *Bearer JWT Token* valid).
3. **Owner Profile** (terikat ke Owner Account).
4. **Venue** (didaftarkan oleh Owner).
5. **Court** (berada di dalam Venue).
6. **Operating Hours** (hari dan jam operasional untuk Court yang bersangkutan).
7. Tanggal dan waktu pengujian di masa depan (tidak tumpang tindih dengan *Blocked Slot*).

## 6. Manual QA Checklist
Alur pengujian manual yang direkomendasikan adalah urutan baku pembuktian E2E:
- [ ] **Data Prep**: *Seed* prasyarat (User, Owner, Venue, Court, Jam Operasional).
- [ ] **Check Availability**: Memastikan slot sasaran berstatus `AVAILABLE`.
- [ ] **Create Booking**: Melakukan `POST /bookings` oleh Customer pada slot tersebut.
- [ ] **Verify Booking Overlap**: Mengulangi langkah 2, slot seharusnya kini berstatus `BOOKED`.
- [ ] **Dummy Payment Confirm**: Mengeksekusi `POST /bookings/:id/pay`.
- [ ] **Failsafe Pay & Cancel**: Mencoba melakukan `pay` kedua kali atau melakukan `cancel` pada booking yang sudah `CONFIRMED` (Harus menerima *Error HTTP 409*).
- [ ] **Owner View**: Memastikan Owner bisa melihat booking yang sudah `CONFIRMED` (atau *all status*) di rute miliknya.
- [ ] **Cancel Flow Validation**: Secara terpisah membuat satu booking lagi dan membatalkannya (`PATCH /bookings/:id/cancel`), lalu memastikan slot tersebut kembali berstatus `AVAILABLE`.

## 7. Risiko dan Blocker
- **Keletihan Pengujian Manual (Manual Testing Fatigue)**: Menyiapkan 7 tahap entitas relasional secara manual (dari pembuatan *user* sampai *operating hours*) hanya mengandalkan cURL/Postman akan memakan waktu sangat lama dan rentan gagal *copy-paste* UUID/Token.
- **Data Cleanup**: Database lokal lama kelamaan akan menjadi "kotor" dengan data pengujian jika tidak ada skrip yang mempermudah _truncate/reset_ data MVP.

## 8. Rekomendasi Tindak Lanjut
**Ya, AntiGravity merekomendasikan untuk langsung mengeksekusi Step 6 setelah Codex melakukan tinjauan (_review_) atas dokumen diskusi ini.**

Pendekatan strategis yang saya sarankan untuk eksekusinya nanti:
1. Kita buat sebuah skrip `seed.go` kecil di `cmd/seeder` untuk menyuntikkan data *prerequisite* agar siap pakai.
2. Kita jalankan satu *Manual QA* berbasis terminal (*cURL*) untuk memvalidasi *checklist*.
3. Kita rekam bukti keberhasilannya (*walkthrough/logs*) ke dalam satu laporan E2E.

Menunggu lampu hijau dari Anda!
