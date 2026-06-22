# Proposal Pengembangan Lanjutan: LapangGo Backend API

Karena percakapan ini berada pada sesi (thread) obrolan yang baru, berikut adalah ringkasan konteks terakhir proyek kita:
- Modul-modul utama sudah sukses diimplementasikan dan diverifikasi dengan status `PASS`.
- Fitur yang sudah tersedia meliputi: Otentikasi (JWT), Manajemen Venue & Lapangan untuk *Owner*, Manajemen Jadwal Buka & *Blocked Slots*, Pencarian Lapangan secara Publik, Modul Ketersediaan (*Availability*), hingga MVP **Booking Flow API**.
- Seluruh mitigasi keamanan N+1 Query, CORS, Graceful Shutdown, dan proteksi *Anti Double-Booking* berbasis `FOR UPDATE` *(Row-Level Locking)* sudah diterapkan utuh pada `master`.

---

## Opsi Langkah Selanjutnya (Next Steps)

Untuk melengkapi fungsionalitas MVP aplikasi penyewaan lapangan ini secara *end-to-end*, ada empat celah fitur yang direkomendasikan untuk dieksekusi berikutnya. Tolong berikan arahan, prioritas mana yang sebaiknya kita kerjakan:

### 1. Payment Flow (Dummy / Mock API)
Pemesanan saat ini mandek pada status `PENDING_PAYMENT`.
- **Target**: Membuka endpoint simulasi semacam `POST /bookings/:id/pay` yang akan mengubah status database menjadi `PAID` / `CONFIRMED`.

### 2. Fitur Pembatalan Pelanggan (Cancellation API)
Sistem belum mengizinkan pemesanan untuk dibatalkan.
- **Target**: Membuat endpoint `PATCH /bookings/:id/cancel` bagi *Customer* agar status pesanan berubah menjadi `CANCELLED` (melepas kunci waktu lapangan).

### 3. Owner Dashboard API (Booking Management)
*Customer* bisa memesan, namun *Owner* (pengelola) saat ini buta dan belum punya Endpoint untuk melacak siapa saja pelanggan yang akan bermain di lapangan mereka hari ini.
- **Target**: Endpoint terproteksi khusus role *Owner*, contoh: `GET /owners/venues/:id/bookings`.

### 4. Background Job: Auto-Cancel Expired Bookings
Mencegah kerugian pemilik jika *customer* tidak bayar.
- **Target**: Pekerjaan latar belakang (*cron* / goroutine) yang terus mendeteksi umur pesanan `PENDING_PAYMENT`. Bila melewati 30 menit atau sekian jam, pesanan otomatis berstatus `CANCELLED`.

Tolong evaluasi konteks ini dan instruksikan fitur nomor berapa yang harus kami kerjakan sekarang beserta arsitektur permintaannya!
