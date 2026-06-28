# Report Antigravity - Batch 2, Step 12 (Extract Shared Frontend Utils)

**To:** Codex
**From:** Antigravity
**Task:** Step 12 - Extract Shared Frontend Utils

## 1. File yang Diubah
- `apps/web/src/lib/utils.ts` (Ditambahkan fungsi utilitas baru)
- `apps/web/src/pages/CustomerBookingDetailPage.tsx`
- `apps/web/src/pages/CustomerBookingsPage.tsx`
- `apps/web/src/pages/MabarDetailPage.tsx`
- `apps/web/src/pages/owner/OwnerVenueBookingsPage.tsx`
- `apps/web/src/components/MabarCard.tsx`
- `apps/web/src/components/owner/BlockedSlotsModal.tsx`
- `apps/web/src/components/VenueCard.tsx`
- `apps/web/src/pages/VenueDetailPage.tsx`

## 2. Ringkasan Implementasi
- **Pemindahan Helper Functions**:
  - Mengisolasi dan mengekspor fungsi formatter mata uang ke `formatRupiah` dalam `lib/utils.ts`.
  - Memusatkan format tanggal dasar (yang berulang di 5 _file_ berbeda) ke `formatDate` dalam `lib/utils.ts`.
  - Memusatkan format tanggal dengan waktu / jam (seperti yang ada di `BlockedSlotsModal`) ke dalam `formatDateTime` di `lib/utils.ts`.
  - Menyederhanakan penunjukan _placeholder_ URL gambar (serta mekanisme `hashString` di baliknya) ke `getPlaceholderImage` di dalam `lib/utils.ts`.
- **Refactoring Komponen**:
  - Telah menghapus deklarasi lokal fungsi-fungsi tersebut secara massal dari masing-masing komponen _frontend_.
  - Komponen tersebut kini hanya mengimpor fungsi eksternal dari `lib/utils.ts` yang membuat struktur _file_ menjadi jauh lebih ringkas (*DRY: Don't Repeat Yourself*).
- **Keamanan Perilaku Visual (Zero UI Changes)**:
  - Gaya _output string_ yang dirender (seperti jam, kapitalisasi tanggal, letak `Rp`, presisi angka) dipertahankan mutlak dan secara eksplisit tidak berubah mengikuti arahan _task_. Pada `MabarCard`, format waktu "Hari Ini" tetap menggunakan logika internalnya untuk menjaga perilaku visual spesifik yang tidak tumpang-tindih dengan formatter global biasa.

## 3. Cara Testing
1. **Automated Build**:
   ```bash
   cd apps/web
   npm run build
   ```
   **Hasil**: Kompilasi Vite dan tipe data TypeScript tervalidasi 100% (**PASS** `✓ built in 386ms`).
2. **Review Kode**:
   - Anda kini dapat mengecek `apps/web/src/lib/utils.ts` untuk menemukan kumpulan fungsi utilitas utama yang telah dibersihkan.

---
Pemeliharaan dan perampingan komponen pada Step 12 sudah dirampungkan. Menunggu instruksi untuk tahapan berikutnya!
