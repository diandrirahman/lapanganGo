# Staff Roles v1.3 Fix Implementation Plan

Status: READY FOR FIX IMPLEMENTATION
Target agent: Antigravity / coding agent
Scope: memperbaiki temuan review implementasi Staff Roles v1.3 tanpa refactor besar di luar authz staff.

## Tujuan
Memastikan staff role benar-benar usable dan aman:
- Staff memakai effective owner untuk resource milik owner.
- Staff memakai actor user ID untuk audit.
- Staff hanya bisa mengakses venue yang diberikan.
- Permission name konsisten backend, DB, dan frontend.
- Staff user yang inactive/suspended tidak bisa login atau memakai token lama.
- UI staff management sesuai kontrak API.

## Temuan yang Harus Diperbaiki

1. Owner route untuk staff masih memakai staff user ID sebagai owner ID.
2. Assignment venue staff belum mengecek venue milik owner yang sama.
3. Venue scope staff sudah disimpan di context tapi belum diterapkan ke query resource owner.
4. Permission refund tidak konsisten: `REFUNDS_*` vs `REFUND_REQUESTS_*`.
5. Status `users.status` milik staff belum menjadi gate login/owner workspace.
6. Frontend staff list membaca response yang salah.
7. Frontend memanggil endpoint revoke yang tidak ada.
8. Copy UI venue access salah: kosong disebut semua venue, padahal seharusnya no access.
9. Belum ada test baru untuk staff/owneraccess.

---

## Prinsip Perbaikan

- Jangan mengandalkan frontend untuk security.
- Untuk owner-owned resource:
  - `effective_owner_user_id` = owner yang memiliki workspace.
  - `actor_user_id` = user yang melakukan aksi, bisa owner atau staff.
  - `owner_profile_id` = profile owner workspace.
  - `allowed_venue_ids` = venue scope staff; owner tidak dibatasi.
- Empty `allowed_venue_ids` untuk staff berarti tidak ada akses venue.
- Direct resource out-of-scope harus return `404` atau `403` secara konsisten. Rekomendasi: `404` untuk anti-IDOR.
- List endpoint staff tanpa venue access harus return list kosong, bukan semua data.

---

## Backend Plan

### 1. Samakan Permission Refund

Pilih satu nama permission dan pakai di semua layer. Rekomendasi: ikuti plan awal dan migration:
- `REFUNDS_READ`
- `REFUNDS_WRITE`

Ubah:
- `apps/api/internal/refunds/handler.go`
  - `REFUND_REQUESTS_READ` -> `REFUNDS_READ`
  - `REFUND_REQUESTS_WRITE` -> `REFUNDS_WRITE`
- `apps/web/src/types/staff.ts`
  - `REFUND_REQUESTS_READ` -> `REFUNDS_READ`
  - `REFUND_REQUESTS_WRITE` -> `REFUNDS_WRITE`
- `apps/web/src/components/Navbar.tsx`
  - `REFUND_REQUESTS_READ` -> `REFUNDS_READ`
- Semua route guard atau UI permission lain yang memakai nama lama.

Acceptance:
- Staff dengan `REFUNDS_READ` bisa membuka halaman refund.
- Staff dengan `REFUNDS_WRITE` bisa approve/reject refund.
- Staff tanpa permission tersebut mendapat `403`.

### 2. Enforce Staff User Status

File:
- `apps/api/internal/auth/service.go`
- `apps/api/internal/owneraccess/repository.go`
- `apps/api/internal/middleware/owner_access.go`

Perubahan:
- Login harus menolak user dengan `users.status != 'ACTIVE'`.
  - Return error generic seperti credential invalid atau error auth generik, jangan leak status account.
- `owneraccess.GetStaffContextByUserID` harus join staff user row juga.
  - Saat ini hanya join owner user `u`.
  - Tambahkan join `users staff_user ON staff_user.id = m.user_id`.
  - Return `StaffUserStatus`.
- `OwnerWorkspaceAccess` untuk role `STAFF` harus reject jika:
  - `StaffUserStatus != ACTIVE`
  - `StaffStatus != ACTIVE`
  - `OwnerStatus != ACTIVE`

Acceptance:
- Staff user `SUSPENDED` tidak bisa login.
- Token lama staff `SUSPENDED` mendapat `403` di `/owner/*`.
- Staff membership `INACTIVE` tetap mendapat `403`.

### 3. Buat Owner Context Struct Untuk Service Layer

Tambahkan struct shared, bisa di `apps/api/internal/httputil` atau package baru seperti `ownercontext`:

```go
type OwnerContext struct {
    ActorUserID          string
    EffectiveOwnerUserID string
    OwnerProfileID       string
    IsOwner              bool
    AllowedVenueIDs      []string
}
```

Tambahkan helper:
- `httputil.GetOwnerContext(c) (OwnerContext, bool)`

Aturan:
- `ActorUserID` dari `auth_actor_user_id`.
- `EffectiveOwnerUserID` dari `auth_effective_owner_user_id`.
- `OwnerProfileID` dari `auth_owner_profile_id`.
- `IsOwner` dari `auth_is_owner`.
- `AllowedVenueIDs` dari `auth_staff_venue_ids`.

### 4. Refactor Handler Owner Agar Tidak Pakai `GetAuthenticatedUserID`

Ganti handler owner yang sekarang masih memakai `GetAuthenticatedUserID()` dengan owner context.

Prioritas file:
- `apps/api/internal/venues/handler.go`
- `apps/api/internal/courts/handler.go`
- `apps/api/internal/schedules/handler.go`
- `apps/api/internal/blockedslots/handler.go`
- `apps/api/internal/bookings/handler.go`
- `apps/api/internal/finance/handler.go`
- `apps/api/internal/promos/handler.go`
- `apps/api/internal/refunds/handler.go`
- `apps/api/internal/analytics/handler.go`

Pola:
- Untuk read/write resource owner: pakai `OwnerContext.EffectiveOwnerUserID` atau `OwnerProfileID`.
- Untuk audit fields: pakai `OwnerContext.ActorUserID`.
- Jangan pakai staff user ID sebagai owner ID.

Contoh:
- `VerifyPayment(ctx, ownerUserID, bookingID, approved)` menjadi salah satu:
  - `VerifyPayment(ctx, ownerCtx, bookingID, approved)`, atau
  - `VerifyPayment(ctx, effectiveOwnerUserID, actorUserID, bookingID, approved)`.

Acceptance:
- Staff dengan permission benar tidak lagi mendapat `owner profile not found`.
- Owner existing behavior tetap sama.

### 5. Terapkan Venue Scope Staff di Service/Repository

Minimal target v1.3 fix: semua endpoint owner yang memakai venue/court/booking/refund/finance/promo/analytics harus membatasi staff ke `AllowedVenueIDs`.

Aturan query:
- Jika `OwnerContext.IsOwner == true`: tidak perlu filter `AllowedVenueIDs`.
- Jika staff dan `AllowedVenueIDs` kosong:
  - list endpoint return kosong.
  - direct endpoint return not found / forbidden.
- Jika staff punya venue IDs:
  - semua query harus join ke venue dan filter `venue_id = ANY($allowedVenueIDs)`.

Area yang wajib:

#### Venues
- `ListVenues`: staff hanya list assigned venues.
- `GetVenue`, `UpdateVenue`, `UpdateVenueStatus`, venue photos: staff hanya assigned venue.
- `CreateVenue`: staff dengan `VENUES_WRITE` sebaiknya dilarang di v1.3, kecuali product mengizinkan. Rekomendasi fix: owner-only untuk create venue karena venue baru belum punya staff assignment.

#### Courts / Schedules / Blocked Slots
- Join court -> venue.
- Staff hanya bisa read/write court pada assigned venues.

#### Bookings
- Join booking -> court -> venue.
- Staff hanya list/mutate bookings pada assigned venues.
- Offline booking request `venue_id` harus assigned to staff.
- Ledger/audit dari offline booking:
  - `owner_id = effectiveOwnerUserID`
  - `created_by_user_id = actorUserID`

#### Refunds
- List/approve/reject refund harus join booking -> court -> venue atau memakai venue ID terkait.
- Staff hanya refund untuk assigned venues.
- `reviewed_by_user_id = actorUserID`.

#### Finance
- Staff read/write hanya transaksi dengan `venue_id` assigned.
- Staff tidak boleh melihat/mutasi transaksi `venue_id IS NULL`.
- Manual transaction create oleh staff wajib menyertakan `venue_id` assigned.
- `owner_id = effectiveOwnerUserID`.
- `created_by_user_id = actorUserID`.

#### Promos
- Staff hanya promo dengan `venue_id` assigned.
- Global promo `venue_id IS NULL` owner-only.

#### Analytics
- Staff analytics difilter assigned venues.
- Staff tanpa venue assignment return zeroed response, bukan 500.

Acceptance:
- Staff venue A tidak bisa melihat/mengubah venue B.
- Staff tanpa venue assignment tidak melihat data owner.
- Owner tetap melihat semua data.

### 6. Validasi Venue Assignment Staff

File:
- `apps/api/internal/staff/repository.go`
- `apps/api/internal/staff/service.go`

Perubahan:
- Sebelum insert ke `owner_staff_venue_access`, validasi semua `venue_ids` milik `owner_profile_id`.
- Lakukan validasi di transaction yang sama.
- Jangan insert satu-per-satu tanpa validasi ownership.

Rekomendasi query:
```sql
SELECT COUNT(*)
FROM venues
WHERE owner_profile_id = $1
  AND id = ANY($2::uuid[])
```

Jika count != jumlah unique venue IDs:
- return `ErrInvalidVenueAccess`.
- handler map ke `400` atau `404`.

Wajib diterapkan di:
- `CreateStaff`
- `UpdateStaff`
- `UpdateVenues`

Acceptance:
- Owner A tidak bisa assign venue milik Owner B.
- Duplicate venue IDs tidak membuat duplicate insert error; normalize/deduplicate dulu.

### 7. Perbaiki Response Contract Staff API

Pilih satu kontrak dan samakan frontend-backend. Rekomendasi mengikuti pola existing owner venues:
- `GET /owner/staff` return:
```json
{ "staff": [...] }
```

Opsi lain boleh array langsung, tapi frontend harus disesuaikan. Rekomendasi terbaik: backend wrap dengan key `staff` agar lebih konsisten dengan existing API.

Ubah:
- `apps/api/internal/staff/handler.go` `ListStaff`
  - dari `c.JSON(http.StatusOK, staffList)`
  - menjadi `c.JSON(http.StatusOK, gin.H{"staff": staffList})`

Acceptance:
- Halaman staff menampilkan staff list setelah fetch.

### 8. Hapus atau Implement Revoke Staff

Saat ini frontend memanggil:
- `POST /owner/staff/:id/revoke`

Backend tidak punya route ini.

Pilih salah satu:
1. Sederhana v1.3 fix: hapus tombol revoke dari frontend, gunakan status `INACTIVE`.
2. Implement route revoke:
   - Tambah enum `REVOKED` butuh migration tambahan, tidak disarankan untuk fix cepat.

Rekomendasi:
- Hapus tombol revoke dan semua UI status `REVOKED`.
- Tombol Power `ACTIVE/INACTIVE` sudah cukup untuk v1.3.

Acceptance:
- Tidak ada button yang memanggil endpoint tidak terdaftar.

### 9. Perbaiki Copy Venue Access

File:
- `apps/web/src/components/owner/StaffModal.tsx`

Ubah copy:
- Dari: `Jika dikosongkan, staff dapat mengakses semua venue.`
- Menjadi: `Jika dikosongkan, staff tidak dapat mengakses data venue mana pun.`

Jangan kirim `venue_ids: undefined` untuk empty selection kalau backend membedakan absent vs empty. Kirim array kosong eksplisit:
- create: `venue_ids: formData.venue_ids || []`
- update: `venue_ids: formData.venue_ids || []`

Acceptance:
- Owner paham empty venue berarti no access.
- Staff yang dibuat tanpa venue tidak melihat data venue.

---

## Test Plan

### Backend Unit Tests

Tambahkan test untuk:
- `apps/api/internal/middleware/owner_access.go`
  - OWNER active passes.
  - STAFF active user + active membership + active owner passes.
  - STAFF user suspended rejected.
  - STAFF membership inactive rejected.
  - Owner user suspended rejected.
  - Missing permission rejected.
  - Matching permission accepted.

- `apps/api/internal/staff`
  - create staff success.
  - duplicate email -> conflict.
  - assign venue from same owner success.
  - assign venue from another owner rejected.
  - update venues from another owner rejected.
  - empty venue IDs accepted as no access.

### Backend Integration/Service Tests

Tambahkan minimal tests untuk:
- Staff with `VENUES_READ` and venue A can list only venue A.
- Staff with `BOOKINGS_READ` and venue A cannot list venue B booking.
- Staff with `PAYMENT_VERIFY` cannot verify booking from venue B.
- Staff with `FINANCE_READ` cannot see `venue_id IS NULL`.
- Staff with `PROMOS_WRITE` cannot create global promo.
- Staff actor ID is stored as `created_by_user_id` for offline booking/finance manual transaction.

### Frontend Build/Manual QA

Run:
- `cd apps/web && cmd /c npm run build`

Manual QA:
- Owner can open `/owner/staff`, see staff list, create staff, activate/deactivate staff.
- Revoke button no longer exists unless backend route exists.
- Staff with `BOOKINGS_READ` sees Pesanan nav.
- Staff with `REFUNDS_READ` sees Refund nav.
- Staff without permission cannot access direct URL.
- Staff without assigned venue sees empty owner data, not all owner data.

---

## Verification Commands

Backend:
```powershell
cd apps/api
go test ./...
```

Frontend:
```powershell
cd apps/web
cmd /c npm run build
```

Optional full-stack:
```powershell
docker compose up --build -d
.\scripts\smoke_test.ps1
```

---

## Acceptance Criteria

- Owner existing flows tetap pass.
- Staff tidak lagi gagal karena owner profile milik staff tidak ditemukan.
- Staff route memakai effective owner ID, bukan staff user ID.
- Audit/action fields memakai actor user ID.
- Owner tidak bisa assign staff ke venue owner lain.
- Staff hanya bisa akses assigned venues.
- Staff tanpa assigned venue tidak melihat semua data.
- Refund permissions konsisten di DB/backend/frontend.
- Staff user suspended/inactive tidak bisa login atau memakai token lama untuk `/owner/*`.
- Staff list page menampilkan data.
- Tidak ada frontend action yang memanggil endpoint nonexistent.
- `go test ./...` pass.
- `cmd /c npm run build` pass.

---

## Stop Conditions

Jangan lanjut merge jika salah satu ini masih terjadi:
- Staff venue A bisa akses venue B.
- Staff user `SUSPENDED` masih bisa akses owner route.
- Permission refund masih memakai dua nama berbeda.
- Frontend staff page kosong padahal API mengembalikan data.
- Tidak ada test yang membuktikan anti-IDOR venue assignment.
