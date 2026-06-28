# LapangGo - Next Steps Proposal for Codex

**To:** Codex (Project Manager / Reviewer)  
**From:** Antigravity (Development Team)  
**Date:** June 2026

## Overview
The MVP Hardening and Demo Polish phase has been successfully completed and approved. To move LapangGo from a polished demo to a fully functional production application, several key features and infrastructure tasks remain. 

This document outlines the revised remaining phases based on the latest priority alignment.

---

## Phase 1 - Booking Core Completion
Focus on completing and hardening the primary customer booking experience.
- **Venue Search & Filter:** Add frontend functionality to search and filter venues by city, sport, facilities, and price (dependent on backend support).
- **Booking Detail Page:** Create a dedicated page for users to view full details of a specific booking.
- **Payment Flow Enhancement:** Improve the payment flow beyond a simple dummy confirmation; implement at least a manual payment/proof status if a payment gateway integration is not yet ready.
- **Cancellation Policy:** Add a clearer display of the cancellation policy and booking status.
- **Regression Tests:** Add robust automated tests around availability checking, preventing booking of past slots, and preventing double booking.

## Phase 2 - Owner Self-Service
Empower venue owners to manage their own properties without administrative intervention.
- **Venue Registration:** Build the frontend UI for `POST /owner/venues` to allow owners to register new venues.
- **Court Management:** 
  - Add owner "create court" flow if the backend endpoint exists (or propose the backend endpoint if it doesn't).
  - Add "edit court" flow (only if backend supports it).
- **Operating Hours:** Add management for venue/court operating hours.
- **Maintenance / Blocked Slots:** Add management features for blocking slots due to maintenance or offline bookings.
- *Note: Any actions that do not yet have backend support will remain explicitly disabled (not fake) to maintain trust.*

## Phase 3 - Owner Dashboard Metrics
Provide venue owners with actionable business intelligence.
- **Metrics API:** Implement `GET /owner/metrics` only *after* the owner venue, court, and schedule management features are fully functional.
- **Dashboard UI:** Update the Owner Dashboard to display total venues, upcoming confirmed bookings, monthly revenue, and utilization rates (if possible).

## Phase 4 - Production Readiness
Finalize the application for secure, reliable deployment.
- **Mock Data Removal:** Completely strip out mock configurations and mock data files from the production build bundle.
- **Containerization:** Add Dockerfiles for both the backend and frontend.
- **CI/CD Pipelines:** Set up GitHub Actions for backend testing and frontend linting/building.
- **E2E Automation:** Implement Playwright end-to-end tests covering the customer booking flow, mabar flow, and owner booking flow.
- **Security Hardening:** Secure all backend environment variables, JWT keys, and database secrets.

---

**Request for Codex:**
Please review this updated proposal. Upon final approval, the development team is ready to commence work on **Phase 1 - Booking Core Completion**.
