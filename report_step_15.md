# Report - Batch 2 Step 15 (Pagination Standardization Backend + Frontend) [REVISI]

Status: SELESAI
Area: Backend & Frontend

## Backend
1. **Pembaruan Infrastruktur (Shared Utils)**: 
   - Ditambahkan struct `PaginatedResponse` dan tipe request parameter standar paginasi di dalam `apps/api/internal/httputil/httputil.go`.
2. **Layer Repository & Service**:
   - Diubah return type pada function-function list di repository dan service layer (`venues`, `bookings`, `mabar`) dari yang sebelumnya me-return slice menjadi `([]T, int, error)` yang merepresentasikan list data dan total rows.
   - **(REVISI)**: Memperbaiki logic default limit/page di `GetPublicVenues`. Parameter limit & page yang sudah dinormalisasi sekarang secara eksplisit di-set ulang ke request object (`req.Limit = limit`, `req.Page = page`) agar query SQL tidak mendapatkan nilai `LIMIT 0`.
3. **Layer Handler**:
   - Menambahkan parsing query param `page` dan `limit`.
   - Mengubah struktur response JSON di handler untuk keempat endpoint list utama (Public Venues, Owner Venue Bookings, Customer Bookings, Open Matches) menjadi terstandarisasi dengan key: `data`, `page`, `limit`, `total`, dan `total_pages`.
4. **Testing**:
   - Mengupdate mock file yang ter-generate dan mengadaptasi mock logic untuk service/handler test di seluruh package terkait.
   - **(REVISI)**: Menambahkan unit test `TestNormalizeListPublicVenuesQuery` untuk memverifikasi fungsionalitas default limit & page.
   - Hasil validasi test: `go test ./...` **PASS**. `gofmt` **CLEAN**.

## Frontend
1. **Komponen Pagination**:
   - Membuat shared component React `Pagination.tsx` di `apps/web/src/components/ui/` beserta styling-nya (menggunakan standar styling LapanganGo).
   - Menambahkan tipe Typescript pendukung untuk paginasi di `apps/web/src/types/pagination.ts`.
2. **API Client**:
   - Mengupdate mapping dan format response di `apps/web/src/lib/api.ts` agar menerima tipe `PaginatedResponse<T>` dan tidak terjadi runtime error terkait perubahan struktur object.
3. **Halaman UI**:
   - Mengintegrasikan state dan komponen `<Pagination />` pada `VenuesSearchPage.tsx`, `CustomerBookingsPage.tsx`, `OwnerVenueBookingsPage.tsx`, dan `OpenMatchesPage.tsx`.
   - **(REVISI)**: Menambahkan logic reset (ke page 1) jika parameter filter berubah:
     - `VenuesSearchPage`: reset page jika `city`, `sportId`, `minPrice`, `maxPrice`, `facilityIds` berubah.
     - `OwnerVenueBookingsPage`: reset page jika `filterDate` dan `filterStatus` berubah.
4. **Build & Compiling**:
   - Hasil validasi Typescript & Vite build: `npm run build` **PASS**.

## Bugfixes Tambahan (Post-Restart)
1. **Migration 007 (Booking Expiry)**:
   - Menjalankan script migration `007_booking_expiry.sql` secara manual ke database untuk menambahkan kolom `expires_at` pada tabel `bookings`. Hal ini memperbaiki error 500 pada endpoint `/courts/:id/availability`.
2. **API Endpoint Fix**:
   - Memperbaiki salah ketik / salah prefix path endpoint API pada frontend di `apps/web/src/lib/api.ts` (menghapus `/api/v1/mabar` menjadi `/open-matches` dan `/api/v1/bookings` menjadi `/bookings`). Hal ini memperbaiki error koneksi saat mengambil data Mabar.

Semua poin revisi dari Acceptance Criteria sudah dipenuhi. Filter reset page dengan benar, default parameter limit/page valid di DB, bug API dan migration telah diperbaiki, serta command verifikasi testing & build sukses dijalankan.
Silahkan disubmit kembali.
