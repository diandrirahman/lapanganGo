# LapangGo Agent Instructions

These instructions are the shared project-level rules for Codex, Antigravity, and any other AI coding agent working in this repository.

## Project Shape

LapangGo is a sports venue booking platform.

- Backend: Go, Gin, PostgreSQL, Redis, golang-migrate
- Frontend: React, Vite, Tailwind CSS, TypeScript
- Deployment: Docker, Docker Compose, Nginx
- Database migrations: `db/migrations`
- Backend app: `apps/api`
- Frontend app: `apps/web`

## Core Rule

Use `.agents/agent_workflow.md` as the working process and `.agents/definition_of_done.md` as the verification bar.

For non-trivial work, do not jump straight into code. First clarify the objective, identify affected areas, then make a small plan with acceptance criteria. For simple, low-risk fixes, keep the plan brief and proceed after reading the relevant code.

## Shared Workflow

1. Read the current task and confirm whether it is discussion-only, planning-only, review-only, or implementation.
2. Check repository state before editing. Do not overwrite unrelated user or agent changes.
3. Load only the context needed for the task.
4. For feature work, write or update a focused implementation plan before coding.
5. Implement in small vertical slices.
6. Add or update tests when behavior, permissions, calculations, or data persistence changes.
7. Run the narrowest useful verification first, then broader checks when the change touches shared behavior.
8. Report what changed, what was verified, and what remains risky or unverified.

## Safety Rules

- Never revert or delete work you did not create unless explicitly asked.
- Never modify unrelated files as cleanup.
- Never create frontend password encryption. Use HTTPS/TLS and server-side hashing.
- Treat auth, owner access, staff roles, finance, refunds, promos, audit logs, booking status, and migrations as high-risk areas.
- Database changes must include both `*.up.sql` and `*.down.sql` migrations unless the user explicitly scopes otherwise.
- Changes to API contracts must update frontend API/types or documentation when applicable.
- Changes to role or owner access must include tests or a clear manual verification path.

## Preferred Verification

Backend:

- From `apps/api`: `go test ./...`
- For targeted packages: `go test ./internal/<package>`

Frontend:

- From `apps/web`: `npm run build`
- Lint if relevant: `npm run lint`

Full stack/manual:

- Use Docker Compose and the smoke test scripts in `scripts/` when the task affects end-to-end flows.
- Record manual QA results in `docs/` only when the user asked for a report or the change is release-facing.

## Agent-Skills Compatibility

If the Addy Osmani `agent-skills` plugin is available, map work to these skills:

- Vague request: `interview-me` or `idea-refine`
- New feature or significant change: `spec-driven-development`
- Task breakdown: `planning-and-task-breakdown`
- Implementation: `incremental-implementation`
- Backend/API contracts: `api-and-interface-design`
- UI work: `frontend-ui-engineering`
- Tests: `test-driven-development`
- Broken behavior: `debugging-and-error-recovery`
- Review: `code-review-and-quality`
- Security-sensitive work: `security-and-hardening`
- Release readiness: `shipping-and-launch`

If the plugin is not available, follow the same workflow manually using the local `.agents/` docs.

## Antigravity Usage

Antigravity should read this root `AGENTS.md` and use the `.agents/` workflow docs as the local project standard.

When using Antigravity, start significant tasks with one of these instructions:

- "Ikuti `AGENTS.md` dan `.agents/agent_workflow.md`."
- "Buat plan dulu, jangan implement sebelum plan disetujui."
- "Setelah implement, cek `.agents/definition_of_done.md`."

If the `agent-skills` plugin is installed in Antigravity, prefer its native slash commands:

- `/spec` for feature definition
- `/planning` for task breakdown
- `/build` for incremental implementation
- `/test` for test-driven verification
- `/review` for code review
- `/code-simplify` for simplification
- `/ship` for release readiness
- `/webperf` for browser performance review

Use `/planning` instead of `/plan` in Antigravity because `/plan` can conflict with Antigravity's internal planning command.
