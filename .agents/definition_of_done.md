# LapangGo Definition Of Done

A task is done when the changed behavior is implemented, verified, and clearly reported.

## Required For Every Code Change

- The change matches the user's latest request.
- Unrelated user or agent changes are preserved.
- The implementation follows existing project structure.
- The diff is scoped to the task.
- Relevant tests, build checks, or manual verification have been run.
- Any skipped verification is explained.

## Backend Done Criteria

- Business logic lives in services, not handlers.
- SQL/data access lives in repositories.
- DTOs and response shapes are consistent with existing API style.
- Authenticated routes validate user identity and role.
- Owner/staff routes validate venue/resource access.
- State transitions are guarded, especially bookings, payments, refunds, promos, staff access, and audit logs.
- New or changed behavior has focused Go tests when practical.
- `go test ./...` is run for broad backend changes, or a targeted package test is run for narrow changes.

## Frontend Done Criteria

- TypeScript types match API responses.
- API calls use the existing API client pattern.
- Pages and components handle loading, empty, error, success, and disabled states where relevant.
- Role-based navigation and route guards remain consistent.
- UI remains responsive on mobile and desktop.
- `npm run build` is run for significant frontend changes.
- `npm run lint` is run when the change affects broad UI or style patterns.

## Database Done Criteria

- Schema changes include matching up and down migrations.
- Migration names use the existing numeric sequence.
- Rollback behavior is understood and documented if it is not fully reversible.
- Repository code and tests reflect the new schema.

## Security Done Criteria

Use extra scrutiny when work touches:

- Login, registration, JWT, password handling
- Owner/staff access
- Audit logs
- Finance, refunds, manual transactions
- Promo validation
- Booking/payment status transitions
- File uploads or user-provided content

For those areas:

- Do not expose secrets or sensitive data in logs.
- Keep auth errors generic where needed.
- Validate ownership and permissions server-side.
- Prefer deny-by-default behavior.
- Update tests or manual QA steps for access-control changes.

## Documentation Done Criteria

Update documentation when:

- A command changes.
- An environment variable changes.
- An API contract changes.
- A migration or operational step is important for future agents.
- A release-facing feature has known limitations.

Keep docs concise and factual. Avoid creating reports unless the user asked for one or the change is release/review oriented.

