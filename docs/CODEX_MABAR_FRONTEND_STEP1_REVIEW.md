# Review Codex: Frontend Step 1 Open Match Discovery

Halo Antigravity,

Codex sudah mereview laporan:

```text
docs/CODEX_MABAR_FRONTEND_STEP1_REPORT.md
```

Dan mengecek implementasi di:

```text
apps/web
```

Hasil verifikasi command:

```text
npm.cmd run lint  -> PASS
npm.cmd run build -> PASS
```

Namun secara review produk dan UX, Step 1 belum saya approve penuh.

Keputusan:

```text
REQUEST CHANGES
```

## Yang Sudah Baik

- Frontend dibuat di lokasi yang benar: `apps/web`.
- Stack sesuai arahan: Vite + React + TypeScript + Tailwind v4.
- Tidak menambahkan routing dependency.
- API client sudah memakai `VITE_API_BASE_URL` dengan fallback `http://localhost:8080`.
- Endpoint yang dipakai benar: `GET /open-matches`, bukan `/api/v1/open-matches`.
- Komponen sudah dipisah: `MabarSection`, `MabarCard`, `Navbar`, `api.ts`, dan `types/mabar.ts`.
- Loading, empty, error, dan populated state sudah tersedia.
- Build dan lint lulus.

## Finding 1: Karakter UI Rusak / Mojibake

Prioritas:

```text
P1 - wajib diperbaiki sebelum approval
```

Ada beberapa teks/ikon di UI yang tampil sebagai karakter rusak:

```text
apps/web/src/components/MabarSection.tsx
apps/web/src/components/Navbar.tsx
```

Contoh yang terlihat di source:

```text
Cari Lawan / Open Match ðŸ”¥
âš ï¸
ðŸŸï¸
âš¡
```

Ini akan terlihat tidak profesional di UI, terutama karena Step 1 adalah halaman discovery pertama yang dilihat user.

Arahan:

- Hapus karakter rusak tersebut.
- Jika butuh ikon, gunakan teks biasa atau bentuk sederhana berbasis CSS/lucide/icon library yang valid.
- Jangan pakai karakter mojibake.
- Untuk MVP, aman juga jika tanpa emoji sama sekali.

## Finding 2: Masih Ada Sisa Template Vite Yang Tidak Dipakai

Prioritas:

```text
P2 - cleanup sebelum approval
```

File berikut masih berisi/menyimpan artifact bawaan template:

```text
apps/web/src/App.css
apps/web/src/assets/vite.svg
apps/web/src/assets/react.svg
```

`App.css` berisi style template seperti `.counter`, `.hero`, `.vite`, dan lain-lain, tetapi tidak dipakai oleh UI Mabar.

Arahan:

- Hapus file CSS/template asset yang tidak dipakai.
- Pastikan tidak ada import yang rusak setelah cleanup.
- Project frontend harus terlihat sengaja dibuat untuk LapanganGo, bukan masih menyimpan sisa scaffold.

## Finding 3: Dekorasi Background/Blur Terlalu Generik

Prioritas:

```text
P2 - polish UX
```

Ada beberapa dekorasi seperti:

```text
apps/web/src/index.css -> .bg-mesh radial-gradient background
apps/web/src/components/MabarCard.tsx -> absolute rounded blur decorative element
```

Ini tidak fatal secara fungsi, tetapi arahan desain kita sebelumnya meminta UI yang lebih clean dan domain-focused, bukan banyak efek blur/orb generik.

Arahan:

- Kurangi atau hapus background mesh radial.
- Hapus decorative blur element di card.
- Biarkan visual strength datang dari layout, typography, spacing, border, badge, dan card content.

## Finding 4: Tombol Gabung Match Masih Memakai alert()

Prioritas:

```text
P3 - tidak blocker, tapi sebaiknya dirapikan
```

Saat ini action button memakai:

```text
alert('Fitur Join belum tersedia untuk fase ini.')
```

Untuk Step 1, join flow memang belum wajib. Namun `alert()` terasa kasar untuk UI produk.

Arahan:

- Lebih baik jadikan tombol disabled/placeholder state dengan label jelas.
- Alternatif: tampilkan inline hint kecil, bukan browser alert.
- Jangan implementasi API join dulu.

## Acceptance Untuk Revisi

Step 1 akan saya approve jika:

1. Tidak ada karakter mojibake di UI.
2. Sisa Vite template yang tidak dipakai sudah dibersihkan.
3. Dekorasi blur/orb generik dikurangi atau dihapus.
4. `npm.cmd run lint` lulus.
5. `npm.cmd run build` lulus.
6. Tidak ada scope creep ke detail/join/create/cancel.

## Keputusan Next

Jangan lanjut Step 2 dulu.

Kerjakan:

```text
Frontend Step 1 Polish & Cleanup
```

Setelah revisi selesai, kirim kembali:

```text
docs/CODEX_MABAR_FRONTEND_STEP1_REPORT.md
git status --short
npm.cmd run lint output
npm.cmd run build output
```
