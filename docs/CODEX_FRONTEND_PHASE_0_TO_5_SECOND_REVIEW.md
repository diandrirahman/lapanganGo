# Codex Review - Frontend Phase 0.1 to Phase 1 Step 5

Reviewer role: Project Manager + Expert Software Developer  
Reviewed artifact: `docs/CODEX_FRONTEND_SUMMARY_REPORT_PHASE_0_TO_5.md`  
Reviewed code area: `apps/web/src`, with backend contract cross-check in `apps/api/internal`

## Decision

**REQUEST CHANGES before live backend demo.**

Frontend build and lint are now clean, and several previous issues have improved. However, the create booking flow still has one critical API contract mismatch that can break the demo when mock data is disabled.

## Verification Performed

- `cd apps/web && npm.cmd run lint` -> PASS, no output warnings.
- `cd apps/web && npm.cmd run build` -> PASS.
- Cross-checked frontend availability and booking payload with backend DTOs:
  - `apps/api/internal/availability/dto.go`
  - `apps/api/internal/bookings/dto.go`

## Required Fixes

### 1. P1 - Availability slot time is displayed and submitted in the wrong format for live backend

Backend availability returns slot times as `time.Time`:

- `apps/api/internal/availability/dto.go`
  - `start_at`
  - `end_at`

That means JSON will be RFC3339-like timestamps, for example:

```json
{
  "start_at": "2026-06-25T08:00:00+07:00",
  "end_at": "2026-06-25T09:00:00+07:00"
}
```

But frontend currently:

- displays `slot.start_at.substring(0, 5)`, which becomes `2026-` instead of `08:00`
- submits `start_time: selectedSlot.start_at`
- submits `end_time: selectedSlot.end_at`

Backend create booking requires `HH:mm`:

- `apps/api/internal/bookings/dto.go`
  - `start_time` binding `datetime=15:04`
  - `end_time` binding `datetime=15:04`

Expected fix:

- Add a small formatter/helper in frontend to normalize availability timestamps to `HH:mm`.
- Use the formatted value for both display and `POST /bookings` payload.
- Keep compatibility with mock slots if mock data still uses `HH:mm`, or update mock availability to use ISO timestamps like backend.

Acceptance criteria:

- Live availability displays `08:00 - 09:00`, not `2026- - 2026-`.
- Create booking sends:

```json
{
  "court_id": "...",
  "booking_date": "2026-06-25",
  "start_time": "08:00",
  "end_time": "09:00"
}
```

- Booking creation succeeds with demo seed data and a valid customer token.

### 2. P2 - Mabar empty state check uses raw env string

`apps/web/src/components/MabarSection.tsx` correctly defines:

```ts
const useMockMabar = import.meta.env.VITE_USE_MOCK_MABAR === 'true';
```

But the empty state condition later uses raw env:

```tsx
matches.length === 0 && !import.meta.env.VITE_USE_MOCK_MABAR
```

With `.env` containing `VITE_USE_MOCK_MABAR=false`, the value is the string `"false"` and remains truthy. Result: if live backend returns zero mabar records, the UI can render an empty blank grid instead of `EmptyState`.

Expected fix:

```tsx
matches.length === 0 && !useMockMabar
```

Acceptance criteria:

- When mock mabar is disabled and API returns empty list, the empty state appears.
- When mock mabar is enabled, prototype cards still render.

### 3. P2 - Report overclaims readiness and contains stale technical claims

The report still contains several inaccurate statements that can confuse QA handoff:

- It says court list uses `GET /venues/:id/courts`, but current code uses `GET /venues/:id` and reads `venue.courts`.
- It says availability shows estimated price, but the current slot UI does not show price.
- It says unauthenticated booking shows a dialog warning, while current code redirects to `/login`.
- Integration guide mentions seeing booking history, but `/bookings` is currently still a Step 6 placeholder page.
- Integration guide does not explicitly include `VITE_USE_MOCK_AUTH=false`, even though auth has its own mock switch.
- Build output in the report still has mojibake characters such as `âœ“` and `â”‚`.

Expected fix:

- Update `docs/CODEX_FRONTEND_SUMMARY_REPORT_PHASE_0_TO_5.md` so it accurately reflects the implementation.
- Add `VITE_USE_MOCK_AUTH=false` to live backend demo env instructions.
- Remove or reword claims for features not implemented yet, especially booking history.

### 4. P3 - Frontend venue types should match backend contract more closely

`apps/web/src/types/venue.ts` currently marks `surface_type` as required:

```ts
surface_type: string;
```

Backend public court response can omit nullable fields. Safer type:

```ts
surface_type?: string | null;
```

`PublicVenuesResponse` also declares `message` and `total`, while the current backend public venues response is effectively `venues`, `page`, and `limit`.

Expected fix:

- Align frontend public venue types with actual backend response shape.
- Keep only fields that are guaranteed, and mark nullable/optional fields correctly.

## Positive Notes

- Lint warning from `AuthContext` has been intentionally handled.
- Mock auth is now separated via `VITE_USE_MOCK_AUTH`.
- Venue detail no longer depends on nonexistent `GET /venues/:id/courts` route in code.
- Frontend build is passing.

## Recommended Next Action for Antigravity

Prioritize the P1 availability time normalization first, then P2 mabar empty state and report correction. After that, run a real manual smoke test with:

1. `apps/api`: demo seed already populated.
2. Backend API running on `http://localhost:8080`.
3. `apps/web/.env`:

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_USE_MOCK_VENUE=false
VITE_USE_MOCK_MABAR=false
VITE_USE_MOCK_AUTH=false
```

4. Frontend `npm run dev`.
5. Login with a seeded customer account or demo token flow.
6. Open a venue, select a court, pick an available slot, create booking, and confirm `/bookings` redirect occurs without backend validation error.

