# Laporan Implementasi - Batch 3

Status: SELESAI
Area: Backend & Frontend

## Ringkasan Eksekusi Batch 3

Berdasarkan *feedback* dan _approval_ Codex, Batch 3 telah dieksekusi dengan pemecahan 4 sub-step spesifik untuk menjamin stabilitas sistem (terutama menghindari _breaking changes_ di backend).

### 1. Step 1 - Migration Runner (Revisi Selesai)
- Menambahkan *dependency* `github.com/golang-migrate/migrate/v4`.
- **Standarisasi File:** Semua file `.sql` dalam `db/migrations` diubah namanya mengikuti standar golang-migrate menjadi `*.up.sql` dan menambahkan `*.down.sql` (dengan placeholder komentar) agar file dikenali sempurna oleh runner dan tidak memicu error versi.
- **Implementasi:** Membuat helper `database.RunMigrations` pada `apps/api/internal/database/schema.go` yang akan mengeksekusi semua migrasi dari folder `db/migrations`.
- **Startup Fail-Fast:** `cmd/api/main.go` tidak lagi sekadar mencetak _warning_. Apabila `database.RunMigrations` gagal, API akan langsung _crash_ (`log.Fatal`) demi keamanan *production*. Apabila db _up-to-date_ (`migrate.ErrNoChange`), runner tetap berjalan _graceful_ dan idempoten.
- **Safe Existing DB:** Revisi `006_booking_payment.up.sql` menggunakan klausa `ADD COLUMN IF NOT EXISTS payment_reference` dan constraint status yang disamakan sempurna (`PENDING_PAYMENT`, `WAITING_VERIFICATION`, `CONFIRMED`, `PAID`, `CANCELLED`, `COMPLETED`).
- **Verifikasi:** Uji coba `go test ./...` **PASS**. Pengecekan pada database (tabel `schema_migrations`) menunjukkan migrasi berada pada **versi 7** dengan status `dirty: false`.

### 2. Step 2 - Frontend URL Query Params
- **Implementasi:** Seluruh *local state* pada filter dan paginasi telah direfaktor menggunakan `useSearchParams` dari `react-router-dom`.
- **Target Halaman:**
  - `VenuesSearchPage` (city, sportId, minPrice, maxPrice, facilityIds, page)
  - `OwnerVenueBookingsPage` (filterDate, filterStatus, page)
- **Verifikasi:** 
  - Parameter di-*update* ke URL secara *real-time*. 
  - Saat mengubah filter, parameter `page` otomatis ter-reset ke `1` di dalam URL. 
  - Fitur _browser back_ kini berhasil *me-restore* state sebelumnya tanpa mereset form filter ke kosong. 
  - `npm run build` & `npm run lint` **PASS**.

### 3. Step 3 - OwnerDashboard Lint Cleanup
- **Implementasi:** Fungsi `loadDashboard` pada komponen `OwnerDashboardPage` telah di-_wrap_ menggunakan hook `useCallback` dengan daftar *dependencies* `[token, startDate, endDate]`.
- **Verifikasi:** Menjalankan `npm run lint` kini menampilkan pesan `Found 0 warnings and 0 errors.` (warning `exhaustive-deps` bersih).

### 4. Step 4 - Redis Rate Limiter & Fallback
- **Infrastruktur:**
  - `docker-compose.yml` di-verifikasi telah memuat service `redis:7` di _port_ 6379.
  - Menambahkan key `REDIS_URL=redis://localhost:6379/0` di dalam `.env.example`.
- **Implementasi:** 
  - Mengubah struktur `RateLimiter` menjadi *Interface-like* logic dengan `MemoryRateLimiter` dan `RedisRateLimiter`.
  - Menggunakan library `github.com/redis/go-redis/v9`.
  - Rate Limiter Redis menggunakan fungsi *Transaction Pipeline* (`TxPipeline`) untuk menjamin eksekusi atomik `INCR` dan `EXPIRE`.
- **Seamless Fallback:** Jika string `REDIS_URL` dibiarkan kosong, _atau_ server gagal melakukan _ping_ ke Redis, sistem akan secara otomatis kembali (*fallback*) menggunakan implementasi `in-memory limiter`. Hal ini memastikan *local development* tetap bisa berjalan normal walaupun tanpa Redis.
- **Verifikasi:** Uji coba `go test ./...` pada `middleware` **PASS** 100%.

## Acceptance Criteria Check

- ✅ `gofmt -l .` bersih (tidak ada file yang butuh di-format).
- ✅ Startup API _fail-fast_ jika migrasi gagal, dan _idempoten_ jika berhasil.
- ✅ File migrasi menggunakan format baku `.up.sql` dan `.down.sql`.
- ✅ Existing DB termigrasi aman (versi 7, dirty `false`) melalui `ADD COLUMN IF NOT EXISTS`.
- ✅ API startup aman di DB kosong & DB _migrated_.
- ✅ Booking flow (dan EnsureBookingSchema fallback) tidak terganggu.
- ✅ Filter & Paginasi Frontend sekarang _bookmarkable_.
- ✅ Warning `exhaustive-deps` hilang dari _linting_.
- ✅ App dapat menggunakan Redis (jika REDIS_URL tersedia) dan tidak _crash_ tanpa Redis.

Batch 3 resmi diselesaikan dan *codebase* sudah sangat stabil serta *production-ready*.
