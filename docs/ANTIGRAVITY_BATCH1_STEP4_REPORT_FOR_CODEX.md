# Report Antigravity - Batch 1A, Step 4 (Fix VenueCard Hardcoded/Misleading Data)

**To:** Codex
**From:** Antigravity
**Task:** Step 4 - Fix VenueCard Hardcoded/Misleading Data

## 1. File yang Diubah
- `apps/web/src/components/VenueCard.tsx`
- `apps/web/src/types/booking.ts` (Minor type update terkait build error dari step sebelumnya)

## 2. Ringkasan Perubahan
- **VenueCard**: 
  - Menghapus badge statis/palsu `Sedang Ramai`.
  - Menghapus daftar _time slots_ dekoratif (`07:00`, `08:00`, `09:00`, dst) yang membingungkan user.
  - Memperbarui label harga yang sebelumnya _hardcoded_ `Rp 150K / Jam` menjadi dinamis. Harga kini diambil dari array `venue.courts`. Jika `courts` tersedia dan valid, harga akan dihitung untuk menemukan _minimum_ dan _maximum_. 
    - Jika `min === max`, harga ditampilkan sebagai `Rp x / Jam`.
    - Jika `min !== max`, harga ditampilkan sebagai range `Rp min - Rp max / Jam`.
    - Jika data harga (atau `courts`) tidak tersedia (misal di halaman yang hanya mengembalikan rangkuman venue tanpa courts), maka card akan menampilkan fallback jujur yaitu `"Harga belum tersedia"`.
- **Booking Types**: Menambahkan `WAITING_VERIFICATION` dan field `payment_reference` ke dalam type frontend supaya `npm run build` sukses sepenuhnya (efek domino dari penambahan field di Step 2).

## 3. Cara Testing
1. **Automated Testing**:
   Menjalankan frontend build TypeScript checker dan Vite build.
   ```powershell
   cd apps/web
   npm run build
   ```
   **Hasil**: Kompilasi berhasil dan Vite memproduksi folder `dist` tanpa error (`built in xxx ms`).

2. **Manual Verification**:
   - Tampilan VenueCard sekarang lebih lega tanpa ornamen-ornamen statis yang menipu mata.
   - User melihat harga nyata jika API mereturn `courts`, dan melihat pesan "Harga belum tersedia" pada summary API (hingga nanti API ListPublicVenues dilengkapi dengan min/max aggregation).
   - Layout tetap rapi di mode grid desktop maupun stack mobile.

## 4. Risiko atau Catatan Lanjutan
- **Aman**: Hanya merubah presentasi komponen UI (presentational component).
- Catatan: Saat ini endpoint `ListPublicVenues` dari backend (yang dipakai halaman home dan explore) belum meng-embed _courts_ ataupun agregasi `min_price/max_price` di dalamnya. Sehingga di halaman utama, UI akan menampilkan "Harga belum tersedia". Ini adalah langkah pertama yang jujur. Jika Codex/tim ingin menampilkan agregasi harga di masa depan, query di repository backend dapat di-extend tanpa merusak komponen `VenueCard` yang baru ini.

---
Silakan direview, dan saya siap lanjut ke step berikutnya di Batch 1A!
