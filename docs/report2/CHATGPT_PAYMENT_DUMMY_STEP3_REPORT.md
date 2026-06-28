# Laporan Dummy Payment & Confirm Booking: Step 3 (Race Fallback & Unit Tests)

Implementasi pengamanan dari kondisi perebutan pembaruan data (_Race Condition Fallback_) pada logika pesanan pelanggan (Dummy Payment Confirm) telah selesai dilakukan secara sempurna di tingkat layanan (_Service_).

Berikut rincian tindakannya:

## 1. File yang Berubah
- `apps/api/internal/bookings/service.go`
- `apps/api/internal/bookings/service_test.go`

## 2. Pemasangan Race Fallback yang Ditambahkan
Sistem layanan kini tidak lagi *panic* atau menyebarkan `500 Internal Server Error` ketika fungsi dari *atomic repository* (`ConfirmPendingByIDAndCustomerID`) mengembalikan `pgx.ErrNoRows`. Sebaliknya, saat skenario penolakan atomik terjadi, sistem bertindak aman dengan melakukan pemanggilan kueri `FindByIDAndCustomerID` (Refetching).
Hasil kueri *refetch* itu lalu dipetakan satu per satu:
- Baris lenyap: Menghasilkan `ErrBookingNotFound`
- Status telah berubah menjadi CANCELLED: Menghasilkan `ErrBookingAlreadyCancelled`
- Status telah berubah menjadi CONFIRMED: Menghasilkan `ErrBookingAlreadyConfirmed`
- Di luar skenario PENDING_PAYMENT (seperti PAID): Menghasilkan `ErrBookingCannotBeConfirmed`
- *Failsafe* (statusnya tetap PENDING tapi entah kenapa update ditolak): Menghasilkan `ErrBookingCannotBeConfirmed`

## 3. Test Kasus yang Ditambahkan
Uji unit menyeluruh (Unit Test Matrix) telah diintegrasikan pada modul `service_test.go`:
- `TestConfirmBookingPayment_Success`
- `TestConfirmBookingPayment_Fail_NotFound`
- `TestConfirmBookingPayment_Fail_AlreadyCancelled`
- `TestConfirmBookingPayment_Fail_AlreadyConfirmed`
- `TestConfirmBookingPayment_Fail_PaidCannotConfirm`
- `TestConfirmBookingPayment_Fail_StatusChangedToCancelledDuringConfirm`
- `TestConfirmBookingPayment_Fail_StatusChangedToConfirmedDuringConfirm`

## 4. Hasil Kompilasi dan Uji Ulang (*go test*)
Modifikasi komprehensif ini diklaim sepenuhnya stabil. Hal ini dibuktikan melalui eksekusi mulus dari kompilator pengujian bawaan bahasa *Go* di terminal:
```text
ok      lapangango-api/internal/bookings        2.293s
```
Semua rincian uji unit (termasuk *race conditions simulation*) lolos 100%.

Proyek ini telah matang secara logika dan antarmuka layanannya, dan menanti perintah kelanjutan Anda menuju **Step 4** (Penyambungan *HTTP Router* ke internet)!
