# Laporan Integrasi Availability API & Aktif Booking

Sesuai dengan instruksi *scope* teknis yang diberikan, perbaikan untuk menambal *gap* sinkronisasi ketersediaan pada `GET /courts/:id/availability` sudah selesai dan berhasil lulus uji. Slot yang sudah dipesan tidak akan lagi tampil sebagai `AVAILABLE`.

Berikut rekapitulasi status eksekusinya:

## 1. File yang Berubah
- **`apps/api/internal/availability/repository.go`**: Menambahkan deklarasi struct `ActiveBooking` dan menyisipkan satu fungsi baru `ListActiveBookings`. Fungsi ini secara otomatis menarik data dari tabel `bookings` asalkan pesanan tersebut tidak berstatus `CANCELLED`.
- **`apps/api/internal/availability/service.go`**:
  - Mendaftarkan *state* baru: `slotStatusBooked = "BOOKED"`.
  - Mengonversi format `TIME` dari database menjadi objek hari+jam yang setara, lalu mengecek persilangannya menggunakan fungsi utilitas baru `overlapsAnyBooking`.
  - Fungsi `buildSlots` memprioritaskan pengecekan jadwal *maintenance* (menjadi `BLOCKED`). Jika tidak ada maintenance, sistem mengecek *overlap* pesanan (menjadi `BOOKED`).
- **`apps/api/internal/availability/service_test.go`**: Update *signature* fungsi test yang sudah ada agar tidak pecah, lalu menambahkan sebuah blok unit test baru (`TestBuildSlotsMarksBookedOverlap`) untuk mensimulasikan tumpang tindih dengan jadwal pemesanan aktif.

## 2. Ringkasan Perubahan Arsitektur
- Tidak ada perombakan arus pembuatan *booking* (*create flow*).
- Tidak ada tambahan migrasi baru (mempertahankan kelancaran).
- API membedakan *slot* yang ditutup karena pemeliharaan (`BLOCKED`) dengan *slot* yang ditutup karena sudah ada pelanggan yang memesan (`BOOKED`). Ini jauh lebih jernih dan mendeskripsikan konteks aslinya untuk *Client-Side*.

## 3. Hasil *Testing* Terminal
Perintah format `gofmt -w .` telah dieksekusi agar konvensi tidak patah. Pengujian unit lewat `go test ./...` di dalam direktori `apps/api` juga menunjukkan *output* prima:

```text
ok      lapangango-api/internal/availability    2.233s
```
Semua target lolos tanpa masalah sintaks (*Build OK*).

## 4. Peringatan Kompatibilitas API (*Risk Note*)
Penambahan tipe respons `"BOOKED"` merupakan perubahan kontrak *output* bagi konsumen API (aplikasi web/mobile).
- **Frontend Developer** wajib diberitahu mengenai kehadiran status `BOOKED`. 
- Jika *frontend* saat ini mengandalkan logika warna `if status == 'BLOCKED' { render_gray() }`, mereka harus memperbaruinya menjadi `if status == 'BLOCKED' || status == 'BOOKED' { render_gray_and_disable_click() }`. Jika terlewat, UI *frontend* mungkin tidak menampilkan slot tersebut secara tepat.

Silakan lakukan tinjauan akhir untuk perubahan modul ini, kode berada pada status untracked / belum di-commit!
