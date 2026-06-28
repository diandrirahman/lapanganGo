# Prompt Perbaikan Review Kode MVP Open Match / Mabar Untuk Antigravity

Halo Antigravity,

Codex sudah melakukan review terhadap implementasi backend MVP Open Match / Mabar. Secara umum struktur modul sudah sesuai arahan: modul `apps/api/internal/mabar`, migration `005_open_matches.sql`, route sudah didaftarkan, dan `go test ./...` lulus.

Namun ada beberapa issue yang perlu diperbaiki sebelum fitur ini dianggap siap.

## 1. Open Match Masih Bisa Aktif Saat Booking Utama Dibatalkan

Masalah:

Saat ini `JoinOpenMatch` hanya mengecek status open match dan waktu match. Belum ada validasi bahwa booking utama yang menjadi sumber open match masih valid/tidak `CANCELLED`.

Risiko:

Jika host membatalkan booking lapangan lewat flow booking existing, open match masih bisa tampil dan peserta masih bisa join, padahal lapangannya sudah tidak tersedia.

File terkait:

```text
apps/api/internal/mabar/service.go
apps/api/internal/mabar/repository.go
```

Yang perlu diperbaiki:

1. `GET /open-matches` jangan menampilkan match yang booking utamanya `CANCELLED`.
2. `GET /open-matches/:id` sebaiknya tetap bisa detail, tapi status booking perlu dipertimbangkan. Minimal join harus ditolak jika booking sudah `CANCELLED`.
3. `POST /open-matches/:id/join` wajib cek status booking terkait.
4. Jika booking status `CANCELLED`, return error domain yang jelas, misalnya:

```go
ErrBookingCancelled = errors.New("booking for this open match is cancelled")
```

5. Handler harus map error tersebut ke HTTP `409 Conflict` atau `400 Bad Request`.

Acceptance criteria:

- User tidak bisa join open match jika booking terkait `CANCELLED`.
- Open match dengan booking `CANCELLED` tidak muncul di list public default.
- Error response tidak 500.

## 2. Error `Scan` Count Peserta Diabaikan Saat Join

Masalah:

Di `JoinOpenMatch`, query:

```go
tx.QueryRow(...).Scan(&joinedCount)
```

tidak mengecek return error.

Risiko:

Jika query count gagal, `joinedCount` tetap `0`, lalu sistem bisa mengizinkan join yang salah dan status `FULL` bisa tidak akurat.

File terkait:

```text
apps/api/internal/mabar/service.go
```

Yang perlu diperbaiki:

Ubah menjadi:

```go
if err := tx.QueryRow(ctx, "...", openMatchID).Scan(&joinedCount); err != nil {
    return err
}
```

Acceptance criteria:

- Jika count participant gagal, transaction rollback dan API return error.
- Tidak ada error database yang diabaikan di logic join.

## 3. Beberapa Validation Error Masih Menjadi HTTP 500

Masalah:

Beberapa validasi mengembalikan `errors.New(...)` biasa sehingga tidak dikenali oleh `respondError`, akhirnya client menerima HTTP 500.

Contoh:

```go
errors.New("max_players must be greater than 0")
errors.New("price_per_player cannot be negative")
errors.New("cannot leave cancelled or completed match")
errors.New("match already cancelled or completed")
```

File terkait:

```text
apps/api/internal/mabar/service.go
apps/api/internal/mabar/handler.go
```

Yang perlu diperbaiki:

Buat error domain eksplisit:

```go
ErrInvalidMaxPlayers
ErrInvalidPricePerPlayer
ErrCannotLeaveClosedMatch
ErrCannotCancelClosedMatch
```

Lalu map di handler:

```go
400 Bad Request
```

atau jika lebih cocok:

```go
409 Conflict
```

Acceptance criteria:

- Input invalid tidak pernah return 500.
- Leave/cancel pada status yang tidak valid tidak return 500.
- Error response konsisten dengan modul lain.

## 4. `ListOpenMatches` Mengabaikan Error Count Participant

Masalah:

Di `ListOpenMatches`:

```go
joined, _ := s.repo.CountJoinedParticipants(ctx, m.ID)
```

Error diabaikan.

Risiko:

Response card bisa salah. `remaining_slots` bisa terlihat masih tersedia padahal count gagal.

File terkait:

```text
apps/api/internal/mabar/service.go
```

Yang perlu diperbaiki:

Ubah agar error dikembalikan:

```go
joined, err := s.repo.CountJoinedParticipants(ctx, m.ID)
if err != nil {
    return nil, err
}
```

Acceptance criteria:

- Tidak ada ignored error untuk count participant.
- Jika count gagal, API list return error yang jelas.

## 5. Default List Public Menampilkan Match `FULL`

Masalah:

Guide menyebut default `GET /open-matches` menampilkan match yang `OPEN`. Saat ini query list mengambil:

```sql
om.status IN ('OPEN', 'FULL')
```

Risiko:

Homepage "Cari Lawan" bisa menampilkan match penuh yang tidak bisa dijoin, padahal default UX seharusnya mencari match yang masih bisa diikuti.

File terkait:

```text
apps/api/internal/mabar/repository.go
```

Yang perlu diperbaiki:

Default list gunakan:

```sql
om.status = 'OPEN'
```

Jika ingin mendukung filter status di masa depan, tambahkan query param secara eksplisit. Untuk MVP, cukup tampilkan `OPEN`.

Acceptance criteria:

- `GET /open-matches` hanya mengembalikan match `OPEN`.
- Match `FULL`, `CANCELLED`, dan `COMPLETED` tidak muncul di list default.

## 6. Unit Test Masih Stub, Belum Menguji Business Logic

Masalah:

`service_test.go` saat ini hanya test compile/stub. Belum menguji business logic penting.

File terkait:

```text
apps/api/internal/mabar/service_test.go
```

Yang perlu diperbaiki:

Tambahkan test minimal sesuai build steps awal:

1. Create open match berhasil dari booking `CONFIRMED`.
2. Create open match gagal jika booking bukan milik user.
3. Create open match gagal jika booking `CANCELLED`.
4. Create open match gagal jika booking belum `CONFIRMED`.
5. Create open match gagal jika level tidak valid.
6. Join berhasil saat slot tersedia.
7. Join gagal jika user adalah host.
8. Join gagal jika user sudah join.
9. Join mengubah status menjadi `FULL` saat slot terakhir terisi.
10. Leave mengubah participant menjadi `CANCELLED`.
11. Leave dari match `FULL` mengubah status kembali ke `OPEN`.
12. Cancel hanya bisa dilakukan host.
13. Join gagal jika booking terkait sudah `CANCELLED`.

Jika sulit membuat pure unit test karena repository concrete, boleh refactor `Service` agar bergantung pada interface repository kecil untuk test, selama tidak merusak struktur existing.

Acceptance criteria:

- Test bukan lagi stub.
- Test benar-benar assert error dan state transition.
- `go test ./...` tetap pass.

## 7. Periksa Perubahan Docs Yang Terhapus Di Luar Scope

Masalah:

`git status` menunjukkan banyak file docs lama terhapus. Ini terlihat tidak terkait langsung dengan fitur Mabar.

Yang perlu dilakukan:

1. Konfirmasi apakah penghapusan file docs memang disengaja.
2. Jika tidak disengaja, restore file docs tersebut.
3. Jangan sertakan deletion massal yang tidak terkait fitur Mabar.

Acceptance criteria:

- Diff final hanya berisi perubahan yang relevan dengan fitur Mabar, dokumentasi Mabar, dan README.
- Tidak ada penghapusan dokumen lama tanpa alasan jelas.

## Checklist Akhir

Setelah perbaikan, jalankan:

```bash
cd apps/api
go test ./...
```

Lalu laporkan:

1. File yang diperbaiki.
2. Error domain baru yang ditambahkan.
3. Test yang ditambahkan.
4. Hasil `go test ./...`.
5. Keputusan soal docs lama yang terhapus.

## Catatan Prioritas

Prioritas paling penting untuk diperbaiki:

1. Booking `CANCELLED` tidak boleh masih bisa menerima join.
2. Error database/count tidak boleh diabaikan.
3. Validation error tidak boleh menjadi HTTP 500.
4. Test harus menguji logic, bukan hanya compile.

