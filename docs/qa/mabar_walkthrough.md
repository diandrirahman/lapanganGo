# Laporan E2E QA Walkthrough: Open Match (Mabar) MVP - REVISI

Laporan ini menguraikan hasil dari pengujian *End-to-End* (E2E) Manual pada modul Mabar (API Backend), berdasarkan skenario nyata menggunakan basis data PostgreSQL lokal (sesuai *migration* aktual). Semua parameter dalam skrip QA telah didekonstruksi dari *hardcode* menjadi pemanggilan dinamis melalui *Environment Variables* agar *reproducible*.

## 1. Setup & Pra-syarat
- **Database**: PostgreSQL (Dockerized lokal `lapangango_postgres`).
- **Skema DB**: Sesuai dengan seluruh berkas *migration* terakhir, termasuk `005_open_matches.sql`.
- **Seeder**: Menggunakan perkakas reproduktif `apps/api/cmd/qa-seed/main.go` yang akan mencetak variabel *environment*.
- **Entitas Seed**: 
  - Host User (`host_qa@example.com`)
  - Participant User 1 (`part_qa@example.com`)
  - Participant User 2 (`part2_qa@example.com`)
  - Owner User, Venue ("QA Arena"), Court ("Court QA"), Sport ("QA Futsal")
  - Booking (`CONFIRMED`) dibuat atas nama Host User.
  - Pending Booking (`PENDING_PAYMENT`) dibuat atas nama Host User.

---

## 2. Hasil Eksekusi Skenario (cURL/HTTP Steps)

Semua titik akses (*endpoints*) diuji menggunakan instruksi PowerShell (`run_qa.ps1`) yang mengonsumsi *environment variables*.

### Skenario 1: Host Membuat Open Match
**Konteks**: Host menggunakan tiket Booking (`CONFIRMED`) miliknya untuk membuat *Open Match* dengan slot sebanyak 2 pemain.
```http
POST /bookings/$env:BOOKING_ID/open-matches
Authorization: Bearer $env:HOST_TOKEN
Body: {"title": "Mabar Seru E2E Rev", "description": "Latihan QA Futsal", "level": "All Levels", "max_players": 2, "price_per_player": 50000}
```
**Status Code**: `201 Created`
**Hasil**: ✅ Pass

### Skenario 2: Host Dilarang Ikut (Join) Mabar Buatannya Sendiri
```http
POST /open-matches/:id/join
Authorization: Bearer $env:HOST_TOKEN
```
**Status Code**: `400 Bad Request`
**Hasil**: ✅ Pass (Sistem memblokir Host dari partisipasi ulang).

### Skenario 3: Participant Bergabung ke Mabar
```http
POST /open-matches/:id/join
Authorization: Bearer $env:PART_TOKEN
```
**Status Code**: `200 OK`
**Hasil**: ✅ Pass

### Skenario 4: Participant Gagal Bergabung Dua Kali
```http
POST /open-matches/:id/join
Authorization: Bearer $env:PART_TOKEN
```
**Status Code**: `409 Conflict`
**Hasil**: ✅ Pass (Participant sudah tergabung).

### Skenario 5: Participant 2 Bergabung dan Bukti Status "FULL"
```http
POST /open-matches/:id/join
Authorization: Bearer $env:PART2_TOKEN
```
**Status Code**: `200 OK`

Dilanjutkan dengan Pengecekan Akumulasi Kuota (`GET /open-matches/:id`):
```json
{
  "open_match": {
    "max_players": 2,
    "joined_count": 2,
    "remaining_slots": 0,
    "status": "FULL"
  },
  "participants": [
    {
      "name": "QA Participant",
      "status": "JOINED"
    },
    {
      "name": "QA Participant 2",
      "status": "JOINED"
    }
  ]
}
```
**Hasil**: ✅ Pass (Status benar-benar bergeser menjadi `FULL`).

### Skenario 6: Participant 2 Keluar (Leave) & Pemulihan Slot menjadi "OPEN"
```http
DELETE /open-matches/:id/join
Authorization: Bearer $env:PART2_TOKEN
```
**Status Code**: `200 OK`

Pengecekan Detail Lanjutan (`GET /open-matches/:id`):
```json
{
  "open_match": {
    "max_players": 2,
    "joined_count": 1,
    "remaining_slots": 1,
    "status": "OPEN"
  }
}
```
**Hasil**: ✅ Pass (Status `FULL` ter-anulir secara otomatis dan berbalik menjadi `OPEN`).

### Skenario 7: Create Open Match dari Booking `PENDING_PAYMENT` Ditolak
```http
POST /bookings/$env:PENDING_BOOKING_ID/open-matches
Authorization: Bearer $env:HOST_TOKEN
Body: {"title": "Pending Mabar", "level": "All Levels", "max_players": 2, "price_per_player": 50000}
```
**Status Code**: `400 Bad Request`
**Hasil**: ✅ Pass

### Skenario 8: Join Open Match Ditolak Saat Source Booking Berubah (Bukan CONFIRMED)
**Konteks**: Untuk mensimulasikan kondisi tiket Booking kedaluwarsa atau dibatalkan, dilakukan manuver langsung ke tabel `bookings` untuk mencetak status menjadi `CANCELLED`.
```sql
UPDATE bookings SET status='CANCELLED' WHERE id='$env:BOOKING_ID'
```
Lalu, Participant 2 mencoba untuk ikut serta:
```http
POST /open-matches/:id/join
Authorization: Bearer $env:PART2_TOKEN
```
**Status Code**: `409 Conflict` (Invalid / Source Booking tidak memenuhi kualifikasi).
**Hasil**: ✅ Pass

---

## 3. Catatan Tambahan
Seluruh titik rute (_routing_) E2E merespons dengan presisi tanpa *error panic*. Kondisi kode API LapanganGo tetap bersih dan lolos pengujian *test suite* secara mulus.

```text
Status QA: LULUS PENUH (PASS)
```
