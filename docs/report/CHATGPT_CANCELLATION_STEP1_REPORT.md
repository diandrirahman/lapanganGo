# Laporan Cancellation Hardening: Step 1 (Repository Atomic Update)

Sesuai dengan instruksi *Step 1*, lapisan repositori telah dikunci agar metode pembatalan di-*handle* secara atomik di level SQL dan memastikan hak kepemilikan (*customer ownership*) tanpa merombak lapisan ekosistem lainnya (Service/Handler/Interface).

Berikut laporan teknis perubahan yang telah dieksekusi:

## 1. File yang Berubah
- `apps/api/internal/bookings/repository.go`

## 2. Metode Lama yang Diganti
Metode *update generic* bawaan sebelumnya:
`UpdateBookingStatus(ctx context.Context, id string, status string) (Booking, error)`

Kini spesifik diganti sepenuhnya menjadi:
`CancelPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)`

## 3. Implementasi Query Atomic Baru
Logika filter berlapis tiga (`id`, `customer_id`, dan `status`) dijahit secara langsung dalam satu tarikan eksekusi SQL. Jika validasi meleset atau ditunggangi, SQL secara natif menolak *update* dan sistem akan melempar `pgx.ErrNoRows` selayaknya metode lain.

```sql
UPDATE bookings
SET status = 'CANCELLED', updated_at = now()
WHERE id = $1 AND customer_id = $2 AND status = 'PENDING_PAYMENT'
RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, created_at, updated_at
```

## 4. Hasil Kompilasi & Pengetesan
Eksekusi pengujian dengan perintah `go test ./...` di dalam direktori `apps/api` dilakukan secara ketat. Sebagaimana yang sudah diantisipasi, hilangnya eksistensi fungsi lama `UpdateBookingStatus` mengakibatkan implementasi antarmuka di level *service* terputus.

**Log Error (*Build Failed*):**
```text
# lapangango-api/cmd/api
cmd\api\main.go:74:41: cannot use bookingsRepository (variable of type *bookings.Repository) as bookings.BookingRepository value in argument to bookings.NewService: *bookings.Repository does not implement bookings.BookingRepository (missing method UpdateBookingStatus)
FAIL	lapangango-api/cmd/api [build failed]
```

Pekerjaan dihentikan tepat di titik kegagalan (*compile-time safety trigger*) ini, selaras dengan perintah yang melarang adanya koreksi terhadap modul lain pada tahap 1. Menunggu konfirmasi instruksi perbaikan dari Anda untuk *Step 2*!
