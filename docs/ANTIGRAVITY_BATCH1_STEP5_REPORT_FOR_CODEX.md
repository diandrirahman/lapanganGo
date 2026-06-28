# Report Antigravity - Batch 1A, Step 5 (Make Hero Search Functional)

**To:** Codex
**From:** Antigravity
**Task:** Step 5 - Make Hero Search Functional

## 1. File yang Diubah
- `apps/web/src/components/HeroSection.tsx`

## 2. Ringkasan Perubahan
- **Mengaktifkan Fungsi Search**: 
  - Mengonversi elemen search bar dari `div` biasa menjadi tag `<form>` dan meng-handle submit lewat event `onSubmit`.
  - Menghapus atribut `disabled` dari elemen `input`, `select`, dan tombol `Search`.
  - Mengaitkan state internal (`searchQuery` dan `sport`) dengan field input dan dropdown.
  - Memastikan user di-redirect (`navigate`) ke `/venues` menggunakan format URL parameter `?search=<query>&sport=<sport>`.
- **Mencegah Eksekusi Kosong**: 
  - Jika input query dan dropdown sport kosong saat user menekan enter, tidak akan ada form submit yang dieksekusi, sehingga URL tetap bersih (tidak nge-link ke `?`).
- **Mengaktifkan Chips Olahraga**: 
  - Menjadikan chips (Mini Soccer, Tenis, dll) dari sebelumnya sekadar visualisasi menjadi `<button>` interaktif.
  - Menghapus atribut seperti `cursor-not-allowed` dan styling kepudaran (`opacity-60`), menggantinya dengan gaya hover interaktif.
  - Menambahkan click handler `handleChipClick` untuk melompat langsung ke `/venues?sport=<kategori>`.

## 3. Cara Testing
1. **Automated Testing**:
   Menjalankan ulang command vite build:
   ```powershell
   cd apps/web
   npm run build
   ```
   **Hasil**: Kompilasi berhasil 100% dan file aset siap (0 TypeScript errors).

2. **Manual Verification**:
   - Di halaman awal (home), user kini bisa mengetik nama kota atau lapangan (misal "Jakarta"). Tekan enter, lalu user akan dialihkan ke halaman filter venue.
   - Pilihan kategori dropdown berfungsi selayaknya filter.
   - Mengklik label sport yang ada di bawah kotak pencarian akan mempercepat proses menuju list venue untuk olahraga spesifik tersebut.

## 4. Risiko atau Catatan Lanjutan
- **Aman**: Hanya murni perbaikan fungsionalitas di lingkup `HeroSection` dan routing. Routing parameter sudah mengikuti standar querystring bawaan yang nantinya bisa ditangkap oleh list halaman filter Venue.
- _Note_: Saat ini endpoint atau halaman explore harus sudah punya logika untuk menangkap variable dari `URLSearchParams` agar filternya berfungsi optimal di sisi display (di luar scope step 5).

---
Silakan direview, dan saya siap untuk step berikutnya (Step 6)!
