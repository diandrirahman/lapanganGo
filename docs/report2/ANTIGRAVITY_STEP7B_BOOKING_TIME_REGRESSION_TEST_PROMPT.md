# AntiGravity Prompt: Step 7B - Add Booking Operating Hours Regression Test

```text
Kamu bertindak sebagai Product-minded Senior Backend Engineer untuk project LapangGo.

Codex review atas Step 7:
Manual E2E smoke test report menunjukkan semua endpoint PASS dan menemukan bug nyata pada validasi operating hours booking. Fix di `apps/api/internal/bookings/service.go` secara konsep benar: waktu request dinormalisasi agar dibandingkan berdasarkan hour/minute saja, karena PostgreSQL/pgx TIME dapat memakai base year 2000 sedangkan `time.Parse("15:04", ...)` memakai base year 0000.

Namun Step 7 belum approved final karena regression test spesifik untuk bug ini belum ada.

Tugas Step 7B:
1. Tambahkan unit test di:
   `apps/api/internal/bookings/service_test.go`

2. Test harus mereproduksi kasus bug:
   - Request booking:
     - `booking_date`: tanggal future yang valid
     - `start_time`: `10:00`
     - `end_time`: `11:00`
   - Operating hour mock dari repository memakai `time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC)` untuk `OpenTime`
   - Operating hour mock memakai `time.Date(2000, 1, 1, 22, 0, 0, 0, time.UTC)` untuk `CloseTime`
   - Court dan venue status `ACTIVE`
   - No blocked slot
   - No existing booking
   - Expected: `CreateBooking` sukses, bukan `ErrOutsideOpHours`

3. Tambahkan juga boundary test bila sederhana:
   - `08:00-09:00` sukses
   - `21:00-22:00` sukses
   Jangan membuat suite besar kalau terlalu banyak perubahan.

4. Jangan ubah behavior production kecuali test membuka bug baru yang nyata.
5. Jangan ubah migration.
6. Jangan ubah README.
7. Update report:
   `docs/CHATGPT_STEP7_E2E_MANUAL_RUN_REPORT.md`

Report wajib menyebut:
1. Regression test yang ditambahkan.
2. Kenapa test ini penting.
3. Hasil `go test ./...`.
4. Status E2E manual tetap PASS dari Step 7.
5. Risiko tersisa.

Verifikasi wajib:
Jalankan:

```powershell
cd apps/api
go test ./...
```

Catatan Windows:
Jika `go test ./...` diblokir Windows Application Control, gunakan terminal Administrator/elevated.
```
