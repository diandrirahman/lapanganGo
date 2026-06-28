# Prompt Revisi E2E QA Mabar Untuk Antigravity

Halo Antigravity,

Codex sudah mereview laporan:

```text
docs/CODEX_E2E_QA_COMPLETION_REPORT.md
docs/qa/mabar_walkthrough.md
apps/api/cmd/qa-seed/main.go
apps/api/run_qa.ps1
```

Kesimpulan sementara:

```text
E2E Mabar partially accepted, tetapi belum approved penuh untuk lanjut frontend.
```

Beberapa flow utama sudah terbukti: create open match, list, detail, host gagal join match sendiri, participant join, duplicate join conflict, leave, cancel, dan join setelah cancel ditolak.

Namun masih ada gap evidence dan cleanup yang perlu ditutup sebelum Codex memberi green light ke frontend.

## Tugas Revisi

Kerjakan revisi E2E QA Mabar saja. Jangan implementasi frontend dulu.

### 1. Buktikan Status FULL Secara Nyata

Report sebelumnya mengklaim status berubah ke `FULL`, tetapi walkthrough hanya menunjukkan:

```text
max_players = 2
joined_count = 1
remaining_slots = 1
status = OPEN
```

Artinya status `FULL` belum benar-benar terbukti.

Tambahkan skenario dengan participant kedua:

1. Seed atau buat user participant kedua, misalnya `part2_qa@example.com`.
2. Participant pertama join.
3. Participant kedua join.
4. Cek detail open match.
5. Pastikan response menunjukkan:

```json
{
  "max_players": 2,
  "joined_count": 2,
  "remaining_slots": 0,
  "status": "FULL"
}
```

6. Buktikan participant ketiga atau participant lain tidak bisa join saat status sudah `FULL`.
7. Participant kedua leave.
8. Cek detail lagi dan pastikan:

```json
{
  "joined_count": 1,
  "remaining_slots": 1,
  "status": "OPEN"
}
```

### 2. Buktikan Booking Non-CONFIRMED Ditolak

Report sebelumnya mengklaim booking selain `CONFIRMED` ditolak, tetapi walkthrough belum menampilkan bukti HTTP nyata.

Tambahkan skenario:

#### A. Create Open Match dari Booking `PENDING_PAYMENT`

1. Seed booking baru milik host dengan status `PENDING_PAYMENT`.
2. Hit endpoint:

```http
POST /bookings/:pendingBookingId/open-matches
Authorization: Bearer <HOST_TOKEN>
```

3. Expected:

```text
400 Bad Request
```

atau status error yang memang sesuai handler saat ini.

4. Response harus menunjukkan bahwa booking belum valid / belum confirmed.

#### B. Join Open Match Dengan Source Booking Tidak CONFIRMED

Untuk membuktikan join guard bekerja:

1. Buat open match valid dari booking `CONFIRMED`.
2. Ubah status booking sumbernya menjadi `PENDING_PAYMENT` atau `CANCELLED` langsung di database untuk kebutuhan QA.
3. Hit endpoint:

```http
POST /open-matches/:id/join
Authorization: Bearer <PART_TOKEN>
```

4. Expected:

```text
409 Conflict
```

5. Response harus menunjukkan booking source tidak confirmed / tidak valid untuk join.

Catatan: Untuk MVP, tidak perlu otomatis mengubah status open match menjadi `CANCELLED` ketika booking source berubah. Yang wajib terbukti adalah list/join tidak memperlakukan booking non-`CONFIRMED` sebagai joinable.

### 3. Rapikan Script QA

Saat ini `apps/api/run_qa.ps1` menyimpan hardcoded JWT token dan booking ID. Ini tidak boleh jadi artifact final karena:

- Tidak reproducible.
- Token akan expired.
- Berisi credential-like artifact.
- Sulit dipakai ulang oleh reviewer.

Pilih salah satu:

#### Opsi A: Hapus `apps/api/run_qa.ps1`

Jika script hanya artifact sementara, hapus file tersebut dan pindahkan langkah yang penting ke dokumentasi `docs/qa/mabar_walkthrough.md`.

#### Opsi B: Jadikan Script Reproducible

Jika ingin mempertahankan script, ubah agar menerima parameter/env var:

```powershell
$env:HOST_TOKEN
$env:PART_TOKEN
$env:PART2_TOKEN
$env:BOOKING_ID
$env:PENDING_BOOKING_ID
$env:BASE_URL
```

Jangan hardcode token dan UUID hasil run lokal.

Jika memakai opsi B, dokumentasikan cara menjalankannya.

### 4. Update Walkthrough Report

Update:

```text
docs/qa/mabar_walkthrough.md
```

Report harus memuat:

- HTTP request yang dijalankan.
- HTTP status code aktual.
- Response body penting.
- PASS/FAIL per skenario.
- Bukti status `FULL`.
- Bukti status kembali `OPEN` setelah leave dari kondisi full.
- Bukti create dari booking `PENDING_PAYMENT` ditolak.
- Bukti join ditolak saat source booking tidak lagi `CONFIRMED`.
- Catatan apakah `go test ./...` lulus setelah revisi.

### 5. Update Completion Report

Update:

```text
docs/CODEX_E2E_QA_COMPLETION_REPORT.md
```

Isi laporan harus jujur:

- Jangan klaim skenario lulus kalau tidak ada bukti di walkthrough.
- Jelaskan artifact apa yang diubah/dihapus.
- Jelaskan hasil final `go test ./...`.

## Acceptance Criteria

Codex hanya akan approve E2E Mabar penuh jika:

1. Status `FULL` benar-benar terbukti dengan `joined_count = max_players`.
2. Leave dari kondisi `FULL` mengembalikan status ke `OPEN`.
3. Booking `PENDING_PAYMENT` tidak bisa dibuat menjadi open match.
4. Join ditolak jika source booking tidak `CONFIRMED`.
5. Tidak ada token JWT hardcoded di artifact final.
6. `go test ./...` lulus.
7. Tidak ada file scratch sementara yang tertinggal.

## Batasan Scope

Jangan kerjakan:

- Frontend.
- Payment participant.
- Host approval flow.
- Perubahan schema baru.
- Refactor besar di luar kebutuhan QA.

Fokus hanya pada penutupan gap E2E evidence dan cleanup artifact QA.

Setelah selesai, kirim kembali:

```text
docs/CODEX_E2E_QA_COMPLETION_REPORT.md
docs/qa/mabar_walkthrough.md
git status --short
```
