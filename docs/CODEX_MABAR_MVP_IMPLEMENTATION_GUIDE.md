# Panduan Implementasi MVP Open Match / Mabar

**Tujuan Dokumen:**  
Dokumen ini merupakan hasil kesimpulan diskusi dan finalisasi arsitektur antara User dan Antigravity. Dokumen ini diserahkan kepada Codex sebagai panduan utama untuk mengeksekusi kode (*Backend* & *Frontend*) fitur "Cari Lawan / Open Match / Mabar" di aplikasi LapanganGo.

---

## 1. Keputusan Alur Bisnis (MVP Logic)

Untuk menghindari celah teknis seperti *double-booking* lapangan, alur MVP yang disepakati adalah sebagai berikut:
1. **Sumber Jadwal**: Host harus melakukan *booking* lapangan reguler terlebih dahulu hingga berstatus sukses/terbayar.
2. **Buka Mabar**: Host mengubah/mendaftarkan *booking* miliknya tersebut menjadi "Open Match".
3. **Bergabung**: Pengguna lain (Participant) dapat langsung menekan tombol *Join* (Auto-Join tanpa perlu *approval* dari Host).
4. **Pembayaran**: Untuk MVP, Host menanggung biaya penuh ke aplikasi. Peserta melakukan patungan/pembayaran secara **informal di luar sistem** (misal bayar tunai di lapangan).
5. **Keluar/Batal**: Peserta dapat menekan tombol *Leave* jika batal ikut. Host dapat menekan tombol *Cancel* untuk membatalkan Mabar (namun ini tidak otomatis membatalkan *booking* lapangan utama).

---

## 2. Rencana Arsitektur & Database

Pembuatan modul ini **harus dipisah** dari modul `bookings` untuk menjaga *Clean Architecture*. Buat modul baru di `apps/api/internal/mabar`.

### A. Tabel Database Baru
1. **`open_matches`**
   - `id` (UUID, PK)
   - `booking_id` (UUID, FK, Unique)
   - `host_user_id` (UUID, FK)
   - `title` (VARCHAR 100)
   - `description` (TEXT)
   - `level` (VARCHAR 50) -> Enum terbatas: `Beginner / Fun`, `Intermediate`, `Advanced`, `All Levels`.
   - `max_players` (INTEGER)
   - `price_per_player` (NUMERIC)
   - `status` (VARCHAR 50) -> Enum: `OPEN`, `FULL`, `CANCELLED`, `COMPLETED`
   - `created_at`, `updated_at`

2. **`open_match_participants`**
   - `id` (UUID, PK)
   - `open_match_id` (UUID, FK)
   - `user_id` (UUID, FK)
   - `status` (VARCHAR 50) -> Enum: `JOINED`, `CANCELLED`
   - *Constraint*: `UNIQUE(open_match_id, user_id)`

### B. Kebutuhan API Endpoints
1. `GET /api/v1/open-matches` (Public/Auth) -> List mabar yang `OPEN`.
2. `GET /api/v1/open-matches/:id` (Public/Auth) -> Detail mabar + partisipan.
3. `POST /api/v1/bookings/:id/open-matches` (Auth) -> Host membuat mabar.
4. `POST /api/v1/open-matches/:id/join` (Auth) -> Join mabar (Gunakan DB Transaction, jika peserta penuh ubah match jadi `FULL`).
5. `DELETE /api/v1/open-matches/:id/join` (Auth) -> Keluar dari mabar.
6. `PATCH /api/v1/open-matches/:id/cancel` (Auth) -> Host membatalkan mabar.

---

## 3. Mitigasi Penipuan (*Fraud & Safety*)

Karena pembayaran patungan dilakukan di luar aplikasi pada fase MVP, kita wajib mencegah *Host* bodong maupun *Peserta* hit & run:

1. **Syarat Pembuatan (Backend)**: Untuk mencegah *Host* penipu, pastikan tombol/API "Buat Open Match" hanya bisa diakses oleh *User* yang memiliki minimal 1 riwayat *Booking* berhasil (atau status *Verified*).
2. **Peringatan UI (Frontend)**: Saat *user* akan menekan tombol "Gabung Match", Frontend wajib menampilkan *Pop-up Warning Banner*: 
   > *"LapanganGo tidak memfasilitasi transaksi di luar aplikasi. Untuk keamanan, selalu lakukan pembayaran/patungan secara langsung saat bertemu di lapangan."*
3. **Fase Selanjutnya (Post-MVP - Info untuk Codex)**: Setelah MVP rilis, fitur ini akan di-upgrade menggunakan skema *Escrow* / In-App Split Payment (Dompet Bersama via Payment Gateway) di mana LapanganGo akan mengambil *Platform Fee* dari tiap transaksi.

---

## 4. Arahan Desain UI/UX (Frontend)

Antigravity dan User telah menyepakati desain visual (tersimpan di `docs/design/antigravity-ui-preview.html`):
- **Tema Visual**: *Light Mode Energik* dengan warna gradasi *Vibrant Orange-Pink*.
- **Tipografi**: Menggunakan font modern geometris (contoh: *Plus Jakarta Sans* atau *Manrope*).
- **Nuansa**: Gaya *Landing Page Dribbble* modern (bersih, banyak *whitespace*, elemen melayang/kaca *glassmorphism*).

**Instruksi untuk Codex:** 
Gunakan dokumen ini sebagai panduan kebenaran (*Single Source of Truth*) saat Anda mulai membuat struktur basis data dan menyusun kode untuk *Backend Go* maupun antarmuka di fase selanjutnya.
