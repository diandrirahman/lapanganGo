# Prompt Revisi Visual Frontend Step 1 Mabar

Halo Antigravity,

Codex sudah membandingkan screenshot prototype design dengan hasil frontend saat ini.

File referensi:

```text
Prototype design: docs/design/antigravity-ui-preview.html
Current frontend: apps/web
```

Kesimpulan:

```text
Frontend Step 1 secara fungsi sudah approved, tetapi perlu revisi visual/layout sebelum lanjut Step 2.
```

## Masalah Utama

### 1. Tampilan Sekarang Tidak Mirip Prototype Karena Masuk Empty State

Prototype menampilkan 3 card Mabar aktif.

Frontend saat ini menampilkan:

```text
Belum Ada Jadwal Mabar
```

Ini kemungkinan terjadi karena `GET /open-matches` mengembalikan array kosong dari database lokal.

Secara logic ini benar, tetapi untuk review visual, kita perlu membuktikan tampilan populated state juga mirip prototype.

Arahan:

- Jalankan frontend dengan backend yang punya minimal 3 open match aktif, atau gunakan data seed/dev yang jelas hanya untuk kebutuhan visual QA.
- Jangan menjadikan hardcoded dummy data sebagai sumber utama production.
- Jika memakai fallback/dev mock, beri guard yang jelas, misalnya hanya aktif saat env khusus:

```text
VITE_USE_MOCK_MABAR=true
```

atau lebih baik gunakan seed backend agar tetap memakai API nyata.

Target visual populated state:

- Header gelap besar di atas.
- Card Mabar overlap/menjorok naik ke bawah header seperti prototype.
- Minimal 3 card pada desktop.
- Spacing, card radius, badge slot, dan tombol mengikuti feel prototype.

### 2. Footer Tidak Menempel ke Bawah Viewport

Pada screenshot current, footer muncul di tengah bawah lalu masih ada area kosong besar di bawahnya.

Ini biasanya terjadi karena struktur page belum memakai full-height layout.

Arahan teknis:

- Pastikan root layout memakai minimal tinggi layar penuh:

```tsx
<div className="min-h-screen flex flex-col bg-bg-main">
  <Navbar />
  <main className="flex-1 ...">
    ...
  </main>
  <footer>...</footer>
</div>
```

- Footer harus terdorong ke bawah saat konten pendek.
- Saat konten panjang, footer tetap muncul setelah content normal, bukan fixed.

Acceptance:

- Pada empty state, footer berada di bawah viewport, tidak menggantung di tengah halaman.
- Pada populated state, footer berada setelah section card dengan spacing yang rapi.

### 3. Empty State Terlalu Besar dan Terlalu Dekat Dengan Header

Current empty state tampil sebagai panel putih besar melebar, tapi tidak membawa karakter visual prototype.

Arahan:

- Empty state boleh tetap ada, tetapi jangan merusak komposisi.
- Buat empty state lebih compact dan tetap align dengan grid/card system.
- Pertahankan overlap dengan header secara elegan, tetapi jangan membuat card/panel terasa seperti banner tunggal terlalu besar.

### 4. Prototype Visual Detail Yang Perlu Didekatkan

Saat data populated, card harus lebih dekat dengan prototype:

- Card muncul dalam grid 3 kolom desktop.
- Card overlap sedikit ke header gelap.
- Badge slot berwarna merah/oranye gradient.
- Tombol `Gabung Match` full-width di bawah card.
- Info row rapi: label kiri, value kanan.
- Location sebaiknya menampilkan `venue_name` dan jika muat `court_name`.
- Avatar placeholder boleh tetap initial host, tapi styling-nya harus mirip avatar circle prototype.

Catatan: Tidak perlu mengembalikan emoji kalau berisiko mojibake. Jika ingin ada ikon api di judul, gunakan icon valid atau text-only saja.

## Scope Revisi

Kerjakan hanya:

```text
Frontend Step 1 Visual/Layout Revision
```

Jangan kerjakan:

- Detail page.
- Join API.
- Leave API.
- Cancel API.
- Create Open Match form.
- Auth flow.
- Payment participant.
- Backend schema changes.

## Verifikasi Wajib

Setelah revisi, jalankan:

```text
npm.cmd run lint
npm.cmd run build
```

Lalu kirim report dengan:

1. Screenshot/penjelasan populated state minimal 3 card.
2. Screenshot/penjelasan empty state.
3. Bukti footer sudah berada di bawah viewport saat empty state.
4. File yang diubah.
5. Hasil lint/build.
6. `git status --short`.

## Acceptance Criteria

Codex akan approve visual Step 1 jika:

1. Populated state terlihat dekat dengan prototype.
2. Empty state tetap rapi dan tidak terasa seperti bug layout.
3. Footer tidak menggantung di tengah halaman saat konten pendek.
4. Tidak ada mojibake/karakter rusak.
5. Tidak ada hardcoded production data.
6. Tidak ada scope creep ke Step 2/3.
