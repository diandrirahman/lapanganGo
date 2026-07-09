# Staff Roles v1.3 Implementation Plan

Status: READY FOR IMPLEMENTATION
Target agent: Antigravity / coding agent
Repo context: LapanganGo monorepo, Go Gin API + React Vite frontend

## Tujuan
Menambahkan akses staff untuk workspace owner tanpa melemahkan ownership guard yang sudah ada. Staff dapat login sebagai user sendiri, masuk ke area `/owner/*`, dan hanya melihat/menjalankan fitur sesuai permission serta venue yang diberikan owner.

Prinsip utama v1.3:
- `OWNER` tetap pemilik bisnis dan memiliki semua akses.
- `STAFF` adalah role global baru pada tabel `users`, tetapi hak operasionalnya ditentukan oleh membership di owner workspace.
- Permission staff harus dicek di backend untuk setiap endpoint owner. Frontend hanya untuk UX, bukan sumber keamanan.
- Token lama staff harus otomatis kehilangan akses setelah staff dinonaktifkan atau permission/venue access dicabut. Karena itu permission tidak boleh hanya dipercaya dari JWT.

## Scope v1.3
Dalam scope:
- Tambah role global `STAFF`.
- Tambah tabel staff membership, permission, dan venue scope.
- Tambah API owner untuk mengelola staff.
- Owner route existing dapat diakses staff sesuai permission dan venue scope.
- Frontend owner dapat digunakan staff dengan menu/action yang difilter permission.
- Unit/integration test untuk authz, IDOR, status staff, dan permission matrix.

Di luar scope:
- Email invitation delivery.
- Forced password reset on first staff login.
- Owner-driven staff password reset and forgot-password flow for staff.
- Multi-owner membership untuk satu akun staff.
- Super admin panel.
- Audit log umum untuk seluruh aksi owner. v1.3 hanya memastikan kolom existing seperti `created_by_user_id` / `reviewed_by_user_id` memakai actor user, bukan effective owner.

Known limitation v1.3:
- `POST /owner/staff` creates a staff account with a password entered by the owner. This means the owner can know the initial staff password. Because invite delivery, forced first-login password change, and reset-password flows are out of scope, this must be documented in release notes and revisited before production onboarding is treated as self-service.

## Permission Model
Tambahkan role global:
- `STAFF`

Tambahkan staff role label:
- `MANAGER`
- `CASHIER`
- `OPERATIONS`

Permission adalah source of truth. Staff role hanya preset awal dan label UI.

Permission keys:
- `DASHBOARD_VIEW`
- `ANALYTICS_READ`
- `VENUES_READ`
- `VENUES_WRITE`
- `COURTS_READ`
- `COURTS_WRITE`
- `SCHEDULE_READ`
- `SCHEDULE_WRITE`
- `BLOCKED_SLOTS_READ`
- `BLOCKED_SLOTS_WRITE`
- `BOOKINGS_READ`
- `BOOKINGS_WRITE`
- `OFFLINE_BOOKINGS_CREATE`
- `PAYMENT_VERIFY`
- `REFUNDS_READ`
- `REFUNDS_WRITE`
- `FINANCE_READ`
- `FINANCE_WRITE`
- `PROMOS_READ`
- `PROMOS_WRITE`

Owner-only capabilities, not assignable to staff:
- Create/update owner profile.
- Update owner bank account fields.
- Manage staff.
- Delete/deactivate owner workspace.

Recommended presets:
- `MANAGER`: all assignable permissions except `FINANCE_WRITE`.
- `CASHIER`: `DASHBOARD_VIEW`, `BOOKINGS_READ`, `BOOKINGS_WRITE`, `OFFLINE_BOOKINGS_CREATE`, `PAYMENT_VERIFY`, `REFUNDS_READ`, `FINANCE_READ`.
- `OPERATIONS`: `DASHBOARD_VIEW`, `VENUES_READ`, `COURTS_READ`, `COURTS_WRITE`, `SCHEDULE_READ`, `SCHEDULE_WRITE`, `BLOCKED_SLOTS_READ`, `BLOCKED_SLOTS_WRITE`, `BOOKINGS_READ`.

## Database Migration
Create migration `016_staff_roles.up.sql` and `016_staff_roles.down.sql`.

Up migration:
1. Add enum value:
   - `ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'STAFF';`
   - Repo uses `golang-migrate/migrate/v4` and Docker uses Postgres 16. Verify this migration in CI or local Docker before implementing code that inserts `users.role = 'STAFF'`.
   - Do not use the new `STAFF` enum value in the same migration file for seed data, defaults, generated columns, constraints, or indexes. This avoids PostgreSQL enum visibility issues if the migration runner wraps the file in a transaction.
2. Create enum `owner_staff_status`:
   - `ACTIVE`
   - `INACTIVE`
3. Create enum `owner_staff_role`:
   - `MANAGER`
   - `CASHIER`
   - `OPERATIONS`
4. Create enum `owner_staff_permission` with all permission keys above.
5. Create table `owner_staff_members`:
   - `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
   - `owner_profile_id UUID NOT NULL REFERENCES owner_profiles(id) ON DELETE CASCADE`
   - `user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE`
   - `role owner_staff_role NOT NULL`
   - `permissions owner_staff_permission[] NOT NULL DEFAULT '{}'`
   - `status owner_staff_status NOT NULL DEFAULT 'ACTIVE'`
   - `created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT`
   - `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
   - `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`
6. Create table `owner_staff_venue_access`:
   - `staff_member_id UUID NOT NULL REFERENCES owner_staff_members(id) ON DELETE CASCADE`
   - `venue_id UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE`
   - `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
   - primary key `(staff_member_id, venue_id)`
7. Add indexes:
   - `idx_owner_staff_members_owner_profile_id`
   - `idx_owner_staff_members_user_id`
   - `idx_owner_staff_members_status`
   - `idx_owner_staff_venue_access_venue_id`
8. Add DB guard:
   - Unique staff email remains enforced by `users.email`.
   - Service layer must reject `owner_staff_members.user_id` if target user role is not `STAFF`.
   - Service layer must reject venue assignments where venue owner_profile_id differs from staff owner_profile_id.

Down migration:
- Drop `owner_staff_venue_access`.
- Drop `owner_staff_members`.
- Do not remove `STAFF` from `user_role` enum because PostgreSQL enum value removal is unsafe in normal down migrations. Document this in the down file comment.
- Drop new staff-specific enums only if no dependent objects remain.

## Backend Implementation

### 1. Auth DTO, JWT, and `/auth/me`
Files:
- `apps/api/internal/auth/dto.go`
- `apps/api/internal/auth/service.go`
- `apps/api/internal/auth/repository.go`
- `apps/api/internal/auth/jwt.go`
- `apps/api/internal/middleware/auth.go`

Changes:
- Allow login for `STAFF` users exactly like existing users, but reject login if `users.status != 'ACTIVE'`.
- Keep JWT claims minimal: `user_id`, `email`, `role`.
- Do not put staff permissions or venue IDs in JWT.
- Extend `UserResponse` with optional `staff_context`:
  - `owner_profile_id`
  - `owner_user_id`
  - `staff_member_id`
  - `staff_role`
  - `permissions`
  - `venue_ids`
  - `staff_status`
- `/auth/me` must return fresh `staff_context` from DB for `STAFF`. If staff membership is missing or inactive, return user data plus no owner access context, and owner routes must reject it.

### 2. Owner Access Middleware
Add middleware/repository helper, recommended package:
- `apps/api/internal/middleware/owner_access.go`
- optionally `apps/api/internal/owneraccess/repository.go`

Behavior:
- Replace direct owner route guard `RequireRole("OWNER")` with owner workspace middleware that allows `OWNER` and `STAFF`.
- For `OWNER`:
  - Resolve owner profile by `auth_user_id`.
  - Set context keys:
    - `auth_actor_user_id`
    - `auth_effective_owner_user_id`
    - `auth_owner_profile_id`
    - `auth_is_owner = true`
    - all permissions allowed.
- For `STAFF`:
  - Load active membership by staff `user_id`.
  - Join to `owner_profiles` to resolve `owner_profile_id`, owner `user_id`, and `verification_status`.
  - Join owner `users` and reject access if the owner user status is not `ACTIVE`.
  - Load venue access IDs.
  - Reject if staff membership not found, `status != ACTIVE`, or user status not active.
  - Set context keys:
    - `auth_actor_user_id`
    - `auth_effective_owner_user_id`
    - `auth_owner_profile_id`
    - `auth_owner_verification_status`
    - `auth_staff_member_id`
    - `auth_staff_permissions`
    - `auth_staff_venue_ids`
    - `auth_is_owner = false`
- Add helpers in `httputil`:
  - `GetActorUserID(c)`
  - `GetEffectiveOwnerUserID(c)`
  - `GetOwnerProfileID(c)`
  - `GetStaffVenueIDs(c)`
  - `IsWorkspaceOwner(c)`
- Add `RequireOwnerPermission(permission string)` middleware:
  - OWNER always passes.
  - STAFF passes only if permission is present.
  - Return `403` with consistent `{ "message": "You do not have permission to access this resource" }`.

Important:
- Permission check must run before handler logic.
- Venue-level scope must be enforced in service/repository, not only hidden in frontend.
- Existing schema has no `owner_profiles.status`; it has `verification_status` (`PENDING`, `APPROVED`, `REJECTED`). v1.3 should not silently invent a new workspace status. Preserve existing owner behavior unless product explicitly decides to gate owner routes by `verification_status = 'APPROVED'`.
- Even if `verification_status` is not used as an access gate in v1.3, staff access must still be blocked when the owner user row itself is `INACTIVE` or `SUSPENDED`.

### 3. Staff Management Module
Add package:
- `apps/api/internal/staff/dto.go`
- `apps/api/internal/staff/repository.go`
- `apps/api/internal/staff/service.go`
- `apps/api/internal/staff/handler.go`
- tests in same package.

Owner-only routes:
- `POST /owner/staff`
- `GET /owner/staff`
- `GET /owner/staff/:id`
- `PUT /owner/staff/:id`
- `PATCH /owner/staff/:id/status`
- `PUT /owner/staff/:id/venues`

Route guard:
- Must require authenticated owner workspace access.
- Must additionally require actual owner, not staff. Do not allow `STAFF_MANAGE` in v1.3.

Request/response:
- `POST /owner/staff`
  - Request: `name`, `email`, optional `phone`, `password`, `role`, `permissions`, `venue_ids`.
  - Backend validates password using existing strong password rule.
  - Create user with `role = STAFF`.
  - Create staff membership and venue access in one DB transaction.
  - Response returns staff representation without password.
- `GET /owner/staff`
  - Return staff list for current owner profile only.
- `PUT /owner/staff/:id`
  - Can update `name`, `phone`, `role`, `permissions`, `venue_ids`.
  - Must not update email in v1.3 to avoid account ownership ambiguity.
- `PATCH /owner/staff/:id/status`
  - Accept only `ACTIVE` or `INACTIVE`.
  - When set `INACTIVE`, existing JWT becomes unusable for owner routes because middleware reads DB every request.
- `PUT /owner/staff/:id/venues`
  - Replaces venue access atomically.
  - Every `venue_id` must belong to same `owner_profile_id`.

Errors:
- Duplicate email -> `409`.
- Invalid role/permission/status -> `400`.
- Staff not found within owner workspace -> `404`.
- Venue not owned by owner -> `400` or `404`; use `404` if avoiding resource existence leakage.
- DB FK/unique violations must be mapped, not leaked as `500`.

### 4. Route Permission Mapping
Update `apps/api/cmd/api/main.go` route registration so owner routes use owner workspace middleware and permission middleware.

Mapping:
- Owner profile:
  - `POST /owner/profile`, `GET /owner/profile`, `PUT /owner/profile`: OWNER only.
- Staff management:
  - `/owner/staff*`: OWNER only.
- Venues:
  - `GET /owner/venues`, `GET /owner/venues/:id`: `VENUES_READ`.
  - `POST /owner/venues`, `PUT /owner/venues/:id`, `PATCH /owner/venues/:id/status`, photo create/update/delete: `VENUES_WRITE`.
- Courts:
  - `GET /owner/venues/:id/courts`, `GET /owner/courts/:id`: `COURTS_READ`.
  - `POST /owner/venues/:id/courts`, `PUT /owner/courts/:id`, `PATCH /owner/courts/:id/status`: `COURTS_WRITE`.
- Schedules:
  - `GET /owner/courts/:id/operating-hours`: `SCHEDULE_READ`.
  - `PUT /owner/courts/:id/operating-hours`: `SCHEDULE_WRITE`.
- Blocked slots:
  - `GET /owner/courts/:id/blocked-slots`: `BLOCKED_SLOTS_READ`.
  - `POST /owner/courts/:id/blocked-slots`, `DELETE /owner/blocked-slots/:id`: `BLOCKED_SLOTS_WRITE`.
- Bookings:
  - `GET /owner/bookings`, `GET /owner/venues/:id/bookings`, `GET /owner/metrics`: `BOOKINGS_READ`.
  - `PATCH /owner/bookings/:id/complete`, `PATCH /owner/bookings/:id/cancel-refund`: `BOOKINGS_WRITE`.
  - `PATCH /owner/bookings/:id/verify-payment`, `PATCH /owner/bookings/:id/mark-paid`: `PAYMENT_VERIFY`.
  - `POST /owner/bookings/offline`: `OFFLINE_BOOKINGS_CREATE`.
- Refunds:
  - `GET /owner/refund-requests`: `REFUNDS_READ`.
  - `PATCH /owner/refund-requests/:id/approve`, `PATCH /owner/refund-requests/:id/reject`: `REFUNDS_WRITE`.
- Finance:
  - `GET /owner/finance/summary`, `GET /owner/finance/transactions`: `FINANCE_READ`.
  - `POST /owner/finance/transactions`, `PATCH /owner/finance/transactions/:id`, `DELETE /owner/finance/transactions/:id`: `FINANCE_WRITE`.
- Promos:
  - `GET /owner/promos`, `GET /owner/promos/:id`: `PROMOS_READ`.
  - `POST /owner/promos`, `PUT /owner/promos/:id`, `PATCH /owner/promos/:id/toggle`, `DELETE /owner/promos/:id`: `PROMOS_WRITE`.
- Analytics:
  - `/owner/analytics/*`: `ANALYTICS_READ`.

### 5. Effective Owner and Actor Refactor
Existing owner handlers mostly call `httputil.GetAuthenticatedUserID(c)` and pass that ID as owner user ID. Change owner handlers to:
- Use `GetEffectiveOwnerUserID(c)` when querying or mutating owner-owned resources.
- Use `GetActorUserID(c)` for audit fields like `created_by_user_id` and `reviewed_by_user_id`.

Modules that need explicit actor handling:
- `bookings.OwnerCreateOfflineBooking`: use effective owner for ownership, actor for `owner_finance_transactions.created_by_user_id`.
- `bookings.VerifyPayment`, `MarkBookingPaid`, `CompleteBooking`, `CancelPaidBookingWithRefund`: use effective owner for ownership; if a ledger/review field is written, store actor user.
- `refunds.ApproveRefundRequest`, `RejectRefundRequest`: check request belongs to effective owner; set `reviewed_by_user_id` to actor user.
- `finance.CreateTransaction`: `owner_id = effective owner`, `created_by_user_id = actor user`.
- Existing read/list endpoints can use effective owner ID.

### 6. Venue Scope Enforcement
Staff venue access applies to all endpoints that reference venue/court/booking/promo/finance data.

Rules:
- OWNER: all venues under owner profile.
- STAFF with empty `venue_ids`: no venue-scoped data access. Return empty list for list endpoints and `404` for direct resource endpoints.
- STAFF with venue IDs: can only access resources belonging to those venues.

Implementation options:
- Preferred: add `OwnerContext` struct to services:
  - `ActorUserID`
  - `EffectiveOwnerUserID`
  - `OwnerProfileID`
  - `IsOwner`
  - `AllowedVenueIDs []string`
  - `Permissions []string`
- Repository queries for list endpoints add venue filter when `!IsOwner`.
- Direct endpoints must verify ownership and venue scope in the same query where possible.

Endpoint-specific scope:
- Venue endpoints: filter by `venues.id`.
- Court/schedule/blocked slot endpoints: join `courts -> venues`.
- Booking endpoints: join `bookings -> courts -> venues`.
- Refund endpoints: join refund booking/court/venue or use stored owner ID plus venue check.
- Finance endpoints: filter `owner_finance_transactions.venue_id`. Transactions with `venue_id IS NULL` are visible only to OWNER and staff with `FINANCE_READ` plus explicit decision: in v1.3, staff must not see owner-level `NULL` venue transactions.
- Promo endpoints:
  - Venue-specific promo: staff needs access to that venue.
  - Global promo with `venue_id IS NULL`: OWNER only in v1.3, because it affects all venues.
- Analytics: filter calculations to staff allowed venues. If no venues, return valid zeroed data, not `500`.

### 7. Frontend Implementation
Files likely affected:
- `apps/web/src/types/auth.ts`
- `apps/web/src/contexts/AuthContext.tsx`
- `apps/web/src/components/ProtectedRoute.tsx`
- `apps/web/src/components/Navbar.tsx`
- `apps/web/src/App.tsx`
- `apps/web/src/lib/api.ts`
- owner pages/components under `apps/web/src/pages/owner` and `apps/web/src/components/owner`

Changes:
- Extend `User` type with optional `staff_context`.
- Add permission helpers:
  - `hasOwnerWorkspaceAccess(user)`
  - `hasPermission(user, permission)`
  - `isWorkspaceOwner(user)`
- Update protected owner routes:
  - Accept `OWNER` or `STAFF` with active `staff_context`.
  - Redirect unauthorized staff to first allowed owner page, or `/` if none.
- Update navbar:
  - Show owner workspace nav for `OWNER` and active `STAFF`.
  - Hide links without permission.
  - Add `Staff` link only for `OWNER`.
  - Display staff label as `STAFF - CASHIER` etc.
- Add owner staff management page:
  - `/owner/staff`
  - List staff, create staff, edit role/permissions/venue access, deactivate/reactivate.
  - Owner only.
- Hide/disable write actions:
  - Create/edit venue requires `VENUES_WRITE`.
  - Court mutations require `COURTS_WRITE`.
  - Operating hours mutations require `SCHEDULE_WRITE`.
  - Blocked slots mutations require `BLOCKED_SLOTS_WRITE`.
  - Verify/mark-paid requires `PAYMENT_VERIFY`.
  - Complete/cancel-refund requires `BOOKINGS_WRITE`.
  - Offline booking button requires `OFFLINE_BOOKINGS_CREATE`.
  - Refund approval/rejection requires `REFUNDS_WRITE`.
  - Finance mutation requires `FINANCE_WRITE`; read-only finance page allowed by `FINANCE_READ`.
  - Promo mutations require `PROMOS_WRITE`; global promo create/edit must be owner-only or blocked for staff.

## Testing Plan

Backend unit tests:
- `middleware`:
  - OWNER passes owner access.
  - STAFF active membership passes.
  - STAFF inactive membership gets `403`.
  - STAFF gets `403` when owner user status is `INACTIVE` or `SUSPENDED`.
  - STAFF missing membership gets `403`.
  - CUSTOMER gets `403`.
  - Required permission missing gets `403`.
- `staff` service:
  - create staff success transaction.
  - duplicate email -> conflict.
  - invalid permission -> bad request.
  - venue assignment outside owner -> rejected.
  - status update inactive prevents owner route access.
- Existing modules:
  - owner still can access all existing owner endpoints.
  - staff with read permission can list scoped resources.
  - staff without write permission cannot mutate.
  - staff assigned venue A cannot access venue B court/booking/refund/finance/promo.
  - actor vs effective owner stored correctly in finance/refund/offline booking writes.

Backend integration/smoke tests:
- Migration `016_staff_roles` runs cleanly on a fresh Postgres 16 database through `golang-migrate`.
- Owner creates staff.
- Staff logs in.
- Staff sees only assigned venue bookings.
- Staff verifies payment if `PAYMENT_VERIFY`.
- Staff gets `403` on finance create without `FINANCE_WRITE`.
- Owner deactivates staff.
- Same staff token gets `403` on `/owner/bookings`.

Frontend tests/build:
- `npm run build`.
- Manual QA:
  - OWNER sees all owner nav including Staff.
  - CASHIER sees Pesanan, Refund read if assigned, Finance read if assigned, no Venue/Promo mutation.
  - OPERATIONS sees Venue/Court/Schedule/Blocked Slot screens but not Finance/Promo.
  - Staff direct URL to forbidden route redirects or shows unauthorized state without firing forbidden mutation loops.

Verification commands:
- Backend: `cd apps/api && go test ./...`
- Frontend: `cd apps/web && npm run build`
- Optional full stack: `docker compose up --build -d` then `.\scripts\smoke_test.ps1`

## Acceptance Criteria
- `OWNER` behavior remains backward compatible.
- `CUSTOMER` still cannot access `/owner/*`.
- `STAFF` can log in and access only allowed owner workspace capabilities.
- Staff permissions are enforced server-side.
- Staff venue scope prevents cross-venue IDOR.
- Disabling staff immediately blocks existing staff token from owner routes.
- Suspending or inactivating the owner user blocks staff owner-route access even if staff membership is active.
- Existing public/customer booking/mabar flows still pass tests.
- All new DB constraint violations have mapped HTTP errors.
- `go test ./...` and `npm run build` pass.

---

# Pre-Implementation Gap Check

## 1. Relasi & Foreign Key
- [x] Semua tabel/entity yang punya FK ke entity yang dimodifikasi sudah diidentifikasi:
  - `users` mendapat role baru `STAFF`.
  - `owner_profiles` menjadi parent staff workspace via `owner_staff_members.owner_profile_id`.
  - `venues` menjadi scope staff via `owner_staff_venue_access.venue_id`.
  - Existing owner-owned entities terdampak scope: `courts`, `court_operating_hours`, `court_blocked_slots`, `bookings`, `offline_booking_customers`, `booking_refund_requests`, `owner_finance_transactions`, `owner_promos`, `venue_photos`, analytics queries.
- [x] Status/enum minor pada entity yang berelasi sudah dipertimbangkan:
  - `users.status`: hanya `ACTIVE` boleh login dan membuka owner workspace.
  - `owner_staff_status`: hanya `ACTIVE` boleh membuka owner workspace.
  - `venue_status`: staff scope berlaku untuk `DRAFT`, `ACTIVE`, `INACTIVE`, `SUSPENDED` pada owner endpoints; public tetap hanya `ACTIVE`.
  - `court_status`: staff scope tetap berlaku pada `ACTIVE`, `INACTIVE`, `MAINTENANCE`.
  - Booking statuses yang harus tetap aman: `PENDING_PAYMENT`, `WAITING_VERIFICATION`, `CONFIRMED`, `PAID`, `COMPLETED`, `CANCELLED`, `EXPIRED`, refund-related statuses existing di service.
  - Refund statuses: `PENDING`, `APPROVED`, `REJECTED`.
  - Promo statuses: `ACTIVE`, `INACTIVE`.
- [x] Cascade behavior didefinisikan:
  - Jika `owner_profiles` dihapus, staff membership dan venue access ikut terhapus (`ON DELETE CASCADE`).
  - Jika staff `users` dihapus, membership ikut terhapus (`ON DELETE CASCADE`).
  - Jika venue dihapus, venue access ikut terhapus (`ON DELETE CASCADE`).
  - `created_by_user_id` memakai `ON DELETE RESTRICT` agar histori pembuat membership tidak hilang diam-diam.
- [x] Soft-delete vs hard-delete konsisten:
  - Staff tidak dihapus lewat API v1.3; gunakan `status = INACTIVE`.
  - Venue/court existing tetap memakai status, bukan delete endpoint umum.
  - Venue access boleh hard-replace karena hanya tabel mapping.

## 2. State & Enum
- [x] Semua state/enum yang mungkin sudah disebutkan eksplisit di plan:
  - Global role, staff status, staff role, staff permissions, venue/court/booking/refund/promo statuses.
- [x] Transisi state yang dilarang sudah didefinisikan:
  - Staff `INACTIVE` tidak boleh membuka owner route walaupun token masih valid.
  - Staff tidak boleh mengubah owner profile, bank info, atau staff lain.
  - Staff tidak boleh membuat/mengubah promo global `venue_id IS NULL`.
  - Staff tidak boleh melihat transaksi finance `venue_id IS NULL`.
  - Staff tanpa venue assignment tidak boleh mengakses direct resource; list endpoint return kosong.
- [x] Potensi race condition:
  - Create staff harus transaction: create `users`, `owner_staff_members`, dan `owner_staff_venue_access`.
  - Replace venue access harus transaction: delete old mapping + insert new mapping.
  - Permission/status updates harus segera efektif karena owner access middleware membaca DB setiap request.

## 3. Aggregate / Counter Consistency
- [x] Definisi counter/aggregate sama di semua tempat:
  - Owner metrics, finance summary, analytics, and booking counts must use effective owner ID.
  - Staff-scoped dashboard/analytics must filter by allowed venue IDs.
- [x] Counter di-refresh setelah operasi relevan:
  - Existing frontend owner pages should refetch after staff performs booking/payment/refund/finance/promo mutations, same as owner.
- [x] Caching aggregate:
  - Tidak ada caching aggregate eksplisit di repo saat ini. Jika Redis rate limiter tetap ada, tidak perlu invalidation aggregate.

## 4. Kontrak API / Response Consistency
- [x] Mutating endpoints return data representasi lengkap:
  - Staff create/update/status/venue assignment returns full staff representation.
  - Existing owner mutation responses remain compatible, but data must be scoped by effective owner.
- [x] Format error response konsisten:
  - Gunakan `{ "message": "..." }`, optional `{ "error": "..." }` untuk validation details mengikuti pola existing.
  - Jangan leak raw SQL/pg error ke response.
- [x] DB constraint violation dipetakan:
  - Duplicate staff email -> `409`.
  - Duplicate membership/user_id -> `409`.
  - Invalid FK venue/staff -> `404` or `400` as specified.
  - Invalid enum/permission/status -> `400`.
  - Forbidden permission/role -> `403`.

## 5. Validasi & Boundary
- [x] Batas angka/string:
  - `name`: min 2 max 120, trim.
  - `email`: valid email max 191, lowercase trim.
  - `phone`: optional numeric min 10 max 15, mengikuti auth register.
  - `password`: existing strong password rule, min 8 plus uppercase/lowercase/number/special.
  - `permissions`: non-null array, every item must be known enum.
  - `venue_ids`: may be empty; empty means no venue-scoped staff access.
- [x] Validasi lebih dari satu layer:
  - Frontend validates form UX.
  - Backend remains authoritative.
  - DB constraints backstop uniqueness/FK.
- [x] Definisi kosong/nol jelas:
  - Empty `venue_ids` valid but gives no venue access.
  - Empty `permissions` valid but staff cannot access owner feature pages.
  - Empty password invalid on create.
  - Email cannot be changed in v1.3.

## 6. Concurrency & Idempotency
- [x] Operasi idempotent:
  - `PUT /owner/staff/:id/venues` should be idempotent for the same sorted set of `venue_ids`.
  - `PATCH /owner/staff/:id/status` to current status should return success and current representation.
- [x] Operasi multi-step atomik:
  - Create staff is transaction.
  - Update staff role/permissions/venue access is transaction if venue access is included.
  - Replace venue access is transaction.
  - Existing booking/payment/refund/finance transactions must continue using existing transaction boundaries and use actor/effective owner IDs correctly.

## 7. Authorization / Ownership
- [x] Guard bukan cuma resource ada, tapi milik owner/role yang berhak:
  - Owner access middleware resolves effective owner and staff context.
  - Every direct resource endpoint verifies owner profile and staff venue scope.
  - Return `404` for out-of-scope direct resource to reduce IDOR leakage where practical.
- [x] Role-based access konsisten:
  - `/owner/*` no longer uses plain `RequireRole("OWNER")`; it uses owner workspace access + permission middleware.
  - Owner-only endpoints remain owner-only: profile management and staff management.
  - Frontend route/menu/action filtering mirrors backend permission mapping.

## 8. Backward Compatibility / Migrasi Data
- [x] Asumsi data lama:
  - Existing `CUSTOMER`, `OWNER`, `SUPER_ADMIN` users remain valid.
  - No existing staff records, so new tables start empty.
  - Existing owner endpoints must keep working for `OWNER`.
- [x] Default value kolom/field baru:
  - `owner_staff_members.permissions DEFAULT '{}'` means newly created staff has no accidental access unless service assigns permissions.
  - `owner_staff_members.status DEFAULT 'ACTIVE'` is acceptable only because create staff endpoint is owner-only and explicit. If future invite flow exists, add `INVITED`.
  - `/auth/me` optional `staff_context` must not break existing frontend for customer/owner.
  - Existing `owner_profiles` has `verification_status`, not a general active/suspended status. Plan decision: do not introduce new owner profile status in v1.3; use owner `users.status` as the hard access gate and preserve existing verification behavior.
  - Because staff passwords are created by owner in v1.3, release notes must call out that forced reset, owner reset, and forgot-password are not available yet.

---

## Ringkasan Temuan Setelah Gap Check

| # | Kategori | Celah yang Ditemukan | Skenario Konkret | Revisi Plan |
|---|----------|----------------------|-------------------|-------------|
| 1 | Authorization / Ownership | Kalau hanya menambah `STAFF` ke `RequireRole`, staff akan melewati semua owner route tanpa permission/venue scope. | Staff kasir membuka `/owner/finance` atau update venue dengan ID milik owner yang sama. | Plan menetapkan owner workspace middleware, `RequireOwnerPermission`, dan venue scope per endpoint. |
| 2 | Auth / Token Staleness | Kalau permission disimpan di JWT, staff yang baru dinonaktifkan masih bisa memakai token lama sampai expiry. | Owner deactivate staff, staff tetap verify payment memakai token lama. | Plan melarang permission/venue scope di JWT dan mewajibkan middleware membaca membership aktif dari DB setiap request. |
| 3 | Audit / Actor Identity | Existing service memakai owner user ID sebagai `created_by_user_id` / `reviewed_by_user_id`; staff action bisa tercatat sebagai owner. | Staff membuat offline booking, ledger finance tercatat dibuat owner sehingga audit salah. | Plan membedakan `actor_user_id` dan `effective_owner_user_id` serta menyebut modul yang wajib diubah. |
| 4 | Venue Scope / IDOR | Owner-level query existing hanya filter owner profile/owner ID; staff perlu filter venue agar tidak melihat semua venue owner. | Staff venue A memanggil `/owner/venues/{venueB}/bookings` atau booking ID venue B. | Plan menambahkan `AllowedVenueIDs` dan aturan join scope untuk venue/court/booking/refund/finance/promo/analytics. |
| 5 | Global Resources | Promo global dan finance transaction `venue_id IS NULL` tidak punya venue scope jelas. | Staff dengan akses satu venue membuat promo global yang berlaku ke semua venue, atau melihat transaksi owner-level. | Plan menetapkan global promo dan finance `venue_id IS NULL` sebagai OWNER-only di v1.3. |
| 6 | Migration / Rollback | PostgreSQL enum `user_role` sulit dihapus saat down migration. | Rollback migration mencoba remove enum value `STAFF` dan gagal. | Plan meminta down migration tidak menghapus enum value `STAFF`, cukup komentar eksplisit. |
| 7 | Frontend Guard | Frontend saat ini hard-code `requiredRole="OWNER"`, sehingga staff yang valid akan selalu ditendang. | Staff login berhasil tapi redirect ke `/` saat membuka `/owner/bookings`. | Plan menambahkan helper owner workspace access dan permission-based route/nav filtering. |
| 8 | Empty Venue Access | Kalau empty venue access ditafsirkan sebagai "all venues", staff baru bisa melihat semua venue tanpa assignment. | Owner membuat staff tapi lupa pilih venue; staff langsung melihat semua bookings. | Plan menetapkan empty `venue_ids` berarti no access: list kosong, direct resource `404`. |
| 9 | Migration Transaction | `ALTER TYPE user_role ADD VALUE 'STAFF'` bisa bermasalah jika value baru dipakai dalam migration yang sama dan runner membungkus file dalam transaction. | CI migration gagal sebelum aplikasi bisa insert user staff. | Plan meminta verifikasi dengan Postgres 16 + `golang-migrate` dan melarang penggunaan enum `STAFF` di migration yang sama. |
| 10 | Staff Password Lifecycle | Owner membuat password awal staff, tetapi tidak ada forced reset atau reset password flow. | Staff bertanya cara mengganti/lupa password, atau owner mengetahui password staff terlalu lama. | Plan mencatat ini sebagai known limitation dan explicit out-of-scope v1.3. |
| 11 | Owner Workspace Status | `owner_profiles` tidak punya status aktif/suspend, hanya `verification_status`; staff bisa ambigu saat owner user disuspend. | Owner user `SUSPENDED`, tetapi staff membership masih `ACTIVE` lalu mencoba akses `/owner/bookings`. | Plan memblokir staff jika owner `users.status` bukan `ACTIVE`, dan tidak menambah gate baru pada `verification_status` tanpa keputusan produk. |

## Implementation Order for Antigravity
1. Add migration `016_staff_roles`.
2. Add staff repository/service/handler and tests.
3. Add owner access middleware and `httputil` helpers.
4. Wire staff routes and replace owner route guards in `cmd/api/main.go`.
5. Refactor backend owner handlers/services to use effective owner + actor context.
6. Add venue scope filtering to repositories/services.
7. Update auth response and `/auth/me` staff context.
8. Update frontend auth types/context, route guards, nav, and owner action gating.
9. Add `/owner/staff` UI.
10. Run backend tests and frontend build.

## Stop Conditions
Do not continue implementation if:
- Migration cannot run cleanly from an empty DB.
- Owner existing tests fail and are not directly updated for the new owner context.
- Any staff route can access a direct resource outside allowed venue IDs.
- Staff inactive status does not immediately block existing token.
- Frontend build passes only by weakening backend authorization assumptions.
