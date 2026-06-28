# Step Pembuatan Fitur MVP Open Match / Mabar Untuk Antigravity

Dokumen ini adalah instruksi implementasi untuk Antigravity. Codex tidak akan mengerjakan implementasi pada fase ini. Setelah Antigravity selesai membuat perubahan kode, Codex akan melakukan review kode dan QA logic.

Referensi utama:

- `docs/CODEX_MABAR_MVP_IMPLEMENTATION_GUIDE.md`
- `docs/design/antigravity-ui-preview.html`

## Tujuan Fitur

Membuat fitur "Cari Lawan / Open Match / Mabar" untuk LapanganGo.

Prinsip MVP:

1. Host harus sudah punya booking lapangan yang sukses/terbayar.
2. Booking tersebut bisa dibuka sebagai Open Match.
3. User lain bisa melihat daftar Open Match.
4. User lain bisa join selama slot masih tersedia.
5. User bisa leave.
6. Host bisa cancel Open Match.
7. Pembayaran patungan masih informal di luar sistem.

## Step 1 - Review Backend Existing

Sebelum coding, pahami struktur backend yang sudah ada:

- `apps/api/cmd/api/main.go`
- `apps/api/internal/bookings`
- `apps/api/internal/availability`
- `apps/api/internal/venues`
- `apps/api/internal/courts`
- `db/migrations`

Pola arsitektur yang perlu diikuti:

- `handler.go`
- `service.go`
- `repository.go`
- `dto.go`
- `service_test.go`

Modul Mabar harus dibuat terpisah:

```text
apps/api/internal/mabar
```

Jangan menaruh logic Mabar langsung di modul `bookings`, kecuali hanya untuk membaca/memvalidasi data booking yang dibutuhkan.

## Step 2 - Buat Migration Database

Buat file migration baru setelah `004_bookings.sql`, misalnya:

```text
db/migrations/005_open_matches.sql
```

Isi tabel:

```text
open_matches
- id UUID PRIMARY KEY DEFAULT gen_random_uuid()
- booking_id UUID NOT NULL UNIQUE REFERENCES bookings(id) ON DELETE CASCADE
- host_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT
- title VARCHAR(100) NOT NULL
- description TEXT
- level VARCHAR(50) NOT NULL
- max_players INTEGER NOT NULL
- price_per_player NUMERIC(12, 2) NOT NULL DEFAULT 0
- status VARCHAR(50) NOT NULL DEFAULT 'OPEN'
- created_at TIMESTAMPTZ NOT NULL DEFAULT now()
- updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
```

```text
open_match_participants
- id UUID PRIMARY KEY DEFAULT gen_random_uuid()
- open_match_id UUID NOT NULL REFERENCES open_matches(id) ON DELETE CASCADE
- user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT
- status VARCHAR(50) NOT NULL DEFAULT 'JOINED'
- joined_at TIMESTAMPTZ NOT NULL DEFAULT now()
- cancelled_at TIMESTAMPTZ
- created_at TIMESTAMPTZ NOT NULL DEFAULT now()
- updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
- UNIQUE(open_match_id, user_id)
```

Tambahkan constraint:

```text
open_matches.level IN ('Beginner / Fun', 'Intermediate', 'Advanced', 'All Levels')
open_matches.status IN ('OPEN', 'FULL', 'CANCELLED', 'COMPLETED')
open_matches.max_players > 0
open_matches.price_per_player >= 0
open_match_participants.status IN ('JOINED', 'CANCELLED')
```

Tambahkan index:

```text
idx_open_matches_booking_id
idx_open_matches_host_user_id
idx_open_matches_status
idx_open_match_participants_match_id
idx_open_match_participants_user_id
```

## Step 3 - Buat Modul Backend `mabar`

Buat struktur:

```text
apps/api/internal/mabar/dto.go
apps/api/internal/mabar/repository.go
apps/api/internal/mabar/service.go
apps/api/internal/mabar/handler.go
apps/api/internal/mabar/service_test.go
```

Gunakan style dan error handling yang konsisten dengan modul existing.

## Step 4 - DTO API

Buat request DTO:

```text
CreateOpenMatchRequest
- title
- description
- level
- max_players
- price_per_player
```

Buat response DTO minimal:

```text
OpenMatchResponse
- id
- booking_id
- host
- title
- description
- sport
- venue
- court
- match_date
- start_time
- end_time
- level
- max_players
- joined_count
- remaining_slots
- price_per_player
- status
- created_at
- updated_at
```

Untuk list card homepage, response harus cukup untuk UI:

```text
title
host.name
remaining_slots
sport.name
venue.name
court.name
match_date
start_time
level
price_per_player
status
```

Buat participant response:

```text
ParticipantResponse
- id
- user_id
- name
- status
- joined_at
```

## Step 5 - Repository Logic

Repository minimal perlu method:

```text
FindBookingForOpenMatch(ctx, bookingID, userID)
CreateOpenMatch(ctx, params)
ListOpenMatches(ctx, filter)
GetOpenMatchByID(ctx, id)
ListParticipants(ctx, openMatchID)
CountJoinedParticipants(ctx, openMatchID)
FindParticipant(ctx, openMatchID, userID)
JoinOpenMatchTx(ctx, openMatchID, userID)
LeaveOpenMatchTx(ctx, openMatchID, userID)
CancelOpenMatch(ctx, openMatchID, hostUserID)
```

Untuk `JoinOpenMatchTx`, wajib menggunakan transaction dan lock row open match:

```sql
SELECT ... FROM open_matches WHERE id = $1 FOR UPDATE
```

Tujuannya mencegah race condition saat banyak user join bersamaan.

## Step 6 - Service Logic

Implementasikan logic berikut.

### Create Open Match

Endpoint:

```text
POST /bookings/:id/open-matches
```

Rules:

1. User harus login.
2. Booking harus milik user.
3. Booking harus status sukses/terbayar.
   - Untuk backend saat ini, gunakan status yang sudah dianggap sukses, misalnya `CONFIRMED`.
4. Booking tidak boleh `CANCELLED`.
5. Booking belum pernah punya open match aktif.
6. Waktu booking belum lewat.
7. `level` harus salah satu:
   - `Beginner / Fun`
   - `Intermediate`
   - `Advanced`
   - `All Levels`
8. `max_players > 0`.
9. `price_per_player >= 0`.

### List Open Matches

Endpoint:

```text
GET /open-matches
```

Rules:

1. Default hanya tampilkan status `OPEN`.
2. Jangan tampilkan match yang waktunya sudah lewat.
3. Support filter opsional:
   - sport_id
   - city
   - date
   - level
   - limit
   - page
4. Return `joined_count` dan `remaining_slots`.

### Detail Open Match

Endpoint:

```text
GET /open-matches/:id
```

Rules:

1. Return detail match.
2. Return participants aktif.
3. Return joined count dan remaining slots.

### Join Open Match

Endpoint:

```text
POST /open-matches/:id/join
```

Rules:

1. User harus login.
2. Match harus `OPEN`.
3. Waktu match belum lewat.
4. User bukan host.
5. User belum aktif join match tersebut.
6. Slot masih tersedia.
7. Jika participant sebelumnya `CANCELLED`, boleh re-join dengan mengubah status kembali ke `JOINED`.
8. Setelah join, jika `joined_count == max_players`, update status match menjadi `FULL`.

Wajib transaction + row lock.

### Leave Open Match

Endpoint:

```text
DELETE /open-matches/:id/join
```

Rules:

1. User harus login.
2. User harus participant aktif.
3. Match belum `CANCELLED`.
4. Match belum `COMPLETED`.
5. Ubah participant menjadi `CANCELLED`.
6. Isi `cancelled_at`.
7. Jika match sebelumnya `FULL`, update status kembali ke `OPEN`.

### Cancel Open Match

Endpoint:

```text
PATCH /open-matches/:id/cancel
```

Rules:

1. User harus login.
2. Hanya host yang bisa cancel.
3. Match belum `CANCELLED`.
4. Match belum `COMPLETED`.
5. Update status menjadi `CANCELLED`.
6. Tidak perlu otomatis cancel booking utama.

## Step 7 - Handler Routes

Register routes di `main.go`.

Ikuti style route existing. Saat ini backend belum memakai prefix `/api/v1`, jadi gunakan route tanpa prefix agar konsisten dengan project sekarang:

```text
GET /open-matches
GET /open-matches/:id
POST /bookings/:id/open-matches
POST /open-matches/:id/join
DELETE /open-matches/:id/join
PATCH /open-matches/:id/cancel
```

Jika Antigravity memutuskan memakai `/api/v1`, pastikan semua route lama dan dokumentasi ikut konsisten. Jangan campur diam-diam.

Route public:

```text
GET /open-matches
GET /open-matches/:id
```

Route auth customer:

```text
POST /bookings/:id/open-matches
POST /open-matches/:id/join
DELETE /open-matches/:id/join
PATCH /open-matches/:id/cancel
```

## Step 8 - Fraud & Safety MVP

Tambahkan rule backend:

```text
Host hanya bisa membuat Open Match dari booking miliknya sendiri yang sudah CONFIRMED.
```

Tambahkan warning di frontend sebelum join:

```text
LapanganGo tidak memfasilitasi transaksi di luar aplikasi. Untuk keamanan, selalu lakukan pembayaran/patungan secara langsung saat bertemu di lapangan.
```

Untuk MVP, jangan buat logic payment split dulu.

## Step 9 - Frontend UI

Ikuti desain:

```text
docs/design/antigravity-ui-preview.html
```

Bagian yang perlu dihubungkan ke backend:

1. Section "Cari Lawan / Open Match".
2. Card open match.
3. Tombol "Gabung Match".
4. Tombol "Buat Jadwal Mabar".

Card harus memakai data backend:

```text
title
host.name
remaining_slots
sport.name
venue.name / court.name
match_date + start_time
level
price_per_player
```

Tambahkan state UI:

```text
loading
empty state
error state
joined state
full state
cancelled state
```

Sebelum join, tampilkan warning modal/banner sesuai Step 8.

## Step 10 - Unit Test Backend

Tambahkan test minimal di:

```text
apps/api/internal/mabar/service_test.go
```

Test cases:

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

Jika memungkinkan, tambahkan repository/integration test untuk race condition join.

## Step 11 - Update Dokumentasi

Update:

```text
README.md
```

Tambahkan bagian:

```text
Open Match / Mabar
- GET /open-matches
- GET /open-matches/:id
- POST /bookings/:id/open-matches
- POST /open-matches/:id/join
- DELETE /open-matches/:id/join
- PATCH /open-matches/:id/cancel
```

Jelaskan:

- Open Match harus berasal dari booking `CONFIRMED`.
- Payment patungan masih informal.
- `remaining_slots = max_players - joined_count`.
- Join/leave/cancel behavior.

## Step 12 - Verifikasi Akhir Sebelum Diserahkan Ke Codex

Sebelum menyerahkan hasil ke Codex untuk review, jalankan:

```bash
cd apps/api
go test ./...
```

Pastikan:

1. Semua test existing tetap pass.
2. Test baru mabar pass.
3. Migration SQL valid.
4. Route sudah terdaftar di `main.go`.
5. README sudah update.
6. UI tidak hardcode data card mabar lagi jika backend sudah tersedia.

## Catatan Untuk Antigravity

Mohon jangan mengubah flow booking existing kecuali memang dibutuhkan untuk membaca data booking.

Prioritas utama adalah menjaga fitur booking yang sudah ada tetap stabil:

- Create booking.
- Confirm payment.
- Cancel booking.
- Availability slot.
- Owner booking list.

Jika ada perubahan pada modul existing, jelaskan alasannya di laporan akhir.

## Output Yang Diharapkan Dari Antigravity

Setelah selesai, mohon kirim ringkasan:

1. File migration yang ditambahkan.
2. Modul backend yang dibuat.
3. Endpoint yang tersedia.
4. Test yang ditambahkan.
5. Perubahan frontend yang dibuat.
6. Cara menjalankan dan menguji fitur.
7. Risiko atau batasan yang masih tersisa.

