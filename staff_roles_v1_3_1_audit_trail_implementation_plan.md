# Implementation Plan: Audit Trail Staff Roles v1.3.1

Tujuan dokumen ini adalah memberi instruksi implementasi yang jelas, bertahap, dan minim ruang interpretasi untuk Antigravity.

Fokus v1.3.1: owner harus bisa menjawab pertanyaan:

> Siapa melakukan aksi apa, terhadap data apa, kapan, dan dalam konteks workspace owner mana?

## Prinsip Implementasi

- Jangan ubah ulang arsitektur Staff Roles v1.3 yang sudah lulus E2E.
- Jangan menghapus atau mengganti field audit yang sudah ada seperti `created_by_user_id` dan `reviewed_by_user_id`.
- Tambahkan audit trail terpusat, bukan hanya menambah kolom kecil di banyak tabel.
- Audit log harus scoped per `owner_profile_id`.
- Staff tidak boleh bisa melihat audit log di v1.3.1. Endpoint audit log hanya untuk actual owner.
- Audit logging tidak boleh membuat aksi utama gagal, kecuali insert audit berada dalam transaksi yang sama dan memang bagian dari perubahan data kritikal.
- Kalau audit logging gagal di luar transaksi utama, log error dengan `log.Printf`, jangan panic.

---

## Phase 0 - Baseline Check

Sebelum coding, jalankan:

```powershell
cd D:\project\lapangGo\apps\api
$env:GOCACHE='D:\project\lapangGo\.gocache'
go test ./...
```

```powershell
cd D:\project\lapangGo\apps\web
cmd /c npm run build
cmd /c npm run lint
```

Catat warning yang sudah ada:

- `OwnerStaffPage.tsx` punya warning `react-hooks/exhaustive-deps` untuk `fetchData`.
- Vite build punya warning chunk size.

Jangan jadikan dua warning itu blocker untuk audit trail.

---

## Phase 1 - Migration Audit Logs

### File Baru

Buat migration baru:

```text
db/migrations/017_owner_audit_logs.up.sql
db/migrations/017_owner_audit_logs.down.sql
```

### Up Migration

```sql
CREATE TABLE owner_audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_profile_id UUID NOT NULL REFERENCES owner_profiles(id) ON DELETE CASCADE,
  actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  actor_role TEXT NOT NULL,
  action TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id UUID,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  ip_address TEXT,
  user_agent TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_owner_audit_logs_owner_created
  ON owner_audit_logs(owner_profile_id, created_at DESC);

CREATE INDEX idx_owner_audit_logs_actor_created
  ON owner_audit_logs(actor_user_id, created_at DESC);

CREATE INDEX idx_owner_audit_logs_action_created
  ON owner_audit_logs(action, created_at DESC);

CREATE INDEX idx_owner_audit_logs_entity
  ON owner_audit_logs(entity_type, entity_id);
```

### Down Migration

```sql
DROP TABLE IF EXISTS owner_audit_logs;
```

### Acceptance Criteria Phase 1

- Migration up berhasil.
- Migration down berhasil di DB kosong/dev.
- Tidak mengubah migration lama `016_staff_roles.*`.

---

## Phase 2 - Backend Package `audit`

### Folder Baru

Buat:

```text
apps/api/internal/audit/
```

### File Baru

```text
apps/api/internal/audit/dto.go
apps/api/internal/audit/repository.go
apps/api/internal/audit/service.go
apps/api/internal/audit/handler.go
```

### `dto.go`

Definisikan constants berikut.

Actions:

```go
const (
	ActionStaffCreated       = "STAFF_CREATED"
	ActionStaffUpdated       = "STAFF_UPDATED"
	ActionStaffStatusUpdated = "STAFF_STATUS_UPDATED"
	ActionStaffVenuesUpdated = "STAFF_VENUES_UPDATED"

	ActionBookingPaymentVerified = "BOOKING_PAYMENT_VERIFIED"
	ActionBookingPaymentRejected = "BOOKING_PAYMENT_REJECTED"
	ActionBookingMarkedPaid      = "BOOKING_MARKED_PAID"
	ActionBookingCompleted       = "BOOKING_COMPLETED"
	ActionBookingCancelRefund    = "BOOKING_CANCEL_REFUND"

	ActionRefundApproved = "REFUND_APPROVED"
	ActionRefundRejected = "REFUND_REJECTED"

	ActionFinanceCreated = "FINANCE_CREATED"
	ActionFinanceUpdated = "FINANCE_UPDATED"
	ActionFinanceDeleted = "FINANCE_DELETED"
)
```

Entity types:

```go
const (
	EntityStaff              = "STAFF"
	EntityBooking            = "BOOKING"
	EntityRefund             = "REFUND"
	EntityFinanceTransaction = "FINANCE_TRANSACTION"
	EntityVenue              = "VENUE"
)
```

Request params:

```go
type CreateAuditLogParams struct {
	OwnerProfileID string
	ActorUserID    string
	ActorRole      string
	Action         string
	EntityType     string
	EntityID       *string
	Metadata       map[string]any
	IPAddress      *string
	UserAgent      *string
}
```

Response DTO:

```go
type AuditActorResponse struct {
	ID    *string `json:"id,omitempty"`
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email,omitempty"`
	Role  string  `json:"role"`
}

type AuditLogResponse struct {
	ID         string             `json:"id"`
	Actor      AuditActorResponse `json:"actor"`
	Action     string             `json:"action"`
	EntityType string             `json:"entity_type"`
	EntityID   *string            `json:"entity_id,omitempty"`
	Metadata   map[string]any     `json:"metadata"`
	IPAddress  *string            `json:"ip_address,omitempty"`
	UserAgent   *string            `json:"user_agent,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
}
```

Query DTO:

```go
type AuditLogQuery struct {
	Action      string `form:"action"`
	EntityType  string `form:"entity_type"`
	ActorUserID string `form:"actor_user_id" binding:"omitempty,uuid"`
	StartDate   string `form:"start_date" binding:"omitempty,datetime=2006-01-02"`
	EndDate     string `form:"end_date" binding:"omitempty,datetime=2006-01-02"`
	Page        int    `form:"page" binding:"omitempty,min=1"`
	Limit       int    `form:"limit" binding:"omitempty,min=1,max=100"`
}
```

### `repository.go`

Minimal methods:

```go
type Repository interface {
	Create(ctx context.Context, params CreateAuditLogParams) error
	ListByOwner(ctx context.Context, ownerProfileID string, query AuditLogQuery) ([]AuditLogResponse, int, error)
}
```

Insert query:

```sql
INSERT INTO owner_audit_logs (
  owner_profile_id,
  actor_user_id,
  actor_role,
  action,
  entity_type,
  entity_id,
  metadata,
  ip_address,
  user_agent
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);
```

List query wajib join users:

```sql
SELECT
  l.id::text,
  l.actor_user_id::text,
  u.name,
  u.email,
  l.actor_role,
  l.action,
  l.entity_type,
  l.entity_id::text,
  l.metadata,
  l.ip_address,
  l.user_agent,
  l.created_at
FROM owner_audit_logs l
LEFT JOIN users u ON u.id = l.actor_user_id
WHERE l.owner_profile_id = $1
ORDER BY l.created_at DESC
LIMIT $n OFFSET $n;
```

Filters yang harus didukung:

- `action`
- `entity_type`
- `actor_user_id`
- `start_date`
- `end_date`

### `service.go`

Minimal methods:

```go
type Service interface {
	Record(ctx context.Context, params CreateAuditLogParams)
	ListOwnerLogs(ctx context.Context, ownerProfileID string, query AuditLogQuery) ([]AuditLogResponse, int, error)
}
```

Penting:

- `Record` tidak return error ke caller.
- Kalau repository gagal, cukup `log.Printf("failed to write audit log: %v", err)`.
- `ListOwnerLogs` return error normal.

### `handler.go`

Endpoint:

```text
GET /owner/audit-logs
```

Rule:

- Harus authenticated.
- Harus owner workspace.
- Harus actual owner, bukan staff.

Response:

```json
{
  "data": [],
  "total": 0,
  "page": 1,
  "limit": 20,
  "total_pages": 0
}
```

Gunakan helper pagination existing kalau ada, misalnya `httputil.NewPaginatedResponse`.

### Acceptance Criteria Phase 2

- Package compile.
- Endpoint `/owner/audit-logs` actual-owner-only.
- Staff mendapat `403` saat akses `/owner/audit-logs`.
- Response audit logs menampilkan actor name/email jika actor masih ada.

---

## Phase 3 - Wiring Audit Service ke `main.go`

Update:

```text
apps/api/cmd/api/main.go
```

Tambahkan:

- audit repository
- audit service
- audit handler
- register route `/owner/audit-logs`

Ikuti pattern dependency injection yang sudah ada di repo.

Jangan membuat global singleton baru.

Acceptance:

- API start.
- Route muncul di log Gin.
- `go test ./...` tetap compile.

---

## Phase 4 - Audit Staff Management

Target file:

```text
apps/api/internal/staff/handler.go
apps/api/internal/staff/service.go
apps/api/internal/staff/repository.go
```

Rekomendasi integrasi:

- Audit dilakukan di handler setelah aksi sukses.
- Handler punya dependency `audit.Service`.
- Kalau ingin lebih bersih, service boleh return old/new data, tapi jangan refactor besar.

Actions:

### `POST /owner/staff`

Audit:

```text
STAFF_CREATED
```

Entity:

```text
STAFF
```

Metadata minimal:

```json
{
  "email": "...",
  "role": "MANAGER",
  "permissions": [],
  "venue_ids": []
}
```

### `PUT /owner/staff/:id`

Audit:

```text
STAFF_UPDATED
```

Metadata minimal:

```json
{
  "role": "...",
  "permissions": [],
  "venue_ids": []
}
```

### `PATCH /owner/staff/:id/status`

Audit:

```text
STAFF_STATUS_UPDATED
```

Metadata:

```json
{
  "old_status": "ACTIVE",
  "new_status": "INACTIVE"
}
```

Untuk mendapatkan `old_status`, fetch staff sebelum update.

### `PUT /owner/staff/:id/venues`

Audit:

```text
STAFF_VENUES_UPDATED
```

Metadata:

```json
{
  "old_venue_ids": [],
  "new_venue_ids": []
}
```

Untuk mendapatkan `old_venue_ids`, fetch staff sebelum update.

Acceptance:

- Semua aksi staff sukses menghasilkan row di `owner_audit_logs`.
- Actor role harus `OWNER`.
- Staff tetap tidak bisa akses endpoint staff management.

---

## Phase 5 - Audit Booking Actions

Target file:

```text
apps/api/internal/bookings/handler.go
apps/api/internal/bookings/service.go
apps/api/internal/bookings/repository.go
```

Actions:

### `PATCH /owner/bookings/:id/verify-payment`

Jika approve:

```text
BOOKING_PAYMENT_VERIFIED
```

Jika reject:

```text
BOOKING_PAYMENT_REJECTED
```

Metadata:

```json
{
  "booking_id": "...",
  "is_approved": true,
  "new_status": "PAID"
}
```

Catatan:

- Repository `VerifyPayment(ctx, ownerUserID, bookingID, isApproved)` saat ini menerima `ownerUserID`, tapi service mengirim `ownerCtx.ActorUserID`.
- Nama param boleh diganti menjadi `actorUserID` supaya tidak membingungkan.
- Ledger `created_by_user_id` harus tetap actor.

### `PATCH /owner/bookings/:id/mark-paid`

Audit:

```text
BOOKING_MARKED_PAID
```

Metadata:

```json
{
  "booking_id": "...",
  "new_status": "PAID"
}
```

### `PATCH /owner/bookings/:id/complete`

Audit:

```text
BOOKING_COMPLETED
```

Metadata:

```json
{
  "booking_id": "...",
  "new_status": "COMPLETED"
}
```

Catatan:

- Saat ini `CompleteBooking` belum mengirim actor ke repository.
- Minimal audit log di handler/service cukup.
- Kalau ingin ledger/kolom khusus untuk completed_by belum perlu di v1.3.1.

### `PATCH /owner/bookings/:id/cancel-refund`

Audit:

```text
BOOKING_CANCEL_REFUND
```

Metadata:

```json
{
  "booking_id": "...",
  "reason": "...",
  "amount": 100000,
  "venue_id": "..."
}
```

Acceptance:

- Staff verify payment menghasilkan audit log actor staff.
- Staff cancel refund menghasilkan audit log actor staff.
- Booking Venue B yang ditolak tidak perlu menghasilkan audit log sukses.
- Optional: log failed attempts hanya jika ada kebutuhan security audit, bukan scope v1.3.1.

---

## Phase 6 - Audit Refund Actions

Target file:

```text
apps/api/internal/refunds/handler.go
apps/api/internal/refunds/service.go
apps/api/internal/refunds/repository.go
```

Actions:

### `PATCH /owner/refund-requests/:id/approve`

Audit:

```text
REFUND_APPROVED
```

Metadata:

```json
{
  "refund_request_id": "...",
  "booking_id": "...",
  "owner_note": "...",
  "amount": 100000
}
```

### `PATCH /owner/refund-requests/:id/reject`

Audit:

```text
REFUND_REJECTED
```

Metadata:

```json
{
  "refund_request_id": "...",
  "booking_id": "...",
  "owner_note": "..."
}
```

Catatan:

- `booking_refund_requests.reviewed_by_user_id` sudah ada dan harus tetap actor.
- Refund ledger `created_by_user_id` juga harus tetap actor.
- Audit log adalah riwayat terpusat agar owner mudah membaca.

Acceptance:

- Approve refund oleh staff menghasilkan:
  - `booking_refund_requests.reviewed_by_user_id = staff user id`
  - `owner_finance_transactions.created_by_user_id = staff user id`
  - `owner_audit_logs.action = REFUND_APPROVED`

---

## Phase 7 - Audit Finance Actions

Target file:

```text
apps/api/internal/finance/handler.go
apps/api/internal/finance/service.go
apps/api/internal/finance/repository.go
```

Actions:

### `POST /owner/finance/transactions`

Audit:

```text
FINANCE_CREATED
```

Metadata:

```json
{
  "venue_id": "...",
  "type": "INCOME",
  "category": "...",
  "amount": 12000,
  "transaction_date": "YYYY-MM-DD"
}
```

### `PATCH /owner/finance/transactions/:id`

Audit:

```text
FINANCE_UPDATED
```

Metadata:

```json
{
  "before": {},
  "after": {},
  "changed_fields": []
}
```

Cara implementasi:

- Fetch existing transaction sebelum update.
- Update transaction.
- Compare field sederhana:
  - venue_id
  - type
  - category
  - amount
  - transaction_date
  - payment_method
  - description

### `DELETE /owner/finance/transactions/:id`

Audit:

```text
FINANCE_DELETED
```

Metadata:

```json
{
  "deleted_transaction": {}
}
```

Cara implementasi:

- Fetch transaction sebelum delete.
- Insert audit log setelah delete sukses, metadata berisi snapshot sebelum delete.

Catatan penting:

- v1.3.1 boleh tetap hard delete, asal audit log menyimpan snapshot.
- Soft delete boleh ditulis sebagai backlog v1.4, jangan dipaksakan di v1.3.1.

Acceptance:

- Staff create finance Venue A menghasilkan audit log actor staff.
- Staff update finance menghasilkan before/after.
- Staff delete finance menghasilkan snapshot transaksi yang dihapus.
- Staff tetap ditolak untuk transaksi Venue B.

---

## Phase 8 - Frontend Minimal Audit Logs Page

Files baru:

```text
apps/web/src/pages/owner/OwnerAuditLogsPage.tsx
apps/web/src/types/audit.ts
```

Update existing:

```text
apps/web/src/App.tsx
apps/web/src/components/Navbar.tsx
apps/web/src/lib/api.ts
```

Route:

```text
/owner/audit-logs
```

Menu label:

```text
Riwayat Aktivitas
```

Rule:

- Actual owner only.
- Staff tidak perlu lihat menu.

Columns:

- Waktu
- Actor
- Action
- Entity
- Metadata ringkas

Filters minimal:

- action
- entity_type
- date range

Jangan terlalu dekoratif. Ini operational page, harus mudah discan.

Acceptance:

- Owner bisa buka `/owner/audit-logs`.
- Staff diarahkan/ditolak sesuai `ProtectedRoute`.
- Data actor name/email tampil.
- Empty state jelas.

---

## Phase 9 - Tests

### Backend Tests

Tambahkan/ubah test untuk:

1. Audit repository create/list.
2. Actual owner bisa list audit logs.
3. Staff tidak bisa list audit logs.
4. Staff create finance menghasilkan `FINANCE_CREATED`.
5. Staff update finance menghasilkan `FINANCE_UPDATED`.
6. Staff delete finance menghasilkan `FINANCE_DELETED`.
7. Staff verify payment menghasilkan `BOOKING_PAYMENT_VERIFIED`.
8. Staff approve refund menghasilkan `REFUND_APPROVED`.
9. Owner update staff venues menghasilkan `STAFF_VENUES_UPDATED`.

### E2E Update

Update file QA jika dipakai:

```text
outputs/qa/staff-roles-v1.3/e2e_staff_roles_v13.mjs
```

Tambahkan assertion:

- Setelah staff create finance, owner audit logs memuat `FINANCE_CREATED`.
- Setelah staff verify payment, owner audit logs memuat `BOOKING_PAYMENT_VERIFIED`.
- Setelah staff approve refund, owner audit logs memuat `REFUND_APPROVED`.
- Staff akses `/owner/audit-logs` mendapat `403`.

### Commands Wajib

```powershell
cd D:\project\lapangGo\apps\api
$env:GOCACHE='D:\project\lapangGo\.gocache'
go test ./...
```

```powershell
cd D:\project\lapangGo\apps\web
cmd /c npm run build
cmd /c npm run lint
```

```powershell
cd D:\project\lapangGo
git diff --check
```

---

## Phase 10 - Manual QA Checklist

1. Login owner.
2. Create staff.
3. Login staff.
4. Staff create finance transaction.
5. Staff verify payment.
6. Staff approve refund.
7. Login owner.
8. Buka `/owner/audit-logs`.
9. Pastikan terlihat:
   - nama staff
   - email staff
   - action
   - waktu
   - entity
10. Staff coba buka `/owner/audit-logs`.
11. Staff harus ditolak.

---

## Known Limitations Tetap di v1.3.1

Hal berikut jangan diselesaikan di scope ini kecuali diminta eksplisit:

- Forced reset password staff saat login pertama.
- Reset password staff oleh owner.
- Forgot password khusus staff.
- Soft delete finance transaction.
- Full failed-attempt security audit.
- Export CSV audit logs.

---

## Definition of Done

Implementasi dianggap selesai jika:

- Migration 017 berhasil.
- Semua aksi penting masuk `owner_audit_logs`.
- Actor staff/owner bisa terlihat sebagai name/email di endpoint audit logs.
- `/owner/audit-logs` hanya actual owner.
- Staff tidak bisa lihat audit logs.
- Finance delete punya audit snapshot sebelum data hilang.
- Existing E2E Staff Roles v1.3 tetap PASS.
- Test backend PASS.
- Frontend build PASS.
- Lint tidak menambah warning baru.
- `git diff --check` PASS.

---

## Anti-Hallucination Notes untuk Antigravity

- Jangan menebak nama permission baru. Untuk v1.3.1, endpoint audit logs actual-owner-only saja.
- Jangan membuat role baru.
- Jangan mengubah enum `owner_staff_permission` kecuali benar-benar diperlukan.
- Jangan memecah migration 016.
- Jangan menghapus `created_by_user_id`.
- Jangan mengganti hard delete finance menjadi soft delete di v1.3.1.
- Jangan menampilkan audit logs lintas owner.
- Jangan membuat UI staff bisa melihat audit logs.
- Jangan menganggap `owner_user_id` sama dengan `actor_user_id`. Untuk staff, actor adalah staff user id, effective owner adalah owner user id.
