# AntiGravity Prompts: Dummy Payment / Confirm Booking Flow

Dokumen ini berisi tahapan implementasi untuk AntiGravity. Kerjakan **satu step saja dalam satu waktu**, lalu berhenti dan buat report. Setelah Codex review dan approve, baru lanjut step berikutnya.

Target fitur:
`POST /bookings/:id/pay`

Status final MVP:
`CONFIRMED`

Prinsip produk:
- Ini dummy payment, bukan payment sungguhan.
- Jangan tambah tabel `payments`.
- Jangan tambah migration.
- Jangan klaim transaksi uang riil.
- Endpoint hanya mensimulasikan bahwa booking sudah dikonfirmasi oleh sistem.

Business rule final:
- Wajib Bearer token.
- Wajib role `CUSTOMER`.
- Customer hanya bisa pay/confirm booking miliknya sendiri.
- Hanya booking status `PENDING_PAYMENT` yang bisa berubah menjadi `CONFIRMED`.
- Status `CANCELLED`, `CONFIRMED`, dan `PAID` harus ditolak dengan `409 Conflict`.
- Update harus atomic dengan filter `id + customer_id + status = 'PENDING_PAYMENT'`.
- Jika atomic update gagal karena race, service harus refetch dan mapping error dengan jelas.
- Availability tetap memblokir booking `CONFIRMED` karena availability hanya mengecualikan `CANCELLED`.

---

## Step 1 - Repository Atomic Confirm Method

```text
Kamu bertindak sebagai senior backend engineer untuk project LapangGo.

Kerjakan Step 1 saja: tambahkan repository method atomic untuk dummy payment confirm.

Scope file:
- `apps/api/internal/bookings/repository.go`

Tugas:
1. Tambahkan method baru:
```go
func (r *Repository) ConfirmPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
```

2. Implementasikan query atomic:
```sql
UPDATE bookings
SET status = 'CONFIRMED',
    updated_at = now()
WHERE id = $1
  AND customer_id = $2
  AND status = 'PENDING_PAYMENT'
RETURNING id::text, customer_id::text, court_id::text, booking_date, start_time, end_time, total_price, status, created_at, updated_at
```

3. Jika tidak ada row yang cocok, return `pgx.ErrNoRows` seperti pattern existing repository.

Batasan:
- Jangan ubah service dulu.
- Jangan ubah handler dulu.
- Jangan ubah test dulu kecuali compile membutuhkan.
- Jangan tambah migration.
- Jangan refactor besar.

Jalankan:
```bash
cd apps/api
gofmt -w internal/bookings/repository.go
go test ./...
```

Catatan Windows:
Jika `go test ./...` diblokir Windows Application Control, jalankan terminal sebagai Administrator/elevated.

Acceptance Step 1:
- Method repository baru ada.
- Query update memastikan ownership customer.
- Query update hanya berhasil saat status masih `PENDING_PAYMENT`.
- Tidak ada perubahan flow service/handler.
- Tidak ada migration.

Report Step 1:
- File yang berubah
- Method baru
- Query atomic yang dipakai
- Hasil test/build
```

---

## Step 2 - Service Interface dan Confirm Payment Logic

```text
Kamu bertindak sebagai senior backend engineer untuk project LapangGo.

Step 1 sudah menambahkan repository method:
`ConfirmPendingByIDAndCustomerID(ctx, bookingID, customerID string) (Booking, error)`

Kerjakan Step 2 saja: wiring interface dan service logic untuk dummy payment.

Scope file:
- `apps/api/internal/bookings/service.go`
- `apps/api/internal/bookings/service_test.go` hanya untuk mock compile, belum wajib tambah semua test detail

Tugas:
1. Tambahkan sentinel error baru di `service.go`:
```go
ErrBookingAlreadyConfirmed = errors.New("booking already confirmed")
ErrBookingCannotBeConfirmed = errors.New("booking cannot be confirmed in current status")
```

2. Tambahkan method repository baru ke interface `BookingRepository`:
```go
ConfirmPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
```

3. Tambahkan service method:
```go
func (s *Service) ConfirmBookingPayment(ctx context.Context, customerID, bookingID string) (BookingResponse, error)
```

4. Flow awal service:
- Ambil booking dengan `FindByIDAndCustomerID(ctx, bookingID, customerID)`.
- Jika `pgx.ErrNoRows`, return `ErrBookingNotFound`.
- Jika status `CANCELLED`, return `ErrBookingAlreadyCancelled`.
- Jika status `CONFIRMED`, return `ErrBookingAlreadyConfirmed`.
- Jika status bukan `PENDING_PAYMENT`, return `ErrBookingCannotBeConfirmed`.
- Jika `PENDING_PAYMENT`, panggil `ConfirmPendingByIDAndCustomerID(ctx, bookingID, customerID)`.

5. Untuk Step 2, jika confirm atomic return `pgx.ErrNoRows`, boleh return error langsung dulu. Race fallback akan dikerjakan di Step 3.

6. Update mock repo di `service_test.go` agar compile:
```go
ConfirmPendingByIDAndCustomerID(ctx context.Context, bookingID, customerID string) (Booking, error)
```

Batasan:
- Jangan ubah handler.
- Jangan tambah route.
- Jangan tambah migration.
- Jangan refactor besar.

Jalankan:
```bash
cd apps/api
gofmt -w internal/bookings/service.go internal/bookings/service_test.go
go test ./...
```

Acceptance Step 2:
- Project compile.
- Interface dan mock sudah memakai method confirm baru.
- Service method `ConfirmBookingPayment` ada.
- Basic status validation ada.
- Tidak ada route baru dulu.

Report Step 2:
- File yang berubah
- Sentinel error baru
- Service flow yang dibuat
- Hasil test/build
```

---

## Step 3 - Race Fallback dan Unit Tests Service

```text
Kamu bertindak sebagai senior backend engineer untuk project LapangGo.

Step 2 sudah membuat service method:
`ConfirmBookingPayment(ctx, customerID, bookingID)`

Kerjakan Step 3 saja: tambahkan race fallback/refetch dan unit test service.

Scope file:
- `apps/api/internal/bookings/service.go`
- `apps/api/internal/bookings/service_test.go`

Tugas:
1. Di `ConfirmBookingPayment`, saat `ConfirmPendingByIDAndCustomerID` return `pgx.ErrNoRows`, lakukan refetch:
```go
latest, findErr := s.repository.FindByIDAndCustomerID(ctx, bookingID, customerID)
```

2. Mapping refetch:
- Jika `findErr` adalah `pgx.ErrNoRows`, return `ErrBookingNotFound`.
- Jika `findErr` error lain, return error itu.
- Jika `latest.Status == "CANCELLED"`, return `ErrBookingAlreadyCancelled`.
- Jika `latest.Status == "CONFIRMED"`, return `ErrBookingAlreadyConfirmed`.
- Jika `latest.Status != "PENDING_PAYMENT"`, return `ErrBookingCannotBeConfirmed`.
- Jika masih `PENDING_PAYMENT` tapi atomic update gagal, return `ErrBookingCannotBeConfirmed`.

3. Tambahkan/update unit test:
- `TestConfirmBookingPayment_Success`
- `TestConfirmBookingPayment_Fail_NotFound`
- `TestConfirmBookingPayment_Fail_AlreadyCancelled`
- `TestConfirmBookingPayment_Fail_AlreadyConfirmed`
- `TestConfirmBookingPayment_Fail_PaidCannotConfirm`
- `TestConfirmBookingPayment_Fail_StatusChangedToCancelledDuringConfirm`
- `TestConfirmBookingPayment_Fail_StatusChangedToConfirmedDuringConfirm`

Batasan:
- Jangan ubah handler.
- Jangan tambah route.
- Jangan tambah migration.
- Jangan refactor besar.

Jalankan:
```bash
cd apps/api
gofmt -w internal/bookings/service.go internal/bookings/service_test.go
go test ./...
```

Acceptance Step 3:
- Race condition tidak menjadi 500.
- `CANCELLED`, `CONFIRMED`, dan `PAID` tidak bisa diproses sebagai dummy payment.
- Semua unit test service lulus.

Report Step 3:
- File yang berubah
- Race fallback yang ditambahkan
- Test yang ditambahkan
- Hasil test/build
```

---

## Step 4 - Handler Route dan HTTP Error Mapping

```text
Kamu bertindak sebagai senior backend engineer untuk project LapangGo.

Step 3 sudah membuat service confirm payment solid dan teruji.

Kerjakan Step 4 saja: expose endpoint HTTP.

Endpoint:
`POST /bookings/:id/pay`

Scope file:
- `apps/api/internal/bookings/handler.go`

Tugas:
1. Tambahkan route di `RegisterRoutes` customer group:
```go
group.POST("/:id/pay", h.ConfirmBookingPayment)
```

2. Tambahkan handler:
```go
func (h *Handler) ConfirmBookingPayment(c *gin.Context)
```

3. Handler behavior:
- Ambil authenticated user ID.
- Validasi booking ID UUID, invalid -> `400 Bad Request`.
- Panggil `h.service.ConfirmBookingPayment(...)`.
- Success -> HTTP 200:
```json
{
  "message": "Booking payment confirmed successfully",
  "booking": { ... }
}
```

4. Update `respondBookingError`:
- `ErrBookingAlreadyCancelled` -> `409 Conflict`
- `ErrBookingAlreadyConfirmed` -> `409 Conflict`
- `ErrBookingCannotBeConfirmed` -> `409 Conflict`
- existing errors jangan dirusak.

Batasan:
- Jangan ubah repository.
- Jangan ubah service kecuali compile membutuhkan.
- Jangan tambah migration.

Jalankan:
```bash
cd apps/api
gofmt -w internal/bookings/handler.go
go test ./...
```

Acceptance Step 4:
- Endpoint route terdaftar.
- Auth role CUSTOMER tetap dipakai karena route masuk group `/bookings`.
- Error mapping sesuai.
- Semua test lulus.

Report Step 4:
- File yang berubah
- Route baru
- Response success
- Error mapping
- Hasil test/build
```

---

## Step 5 - Documentation and Final Verification

```text
Kamu bertindak sebagai senior backend engineer untuk project LapangGo.

Step 4 sudah expose endpoint:
`POST /bookings/:id/pay`

Kerjakan Step 5 saja: update dokumentasi dan final verification.

Scope file:
- `README.md`
- `docs/` report final jika diperlukan

Tugas:
1. Update README bagian Customer bookings:
Tambahkan:
```text
- POST /bookings/:id/pay
```

2. Tambahkan catatan singkat:
```text
Dummy payment: marks a PENDING_PAYMENT booking as CONFIRMED. This is not a real payment gateway integration.
```

3. Jalankan test penuh:
```bash
cd apps/api
go test ./...
```

Catatan Windows:
Jika diblokir Windows Application Control, gunakan terminal Administrator/elevated.

Acceptance Step 5:
- README menyebut endpoint payment dummy.
- README menjelaskan ini bukan payment gateway sungguhan.
- Semua test lulus.
- Tidak ada migration baru.

Report final:
- File yang berubah
- Ringkasan final feature
- Hasil `go test ./...`
- Konfirmasi no migration
- Risiko MVP dummy payment
```
