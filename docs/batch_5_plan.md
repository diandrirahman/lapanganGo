# Batch 5 Implementation Plan: UX Polish & Code Quality (Final Polish)

Batch 5 akan berfokus pada penyelesaian isu-isu minor terkait _User Experience_ (UX), perbaikan kualitas kode TypeScript di _frontend_, dan penghapusan sisa *technical debt* yang diidentifikasi pada ulasan awal (*review baseline*). Ini akan menjadi fase akhir sebelum proyek ini dianggap siap rilis penuh.

## Open Questions untuk Codex
- **Occupancy Rate**: Pada metrik _Owner Dashboard_, *occupancy rate* (tingkat keterisian) saat ini di-*hardcode* menjadi `75%`. Apakah sebaiknya kami mengimplementasikan algoritma untuk menghitung rasio jumlah slot ter-booking berbanding total slot yang tersedia (ini butuh kalkulasi operasional yang cukup kompleks), atau cukup kembalikan nilai `0` (atau omit *UI*) untuk sementara jika itu membebani penyelesaian MVP?

## Proposed Changes

### 1. Frontend: Type Safety & API Client Polish (`apps/web/src/lib/api.ts`)
- **Penghapusan tipe `any`**: Mengganti semua tipe kembalian (seperti `fetchSports`, `fetchFacilities`, `createOwnerVenue`, `fetchOwnerMetrics`, dll.) dari `Promise<any>` menjadi *interface* yang konkret dan ketat.
- **Tangkapan Error yang Aman**: Mengubah blok `catch (err: any)` yang tersebar di `api.ts` menjadi pendekatan yang *type-safe* menggunakan `catch (err: unknown)` dan `if (err instanceof Error)`.
- **Ekspansi Model**: Mendefinisikan antarmuka TypeScript untuk sumber daya yang masih kurang terdefinisi, seperti `Sport`, `Facility`, `OwnerMetrics`, dan `BlockedSlot`.

### 2. Frontend: UX Feedback (Toast Notifications)
- **Sistem Notifikasi Global**: Saat ini banyak aksi yang berhasil/gagal secara diam-diam tanpa masukan nyata dari sistem (misal: saat sukses _login_, membuat _venue_, membatalkan pemesanan). Rencana pelaksanaannya adalah dengan menambahkan modul *Toast notification* (menggunakan _library_ ringan seperti `react-hot-toast` atau dengan komponen buatan sendiri + *Context*) untuk memberikan _feedback_ status pada pengguna.

### 3. Frontend: Pembersihan Dead Code
- Menelusuri sisa kode yang tidak pernah dieksekusi, termasuk variabel tidak terpakai dan berkas komponen seperti `Select.tsx` yang kemungkinan dibiarkan dari fase *layouting* tahap awal, demi menekan ukuran bundel aplikasi.

### 4. Backend: Perbaikan Minor Analitik (`apps/api/internal/bookings/repository.go`)
- Memperbaiki perhitungan *Occupancy Rate* pada fungsi `GetOwnerMetrics`. Akan dikalkulasikan dengan logika rasio berdasarkan _Upcoming Bookings_ atau di-_fallback_ secara moderat berdasarkan persetujuan dari umpan balik bagian *Open Questions* di atas.
- Memverifikasi kolom *selection* seperti `PaymentReference` di fungsi-fungsi kueri `ListByCustomerID` agar selaras dengan skema final di _database_.

---

## Verification Plan

### Automated Tests
- `npm run lint` untuk memastikan peringatan _implicit any_ (`no-explicit-any`) tidak muncul kembali.
- `npm run build` untuk memverifikasi bahwa perubahan struktur *tipe data* kompatibel dengan komponen *React* di seluruh proyek tanpa kegagalan saat *build-time*.
- `go test ./...` dijalankan guna memastikan perubahan analitik pada `GetOwnerMetrics` tidak merusak cakupan uji (*test coverage*) milik sub-sistem internal _booking_.

### Manual Verification
- Uji alur _happy path_ dari pendaftaran/login, pembuatan _venue_, sampai proses pemesanan dengan memastikan respons antarmuka (pesan _Toast_) muncul setiap usai menekan tombol _submit_.
