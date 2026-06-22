# Laporan Implementasi: Fitur Cancellation API

Tugas untuk memfasilitasi pembatalan (*cancellation*) pemesanan oleh pelanggan telah berhasil direalisasikan secara menyeluruh sesuai dengan seluruh _Business Rules_ (aturan bisnis) yang ditetapkan. API kini memiliki perlindungan *state-machine* dan isolasi vertikal berdasarkan pemilik pesanan.

Berikut ringkasan eksekusi *patch*-nya:

## 1. Arsitektur Perubahan File
- **`apps/api/internal/bookings/repository.go`**: Menambahkan deklarasi metode `UpdateBookingStatus(ctx, id, status)` untuk mengamankan modifikasi *state* tabel *bookings* yang hanya dilakukan spesifik pada parameter UUID target, dengan mencatat pembaruan waktu `updated_at`.
- **`apps/api/internal/bookings/service.go`**: 
   - Disuntikkan logika _sentinel errors_ seperti `ErrBookingAlreadyCancelled` dan `ErrBookingCannotBeCancelled`.
   - Mengimplementasikan `CancelBooking(ctx, customerID, bookingID)`. Sistem terlebih dahulu memvalidasi keabsahan data pesanan dengan ID pelanggan secara bersamaan, mengecek syarat kelayakan bahwa status harus murni bernilai `"PENDING_PAYMENT"`, lalu mengeksekusi _update_ status menjadi `"CANCELLED"`.
- **`apps/api/internal/bookings/handler.go`**:
   - Memetakan fungsi pembatalan ke *route* baru: `group.PATCH("/:id/cancel", h.CancelBooking)`.
   - Menambahkan pengaman validasi format `isValidUUID` (return *400 Bad Request*).
   - Melakukan konversi respons dari `ErrBookingAlreadyCancelled` dan `ErrBookingCannotBeCancelled` menjadi kode asali yang sesuai, yaitu *409 Conflict*.
- **`apps/api/internal/bookings/service_test.go`**: Pengembangan total pada blok *Mock* repo untuk melegalkan unit test yang mencakup uji skenario gagal-sukses (`TestCancelBooking_Success`, dll).
- **`README.md`**: Menambahkan pemetaan panduan API `/bookings/:id/cancel`.

## 2. Penuhan Aturan Integrasi Kesisteman
- API memastikan baris jadwal (*row bookings*) murni **tidak dihapus dari DB**, dan perlindungan *double-booking* orisinal tidak disentuh. 
- Saat pelanggan membatalkan jadwal mereka, proses sinkronisasi dengan kueri `GET /courts/:id/availability` akan langsung berjalan mulus di latar belakang (*query* milik *availability* mengabaikan status *cancelled*), yang alhasil mengembalikan jadwal menjadi `"AVAILABLE"` tanpa insiden teknis maupun perlunya migrasi.

## 3. Laporan Kelulusan Automasi
Proses pengetesan penuh `go test ./...` dalam lingkup folder API sukses besar di mana modul _bookings_ lolos mulus:
```text
ok      lapangango-api/internal/bookings        2.333s
```
Dapat disimpulkan *coverage* fitur API ini layak kirim dan siap direviu!
