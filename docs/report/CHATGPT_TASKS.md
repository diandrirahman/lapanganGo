# Task Pengembangan Backend LapanganGo (Untuk ChatGPT / Codex)

Berikut adalah daftar task berukuran kecil dan terfokus untuk melanjutkan pengembangan backend LapanganGo. Tolong kerjakan satu per satu secara berurutan. Jangan merombak arsitektur yang sudah ada, tetap ikuti konvensi di `docs/AI_HANDOFF.md`.

---

## Task 1: Endpoint Public Listing Venues

**Tujuan:**
Membuat endpoint API publik agar customer (atau guest yang belum login) bisa melihat daftar venue yang aktif. Mendukung pagination.

**File yang kemungkinan terdampak:**
1. `apps/api/internal/venues/dto.go` (Tambah struct `VenuePublicResponse` dan query param dto)
2. `apps/api/internal/venues/repository.go` (Tambah query SQL untuk get all status='ACTIVE')
3. `apps/api/internal/venues/service.go` (Tambah logika `GetPublicVenues`)
4. `apps/api/internal/venues/handler.go` (Tambah fungsi handler `GetPublicVenues`)
5. `apps/api/cmd/api/main.go` (Daftarkan route `GET /venues` ke public route, BUKAN di bawah middleware auth)

**Acceptance Criteria:**
- Endpoint `GET /venues` berhasil di-hit tanpa *Bearer token*.
- Response hanya mengembalikan venue dengan status `ACTIVE`.
- Mendukung query parameter `limit` (default: 10) dan `page` (default: 1) untuk pagination.
- JSON response field menggunakan `snake_case`.

**Test yang perlu disiapkan (opsional jika diminta):**
- Unit test pada repository untuk memastikan query memfilter status `ACTIVE`.
- Unit test pada handler untuk memastikan endpoint berstatus 200 tanpa Auth.

**Risiko & Perhatian:**
- Hati-hati jangan sampai membocorkan data venue berstatus `INACTIVE` atau `DRAFT`.

*Silakan berikan kode implementasinya untuk Task 1.*

---

## Task 2: Endpoint Public Detail Venue & Court

**Tujuan:**
Membuat endpoint publik untuk melihat detail sebuah venue berserta daftar lapangan (courts) di dalamnya, hanya untuk yang berstatus aktif.

**File yang kemungkinan terdampak:**
1. `apps/api/internal/venues/repository.go`, `service.go`, `handler.go`
2. `apps/api/internal/courts/repository.go`, `service.go` (jika dipanggil oleh venue service)
3. `apps/api/cmd/api/main.go` (Daftarkan route `GET /venues/:id` ke public route)

**Acceptance Criteria:**
- Endpoint `GET /venues/:id` berhasil di-hit tanpa token.
- Response mengembalikan data detail venue digabung dengan list array courts milik venue tersebut.
- Baik Venue maupun Courts yang direturn WAJIB berstatus `ACTIVE`.
- Jika venue tidak ditemukan atau statusnya bukan `ACTIVE`, return HTTP `404 Not Found`.

**Risiko & Perhatian:**
- Hindari N+1 query problem. Sebaiknya ambil venue, lalu jalankan satu query `SELECT * FROM courts WHERE venue_id = ? AND status = 'ACTIVE'`.

*Kerjakan ini setelah Task 1 selesai.*

---

## Task 3: Initial Database Migration untuk Bookings

**Tujuan:**
Menyiapkan struktur tabel database untuk menyimpan transaksi pemesanan (booking) sebagai fondasi fitur selanjutnya. Hanya buat file migrasinya, belum perlu kode Go.

**File yang dibuat:**
1. `db/migrations/004_bookings.sql`

**Acceptance Criteria:**
- Tabel `bookings` dibuat dengan kolom:
  - `id` (uuid, primary key)
  - `customer_id` (uuid, fk ke tabel users)
  - `court_id` (uuid, fk ke tabel courts)
  - `booking_date` (date)
  - `start_time` (time)
  - `end_time` (time)
  - `total_price` (numeric/decimal)
  - `status` (varchar, default 'PENDING_PAYMENT')
  - `created_at` (timestamp)
  - `updated_at` (timestamp)

**Risiko & Perhatian:**
- Sesuaikan tipe data `date` dan `time` dengan standar tabel `operating_hours` yang sudah ada agar konsisten.

*Kerjakan ini setelah Task 2 selesai.*
