# E2E Booking Flow QA Manual Walkthrough (Step 6B)

This document contains a manual QA walkthrough using `curl` (compatible with Bash and PowerShell via `curl.exe` or `Invoke-RestMethod`) to verify the end-to-end booking flow for LapangGo API.

The target date for testing is **2026-12-01**. The timezone used is Asia/Jakarta (WIB).

## Prerequisite
1. Apply the database migrations.
2. Apply the seed data located in `docs/qa/step6_e2e_seed.sql`:
   ```bash
   psql -U postgres -d lapangango_db -f docs/qa/step6_e2e_seed.sql
   ```
3. Start the API locally: `go run ./cmd/api` in `apps/api` (default port 8080).

## Environment Variables
To make copy-pasting easier, you can export these variables into your terminal after getting the tokens.
We use these fixed UUIDs from the seeder:
- `VENUE_ID`: `88888888-8888-8888-8888-888888888888`
- `COURT_ID`: `99999999-9999-9999-9999-999999999999`
- `TEST_DATE`: `2026-12-01`
- `START_TIME`: `10:00`
- `END_TIME`: `11:00`
- `START_TIME_2`: `13:00`
- `END_TIME_2`: `14:00`

*PowerShell Users: Replace `export VAR="value"` with `$env:VAR="value"` and use `curl.exe` instead of `curl` alias.*

---

## 1. Authentication

### 1a. Login Customer
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"qa.customer@lapanggo.test", "password":"QaPass123!"}'
```
> **Action**: Copy the `token` from the JSON response and export it in your terminal:
> `export CUSTOMER_TOKEN="your_token_here"`

### 1b. Login Owner
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"qa.owner@lapanggo.test", "password":"QaPass123!"}'
```
> **Action**: Copy the `token` from the JSON response and export it in your terminal:
> `export OWNER_TOKEN="your_token_here"`

---

## 2. Happy Path Validation

### Step 5: Check Initial Availability
```bash
curl -X GET "http://localhost:8080/courts/99999999-9999-9999-9999-999999999999/availability?date=2026-12-01"
```
> **Expected**: In the response JSON array `slots`, find the object where `start_at` corresponds to 10:00:00 local time and `end_at` is 11:00:00. It should have `"status": "AVAILABLE"`.

### Step 6: Create Booking
```bash
curl -X POST http://localhost:8080/bookings \
  -H "Authorization: Bearer $CUSTOMER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"court_id":"99999999-9999-9999-9999-999999999999", "booking_date":"2026-12-01", "start_time":"10:00", "end_time":"11:00"}'
```
> **Expected**: HTTP 201 Created. Response contains `"status": "PENDING_PAYMENT"`.
> **Action**: Copy the new booking ID and export it:
> `export BOOKING_ID="the_booking_id"`

### Step 7: Check Availability Again
```bash
curl -X GET "http://localhost:8080/courts/99999999-9999-9999-9999-999999999999/availability?date=2026-12-01"
```
> **Expected**: In the `slots` array, the slot for 10:00 - 11:00 should now have `"status": "BOOKED"`.

### Step 8: Dummy Payment Confirm
```bash
curl -X POST http://localhost:8080/bookings/$BOOKING_ID/pay \
  -H "Authorization: Bearer $CUSTOMER_TOKEN"
```
> **Expected**: HTTP 200 OK. Message "Booking payment confirmed successfully", and booking status is now `"CONFIRMED"`.

### Step 9: Failsafe - Cancel Confirmed Booking
```bash
curl -X PATCH http://localhost:8080/bookings/$BOOKING_ID/cancel \
  -H "Authorization: Bearer $CUSTOMER_TOKEN"
```
> **Expected**: HTTP 409 Conflict. Message "booking cannot be cancelled in current status".

### Step 10: Failsafe - Pay Confirmed Booking Twice
```bash
curl -X POST http://localhost:8080/bookings/$BOOKING_ID/pay \
  -H "Authorization: Bearer $CUSTOMER_TOKEN"
```
> **Expected**: HTTP 409 Conflict. Message "booking already confirmed".

### Step 11: Owner Views Confirmed Booking
```bash
curl -X GET "http://localhost:8080/owner/venues/88888888-8888-8888-8888-888888888888/bookings?date=2026-12-01&status=CONFIRMED" \
  -H "Authorization: Bearer $OWNER_TOKEN"
```
> **Expected**: The booking appears in the `bookings` array with status `CONFIRMED`.

---

## 3. Cancel Availability Validation

### Step 12: Create Second Booking
```bash
curl -X POST http://localhost:8080/bookings \
  -H "Authorization: Bearer $CUSTOMER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"court_id":"99999999-9999-9999-9999-999999999999", "booking_date":"2026-12-01", "start_time":"13:00", "end_time":"14:00"}'
```
> **Expected**: HTTP 201 Created. `"status": "PENDING_PAYMENT"`.
> **Action**: Copy the new booking ID:
> `export BOOKING_ID_2="the_second_booking_id"`

### Step 13: Cancel Second Booking
```bash
curl -X PATCH http://localhost:8080/bookings/$BOOKING_ID_2/cancel \
  -H "Authorization: Bearer $CUSTOMER_TOKEN"
```
> **Expected**: HTTP 200 OK. `"status": "CANCELLED"`.

### Step 14: Check Availability for Cancelled Slot
```bash
curl -X GET "http://localhost:8080/courts/99999999-9999-9999-9999-999999999999/availability?date=2026-12-01"
```
> **Expected**: In the `slots` array, the slot for 13:00 - 14:00 should have `"status": "AVAILABLE"`.
