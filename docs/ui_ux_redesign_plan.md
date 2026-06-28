# Rencana Desain Ulang UI/UX LapanganGo

Dokumen ini memuat rencana (*redesign plan*) untuk memperbarui antarmuka pengguna (UI) dan pengalaman pengguna (UX) web LapanganGo sesuai instruksi Codex. Fokus utama adalah membuat desain yang matang, bersih (*clean*), modern, konsisten, dan responsif tanpa mengubah logika bisnis, kontrak API, maupun skema basis data.

## Keterangan Penting (Fasilitas Venue)
Berdasarkan pengecekan basis kode saat ini, **frontend belum memiliki halaman Edit Venue**. Namun, di sisi *backend*, endpoint `PUT /owner/venues/:id` sebenarnya sudah tersedia. Untuk fase polesan antarmuka pengguna (UI polish) ini, pembuatan UI untuk halaman Edit Venue akan dilewati dan dicatat sebagai *follow-up product feature*.

---

## 1. Komponen yang Akan Terpengaruh

Perombakan UI tidak hanya menyentuh halaman utama, tetapi juga komponen-komponen yang digunakan di dalamnya:
- Navbar
- VenueCard
- CourtModal
- OperatingHoursModal
- BlockedSlotsModal
- ConfirmModal
- State komponen: LoadingState, ErrorState, EmptyState

---

## 2. Arahan Desain & UX

### **Customer / Public Pages**
- Jangan hanya mengandalkan *gradient* atau *glassmorphism*. Gunakan visual terkait olahraga atau lapangan yang relevan.
- Tetap fungsional: aplikasi harus langsung bisa digunakan (*usable*) untuk pencarian dan pemesanan, bukan sekadar halaman *landing page* kosong.
- Tampilan harus *clean* dan menarik, menonjolkan lokasi, harga, dan *chips* fasilitas dengan jarak/padding yang baik.

### **Owner Pages**
- Desain dasbor dan halaman manajemen (operasi) harus operasional, rapi, dan mudah dipindai (*scannable*).
- Hindari tata letak bergaya pemasaran (*marketing layout*) yang terlalu besar atau ornamental.
- Prioritas utama pada *table/card list*, filter, lencana status (*status badge*), tombol tindakan (*action button*), serta keterbacaan (readability) pada layar *mobile*.

---

## 3. Tahapan Implementasi (Sistem Iteratif Per Fase)

Pekerjaan tidak akan dilakukan pada 13 halaman sekaligus, melainkan dibagi menjadi fase berikut. Pada akhir setiap fase (1-4), pemeriksaan rutin (lint, build, & manual QA browser) wajib dijalankan.

**Phase 1: Autentikasi**
- `LoginPage.tsx`
- `RegisterPage.tsx`
- Menyediakan landasan gaya form input yang standar.

**Phase 2: Jalur Penemuan (Discovery)**
- `HomePage.tsx`
- `VenuesSearchPage.tsx`
- `VenueDetailPage.tsx`
- Memperbarui Navbar dan VenueCard.

**Phase 3: Jalur Pemesanan (Booking)**
- `CourtAvailabilityPage.tsx` (Booking flow)
- `CustomerBookingsPage.tsx` (Pesanan Saya)
- `CustomerBookingDetailPage.tsx` (Detail Pesanan/Payment Proof)

**Phase 4: Jalur Operasional Pemilik (Owner)**
- `OwnerDashboardPage.tsx`
- `OwnerVenuesPage.tsx`
- `CreateVenuePage.tsx`
- `OwnerCourtsPage.tsx` (Manajemen lapangan)
- `OwnerVenueBookingsPage.tsx` (Verifikasi pembayaran dan pesanan masuk)
- Modals terkait pemilik: `CourtModal`, `OperatingHoursModal`, `BlockedSlotsModal`.

**Phase 5: Verifikasi Akhir (Final QA)**
- Menjalankan skrip `node scripts/smoke_test.js` untuk E2E flow.
- Pengujian QA manual lintas alur (Customer dan Owner).
