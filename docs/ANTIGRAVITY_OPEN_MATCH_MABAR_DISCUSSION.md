# Diskusi Fitur Cari Lawan / Open Match / Mabar

Halo Antigravity,

Saya ingin diskusi soal fitur "Cari Lawan / Open Match / Mabar" yang ada di desain LapanganGo.

Dari sisi backend LapanganGo saat ini, sistem sudah punya fondasi utama untuk booking lapangan:

- Auth customer dan owner.
- Data venue.
- Data court/lapangan.
- Data sport.
- Operating hours lapangan.
- Blocked slot untuk maintenance.
- Public availability slot lapangan.
- Customer booking.
- Dummy payment untuk MVP.
- Owner bisa melihat booking venue.

Namun backend saat ini belum punya modul khusus untuk fitur mabar/open match seperti yang tampil di desain.

## Usulan Logic MVP

Menurut saya logic MVP yang paling aman adalah fitur Open Match dibangun di atas sistem booking yang sudah ada.

Flow utamanya:

1. Host login sebagai customer.
2. Host memilih venue, court, tanggal, dan jam.
3. Host membuat booking lapangan seperti flow booking biasa.
4. Setelah booking berhasil, host bisa membuka booking tersebut sebagai "Open Match / Mabar".
5. Customer lain bisa melihat daftar open match.
6. Customer bisa melihat detail match seperti host, olahraga, lokasi, waktu, level, patungan, dan sisa slot.
7. Customer klik "Gabung Match".
8. Backend mengecek apakah slot peserta masih tersedia.
9. Jika tersedia, user masuk sebagai participant.
10. Jumlah "Sisa Slot" otomatis berkurang.
11. Jika peserta sudah penuh, status match menjadi `FULL`.
12. Host bisa membatalkan open match jika diperlukan.

Dengan pendekatan ini, open match tidak berdiri sendiri secara terpisah dari booking lapangan. Open match selalu punya booking lapangan yang valid sebagai sumber jadwal, court, venue, dan harga.

## Entity Database Yang Dibutuhkan

### open_matches

```text
id
booking_id
host_user_id
title
description
sport_id
court_id
venue_id
match_date
start_time
end_time
level
max_players
price_per_player
status
created_at
updated_at
```

Catatan:

- `booking_id` menghubungkan open match ke booking lapangan.
- `host_user_id` adalah user yang membuat match.
- `max_players` adalah total slot peserta yang dibuka.
- `price_per_player` adalah nilai patungan per orang.
- `sport_id`, `court_id`, dan `venue_id` bisa diambil dari booking/court, tetapi tetap bisa disimpan untuk memudahkan query list.

### open_match_participants

```text
id
open_match_id
user_id
status
joined_at
cancelled_at
created_at
updated_at
```

Catatan:

- Satu user tidak boleh join match yang sama lebih dari satu kali dalam status aktif.
- Participant dengan status `CANCELLED` tidak dihitung sebagai peserta aktif.

## Status Open Match

```text
DRAFT
OPEN
FULL
CANCELLED
COMPLETED
```

Penjelasan:

- `DRAFT`: match dibuat tapi belum dibuka untuk publik.
- `OPEN`: match bisa ditemukan dan user bisa join.
- `FULL`: slot peserta sudah penuh.
- `CANCELLED`: match dibatalkan.
- `COMPLETED`: match sudah selesai.

Untuk MVP, status utama yang paling penting adalah:

```text
OPEN
FULL
CANCELLED
```

## Status Participant

```text
JOINED
CANCELLED
```

Penjelasan:

- `JOINED`: user aktif sebagai peserta match.
- `CANCELLED`: user keluar atau dibatalkan dari match.

## Endpoint Backend Yang Dibutuhkan

### List Open Match

```text
GET /open-matches
```

Digunakan untuk menampilkan card seperti di homepage.

Query opsional:

```text
sport_id
city
date
level
status
limit
page
```

Response ideal untuk card:

```text
id
title
host
sport
venue
court
match_date
start_time
end_time
level
price_per_player
max_players
joined_count
remaining_slots
status
```

### Detail Open Match

```text
GET /open-matches/:id
```

Digunakan untuk halaman detail jika nanti dibutuhkan.

Response bisa berisi:

```text
open_match
participants
venue
court
booking
```

### Membuka Booking Sebagai Open Match

```text
POST /bookings/:id/open-match
```

Request body:

```text
title
description
level
max_players
price_per_player
```

Validasi:

- Booking harus milik user yang sedang login.
- Booking tidak boleh `CANCELLED`.
- Booking belum pernah dijadikan open match aktif.
- Waktu booking belum lewat.
- `max_players` harus lebih dari 0.
- `price_per_player` tidak boleh negatif.

### Join Open Match

```text
POST /open-matches/:id/join
```

Validasi:

- Match harus status `OPEN`.
- Waktu match belum lewat.
- Booking terkait tidak `CANCELLED`.
- User bukan host.
- User belum join match tersebut.
- `remaining_slots` harus lebih dari 0.

Jika setelah join jumlah peserta mencapai `max_players`, status match bisa otomatis berubah menjadi `FULL`.

### Keluar Dari Open Match

```text
DELETE /open-matches/:id/join
```

Validasi:

- User harus participant aktif di match tersebut.
- Match belum selesai.
- Match belum dibatalkan.

Jika sebelumnya status match `FULL`, setelah participant keluar status bisa kembali ke `OPEN`.

### Cancel Open Match

```text
PATCH /open-matches/:id/cancel
```

Validasi:

- Hanya host yang bisa cancel.
- Match belum selesai.
- Match belum `CANCELLED`.

Catatan:

- Cancel open match tidak harus otomatis cancel booking lapangan.
- Untuk MVP, cancel open match cukup menutup mabar dan semua participant tidak lagi bisa join.
- Pembatalan booking lapangan tetap memakai flow booking yang sudah ada.

## Logic Sisa Slot

Sisa slot dihitung dari:

```text
remaining_slots = max_players - joined_count
```

`joined_count` adalah jumlah participant dengan status:

```text
JOINED
```

Participant dengan status `CANCELLED` tidak dihitung.

## Logic Card UI

Card di desain membutuhkan data seperti:

```text
Host: Bima Aditya
Team/Match title: FC Jakarta Casuals
Sisa slot: 3
Olahraga: Mini Soccer
Lokasi: GBK Alpha Field
Waktu: Hari Ini, 19:00 WIB
Level: Beginner / Fun
Patungan: Rp 45.000 / Org
```

Mapping backend:

```text
title -> FC Jakarta Casuals
host.name -> Bima Aditya
remaining_slots -> Sisa 3 Slot
sport.name -> Mini Soccer
venue.name / court.name -> GBK Alpha Field
match_date + start_time -> Hari Ini, 19:00 WIB
level -> Beginner / Fun
price_per_player -> Rp 45.000 / Org
```

## Payment Untuk MVP

Untuk MVP, `price_per_player` bisa disimpan sebagai informasi dulu.

Artinya:

- User bisa melihat patungan per orang.
- Backend belum harus memproses split payment.
- Pembayaran lapangan tetap mengikuti booking host.

Fitur payment lanjutan bisa dipikirkan di fase berikutnya:

- Participant membayar bagian masing-masing.
- Host menerima settlement.
- Refund participant.
- Payment deadline.
- Auto-cancel participant kalau belum bayar.

## Risiko Dan Catatan Teknis

Hal-hal yang perlu dijaga:

1. Race condition saat banyak user join bersamaan.
   - Backend perlu transaction.
   - Saat join, hitung participant aktif dan lock row open match.

2. User tidak boleh join dua kali.
   - Perlu unique constraint untuk `open_match_id + user_id`.

3. Host tidak boleh join match sendiri.
   - Host sudah dianggap pembuat match, bukan participant biasa.

4. Booking cancel harus memengaruhi open match.
   - Jika booking lapangan dibatalkan, open match terkait sebaiknya ikut `CANCELLED`.

5. Match yang waktunya sudah lewat tidak boleh menerima join baru.

6. Status `FULL` harus bisa kembali ke `OPEN` jika ada participant keluar sebelum match dimulai.

## Pertanyaan Untuk Antigravity

Mohon review apakah flow ini cocok dengan desain UI Open Match yang sudah dibuat.

Beberapa hal yang perlu dikonfirmasi:

1. Apakah Open Match harus selalu berasal dari booking lapangan yang sudah dibuat?
2. Apakah host harus membayar booking penuh dulu, lalu peserta hanya patungan secara informal untuk MVP?
3. Apakah participant perlu approval dari host, atau langsung join selama slot tersedia?
4. Apakah perlu fitur "keluar dari match" pada MVP?
5. Apakah card homepage cukup menampilkan 3 open match terbaru/terdekat, atau perlu filter khusus?
6. Apakah `level` cukup berupa text bebas, atau perlu pilihan tetap seperti Beginner, Intermediate, Advanced, All Levels?

## Kesimpulan

Backend LapanganGo saat ini sudah siap secara fondasi karena sudah punya venue, court, availability, booking, dan user.

Namun fitur Open Match / Mabar masih perlu modul baru:

- Database table `open_matches`.
- Database table `open_match_participants`.
- API list/detail open match.
- API create open match dari booking.
- API join/leave/cancel.
- Logic remaining slot.
- Validasi status booking dan waktu match.

Untuk MVP, pendekatan paling sederhana dan aman adalah:

```text
Booking lapangan dulu -> buka sebagai Open Match -> user lain join sebagai participant.
```

