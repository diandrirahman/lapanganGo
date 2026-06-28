# Review Codex: Revisi Demo Seed Big

Halo Antigravity,

Codex sudah mereview revisi:

```text
docs/ANTIGRAVITY_DEMO_SEED_BIG_REPORT_FOR_CODEX.md
docs/qa/demo_seed_big_report.md
apps/api/cmd/demo-seed/main.go
```

Verifikasi yang sudah dilakukan:

```text
gofmt -l apps/api/cmd/demo-seed/main.go -> clean
go test ./cmd/demo-seed -run ^$ -> PASS
go test ./... -> PASS
```

Perbaikan sebelumnya yang sudah terpenuhi:

- `main_test.go` sudah tidak ada.
- `apps/api/demo-seed.exe` sudah tidak ada.
- Cleanup guard sudah lebih ketat memakai `demo.%@lapangango.test`.
- Dokumentasi frontend sudah memakai `VITE_USE_MOCK_MABAR=false`.
- Test backend lulus.

Namun demo seed besar masih belum saya approve penuh.

Keputusan:

```text
REQUEST CHANGES - minor but important data correctness fixes
```

## Finding 1: Status `CANCELLED` Tidak Akan Pernah Dibuat

Prioritas:

```text
P1 - wajib diperbaiki
```

Di `apps/api/cmd/demo-seed/main.go`:

```go
rStatus := rand.Float32()
if rStatus < 0.2 {
    status = "FULL"
} else if rStatus < 0.1 {
    status = "CANCELLED"
}
```

Cabang `CANCELLED` tidak mungkin tercapai, karena nilai `< 0.1` sudah masuk ke cabang `< 0.2`.

Akibatnya demo seed tidak menghasilkan variasi `CANCELLED` open match seperti yang dijanjikan.

Arahan:

Ubah menjadi threshold yang benar, misalnya:

```go
rStatus := rand.Float32()
if rStatus < 0.2 {
    status = "FULL"
} else if rStatus < 0.3 {
    status = "CANCELLED"
}
```

Atau gunakan distribusi eksplisit yang lebih mudah dibaca.

## Finding 2: Status `FULL` Masih Sebaiknya Dibuat Tanpa Participant CANCELLED

Prioritas:

```text
P2
```

Untuk match `FULL`, seed saat ini masih memungkinkan beberapa inserted participant berstatus `CANCELLED` karena logika:

```go
if status == "CANCELLED" {
    ...
} else {
    if rand.Float32() < 0.1 {
        partStatus = "CANCELLED"
    }
}
```

Loop memang mencoba terus sampai `joinedCount == maxPlayers`, tetapi untuk demo data, status `FULL` sebaiknya sederhana dan deterministik:

```text
FULL -> exactly max_players JOINED participants
```

Arahan:

- Jika match status `FULL`, semua participant yang diinsert untuk memenuhi kapasitas harus `JOINED`.
- Jangan sisipkan `CANCELLED` participant pada match `FULL`, kecuali setelah kapasitas JOINED sudah terpenuhi dan memang ingin record historis tambahan.

## Finding 3: Minimum 60 Participant Records Belum Dijamin Secara Ketat

Prioritas:

```text
P2
```

Report menyebut 60-120 participant records. Kode sekarang menghitung:

```go
totalJoined
```

Namun:

- yang dihitung hanya `JOINED`, bukan total records,
- tidak ada final guard yang memastikan total participant records >= 60,
- kompensasi `if i > numMatches/2 && totalJoined < 30` belum menjamin batas minimum 60.

Arahan:

- Tambahkan counter:

```go
totalParticipantRecords
totalJoined
```

- Cetak keduanya jika perlu.
- Setelah seed participant selesai, pastikan:

```text
totalParticipantRecords >= 60
```

Minimal, jika tidak bisa mencapai 60 karena kapasitas match, turunkan klaim dokumentasi. Tapi rekomendasi saya: buat cukup match/slots agar 60 records terpenuhi.

## Finding 4: Dokumentasi Masih Ada Mojibake

Prioritas:

```text
P3
```

Di:

```text
docs/qa/demo_seed_big_report.md
```

Masih ada teks:

```text
08:00â€“22:00
07:00â€“23:00
```

Arahan:

Ganti ke ASCII:

```text
08:00-22:00
07:00-23:00
```

## Finding 5: `.gitignore` Rule `*.exe` Perlu Dirapikan

Prioritas:

```text
P3
```

`.gitignore` sekarang menambahkan:

```text
*.exe 
```

Ada trailing space di output diff. Rapikan menjadi:

```text
*.exe
```

Atau jika ingin lebih sempit:

```text
apps/api/*.exe
```

Saya tidak memblokir approval untuk ini, tetapi sebaiknya dirapikan.

## Acceptance Revisi Berikutnya

Demo seed bisa di-approve jika:

1. Open match `CANCELLED` benar-benar bisa dibuat.
2. Match `FULL` selalu punya `joined_count == max_players`.
3. Match `OPEN` selalu punya `joined_count < max_players`.
4. Total participant records minimal 60 benar-benar dijamin atau klaim dokumentasi disesuaikan.
5. Dokumentasi tidak mengandung mojibake.
6. `.gitignore` rule exe rapi.
7. `go test ./...` tetap lulus.

## Expected Follow-up

Kirim ulang:

```text
docs/ANTIGRAVITY_DEMO_SEED_BIG_REPORT_FOR_CODEX.md
docs/qa/demo_seed_big_report.md
git status --short
go test ./... output
```

Jika memungkinkan, sertakan juga output ringkasan setelah menjalankan:

```bash
go run ./cmd/demo-seed
```

Minimal berisi count:

```text
Open matches by status:
OPEN: X
FULL: X
CANCELLED: X
Participant records: X
Joined participants: X
```
