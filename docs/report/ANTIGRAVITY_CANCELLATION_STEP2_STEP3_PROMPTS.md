# AntiGravity Prompts: Cancellation API Hardening

Dokumen ini berisi prompt bertahap untuk AntiGravity. Jalankan **Step 2 dulu**. Setelah hasil Step 2 direview dan approved, baru lanjut **Step 3**.

---

## Step 2 - Wiring Interface, Service Call, dan Mock

```text
Kamu bertindak sebagai senior backend engineer untuk project LapangGo.

Step 1 sudah selesai:
Repository sekarang punya method atomic:
`CancelPendingByIDAndCustomerID(ctx, bookingID, customerID string) (Booking, error)`

Query repository sudah memastikan:
- booking ID cocok
- customer ID cocok
- status saat update masih `PENDING_PAYMENT`

Sekarang kerjakan Step 2 saja: wiring interface, service call, dan mock agar project compile kembali.

Tujuan Step 2:
Ganti penggunaan method lama `UpdateBookingStatus` menjadi method baru `CancelPendingByIDAndCustomerID`.

Scope file:
- `apps/api/internal/bookings/service.go`
- `apps/api/internal/bookings/service_test.go`

Tugas:
1. Di interface `BookingRepository`, hapus method lama:
```go
UpdateBookingStatus(ctx context.Context, id string, status string) (Booking, error)
```

2. Ganti dengan method baru:
```go
CancelPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
```

3. Di `CancelBooking`, ganti call lama:
```go
updated, err := s.repository.UpdateBookingStatus(ctx, bookingID, "CANCELLED")
```

Menjadi:
```go
updated, err := s.repository.CancelPendingByIDAndCustomerID(ctx, bookingID, customerID)
```

4. Update mock repo di `service_test.go`.
Ganti mock method lama:
```go
UpdateBookingStatus(...)
```

Menjadi:
```go
CancelPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
```

5. Jangan ubah flow logic besar dulu.
Untuk Step 2, belum perlu menangani race fallback/refetch saat `CancelPendingByIDAndCustomerID` return `pgx.ErrNoRows`. Itu akan dikerjakan di Step 3.

6. Jangan ubah handler.
7. Jangan ubah repository lagi kecuali ada compile error kecil.
8. Jangan tambah migration.
9. Jangan refactor besar.

Jalankan:
```bash
cd apps/api
gofmt -w internal/bookings/service.go internal/bookings/service_test.go
go test ./...
```

Acceptance criteria Step 2:
- Project compile kembali.
- Semua test yang sudah ada lulus.
- Interface `BookingRepository` memakai method baru.
- Mock test memakai method baru.
- Tidak ada referensi ke `UpdateBookingStatus` yang tersisa.
- CancelBooking sudah memanggil `CancelPendingByIDAndCustomerID`.

Report akhir Step 2:
- File yang berubah
- Referensi method lama yang dihapus
- Hasil `go test ./...`
- Jika masih gagal, tampilkan error compile/test secara lengkap
```

---

## Step 3 - Race Fallback dan Error Mapping Saat Atomic Update Gagal

```text
Kamu bertindak sebagai senior backend engineer untuk project LapangGo.

Step 2 seharusnya sudah membuat project compile kembali dan `CancelBooking` sudah memanggil:
`CancelPendingByIDAndCustomerID(ctx, bookingID, customerID)`

Sekarang kerjakan Step 3 saja: tangani race condition saat atomic update gagal karena status booking berubah di antara read awal dan update.

Masalah yang diselesaikan:
Flow cancel sekarang:
1. `FindByIDAndCustomerID`
2. validasi status awal harus `PENDING_PAYMENT`
3. atomic update dengan `WHERE id = $1 AND customer_id = $2 AND status = 'PENDING_PAYMENT'`

Jika step 3 atomic update return `pgx.ErrNoRows`, kemungkinan:
- booking tidak ditemukan lagi
- booking bukan milik customer
- status sudah berubah menjadi `CANCELLED`
- status sudah berubah menjadi `PAID` / `CONFIRMED`

Service tidak boleh mengembalikan 500 untuk kondisi ini.

Scope file:
- `apps/api/internal/bookings/service.go`
- `apps/api/internal/bookings/service_test.go`

Tugas:
1. Di `CancelBooking`, setelah call:
```go
updated, err := s.repository.CancelPendingByIDAndCustomerID(ctx, bookingID, customerID)
```

Jika `err` adalah `pgx.ErrNoRows`, lakukan refetch:
```go
latest, findErr := s.repository.FindByIDAndCustomerID(ctx, bookingID, customerID)
```

2. Mapping hasil refetch:
- Jika `findErr` adalah `pgx.ErrNoRows`, return `ErrBookingNotFound`
- Jika `findErr` error lain, return error itu
- Jika `latest.Status == "CANCELLED"`, return `ErrBookingAlreadyCancelled`
- Jika `latest.Status != "PENDING_PAYMENT"`, return `ErrBookingCannotBeCancelled`
- Jika status masih `PENDING_PAYMENT` tapi update tetap gagal, return `ErrBookingCannotBeCancelled` sebagai fallback aman

3. Jika `CancelPendingByIDAndCustomerID` error selain `pgx.ErrNoRows`, return error tersebut.

4. Jangan ubah handler.
5. Jangan ubah repository kecuali benar-benar diperlukan.
6. Jangan tambah migration.
7. Jangan refactor besar.

Tambahkan/update unit test:
1. `TestCancelBooking_Fail_StatusChangedDuringCancel`
   - read awal mengembalikan booking status `PENDING_PAYMENT`
   - `CancelPendingByIDAndCustomerID` return `pgx.ErrNoRows`
   - refetch mengembalikan booking status `PAID` atau `CONFIRMED`
   - service return `ErrBookingCannotBeCancelled`

2. `TestCancelBooking_Fail_BecameCancelledDuringCancel`
   - read awal `PENDING_PAYMENT`
   - atomic update return `pgx.ErrNoRows`
   - refetch status `CANCELLED`
   - service return `ErrBookingAlreadyCancelled`

3. Pastikan existing tests tetap ada dan lulus:
   - `TestCancelBooking_Success`
   - `TestCancelBooking_Fail_NotFound`
   - `TestCancelBooking_Fail_AlreadyCancelled`
   - `TestCancelBooking_Fail_CannotCancelPaid`

4. Tambahkan test jika belum ada:
   - `TestCancelBooking_Fail_CannotCancelConfirmed`

Jalankan:
```bash
cd apps/api
gofmt -w internal/bookings/service.go internal/bookings/service_test.go
go test ./...
```

Acceptance criteria Step 3:
- Race condition saat status berubah tidak menjadi 500.
- Status `PAID` / `CONFIRMED` tidak bisa tertimpa menjadi `CANCELLED`.
- Status `CANCELLED` tidak bisa dicancel ulang.
- Semua test lulus.
- Tidak ada migration baru.
- Tidak ada perubahan besar di luar service dan service test.

Report akhir Step 3:
- File yang berubah
- Ringkasan race fallback/refetch
- Test yang ditambahkan
- Hasil `go test ./...`
- Jika masih gagal, tampilkan error compile/test secara lengkap
```
