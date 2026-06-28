# Roadmap: Next Phase (Production Preparation)

Following the successful Live Demo of the LapangGo MVP, the next phase focuses on preparing the application for real-world production deployment. Based on Codex's final review and recommendations, here are the core objectives for the upcoming sprint:

## 1. Environment Config Hardening
- Audit all environment variables (`.env`).
- Ensure all `VITE_USE_MOCK_*` flags are strictly set to `false` in the production configuration.
- Implement robust configuration validation on both the backend (Go) and frontend (Vite) startups.

## 2. End-to-End (E2E) Automation
- Implement automated E2E tests for the core critical flows using Cypress or Playwright:
  - Customer Booking & Payment Flow
  - Open Match (Mabar) Creation, Joining, and Cancellation Flow
  - Owner Dashboard & Venue Bookings Filter Flow

## 3. Owner Metrics Endpoint
- Build the `GET /owner/metrics` backend endpoint to aggregate real-time statistics:
  - Total Revenue (daily, weekly, monthly)
  - Active Bookings count
  - Most popular courts
- Update the `OwnerDashboardPage` frontend to consume this live data instead of static placeholders.

## 4. Real Court Management
- Implement backend endpoints:
  - `PATCH /courts/:id` (Edit Info)
  - `POST /courts/:id/schedules` (Atur Jadwal)
- Enable the corresponding buttons on the `OwnerCourtsPage` frontend and create the necessary modal/form UI to support these actions.

## 5. Deployment Readiness
- Configure CI/CD pipelines (e.g., GitHub Actions) for automated testing, linting, and building on every pull request.
- Prepare Dockerfiles for the Go API and Nginx configurations for the React/Vite frontend.
- Provision cloud infrastructure (Database, Compute instances/Containers) for staging and production environments.
