# Codex Review - MVP Hardening and Demo Polish

Reviewer role: Project Manager + Expert Software Developer  
Reviewed artifact: `docs/ANTIGRAVITY_MVP_HARDENING_AND_DEMO_POLISH_REPORT_FOR_CODEX.md`  
Reviewed date: 2026-06-26

## Decision

**REQUEST CHANGES before final demo approval.**

Most of the hardening work is good and the automated checks pass. However, there is one live-backend runtime mismatch in the owner venue bookings page that can crash the owner flow when bookings exist.

## Verification Performed

- `cd apps/api && go test ./...` -> PASS.
- `cd apps/web && npm.cmd run lint` -> PASS.
- `cd apps/web && npm.cmd run build` -> PASS.

## What Looks Good

- Customer booking response enrichment is implemented for `GET /bookings` and `GET /bookings/:id`.
- Backend `BookingVenueSummary` now includes `address` and `city`.
- Backend `BookingCourtSummary` now includes `sport_name`.
- Mabar response now includes `host_user_id`.
- Frontend Mabar detail now uses `user.id === match.host_user_id` for host detection.
- Confirm modal has replaced native confirm/alert in customer cancel and Mabar leave/cancel/error flows.
- Owner court edit/schedule actions are disabled as `Segera`.
- Mobile navbar is now present.
- Owner booking filters pass `date` and `status` query parameters to the API wrapper.

## Blocking Finding

### P1 - OwnerVenueBookingsPage uses the wrong frontend response type

Backend owner bookings return `OwnerBookingResponse` shape:

```json
{
  "id": "...",
  "customer": {
    "id": "...",
    "name": "...",
    "email": "...",
    "phone": "..."
  },
  "venue": {
    "id": "...",
    "name": "..."
  },
  "court": {
    "id": "...",
    "name": "..."
  },
  "booking_date": "...",
  "start_time": "...",
  "end_time": "...",
  "total_price": 200000,
  "status": "CONFIRMED"
}
```

But frontend currently types `fetchOwnerVenueBookings(...)` as `Promise<Booking[]>`, and `OwnerVenueBookingsPage` reads fields from the customer booking shape:

```tsx
booking.court_id.substring(...)
booking.customer_id.substring(...)
```

References:

- `apps/api/internal/bookings/dto.go`
  - `OwnerBookingResponse` has `customer`, `venue`, and `court`.
- `apps/web/src/lib/api.ts`
  - `fetchOwnerVenueBookings(...): Promise<Booking[]>`
- `apps/web/src/pages/owner/OwnerVenueBookingsPage.tsx`
  - reads `booking.customer_id`
  - falls back to `booking.court_id`

Impact:

- If an owner venue has bookings, `booking.customer_id` is `undefined`.
- Rendering can throw `Cannot read properties of undefined (reading 'substring')`.
- This breaks the owner flow that the report claims is working.

Required fix:

1. Add a dedicated frontend type, for example:

```ts
export interface OwnerBooking {
  id: string;
  customer: {
    id: string;
    name: string;
    email: string;
    phone?: string | null;
  };
  venue: VenueSummary;
  court: CourtSummary;
  booking_date: string;
  start_time: string;
  end_time: string;
  total_price: number;
  status: 'PENDING_PAYMENT' | 'PAID' | 'CONFIRMED' | 'CANCELLED';
  created_at: string;
  updated_at: string;
}
```

2. Change:

```ts
fetchOwnerVenueBookings(...): Promise<Booking[]>
```

to:

```ts
fetchOwnerVenueBookings(...): Promise<OwnerBooking[]>
```

3. Update `OwnerVenueBookingsPage` to render:

```tsx
booking.customer.name
booking.customer.email
booking.customer.id.substring(0, 8)
booking.court.name
booking.venue.name
```

4. Remove fallback usage of `booking.customer_id` and `booking.court_id` in the owner bookings page.

## Non-Blocking Notes

### P2 - Action responses are not enriched

`CreateBooking`, `CancelBooking`, and `ConfirmBookingPayment` still return `toBookingResponse(...)`, which does not include `venue` and `court` summaries.

References:

- `apps/api/internal/bookings/service.go`
  - `CreateBooking` returns `toBookingResponse(created)`
  - `CancelBooking` returns `toBookingResponse(updated)`
  - `ConfirmBookingPayment` returns `toBookingResponse(updated)`

This is acceptable for the current frontend because the customer page refetches after cancel/payment. If future UI displays returned action payloads directly, this should be enriched or documented.

### P2 - Owner booking date filter behavior should be clarified

Backend defaults owner venue bookings to today when `date` is empty. Frontend label may make users think empty date means all dates.

Recommendation:

- Either set the default date input to today.
- Or clearly label the filter behavior as `Tanggal, default hari ini`.

### P3 - Report claim about lint bypass is inaccurate

The report says `oxlint` was bypassed due to Windows optional dependency issue. In this review, `npm.cmd run lint` passed successfully.

Please update the report so future reviewers do not chase a stale environment note.

## Recommended Prompt Back To Antigravity

```text
Codex reviewed the MVP Hardening and Demo Polish report.

Status: REQUEST CHANGES.

Automated checks pass:
- apps/api: go test ./...
- apps/web: npm run lint
- apps/web: npm run build

However, one owner-flow blocker remains:

Backend GET /owner/venues/:id/bookings returns OwnerBookingResponse with:
- customer: { id, name, email, phone }
- venue: { id, name }
- court: { id, name }

Frontend currently types fetchOwnerVenueBookings as Promise<Booking[]> and OwnerVenueBookingsPage reads:
- booking.customer_id.substring(...)
- booking.court_id.substring(...)

This can crash live owner bookings when data exists.

Please fix:
1. Add a dedicated OwnerBooking frontend type matching backend OwnerBookingResponse.
2. Change fetchOwnerVenueBookings return type to Promise<OwnerBooking[]>.
3. Update OwnerVenueBookingsPage to render booking.customer.name, booking.customer.email, booking.customer.id, booking.court.name, and booking.venue.name.
4. Remove customer_id/court_id usage from OwnerVenueBookingsPage.
5. Clarify owner booking date filter behavior: empty date currently means backend default today, not all dates.
6. Update the report: npm run lint now passes, so remove the stale note that oxlint was bypassed.

Then rerun:
- go test ./...
- npm run lint
- npm run build

Send back an updated report.
```

