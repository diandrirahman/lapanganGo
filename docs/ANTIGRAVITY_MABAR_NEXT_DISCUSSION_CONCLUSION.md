# Kesimpulan & Bahan Diskusi Lanjutan: Open Match / Mabar

Halo Antigravity,

Setelah review terakhir, posisi fitur **Open Match / Mabar MVP** saat ini sudah cukup matang di sisi backend. Modul `mabar` sudah dibuat, migration database sudah tersedia, endpoint utama sudah terdaftar, dan test backend lokal sudah lulus.

## Status Saat Ini

Backend sudah memiliki:

- Migration `db/migrations/005_open_matches.sql`.
- Tabel `open_matches`.
- Tabel `open_match_participants`.
- Modul `apps/api/internal/mabar`.
- Endpoint public list dan detail open match.
- Endpoint protected untuk create, join, leave, dan cancel mabar.
- Validasi booking harus `CONFIRMED`.
- Validasi host tidak bisa join match sendiri.
- Validasi slot penuh dan status `FULL`.
- Logic leave yang bisa mengembalikan status dari `FULL` ke `OPEN`.
- Locking transaksi saat join/leave/cancel untuk mengurangi risiko race condition.
- Timezone logic memakai `Asia/Jakarta`.

Hasil test terakhir:

```text
go test ./...
PASS
```

## Kesimpulan Codex

Fitur Mabar MVP secara struktur backend **sudah layak dianggap selesai untuk tahap unit-level/backend implementation**.

Namun sebelum masuk ke frontend besar, sebaiknya jangan langsung menganggap fitur ini production-ready. Masih perlu satu tahap validasi integrasi nyata dengan database dan skenario API end-to-end, karena sebagian risiko utama fitur Mabar ada di interaksi antar data: booking, availability, status booking, open match, dan participants.

Jadi next step yang paling aman adalah:

```text
Step berikutnya: E2E Manual QA untuk fitur Open Match / Mabar backend.
```

## Yang Perlu Didiskusikan / Dikerjakan Berikutnya

### 1. E2E Manual QA Mabar

Mohon Antigravity siapkan dan/atau jalankan skenario API manual dengan database lokal:

1. Buat customer host.
2. Buat customer participant.
3. Siapkan venue, court, sport, schedule, dan availability yang valid.
4. Host membuat booking lapangan sampai status `CONFIRMED`.
5. Host membuat open match dari booking tersebut.
6. Public user bisa melihat open match di `GET /open-matches`.
7. Public user bisa melihat detail di `GET /open-matches/:id`.
8. Participant join match.
9. `joined_count` dan `remaining_slots` berubah dengan benar.
10. Participant yang sama tidak bisa join dua kali.
11. Host tidak bisa join match sendiri.
12. Saat slot penuh, status berubah ke `FULL`.
13. Participant leave, status bisa kembali ke `OPEN`.
14. Host cancel match, match tidak bisa di-join lagi.
15. Booking yang bukan `CONFIRMED` tidak bisa dibuat/join sebagai open match.

Output yang diharapkan:

- File QA walkthrough atau report baru.
- Curl/Postman steps yang sesuai endpoint aktual.
- Catatan hasil response nyata.
- Catatan bug jika ada.

### 2. Kontrak Response untuk Frontend

Sebelum frontend dikerjakan, perlu disepakati apakah response saat ini sudah cukup untuk card desain:

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
- `max_players`
- `joined_count`
- `remaining_slots`
- `status`

Jika card UI butuh data tambahan seperti kota, alamat venue, avatar host, atau label waktu seperti "Hari Ini", mohon konfirmasi apakah itu dihitung di frontend atau perlu ditambahkan dari backend.

### 3. Keputusan MVP: Payment Participant

Untuk MVP saat ini, `price_per_player` masih bersifat informasi patungan. Backend belum memproses split payment participant.

Mohon konfirmasi apakah keputusan ini tetap:

```text
Host membayar booking utama, participant join tanpa payment flow di MVP.
```

Jika tidak, fitur payment participant harus menjadi modul baru dan sebaiknya tidak dicampur ke MVP Mabar saat ini.

### 4. Keputusan MVP: Participant Approval

Backend saat ini memakai logic:

```text
Participant langsung join selama slot masih tersedia.
```

Mohon konfirmasi apakah untuk MVP tidak perlu approval host. Jika butuh approval, perlu status participant tambahan seperti `PENDING`, `APPROVED`, dan `REJECTED`, sehingga scope backend berubah.

### 5. Urutan Setelah E2E QA

Jika E2E Manual QA Mabar lulus, rekomendasi urutan berikutnya:

1. Finalisasi kontrak API Mabar untuk frontend.
2. Buat seed/demo data open match agar UI bisa diuji.
3. Mulai implementasi frontend card/list Open Match.
4. Implementasi halaman detail Open Match.
5. Implementasi tombol Join/Leave/Cancel sesuai role user.

## Rekomendasi Keputusan

Untuk menjaga scope tetap rapi, saya rekomendasikan:

```text
Approve backend Mabar secara unit-level.
Lanjutkan ke Step E2E Manual QA Mabar.
Tunda frontend sampai kontrak response dan hasil E2E sudah jelas.
```

Tujuan diskusi dengan Antigravity sekarang bukan lagi membahas apakah fitur Mabar perlu dibuat, tetapi memastikan:

- implementasi backend benar-benar jalan dalam flow nyata,
- response sudah cocok untuk UI,
- keputusan payment dan approval participant tidak berubah diam-diam,
- frontend nanti tidak perlu bolak-balik karena kontrak API belum stabil.
