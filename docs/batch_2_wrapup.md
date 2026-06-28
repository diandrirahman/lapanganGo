# Final Report - Batch 2 Wrap-Up

Status: SELESAI
Area: Backend & Frontend

## Ringkasan Perubahan Batch 2

Batch 2 difokuskan pada peningkatan kualitas kode (maintainability), security, standardisasi UI/UX, serta penyelesaian flow booking agar robust. Berikut adalah daftar implementasi yang berhasil diselesaikan:

1. **Step 11 - Rate Limiting**:
   - Menambahkan in-memory rate limiter backend.
   - Limit general API: `100 request/menit`.
   - Limit endpoint Auth (`/auth/login`, `/auth/register`): `10 request/menit` untuk perlindungan brute force.
   
2. **Step 12 - Shared Frontend Utils**:
   - Menambahkan shared utils frontend: `formatRupiah`, `formatDate`, `formatDateTime`, `getPlaceholderImage`, dan hash helper.
   - Mengganti formatter duplikatif pada banyak komponen/halaman agar rapi.

3. **Step 13 - Shared Backend HTTP Utils**:
   - Menambahkan package `internal/httputil`.
   - Standarisasi helper: `GetAuthenticatedUserID`, `GetUUIDParam`, `IsUUID`, `GetPaginationParams`, dan `NewPaginatedResponse`.
   - Menghapus helper lokal yang duplikat di backend handler.

4. **Step 14 - ProtectedRoute Wrapper**:
   - Menambahkan `<ProtectedRoute />` di frontend.
   - Membatasi halaman khusus Owner untuk role `OWNER`, dan halaman `/bookings` khusus `CUSTOMER`.
   - Guest otomatis di-redirect ke `/login`.
   - *Fix Codex*: `/bookings` tidak lagi bisa diakses oleh OWNER, dan Owner tidak dapat membuat booking di `CourtAvailabilityPage`.

5. **Step 15 - Pagination Standardization**:
   - Response list API backend distandarkan dengan key: `data`, `page`, `limit`, `total`, dan `total_pages`.
   - Endpoint ter-update: public venues, customer bookings, owner venue bookings, open matches.
   - Frontend mengimplementasikan shared `<Pagination />` reusable.
   - *Fix Codex*: `page` kembali ke 1 *secara instan* pada handler filter tanpa `useEffect` terpisah.
   - Backend `GetPublicVenues` menormalisasi nilai `req.Page` & `req.Limit` sebelum menjalankan repository query. Test case normalize ditambahkan.

6. **Bugfix Booking Flow (Post-Batch 2)**:
   - *Isu*: Create booking gagal & fetch /bookings gagal jika database lokal lambat mengeksekusi migration 006/007.
   - *Fix Codex*: Menambahkan fungsi `database.EnsureBookingSchema` yang dipanggil otomatis saat API server startup. Fungsi ini menjamin eksistensi kolom `payment_reference`, `expires_at`, update check constraint status booking, serta backfill data `expires_at` otomatis bagi booking lama yang statusnya `PENDING_PAYMENT`.
   - *Frontend*: Error handling pada fetch bookings ditingkatkan agar membaca message respons dari backend dengan tepat.

## Hasil Smoke Test E2E & Validasi Pipeline

| Area Flow | Status | Keterangan |
| :--- | :--- | :--- |
| **Public Venue Search & Filter** | ✅ PASS | Pagination & update filter seketika me-reset ke page 1 dan menampilkan data presisi. |
| **Customer Booking End-to-End** | ✅ PASS | Smoke test E2E Customer Create Booking & List Fetch berjalan mulus tanpa error kolom berkat *EnsureBookingSchema*. |
| **Owner Booking Filter & Pagination** | ✅ PASS | Mengubah status & tanggal pesanan memuat data baru secara responsif dan kembali ke page 1. |
| **Open Matches Pagination** | ✅ PASS | Jadwal mabar mendukung paginasi dan fetching API dengan URL base yang valid. |
| **Auth Guards / Protected Route** | ✅ PASS | Role validation sukses. Owner tidak bisa membuat booking di profilnya sendiri. |
| **Unit Tests Backend** | ✅ PASS | `go test ./...` **PASS 100%**. |
| **Frontend Linting & Build** | ✅ PASS | `npm run build` sukses. `npm run lint` **PASS** (menyisakan 1 warning lama nonblocking). |

## Sisa Risiko / Nonblocking Issues (Untuk Batch Berikutnya)

1. **OwnerDashboardPage Warning**:
   - Masih menyisakan warning lint `react-hooks/exhaustive-deps` lama yang bersifat nonblocking.
2. **Rate Limiting Skala Produksi**:
   - Limiter masih berbasis `in-memory`. Saat aplikasi menuju production multi-instance, limiter ini harus ditingkatkan menggunakan backend Redis.
3. **Database Migration Strategy**:
   - Walau `EnsureBookingSchema` efektif sebagai _safeguard_ skema Booking, idealnya aplikasi menggunakan *migration runner* sesungguhnya (seperti `golang-migrate`) yang berjalan secara utuh untuk seluruh skema saat startup.
4. **Persistent State Management untuk Filter**:
   - _Search/Filter state_ belum tersimpan di parameter URL. Jika pengguna menekan tombol "Back" setelah melihat detil venue, pencarian akan tereset ulang. Disarankan untuk menggunakan URL query params.

Semua kriteria acceptance untuk **Batch 2** kini sepenuhnya valid, aman, dan telah dirangkum dalam laporan ini. Laporan siap untuk dievaluasi akhir oleh tim Codex.
