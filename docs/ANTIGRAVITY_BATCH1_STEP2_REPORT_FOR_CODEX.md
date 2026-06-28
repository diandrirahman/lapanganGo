# Report Antigravity - Batch 1A, Step 2 (Fix Booking List Column/Response Consistency)

**To:** Codex
**From:** Antigravity
**Task:** Step 2 - Fix Booking List Column/Response Consistency

## 1. File yang Diubah
- `apps/api/internal/bookings/repository.go`
- `apps/api/internal/bookings/service.go`

## 2. Ringkasan Perubahan
- **Repository**:
  - Merevisi query pada `ListByCustomerID` untuk mengambil nilai `b.payment_reference`. Field ini ditambahkan ke baris SELECT agar sejajar dengan posisi struct property saat dilakukan Scan.
  - Merevisi query pada `ListOwnerVenueBookings` untuk juga ikut memuat `b.payment_reference` ke dalam SELECT dan menyesuaikan Scan. Hal ini menjamin konsistensi bahwa semua API booking yang mereturn detail memiliki field terkait.
  - Memastikan kolom `FindCustomerBookingByID` dan `FindByIDAndCustomerID` sudah fully matched antara jumlah field di SELECT dan destination variable di Scan.
- **Service (Mapper)**:
  - Memperbarui fungsi `toBookingResponse` dan `toOwnerBookingResponse` untuk memetakan pointer field `PaymentReference` (dari entity `Booking` dan `OwnerBooking`) langsung ke DTO response sehingga datanya betul-betul dikirim melalui API (jika terisi/not null).

## 3. Cara Testing
1. **Automated Testing**:
   Menjalankan test backend API melalui PowerShell:
   ```powershell
   cd apps/api
   go test ./internal/bookings -v
   ```
   **Hasil**: Semua test (24 scenarios) berlalu dengan sukses (0 errors).

2. **Manual Verification**:
   - Jika endpoint `GET /bookings` dipanggil oleh customer, maka response untuk booking yang berstatus `WAITING_VERIFICATION` atau sesudahnya akan memiliki field `payment_reference`.
   - Endpoint `GET /bookings/:id` dan `GET /owner/venues/:id/bookings` juga berhasil mereturn payload lengkap tanpa _runtime error scan mismatch_.

## 4. Risiko atau Catatan Lanjutan
- **Aman**: Hanya merubah projection field dari `SELECT` dan men-wire up nilainya ke payload DTO. Tidak ada flow core/state machine dari proses booking yang diubah.
- Endpoint-endpoint yang tadinya berisiko 500 error dikarenakan mismatch parameter scan kini sudah normal dan stabil secara struktur.

---
Silakan direview, dan saya siap lanjut ke step berikutnya di Batch 1A!
