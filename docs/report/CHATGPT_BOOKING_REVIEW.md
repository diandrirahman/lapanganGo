# Review & Rencana Implementasi: Booking Flow API (MVP)

Konteks Repo Saat Ini:
- Fitur dasar (Auth, Venues, Courts, Schedules, Blocked Slots) sudah rampung.
- Migration untuk bookings (`004_bookings.sql`) sudah tersedia.
- Security fixes, CORS, dan Graceful Shutdown sudah berjalan di master.
- Binary file sudah di-ignore dengan aman.

## 1. Apakah booking flow memang step paling tepat berikutnya?
**Status:** `PASS`.
Ya, pengembangan Booking Flow API adalah step paling natural selanjutnya. Semua persyaratan struktural untuk membuat transaksi (user, lapangan, waktu, jadwal) sudah lengkap.

## 2. File/module apa saja yang perlu dibuat atau diubah?
Akan dibuat modul baru di dalam `apps/api/internal/bookings`:
- `dto.go`: Struktur data *request* dan *response*.
- `repository.go`: Eksekusi DB, `INSERT` data pemesanan, dan pengecekan bentrok jadwal.
- `service.go`: Pusat validasi *business logic* (cek ketersediaan, jam buka, kalkulasi harga).
- `handler.go`: Penerima *request* HTTP dan parser JSON.
- Penambahan *routing* di `apps/api/cmd/api/main.go`.

## 3. Endpoint minimal MVP
1. `POST /bookings` (Membuat pemesanan baru)
2. `GET /bookings` (Daftar pemesanan milik customer terkait)
3. `GET /bookings/:id` (Detail sebuah pemesanan)

## 4. Validasi yang wajib ada
Seluruh *logic* berikut akan dijalankan di *Service Level*:
- **Auth Customer:** Di-intercept otomatis oleh middleware bawaan sistem.
- **Court & Venue ACTIVE:** Verifikasi status lapangan dan stadion tujuan harus `"ACTIVE"`.
- **Tanggal/Jam Valid:**
  - Waktu mulai (`start_time`) < waktu selesai (`end_time`).
  - Pemesanan tidak bisa di tanggal masa lalu.
- **Operating Hours:** Pengecekan interval terhadap tabel `schedules`.
- **Blocked Slots:** Pengecekan apakah jam yang dipilih sedang terkena *maintenance* (ada di `blocked_slots`).
- **Overlapping Booking:** Mengecek apakah jam yang diminta sudah terpesan oleh *customer* lain dengan `status != CANCELLED`.
- **Total Price Server-Side:** Harga total dihitung murni di sisi *server* berdasarkan `price_per_hour` dari tabel `courts` x durasi sewa, untuk mencegah *fraud*.

## 5. Status booking untuk MVP
- Awal buat: `PENDING_PAYMENT`
- Lunas: `PAID` (bisa menggunakan mock endpoint pembayaran `/bookings/:id/pay`)
- Batal: `CANCELLED`

## 6. Test case yang wajib dibuat
1. Berhasil memesan (Skenario utama).
2. Gagal: Tumpang-tindih (overlap) jam dengan pemesan lain.
3. Gagal: Memesan lapangan di luar jam operasional.
4. Gagal: Memesan di tanggal/waktu yang diblokir oleh admin.
5. Gagal: Memesan lapangan untuk jadwal kemarin (past time).

## 7. Risiko Teknis & Proteksi Tambahan
**Risiko:** Terjadinya *Double Booking* / *Race Condition*. Jika 2 pengguna memesan 1 jadwal secara persis bersamaan, *query* validasi bisa kebobolan di level *Service*.

**Rekomendasi:**
Untuk mencegahnya, kita memiliki 2 pendekatan:
1. **Solusi DB (Terkuat):** Membuat migration baru `005_booking_constraints.sql` untuk menambahkan fitur PostgreSQL Extension `btree_gist` dan melakukan `EXCLUDE` constraint rentang waktu.
2. **Solusi Service:** Mengunci baris menggunakan Transaction Level `SERIALIZABLE` atau `FOR UPDATE`.
