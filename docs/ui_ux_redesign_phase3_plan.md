# UI/UX Redesign Phase 3: Owner Dashboard & Management Polish Plan

Tujuan fase ini adalah meningkatkan antarmuka manajemen pemilik (Owner Dashboard, Manajemen Venue, Court, dan Pesanan) agar lebih profesional, padat informasi (dense/operational), serta mudah dipindai dengan cepat oleh pengelola.

## 1. Modifikasi API Backend (Metrics)

Untuk memenuhi kebutuhan metrik operasional yang jelas, khususnya status pembayaran tertunda, kami akan memodifikasi struktur metrik yang dikembalikan oleh API.

- **Lokasi File**: `apps/api/internal/bookings/repository.go` dan `service.go` / `dto.go`
- **Perubahan**: 
  - Menambahkan _field_ `PendingVerifications int` pada struct balasan metrik.
  - Menambahkan kueri `COUNT(*)` untuk `status = 'WAITING_VERIFICATION'` ke dalam logika `GetOwnerMetrics`.
- **Dampak**: *Non-breaking change* yang langsung memperkaya payload `/owner/metrics` untuk dikonsumsi frontend tanpa perlu *fetching* ganda.
- **Tindakan Lanjutan**: API backend **wajib di-restart** setelah perubahan ini sebelum testing manual dilanjutkan.

## 2. Redesign: Owner Dashboard Page

- **Lokasi File**: `apps/web/src/pages/owner/OwnerDashboardPage.tsx`
- **Target Perubahan**:
  - Menghapus gradien *hero* berukuran besar ala *marketing* dan menggantinya dengan panel ringkas bergaya operasional.
  - Memasukkan data metrik baru: `Pending Verifications` sejajar dengan Total Venue, Upcoming Bookings, dan Estimated Revenue.
  - Mengubah blok Quick Actions menjadi deretan navigasi yang tidak boros ruang vertikal (Kelola Venue, Tambah Venue, Cek Pesanan Masuk).
  - Menyediakan fallback data yang lebih rapi untuk `occupancy_rate`.

## 3. Redesign: Owner Venues Page

- **Lokasi File**: `apps/web/src/pages/owner/OwnerVenuesPage.tsx`
- **Target Perubahan**:
  - Menggunakan komponen `SafeVenueImage` yang sudah dibuat pada seluruh *card venue*.
  - Mengatur ulang hierarki kartu agar ringkasan fasilitas, kota, dan *status venue* tampil proporsional tanpa *text overlap*.
  - Merapikan struktur grup tombol (Edit Detail & Foto, Kelola Court, Lihat Pesanan) agar rapi berjajar pada ukuran layar Desktop, Laptop, Tablet, dan Mobile.

## 4. Redesign: Owner Courts Page

- **Lokasi File**: `apps/web/src/pages/owner/OwnerCourtsPage.tsx`
- **Target Perubahan**:
  - Mengonversi daftar *court* yang saat ini mungkin terkesan renggang menjadi tabel atau grid informasi solid (dense).
  - Menempatkan indikator jelas terkait Tipe Lokasi, Permukaan (Surface), dan Harga Per Jam.
  - Memastikan *button action* (Operating Hours, Blocked Slots, Edit Lapangan) mudah dipencet (*touch-friendly*) di HP namun tidak pecah keluar *container*.

## 5. Redesign: Owner Venue Bookings Page

- **Lokasi File**: `apps/web/src/pages/owner/OwnerVenueBookingsPage.tsx`
- **Target Perubahan**:
  - Menyusun filter Tanggal dan Status menjadi bentuk _Toolbar/Filter Bar_ terpadu.
  - Mempertegas *badge status* (terutama `WAITING_VERIFICATION` dengan warna mencolok/peringatan).
  - Menampilkan *Payment Reference* lebih strategis.
  - Memperbaiki konfirmasi pada aksi *Approve/Reject* pembayaran demi keamanan alur UX (*ConfirmModal*).

## 6. Cleanup Komponen Reusable

- Menghindari pembuatan "Card-in-Card" (menumpuk elemen *border* berlapis) yang merusak hierarki *border-radius*.
- Mengunci `border-radius` agar konsisten (batas maksimal `rounded-2xl` atau `rounded-3xl` sesuai pedoman Phase 2).
- Memastikan tidak terjadi *code-duplication* untuk status *Error/Loading/Empty*.

## 7. Quality Assurance (QA) Checklist

1. **Automated Checks**:
   - `npm run lint` & `npm run build` berhasil sempurna (0 warnings/errors).
   - `go test ./...` tidak ada _failing test_.
2. **Visual/Responsive Checks**:
   - Skala desktop (1920x1080 & 1366x768).
   - Skala *Mobile* & *Tablet* (pastikan *grid* merespons secara reaktif, input ukuran layar tidak terpotong).
3. **Functional Checks (Manual)**:
   - Akses metrik `Pending Verifications` sukses.
   - Kelola informasi jam buka-tutup (Operating Hours) + Blocked Slots tidak saling tindih secara *state*.
   - Menerima pesanan dan mengeksekusi "Terima Pembayaran" memicu validasi hijau.
