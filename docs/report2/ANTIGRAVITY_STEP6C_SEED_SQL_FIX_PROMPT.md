# AntiGravity Prompt: Step 6C - Fix E2E Seed SQL Sport Reference

```text
Kamu bertindak sebagai Product-minded Senior Backend Engineer untuk project LapangGo.

Codex review atas Step 6B:
Sebagian besar artifact sudah benar, tetapi Step 6B belum approved karena `docs/qa/step6_e2e_seed.sql` masih punya blocker pada seed `sports`.

Masalah wajib diperbaiki:

1. Migration `db/migrations/001_init_core.sql` mendefinisikan tabel `sports` hanya dengan kolom:
   - `id`
   - `name`
   - `status`
   - `created_at`

   Tidak ada kolom `updated_at`.

   Tetapi seed saat ini memakai:

   ```sql
   INSERT INTO sports (id, name, created_at, updated_at)
   ```

   Ini akan gagal.

2. Migration juga sudah menjalankan:

   ```sql
   INSERT INTO sports (name) VALUES ('Futsal'), ...
   ON CONFLICT (name) DO NOTHING;
   ```

   Karena `sports.name` unik, seed Step 6B yang mencoba insert fixed UUID untuk `Futsal` bisa gagal dengan unique violation jika `Futsal` sudah ada dengan UUID berbeda.

Perbaikan yang diharapkan:

1. Jangan insert fixed UUID untuk sport `Futsal`.
2. Ambil `sport_id` dari row `sports` yang sudah ada berdasarkan `name = 'Futsal'`.
3. Gunakan pendekatan SQL yang executable dan idempotent, misalnya:
   - CTE `WITH futsal_sport AS (...)`
   - atau subquery `(SELECT id FROM sports WHERE name = 'Futsal')`
4. Pastikan insert `courts` memakai sport ID hasil lookup tersebut.
5. Jika ingin tetap memastikan `Futsal` ada, gunakan insert yang cocok dengan schema:

   ```sql
   INSERT INTO sports (name, status)
   VALUES ('Futsal', 'ACTIVE')
   ON CONFLICT (name) DO UPDATE SET status = EXCLUDED.status;
   ```

   Lalu lookup ID-nya dari `sports`.

6. Perbaiki `ON CONFLICT DO UPDATE` agar seed repeatable:
   - users: update `name`, `phone`, `password_hash`, `role`, `status`, `updated_at`
   - owner_profiles: update `business_name`, bank fields, `verification_status`, `updated_at`
   - venues: update `owner_profile_id`, name/details/location/status, `updated_at`
   - courts: update `venue_id`, `sport_id`, name/details/location/price/status, `updated_at`
   - court_operating_hours: update `open_time`, `close_time`, `is_closed`, `updated_at`

Scope:
- Boleh ubah:
  - `docs/qa/step6_e2e_seed.sql`
  - `docs/CHATGPT_STEP6_E2E_QA_REPORT.md`
- Jika perlu, boleh update `docs/qa/STEP6_E2E_BOOKING_FLOW_QA.md`, tetapi hanya bila ada perubahan relevan.
- Jangan ubah source Go.
- Jangan ubah migration.
- Jangan ubah README.
- Jangan tambahkan helper binary/script.

Verifikasi wajib:
- Pastikan tidak ada lagi `apps/api/gen_hash.go`.
- Pastikan tidak ada lagi `apps/api/gen_hash.test.exe`.
- Jalankan:

  ```powershell
  cd apps/api
  go test ./...
  ```

Catatan Windows:
Jika `go test ./...` diblokir Windows Application Control, gunakan terminal Administrator/elevated.

Report:
Update:
`docs/CHATGPT_STEP6_E2E_QA_REPORT.md`

Isi report wajib:
1. File yang berubah.
2. Ringkasan blocker sport seed yang diperbaiki.
3. Konfirmasi seed SQL cocok dengan migration `sports`.
4. Konfirmasi walkthrough masih cocok dengan DTO backend.
5. Status E2E: executed atau pending manual run.
6. Hasil `go test ./...`.
7. Risiko tersisa.
```
