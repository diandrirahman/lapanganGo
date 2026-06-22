# AntiGravity Prompt: Cancellation API Step 4 Cleanup

```text
Kamu bertindak sebagai senior backend engineer untuk project LapangGo.

Review Codex untuk Cancellation Step 3:
- Race fallback/refetch sudah benar.
- Atomic cancel update sudah benar.
- Test `go test ./...` sudah lulus saat dijalankan dengan privilege/admin agar tidak diblokir Windows Application Control.
- Cancellation API secara produk sudah hampir approved.

Namun masih ada satu cleanup kecil sebelum final approval:
Ada perubahan error message lama yang tidak terkait Cancellation API. Ini berpotensi mengubah kontrak response client tanpa alasan.

Kerjakan Step 4 cleanup saja.

Scope file:
- `apps/api/internal/bookings/service.go`
- Report Step 4 di `docs/` jika ingin mencatat hasil

Tugas:
1. Di `apps/api/internal/bookings/service.go`, kembalikan pesan error lama berikut:

```go
ErrOverlapBlockedSlot = errors.New("court is blocked/maintenance during the requested time")
ErrOverlapBooking     = errors.New("court is already booked for the requested time")
```

2. Jangan ubah logic cancellation.
3. Jangan ubah repository.
4. Jangan ubah handler.
5. Jangan ubah test kecuali memang ada test yang secara eksplisit bergantung ke message baru.
6. Jangan tambah migration.
7. Jangan refactor besar.

Jalankan:
```bash
cd apps/api
gofmt -w internal/bookings/service.go
go test ./...
```

Catatan Windows:
Jika `go test ./...` diblokir Windows Application Control, jalankan terminal sebagai Administrator atau gunakan session elevated/admin.

Acceptance criteria:
- Error message overlap blocked slot dan overlap booking kembali seperti semula.
- Cancellation race fallback tetap ada.
- Atomic cancel update tetap memakai `id + customer_id + status = 'PENDING_PAYMENT'`.
- Semua test lulus.
- Tidak ada perubahan selain cleanup message dan report.

Report akhir Step 4:
- File yang berubah
- Error message yang dikembalikan
- Hasil `go test ./...`
- Konfirmasi tidak ada migration dan tidak ada perubahan flow cancellation
```
