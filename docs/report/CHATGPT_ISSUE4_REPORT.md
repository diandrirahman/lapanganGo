# Laporan Penyelesaian Issue 4: Implementasi Graceful Shutdown

**Status:** Selesai (Fixed)

## Penjelasan Masalah
Pada metode sebelumnya, aplikasi dijalankan murni menggunakan `r.Run(":" + cfg.AppPort)` dari *library* Gin. Ini menyebabkan *server* berjalan secara *blocking* mutlak. Ketika kita (atau sistem CI/CD) mencoba menghentikan aplikasi (misalnya saat proses pembaruan versi *deploy*), *server* akan mati dalam hitungan milidetik secara paksa. Hal ini sangat berisiko karena semua koneksi *client* dan kueri *database* yang sedang berlangsung akan seketika *error*.

## Tindakan Perbaikan

1. **Memindahkan ke `http.Server`:**
   - Metode `r.Run()` dihentikan penggunannya.
   - Menggunakan instansi `http.Server` agar kontrol eksekusinya bisa dilakukan secara lebih presisi melalui Goroutine terpisah (`go func()`).

2. **Deteksi Sinyal OS (OS Signals):**
   - Menginjeksi *channel* untuk mendeteksi perintah penghentian (interrupt/kill) dari sistem operasi melalui `syscall.SIGINT` dan `syscall.SIGTERM`.
   - Ketika *container* Docker atau OS ingin mematikan aplikasi, aplikasi akan menerima sinyal ini dan bersiap mematikan diri secara aman (*intercept*).

3. **Injeksi Timeout dan `Shutdown`:**
   - Setelah sinyal terminasi ditangkap, `srv.Shutdown()` dipanggil dengan `context.WithTimeout` (durasi tunggu maksimal 5 detik).
   - Selama durasi *timeout* tersebut, aplikasi berhenti menerima permintaan (HTTP Request) yang baru, tetapi memberikan kesempatan (jeda toleransi waktu) agar permintaan yang sedang berjalan dapat diselesaikan terlebih dahulu. 
   - Akhirnya, *pool database* dipastikan ditutup dengan baik.

Aplikasi sekarang sudah "tahan banting" dan aman di-*restart* tanpa takut membuat aliran transaksi pelanggan *corrupt* mendadak!

---
*(Laporan ini digunakan sebagai catatan log penyelesaian bug untuk AI Agent atau dokumentasi tim)*
