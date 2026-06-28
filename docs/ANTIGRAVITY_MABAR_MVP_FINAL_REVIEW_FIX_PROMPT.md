# Prompt Perbaikan Final Review MVP Open Match / Mabar Untuk Antigravity

Halo Antigravity,

Codex sudah melakukan review tahap terbaru sebagai Product Manager dan Expert Software Engineer.

Secara umum implementasi backend Mabar sudah jauh lebih kuat:

- `gofmt` sudah bersih.
- `go test ./...` sudah pass.
- Join flow sudah memakai transaction context dan `FOR UPDATE OF om, b`.
- Duplicate insert sudah menangkap PostgreSQL unique violation `23505`.
- Test state transition sudah jauh lebih baik.
- README sudah menjelaskan behavior booking `CANCELLED`.

Tidak ada P1 blocker baru. Namun ada beberapa perbaikan final yang perlu dilakukan sebelum fitur dianggap siap merge dan siap dipakai frontend.

## Prioritas Perbaikan

1. P2: List dan join harus mensyaratkan booking `CONFIRMED`, bukan hanya `!= CANCELLED`.
2. P2: Validasi payload create open match, terutama `title`.
3. P3: Konsistensi timezone untuk expiry match.
4. P3: Tambahkan test untuk branch unique violation repository jika memungkinkan.

---

## 1. P2 - List Dan Join Harus Mensyaratkan Booking `CONFIRMED`

### Masalah

Kontrak produk MVP adalah:

```text
Open Match hanya boleh berasal dari booking yang sukses/terbayar.
```

Di backend saat ini, create open match sudah mensyaratkan:

```go
b.Status == "CONFIRMED"
```

Namun public list dan join masih hanya menolak booking `CANCELLED`.

Contoh saat ini:

```sql
b.status != 'CANCELLED'
```

dan di service:

```go
if joinCtx.BookingStatus == "CANCELLED" {
    return ErrBookingCancelled
}
```

### Risiko Produk

Jika nanti ada status booking lain, misalnya:

```text
PENDING_PAYMENT
PAID
REFUNDED
EXPIRED
FAILED
```

maka open match bisa tetap tampil atau bisa menerima join walaupun booking belum benar-benar valid untuk dipakai.

Ini bisa membuat user join match yang belum punya lapangan confirmed.

### File Terkait

```text
apps/api/internal/mabar/repository.go
apps/api/internal/mabar/service.go
apps/api/internal/mabar/service_test.go
README.md
```

### Arahan Perbaikan

Ubah filter list dari:

```sql
AND b.status != 'CANCELLED'
```

menjadi:

```sql
AND b.status = 'CONFIRMED'
```

Ubah validasi join dari:

```go
if joinCtx.BookingStatus == "CANCELLED" {
    return ErrBookingCancelled
}
```

menjadi validasi lebih ketat:

```go
if joinCtx.BookingStatus != "CONFIRMED" {
    return ErrBookingInvalid
}
```

atau jika ingin error lebih spesifik:

```go
ErrBookingNotConfirmed = errors.New("booking for this open match is not confirmed")
```

Lalu map error tersebut ke HTTP `409 Conflict` atau `400 Bad Request`.

README juga perlu disesuaikan:

```text
Open matches are only listed and joinable while the source booking status is CONFIRMED.
```

### Acceptance Criteria

- `GET /open-matches` hanya mengembalikan match dengan source booking `CONFIRMED`.
- `POST /open-matches/:id/join` menolak semua booking status selain `CONFIRMED`.
- Test mencakup join gagal jika booking status bukan `CONFIRMED`.
- README menjelaskan kontrak ini.

---

## 2. P2 - Validasi Payload Create Open Match

### Masalah

`CreateOpenMatchRequest` belum memvalidasi field yang penting untuk UI, terutama:

```text
title
```

Saat ini host bisa membuat open match dengan title kosong.

### Risiko Produk

Card UI "Cari Lawan / Open Match" membutuhkan title/team name seperti:

```text
FC Jakarta Casuals
Smash Yuk
Hoops Weekend
```

Jika title kosong, card frontend akan terlihat rusak dan membingungkan.

### File Terkait

```text
apps/api/internal/mabar/dto.go
apps/api/internal/mabar/service.go
apps/api/internal/mabar/handler.go
apps/api/internal/mabar/service_test.go
```

### Arahan Perbaikan

Tambahkan validation tag di DTO:

```go
type CreateOpenMatchRequest struct {
    Title          string  `json:"title" binding:"required,min=2,max=100"`
    Description    string  `json:"description" binding:"omitempty,max=500"`
    Level          string  `json:"level" binding:"required"`
    MaxPlayers     int     `json:"max_players" binding:"required,min=1"`
    PricePerPlayer float64 `json:"price_per_player" binding:"min=0"`
}
```

Tambahkan juga normalisasi di service agar aman walaupun request dibuat dari test/internal caller:

```go
title := strings.TrimSpace(req.Title)
if title == "" {
    return OpenMatchResponse{}, ErrInvalidTitle
}
```

Tambahkan error domain:

```go
ErrInvalidTitle = errors.New("title is required")
```

Gunakan `title` yang sudah di-trim saat create:

```go
Title: title,
```

Opsional:

```go
description := strings.TrimSpace(req.Description)
```

### Acceptance Criteria

- `POST /bookings/:id/open-matches` menolak title kosong.
- Title disimpan tanpa leading/trailing spaces.
- Handler mengembalikan HTTP 400 untuk invalid payload/title.
- Test mencakup title kosong dan title dengan spaces.

---

## 3. P3 - Konsistensi Timezone Untuk Expiry Match

### Masalah

Mabar expiry saat create/join menggunakan:

```go
time.Now()
```

Sedangkan public list menggunakan:

```sql
now()
```

Booking flow utama sudah memakai konsep `Asia/Jakarta`.

### Risiko

Jika server atau database berjalan di timezone UTC, match expiry bisa berbeda dari ekspektasi WIB/Jakarta.

Ini bisa menyebabkan:

- Match hilang terlalu cepat/lambat dari list.
- User masih bisa join match yang seharusnya sudah lewat di WIB.
- User ditolak join padahal secara WIB belum lewat.

### File Terkait

```text
apps/api/internal/mabar/service.go
apps/api/internal/mabar/repository.go
apps/api/internal/mabar/service_test.go
```

### Arahan Perbaikan

Buat helper waktu eksplisit:

```go
func nowJakarta() time.Time {
    loc, err := time.LoadLocation("Asia/Jakarta")
    if err != nil {
        loc = time.FixedZone("Asia/Jakarta", 7*60*60)
    }
    return time.Now().In(loc)
}
```

Saat membangun `matchTime`, gunakan location yang sama:

```go
loc, err := time.LoadLocation("Asia/Jakarta")
if err != nil {
    loc = time.FixedZone("Asia/Jakarta", 7*60*60)
}

matchTime := time.Date(
    date.Year(), date.Month(), date.Day(),
    start.Hour(), start.Minute(), 0, 0,
    loc,
)

if matchTime.Before(nowJakarta()) {
    return ErrMatchPassed
}
```

Untuk list query, hindari DB `now()` tanpa timezone eksplisit.

Pilihan yang lebih testable:

1. Service menghitung `nowJakarta`.
2. Repository menerima `Now` sebagai filter tambahan.
3. Query membandingkan timestamp dari parameter.

Contoh:

```go
type ListOpenMatchesFilter struct {
    SportID string
    City    string
    Date    string
    Level   string
    Limit   int
    Offset  int
    Now     time.Time
}
```

Atau gunakan SQL timezone eksplisit jika ingin minimal change:

```sql
(b.booking_date + b.start_time::time) > (now() AT TIME ZONE 'Asia/Jakarta')
```

Pastikan tipe comparison benar dan tidak ambigu.

### Acceptance Criteria

- Create, join, dan list memakai konsep waktu yang konsisten dengan Asia/Jakarta.
- Test mencakup match future dan past berdasarkan helper waktu yang bisa dikontrol atau minimal tidak flaky.
- Tidak ada perbedaan perilaku karena server timezone UTC.

---

## 4. P3 - Tambahkan Test Untuk Branch Unique Violation Repository

### Masalah

Production code sudah menangkap unique violation PostgreSQL:

```go
pgErr.Code == "23505"
```

Namun test saat ini hanya menguji duplicate via:

```go
exists: true
```

Artinya branch repository unique violation belum benar-benar diuji.

### File Terkait

```text
apps/api/internal/mabar/repository.go
apps/api/internal/mabar/service_test.go
```

### Arahan Perbaikan

Jika memungkinkan, tambahkan repository/integration test dengan test DB yang:

1. Insert open match pertama untuk `booking_id`.
2. Insert open match kedua untuk `booking_id` yang sama.
3. Assert error adalah `ErrMatchAlreadyExists`.

Jika integration test belum siap di project ini, minimal:

- Buat unit test service yang mensimulasikan `CreateOpenMatch` mengembalikan `ErrMatchAlreadyExists`.
- Pastikan service meneruskan error tersebut dan handler memetakannya ke 409.

### Acceptance Criteria

- Duplicate create karena unique conflict terbukti menghasilkan `ErrMatchAlreadyExists`.
- Jika belum ada integration test infra, tulis catatan eksplisit di completion report bahwa branch `23505` belum diuji end-to-end.

---

## Checklist Akhir

Jalankan:

```bash
cd apps/api
gofmt -w internal/mabar
go test ./...
```

Dari root repo:

```bash
gofmt -l apps/api/internal/mabar
git status --short
```

Pastikan:

1. `gofmt -l apps/api/internal/mabar` kosong.
2. `go test ./...` pass.
3. Tidak ada deletion file docs yang tidak disengaja.
4. Public list hanya booking `CONFIRMED`.
5. Join hanya booking `CONFIRMED`.
6. Title wajib valid dan tidak kosong.
7. Timezone expiry konsisten dengan Asia/Jakarta.
8. README update sesuai kontrak final.

## Output Laporan Yang Diharapkan

Setelah selesai, mohon kirim laporan berisi:

1. Ringkasan perubahan.
2. Keputusan error domain untuk booking non-confirmed.
3. Validasi payload yang ditambahkan.
4. Cara timezone dibuat konsisten.
5. Test baru yang ditambahkan.
6. Hasil `gofmt -l apps/api/internal/mabar`.
7. Hasil `go test ./...`.
8. Catatan apakah unique violation `23505` sudah diuji integration atau hanya melalui unit-level simulation.

