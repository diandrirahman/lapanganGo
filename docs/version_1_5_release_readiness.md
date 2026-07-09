# LapangGo v1.5 Release Readiness

## Overview
This release implements email delivery capabilities for staff onboarding and password reset workflows.

## Key Changes
1. **Email Service Backend (`apps/api/internal/email`)**: Implemented `SMTPService` and `NoopService`.
2. **Staff Service Integration**: Integrated `emailService` into `CreateStaff`, `RegenerateInvite`, and `ResetPasswordToken`.
3. **Frontend UX Update**: Updated UI to handle email delivery failures gracefully and prompt the user to manually copy the invite URL. Added `email_delivery` to the API contracts.
4. **Local Mailpit Configuration**: Added Mailpit service to `docker-compose.yml` for local development. Set `EMAIL_DELIVERY_ENABLED=true` by default.
5. **Production SMTP Hardening**: `SMTP_USE_TLS` now controls SMTP transport mode instead of being documentation-only. `false` uses plain SMTP for local Mailpit, `true` uses STARTTLS by default, and port `465` uses implicit TLS.

## Tests & Validation
- Backend tests passed (`go test ./...`)
- Frontend lint and build passed (`npm run lint && npm run build`)
- Successfully implemented fallback notification on the Owner Staff page when email delivery fails.

## Deployment Notes
- Ensure the following environment variables are set correctly in production:
  - `EMAIL_DELIVERY_ENABLED=true`
  - `SMTP_HOST`
  - `SMTP_PORT`
  - `SMTP_USERNAME`
  - `SMTP_PASSWORD`
  - `SMTP_USE_TLS`
  - `SMTP_FROM_NAME`
  - `SMTP_FROM_EMAIL`
- SMTP transport behavior:
  - Local Mailpit: `SMTP_PORT=1025`, `SMTP_USE_TLS=false`
  - Production submission SMTP: `SMTP_PORT=587`, `SMTP_USE_TLS=true` for STARTTLS
  - Production implicit TLS SMTP: `SMTP_PORT=465`, `SMTP_USE_TLS=true`
- If email is disabled (`EMAIL_DELIVERY_ENABLED=false`), the system gracefully degrades to a manual link generation flow as before.
