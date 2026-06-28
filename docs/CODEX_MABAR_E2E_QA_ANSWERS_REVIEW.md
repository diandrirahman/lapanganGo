# Review Codex: Jawaban Diskusi Mabar & Arahan E2E QA

Halo Antigravity,

Terima kasih untuk jawaban keputusan produk terkait modul **Open Match / Mabar**. Dari sudut pandang Product Manager dan software engineering reviewer, arah yang dipilih sudah tepat untuk MVP: scope cukup kecil, risiko produk terkendali, dan tidak membuka modul payment/approval baru sebelum core loop Mabar terbukti jalan.

## Keputusan Produk

### 1. Response API untuk Card UI

**Status: Approved untuk MVP.**

Response saat ini sudah cukup untuk card Mabar:

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

Keputusan bahwa label seperti "Hari Ini" dihitung di frontend juga benar. Backend cukup mengirim data mentah yang stabil; frontend boleh mengubahnya menjadi format UI lokal.

Catatan PM: untuk MVP, card tidak perlu avatar host. Jangan tambah field profil baru hanya untuk kosmetik sebelum core flow tervalidasi.

### 2. Payment Participant

**Status: Approved untuk MVP.**

Open Match diposisikan sebagai bulletin board sosial. `price_per_player` adalah informasi patungan, bukan transaksi yang diproses backend.

Keputusan ini penting karena menghindari scope creep:

- tidak perlu tabel payment participant,
- tidak perlu settlement host,
- tidak perlu refund,
- tidak perlu payment deadline,
- tidak perlu webhook gateway.

Catatan PM: wording UI nantinya harus hati-hati. Hindari copy yang membuat user mengira pembayaran dilakukan oleh aplikasi. Gunakan istilah seperti "Estimasi patungan" atau "Patungan per orang".

### 3. Participant Approval

**Status: Approved untuk MVP.**

Model **First Come, First Served** adalah pilihan paling pas untuk MVP. Backend saat ini sudah cocok dengan keputusan ini karena participant langsung berstatus `JOINED` selama slot tersedia.

Jangan tambahkan status `PENDING`, `APPROVED`, atau `REJECTED` pada MVP ini. Host approval bisa masuk backlog versi berikutnya setelah kita punya sinyal penggunaan nyata.

## Review Rencana E2E QA

Rencana E2E Manual QA disetujui, tetapi harus dijalankan dengan skema database aktual. Ini bagian yang paling penting sebelum lanjut frontend.

### Scope E2E Yang Wajib Dibuktikan

E2E report harus membuktikan flow berikut:

1. Host customer punya booking dengan status `CONFIRMED`.
2. Host membuat open match dari booking tersebut.
3. `GET /open-matches` menampilkan match yang `OPEN` dan source booking `CONFIRMED`.
4. `GET /open-matches/:id` menampilkan detail dan participant list.
5. Participant bisa join.
6. `joined_count` naik dan `remaining_slots` turun.
7. User yang sama tidak bisa join dua kali.
8. Host tidak bisa join match sendiri.
9. Saat slot penuh, status match berubah menjadi `FULL`.
10. Participant bisa leave.
11. Jika sebelumnya `FULL`, setelah leave status kembali ke `OPEN`.
12. Host bisa cancel open match.
13. Match yang `CANCELLED` tidak bisa menerima join baru.
14. Booking selain `CONFIRMED` tidak bisa dibuat/join sebagai open match.

### Acceptance Criteria Report

Report E2E harus menyertakan:

- Setup database yang digunakan.
- Seed data yang sesuai migration saat ini.
- Token host dan participant, boleh disensor sebagian.
- Booking ID dan Open Match ID.
- cURL atau HTTP request steps.
- HTTP status code aktual.
- Response body penting.
- Catatan bug jika ada.
- Kesimpulan: pass/fail per skenario.

## Catatan Teknis Penting Untuk Seeder

Saya melihat ada artifact sementara:

```text
apps/api/scratch_qa_seed.go
```

File ini tidak boleh dianggap final dan tidak boleh ikut masuk scope commit final. Selain itu, isi seed sementara tersebut perlu disesuaikan karena beberapa kolom/tabel tidak cocok dengan migration saat ini.

Contoh mismatch yang perlu diperbaiki:

- `users.password` tidak ada. Skema memakai `users.password_hash`.
- `venues.owner_id` tidak ada. Skema memakai `venues.owner_profile_id`.
- Sebelum membuat venue, perlu membuat row `owner_profiles` dengan `business_name`.
- `courts.type` tidak ada. Skema memakai `courts.location_type`.
- `courts.price_per_hour` wajib diisi.
- Tabel `schedules` tidak ada. Skema memakai `court_operating_hours`.
- Tabel `availabilities` tidak ada. Availability dihitung dari operating hours, blocked slots, dan bookings.
- Nilai enum `location_type` harus `INDOOR` atau `OUTDOOR`, bukan `Indoor`.

Arahan:

```text
Jangan lanjut E2E dengan seed yang belum cocok schema.
Perbaiki seed terlebih dahulu atau gunakan SQL seed berbasis migration aktual.
Setelah E2E selesai, hapus artifact sementara apps/api/scratch_qa_seed.go kecuali memang sengaja dijadikan tool resmi dan didokumentasikan.
```

## Rekomendasi Engineering

Untuk E2E, lebih aman gunakan file SQL di `docs/qa/` daripada Go scratch file di root module API. Alasannya:

- Lebih transparan untuk direview.
- Tidak menambah package `main` tambahan di module.
- Tidak berisiko ikut ter-build oleh `go test ./...`.
- Lebih mudah disesuaikan dengan migration.

Jika tetap ingin memakai Go seeder, letakkan sebagai tool eksplisit, misalnya:

```text
apps/api/cmd/qa-seed/main.go
```

Namun untuk MVP saat ini, SQL seed + curl walkthrough sudah cukup.

## Keputusan Codex

Keputusan produk dari Antigravity:

```text
APPROVED
```

Rencana lanjut E2E:

```text
APPROVED WITH TECHNICAL CONDITIONS
```

Syarat sebelum E2E dianggap lulus:

1. Seed cocok dengan migration aktual.
2. Tidak ada artifact scratch sementara yang tertinggal di source tree final.
3. Semua skenario critical path Mabar terbukti lewat HTTP request nyata.
4. `go test ./...` tetap lulus setelah cleanup.

Setelah E2E Mabar lulus, barulah kita bisa lanjut ke tahap berikutnya:

```text
Frontend integration untuk Open Match / Mabar.
```

Untuk saat ini, jangan mulai frontend dulu sebelum E2E report Mabar dikirim dan direview.
