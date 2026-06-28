# Roadmap Frontend Mabar Untuk Antigravity

Halo Antigravity,

Codex sudah approve:

```text
Backend Mabar MVP
E2E QA Mabar
Frontend Step 1: Open Match Discovery / Card List
Frontend Step 1 Visual Revision
```

Sekarang frontend Mabar bisa dilanjutkan bertahap. Jangan kerjakan semua sekaligus. Ikuti urutan step di bawah agar scope tetap aman dan mudah direview.

## Status Saat Ini

Sudah selesai:

- `apps/web` memakai Vite + React + TypeScript + Tailwind CSS v4.
- `GET /open-matches` sudah dipakai untuk list/card.
- Loading, error, empty, populated state sudah ada.
- Mock visual QA tersedia via:

```text
VITE_USE_MOCK_MABAR=true
```

Jangan rusak atau refactor besar Step 1 kecuali diperlukan oleh step berikutnya.

---

# Step 2: Open Match Detail Page

## Tujuan

User bisa membuka detail satu Open Match dari card/list.

## Scope

Buat halaman/detail view untuk endpoint:

```http
GET /open-matches/:id
```

Karena routing belum ada, untuk Step 2 boleh mulai menambahkan routing ringan memakai:

```text
react-router-dom
```

Route minimum:

```text
/              -> Open Match Discovery
/open-matches/:id -> Open Match Detail
```

## UI Detail Minimal

Tampilkan:

- title
- description
- host_name
- sport_name
- venue_name
- court_name
- match_date
- start_time
- end_time
- level
- price_per_player
- max_players
- joined_count
- remaining_slots
- status
- participants list

## Behavior

- Klik card atau tombol/card action dari list menuju detail page.
- Detail page punya tombol back/kembali.
- Loading, error, not found, dan empty participants state wajib ada.
- Tombol `Gabung Match` masih boleh disabled/placeholder. Jangan implement join API dulu di Step 2.

## Jangan Kerjakan

- Join API.
- Leave API.
- Cancel API.
- Create Open Match form.
- Auth/session flow.
- Payment participant.

## Acceptance Criteria

- Route `/open-matches/:id` berjalan.
- Fetch detail dari API asli.
- Participants list tampil.
- Responsive desktop/mobile.
- `npm.cmd run lint` lulus.
- `npm.cmd run build` lulus.

## Expected Report

Kirim:

```text
docs/CODEX_MABAR_FRONTEND_STEP2_REPORT.md
git status --short
npm.cmd run lint output
npm.cmd run build output
```

---

# Step 3: Auth Token Handling Untuk Customer Action

## Tujuan

Menyiapkan frontend agar bisa menjalankan action customer seperti join/leave di step berikutnya.

## Scope

Buat mekanisme sederhana untuk menyimpan dan membaca token auth.

Untuk MVP/dev, boleh pakai:

```text
localStorage
```

atau dev token input panel sederhana.

## UI Minimal

Buat area kecil/dev-friendly untuk:

- melihat apakah token tersedia,
- input/paste token customer,
- clear token.

Nama env/config jangan hardcode token.

## API Helper

Update `src/lib/api.ts` agar punya helper request authenticated:

```text
Authorization: Bearer <token>
```

## Jangan Kerjakan

- Login/register UI penuh.
- Owner auth.
- Join/leave action final.
- Payment.

## Acceptance Criteria

- Token bisa disimpan dan dipakai oleh API helper.
- Tidak ada token hardcoded.
- UI tetap rapi dan tidak mengganggu discovery/detail.
- `npm.cmd run lint` lulus.
- `npm.cmd run build` lulus.

## Expected Report

Kirim:

```text
docs/CODEX_MABAR_FRONTEND_STEP3_AUTH_REPORT.md
git status --short
npm.cmd run lint output
npm.cmd run build output
```

---

# Step 4: Join Open Match

## Tujuan

Customer bisa join open match dari detail page.

## Endpoint

```http
POST /open-matches/:id/join
Authorization: Bearer <CUSTOMER_TOKEN>
```

## Scope

Implement tombol `Gabung Match` di detail page.

## Behavior

- Jika belum ada token, tampilkan pesan bahwa user perlu token/login.
- Jika match `OPEN`, tombol aktif.
- Jika match `FULL`, tombol disabled.
- Jika match `CANCELLED`, tombol disabled.
- Setelah join sukses:
  - refresh detail,
  - participants list bertambah,
  - joined_count/remaining_slots update.
- Jika API mengembalikan error:
  - host cannot join,
  - already joined,
  - match full,
  - booking not confirmed,
  - tampilkan pesan user-friendly.

## Jangan Kerjakan

- Leave.
- Cancel.
- Create Open Match.
- Payment.

## Acceptance Criteria

- Join sukses dari detail page.
- Error state join jelas.
- UI tidak double-submit saat request berjalan.
- Detail data refresh setelah join.
- `npm.cmd run lint` lulus.
- `npm.cmd run build` lulus.

## Expected Report

Kirim:

```text
docs/CODEX_MABAR_FRONTEND_STEP4_JOIN_REPORT.md
git status --short
npm.cmd run lint output
npm.cmd run build output
```

---

# Step 5: Leave Open Match

## Tujuan

Participant yang sudah join bisa keluar dari open match.

## Endpoint

```http
DELETE /open-matches/:id/join
Authorization: Bearer <CUSTOMER_TOKEN>
```

## Scope

Tambahkan tombol `Keluar dari Match` jika user saat ini terdeteksi sudah menjadi participant.

Karena belum ada `/auth/me` integration penuh, boleh gunakan token + participant user id jika sudah bisa dibaca dari JWT, atau pendekatan MVP yang paling sederhana dan jelas.

## Behavior

- Leave sukses refresh detail.
- Jika sebelumnya `FULL`, status bisa kembali `OPEN`.
- Error `not joined` ditampilkan jelas.

## Jangan Kerjakan

- Cancel host.
- Create Open Match.
- Payment.

## Acceptance Criteria

- Leave sukses.
- Detail refresh.
- Button state tidak membingungkan.
- `npm.cmd run lint` lulus.
- `npm.cmd run build` lulus.

## Expected Report

Kirim:

```text
docs/CODEX_MABAR_FRONTEND_STEP5_LEAVE_REPORT.md
git status --short
npm.cmd run lint output
npm.cmd run build output
```

---

# Step 6: Create Open Match Dari Booking CONFIRMED

## Tujuan

Host/customer bisa membuat Open Match dari booking yang sudah `CONFIRMED`.

## Endpoint

```http
POST /bookings/:id/open-matches
Authorization: Bearer <CUSTOMER_TOKEN>
```

## UI

Buat form:

- booking_id
- title
- description
- level
- max_players
- price_per_player

Untuk MVP, booking_id boleh diinput manual dulu jika belum ada halaman booking history.

Level harus mengikuti backend:

```text
Beginner / Fun
Intermediate
Advanced
All Levels
```

## Validation

Frontend validasi:

- title required
- level required
- max_players > 0
- price_per_player >= 0
- booking_id required

## Behavior

- Success: redirect ke detail match baru.
- Error:
  - booking not found,
  - booking not confirmed,
  - booking not owned by user,
  - duplicate open match,
  - tampilkan pesan jelas.

## Jangan Kerjakan

- Booking history page penuh.
- Payment participant.
- Host approval.

## Acceptance Criteria

- Form create bekerja.
- Validasi frontend ada.
- Error API terbaca rapi.
- Success redirect/detail.
- `npm.cmd run lint` lulus.
- `npm.cmd run build` lulus.

## Expected Report

Kirim:

```text
docs/CODEX_MABAR_FRONTEND_STEP6_CREATE_REPORT.md
git status --short
npm.cmd run lint output
npm.cmd run build output
```

---

# Step 7: Host Cancel Open Match

## Tujuan

Host bisa membatalkan open match.

## Endpoint

```http
PATCH /open-matches/:id/cancel
Authorization: Bearer <CUSTOMER_TOKEN>
```

## Scope

Tambahkan action cancel di detail page untuk host.

Karena auth/ownership detection mungkin belum sempurna, implementasi MVP boleh:

- tampilkan tombol hanya di dev mode/token owner detection jika memungkinkan, atau
- tampilkan dengan guard API, lalu error unauthorized ditangani rapi.

## Behavior

- Konfirmasi sebelum cancel.
- Cancel sukses refresh detail.
- Status menjadi `CANCELLED`.
- Join button disabled setelah cancel.

## Acceptance Criteria

- Cancel sukses dengan token host.
- Participant/non-host mendapat error rapi.
- Tidak ada accidental cancel tanpa confirmation.
- `npm.cmd run lint` lulus.
- `npm.cmd run build` lulus.

## Expected Report

Kirim:

```text
docs/CODEX_MABAR_FRONTEND_STEP7_CANCEL_REPORT.md
git status --short
npm.cmd run lint output
npm.cmd run build output
```

---

# Step 8: Frontend E2E Manual QA Mabar

## Tujuan

Membuktikan frontend + backend berjalan dari awal sampai akhir.

## Flow Wajib

1. Seed/backend punya booking `CONFIRMED`.
2. User buka list Mabar.
3. User buka detail Mabar.
4. Customer join.
5. Customer leave.
6. Host create mabar dari booking.
7. Host cancel mabar.
8. UI menampilkan status dan error dengan benar.

## Report

Buat:

```text
docs/CODEX_MABAR_FRONTEND_E2E_REPORT.md
```

Isi:

- cara setup backend,
- cara setup frontend,
- env yang dipakai,
- token/test user yang dipakai, boleh disensor,
- screenshot/deskripsi tiap flow,
- bug jika ada,
- lint/build result,
- git status.

---

# Catatan Umum Untuk Semua Step

## Jangan Lakukan

Jangan menambah:

- payment participant,
- approval host,
- chat,
- notification,
- upload avatar,
- frontend framework baru,
- global state library besar kecuali benar-benar perlu,
- backend schema changes tanpa approval Codex.

## Jaga Kualitas UI

- Jangan ada mojibake/karakter rusak.
- Jangan hardcode token production.
- Jangan hardcode data production.
- Mock data hanya untuk dev visual QA dengan env guard.
- Footer harus tetap berada di bawah viewport saat konten pendek.
- Layout harus responsive.
- Jangan pakai browser `alert()` untuk UX utama.

## Urutan Approval

Antigravity harus mengirim report setelah setiap step.

Codex/User akan review sebelum lanjut step berikutnya.

Urutan yang disarankan:

```text
Step 2 Detail Page
Step 3 Auth Token Handling
Step 4 Join
Step 5 Leave
Step 6 Create
Step 7 Cancel
Step 8 Frontend E2E Manual QA
```

Jika waktu terbatas, prioritas MVP frontend adalah:

```text
Step 2 -> Step 3 -> Step 4
```

Karena list + detail + join adalah core loop paling penting untuk user Mabar.
