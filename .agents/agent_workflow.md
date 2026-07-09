# LapangGo Agent Workflow

This is the shared working process for Codex, Antigravity, and other agents. It adapts the agent-skills lifecycle to this repository without requiring a specific agent plugin.

## 1. Classify The Request

Before acting, decide which mode applies:

- Discussion: explain tradeoffs, do not edit files.
- Planning: produce a plan, do not implement until asked.
- Review: inspect for bugs, risks, regressions, missing tests, and unclear contracts.
- Implementation: change files, verify, and report results.
- QA/release: run checks, collect evidence, and document readiness or blockers.

When the user says "jangan implementasikan dulu", "diskusi dulu", or similar, stay in discussion/planning mode.

## 2. Load Context

Read only what is needed:

- Backend feature: route, handler, service, repository, DTO, tests, migrations.
- Frontend feature: page/component, API client, types, auth context, route guards.
- Data change: migrations, repository methods, seed/demo scripts, affected tests.
- Permission change: auth service, middleware, owner/staff access code, frontend role navigation.

Always check `git status --short` before editing. Existing unrelated changes are treated as user/agent work and must be preserved.

## 3. Plan The Slice

For non-trivial implementation, create a brief plan with:

- Goal
- Affected files or modules
- Acceptance criteria
- Verification commands
- Risks and assumptions

Keep the plan small enough that one slice can be reviewed and tested. If a plan grows large, split it.

## 4. Implement Conservatively

Prefer existing project patterns over new abstractions.

Backend expectations:

- Keep handlers thin and put business rules in services.
- Keep SQL/data access in repositories.
- Validate ownership, staff access, booking status, and role boundaries close to the service/repository boundary.
- Use explicit error handling and consistent HTTP responses.
- Add tests around calculations, state transitions, access control, and data persistence.

Frontend expectations:

- Keep API types in `apps/web/src/types`.
- Keep API calls in `apps/web/src/lib/api.ts` or the existing local pattern.
- Use existing components, Tailwind style, route guards, auth context, and toast patterns.
- Avoid marketing-page treatment for operational owner/staff tools.
- Make loading, empty, error, and disabled states explicit for user-facing flows.

Database expectations:

- Every schema change needs an up and down migration.
- Keep migrations deterministic and reversible where possible.
- Avoid destructive data changes unless explicitly approved.

## 5. Verify

Use the narrowest meaningful check first, then broaden as risk increases.

Common checks:

- Backend unit/package: `go test ./internal/<package>`
- Backend full: `go test ./...`
- Frontend build: `npm run build`
- Frontend lint: `npm run lint`
- Docker smoke: `scripts/smoke_test.ps1` or `scripts/smoke_test.sh`

If a command cannot run, record the reason and the residual risk.

## 6. Review Before Handoff

Before finishing, inspect the diff and ask:

- Did this touch only the intended scope?
- Are role/owner/staff boundaries still enforced?
- Are booking/payment/refund/promo state transitions still valid?
- Are migrations paired and compatible with existing data?
- Are API and frontend types still aligned?
- Did tests or build checks prove the changed behavior?

## 7. Report

Final reports should include:

- What changed
- Where it changed
- What was verified
- What was not verified, if anything
- Any follow-up risk worth tracking

Do not bury blockers. Name them plainly.

