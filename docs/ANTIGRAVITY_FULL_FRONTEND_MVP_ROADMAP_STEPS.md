# Roadmap Lengkap Frontend MVP LapanganGo Untuk Antigravity

Halo Antigravity,

Dokumen ini adalah roadmap lengkap frontend MVP LapanganGo.

Status saat ini:

```text
Backend booking MVP sudah tersedia.
Backend dummy payment sudah tersedia.
Backend owner dashboard booking management sudah tersedia.
Backend Mabar/Open Match sudah tersedia dan E2E-approved.
Frontend Step 1 Mabar Discovery sudah approved.
```

Frontend jangan dikerjakan sekaligus besar. Ikuti step kecil di bawah agar mudah direview.

---

# Phase 0: Frontend Foundation

## Step 0.1: App Shell & Design System

Status:

```text
Sebagian sudah selesai di apps/web.
```

Pastikan tersedia:

- Vite + React + TypeScript.
- Tailwind CSS v4.
- Navbar.
- Footer sticky bottom.
- Layout responsive.
- API base config:

```text
VITE_API_BASE_URL=http://localhost:8080
```

Komponen dasar yang disarankan:

```text
Button
Input
Select
Badge
Card
EmptyState
ErrorState
LoadingState
PageShell
```

Acceptance:

- `npm.cmd run lint` lulus.
- `npm.cmd run build` lulus.
- Tidak ada mojibake.
- Tidak ada sisa template Vite.

---

# Phase 1: Public Customer Flow

## Step 1: Homepage / Venue Discovery

Tujuan:

User bisa melihat halaman awal untuk mencari lapangan.

UI:

- Hero/search area.
- Section venue/lapangan populer.
- Section Mabar discovery yang sudah ada.
- CTA booking.

API yang bisa dipakai jika tersedia:

```http
GET /owner/venues
GET /owner/venues/:id
```

Catatan:

Jika public venue endpoint belum tersedia, jangan pakai endpoint owner sebagai public UX final tanpa diskusi. Buat UI scaffold dulu atau minta backend public venue endpoint.

Acceptance:

- Homepage rapi.
- Mabar section tetap berfungsi.
- Tidak ada hardcoded production data.

Expected report:

```text
docs/CODEX_FRONTEND_STEP1_HOMEPAGE_REPORT.md
```

---

## Step 2: Auth UI - Register, Login, Me

Tujuan:

Customer bisa login/register dan token disimpan frontend.

Endpoint:

```http
POST /auth/register
POST /auth/login
GET /auth/me
```

UI:

- Login page.
- Register page.
- Auth state di navbar.
- Logout.

Token:

- Simpan di `localStorage` untuk MVP.
- Jangan hardcode token.

Acceptance:

- Register customer berhasil.
- Login berhasil.
- Navbar berubah sesuai auth state.
- Logout membersihkan token.

Expected report:

```text
docs/CODEX_FRONTEND_STEP2_AUTH_REPORT.md
```

---

## Step 3: Venue Detail + Court List

Tujuan:

User bisa melihat detail venue dan daftar court.

UI:

- Venue name, address, city.
- Court cards.
- Sport/type/price.
- CTA pilih jadwal.

Endpoint:

```http
GET /owner/venues/:id
GET /owner/venues/:id/courts
```

Catatan:

Jika endpoint masih owner-only, perlu diskusi apakah backend harus membuat public venue/court endpoint. Jangan menabrak auth/role backend.

Acceptance:

- Detail venue tampil.
- Court list tampil.
- Empty/error/loading state ada.

Expected report:

```text
docs/CODEX_FRONTEND_STEP3_VENUE_DETAIL_REPORT.md
```

---

## Step 4: Court Availability View

Tujuan:

User bisa melihat slot available/booked/blocked untuk court tertentu.

Endpoint:

```http
GET /courts/:id/availability?date=YYYY-MM-DD
```

UI:

- Date picker.
- Slot grid/list.
- Status slot:

```text
AVAILABLE
BOOKED
BLOCKED
```

Rules:

- `AVAILABLE` bisa dipilih.
- `BOOKED` disabled.
- `BLOCKED` disabled.

Acceptance:

- Slot tampil sesuai status.
- User bisa memilih slot available.
- Date change refetch availability.

Expected report:

```text
docs/CODEX_FRONTEND_STEP4_AVAILABILITY_REPORT.md
```

---

## Step 5: Create Booking

Tujuan:

Customer bisa booking lapangan.

Endpoint:

```http
POST /bookings
Authorization: Bearer <CUSTOMER_TOKEN>
```

Payload backend:

```json
{
  "court_id": "...",
  "booking_date": "YYYY-MM-DD",
  "start_time": "10:00",
  "end_time": "11:00"
}
```

UI:

- Booking summary.
- Court/date/time.
- Total price jika tersedia.
- Confirm booking button.

Acceptance:

- Booking berhasil.
- Error slot conflict ditampilkan.
- Setelah berhasil, user diarahkan ke booking detail/list.

Expected report:

```text
docs/CODEX_FRONTEND_STEP5_CREATE_BOOKING_REPORT.md
```

---

## Step 6: Customer Booking List & Detail

Tujuan:

Customer bisa melihat booking miliknya.

Endpoint:

```http
GET /bookings
GET /bookings/:id
```

UI:

- Booking list.
- Status badge:

```text
PENDING_PAYMENT
CONFIRMED
CANCELLED
PAID
```

- Detail booking.

Acceptance:

- Booking list tampil.
- Detail booking tampil.
- Empty/error/loading state.

Expected report:

```text
docs/CODEX_FRONTEND_STEP6_CUSTOMER_BOOKINGS_REPORT.md
```

---

## Step 7: Dummy Payment Confirm

Tujuan:

Customer bisa menjalankan dummy payment untuk mengubah booking menjadi `CONFIRMED`.

Endpoint:

```http
POST /bookings/:id/pay
Authorization: Bearer <CUSTOMER_TOKEN>
```

UI:

- Tombol `Konfirmasi Pembayaran`.
- Copy harus jelas bahwa ini dummy/MVP, bukan payment gateway nyata.

Acceptance:

- `PENDING_PAYMENT` bisa dikonfirmasi.
- Status berubah menjadi `CONFIRMED`.
- Booking `CONFIRMED/CANCELLED/PAID` tidak bisa diproses dan error tampil rapi.

Expected report:

```text
docs/CODEX_FRONTEND_STEP7_DUMMY_PAYMENT_REPORT.md
```

---

## Step 8: Cancel Booking

Tujuan:

Customer bisa cancel booking miliknya yang masih `PENDING_PAYMENT`.

Endpoint:

```http
PATCH /bookings/:id/cancel
Authorization: Bearer <CUSTOMER_TOKEN>
```

UI:

- Confirmation sebelum cancel.
- Status refresh setelah cancel.

Acceptance:

- `PENDING_PAYMENT` bisa cancel.
- `CONFIRMED`, `PAID`, `CANCELLED` ditolak dan error tampil rapi.

Expected report:

```text
docs/CODEX_FRONTEND_STEP8_CANCEL_BOOKING_REPORT.md
```

---

# Phase 2: Mabar/Open Match Frontend

## Step 9: Open Match Discovery

Status:

```text
DONE / APPROVED
```

Sudah ada:

- `GET /open-matches`.
- Card list.
- Loading/error/empty/populated state.
- Mock visual QA dengan `VITE_USE_MOCK_MABAR=true`.

---

## Step 10: Open Match Detail Page

Tujuan:

User bisa membuka detail match.

Endpoint:

```http
GET /open-matches/:id
```

UI:

- Detail match.
- Participants.
- Status/slot.
- Back button.

Acceptance:

- Route `/open-matches/:id`.
- Participants tampil.
- Belum join dulu.

Expected report:

```text
docs/CODEX_MABAR_FRONTEND_STEP2_REPORT.md
```

---

## Step 11: Join Open Match

Endpoint:

```http
POST /open-matches/:id/join
Authorization: Bearer <CUSTOMER_TOKEN>
```

Acceptance:

- Customer bisa join.
- Detail refresh.
- Error handled:
  - host cannot join,
  - already joined,
  - match full,
  - booking not confirmed.

Expected report:

```text
docs/CODEX_MABAR_FRONTEND_STEP4_JOIN_REPORT.md
```

---

## Step 12: Leave Open Match

Endpoint:

```http
DELETE /open-matches/:id/join
Authorization: Bearer <CUSTOMER_TOKEN>
```

Acceptance:

- Participant bisa leave.
- Detail refresh.
- Status `FULL` bisa kembali `OPEN`.

Expected report:

```text
docs/CODEX_MABAR_FRONTEND_STEP5_LEAVE_REPORT.md
```

---

## Step 13: Create Open Match

Endpoint:

```http
POST /bookings/:id/open-matches
Authorization: Bearer <CUSTOMER_TOKEN>
```

UI:

- booking_id
- title
- description
- level
- max_players
- price_per_player

Acceptance:

- Create berhasil dari booking `CONFIRMED`.
- Success redirect ke detail.
- Error duplicate/non-confirmed/not-owned tampil rapi.

Expected report:

```text
docs/CODEX_MABAR_FRONTEND_STEP6_CREATE_REPORT.md
```

---

## Step 14: Cancel Open Match

Endpoint:

```http
PATCH /open-matches/:id/cancel
Authorization: Bearer <CUSTOMER_TOKEN>
```

Acceptance:

- Host bisa cancel.
- Non-host mendapat error rapi.
- Join disabled setelah cancel.

Expected report:

```text
docs/CODEX_MABAR_FRONTEND_STEP7_CANCEL_REPORT.md
```

---

# Phase 3: Owner Frontend

## Step 15: Owner Profile

Tujuan:

Owner bisa membuat dan melihat profile.

Endpoint:

```http
POST /owner/profile
GET /owner/profile
PUT /owner/profile
```

UI:

- Business name.
- Identity/bank fields jika tersedia.
- Verification status.

Acceptance:

- Owner profile CRUD dasar berjalan.
- Role error tampil rapi jika bukan owner.

Expected report:

```text
docs/CODEX_FRONTEND_STEP15_OWNER_PROFILE_REPORT.md
```

---

## Step 16: Owner Venue Management

Endpoint:

```http
POST /owner/venues
GET /owner/venues
GET /owner/venues/:id
PUT /owner/venues/:id
PATCH /owner/venues/:id/status
```

UI:

- Venue list.
- Create/edit venue form.
- Status badge.
- Publish/disable action.

Acceptance:

- Owner bisa manage venue.
- Validation/error state.

Expected report:

```text
docs/CODEX_FRONTEND_STEP16_OWNER_VENUES_REPORT.md
```

---

## Step 17: Owner Court Management

Endpoint:

```http
POST /owner/venues/:id/courts
GET /owner/venues/:id/courts
GET /owner/courts/:id
PUT /owner/courts/:id
PATCH /owner/courts/:id/status
```

UI:

- Court list per venue.
- Court create/edit.
- Sport/location type/price/status.

Acceptance:

- Owner bisa manage court.

Expected report:

```text
docs/CODEX_FRONTEND_STEP17_OWNER_COURTS_REPORT.md
```

---

## Step 18: Owner Operating Hours

Endpoint:

```http
GET /owner/courts/:id/operating-hours
PUT /owner/courts/:id/operating-hours
```

UI:

- Day-of-week schedule editor.
- Open/close time.
- Closed toggle.

Acceptance:

- Operating hours bisa dilihat dan diupdate.

Expected report:

```text
docs/CODEX_FRONTEND_STEP18_OWNER_OPERATING_HOURS_REPORT.md
```

---

## Step 19: Owner Blocked Slots

Endpoint:

```http
POST /owner/courts/:id/blocked-slots
GET /owner/courts/:id/blocked-slots
DELETE /owner/blocked-slots/:id
```

UI:

- Block slot form.
- Blocked slots list.
- Delete blocked slot.

Acceptance:

- Owner bisa block maintenance slot.
- Availability customer ikut berubah.

Expected report:

```text
docs/CODEX_FRONTEND_STEP19_OWNER_BLOCKED_SLOTS_REPORT.md
```

---

## Step 20: Owner Booking Dashboard

Endpoint:

```http
GET /owner/venues/:id/bookings?date=YYYY-MM-DD&status=PENDING_PAYMENT
```

UI:

- Filter date.
- Filter status.
- Booking table.
- Pagination jika backend mendukung.

Acceptance:

- Owner hanya melihat booking venue miliknya.
- Filter bekerja.

Expected report:

```text
docs/CODEX_FRONTEND_STEP20_OWNER_BOOKINGS_REPORT.md
```

---

# Phase 4: Final Integration & QA

## Step 21: Navigation & Role-Based UX Polish

Tujuan:

Merakit semua halaman menjadi app yang koheren.

Scope:

- Customer nav.
- Owner nav.
- Auth-aware navbar.
- Protected route handling.
- Unauthorized/forbidden pages.

Acceptance:

- Customer dan owner flow tidak tercampur.
- User tanpa token diarahkan dengan jelas.

Expected report:

```text
docs/CODEX_FRONTEND_STEP21_NAV_ROLE_REPORT.md
```

---

## Step 22: Full Frontend E2E Manual QA

Flow wajib:

1. Register/login customer.
2. Lihat venue/court.
3. Lihat availability.
4. Create booking.
5. Dummy payment confirm.
6. Booking muncul di list customer.
7. Create open match dari booking confirmed.
8. User lain join open match.
9. User leave open match.
10. Host cancel open match.
11. Owner lihat booking venue.
12. Owner block slot dan availability berubah.

Report:

```text
docs/CODEX_FULL_FRONTEND_E2E_REPORT.md
```

Isi:

- setup backend/frontend,
- env,
- seed/test data,
- screenshot/deskripsi flow,
- bug list,
- lint/build result,
- git status.

---

## Step 23: MVP Polish & Release Readiness

Scope:

- Copywriting final.
- Empty/error/loading consistency.
- Mobile check.
- Broken link check.
- Remove unused assets.
- Ensure no mock enabled by default.
- Ensure `.env` not committed.

Acceptance:

- `npm.cmd run lint` pass.
- `npm.cmd run build` pass.
- Backend `go test ./...` pass.
- No hardcoded token.
- No mojibake.
- No obvious layout break mobile/desktop.

Report:

```text
docs/CODEX_FRONTEND_MVP_RELEASE_READINESS_REPORT.md
```

---

# Prioritas Jika Waktu Terbatas

Jika tidak bisa mengerjakan semua sekaligus, urutan MVP paling penting:

```text
Step 2 Auth
Step 4 Availability
Step 5 Create Booking
Step 7 Dummy Payment
Step 10 Mabar Detail
Step 11 Join Mabar
Step 22 Full Frontend E2E
```

Untuk Mabar khusus:

```text
Step 10 Detail
Step 11 Join
Step 12 Leave
Step 13 Create
Step 14 Cancel
```

Untuk owner dashboard:

```text
Step 15 Owner Profile
Step 16 Venue
Step 17 Court
Step 18 Operating Hours
Step 19 Blocked Slots
Step 20 Booking Dashboard
```

# Aturan Umum

Setiap step harus mengirim report sebelum lanjut.

Report minimal:

```text
1. File yang dibuat/diubah.
2. Cara menjalankan.
3. Env yang dibutuhkan.
4. Hasil lint/build.
5. Screenshot/deskripsi UI.
6. Limitations.
7. git status --short.
```

Jangan lakukan:

- Payment gateway real.
- Split payment participant.
- Host approval.
- Chat/notification.
- Upload avatar.
- Backend schema change tanpa approval.
- Hardcoded token.
- Hardcoded production data.

Mock data hanya boleh untuk visual QA dengan env guard, misalnya:

```text
VITE_USE_MOCK_MABAR=true
```
