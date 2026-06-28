# Prompt Demo Seed Besar LapanganGo Untuk Antigravity

Halo Antigravity,

Kita ingin membuat **demo seed besar** agar frontend LapanganGo terlihat lebih realistis saat dipresentasikan dan diuji manual.

Penting:

```text
Jangan kerjakan frontend dulu.
Jangan ubah schema database.
Jangan ubah business logic backend kecuali benar-benar perlu untuk membuat seed berjalan.
Fokus hanya membuat demo seed besar yang realistis dan reproducible.
```

## Tujuan

Buat seed data besar untuk:

- venue discovery,
- court availability,
- booking flow,
- owner dashboard,
- open match / mabar cards,
- mabar detail dan participant list.

Data harus cukup banyak supaya UI tidak terlihat kosong atau terlalu artifisial.

## Lokasi Tool

Buat tool baru:

```text
apps/api/cmd/demo-seed/main.go
```

Jangan mengganti:

```text
apps/api/cmd/qa-seed/main.go
```

`qa-seed` tetap untuk QA kecil/deterministic. `demo-seed` untuk data besar dan realistis.

## Prinsip Seed

Seed harus:

1. **Idempotent**
   - Bisa dijalankan berkali-kali tanpa membuat data duplikat kacau.
   - Gunakan email/name/code prefix yang konsisten.

2. **Mudah dibersihkan**
   - Semua data demo harus punya prefix yang mudah dikenali:

```text
demo.
Demo
```

   Contoh email:

```text
demo.owner1@lapangango.test
demo.customer01@lapangango.test
demo.host01@lapangango.test
```

3. **Tidak memakai data pribadi nyata**
   - Semua nama, email, alamat, nomor telepon harus dummy.

4. **Tidak menyimpan token di file**
   - Token boleh dicetak saat command dijalankan, tapi jangan di-hardcode ke repo.

5. **Cocok dengan migration aktual**
   - Pakai kolom sebenarnya:
     - `users.password_hash`
     - `owner_profiles.business_name`
     - `venues.owner_profile_id`
     - `courts.location_type`
     - `courts.price_per_hour`
     - `court_operating_hours`
     - `court_blocked_slots`
     - `bookings`
     - `open_matches`
     - `open_match_participants`

6. **Tidak memakai tabel yang tidak ada**
   - Jangan gunakan tabel `schedules`.
   - Jangan gunakan tabel `availabilities`.

## Data Yang Harus Dibuat

### 1. Users

Buat minimal:

- 2 owner users.
- 10 host/customer users.
- 20 participant/customer users.

Total minimal:

```text
32 users
```

Role:

```text
OWNER
CUSTOMER
```

Password hash boleh dummy string yang valid untuk seed, kecuali login demo ingin diuji. Jika login ingin diuji, gunakan hash bcrypt yang bisa dipakai dengan password demo yang sama, misalnya:

```text
DemoPassword123!
```

Jika belum yakin format auth service, cukup gunakan dummy hash dan cetak JWT demo via token service.

### 2. Owner Profiles

Buat owner profile untuk 2 owner:

- `Demo Arena Group`
- `Demo Sport Hub`

Isi minimal:

- `user_id`
- `business_name`
- `verification_status = 'APPROVED'` jika enum mendukung.

### 3. Sports

Pastikan sports tersedia:

- Futsal
- Badminton
- Mini Soccer
- Basket
- Tenis
- Voli

Gunakan existing sports kalau sudah ada dari migration.

### 4. Venues

Buat 10-12 venue demo.

Contoh:

- Demo GBK Alpha Field
- Demo Smash Arena Bintaro
- Demo Kuningan Court
- Demo Senayan Futsal Center
- Demo BSD Sport Hall
- Demo Depok Badminton House
- Demo Bekasi Mini Soccer Park
- Demo Tebet Tennis Court
- Demo Kemang Sports Club
- Demo Kelapa Gading Arena

Variasi kota:

- Jakarta Selatan
- Jakarta Pusat
- Tangerang Selatan
- Depok
- Bekasi
- Jakarta Utara

Isi minimal sesuai schema:

- `owner_profile_id`
- `name`
- `description`
- `address`
- `district`
- `city`
- `province`
- `status = 'ACTIVE'`

### 5. Courts

Buat 2-5 court per venue.

Target total:

```text
30-40 courts
```

Variasi:

- sport berbeda,
- `location_type`: `INDOOR` / `OUTDOOR`,
- harga per jam:
  - Rp 75.000
  - Rp 100.000
  - Rp 150.000
  - Rp 200.000
  - Rp 300.000

Status:

```text
ACTIVE
```

### 6. Court Operating Hours

Buat operating hours untuk semua court.

Variasi:

- Mayoritas buka setiap hari `08:00-22:00`.
- Beberapa court buka weekend lebih lama `07:00-23:00`.
- Beberapa court tutup hari Senin atau Selasa.

Gunakan table:

```text
court_operating_hours
```

Kolom:

- `court_id`
- `day_of_week`
- `open_time`
- `close_time`
- `is_closed`

### 7. Blocked Slots

Buat 15-25 blocked slots.

Alasan:

- Maintenance
- Private event
- Cleaning
- Tournament prep

Gunakan tanggal relatif:

- hari ini,
- besok,
- 3 hari ke depan,
- 7 hari ke depan.

Gunakan table:

```text
court_blocked_slots
```

Pastikan:

```text
end_at > start_at
```

### 8. Bookings

Buat 60-100 bookings.

Distribusi status:

- 55% `CONFIRMED`
- 30% `PENDING_PAYMENT`
- 15% `CANCELLED`

Tanggal:

- hari ini sampai 14 hari ke depan,
- beberapa data 3 hari ke belakang boleh ada untuk history, tapi jangan terlalu banyak.

Rules:

- Hindari overlap booking pada court yang sama di tanggal/jam yang sama jika memungkinkan.
- Gunakan durasi realistis:
  - 1 jam
  - 2 jam

Kolom:

- `customer_id`
- `court_id`
- `booking_date`
- `start_time`
- `end_time`
- `total_price`
- `status`

### 9. Open Matches / Mabar

Buat 20-30 open matches.

Sumber:

- Harus berasal dari booking `CONFIRMED`.
- Match date/time mengikuti booking source.

Status:

- Mayoritas `OPEN`.
- Beberapa `FULL`.
- Beberapa `CANCELLED`.

Contoh title:

- Demo FC Jakarta Casuals
- Demo Smash Yuk
- Demo Hoops Weekend
- Demo Futsal Santai Senayan
- Demo Badminton After Office
- Demo Mini Soccer Fun Match
- Demo Basket Sunday Run
- Demo Tennis Beginner Club

Level:

- Beginner / Fun
- Intermediate
- Advanced
- All Levels

Price per player:

- 20000
- 35000
- 45000
- 50000
- 75000
- 100000

Max players:

- Badminton: 2-4
- Futsal: 8-12
- Mini Soccer: 10-16
- Basket: 6-10
- Tennis: 2-4

### 10. Open Match Participants

Buat 60-120 participant records.

Rules:

- Participant tidak boleh host match itu sendiri.
- Jangan duplicate active participant untuk match yang sama.
- Untuk status `OPEN`, jumlah joined harus < max_players.
- Untuk status `FULL`, jumlah joined harus = max_players.
- Untuk status `CANCELLED`, boleh ada participants lama tapi match tidak joinable.

Participant status:

- Mayoritas `JOINED`.
- Beberapa `CANCELLED`.

## Output Command

Saat dijalankan:

```bash
cd apps/api
go run ./cmd/demo-seed
```

Command harus mencetak ringkasan:

```text
Demo seed completed
Owners: X
Customers: X
Venues: X
Courts: X
Operating hours: X
Blocked slots: X
Bookings: X
Open matches: X
Participants: X
```

Cetak juga beberapa token demo yang berguna untuk frontend manual test:

```text
DEMO_OWNER_TOKEN=...
DEMO_HOST_TOKEN=...
DEMO_CUSTOMER_TOKEN=...
```

Jangan tulis token ke file.

## Optional: Cleanup Mode

Jika mudah, tambahkan cleanup mode:

```bash
go run ./cmd/demo-seed --cleanup
```

Cleanup harus menghapus data demo berdasarkan prefix:

```text
demo.
Demo
```

Jika cleanup terlalu berisiko, jangan implement cleanup dulu. Cukup dokumentasikan cara reset database lokal.

## Dokumentasi

Buat dokumen:

```text
docs/qa/demo_seed_big_report.md
```

Isi:

- Cara menjalankan seed.
- Data apa saja yang dibuat.
- Contoh token yang dicetak saat run, boleh disensor.
- Cara mengaktifkan frontend agar memakai backend demo data.
- Catatan cleanup/reset DB.
- Hasil `go test ./...`.

## Validasi

Wajib jalankan:

```bash
cd apps/api
go test ./...
```

Jika di Windows butuh elevated/admin, jalankan sesuai instruksi project.

## Acceptance Criteria

Demo seed akan dianggap selesai jika:

1. `apps/api/cmd/demo-seed/main.go` tersedia.
2. Data demo besar berhasil dibuat.
3. Seed idempotent atau minimal aman dijalankan ulang tanpa duplikasi kacau.
4. Minimal ada 10 venue.
5. Minimal ada 30 court.
6. Minimal ada 60 booking.
7. Minimal ada 20 open match.
8. Minimal ada 60 participant record.
9. `GET /open-matches` mengembalikan banyak data realistis tanpa perlu mock frontend.
10. Tidak ada schema change.
11. Tidak ada token hardcoded di repo.
12. `go test ./...` lulus.

## Batasan Scope

Jangan kerjakan:

- Frontend UI baru.
- Payment gateway real.
- Split payment participant.
- Host approval.
- Chat/notification.
- Upload avatar.
- Schema migration baru.
- Refactor besar backend.

Fokus hanya:

```text
Demo seed besar yang realistis untuk backend + frontend manual demo.
```

Setelah selesai, kirim:

```text
docs/qa/demo_seed_big_report.md
git status --short
go test ./... output
```
