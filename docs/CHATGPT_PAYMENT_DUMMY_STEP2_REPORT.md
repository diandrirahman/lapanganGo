# Laporan Dummy Payment & Confirm Booking: Step 2 (Service Interface & Logic)

Pembuatan antarmuka (*interface wiring*) dan implementasi lapisan logika layanan untuk mengukuhkan pesanan pelangan (Dummy Payment Confirm) telah selesai tanpa modifikasi ke area *route* (rute HTTP).

Berikut rincian tindakannya:

## 1. File yang Berubah
- `apps/api/internal/bookings/service.go`
- `apps/api/internal/bookings/service_test.go` (khusus memfasilitasi _mock compiler_ agar tidak terputus)

## 2. Sentinel Error Baru
Dua tipe galat terdefinisi (*sentinel error*) dicanangkan untuk memperketat pelaporan kondisi pembatalan silang:
```go
ErrBookingAlreadyConfirmed = errors.New("booking already confirmed")
ErrBookingCannotBeConfirmed = errors.New("booking cannot be confirmed in current status")
```

## 3. Alur Service (Service Flow)
Metode `ConfirmBookingPayment` telah dieksekusi dengan rentetan validasi defensif sebelum memanggil *atomic update*:
1. Membaca (*fetch*) pesanan target melalui `FindByIDAndCustomerID`.
2. Melakukan evaluasi penolakan cepat (*Early Reject*) jika pesanan fiktif (bukan `ErrNoRows`).
3. Mencegah konfirmasi jika pesanan sudah kedapatan berstatus: `CANCELLED`.
4. Mencegah konfirmasi redundan jika pesanan berstatus: `CONFIRMED`.
5. Membatasi dan menolak eksekusi jika status aslinya bukan: `PENDING_PAYMENT`.
6. Bila lolos, `s.repository.ConfirmPendingByIDAndCustomerID` akan bertugas mengunci. (Skema mitigasi tumpang tindih / *Race Fallback* jika update ini gagal baru akan diterapkan menyusul pada **Step 3**).

## 4. Hasil Pengetesan dan Kompilasi
Kompilasi dan seluruh *existing test* berhasil tervalidasi sehat dalam *compiler*:
```text
ok      lapangango-api/internal/bookings        2.247s
```

Tahap integrasi pada _Service Interface_ ini tuntas, sesuai koridor batas (*No route handler* dan *No migration*). Proyek siap dibawa menuju pelengkap skema antiserobot pada eksekusi **Step 3**!
