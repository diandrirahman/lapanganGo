# Report Antigravity - Batch 1B, Step 9 (Ignore Expired Pending Bookings in Availability)

**To:** Codex
**From:** Antigravity
**Task:** Step 9 - Ignore Expired Pending Bookings in Availability

## 1. File yang Diubah
- `apps/api/internal/availability/repository.go`
- `apps/api/internal/availability/service.go`
- `apps/api/internal/availability/service_test.go`
- `apps/api/internal/bookings/repository.go`

## 2. Ringkasan Perubahan
- **Availability Repository**: Menambahkan field `status` dan `expires_at` pada `SELECT` query `ListActiveBookings` dan memetakannya ke struct internal `ActiveBooking`. Selain itu, kami juga tetap mempertahankan penyaringan query level *database* untuk meningkatkan efisiensi dengan clause `AND NOT (status = 'PENDING_PAYMENT' AND (expires_at IS NULL OR expires_at <= NOW()))`.
- **Availability Service**: Modifikasi logika iterasi *overlap checking* di `overlapsAnyBooking`. Apabila sebuah slot terikat dengan status `PENDING_PAYMENT` dan `ExpiresAt` sudah memotong batas `time.Now()`—ataupun *null*—kami mengabaikannya (di-_bypass_ dari status `BOOKED`), sehingga slot waktu bisa dipesan kembali oleh pelanggan lain.
- **Booking Repository (Overlap Query)**: Untuk mencegah kolisi saat *create booking*, metode `CheckExistingBookings` juga kami perkuat dengan clause SQL `AND NOT (status = 'PENDING_PAYMENT' AND (expires_at IS NULL OR expires_at <= NOW()))`. Ini penting untuk menyingkirkan pesanan kadaluwarsa ketika memvalidasi ketersediaan di waktu aktual pemesanan.
- **Availability Service Test**: Menyusun *unit test* lengkap `TestBuildSlotsIgnoresExpiredBookings` dengan total empat iterasi kasus:
  1. Status *Confirmed* memblokir slot,
  2. Status *Pending Payment* kadaluwarsa terbukti **TIDAK** memblokir slot,
  3. Status *Pending Payment* masih aktif *(active timeout)* sukses memblokir slot, dan
  4. Status *Pending Payment* di mana *expire*-nya `null` **TIDAK** memblokir slot.

## 3. Cara Testing
1. **Automated Testing**:
   Jalankan Go test package ketersediaan dan semua package lain.
   ```powershell
   cd apps/api
   go test ./internal/availability -v
   go test ./...
   ```
   **Hasil**: Semua test di package ketersediaan sukses (termasuk 3 kriteria dari spesifikasi di-*cover* 100%). Total hasil kompilasi *root level* `go test ./...` berstatus **ok / PASS**.

## 4. Risiko atau Catatan Lanjutan
- **Aman**: Desain pemisahan parameter SQL filter *(belt)* dan logic program di Go *(suspenders)* sangat kuat. Walaupun data belum diberesihkan oleh *worker job* (yang direncanakan di Step 10), *end user* tidak akan merasakan pemblokiran palsu (false blocking) dari pesanan kadaluwarsa yang menggantung di *database*.

---
Tugas Step 9 (Availability Expiry Filter) siap direview. Saya menantikan konfirmasi Anda untuk segera mengimplementasikan *Worker* eksekusi pembatalan otomatis di Step 10!
