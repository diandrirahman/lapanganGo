# Final Release QA Report

**Project:** LapanganGo
**Date:** 2026-06-27
**Status:** READY FOR LOCAL DEMO / RELEASE

## Executive Summary
A comprehensive Final Release QA / Smoke Test was conducted on the LapanganGo platform to ensure the system is ready for local deployment and demonstration. The tests focused on the critical user journeys encompassing authentication, venue discovery, owner management flows, and the complete booking & payment lifecycle.

Overall, the application successfully passed the extended smoke test suite, validating both basic API readiness and critical operational flows.

## Scope of Testing
The following critical paths were tested via automated E2E integration script (`scripts/smoke_test.js`):
1. **System Health:** Application (`/health`) and Database (`/db-health`).
2. **Authentication:** Customer and Owner registration, login, and JWT generation.
3. **Owner Onboarding:** Owner profile creation and Venue creation.
4. **Customer Discovery:** Searching venues (`/venues`) and viewing venue details (`/venues/:id`).
5. **Owner Management:** Court creation, operating hours setup, dashboard metrics validation, and blocked slot management.
6. **Booking & Payment Flow:** Court availability checking, booking creation (Customer), payment proof submission (Customer), listing owner bookings, and payment verification (Owner).

## Test Results

| Test Case | Description | Result |
| :--- | :--- | :--- |
| **API Healthcheck** | Validated `/health` endpoint responds with 200 OK | ✅ PASS |
| **DB Healthcheck** | Validated `/db-health` verifies Postgres connection | ✅ PASS |
| **User Registration** | Registered a Customer and Owner account successfully | ✅ PASS |
| **Authentication** | Validated login and JWT generation for both roles | ✅ PASS |
| **Owner Profile** | Created an owner profile with business details | ✅ PASS |
| **Venue Creation** | Created a venue linked to the owner profile | ✅ PASS |
| **Venue Search** | Verified the newly created venue appears in public search | ✅ PASS |
| **Venue Details** | Fetched venue details successfully by ID | ✅ PASS |
| **Court Creation** | Created a court linked to a valid Sport ID | ✅ PASS |
| **Operating Hours** | Configured operating hours for the newly created court | ✅ PASS |
| **Owner Metrics** | Validated dashboard metrics accurately reflect created venue | ✅ PASS |
| **Blocked Slots** | Verified Create, List, and Delete flows for blocked slots | ✅ PASS |
| **Court Availability** | Verified `GET /courts/:id/availability` endpoint responds with available slots | ✅ PASS |
| **Create Booking** | Customer successfully created a booking (`POST /bookings`) | ✅ PASS |
| **List Bookings** | Verified customer booking appears in `GET /bookings` | ✅ PASS |
| **Detail Booking** | Fetched specific booking details (`GET /bookings/:id`) | ✅ PASS |
| **Payment Proof** | Customer successfully submitted payment proof (`POST /bookings/:id/payment-proof`) | ✅ PASS |
| **Owner Bookings** | Owner successfully listed venue bookings (`GET /owner/venues/:id/bookings`) | ✅ PASS |
| **Verify Payment** | Owner successfully verified payment (`PATCH /owner/bookings/:id/verify-payment`) | ✅ PASS |

## Issues Found & Resolved

### 1. Blocking Bug: Rate Limiter on Healthchecks
- **Severity:** High (Blocking)
- **File/Area:** `apps/api/cmd/api/main.go`
- **Issue:** The API container was entering an `unhealthy` state because the Docker healthcheck was triggering a `429 Too Many Requests` error. This was caused by the `/health` and `/db-health` routes being registered *after* the global rate limiting middleware (100 req/min).
- **Resolution:** Moved the route definitions for `/health` and `/db-health` above `r.Use(generalRateLimiter)` in `main.go`. This exempts these critical observability endpoints from rate-limiting and stabilizes the container.

### 2. Minor Data Dependency: Venue Status Default (Test Setup)
- **Severity:** Low (Test Automation Only)
- **Issue:** Venues created via API default to `PENDING` status and do not appear in the public `/venues` search.
- **Test-Only Setup:** For the purpose of the smoke test, an explicit DB override was applied (`UPDATE venues SET status='ACTIVE'`) to enable end-to-end testing of the public search endpoint. In a real-world scenario, this is expected behavior (Admin approval).

### 3. Missing Filter Parameter: Owner Venue Bookings
- **Severity:** Medium
- **Issue:** The endpoint `GET /owner/venues/:id/bookings` requires the `date` query parameter for the DB query to function correctly, otherwise it returns no data.
- **Resolution:** Updated the QA smoke test script to include the `date` parameter in the query string as required by the API contract.

## Conclusion
The LapanganGo application is structurally sound across its critical paths, including the crucial booking and payment flows. The frontend and backend communicate securely using the finalized contracts from previous batches. The system is **Ready for Local Demo and Release**. No further features should be added at this stage to preserve stability.
