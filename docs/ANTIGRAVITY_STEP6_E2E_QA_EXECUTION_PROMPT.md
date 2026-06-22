# AntiGravity Prompt: Step 6 - E2E Booking Flow QA

```text
Kamu bertindak sebagai Product-minded Senior Backend Engineer untuk project LapangGo.

Codex review atas diskusi Step 6:
- Rekomendasi E2E Booking Flow QA disetujui.
- Jangan langsung tambah fitur besar.
- Jangan dulu membuat `cmd/seeder` permanen.
- Untuk Step 6 ini, buat QA artifact yang aman, eksplisit, dan mudah diulang.

Tujuan Step 6:
Membuktikan alur booking MVP berjalan end-to-end dengan API/backend yang ada:
1. Availability awal menampilkan slot target sebagai `AVAILABLE`.
2. Customer membuat booking.
3. Availability berubah dan slot target menjadi `BOOKED`.
4. Customer menjalankan dummy payment:
   `POST /bookings/:id/pay`
5. Booking berubah dari `PENDING_PAYMENT` ke `CONFIRMED`.
6. Customer tidak bisa cancel booking yang sudah `CONFIRMED`.
7. Customer tidak bisa pay booking yang sudah `CONFIRMED` untuk kedua kali.
8. Owner bisa melihat booking tersebut di owner booking list.
9. Booking yang `CANCELLED` tidak memblokir availability.

Scope Step 6:
1. Buat dokumentasi manual QA berbasis curl atau HTTP request yang bisa dijalankan lokal.
2. Jika perlu data awal, buat seed SQL khusus QA yang idempotent.
3. Jalankan smoke test lokal jika environment memungkinkan.
4. Buat report hasil eksekusi atau blocker teknis.

File yang boleh dibuat:
- `docs/qa/STEP6_E2E_BOOKING_FLOW_QA.md`
- `docs/qa/step6_e2e_seed.sql` jika seed SQL diperlukan
- `docs/CHATGPT_STEP6_E2E_QA_REPORT.md`

File/source yang tidak boleh diubah:
- Jangan ubah source code Go.
- Jangan ubah handler/service/repository.
- Jangan ubah migration existing.
- Jangan tambah migration baru.
- Jangan tambah `cmd/seeder` dulu.
- Jangan ubah README kecuali Codex minta di step terpisah.

Arahan seed data:
- Gunakan data QA yang jelas dan tidak menyerupai data produksi.
- Seed harus idempotent semaksimal mungkin:
  - gunakan email unik seperti `qa.customer@lapanggo.test` dan `qa.owner@lapanggo.test`
  - gunakan fixed UUID atau `ON CONFLICT` dengan unique key yang sudah ada
  - hindari `TRUNCATE`, `DELETE` massal, atau reset database
- Karena public registration saat ini hanya membuat role `CUSTOMER`, owner QA boleh dibuat via SQL seed dengan role `OWNER`.
- Jika membuat user via SQL, pastikan password hash bisa dipakai login oleh endpoint `/auth/login`.
- Catat password QA plaintext yang dipakai hanya untuk lokal, misalnya `QaPass123!`.
- Pastikan venue status `ACTIVE`, court status `ACTIVE`, dan operating hours terbuka untuk tanggal/time slot QA.
- Pilih tanggal masa depan agar tidak gagal validasi booking past date.
- Gunakan timezone Asia/Jakarta dalam penjelasan tanggal jika relevan.

Manual QA checklist yang wajib dibuktikan:

Data/auth:
1. Apply migration lokal jika belum.
2. Apply seed SQL QA jika dibuat.
3. Login customer dan simpan token.
4. Login owner dan simpan token.

Happy path:
5. `GET /courts/:id/availability?date=YYYY-MM-DD`
   - Expected: slot target `AVAILABLE`.
6. `POST /bookings`
   - Gunakan token customer.
   - Expected: HTTP 201 dan status `PENDING_PAYMENT`.
   - Simpan `booking.id`.
7. `GET /courts/:id/availability?date=YYYY-MM-DD`
   - Expected: slot target berubah menjadi `BOOKED`.
8. `POST /bookings/:id/pay`
   - Gunakan token customer.
   - Expected: HTTP 200 dan booking status `CONFIRMED`.
9. `PATCH /bookings/:id/cancel`
   - Gunakan token customer.
   - Expected: HTTP 409 karena booking sudah `CONFIRMED`.
10. `POST /bookings/:id/pay` kedua kali.
    - Gunakan token customer.
    - Expected: HTTP 409 karena booking sudah `CONFIRMED`.
11. `GET /owner/venues/:id/bookings?date=YYYY-MM-DD&status=CONFIRMED`
    - Gunakan token owner.
    - Expected: booking muncul di list owner.

Cancel availability validation:
12. Buat booking kedua pada slot berbeda.
    - Expected: HTTP 201 dan status `PENDING_PAYMENT`.
13. `PATCH /bookings/:id/cancel` untuk booking kedua.
    - Expected: HTTP 200 dan status `CANCELLED`.
14. `GET /courts/:id/availability?date=YYYY-MM-DD`
    - Expected: slot booking kedua kembali `AVAILABLE` atau minimal tidak `BOOKED`.

Verifikasi test backend:
- Jalankan:
  `cd apps/api`
  `go test ./...`

Catatan Windows:
Jika `go test ./...` diblokir Windows Application Control, jalankan terminal sebagai Administrator/elevated.

Jika environment lokal tidak bisa menjalankan DB/API:
- Jangan mengarang hasil.
- Tulis blocker jelas di report.
- Tetap buat QA checklist dan seed/request artifact.

Acceptance Step 6:
- Ada dokumen QA walkthrough yang bisa diikuti manusia.
- Jika seed SQL dibuat, seed tidak destructive.
- E2E checklist mencakup happy path, conflict path, owner view, dan cancelled availability.
- `go test ./...` tetap lulus, atau blocker dicatat jujur.
- Tidak ada perubahan source Go/migration.

Output report:
Buat file:
`docs/CHATGPT_STEP6_E2E_QA_REPORT.md`

Isi report:
1. File yang dibuat/diubah.
2. Data QA yang dipakai:
   - customer email
   - owner email
   - venue id/name
   - court id/name
   - tanggal dan slot waktu
3. Ringkasan QA walkthrough.
4. Hasil setiap checklist:
   - endpoint
   - expected result
   - actual result
   - pass/fail
5. Hasil `go test ./...`.
6. Blocker jika ada.
7. Risiko/next recommendation setelah Step 6.
```
