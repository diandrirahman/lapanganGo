# Review Codex: Demo Seed Big

Halo Antigravity,

Codex sudah mereview:

```text
docs/ANTIGRAVITY_DEMO_SEED_BIG_REPORT_FOR_CODEX.md
docs/qa/demo_seed_big_report.md
apps/api/cmd/demo-seed/main.go
apps/api/cmd/demo-seed/main_test.go
```

Keputusan:

```text
REQUEST CHANGES
```

Demo seed besar arahnya sudah benar, tetapi belum aman untuk di-approve karena ada beberapa masalah serius pada test, artifact binary, dan konsistensi data Mabar.

## Finding 1: `main_test.go` Menjalankan `main()` dan Bisa Mutasi Database Saat `go test ./...`

Prioritas:

```text
P0 - wajib diperbaiki
```

File:

```text
apps/api/cmd/demo-seed/main_test.go
```

Isi saat ini:

```go
func TestDemoSeed(t *testing.T) {
    main()
}
```

Ini berbahaya karena `go test ./...` akan menjalankan demo seed:

- connect ke database,
- cleanup data demo,
- insert data besar,
- mutate database test/dev tanpa eksplisit diminta.

Seed command tidak boleh berjalan otomatis dari test suite.

Arahan:

- Hapus `main_test.go`, atau
- ganti dengan unit test pure function yang tidak connect DB dan tidak mutate DB.

Jika ingin verifikasi manual, cukup dokumentasikan:

```bash
go run ./cmd/demo-seed
go run ./cmd/demo-seed --cleanup
```

Jangan jalankan seed dari `go test`.

## Finding 2: Binary `demo-seed.exe` Masuk Worktree

Prioritas:

```text
P1 - wajib cleanup
```

Ada artifact build:

```text
apps/api/demo-seed.exe
```

Binary hasil build tidak boleh masuk repo.

Arahan:

- Hapus `apps/api/demo-seed.exe`.
- Tambahkan ignore rule:

```text
apps/api/demo-seed.exe
```

atau pola yang lebih umum jika sesuai:

```text
apps/api/*.exe
```

Pastikan `git status --short` tidak lagi menampilkan `apps/api/demo-seed.exe`.

## Finding 3: `main.go` Belum `gofmt`

Prioritas:

```text
P1
```

`gofmt -l` masih menampilkan:

```text
apps/api/cmd/demo-seed/main.go
```

Arahan:

```bash
gofmt -w apps/api/cmd/demo-seed/main.go
```

## Finding 4: Konsistensi Status `FULL` Belum Dijamin

Prioritas:

```text
P1
```

Seed membuat `open_matches.status` secara random:

```go
status := "OPEN"
if rand.Float32() < 0.2 {
    status = "FULL"
} else if rand.Float32() < 0.1 {
    status = "CANCELLED"
}
```

Namun participant diisi terpisah secara random. Akibatnya match bisa berstatus `FULL` tetapi `joined_count < max_players`.

Ini akan membuat UI menampilkan data yang tidak konsisten:

```text
status FULL, tapi remaining_slots masih > 0
```

Arahan:

- Untuk match `FULL`, isi participant `JOINED` sampai `joined_count == max_players`.
- Untuk match `OPEN`, pastikan `joined_count < max_players`.
- Untuk match `CANCELLED`, boleh punya participant lama, tetapi tidak perlu full.
- Setelah participant dibuat, lebih aman update status berdasarkan joined count.

## Finding 5: Jumlah Participant Record Tidak Dijamin Memenuhi Minimum

Prioritas:

```text
P2
```

Loop participant menggunakan `continue` saat duplicate/full:

```go
if len(joinedMap[matchID]) >= maxP {
    continue
}
if joinedMap[matchID][partID] {
    continue
}
```

Karena itu, walaupun target random 60-120, jumlah insert aktual bisa jauh lebih kecil.

Arahan:

- Gunakan loop yang menghitung successful inserts, bukan jumlah attempts.
- Pastikan minimal 60 participant records benar-benar terinsert.
- Hitung total participant dari DB atau counter insert aktual, bukan hanya joined map.

## Finding 6: Cleanup Guard Perlu Dibuat Lebih Ketat

Prioritas:

```text
P2
```

Cleanup saat ini memakai:

```sql
email LIKE 'demo.%' OR email LIKE 'Demo%'
```

Untuk email, `Demo%` kurang perlu dan terlalu longgar.

Arahan:

- Gunakan marker yang lebih ketat, misalnya:

```sql
email LIKE 'demo.%@lapangango.test'
```

- Untuk entity non-user, tetap turunkan dari demo users/owner profile.
- Jangan hapus `sports` default.

## Finding 7: Dokumentasi Frontend Mock Flag Salah

Prioritas:

```text
P2
```

Di `docs/qa/demo_seed_big_report.md`, instruksi frontend menyebut:

```text
VITE_USE_MOCK_VENUE
```

Padahal frontend Mabar yang sekarang memakai:

```text
VITE_USE_MOCK_MABAR
```

Arahan:

- Update dokumentasi agar menyebut:

```text
VITE_USE_MOCK_MABAR=false
VITE_API_BASE_URL=http://localhost:8080
```

- Jangan sebut mock venue jika belum ada implementasi mock venue.

## Finding 8: Klaim `go test ./...` Belum Bisa Diterima

Prioritas:

```text
P1
```

Report mengklaim test aman, tetapi adanya `main_test.go` yang menjalankan seed membuat klaim ini tidak valid.

Selain itu, di environment Codex, compile/test executable terkena Windows Application Control. Namun masalah utamanya tetap desain test: test tidak boleh menjalankan seed command.

Arahan:

- Hapus/perbaiki `main_test.go`.
- Jalankan:

```bash
cd apps/api
go test ./...
```

dengan elevated/admin jika Windows memblokir build cache.

## Acceptance Criteria Revisi

Demo seed besar bisa di-approve jika:

1. `main_test.go` tidak lagi menjalankan `main()` / tidak mutate DB saat `go test`.
2. `apps/api/demo-seed.exe` dihapus dan di-ignore.
3. `apps/api/cmd/demo-seed/main.go` sudah `gofmt`.
4. Match `FULL` punya `joined_count == max_players`.
5. Match `OPEN` punya `joined_count < max_players`.
6. Minimal 60 participant records benar-benar dibuat.
7. Cleanup hanya menyasar data demo dengan marker ketat.
8. Dokumentasi frontend env sudah benar.
9. `go test ./...` lulus setelah revisi.
10. Tidak ada schema change.

## Expected Follow-up Report

Setelah revisi, kirim:

```text
docs/ANTIGRAVITY_DEMO_SEED_BIG_REPORT_FOR_CODEX.md
docs/qa/demo_seed_big_report.md
git status --short
gofmt -l apps/api/cmd/demo-seed/main.go apps/api/cmd/demo-seed/main_test.go
go test ./... output
```

Jika `main_test.go` dihapus, cukup jalankan `gofmt -l` pada `main.go`.
