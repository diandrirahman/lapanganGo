# Prompt Frontend Step 1: Open Match Discovery / Card List

Halo Antigravity,

Codex sudah menyelesaikan review backend + E2E QA untuk fitur **Open Match / Mabar MVP**.

Status backend:

```text
APPROVED untuk lanjut ke frontend.
```

Namun frontend jangan langsung dibuat besar sekaligus. Kita mulai dari vertical slice paling kecil dan aman:

```text
Frontend Step 1: Open Match Discovery / Card List
```

Tujuan step ini adalah menampilkan daftar Open Match dari backend agar user bisa melihat mabar yang tersedia.

## Referensi Desain

Gunakan referensi UI dari:

```text
docs/design/antigravity-ui-preview.html
```

Bagian yang relevan:

```text
Cari Lawan / Open Match
Mabar card
Button: Gabung Match
```

Namun implementasi step ini tidak harus menyalin HTML preview mentah-mentah. Sesuaikan dengan struktur frontend yang akan dipakai di project.

## Scope Step 1

Buat UI untuk:

### 1. Section Open Match / Mabar

Tampilkan section dengan judul:

```text
Cari Lawan / Open Match
```

Isi section mengambil data dari endpoint:

```http
GET /open-matches
```

### 2. Card Open Match

Setiap card minimal menampilkan:

- `title`
- `host_name`
- `sport_name`
- `venue_name`
- `court_name`
- `match_date`
- `start_time`
- `end_time`
- `level`
- `price_per_player`
- `remaining_slots`
- `status`

Mapping visual:

```text
title -> nama match / team
host_name -> host
sport_name -> olahraga
venue_name + court_name -> lokasi
match_date + start_time -> waktu
level -> level permainan
price_per_player -> patungan per orang
remaining_slots -> sisa slot
status -> status match
```

### 3. UI State

Wajib ada state:

- Loading state saat fetch data.
- Empty state saat tidak ada open match.
- Error state saat API gagal.
- Responsive layout desktop/mobile.

### 4. Button Card

Tampilkan tombol:

```text
Gabung Match
```

Untuk Step 1, tombol ini belum perlu menjalankan join API.

Pilihan aman:

- tombol diarahkan ke detail route placeholder, atau
- tombol disabled dengan state UI yang rapi, atau
- tombol memunculkan placeholder action/log sederhana.

Jangan implementasi join flow penuh dulu di step ini.

## API Contract

Backend endpoint saat ini:

```http
GET /open-matches
```

Expected response:

```json
{
  "open_matches": [
    {
      "id": "uuid",
      "booking_id": "uuid",
      "host_name": "QA Host",
      "title": "Mabar Seru",
      "description": "Latihan QA Futsal",
      "sport_name": "Futsal",
      "venue_name": "QA Arena",
      "court_name": "Court QA",
      "match_date": "2026-06-25",
      "start_time": "18:00",
      "end_time": "20:00",
      "level": "All Levels",
      "max_players": 2,
      "joined_count": 1,
      "remaining_slots": 1,
      "price_per_player": 50000,
      "status": "OPEN",
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

Jika frontend memakai base URL config/env, gunakan konfigurasi yang rapi, misalnya:

```text
API_BASE_URL=http://localhost:8080
```

atau mekanisme env yang sesuai stack frontend.

## Batasan Scope

Jangan kerjakan dulu:

- Detail page penuh.
- Join API.
- Leave API.
- Cancel API.
- Create Open Match form.
- Auth/session flow baru.
- Payment participant.
- Host approval.
- Refactor backend.
- Perubahan schema database.

Fokus hanya:

```text
Menampilkan daftar Open Match dari API ke UI.
```

## Acceptance Criteria

Step 1 dianggap selesai jika:

1. UI dapat fetch `GET /open-matches`.
2. Card menampilkan field utama dengan benar.
3. Loading, empty, dan error state tersedia.
4. Layout responsive dan mengikuti rasa desain `antigravity-ui-preview.html`.
5. Tidak ada hardcoded data sebagai sumber utama. Dummy/static data hanya boleh dipakai sebagai fallback dev sementara dan harus jelas ditandai.
6. Tidak ada implementasi join/create/leave/cancel di step ini.
7. Jika ada frontend test/lint/build command, jalankan dan laporkan hasilnya.

## Expected Report

Setelah selesai, kirim report berisi:

```text
1. File frontend yang dibuat/diubah.
2. Cara menjalankan frontend.
3. Env/config yang dibutuhkan.
4. Screenshot atau deskripsi hasil UI.
5. Hasil test/lint/build.
6. Catatan limitation.
7. git status --short.
```

## Catatan PM

Tujuan step ini adalah validasi pertama bahwa data Mabar backend bisa hidup di UI.

Setelah Step 1 approved, baru kita lanjut ke:

```text
Step 2: Open Match Detail Page
Step 3: Join / Leave interaction
Step 4: Create Open Match from confirmed booking
```
