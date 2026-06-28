# Codex Handoff: LapangGo Current State

Gunakan file ini untuk melanjutkan pekerjaan di chat Codex baru.

## Cara Pakai di Chat Baru

Mulai chat Codex baru dengan pesan:

```text
Saya ingin lanjut dari chat Codex sebelumnya di project D:\project\lapangGo.

Baca dulu file:
docs/CODEX_HANDOFF_CURRENT_STATE.md

Konteks penting:
- Workflow saya pakai AntiGravity step-by-step.
- Codex bertindak sebagai Product Manager + expert software developer reviewer.
- Jangan langsung implementasi besar tanpa review step.
- Kalau run go test di Windows, gunakan elevated/admin karena Windows Application Control.
- Lanjutkan dari status terakhir di handoff ini.
```

## Peran dan Workflow

- User memakai AntiGravity untuk mengerjakan task bertahap.
- Codex bertindak sebagai reviewer Product Manager + expert software developer.
- Pola kerja:
  1. Codex buat prompt untuk AntiGravity.
  2. AntiGravity mengerjakan satu step.
  3. User kirim report AntiGravity ke Codex.
  4. Codex review.
  5. Kalau approved, lanjut step berikutnya.
- Jangan gabungkan banyak step sekaligus kecuali user minta eksplisit.

## Instruksi Penting Test

User meminta:

> Jalanin test nanti kalau pakai PowerShell atau cmd atau git bash pakai run as administrator biar tidak diblok sama Windows Application Control.

Jadi saat menjalankan test:

```powershell
cd apps/api
go test ./...
```

Gunakan mode elevated/admin. Di Codex, gunakan escalated command jika tersedia.

## Status Fitur yang Sudah Approved

### 1. Availability + Booking Sync

Status: approved.

Behavior:
- Availability membaca booking aktif.
- Slot dengan booking aktif menjadi `BOOKED`.
- Booking `CANCELLED` tidak memblokir availability.
- README sudah mendokumentasikan status:
  - `AVAILABLE`
  - `BLOCKED`
  - `BOOKED`

### 2. Cancellation API

Status: approved.

Endpoint:

```text
PATCH /bookings/:id/cancel
```

Business rule:
- Auth required.
- Role `CUSTOMER`.
- Customer hanya bisa cancel booking miliknya.
- Hanya status `PENDING_PAYMENT` yang bisa cancel.
- `CONFIRMED`, `PAID`, `CANCELLED` ditolak.
- Atomic update memakai filter:
  - `id`
  - `customer_id`
  - `status = 'PENDING_PAYMENT'`
- Race fallback/refetch sudah ada.

### 3. Owner Dashboard Booking Management

Status: approved.

Endpoint:

```text
GET /owner/venues/:id/bookings?date=YYYY-MM-DD&status=PENDING_PAYMENT
```

Business rule:
- Auth required.
- Role `OWNER`.
- Owner hanya bisa melihat booking venue miliknya.
- Query mendukung:
  - `date`
  - `status`
  - `limit`
  - `page`

### 4. Dummy Payment / Confirm Booking

Status: approved.

Endpoint:

```text
POST /bookings/:id/pay
```

Business rule:
- Auth required.
- Role `CUSTOMER`.
- Customer hanya bisa confirm booking miliknya.
- Hanya booking `PENDING_PAYMENT` yang bisa diproses.
- Status final MVP adalah `CONFIRMED`.
- Tidak ada tabel `payments`.
- Tidak ada migration baru.
- Atomic update memakai filter:
  - `id`
  - `customer_id`
  - `status = 'PENDING_PAYMENT'`
- Jika atomic update gagal karena race, service refetch lalu map error.

Catatan:
- Step 4 sempat punya bug memakai `c.GetString("user_id")`.
- Step 4B sudah memperbaiki handler agar memakai helper `getAuthenticatedUserID(c)`.
- Step 5B sudah memperbaiki README yang sempat terduplikasi.

## Status Terakhir: Step 6 E2E QA

Step 6 belum approved.

AntiGravity membuat:

- `docs/qa/step6_e2e_seed.sql`
- `docs/qa/STEP6_E2E_BOOKING_FLOW_QA.md`
- `docs/CHATGPT_STEP6_E2E_QA_REPORT.md`

Review Codex:
- Arahnya benar: perlu E2E Booking Flow QA / API Smoke Test.
- Tapi artifact QA belum executable terhadap skema dan DTO backend saat ini.

Masalah yang ditemukan:

1. `docs/qa/step6_e2e_seed.sql` memakai nama kolom yang tidak cocok dengan migration:
   - `users.phone_number` seharusnya `users.phone`
   - `owner_profiles.id_card_number` tidak ada
   - `owner_profiles.verified_status` seharusnya `verification_status`
   - `owner_profiles` wajib punya `business_name`
   - `venues.owner_id` seharusnya `owner_profile_id`
   - `venues.contact_phone` dan `venues.contact_email` tidak ada
   - `courts.type` seharusnya `location_type`
   - `courts` wajib punya `sport_id`

2. `docs/qa/STEP6_E2E_BOOKING_FLOW_QA.md` memakai payload booking yang salah.

   Backend DTO saat ini:

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
     "date": "YYYY-MM-DD",
     "start_time": "10:00:00",
     "end_time": "11:00:00"
   }
   ```

3. Login response backend memakai field:

   ```json
   "token"
   ```

   bukan `access_token`.

4. Availability response memakai:
   - `start_at`
   - `end_at`
   - `status`

   Jadi walkthrough harus menjelaskan cara mencocokkan slot dari timestamp, bukan hanya jam mentah.

5. Ada artifact sementara di luar scope:
   - `apps/api/gen_hash.go`
   - `apps/api/gen_hash.test.exe`

   Keduanya harus dihapus oleh AntiGravity atau dibersihkan sebelum final.

## Prompt Terakhir untuk AntiGravity

Prompt perbaikan Step 6B sudah dibuat di:

```text
docs/ANTIGRAVITY_STEP6B_QA_ARTIFACT_FIX_PROMPT.md
```

Berikan file itu ke AntiGravity.

Expected report setelah AntiGravity selesai:

```text
docs/CHATGPT_STEP6_E2E_QA_REPORT.md
```

## Acceptance Step 6B

Step 6B boleh approved hanya jika:

- Seed SQL cocok dengan migration saat ini.
- Walkthrough curl cocok dengan DTO/response backend saat ini.
- Tidak ada `apps/api/gen_hash.go`.
- Tidak ada `apps/api/gen_hash.test.exe`.
- Tidak ada perubahan source Go/migration/README.
- `go test ./...` lulus.
- Jika E2E tidak benar-benar dijalankan, report harus jujur menyatakan `pending manual run`.

## File Penting

Prompt diskusi dan implementasi:

- `docs/ANTIGRAVITY_NEXT_STEP_DISCUSSION_PROMPT.md`
- `docs/ANTIGRAVITY_STEP6_E2E_QA_EXECUTION_PROMPT.md`
- `docs/ANTIGRAVITY_STEP6B_QA_ARTIFACT_FIX_PROMPT.md`

Report terkait:

- `docs/CHATGPT_NEXT_STEP6_E2E_QA_DISCUSSION_REPORT.md`
- `docs/CHATGPT_STEP6_E2E_QA_REPORT.md`

QA artifacts yang sedang perlu diperbaiki:

- `docs/qa/step6_e2e_seed.sql`
- `docs/qa/STEP6_E2E_BOOKING_FLOW_QA.md`

## Repo State Notes

Worktree saat handoff masih dirty dan banyak file source Go sudah modified dari pekerjaan sebelumnya. Jangan revert perubahan user/AntiGravity.

`git status --short` terakhir menunjukkan banyak modified files, termasuk:

- `README.md`
- `apps/api/cmd/api/main.go`
- banyak file di `apps/api/internal/*`
- untracked `docs/`
- untracked artifact sementara:
  - `apps/api/gen_hash.go`
  - `apps/api/gen_hash.test.exe`

Jangan pakai `git reset --hard`.
Jangan revert file source kecuali user minta eksplisit.

## Rekomendasi Next Action di Chat Baru

1. Baca file ini.
2. Baca `docs/ANTIGRAVITY_STEP6B_QA_ARTIFACT_FIX_PROMPT.md`.
3. Jika user belum menjalankan AntiGravity, minta user kasih prompt itu ke AntiGravity.
4. Jika user sudah punya report Step 6B, review:
   - `docs/CHATGPT_STEP6_E2E_QA_REPORT.md`
   - `docs/qa/step6_e2e_seed.sql`
   - `docs/qa/STEP6_E2E_BOOKING_FLOW_QA.md`
   - `git status --short`
5. Jalankan:

   ```powershell
   cd apps/api
   go test ./...
   ```

   dengan elevated/admin.

6. Putuskan:
   - approve Step 6B, atau
   - buat prompt perbaikan lanjutan.
