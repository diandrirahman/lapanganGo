# Codex Review - Frontend MVP Completion Report

Reviewer role: Project Manager + Expert Software Developer  
Reviewed artifact: `docs/ANTIGRAVITY_FRONTEND_MVP_COMPLETION_REPORT_FOR_CODEX.md`  
Reviewed date: 2026-06-25

## Decision

**REQUEST CHANGES before calling the frontend MVP production-ready.**

Antigravity has made substantial progress: customer bookings, owner pages, mabar detail, and build/lint are now in place. However, several live-backend runtime mismatches remain. These are not TypeScript build failures, but they will break important demo flows when mocks are disabled.

## Verification Performed

- `cd apps/web && npm.cmd run lint` -> PASS.
- `cd apps/web && npm.cmd run build` -> PASS.
- Cross-checked frontend calls against backend Mabar and booking handlers.

## Blocking Findings

### P1 - Homepage Mabar list reads the wrong response field

Backend returns:

```go
c.JSON(http.StatusOK, gin.H{"open_matches": matches})
```

Reference: `apps/api/internal/mabar/handler.go`

Frontend type currently declares:

```ts
export interface OpenMatchesResponse {
  matches: OpenMatch[];
}
```

Frontend consumer then does:

```ts
setMatches(data.matches || []);
```

References:

- `apps/web/src/types/mabar.ts`
- `apps/web/src/components/MabarSection.tsx`

Impact:

- With `VITE_USE_MOCK_MABAR=false`, live backend data will be ignored.
- The homepage Mabar section can show an empty state even when demo seed has open matches.

Required fix:

- Change the type to `open_matches: OpenMatch[]`.
- Update all consumers to read `data.open_matches`.
- Keep mock response shape consistent with backend.

### P1 - Create Mabar sends invalid `level` values

Backend accepts these exact level strings:

```go
"Beginner / Fun"
"Intermediate"
"Advanced"
"All Levels"
```

Reference: `apps/api/internal/mabar/service.go`

Frontend currently submits enum-like values:

```tsx
BEGINNER
INTERMEDIATE
ADVANCED
ALL_LEVELS
```

Reference: `apps/web/src/components/CreateMabarModal.tsx`

Impact:

- Creating Mabar from a confirmed booking will fail with `invalid level`.
- This breaks the demo flow: booking -> confirm payment -> create mabar.

Required fix:

- Change select values to backend-accepted labels:

```tsx
<option value="Beginner / Fun">Pemula / Fun</option>
<option value="Intermediate">Menengah</option>
<option value="Advanced">Mahir</option>
<option value="All Levels">Semua Level</option>
```

### P1 - Owner “Pesanan Masuk” navigation points to a route that does not exist

Registered route:

```tsx
/owner/venues/:id/bookings
```

But navbar and dashboard navigate to:

```tsx
/owner/bookings
```

References:

- `apps/web/src/App.tsx`
- `apps/web/src/components/Navbar.tsx`
- `apps/web/src/pages/owner/OwnerDashboardPage.tsx`

Impact:

- Owner clicking “Pesanan Masuk” lands on a blank/no-match route.
- This directly contradicts the report claim that owner booking monitoring is fully navigable.

Required fix options:

- Option A: Remove global `/owner/bookings` links and route users through `/owner/venues`, where each venue has “Lihat Pesanan”.
- Option B: Implement a real `/owner/bookings` aggregate page if backend supports or if frontend fetches all owner venues then bookings per venue.

For MVP, Option A is safer.

## Non-Blocking But Important Findings

### P2 - Mabar host/participant detection relies on display names

Current logic:

```ts
const isHost = user && user.name === match.host_name;
const isParticipant = participants.some(p => p.name === user?.name && p.status === 'JOINED');
```

Reference: `apps/web/src/pages/MabarDetailPage.tsx`

This is fragile because names are not unique and may differ from authenticated profile display names. Backend participant data includes `user_id`, but `open_match` detail does not expose `host_user_id`.

Recommendation:

- Use `participants.some(p => p.user_id === user.id && p.status === 'JOINED')` for participant detection.
- Ask backend to expose `host_user_id` in open match detail, or avoid host-only UI assumptions until backend provides it.

### P2 - Owner courts page uses public venue detail endpoint

`OwnerCourtsPage` uses `fetchVenueById`, which calls public `GET /venues/:id`.

This can work for active public venues, but it is not a true owner management endpoint and may hide inactive/private owner data. For MVP read-only viewing this is acceptable, but the report should not imply full owner court management if create/edit/schedule actions are placeholders.

### P2 - Report overstates “Production-Ready”

The report says the frontend is “Production-Ready (MVP)”. Current state is better described as:

**Frontend MVP shell and major flows implemented, pending live-backend runtime fixes and final smoke test.**

## Positive Notes

- Customer booking list, cancel, and confirm payment integrations are present.
- `/bookings` is no longer a placeholder.
- Status badges, loading, empty, and error states exist in the main customer booking page.
- Lint and production build pass.
- Owner venue list and per-venue booking page are present.

## Required Re-Verification After Fix

After Antigravity applies the fixes:

1. `cd apps/web && npm run lint`
2. `cd apps/web && npm run build`
3. Run live backend with demo seed.
4. Set frontend env:

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_USE_MOCK_VENUE=false
VITE_USE_MOCK_MABAR=false
VITE_USE_MOCK_AUTH=false
```

5. Smoke test:
   - Homepage shows live open matches.
   - Customer can create booking.
   - Customer can confirm payment.
   - Customer can create mabar from confirmed booking.
   - Mabar detail opens and join/leave works.
   - Owner dashboard links do not navigate to nonexistent routes.
   - Owner can open venue bookings through a valid venue route.

## Suggested Prompt Back To Antigravity

```text
Codex reviewed the Frontend MVP completion report and status is REQUEST CHANGES.

Please fix these blockers:

1. Mabar list response mismatch:
   - Backend returns { "open_matches": [...] }.
   - Frontend currently types/reads { matches: [...] }.
   - Update OpenMatchesResponse, fetchOpenMatches mock, and MabarSection to use open_matches.

2. Create Mabar level mismatch:
   - Backend accepts: "Beginner / Fun", "Intermediate", "Advanced", "All Levels".
   - Frontend currently sends: BEGINNER, INTERMEDIATE, ADVANCED, ALL_LEVELS.
   - Update CreateMabarModal select values to backend-accepted strings.

3. Owner bookings navigation broken:
   - App only registers /owner/venues/:id/bookings.
   - Navbar and OwnerDashboard navigate to /owner/bookings, which does not exist.
   - For MVP, remove/replace the global /owner/bookings links and route owner to /owner/venues first.

Also improve if possible:
   - Participant detection in MabarDetail should use participant.user_id === user.id, not participant name.
   - Do not claim full owner court management if create/edit/schedule buttons are still placeholders.

Run:
   - npm run lint
   - npm run build

Then update docs/ANTIGRAVITY_FRONTEND_MVP_COMPLETION_REPORT_FOR_CODEX.md with accurate status and a live-backend smoke test result.
```

