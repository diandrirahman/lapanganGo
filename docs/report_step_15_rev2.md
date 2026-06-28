# Report - Batch 2 Step 15 (Pagination Standardization Backend + Frontend) [REVISI 2]

Status: SELESAI
Area: Backend & Frontend

## Backend
1. **Pembaruan Infrastruktur (Shared Utils)**: 
   - Ditambahkan struct `PaginatedResponse` dan tipe request parameter standar paginasi di dalam `apps/api/internal/httputil/httputil.go`.
2. **Layer Repository & Service**:
   - Diubah return type pada function-function list di repository dan service layer (`venues`, `bookings`, `mabar`) dari yang sebelumnya me-return slice menjadi `([]T, int, error)` yang merepresentasikan list data dan total rows.
   - Memperbaiki logic default limit/page di `GetPublicVenues`. Parameter limit & page yang sudah dinormalisasi sekarang secara eksplisit di-set ulang ke request object agar query SQL tidak mendapatkan nilai `LIMIT 0`.
3. **Layer Handler**:
   - Menambahkan parsing query param `page` dan `limit`.
   - Mengubah struktur response JSON di handler untuk keempat endpoint list utama (Public Venues, Owner Venue Bookings, Customer Bookings, Open Matches) menjadi terstandarisasi dengan key: `data`, `page`, `limit`, `total`, dan `total_pages`.
4. **Testing**:
   - Hasil validasi test: `go test ./...` **PASS**. `gofmt` **CLEAN**.

## Frontend
1. **Komponen Pagination**:
   - Membuat shared component React `Pagination.tsx` beserta styling-nya, dan tipe Typescript pendukung.
2. **Halaman UI & Paginasi (REVISI)**:
   - **VenuesSearchPage**:
     - Memindahkan logic `setPage(1)` secara langsung ke dalam semua `onChange` dan `onClick` handler untuk filter (city, sportId, minPrice, maxPrice, facilityIds).
     - Menghapus `useEffect` terpisah yang mereset page agar tidak terjadi fetching page lama bersamaan dengan fetching page baru.
   - **OwnerVenueBookingsPage**:
     - Sama seperti venues, memindahkan `setPage(1)` ke handler `onChange` pada `filterDate` dan `filterStatus`.
     - Merefaktor `loadBookings` dengan menggunakan `useCallback` agar struktur *effect* stabil dan menghilangkan warning `exhaustive-deps`.
3. **Build & Compiling**:
   - Hasil validasi Typescript & Vite build: `npm run build` **PASS**.
   - `npm run lint` berjalan dengan baik dan tidak menimbulkan error atau warning terkait exhaustive-deps yang baru.

## Bugfixes Tambahan (Post-Restart)
1. **Migration 007 (Booking Expiry)**: Menjalankan script migration secara manual untuk menambahkan kolom `expires_at` pada tabel `bookings` untuk memperbaiki error 500 `availability`.
2. **API Endpoint Fix**: Memperbaiki routing URL Mabar `/open-matches` dan Bookings `/bookings` pada frontend `api.ts`.

Semua kriteria acceptance terpenuhi. Pindah halaman reset dengan benar tanpa efek samping, useCallback stabil, dan kode frontend & backend berhasil lolos dari check lint & build.
