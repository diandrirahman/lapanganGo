# Report Antigravity - Batch 1B, Step 8 (Revision)

**To:** Codex
**From:** Antigravity
**Task:** Revisi Step 8 - Booking Expiry Data Model

## 1. File yang Diubah
- `db/migrations/007_booking_expiry.sql`
- `apps/api/internal/config/config.go`
- `apps/api/internal/bookings/service_test.go`

## 2. Ringkasan Revisi
- **Fix 1: Backfill Existing Pending Bookings**
  - Mengupdate skema SQL untuk menggunakan opsi `ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;`.
  - Menambahkan *backfill query* yang secara eksplisit mencari baris yang masih `PENDING_PAYMENT` dan `expires_at IS NULL`, lalu menyetel nilainya menjadi `created_at + interval '30 minutes'`.
- **Fix 2: Add TTL Unit Test**
  - Memperbarui objek `mockRepo` agar merekam input parameter dari pemanggilan `InsertBooking` ke dalam atribut rekam jejak `LastCreateParams`.
  - Membuat *test case* baru: `TestCreateBooking_Success_CustomTTL`. Pengujian ini menginjeksi sebuah custom TTL (45 menit), melakukan sebuah booking mock, lalu memvalidasi secara deterministik bahwa `LastCreateParams.ExpiresAt` nilainya persis (dengan toleransi `time.Second` kecil) sebesar `waktu_sekarang + 45 menit`.
- **Fix 3: Validate Invalid TTL Env**
  - Pada `config.go`, jika admin _environment_ salah menset `BOOKING_PAYMENT_TTL_MINUTES` ke string yang tidak bisa di-_parse_ (mis. 'abc') atau angka <= 0, maka program akan langsung melempar `log.Fatal` ("fail fast").
  - Jika _env_ kosong, otomatis *fallback* dijamin menggunakan `30`.

## 3. Cara Testing
1. **Automated Testing**:
   Menjalankan ulang seluruh Go test untuk package `bookings`.
   ```powershell
   cd apps/api
   go test ./internal/bookings -v
   ```
   **Hasil**: 31 Test PASS (termasuk 1 test TTL baru).

## 4. Risiko atau Catatan Lanjutan
- **Aman**: Penambahan *fail fast logic* di *boot config* menjamin tidak akan ada perilaku aneh atau TTL = 0 saat sistem running jika admin salah memasukkan konfigurasi. Skema *idempotent SQL* menjamin kemudahan *re-run migration* tanpa kendala pada *database* produksi.

---
Silakan direview, hasil revisi siap diperiksa kembali oleh tim Codex!
