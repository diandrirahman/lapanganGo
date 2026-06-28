# AntiGravity Prompt: Step 6B - Fix E2E QA Artifacts

```text
Kamu bertindak sebagai Product-minded Senior Backend Engineer untuk project LapangGo.

Codex review atas Step 6:
Arahnya sudah benar, tetapi hasil Step 6 belum approved karena artifact QA belum executable terhadap skema dan DTO backend saat ini.

Masalah utama yang wajib diperbaiki:

1. `docs/qa/step6_e2e_seed.sql` memakai nama kolom yang tidak cocok dengan migration saat ini.
   Contoh mismatch:
   - tabel `users` memakai `phone`, bukan `phone_number`
   - tabel `owner_profiles` wajib `business_name`, bukan `id_card_number` / `verified_status`
   - tabel `venues` memakai `owner_profile_id`, bukan `owner_id`
   - tabel `venues` tidak punya `contact_phone` / `contact_email`
   - tabel `courts` wajib `sport_id` dan `location_type`, bukan `type`

2. `docs/qa/STEP6_E2E_BOOKING_FLOW_QA.md` memakai payload booking yang salah.
   DTO backend saat ini:
   ```json
   {
     "court_id": "...",
     "booking_date": "YYYY-MM-DD",
     "start_time": "10:00",
     "end_time": "11:00"
   }
   ```
   Bukan:
   ```json
   {
     "date": "...",
     "start_time": "10:00:00",
     "end_time": "11:00:00"
   }
   ```

3. Response login memakai field:
   ```json
   "token"
   ```
   bukan `access_token`.

4. Availability response memakai `start_at` dan `end_at` berbentuk timestamp, jadi dokumen QA harus menjelaskan cara mencocokkan slot berdasarkan timestamp tersebut, bukan hanya string jam mentah.

5. Ada artifact sementara di luar scope:
   - `apps/api/gen_hash.go`
   - `apps/api/gen_hash.test.exe`

   Ini harus dihapus. Jangan tinggalkan helper generator atau binary test di repo.

Scope Step 6B:
- Perbaiki artifact QA saja.
- Boleh ubah:
  - `docs/qa/step6_e2e_seed.sql`
  - `docs/qa/STEP6_E2E_BOOKING_FLOW_QA.md`
  - `docs/CHATGPT_STEP6_E2E_QA_REPORT.md`
- Hapus artifact sementara:
  - `apps/api/gen_hash.go`
  - `apps/api/gen_hash.test.exe`
- Jangan ubah source Go lain.
- Jangan ubah migration.
- Jangan ubah README.
- Jangan tambah `cmd/seeder`.

Arahan seed SQL yang benar:
1. Seed harus idempotent dan non-destructive.
2. Jangan pakai `TRUNCATE`, `DELETE` massal, atau reset database.
3. Gunakan fixed UUID QA yang sudah ada boleh dipertahankan.
4. User:
   - insert ke kolom `phone`
   - password hash harus valid untuk password `QaPass123!`
   - role customer: `CUSTOMER`
   - role owner: `OWNER`
5. Owner profile:
   - gunakan kolom `business_name`
   - `verification_status` valid adalah `PENDING`, `APPROVED`, atau `REJECTED`
   - boleh set `APPROVED`
6. Venue:
   - gunakan `owner_profile_id`
   - status `ACTIVE`
7. Court:
   - ambil `sport_id` dari tabel `sports`, misalnya sport `Futsal`
   - gunakan `location_type = 'INDOOR'`
   - `price_per_hour > 0`
   - status `ACTIVE`
8. Operating hours:
   - gunakan `day_of_week` sesuai tanggal QA
   - boleh seed semua hari
   - isi `is_closed = false`
   - `open_time = '08:00'`, `close_time = '22:00'`
9. Jika membuat `ON CONFLICT DO UPDATE`, pastikan field penting ikut diperbarui agar seed repeatable.

Arahan walkthrough QA:
1. Perbaiki instruksi token:
   - response login field adalah `token`
2. Perbaiki payload `POST /bookings`:
   - gunakan `booking_date`
   - gunakan `start_time` / `end_time` format `HH:MM`
3. Tambahkan contoh untuk PowerShell dan Bash jika memungkinkan.
4. Jelaskan bahwa availability slot dicek dari array `slots`, field `start_at`, `end_at`, `status`.
5. Jangan klaim actual E2E passed jika belum benar-benar menjalankan API + DB.
6. Jika masih pending manual run, tulis jujur:
   - artifact prepared
   - not executed
   - reason

Verifikasi wajib:
- Jalankan:
  `cd apps/api`
  `go test ./...`

Catatan Windows:
Jika `go test ./...` diblokir Windows Application Control, gunakan terminal Administrator/elevated.

Acceptance Step 6B:
- Seed SQL cocok dengan migration saat ini.
- Walkthrough curl cocok dengan DTO/response backend saat ini.
- Tidak ada `apps/api/gen_hash.go`.
- Tidak ada `apps/api/gen_hash.test.exe`.
- Tidak ada perubahan source Go/migration/README.
- `go test ./...` lulus.

Report:
Update/buat ulang:
`docs/CHATGPT_STEP6_E2E_QA_REPORT.md`

Isi report wajib:
1. File yang berubah/dihapus.
2. Ringkasan mismatch yang diperbaiki.
3. Konfirmasi seed SQL sekarang sesuai skema.
4. Konfirmasi walkthrough sekarang sesuai DTO.
5. Status E2E:
   - executed atau pending manual
   - jangan mengarang hasil
6. Hasil `go test ./...`.
7. Risiko tersisa.
```
