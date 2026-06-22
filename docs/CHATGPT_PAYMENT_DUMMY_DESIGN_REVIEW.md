# Design Review: Dummy Payment / Confirm Booking Flow

Sebagai *Product-minded Senior Backend Engineer*, saya telah mengevaluasi usulan arsitektur untuk fitur penyelesaian (*checkout/payment*) fiktif demi menutup *flow* MVP LapangGo. Berikut adalah ulasan dan rekomendasi teknis rancangannya.

## 1. Rekomendasi Status Final
**Rekomendasi:** Menggunakan satu status `CONFIRMED`.
**Alasan:** Karena kita belum memiliki integrasi tabel *payments* yang melacak nominal transaksi secara historis (atau *payment gateway* seperti Midtrans/Xendit), menetapkan status `PAID` bisa menimbulkan ambiguitas data seolah-olah terjadi mutasi finansial sungguhan. Menggunakan `CONFIRMED` jauh lebih netral; status ini mendeskripsikan bahwa sistem (dan *owner*) mengakui keabsahan *booking* tersebut untuk digunakan pada jadwal yang tertera.

## 2. Endpoint Final
**Rekomendasi API:** `POST /bookings/:id/pay`
(Bisa juga dipertimbangkan `POST /bookings/:id/confirm` agar selaras dengan nama status akhirnya, namun `pay` sangat intuitif untuk mewakili tombol "Bayar Sekarang" di sisi *frontend*).

## 3. Business Rules Final
Sangat selaras dengan rancangan *Cancellation API*, *business rules* pengamanan untuk _Payment API_ akan berlaku super ketat:
- Memerlukan otentikasi *Bearer token* dengan peran (`role`) wajib: `CUSTOMER`.
- Identifikasi isolasi vertikal: Pelanggan hanya bisa membayar *booking ID* milik mereka sendiri.
- Hanya melayani pesanan berstatus `PENDING_PAYMENT`.
- Kondisi konflik (*Race Guard*):
  - Jika pesanan memuat status `CANCELLED` (baik secara sengaja maupun otomatis *expired* nantinya), pembayaran ditolak.
  - Jika pesanan sudah memuat status `CONFIRMED` (terindikasi klik ganda atau *race condition*), pembayaran ditolak.

## 4. Pendekatan Repository, Service, dan Handler
Konvensi struktural yang digunakan akan meminjam secara *plug-and-play* mekanisme yang sudah teruji di modul `CancelBooking`.
- **Repository**: Membuat _method_ tunggal yang sangat atomik, misal `ConfirmPendingByIDAndCustomerID(ctx, bookingID, customerID)`. Kuerinya:
  ```sql
  UPDATE bookings SET status = 'CONFIRMED', updated_at = now()
  WHERE id = $1 AND customer_id = $2 AND status = 'PENDING_PAYMENT'
  ```
- **Service**: Jika repo mengembalikan `pgx.ErrNoRows`, Service diwajibkan untuk menembak ulang kueri baca (*Refetch*) menggunakan `FindByIDAndCustomerID` dan melakukan mitigasi galat bersyarat (*Race Fallback Mapping*).
- **Handler**: Mengambil `id` dari path, melakukan pembersihan format ke standar *UUID*, lalu melemparkan ke *service*.

## 5. Pemetaan Error HTTP
- `ErrInvalidUUID` -> **400 Bad Request**
- `ErrBookingNotFound` -> **404 Not Found**
- `ErrBookingAlreadyCancelled` -> **409 Conflict** (*Message*: "Booking has been cancelled and cannot be paid")
- `ErrBookingAlreadyConfirmed` -> **409 Conflict** (*Message*: "Booking is already paid/confirmed")
- *Unhandled Error* -> **500 Internal Server Error**

## 6. Test Plan
Cakupan unit testing *service_test.go* wajib memenuhi *matrix* ini:
- `[PASS]` Pembayaran berhasil (`PENDING_PAYMENT` ke `CONFIRMED`).
- `[FAIL]` *Booking* milik orang lain atau ID salah sasaran (*NotFound*).
- `[FAIL]` Coba bayar pesanan yang statusnya sudah `CANCELLED`.
- `[FAIL]` Coba bayar pesanan yang statusnya sudah `CONFIRMED`.
- `[FAIL]` *Race Condition*: _Atomic Update_ gagal karena status terganti tepat di sela-sela eksekusi, memicu proses *Refetch*, lalu sistem secara cermat mengembalikan `ErrBookingAlreadyCancelled` atau `ErrBookingAlreadyConfirmed`.

## 7. Risiko dan Batasan MVP
- **Tanpa Proteksi Uang Riil**: Karena ini sepenuhnya hanyalah tiruan (*dummy*), segala entitas dapat merubah pesanan mereka menjadi sah secara cuma-cuma. Pengguna berniat jahat bisa melakukan pendaftaran palsu (*spam*) dan memborong jadwal lapangan seharian.
- **Batasan Operasional**: *Owner* akan melihat jadwal lapangan mereka terpesan, padahal tidak ada rekam jejak transfer dana yang terintegrasi (mutasi).
- **Pengembangan Lanjutan**: Ketika *payment gateway* betulan ditambahkan, endpoint ini kemungkinan besar akan dihapus/direstrukturisasi untuk digantikan dengan eksekusi internal melalui _Webhook_ notifikasi pihak ketiga (seperti notifikasi _Charge_ sukses).

## 8. Kesiapan Implementasi
Desain *flow* penyelesaian fiktif ini **sangat solid, minim efek samping, sangat terprediksi, dan 100% siap** untuk diimplementasikan tanpa merombak migrasi basis data jika persetujuan dari *Codex* diberikan!
