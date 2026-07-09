# Antigravity Implementation Plan: Staff Roles v1.3 + Audit Trail v1.3.1 Stabilization

Target agent: Antigravity
Project: LapangGo
Mode: review, plan, implement only after approval, verify, final review

## Mission

Stabilize the current Staff Roles v1.3 and Audit Trail v1.3.1 work so it is safe enough to become the next milestone after the v1.2 release candidate.

Do not start a new product feature. This task is only about making the existing staff role, owner access, venue scope, and audit trail implementation correct, secure, tested, and release-ready.

## Required Agent-Skills Lifecycle

If the upstream `agent-skills` plugin is installed, use these Antigravity commands:

1. `/review`
2. `/planning`
3. `/build`
4. `/test`
5. `/review`
6. `/ship`

If the plugin is not installed, follow the same lifecycle manually using:

- `code-review-and-quality`
- `planning-and-task-breakdown`
- `incremental-implementation`
- `test-driven-development`
- `security-and-hardening`
- `shipping-and-launch`

Do not jump straight to code. Produce a short reviewed task list first and wait for approval.

## Source Documents

Read these before changing files:

- `AGENTS.md`
- `.agents/agent_workflow.md`
- `.agents/definition_of_done.md`
- `.agents/antigravity.md`
- `staff_roles_v1_3_implementation_plan.md`
- `staff_roles_v1_3_fix_implementation_plan.md`
- `staff_roles_v1_3_1_audit_trail_implementation_plan.md`
- `docs/version_1_2_release_readiness.md`

Treat `staff_roles_v1_3_fix_implementation_plan.md` as the primary v1.3 stabilization checklist.
Treat `staff_roles_v1_3_1_audit_trail_implementation_plan.md` as the primary audit trail checklist.

## Current Context

The repository already contains in-progress changes for:

- Staff role migrations: `db/migrations/016_staff_roles.*`
- Audit log migrations: `db/migrations/017_owner_audit_logs.*`
- Backend staff package: `apps/api/internal/staff`
- Backend owner access package/middleware: `apps/api/internal/owneraccess`, `apps/api/internal/middleware/owner_access.go`
- Backend audit package: `apps/api/internal/audit`
- Owner route changes across venues, courts, schedules, blocked slots, bookings, finance, promos, refunds, analytics
- Frontend staff management UI and audit log UI
- Auth context and role navigation changes

Preserve unrelated changes. Always run `git status --short` before editing.

## Phase 0: Baseline Review Before Code

Goal: understand what is already implemented and where it diverges from the plans.

Required checks:

- Inspect current diff, especially staff, owner access, audit, auth, route guards, migrations, and frontend staff/audit pages.
- Compare code against both staff role plans.
- Identify exact blockers before coding.
- Produce a compact task list with file/module targets.

Acceptance criteria:

- No code is changed during this phase.
- Antigravity reports the top risks and asks for approval before implementation.

## Phase 1: Staff Authorization Correctness

Goal: make staff access safe and usable.

Tasks:

- Ensure `STAFF` login is allowed only when `users.status = 'ACTIVE'`.
- Ensure old staff tokens are rejected if staff user, staff membership, or owner user becomes inactive.
- Ensure owner workspace middleware uses:
  - `actor_user_id` for the real actor
  - `effective_owner_user_id` for owner-owned resources
  - `owner_profile_id` for workspace scope
  - `allowed_venue_ids` for staff venue scope
- Ensure handlers do not use staff user ID as owner ID.
- Ensure refund permission naming is consistent as `REFUNDS_READ` and `REFUNDS_WRITE`.

Acceptance criteria:

- Staff with valid permission can use assigned owner routes.
- Staff without permission gets `403`.
- Staff trying to access another venue gets `404` or `403` consistently.
- Owner behavior remains unchanged.

## Phase 2: Venue Scope Enforcement

Goal: staff can only see and mutate assigned venues and data derived from those venues.

Required areas:

- Venues
- Courts
- Schedules
- Blocked slots
- Bookings
- Refunds
- Finance
- Promos
- Analytics

Rules:

- Owner sees all owner data.
- Staff with assigned venues sees only assigned venues.
- Staff with no venue assignment sees empty lists and cannot mutate direct resources.
- Staff must not see global owner-level finance/promo records where `venue_id IS NULL`, unless explicitly owner-only behavior already allows it for owners.

Acceptance criteria:

- Staff venue A cannot read or mutate venue B data.
- Staff no-venue account cannot infer owner data through list or detail endpoints.
- List endpoints return empty results for no-access staff rather than leaking all data.

## Phase 3: Staff Management Contract

Goal: owner staff management works end-to-end with a stable API/frontend contract.

Tasks:

- Validate staff venue assignment belongs to the same owner profile before insert/update.
- Deduplicate submitted venue IDs before DB insert.
- Align `GET /owner/staff` backend response and frontend parsing.
- Remove or implement unsupported revoke behavior; prefer using status `INACTIVE` for v1.3 unless a route already exists.
- Fix UI copy: empty venue access means no venue access, not all venues.

Acceptance criteria:

- Owner cannot assign another owner's venue to staff.
- Staff list loads in frontend.
- Staff update/status/venue access flows match backend routes.

## Phase 4: Audit Trail v1.3.1

Goal: owner can answer who did what, to what entity, when, and inside which owner workspace.

Tasks:

- Verify migration `017_owner_audit_logs` is paired and ordered after `016_staff_roles`.
- Verify audit records are scoped by `owner_profile_id`.
- Ensure audit actor uses actual actor user ID and role, not only effective owner.
- Ensure staff cannot access audit log endpoints in v1.3.1.
- Add or verify audit logging for staff management and critical owner actions already listed in the audit trail plan.
- Ensure audit logging failure does not break non-critical main flows unless it is intentionally part of the same critical transaction.

Acceptance criteria:

- Owner sees audit log entries for owner/staff actions.
- Staff gets forbidden on audit log endpoints.
- Audit entries contain action, entity type, entity ID when applicable, metadata, actor, and timestamp.

## Phase 5: Tests

Add or update targeted tests before broad verification.

Required backend test focus:

- Staff inactive user cannot login.
- Staff inactive membership cannot access owner routes with old token.
- Staff venue scope blocks out-of-scope venue/court/booking/refund/finance/promo access.
- Staff with no venue scope gets empty list behavior.
- Owner can still access all owner data.
- Staff venue assignment rejects another owner's venue.
- Audit log list is owner-only.
- Audit log repository/service handles metadata and actor join correctly.

Frontend test/build focus:

- TypeScript types match API responses.
- Staff list parses backend response correctly.
- Navbar and route guards use consistent permission names.
- Audit logs page is owner-only in UI.

## Phase 6: Verification Commands

Use PowerShell on Windows.

Backend targeted first:

```powershell
cd D:\project\lapangGo\apps\api
$env:GOCACHE='D:\project\lapangGo\.gocache'
go test ./internal/staff ./internal/owneraccess ./internal/audit ./internal/auth
```

Backend broad:

```powershell
cd D:\project\lapangGo\apps\api
$env:GOCACHE='D:\project\lapangGo\.gocache'
go test ./...
```

Frontend:

```powershell
cd D:\project\lapangGo\apps\web
cmd /c npm run build
cmd /c npm run lint
```

Known non-blocking warnings:

- `OwnerStaffPage.tsx` may have a `react-hooks/exhaustive-deps` warning for `fetchData`.
- Vite may warn about chunk size.

Do not treat those warnings as blockers unless the current task changes them.

## Phase 7: Manual QA Checklist

Run or document these before declaring release readiness:

- Owner creates staff with selected permissions and selected venue access.
- Staff logs in and receives fresh `staff_context`.
- Staff sees only permitted navigation.
- Staff can access assigned venue data.
- Staff cannot access unassigned venue data by URL or API.
- Staff with no venue access sees no owner data.
- Owner disables staff membership; old staff token loses owner access.
- Suspended staff user cannot login.
- Staff cannot manage staff.
- Staff cannot view audit logs.
- Owner can view audit logs.
- Staff actions create audit entries with actor = staff user.
- Owner actions create audit entries with actor = owner user.

## Phase 8: Handoff Report

Antigravity final response must include:

- Files changed
- Security-sensitive decisions made
- Tests run and results
- Manual QA performed or skipped
- Remaining known limitations
- Whether v1.3/v1.3.1 is ready for final regression

## Out Of Scope

Do not implement these in this stabilization task:

- Email invitations
- Forced password reset on first staff login
- Staff forgot-password/reset-password flow
- Multi-owner staff membership
- Superadmin panel
- Real payment gateway
- Owner payout
- Email/WhatsApp notifications

