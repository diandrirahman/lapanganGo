# Version 1.4 Release Readiness

## Scope
- Staff onboarding invite flow.
- Staff password setup via token.
- Flexible invite/setup URL by frontend origin.
- Staff venue-scoped notifications.
- Staff routing and booking-access hardening.

## Verification
- Backend: `go test ./...` PASS
- Frontend build: `cmd /c npm run build` PASS
- Frontend lint: `cmd /c npm run lint` PASS
- Manual E2E QA v1.4: PASS

## Manual QA Coverage
- Owner creates staff without password.
- Owner copies invite link.
- Invite link opens on active frontend origin/port.
- Staff sets password from token.
- Staff cannot login before setup.
- Staff can login after setup.
- Staff only sees allowed venue data.
- Staff receives only scoped/permission-appropriate notifications.
- Staff notification click routes to owner management pages.
- Staff cannot create customer booking.

## Known Limitations
- Belum ada automatic email delivery.
- Invite/reset link masih ditampilkan di UI untuk copy manual.
- Production deployment harus mengatur trusted frontend domain/origin dengan benar.

## Release Decision
Status: READY FOR COMMIT
