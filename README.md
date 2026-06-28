# LapanganGo

LapanganGo is a sports venue booking platform built with:
- **Backend:** Go, Gin, PostgreSQL, Redis, golang-migrate
- **Frontend:** React, Vite, Tailwind CSS, TypeScript
- **Deployment:** Docker, Docker Compose, Nginx

## Prerequisites

- Docker and Docker Compose
- Node.js 20+ (for local development)
- Go 1.26.4 (for local development)

## Environment Setup

Create local environment files for both Backend and Frontend:

```bash
# Backend Environment
cp apps/api/.env.example apps/api/.env

# Frontend Environment
cp apps/web/.env.example apps/web/.env
```

Ensure `apps/web/.env` contains `VITE_API_BASE_URL=/api` if using Docker.

## Running Locally (Full Stack Docker)

You can run the entire application stack (PostgreSQL, Redis, Go API, React Web) with a single command:

```bash
docker compose up --build -d
```

- **Frontend Web:** `http://localhost:3000`
- **Backend API:** `http://localhost:8080`
- **API Healthcheck:** `http://localhost:8080/health`
- **DB Healthcheck:** `http://localhost:8080/db-health`

### Smoke Testing
Verify the deployment with our smoke test scripts:
- Windows: `.\scripts\smoke_test.ps1`
- Unix/Linux: `./scripts/smoke_test.sh`

## Development Mode (Local without Docker)

If you prefer to run the services separately for development:

1. Start databases only:
```bash
docker compose up -d postgres redis
```

2. Run the Backend API:
```bash
cd apps/api
go run ./cmd/api
```

3. Run the Frontend Web:
Change `VITE_API_BASE_URL=http://localhost:8080` in `apps/web/.env`
```bash
cd apps/web
npm install
npm run dev
```

## Environment Variables

The API reads configuration from `apps/api/.env` or system environment variables.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `APP_PORT` | No | `8080` | HTTP server port |
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `REDIS_URL` | No | - | Redis connection string for rate limiting (optional) |
| `JWT_SECRET` | Yes | - | Secret used to sign JWT access tokens |
| `JWT_EXPIRES_IN_HOURS` | No | `24` | JWT expiry duration in hours |
| `BOOKING_PAYMENT_TTL_MINUTES` | No | `30` | Time-to-live for PENDING_PAYMENT bookings in minutes |
| `BOOKING_EXPIRY_SWEEP_INTERVAL_SECONDS` | No | `60` | Background sweep interval for expired bookings in seconds |
| `GENERAL_RATE_LIMIT_PER_MINUTE` | No | `100` | Rate limit for general routes per IP per minute |
| `AUTH_RATE_LIMIT_PER_MINUTE` | No | `100` | Rate limit for auth routes per IP per minute |

For the Frontend (`apps/web/.env`), use:
| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `VITE_API_BASE_URL` | No | `http://localhost:8080` | API base URL. Use `/api` in Docker Mode. |

## Database

Migrations live in `db/migrations`.

Current core tables:

- `users`
- `sports`
- `facilities`
- `owner_profiles`
- `venues`
- `venue_facilities`
- `courts`
- `court_operating_hours`
- `court_blocked_slots`

Migrations run automatically on API startup using golang-migrate. There is no need to apply migrations manually.

## API Overview

Health checks:

- `GET /health`
- `GET /db-health`

Authentication:

- `POST /auth/register`
- `POST /auth/login`
- `GET /auth/me`

Public availability:

- `GET /courts/:id/availability?date=YYYY-MM-DD`
  - Status slot yang dikembalikan:
    - `AVAILABLE`: slot bisa dipesan
    - `BLOCKED`: slot diblokir owner/maintenance
    - `BOOKED`: slot sudah overlap dengan booking aktif
  - Catatan: Frontend harus memperlakukan `BLOCKED` dan `BOOKED` sebagai disabled/unselectable.

Customer bookings:

- `POST /bookings`
- `GET /bookings`
- `GET /bookings/:id`
- `PATCH /bookings/:id/cancel`
- `POST /bookings/:id/pay`

*Note: The `/pay` endpoint is a dummy payment feature for MVP. It marks a `PENDING_PAYMENT` booking as `CONFIRMED`. This is not a real payment gateway integration.*

Open Match / Mabar (MVP):

- `GET /open-matches`
- `GET /open-matches/:id`
- `POST /bookings/:id/open-matches`
- `POST /open-matches/:id/join`
- `DELETE /open-matches/:id/join`
- `PATCH /open-matches/:id/cancel`

*Note: Open Match must originate from a `CONFIRMED` booking. Open matches are only listed and joinable while the source booking status is `CONFIRMED`. Payment for open match slots is informal for MVP. `remaining_slots` is calculated dynamically. Join/leave/cancel handles status updates. If the source booking is CANCELLED or any other status, the open match is excluded from public list and cannot accept new participants. The open match status itself is not automatically changed in MVP.*

Owner profile:

- `POST /owner/profile`
- `GET /owner/profile`
- `PUT /owner/profile`

Owner venues:

- `POST /owner/venues`
- `GET /owner/venues`
- `GET /owner/venues/:id`
- `PUT /owner/venues/:id`
- `PATCH /owner/venues/:id/status`
- `GET /owner/venues/:id/bookings?date=YYYY-MM-DD&status=PENDING_PAYMENT`

Owner courts:

- `POST /owner/venues/:id/courts`
- `GET /owner/venues/:id/courts`
- `GET /owner/courts/:id`
- `PUT /owner/courts/:id`
- `PATCH /owner/courts/:id/status`

Owner schedules:

- `GET /owner/courts/:id/operating-hours`
- `PUT /owner/courts/:id/operating-hours`

Owner blocked slots:

- `POST /owner/courts/:id/blocked-slots`
- `GET /owner/courts/:id/blocked-slots`
- `DELETE /owner/blocked-slots/:id`

Public registration currently creates customer accounts only. Owner onboarding should go through a dedicated owner registration or verification flow.
