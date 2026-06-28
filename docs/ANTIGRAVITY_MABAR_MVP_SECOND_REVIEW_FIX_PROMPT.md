# Prompt Perbaikan Review Kedua MVP Open Match / Mabar Untuk Antigravity

Halo Antigravity,

Codex sudah melakukan review kedua terhadap implementasi MVP Open Match / Mabar dari dua sudut pandang:

1. Product Manager: apakah fitur sudah aman dan jelas untuk MVP.
2. Expert Software Engineer: apakah logic backend cukup konsisten, testable, dan aman dari race condition.

Secara umum implementasi sudah jauh membaik. Perbaikan sebelumnya sudah masuk:

- `GET /open-matches` sekarang hanya menampilkan `OPEN`.
- Booking `CANCELLED` sudah ditolak saat join.
- Error count participant tidak lagi diabaikan.
- Error domain validation sudah dimap ke HTTP response yang lebih tepat.
- Deletion massal file docs sudah tidak muncul lagi di `git status`.

Namun masih ada beberapa hal yang perlu diperbaiki sebelum fitur dianggap siap merge.

## Prioritas Perbaikan

Urutan prioritas:

1. P1: Konsistensi transaction saat join vs booking cancellation.
2. P2: Race condition saat create open match dari booking yang sama.
3. P2: Test state transition yang masih belum membuktikan behavior penting.
4. P3: Jalankan `gofmt`.
5. Product polish: dokumentasikan behavior ketika booking utama dibatalkan.

---

## 1. P1 - Join Masih Bisa Race Dengan Pembatalan Booking Utama

### Masalah

Saat ini flow `JoinOpenMatch` membuka transaction dan lock row `open_matches`, tetapi detail match dan status booking terkait masih dibaca menggunakan repository method non-transaction:

```go
fullOm, err := s.repo.GetOpenMatchWithDetails(ctx, openMatchID)
bInfo, err := s.repo.FindBookingInfo(ctx, fullOm.BookingID)
```

Kedua method tersebut memakai `r.db.QueryRow`, bukan `tx.QueryRow`.

### Risiko

Jika booking utama dibatalkan bersamaan dengan participant join:

1. Join sudah lock `open_matches`.
2. Booking cancellation bisa terjadi di flow booking lain.
3. Join membaca status booking dari koneksi/query non-transaction.
4. Ada kemungkinan validasi booking status tidak konsisten dengan perubahan terbaru.
5. Participant bisa berhasil join match yang booking utamanya baru saja dibatalkan.

Ini adalah risiko produk yang penting karena user bisa join pertandingan yang lapangannya sudah tidak tersedia.

### File Terkait

```text
apps/api/internal/mabar/service.go
apps/api/internal/mabar/repository.go
apps/api/internal/mabar/service_test.go
```

### Arahan Perbaikan

Buat method repository transaction khusus untuk join context, misalnya:

```go
GetOpenMatchJoinContextTx(ctx context.Context, tx pgx.Tx, openMatchID string) (OpenMatchJoinContext, error)
```

Struct yang disarankan:

```go
type OpenMatchJoinContext struct {
    OpenMatchID   string
    BookingID     string
    HostUserID    string
    MatchDate     time.Time
    StartTime     time.Time
    MaxPlayers    int
    MatchStatus   string
    BookingStatus string
}
```

Query harus berjalan memakai `tx.QueryRow`.

Minimal query:

```sql
SELECT
  om.id::text,
  om.booking_id::text,
  om.host_user_id::text,
  b.booking_date,
  b.start_time,
  om.max_players,
  om.status,
  b.status
FROM open_matches om
JOIN bookings b ON b.id = om.booking_id
WHERE om.id = $1
FOR UPDATE OF om
```

Jika memungkinkan, lock booking juga:

```sql
FOR UPDATE OF om, b
```

Catatan:

- Gunakan locking yang tidak membuat deadlock dengan flow booking cancellation.
- Jika cancellation booking juga perlu aware terhadap open match, pertimbangkan urutan lock yang konsisten untuk semua flow.

Kemudian ubah `JoinOpenMatch` agar tidak memanggil read non-transaction untuk data yang diperlukan join.

Sebelum:

```go
om, err := s.repo.LockOpenMatch(ctx, tx, openMatchID)
fullOm, err := s.repo.GetOpenMatchWithDetails(ctx, openMatchID)
bInfo, err := s.repo.FindBookingInfo(ctx, fullOm.BookingID)
```

Sesudah:

```go
joinCtx, err := s.repo.GetOpenMatchJoinContextTx(ctx, tx, openMatchID)
```

Lalu validasi semua dari `joinCtx`.

### Acceptance Criteria

- `JoinOpenMatch` tidak membaca status booking menggunakan `r.db.QueryRow` di luar transaction.
- Status booking yang dipakai untuk validasi join dibaca dalam transaction yang sama.
- Jika booking status `CANCELLED`, join return `ErrBookingCancelled`.
- Tidak ada participant baru yang bisa masuk ke open match dengan booking utama `CANCELLED`.

---

## 2. P2 - Double Create Open Match Bisa Menjadi HTTP 500

### Masalah

Saat ini create open match melakukan:

```go
exists, err := s.repo.CheckOpenMatchExistsByBookingID(ctx, bookingID)
...
om, err := s.repo.CreateOpenMatch(...)
```

Karena check dan insert terpisah, dua request bersamaan dari booking yang sama bisa sama-sama melewati check. Salah satunya akan gagal di unique constraint `booking_id`.

### Risiko

Database aman karena ada unique constraint, tapi API client kemungkinan menerima HTTP 500, bukan error domain `ErrMatchAlreadyExists`.

### File Terkait

```text
apps/api/internal/mabar/repository.go
apps/api/internal/mabar/service.go
apps/api/internal/mabar/handler.go
apps/api/internal/mabar/service_test.go
```

### Arahan Perbaikan

Tangkap unique violation PostgreSQL saat insert `open_matches`.

Gunakan `pgconn.PgError`, misalnya:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    return OpenMatch{}, ErrDuplicateOpenMatch
}
```

Lalu di service map menjadi:

```go
ErrMatchAlreadyExists
```

Atau repository langsung return `ErrMatchAlreadyExists` jika package layering masih dianggap aman.

Alternatif SQL:

```sql
INSERT ... ON CONFLICT (booking_id) DO NOTHING
```

Lalu jika tidak ada row returned, return `ErrMatchAlreadyExists`.

### Acceptance Criteria

- Dua create request bersamaan untuk booking yang sama tidak menghasilkan HTTP 500.
- Request yang kalah race mendapat response conflict, idealnya HTTP `409`.
- Test mencakup create duplicate/unique conflict.

---

## 3. P2 - Test Belum Membuktikan State Transition Penting

### Masalah

Test saat ini sudah lebih baik dari stub, tetapi belum membuktikan state transition utama:

- Join slot terakhir mengubah status match menjadi `FULL`.
- Leave dari match `FULL` mengubah status kembali menjadi `OPEN`.
- Already joined ditolak.
- Match full ditolak.
- Invalid level ditolak.
- Invalid `max_players` ditolak.
- Invalid `price_per_player` ditolak.
- Error count participant menyebabkan join gagal dan tidak upsert.

Mock saat ini belum merekam pemanggilan:

```go
UpsertParticipant(...)
UpdateOpenMatchStatus(...)
```

Sehingga test tidak bisa memastikan status yang ditulis benar.

### File Terkait

```text
apps/api/internal/mabar/service_test.go
```

### Arahan Perbaikan

Perkuat mock repository agar merekam:

```go
upsertCalled bool
upsertStatus string
updatedStatus string
updateStatusCalled bool
```

Contoh:

```go
func (m *mockRepo) UpsertParticipant(ctx context.Context, tx pgx.Tx, openMatchID, userID, status string) error {
    m.upsertCalled = true
    m.upsertStatus = status
    return m.upsertErr
}

func (m *mockRepo) UpdateOpenMatchStatus(ctx context.Context, tx pgx.Tx, openMatchID, status string) error {
    m.updateStatusCalled = true
    m.updatedStatus = status
    return m.updateStatusErr
}
```

Tambahkan test minimal:

```text
TestService_CreateOpenMatch_InvalidLevel
TestService_CreateOpenMatch_InvalidMaxPlayers
TestService_CreateOpenMatch_InvalidPricePerPlayer
TestService_CreateOpenMatch_DuplicateConflict
TestService_JoinOpenMatch_AlreadyJoined
TestService_JoinOpenMatch_Full
TestService_JoinOpenMatch_LastSlotMarksFull
TestService_JoinOpenMatch_CountErrorDoesNotUpsert
TestService_LeaveOpenMatch_FromFullMarksOpen
TestService_LeaveOpenMatch_NotJoined
TestService_CancelOpenMatch_ClosedMatch
```

Jika membuat method transaction baru seperti poin 1, update mock sesuai interface baru.

### Acceptance Criteria

- Test bukan hanya cek error, tapi juga cek state transition.
- Test membuktikan `FULL` dan `OPEN` status update dipanggil pada kondisi yang benar.
- Test membuktikan error count mencegah `UpsertParticipant`.
- `go test ./...` pass.

---

## 4. P3 - Jalankan `gofmt`

### Masalah

Output:

```text
gofmt -l apps/api/internal/mabar
```

masih menampilkan:

```text
apps/api/internal/mabar/dto.go
apps/api/internal/mabar/repository.go
apps/api/internal/mabar/service.go
apps/api/internal/mabar/service_test.go
```

### Arahan Perbaikan

Jalankan:

```bash
cd apps/api
gofmt -w internal/mabar
```

Atau dari root repo:

```bash
gofmt -w apps/api/internal/mabar
```

### Acceptance Criteria

Command berikut tidak menampilkan file apa pun:

```bash
gofmt -l apps/api/internal/mabar
```

---

## 5. Product Polish - Behavior Saat Booking Utama Dibatalkan

### Masalah Produk

Untuk MVP, backend sudah mencegah list/join jika booking `CANCELLED`. Namun behavior lifecycle belum terdokumentasi jelas:

- Apakah open match otomatis ikut `CANCELLED` saat booking utama dibatalkan?
- Atau tetap status lama, tapi disembunyikan dari list dan join ditolak?

Saat ini implementasi tampaknya memilih opsi kedua.

### Arahan Produk

Untuk MVP, opsi kedua masih bisa diterima:

```text
Booking utama CANCELLED -> Open match tidak muncul di list dan join ditolak.
```

Namun README atau laporan harus menjelaskan behavior ini agar frontend tidak salah asumsi.

Tambahkan dokumentasi singkat:

```text
If the source booking is CANCELLED, the open match is excluded from public list and cannot accept new participants. The open match status itself is not automatically changed in MVP.
```

Jika ingin lebih rapi secara produk, boleh buat improvement:

```text
Saat booking cancel, open match terkait ikut di-set CANCELLED.
```

Namun ini menyentuh modul booking existing, jadi jangan dilakukan tanpa memastikan tidak merusak cancellation flow.

### Acceptance Criteria

- README menjelaskan behavior booking cancelled terhadap open match.
- Frontend nanti punya kontrak yang jelas: match bisa hilang dari list atau gagal join jika booking utama cancelled.

---

## Checklist Akhir Sebelum Dikembalikan Ke Codex

Jalankan:

```bash
cd apps/api
gofmt -w internal/mabar
go test ./...
```

Dari root repo, cek:

```bash
gofmt -l apps/api/internal/mabar
git status --short
```

Pastikan:

1. `gofmt -l apps/api/internal/mabar` kosong.
2. `go test ./...` pass.
3. Tidak ada deletion massal file docs.
4. Tidak ada HTTP 500 untuk known validation/domain error.
5. Join memakai transaction-consistent read untuk booking status.
6. Duplicate create open match menghasilkan conflict, bukan 500.
7. Test state transition utama sudah ada.

## Output Laporan Yang Diharapkan

Setelah selesai, mohon kirim laporan berisi:

1. Ringkasan perubahan teknis.
2. Method transaction baru yang dibuat.
3. Cara duplicate create ditangani.
4. Daftar test baru.
5. Hasil `gofmt -l apps/api/internal/mabar`.
6. Hasil `go test ./...`.
7. Keputusan final untuk lifecycle saat booking utama `CANCELLED`.

