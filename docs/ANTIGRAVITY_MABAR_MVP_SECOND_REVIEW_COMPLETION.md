# Laporan Perbaikan Code Review MVP Mabar (Tahap 2)

Halo Tim & Codex! Berdasarkan ulasan level *Expert Software Engineer* dan *Product Manager*, seluruh potensi kerentanan level kritikal (seperti *Race Condition*) kini telah diatasi. 

Berikut adalah detail laporan dari perbaikan yang telah saya terapkan secara menyeluruh ke dalam sistem Backend LapanganGo:

## 1. Ringkasan Perubahan Teknis
- **Konsistensi Transaksi (P1):** Perombakan struktur pembacaan data saat proses `JoinOpenMatch`. Sistem kini tidak lagi mengandalkan pembacaan *non-transactional* (*Dirty Read*) untuk mengecek status *Booking*, melainkan menerapkan *Row-Level Lock* langsung ke tabel `bookings` dan `open_matches` secara atomik di dalam satu *transaction context*.
- **Keamanan Unique Insert (P2):** *Double HTTP POST* saat pembuatan *Open Match* untuk *booking* ID yang sama kini ditangani secara efisien di level *Database Driver* tanpa menyebabkan HTTP 500.
- **Penyempurnaan Formatter (P3):** Source code di modul `mabar` telah distandarisasi format spasinya lewat *tool* `gofmt`.
- **Ketahanan Testing:** Pengembangan unit testing kini dirombak untuk mengevaluasi *State Transition* dari `OPEN` ke `FULL` atau sebaliknya secara riil.

## 2. Method Transaction Baru
Di dalam `apps/api/internal/mabar/repository.go` dan antarmuka `MabarRepository`, saya menambahkan *method*:
```go
GetOpenMatchJoinContextTx(ctx context.Context, tx pgx.Tx, openMatchID string) (OpenMatchJoinContext, error)
```
*Method* ini mengunci kedua tabel secara bersamaan dengan klausa SQL `FOR UPDATE OF om, b`. Hal ini mengamankan sinkronisasi data sehingga *race condition* (di mana *participant* masuk tepat saat lapangan tiba-tiba dibatalkan oleh *host* atau *admin* di sistem) menjadi mustahil secara teori database (ACID).

## 3. Penanganan Duplicate Create
Jika *user* melakukan penekanan tombol berkali-kali untuk membuat Mabar, sistem akan menabrak batasan `UNIQUE(booking_id)` di Postgres. Saya memanfaatkan modul `github.com/jackc/pgx/v5/pgconn` untuk menangkap kode error pelanggaran constraint asli dari PostgreSQL (`23505`).
Ketika kode ini tertangkap, sistem dengan rapi melempar balik *domain error* `ErrMatchAlreadyExists` yang pada akhirnya akan menjadi respons HTTP standar **`409 Conflict`**—bukan *Internal Server Error 500*.

## 4. Daftar Test Baru (State Transition)
Semua skenario kritis telah direplika di `service_test.go`:
- `TestService_CreateOpenMatch_DuplicateConflict` (Menjamin 409 pada duplikasi Mabar)
- `TestService_CreateOpenMatch_InvalidLevel`
- `TestService_CreateOpenMatch_InvalidMaxPlayers`
- `TestService_CreateOpenMatch_InvalidPricePerPlayer`
- `TestService_JoinOpenMatch_LastSlotMarksFull` (Membuktikan *status* update dari `OPEN` menjadi `FULL` jika batas pemain tercapai)
- `TestService_JoinOpenMatch_AlreadyJoined`
- `TestService_JoinOpenMatch_Full`
- `TestService_JoinOpenMatch_CountErrorDoesNotUpsert` (Memastikan *Upsert* participant tidak tereksekusi jika kueri perhitungan *count* gagal di tengah jalan)
- `TestService_LeaveOpenMatch_FromFullMarksOpen` (Membuktikan bahwa keluarnya pemain mengembalikan ruang bernapas pertandingan ke `OPEN`)
- `TestService_LeaveOpenMatch_NotJoined`
- `TestService_CancelOpenMatch_ClosedMatch`

## 5. Hasil Formatting (`gofmt -l`)
Kosong. Seluruh *file* di direktori `apps/api/internal/mabar` kini telah mematuhi aturan lekukan standar Go setelah `gofmt -w` dioperasikan.

## 6. Hasil Testing Lanjutan (`go test ./...`)
Keseluruhan uji coba *Regression Test* dan *Unit Test* berhasil lolos:
```text
?       lapangango-api/cmd/api  [no test files]
ok      lapangango-api/internal/auth    (cached)
...
ok      lapangango-api/internal/mabar   3.308s
...
ok      lapangango-api/internal/venues  (cached)
```

## 7. Lifecycle Final Saat Booking Dibatalkan
Dari perspektif produk (Product Polish), disepakati bahwa status dari `Open Match` itu sendiri **tidak otomatis ikut berganti menjadi CANCELLED** di dalam MVP demi menjaga pemisahan modul.
Namun, efeknya tetap riil: setiap *Open Match* yang sumber *booking* utamanya berstatus `CANCELLED` **akan lenyap secara gaib dari Public List** dan **akan secara agresif menolak participant baru yang mencoba bergabung**. Kontrak logis ini telah saya sematkan secara resmi ke dalam berkas `README.md` repositori.

---
Dengan tercapainya level stabil dan aman, **Siklus Implementasi Backend Mabar Dinyatakan Selesai Penuh**. Modul Mabar siap *Merge*, *Deploy*, dan diintegrasikan oleh Codex di sisi UI/UX Next.js!
