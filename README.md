# LapanganGo

LapanganGo is a sports venue booking API built with Go, Gin, PostgreSQL, and Redis.

## Prerequisites

- Go 1.26.4
- Docker and Docker Compose

## Local Setup

Start PostgreSQL and Redis:

```bash
docker compose up -d
```

Create a local environment file:

```bash
cp apps/api/.env.example apps/api/.env
```

Update `apps/api/.env` if your local database credentials or port are different.

Run the API:

```bash
cd apps/api
go run ./cmd/api
```

Run tests:

```bash
cd apps/api
go test ./...
```

## Environment Variables

The API reads configuration from `apps/api/.env` or system environment variables.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `APP_PORT` | No | `8080` | HTTP server port |
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `JWT_SECRET` | Yes | - | Secret used to sign JWT access tokens |
| `JWT_EXPIRES_IN_HOURS` | No | `24` | JWT expiry duration in hours |

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

Apply migrations with your preferred PostgreSQL migration tool, or run the SQL files in order against the local database.

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
