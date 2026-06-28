# UI/UX Redesign Phase 2: Public Venue Discovery & Booking Flow

Melanjutkan ke fase 2 dan bagian pemesanan, fokus kita adalah memperbaiki alur konsumen (customer flow) agar lebih terpoles, mudah digunakan, dan terlihat profesional. Desain akan memprioritaskan pencarian, fungsionalitas, serta keterbacaan data (fasilitas, harga, ketersediaan) dibandingkan dengan ornamen visual yang berlebihan.

## Open Questions
- Pada `CourtAvailabilityPage.tsx` saat ini, frontend tidak memanggil nama court secara langsung dari API (API ketersediaan jadwal hanya mengembalikan status slot). Untuk menampilkan ringkasan pesanan yang lebih kaya (Nama Venue, Nama Lapangan, Harga), saya akan mengirimkannya melalui _router state_ (via fungsi `navigate`) dari halaman Detail Venue. Apakah pendekatan ini disetujui (agar tidak perlu merombak backend API)?

## Proposed Changes

### 1. App Routing & Home
#### [MODIFY] [App.tsx](file:///d:/project/lapangGo/apps/web/src/App.tsx)
- Mengarahkan Route path `/` langsung ke komponen `VenuesSearchPage`.
- Menghapus komponen `HomePage` lama yang berisi *landing page* kosong (atau me-redirect rute `/venues` ke `/` jika diperlukan).

#### [MODIFY] [HomePage.tsx](file:///d:/project/lapangGo/apps/web/src/pages/HomePage.tsx)
- Mengganti isinya agar merender tampilan pencarian lapangan secara instan. Menghapus komponen marketing landing page berlebihan yang tidak fungsional.

### 2. Search Page & Components
#### [MODIFY] [VenuesSearchPage.tsx](file:///d:/project/lapangGo/apps/web/src/pages/VenuesSearchPage.tsx)
- Merapikan form filter ke dalam layout yang lebih padat (*compact*) dan modern.
- Menghapus bayangan *orb* atau gradient yang berlebihan, menggunakan latar putih/abu-abu netral yang lebih matang.

#### [MODIFY] [VenueCard.tsx](file:///d:/project/lapangGo/apps/web/src/components/VenueCard.tsx)
- Menggunakan `primary_photo` sebagai foto utama *card* (dengan fallback image).
- Menampilkan *chips* fasilitas secara ringkas (maksimal 3-4 fasilitas).
- Merapikan hierarki tipografi (Nama Venue tebal, Kota/Alamat lebih lembut).

### 3. Venue Detail Page
#### [MODIFY] [VenueDetailPage.tsx](file:///d:/project/lapangGo/apps/web/src/pages/VenueDetailPage.tsx)
- Merombak bagian atas (*Hero*) menjadi tampilan *banner* lebar atau *split-grid* yang elegan (terinspirasi dari platform pemesanan modern).
- Memisahkan informasi deskripsi, lokasi, dan galeri ke dalam blok konten yang rapi.
- Menyusun daftar lapangan (*court list*) dengan tampilan *card* yang jelas beserta informasi harga `price_per_hour` untuk masing-masing lapangan.
- Menyertakan data court saat navigasi ke halaman Booking melalui *state*.

### 4. Booking Flow
#### [MODIFY] [CourtAvailabilityPage.tsx](file:///d:/project/lapangGo/apps/web/src/pages/CourtAvailabilityPage.tsx)
- Menghadirkan seleksi tanggal (Date Picker) yang lebih ramah pengguna.
- Merapikan grid jam/waktu: Menggunakan kode warna fungsional yang lebih tegas (mis. Outline hijau untuk tersedia, Abu-abu pudar untuk Booked).
- Menambahkan **Booking Summary** (*sidebar* atau *bottom sheet* di mobile) sebelum tombol bayar, yang merangkum tanggal, waktu, dan estimasi total harga (berdasarkan data dari *router state*).

## Verification Plan

### Automated Tests
- Menjalankan linting: `npm run lint`
- Menjalankan build test: `npm run build`
- Menjalankan backend test (jika ada *state/logic* terkait API yang tak sengaja terpengaruh): `go test ./...`

### Manual Verification
- Membuka halaman utama (`/`) dan memastikan yang tampil adalah laman pencarian lapangan lengkap dengan filter.
- Membuka halaman detail venue, memastikan ketersediaan UI hero galeri, *chips* fasilitas, serta daftar court.
- Menyimulasikan _booking flow_: Memilih tanggal, memilih jam yang tersedia, memvalidasi ringkasan pesanan (*Booking Summary*), hingga konfirmasi ke halaman pembayaran _pending_.
- Mengecek keresponsifan komponen pada ukuran layar *mobile/tablet*.
