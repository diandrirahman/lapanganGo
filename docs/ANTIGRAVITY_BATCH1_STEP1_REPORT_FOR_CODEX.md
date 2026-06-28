# Report Antigravity - Batch 1A, Step 1 (Fix VerifyPayment Authorization Bypass)

**To:** Codex
**From:** Antigravity
**Task:** Step 1 - Fix VerifyPayment Authorization Bypass

## 1. File yang Diubah
- `apps/api/internal/bookings/repository.go`
- `apps/api/internal/bookings/service.go`
- `apps/api/internal/bookings/handler.go`
- `apps/api/internal/bookings/service_test.go`

## 2. Ringkasan Perubahan & Revisi
- **Repository**: 
  - Menambahkan fungsi `GetBookingOwnerProfileID` yang melakukan join pada tabel `bookings`, `courts`, dan `venues` untuk mengekstrak `owner_profile_id` dari venue tempat booking terkait berada.
  - Memperbaiki query `FindCustomerBookingByID` agar mencantumkan `b.payment_reference` di baris SELECT, sehingga jumlah field sesuai dengan target struct saat _scan_.
- **Service**: 
  - Memperbarui fungsi `VerifyPayment` dengan memanggil `GetBookingOwnerProfileID`. Fungsi ini akan membandingkan `owner_profile_id` dari venue dengan `owner_profile_id` dari user yang saat ini sedang teraotentikasi.
  - Menambahkan mapping error dari `FindOwnerProfileByUserID` sehingga bila `pgx.ErrNoRows` terjadi, error dipetakan dengan rapi ke `ErrOwnerProfileNotFound` (tidak menghasilkan HTTP 500).
  - Menambahkan definisi error standar baru: `ErrForbidden = errors.New("forbidden: you do not own this booking's venue")`.
- **Handler**: Menambahkan penanganan untuk `ErrForbidden` pada pemetaan error `respondBookingError`, sehingga request akan mendapatkan HTTP status code 403 (Forbidden) jika validasi gagal.
- **Test**: 
  - Memodifikasi interface `mockRepo` agar mendukung dummy method untuk `GetBookingOwnerProfileID` beserta property yang mensimulasikan kepemilikan booking.
  - Menambahkan 4 unit test baru (`TestVerifyPayment_Success`, `TestVerifyPayment_Fail_ErrForbidden`, `TestVerifyPayment_Fail_ErrBookingNotFound`, `TestVerifyPayment_Fail_ErrOwnerProfileNotFound`) yang menjamin logika autorisasi berjalan sesuai ekspektasi.

## 3. Cara Testing
1. **Automated Testing**:
   Menjalankan Go test di direktori backend API dengan command PowerShell (run as Administrator sesuai kebijakan sistem). Seluruh unit test dalam subdirektori `bookings` telah berhasil (0 errors):
   ```powershell
   cd apps/api
   go test ./internal/bookings -v
   ```
2. **Manual Testing / Validation Scenario**:
   - Daftarkan/Siapkan dua akun Owner: Owner A (pemilik Venue A) dan Owner B (pemilik Venue B).
   - Buat sebuah booking untuk lapangan di Venue A yang mencapai status `WAITING_VERIFICATION`.
   - Gunakan Token JWT dari **Owner B** untuk mengakses API: `PATCH /owner/bookings/{bookingID}/verify-payment`.
   - **Hasil**: API akan menolak request dengan status `403 Forbidden` dan menampilkan pesan `"forbidden: you do not own this booking's venue"`.
   - Lakukan ulang menggunakan Token JWT dari **Owner A**.
   - **Hasil**: API akan memproses dan mengkonfirmasi payment (`200 OK`).

## 4. Risiko atau Catatan Lanjutan
- **Aman dan Solved**. Perbaikan ini sepenuhnya menutup celah bypass keamanan tanpa menimbulkan side effects terhadap fungsi lain.
- Validasi dilakukan langsung melalui kueri relasional database dari `booking` ke `court` ke `venue` ke `owner_profile_id`, memastikan ketepatan pengecekan hak akses.
- Demo bypass comment di source code sudah dihilangkan.

---
Silakan direview, dan saya siap lanjut ke **Step 2 - Fix Booking List Column Mismatch**.
