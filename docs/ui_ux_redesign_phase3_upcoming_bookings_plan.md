# UI/UX Redesign Phase 3: Upcoming Bookings Flow Plan

## Objective
Menghubungkan kartu statistik "Pesanan Mendatang" di *Owner Dashboard* sehingga menjadi *clickable* dan dapat langsung menampilkan daftar pesanan mendatang milik *owner*.

## Rencana Implementasi

### 1. Perubahan Database Query & Backend API
- **Endpoint Target**: `GET /owner/venues/:id/bookings`
- **DTO**: Menambahkan parameter `Scope` (`form:"scope" binding:"omitempty,oneof=upcoming"`) ke dalam *struct* `OwnerVenueBookingsQuery` (berkas `dto.go`).
- **Repository (`repository.go`)**:
  - Memodifikasi fungsi `ListOwnerVenueBookings` agar dapat menerima argumen *scope*.
  - Menyuntikkan kondisi filter dinamis pada kueri `count` dan `select` jika `scope == "upcoming"`:
    - Hanya mengambil pesanan dengan `booking_date >= CURRENT_DATE` (hari ini atau di masa depan).
    - Menyingkirkan/mengecualikan pesanan yang dibatalkan (`status != 'CANCELLED'`).
- **Service (`service.go`)**: Mengalirkan parameter baru ini dari struktur kueri ke pemanggilan *repository*.
- **Tests (`service_test.go`)**: Menyesuaikan deklarasi `mockRepo` dengan *signature* fungsi terbaru agar *unit test* tetap lulus tanpa kendala.

### 2. Perubahan Frontend API Client
- **`fetchOwnerVenueBookings` (`apps/web/src/lib/api.ts`)**:
  - Menambahkan opsional parameter `scope?: string`.
  - Melampirkannya ke objek `URLSearchParams` agar bisa dikirimkan ke backend.

### 3. Perubahan UI/UX (Frontend Pages)
- **`OwnerDashboardPage.tsx`**:
  - Mengubah penampang *card* "Pesanan Mendatang" menjadi komponen tombol interaktif (`<button>`).
  - Menambahkan aksi `onClick={() => navigate('/owner/venues?intent=upcoming_bookings')}` untuk mengarahkan pengguna dengan menyisipkan sinyal *intent*.
- **`OwnerVenuesPage.tsx`**:
  - Membaca _query parameter_ `intent`.
  - Jika `intent === 'upcoming_bookings'`, tampilkan *banner* petunjuk (Helper Text): *"Pilih 'Lihat Pesanan' pada venue untuk melihat pesanan mendatang."* di atas daftar *venue*.
  - Pada *Quick Action* "Lihat Pesanan", URL akan dimodifikasi dinamis: mengoper parameter `?scope=upcoming` (menjadi `/owner/venues/:id/bookings?scope=upcoming`).
- **`OwnerVenueBookingsPage.tsx`**:
  - Menangkap `scope=upcoming` dari `searchParams`.
  - Jika `scope` terdeteksi, tambahkan penanda teks berbunyi *"Menampilkan Pesanan Mendatang"* di bagian atas daftar tabel.
  - Memastikan *state* parameter `scope` ikut diteruskan ke fungsi `loadBookings` (via `fetchOwnerVenueBookings`), sehingga backend melakukan *filtering* otomatis pada muatan awal (initial load).
  - Menyediakan perlindungan agar *filter* manual status/tanggal yang sudah diperbaiki sebelumnya tidak rusak ketika parameter `scope` digunakan.

## Verification & Acceptance Criteria
- Menguji API Backend secara terisolasi via perintah `go test ./...`
- Menjalankan `npm run lint` & `npm run build` untuk memvalidasi tidak ada tipe `TypeScript` yang rusak (*type break*).
- Melakukan verifikasi *flow* manual di *browser*:
  1. Klik "Pesanan Mendatang" membawa pengguna ke laman manajemen *Venue* dengan instruksi spesifik.
  2. Mengeklik "Lihat Pesanan" menuntun ke daftar *booking* dengan status hanya untuk periode waktu ini dan mendatang, tanpa status "CANCELLED".
  3. Filter bawaan tanggal dan status tetap normal berfungsi.
